package server

import (
	"encoding/json"
	"fmt"
	"html/template"
	"net/http"
	"sort"

	"github.com/rophy/prom-replay/replay-manager/internal/model"
)

type grafanaDashboard struct {
	UID   string `json:"uid"`
	Title string `json:"title"`
	URL   string `json:"url"`
}

var uiTmpl = template.Must(template.New("ui").Funcs(template.FuncMap{
	"formatBytes": func(b int64) string {
		switch {
		case b < 1024:
			return fmt.Sprintf("%d B", b)
		case b < 1048576:
			return fmt.Sprintf("%.1f KB", float64(b)/1024)
		default:
			return fmt.Sprintf("%.1f MB", float64(b)/1048576)
		}
	},
	"unixMilli": func(t interface{ UnixMilli() int64 }) int64 {
		return t.UnixMilli()
	},
}).Parse(`<!DOCTYPE html>
<html>
<head>
<meta charset="utf-8">
<title>Prom Replay - Run Management</title>
<style>
  * { box-sizing: border-box; margin: 0; padding: 0; }
  body { font-family: Inter, -apple-system, sans-serif; background: #181b1f; color: #d8d9da; font-size: 13px; padding: 16px; }
  h2 { font-size: 16px; font-weight: 500; margin: 20px 0 10px 0; color: #d8d9da; }
  h2:first-of-type { margin-top: 0; }
  table { width: 100%; border-collapse: collapse; margin-bottom: 4px; }
  th, td { text-align: left; padding: 8px 12px; border-bottom: 1px solid #2c3035; }
  th { color: #8e8e8e; font-weight: 500; font-size: 12px; }
  tr:hover { background: #22252a; }
  .actions form { display: inline; }
  button { padding: 4px 12px; border: none; border-radius: 4px; cursor: pointer; font-size: 12px; color: #fff; margin-right: 4px; }
  .btn-load { background: #296935; }
  .btn-load:hover { background: #37873d; }
  .btn-unload { background: #4a3820; }
  .btn-unload:hover { background: #5a4828; }
  .btn-delete { background: #611919; }
  .btn-delete:hover { background: #802020; }
  .status { padding: 8px 10px; font-size: 12px; color: #8e8e8e; }
  .loaded-yes { color: #73bf69; }
  .loaded-no { color: #8e8e8e; }
  .size { color: #8ab8ff; }
  .msg { padding: 8px 10px; font-size: 12px; margin-bottom: 12px; border-radius: 4px; }
  .msg-ok { background: #1a3a1a; color: #73bf69; }
  .msg-err { background: #3a1a1a; color: #f2495c; }
  .run-selected { background: #1f3a5f; }
  .run-link { color: #6e9fff; text-decoration: underline; text-underline-offset: 2px; }
  .run-link:hover { color: #8ab8ff; }
  .hint { font-size: 12px; color: #5a5a5a; padding: 4px 12px; }
  a.dashboard-link { display: inline-block; padding: 6px 14px; border-radius: 4px; font-size: 13px; color: #6e9fff; text-decoration: none; background: #1a2332; margin: 0 6px 6px 0; }
  a.dashboard-link:hover { background: #1f3a5f; color: #8ab8ff; }
  .dashboard-list { display: flex; flex-wrap: wrap; padding: 4px 0; }
  .dashboard-info { font-size: 12px; color: #8e8e8e; margin-bottom: 8px; }
</style>
</head>
<body>
<h2>Runs</h2>
{{if .Message}}<div class="msg {{if .Error}}msg-err{{else}}msg-ok{{end}}">{{.Message}}</div>{{end}}
{{if .Runs}}
<table>
<thead><tr><th>Run ID</th><th>Start</th><th>End</th><th>Created</th><th>Size</th><th>Loaded</th><th>Actions</th></tr></thead>
<tbody>
{{range .Runs}}
<tr{{if eq .RunID $.SelectedRun}} class="run-selected"{{end}}>
  <td><a class="run-link" href="?run={{.RunID}}">&#9654; {{.RunID}}</a></td>
  <td>{{.Start.Format "2006-01-02 15:04:05"}}</td>
  <td>{{.End.Format "2006-01-02 15:04:05"}}</td>
  <td>{{.CreatedAt.Format "2006-01-02 15:04:05"}}</td>
  <td class="size">{{formatBytes .SizeBytes}}</td>
  <td class="{{if .Loaded}}loaded-yes{{else}}loaded-no{{end}}">{{if .Loaded}}Yes{{else}}No{{end}}</td>
  <td class="actions">
    <form method="POST" action="ui/action"><input type="hidden" name="run_id" value="{{.RunID}}"><input type="hidden" name="action" value="load"><button class="btn-load" type="submit">Load</button></form>
    <form method="POST" action="ui/action"><input type="hidden" name="run_id" value="{{.RunID}}"><input type="hidden" name="action" value="unload"><button class="btn-unload" type="submit">Unload</button></form>
    <form method="POST" action="ui/action"><input type="hidden" name="run_id" value="{{.RunID}}"><input type="hidden" name="action" value="delete"><button class="btn-delete" type="submit" onclick="return confirm('Delete {{.RunID}}?')">Delete</button></form>
  </td>
</tr>
{{end}}
</tbody>
</table>
{{if not .SelectedRun}}<div class="hint">Click a run ID to view dashboards.</div>{{end}}
{{else}}
<div class="status">No runs found.</div>
{{end}}

{{if .SelectedRun}}
<h2>Dashboards</h2>
<div class="dashboard-info">Run <strong>{{.SelectedRun}}</strong> &mdash; {{.SelectedStart}} to {{.SelectedEnd}}</div>
{{if .Dashboards}}
<div class="dashboard-list">
{{range .Dashboards}}
  <a class="dashboard-link" href="{{.URL}}?from={{$.SelectedFromMS}}&to={{$.SelectedToMS}}" target="_blank">{{.Title}}</a>
{{end}}
</div>
{{else}}
<div class="status">No dashboards found.</div>
{{end}}
{{end}}

</body>
</html>`))

type uiData struct {
	Runs           []model.RunInfo
	Dashboards     []grafanaDashboard
	Message        string
	Error          bool
	SelectedRun    string
	SelectedStart  string
	SelectedEnd    string
	SelectedFromMS int64
	SelectedToMS   int64
}

func (s *Server) fetchDashboards() []grafanaDashboard {
	resp, err := http.Get(s.grafanaURL + "/api/search?tag=prom-replay")
	if err != nil {
		return nil
	}
	defer resp.Body.Close()
	var dashboards []grafanaDashboard
	json.NewDecoder(resp.Body).Decode(&dashboards)
	return dashboards
}

func (s *Server) handleUI(w http.ResponseWriter, r *http.Request) {
	runs, err := s.minio.ListRuns(r.Context())
	if err != nil {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		uiTmpl.Execute(w, uiData{Message: "Error listing runs: " + err.Error(), Error: true})
		return
	}

	loadedIDs, err := s.vm.LoadedRunIDs(r.Context())
	if err == nil {
		loaded := make(map[string]bool, len(loadedIDs))
		for _, id := range loadedIDs {
			loaded[id] = true
		}
		for i := range runs {
			runs[i].Loaded = loaded[runs[i].RunID]
		}
	}

	sort.Slice(runs, func(i, j int) bool {
		return runs[i].CreatedAt.After(runs[j].CreatedAt)
	})

	data := uiData{Runs: runs}

	selectedRun := r.URL.Query().Get("run")
	if selectedRun != "" {
		for _, run := range runs {
			if run.RunID == selectedRun {
				data.SelectedRun = selectedRun
				data.SelectedStart = run.Start.Format("2006-01-02 15:04:05")
				data.SelectedEnd = run.End.Format("2006-01-02 15:04:05")
				data.SelectedFromMS = run.Start.UnixMilli()
				data.SelectedToMS = run.End.UnixMilli()
				data.Dashboards = s.fetchDashboards()
				break
			}
		}
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	uiTmpl.Execute(w, data)
}

func (s *Server) handleUIAction(w http.ResponseWriter, r *http.Request) {
	runID := r.FormValue("run_id")
	action := r.FormValue("action")

	var msg string
	var isErr bool

	switch action {
	case "load":
		if err := s.loadRun(r, runID); err != nil {
			msg = fmt.Sprintf("Failed to load %s: %v", runID, err)
			isErr = true
		} else {
			msg = fmt.Sprintf("Loaded %s", runID)
		}
	case "unload":
		if err := s.vm.DeleteSeries(r.Context(), runID); err != nil {
			msg = fmt.Sprintf("Failed to unload %s: %v", runID, err)
			isErr = true
		} else {
			msg = fmt.Sprintf("Unloaded %s", runID)
		}
	case "delete":
		if err := s.deleteRun(r, runID); err != nil {
			msg = fmt.Sprintf("Failed to delete %s: %v", runID, err)
			isErr = true
		} else {
			msg = fmt.Sprintf("Deleted %s", runID)
		}
	}

	runs, _ := s.minio.ListRuns(r.Context())
	loadedIDs, err := s.vm.LoadedRunIDs(r.Context())
	if err == nil {
		loaded := make(map[string]bool, len(loadedIDs))
		for _, id := range loadedIDs {
			loaded[id] = true
		}
		for i := range runs {
			runs[i].Loaded = loaded[runs[i].RunID]
		}
	}
	sort.Slice(runs, func(i, j int) bool {
		return runs[i].CreatedAt.After(runs[j].CreatedAt)
	})

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	uiTmpl.Execute(w, uiData{Runs: runs, Message: msg, Error: isErr})
}

func (s *Server) loadRun(r *http.Request, runID string) error {
	loadedIDs, err := s.vm.LoadedRunIDs(r.Context())
	if err != nil {
		return err
	}
	for _, id := range loadedIDs {
		if id == runID {
			return nil
		}
	}
	data, err := s.minio.GetData(r.Context(), runID)
	if err != nil {
		return err
	}
	defer data.Close()
	return s.vm.Import(r.Context(), runID, data)
}

func (s *Server) deleteRun(r *http.Request, runID string) error {
	loadedIDs, _ := s.vm.LoadedRunIDs(r.Context())
	for _, id := range loadedIDs {
		if id == runID {
			s.vm.DeleteSeries(r.Context(), runID)
			break
		}
	}
	return s.minio.DeleteRun(r.Context(), runID)
}
