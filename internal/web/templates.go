package web

import (
	"embed"
	"html/template"
)

//go:embed templates/*.html
var templatesFS embed.FS

// DashboardPage is the HTML template for the main status page in ultra-premium dark glassmorphic design.
var DashboardPage = template.Must(template.Must(template.New("dashboard.html").ParseFS(templatesFS, "templates/dashboard.html")).ParseFS(templatesFS, "templates/schedules_list.html"))

// PendingApprovalTemplate renders a pending approval card with glowing amber left border.
var PendingApprovalTemplate = template.Must(template.New("pending_approval.html").ParseFS(templatesFS, "templates/pending_approval.html"))

// LogSnippetTemplate renders a log entry with marked.js markdown rendering and role-colored badges.
var LogSnippetTemplate = template.Must(template.New("log_snippet.html").ParseFS(templatesFS, "templates/log_snippet.html"))

// ActionTakenTemplate renders the state of the card after decision is submitted.
var ActionTakenTemplate = template.Must(template.New("action_taken.html").ParseFS(templatesFS, "templates/action_taken.html"))

// ArtifactCardTemplate renders a downloadable artifact card pushed via SSE.
var ArtifactCardTemplate = template.Must(template.New("artifact_card.html").ParseFS(templatesFS, "templates/artifact_card.html"))
