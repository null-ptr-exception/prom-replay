package server

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"time"

	minioclient "github.com/rophy/prom-replay/replay-manager/internal/minio"
	"github.com/rophy/prom-replay/replay-manager/internal/model"
	"github.com/rophy/prom-replay/replay-manager/internal/vm"
)

type Server struct {
	vm    *vm.Client
	minio *minioclient.Client
	mux   *http.ServeMux
}

func New(vmClient *vm.Client, minioClient *minioclient.Client) *Server {
	s := &Server{
		vm:    vmClient,
		minio: minioClient,
		mux:   http.NewServeMux(),
	}
	s.mux.HandleFunc("GET /healthz", s.handleHealthz)
	s.mux.HandleFunc("POST /runs", s.handleCreateRun)
	s.mux.HandleFunc("GET /runs", s.handleListRuns)
	s.mux.HandleFunc("POST /runs/{id}/load", s.handleLoadRun)
	s.mux.HandleFunc("DELETE /runs/{id}/load", s.handleUnloadRun)
	s.mux.HandleFunc("DELETE /runs/{id}", s.handleDeleteRun)
	return s
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, DELETE, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	s.mux.ServeHTTP(w, r)
}

func (s *Server) handleHealthz(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	fmt.Fprint(w, "ok")
}

func (s *Server) handleCreateRun(w http.ResponseWriter, r *http.Request) {
	var req model.CreateRunRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, fmt.Sprintf("invalid request: %v", err), http.StatusBadRequest)
		return
	}
	if req.Start.IsZero() || req.End.IsZero() {
		http.Error(w, "start and end are required", http.StatusBadRequest)
		return
	}
	if !req.Start.Before(req.End) {
		http.Error(w, "start must be before end", http.StatusBadRequest)
		return
	}

	runID := time.Now().UTC().Format("20060102T150405Z")

	slog.Info("exporting from VM", "run_id", runID, "start", req.Start, "end", req.End)
	exportBody, err := s.vm.Export(r.Context(), req.Start, req.End)
	if err != nil {
		http.Error(w, fmt.Sprintf("export failed: %v", err), http.StatusInternalServerError)
		return
	}

	tmpFile, err := os.CreateTemp("", "prom-replay-export-*.jsonl")
	if err != nil {
		exportBody.Close()
		http.Error(w, fmt.Sprintf("creating temp file: %v", err), http.StatusInternalServerError)
		return
	}
	defer os.Remove(tmpFile.Name())

	n, err := io.Copy(tmpFile, exportBody)
	exportBody.Close()
	if err != nil {
		tmpFile.Close()
		http.Error(w, fmt.Sprintf("writing export data: %v", err), http.StatusInternalServerError)
		return
	}
	tmpFile.Close()

	dataFile, err := os.Open(tmpFile.Name())
	if err != nil {
		http.Error(w, fmt.Sprintf("reopening temp file: %v", err), http.StatusInternalServerError)
		return
	}
	defer dataFile.Close()

	meta := model.Meta{
		RunID:     runID,
		Start:     req.Start,
		End:       req.End,
		CreatedAt: time.Now().UTC(),
		Labels:    req.Labels,
	}

	slog.Info("uploading to MinIO", "run_id", runID, "size_bytes", n)
	if err := s.minio.PutRun(r.Context(), meta, dataFile, n); err != nil {
		http.Error(w, fmt.Sprintf("upload failed: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(meta)
}

func (s *Server) handleListRuns(w http.ResponseWriter, r *http.Request) {
	runs, err := s.minio.ListRuns(r.Context())
	if err != nil {
		http.Error(w, fmt.Sprintf("listing runs: %v", err), http.StatusInternalServerError)
		return
	}

	loadedIDs, err := s.vm.LoadedRunIDs(r.Context())
	if err != nil {
		http.Error(w, fmt.Sprintf("querying loaded runs: %v", err), http.StatusInternalServerError)
		return
	}

	loaded := make(map[string]bool, len(loadedIDs))
	for _, id := range loadedIDs {
		loaded[id] = true
	}
	for i := range runs {
		runs[i].Loaded = loaded[runs[i].RunID]
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(runs)
}

func (s *Server) handleLoadRun(w http.ResponseWriter, r *http.Request) {
	runID := r.PathValue("id")

	loadedIDs, err := s.vm.LoadedRunIDs(r.Context())
	if err != nil {
		http.Error(w, fmt.Sprintf("checking loaded runs: %v", err), http.StatusInternalServerError)
		return
	}
	for _, id := range loadedIDs {
		if id == runID {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]string{"status": "already loaded"})
			return
		}
	}

	slog.Info("downloading from MinIO", "run_id", runID)
	data, err := s.minio.GetData(r.Context(), runID)
	if err != nil {
		http.Error(w, fmt.Sprintf("downloading run: %v", err), http.StatusNotFound)
		return
	}
	defer data.Close()

	slog.Info("importing into VM", "run_id", runID)
	if err := s.vm.Import(r.Context(), runID, data); err != nil {
		http.Error(w, fmt.Sprintf("import failed: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "loaded"})
}

func (s *Server) handleUnloadRun(w http.ResponseWriter, r *http.Request) {
	runID := r.PathValue("id")

	slog.Info("deleting series from VM", "run_id", runID)
	if err := s.vm.DeleteSeries(r.Context(), runID); err != nil {
		http.Error(w, fmt.Sprintf("unload failed: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "unloaded"})
}

func (s *Server) handleDeleteRun(w http.ResponseWriter, r *http.Request) {
	runID := r.PathValue("id")

	loadedIDs, err := s.vm.LoadedRunIDs(r.Context())
	if err == nil {
		for _, id := range loadedIDs {
			if id == runID {
				slog.Info("unloading before delete", "run_id", runID)
				s.vm.DeleteSeries(r.Context(), runID)
				break
			}
		}
	}

	slog.Info("deleting from MinIO", "run_id", runID)
	if err := s.minio.DeleteRun(r.Context(), runID); err != nil {
		http.Error(w, fmt.Sprintf("delete failed: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "deleted"})
}
