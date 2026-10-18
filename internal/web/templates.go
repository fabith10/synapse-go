package web

import (
	"embed"
	"html/template"
	"io"
	"os"
)

//go:embed templates/*.html
var templatesFS embed.FS

// DashboardPage is the HTML template for the main status page in ultra-premium dark glassmorphic design.
var DashboardPage = template.Must(template.New("dashboard.html").ParseFS(templatesFS, "templates/*.html"))

// RenderDashboard renders dashboard, reloading from disk if available in development.
func RenderDashboard(w io.Writer, data any) error {
	if _, err := os.Stat("internal/web/templates/dashboard.html"); err == nil {
		tmpl, err := template.New("dashboard.html").ParseGlob("internal/web/templates/*.html")
		if err == nil {
			return tmpl.Execute(w, data)
		}
	}
	return DashboardPage.Execute(w, data)
}

// PendingApprovalTemplate renders a pending approval card with glowing amber left border.
var PendingApprovalTemplate = template.Must(template.New("pending_approval.html").ParseFS(templatesFS, "templates/pending_approval.html"))

// LogSnippetTemplate renders a log entry with marked.js markdown rendering and role-colored badges.
var LogSnippetTemplate = template.Must(template.New("log_snippet.html").ParseFS(templatesFS, "templates/log_snippet.html"))

// ActionTakenTemplate renders the state of the card after decision is submitted.
var ActionTakenTemplate = template.Must(template.New("action_taken.html").ParseFS(templatesFS, "templates/action_taken.html"))

// ArtifactCardTemplate renders a downloadable artifact card pushed via SSE.
var ArtifactCardTemplate = template.Must(template.New("artifact_card.html").ParseFS(templatesFS, "templates/artifact_card.html"))

// RenderArtifactCard renders a single artifact card, reloading from disk if available in development.
func RenderArtifactCard(w io.Writer, data any) error {
	if _, err := os.Stat("internal/web/templates/artifact_card.html"); err == nil {
		tmpl, err := template.ParseFiles("internal/web/templates/artifact_card.html")
		if err == nil {
			return tmpl.Execute(w, data)
		}
	}
	return ArtifactCardTemplate.Execute(w, data)
}

