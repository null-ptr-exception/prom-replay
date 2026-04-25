package vm

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"
)

type Client struct {
	baseURL    string
	httpClient *http.Client
}

func NewClient(baseURL string) *Client {
	return &Client{
		baseURL:    baseURL,
		httpClient: &http.Client{Timeout: 10 * time.Minute},
	}
}

func (c *Client) Export(ctx context.Context, start, end time.Time) (io.ReadCloser, error) {
	params := url.Values{}
	params.Set("match[]", `{__name__!=""}`)
	params.Set("start", fmt.Sprintf("%d", start.Unix()))
	params.Set("end", fmt.Sprintf("%d", end.Unix()))

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/api/v1/export?"+params.Encode(), nil)
	if err != nil {
		return nil, fmt.Errorf("creating export request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("executing export request: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, fmt.Errorf("export returned status %d", resp.StatusCode)
	}

	return resp.Body, nil
}

type exportLine struct {
	Metric     map[string]string `json:"metric"`
	Values     json.RawMessage   `json:"values"`
	Timestamps json.RawMessage   `json:"timestamps"`
}

func (c *Client) Import(ctx context.Context, runID string, data io.Reader) error {
	pr, pw := io.Pipe()

	go func() {
		defer pw.Close()
		scanner := bufio.NewScanner(data)
		scanner.Buffer(make([]byte, 0, 1024*1024), 10*1024*1024)
		for scanner.Scan() {
			var line exportLine
			if err := json.Unmarshal(scanner.Bytes(), &line); err != nil {
				pw.CloseWithError(fmt.Errorf("parsing export line: %w", err))
				return
			}
			line.Metric["run_id"] = runID
			out, err := json.Marshal(line)
			if err != nil {
				pw.CloseWithError(fmt.Errorf("marshaling import line: %w", err))
				return
			}
			if _, err := pw.Write(out); err != nil {
				return
			}
			if _, err := pw.Write([]byte("\n")); err != nil {
				return
			}
		}
		if err := scanner.Err(); err != nil {
			pw.CloseWithError(fmt.Errorf("scanning export data: %w", err))
		}
	}()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/api/v1/import", pr)
	if err != nil {
		return fmt.Errorf("creating import request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-ndjson")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("executing import request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("import returned status %d: %s", resp.StatusCode, string(body))
	}
	return nil
}

func (c *Client) DeleteSeries(ctx context.Context, runID string) error {
	params := url.Values{}
	params.Set("match[]", fmt.Sprintf(`{run_id=%q}`, runID))

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/api/v1/admin/tsdb/delete_series?"+params.Encode(), nil)
	if err != nil {
		return fmt.Errorf("creating delete request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("executing delete request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("delete returned status %d: %s", resp.StatusCode, string(body))
	}
	return nil
}

type labelValuesResponse struct {
	Status string   `json:"status"`
	Data   []string `json:"data"`
}

func (c *Client) LoadedRunIDs(ctx context.Context) ([]string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/api/v1/label/run_id/values", nil)
	if err != nil {
		return nil, fmt.Errorf("creating label values request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("executing label values request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("label values returned status %d: %s", resp.StatusCode, string(body))
	}

	var result labelValuesResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decoding label values response: %w", err)
	}
	return result.Data, nil
}

func (c *Client) Healthy(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/health", nil)
	if err != nil {
		return err
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("VM health check returned %d", resp.StatusCode)
	}
	return nil
}

func InjectRunID(data []byte, runID string) ([]byte, error) {
	var buf bytes.Buffer
	scanner := bufio.NewScanner(bytes.NewReader(data))
	scanner.Buffer(make([]byte, 0, 1024*1024), 10*1024*1024)
	for scanner.Scan() {
		var line exportLine
		if err := json.Unmarshal(scanner.Bytes(), &line); err != nil {
			return nil, fmt.Errorf("parsing line: %w", err)
		}
		line.Metric["run_id"] = runID
		out, err := json.Marshal(line)
		if err != nil {
			return nil, fmt.Errorf("marshaling line: %w", err)
		}
		buf.Write(out)
		buf.WriteByte('\n')
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
