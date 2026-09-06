package dashboard

import (
	"html/template"
	"net/http"
	"time"

	"github.com/tempest-io/tempest/internal/persistence"
	ttypes "github.com/tempest-io/tempest/pkg/types"
)

type Server struct {
	store    persistence.Store
	template *template.Template
}

func NewServer(store persistence.Store) *Server {
	tmpl := template.Must(template.New("dashboard").Parse(dashboardHTML))
	return &Server{
		store:    store,
		template: tmpl,
	}
}

func (s *Server) Handler() http.Handler {
	return http.HandlerFunc(s.serveDashboard)
}

func (s *Server) serveDashboard(w http.ResponseWriter, r *http.Request) {
	ns := r.URL.Query().Get("namespace")
	if ns == "" {
		ns = "default"
	}
	runs, err := s.store.ListRuns(r.Context(), persistence.RunFilter{
		Namespace: ttypes.Namespace(ns),
		Limit:     100,
	})
	if err != nil {
		http.Error(w, "failed to list runs", http.StatusInternalServerError)
		return
	}
	defs, err := s.store.ListDefinitions(r.Context(), persistence.DefinitionFilter{
		Namespace: ttypes.Namespace(ns),
		Limit:     50,
	})
	if err != nil {
		http.Error(w, "failed to list definitions", http.StatusInternalServerError)
		return
	}
	data := struct {
		Namespace   string
		Runs        []ttypes.Run
		Definitions []ttypes.WorkflowDefinition
		Timestamp   string
		QueueLen    int
		Running     int
		Succeeded   int
		Failed      int
	}{
		Namespace:   ns,
		Runs:        runs,
		Definitions: defs,
		Timestamp:   time.Now().Format(time.RFC3339),
	}
	for _, run := range runs {
		switch run.State {
		case ttypes.StateRunning, ttypes.StateQueued:
			data.Running++
		case ttypes.StateSucceeded:
			data.Succeeded++
		case ttypes.StateFailed:
			data.Failed++
		}
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := s.template.Execute(w, data); err != nil {
		http.Error(w, "template error", http.StatusInternalServerError)
	}
}

var dashboardHTML = `<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="UTF-8">
  <meta name="viewport" content="width=device-width, initial-scale=1.0">
  <title>Tempest Dashboard</title>
  <style>
    * { margin: 0; padding: 0; box-sizing: border-box; }
    body { font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', sans-serif; background: #f5f5f5; color: #333; padding: 20px; }
    .container { max-width: 1200px; margin: 0 auto; }
    header { background: #2c3e50; color: white; padding: 20px; border-radius: 8px; margin-bottom: 20px; }
    header h1 { font-size: 24px; }
    header p { opacity: 0.8; margin-top: 4px; }
    .stats { display: grid; grid-template-columns: repeat(4, 1fr); gap: 16px; margin-bottom: 20px; }
    .stat-card { background: white; padding: 16px; border-radius: 8px; box-shadow: 0 1px 3px rgba(0,0,0,0.1); }
    .stat-card h3 { font-size: 14px; color: #666; text-transform: uppercase; }
    .stat-card .value { font-size: 32px; font-weight: bold; margin-top: 8px; }
    .stat-card .value.running { color: #e67e22; }
    .stat-card .value.succeeded { color: #27ae60; }
    .stat-card .value.failed { color: #e74c3c; }
    .stat-card .value.total { color: #3498db; }
    section { background: white; padding: 20px; border-radius: 8px; box-shadow: 0 1px 3px rgba(0,0,0,0.1); margin-bottom: 20px; }
    section h2 { font-size: 18px; margin-bottom: 12px; }
    table { width: 100%; border-collapse: collapse; }
    th, td { padding: 8px 12px; text-align: left; border-bottom: 1px solid #eee; }
    th { background: #f8f9fa; font-weight: 600; font-size: 12px; text-transform: uppercase; color: #666; }
    tr:hover { background: #f8f9fa; }
    .state { padding: 2px 8px; border-radius: 4px; font-size: 12px; font-weight: 600; }
    .state-PENDING { background: #ecf0f1; color: #7f8c8d; }
    .state-QUEUED { background: #ebf5fb; color: #2980b9; }
    .state-RUNNING { background: #fef9e7; color: #e67e22; }
    .state-SUCCEEDED { background: #eafaf1; color: #27ae60; }
    .state-FAILED { background: #fdedec; color: #e74c3c; }
    .state-CANCELLED { background: #f4ecf7; color: #8e44ad; }
    .empty { text-align: center; padding: 40px; color: #999; }
    footer { text-align: center; padding: 20px; color: #999; font-size: 12px; }
  </style>
</head>
<body>
  <div class="container">
    <header>
      <h1>Tempest Dashboard</h1>
      <p>Namespace: {{.Namespace}} | Last updated: {{.Timestamp}}</p>
    </header>
    <div class="stats">
      <div class="stat-card"><h3>Total Runs</h3><div class="value total">{{len .Runs}}</div></div>
      <div class="stat-card"><h3>Running</h3><div class="value running">{{.Running}}</div></div>
      <div class="stat-card"><h3>Succeeded</h3><div class="value succeeded">{{.Succeeded}}</div></div>
      <div class="stat-card"><h3>Failed</h3><div class="value failed">{{.Failed}}</div></div>
    </div>
    <section>
      <h2>Workflow Definitions</h2>
      {{if .Definitions}}
      <table>
        <thead><tr><th>Name</th><th>Version</th><th>Steps</th></tr></thead>
        <tbody>
        {{range .Definitions}}
          <tr><td>{{.ID.Name}}</td><td>{{.ID.Version}}</td><td>{{len .Steps}}</td></tr>
        {{end}}
        </tbody>
      </table>
      {{else}}<div class="empty">No workflow definitions found</div>{{end}}
    </section>
    <section>
      <h2>Recent Runs</h2>
      {{if .Runs}}
      <table>
        <thead><tr><th>ID</th><th>Workflow</th><th>State</th><th>Created</th></tr></thead>
        <tbody>
        {{range .Runs}}
          <tr>
            <td>{{.ID}}</td>
            <td>{{.Workflow.Name}}@{{.Workflow.Version}}</td>
            <td><span class="state state-{{.State}}">{{.State}}</span></td>
            <td>{{.CreatedAt.Format "2006-01-02 15:04:05"}}</td>
          </tr>
        {{end}}
        </tbody>
      </table>
      {{else}}<div class="empty">No runs found</div>{{end}}
    </section>
    <footer>Tempest Workflow Engine</footer>
  </div>
</body>
</html>`
