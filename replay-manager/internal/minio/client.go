package minio

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
	"github.com/rophy/prom-replay/replay-manager/internal/model"
)

type Client struct {
	mc     *minio.Client
	bucket string
}

func NewClient(endpoint, accessKey, secretKey, bucket string, useSSL bool) (*Client, error) {
	mc, err := minio.New(endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(accessKey, secretKey, ""),
		Secure: useSSL,
	})
	if err != nil {
		return nil, fmt.Errorf("creating minio client: %w", err)
	}
	return &Client{mc: mc, bucket: bucket}, nil
}

func (c *Client) EnsureBucket(ctx context.Context) error {
	exists, err := c.mc.BucketExists(ctx, c.bucket)
	if err != nil {
		return fmt.Errorf("checking bucket: %w", err)
	}
	if !exists {
		if err := c.mc.MakeBucket(ctx, c.bucket, minio.MakeBucketOptions{}); err != nil {
			return fmt.Errorf("creating bucket: %w", err)
		}
	}
	return nil
}

func (c *Client) PutRun(ctx context.Context, meta model.Meta, data io.Reader, dataSize int64) error {
	metaBytes, err := json.Marshal(meta)
	if err != nil {
		return fmt.Errorf("marshaling meta: %w", err)
	}

	metaKey := fmt.Sprintf("runs/%s/meta.json", meta.RunID)
	_, err = c.mc.PutObject(ctx, c.bucket, metaKey, bytes.NewReader(metaBytes), int64(len(metaBytes)), minio.PutObjectOptions{
		ContentType: "application/json",
	})
	if err != nil {
		return fmt.Errorf("uploading meta: %w", err)
	}

	dataKey := fmt.Sprintf("runs/%s/data.jsonl", meta.RunID)
	_, err = c.mc.PutObject(ctx, c.bucket, dataKey, data, dataSize, minio.PutObjectOptions{
		ContentType: "application/x-ndjson",
	})
	if err != nil {
		return fmt.Errorf("uploading data: %w", err)
	}

	return nil
}

func (c *Client) GetMeta(ctx context.Context, runID string) (model.Meta, error) {
	key := fmt.Sprintf("runs/%s/meta.json", runID)
	obj, err := c.mc.GetObject(ctx, c.bucket, key, minio.GetObjectOptions{})
	if err != nil {
		return model.Meta{}, fmt.Errorf("getting meta: %w", err)
	}
	defer obj.Close()

	var meta model.Meta
	if err := json.NewDecoder(obj).Decode(&meta); err != nil {
		return model.Meta{}, fmt.Errorf("decoding meta: %w", err)
	}
	return meta, nil
}

func (c *Client) GetData(ctx context.Context, runID string) (io.ReadCloser, error) {
	key := fmt.Sprintf("runs/%s/data.jsonl", runID)
	obj, err := c.mc.GetObject(ctx, c.bucket, key, minio.GetObjectOptions{})
	if err != nil {
		return nil, fmt.Errorf("getting data: %w", err)
	}
	return obj, nil
}

func (c *Client) ListRuns(ctx context.Context) ([]model.RunInfo, error) {
	var runs []model.RunInfo
	seen := make(map[string]bool)

	for obj := range c.mc.ListObjects(ctx, c.bucket, minio.ListObjectsOptions{
		Prefix:    "runs/",
		Recursive: true,
	}) {
		if obj.Err != nil {
			return nil, fmt.Errorf("listing objects: %w", obj.Err)
		}

		parts := strings.SplitN(strings.TrimPrefix(obj.Key, "runs/"), "/", 2)
		if len(parts) != 2 {
			continue
		}
		runID := parts[0]
		if seen[runID] {
			continue
		}
		seen[runID] = true

		meta, err := c.GetMeta(ctx, runID)
		if err != nil {
			continue
		}

		dataKey := fmt.Sprintf("runs/%s/data.jsonl", runID)
		stat, err := c.mc.StatObject(ctx, c.bucket, dataKey, minio.StatObjectOptions{})
		var sizeBytes int64
		if err == nil {
			sizeBytes = stat.Size
		}

		runs = append(runs, model.RunInfo{
			Meta:      meta,
			SizeBytes: sizeBytes,
		})
	}

	return runs, nil
}

func (c *Client) DeleteRun(ctx context.Context, runID string) error {
	prefix := fmt.Sprintf("runs/%s/", runID)
	for obj := range c.mc.ListObjects(ctx, c.bucket, minio.ListObjectsOptions{
		Prefix:    prefix,
		Recursive: true,
	}) {
		if obj.Err != nil {
			return fmt.Errorf("listing for delete: %w", obj.Err)
		}
		if err := c.mc.RemoveObject(ctx, c.bucket, obj.Key, minio.RemoveObjectOptions{}); err != nil {
			return fmt.Errorf("removing %s: %w", obj.Key, err)
		}
	}
	return nil
}
