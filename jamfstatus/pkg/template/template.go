package template

import (
	"fmt"
	"strings"
	"text/template"
	"time"

	"github.com/deploymenttheory/go-jamf-service-status/jamfstatus/pkg/core"
)

// TemplateEngine manages templates for notifications
type TemplateEngine struct {
	Templates map[string]*template.Template
}

// NewTemplateEngine creates a new template engine with default templates
func NewTemplateEngine() *TemplateEngine {
	templates := make(map[string]*template.Template)

	// Add default templates
	templates["outage_text"] = defaultTextOutageTemplate()
	templates["outage_html"] = defaultHTMLOutageTemplate()
	templates["report_text"] = defaultTextReportTemplate()
	templates["report_html"] = defaultHTMLReportTemplate()
	templates["summary_text"] = defaultTextSummaryTemplate()

	return &TemplateEngine{
		Templates: templates,
	}
}

// GetTemplate returns a template by name
func (e *TemplateEngine) GetTemplate(name string) (*template.Template, error) {
	if tmpl, ok := e.Templates[name]; ok {
		return tmpl, nil
	}
	return nil, fmt.Errorf("template not found: %s", name)
}

// AddTemplate adds a new template to the engine
func (e *TemplateEngine) AddTemplate(name, content string) error {
	tmpl, err := template.New(name).Funcs(standardFuncMap()).Parse(content)
	if err != nil {
		return fmt.Errorf("error parsing template: %w", err)
	}

	e.Templates[name] = tmpl
	return nil
}

// RenderOutage renders an outage template with incidents
func (e *TemplateEngine) RenderOutage(templateName string, incidents []core.StatusIncident) (string, error) {
	tmpl, err := e.GetTemplate(templateName)
	if err != nil {
		return "", err
	}

	var sb strings.Builder
	if err := tmpl.Execute(&sb, incidents); err != nil {
		return "", fmt.Errorf("error executing template: %w", err)
	}

	return sb.String(), nil
}

// RenderReport renders a report template with report data
func (e *TemplateEngine) RenderReport(templateName string, report *core.StatusReport) (string, error) {
	tmpl, err := e.GetTemplate(templateName)
	if err != nil {
		return "", err
	}

	var sb strings.Builder
	if err := tmpl.Execute(&sb, report); err != nil {
		return "", fmt.Errorf("error executing template: %w", err)
	}

	return sb.String(), nil
}

// standardFuncMap returns a standard function map for templates
func standardFuncMap() template.FuncMap {
	return template.FuncMap{
		"join": strings.Join,
		"now": func() string {
			return time.Now().Format(time.RFC1123)
		},
		"formatTime": func(t *time.Time) string {
			if t == nil {
				return "N/A"
			}
			return t.Format(time.RFC1123)
		},
		"lower": strings.ToLower,
		"upper": strings.ToUpper,
		"title": strings.Title,
		"formatDuration": func(d time.Duration) string {
			d = d.Round(time.Minute)
			h := d / time.Hour
			d -= h * time.Hour
			m := d / time.Minute
			if h > 0 {
				return fmt.Sprintf("%dh %dm", h, m)
			}
			return fmt.Sprintf("%dm", m)
		},
		"serviceStatus": func(status string) string {
			switch strings.ToLower(status) {
			case "operational":
				return "✅ Operational"
			case "degraded":
				return "⚠️ Degraded"
			case "outage":
				return "❌ Outage"
			case "maintenance":
				return "🔧 Maintenance"
			default:
				return status
			}
		},
	}
}

// Default templates
func defaultTextOutageTemplate() *template.Template {
	content := `
Jamf Status Alert: {{len .}} Active Outage(s)
{{range .}}
* {{.Title}} - {{.Status}}
  Affected: {{join .AffectedRegions ", "}}
  Impact: {{.Impact}}
  {{if .Description}}{{.Description}}{{end}}
{{end}}
Last updated: {{now}}
`
	return template.Must(template.New("outage_text").Funcs(standardFuncMap()).Parse(content))
}

func defaultHTMLOutageTemplate() *template.Template {
	content := `
<!DOCTYPE html>
<html>
<head>
    <meta charset="UTF-8">
    <title>Jamf Status Alert</title>
    <style>
        body { font-family: Arial, sans-serif; margin: 0; padding: 20px; color: #333; }
        .container { max-width: 600px; margin: 0 auto; }
        .header { background-color: #e74c3c; color: white; padding: 10px 20px; border-radius: 5px 5px 0 0; }
        .content { padding: 20px; border: 1px solid #ddd; border-top: none; border-radius: 0 0 5px 5px; }
        .incident { margin-bottom: 15px; padding-bottom: 15px; border-bottom: 1px solid #eee; }
        .incident:last-child { border-bottom: none; margin-bottom: 0; padding-bottom: 0; }
        .title { font-weight: bold; }
        .status { font-style: italic; }
        .affected { color: #666; }
        .impact-high { color: #e74c3c; font-weight: bold; }
        .impact-medium { color: #f39c12; font-weight: bold; }
        .impact-low { color: #3498db; font-weight: bold; }
        .footer { margin-top: 20px; font-size: 12px; color: #777; }
    </style>
</head>
<body>
    <div class="container">
        <div class="header">
            <h2>Jamf Status Alert: {{len .}} Active Outage(s)</h2>
        </div>
        <div class="content">
            {{range .}}
            <div class="incident">
                <div class="title">{{.Title}}</div>
                <div class="status">Status: {{.Status}}</div>
                <div class="affected">Affected: {{join .AffectedRegions ", "}}</div>
                <div class="impact impact-{{lower .Impact}}">Impact: {{.Impact}}</div>
                {{if .Description}}<div class="description">{{.Description}}</div>{{end}}
            </div>
            {{end}}
            <div class="footer">
                Last updated: {{now}}<br>
                This is an automated message from the Jamf Status Monitor.
            </div>
        </div>
    </div>
</body>
</html>
`
	return template.Must(template.New("outage_html").Funcs(standardFuncMap()).Parse(content))
}

func defaultTextReportTemplate() *template.Template {
	content := `
Jamf Status Report

Current Status:
{{if .HasActiveOutages}}There are {{len .ActiveOutages}} active outage(s){{else}}All systems operational{{end}}

{{if .HasActiveOutages}}Active Outages:
{{range .ActiveOutages}}* {{.Title}} - {{.Status}}
  Affected: {{join .AffectedRegions ", "}}
  Impact: {{.Impact}}
{{end}}
{{end}}

{{if .PlannedMaintenance}}Planned Maintenance:
{{range .PlannedMaintenance}}* {{.Title}} - {{.Status}}
  Scheduled: {{formatTime .PublishedAt}}
  Affected: {{join .AffectedRegions ", "}}
{{end}}
{{end}}

Service Status:
{{range .Services}}* {{.Name}}: {{serviceStatus .Status}}
{{end}}

Last updated: {{now}}
`
	return template.Must(template.New("report_text").Funcs(standardFuncMap()).Parse(content))
}

func defaultHTMLReportTemplate() *template.Template {
	content := `
<!DOCTYPE html>
<html>
<head>
    <meta charset="UTF-8">
    <title>Jamf Status Report</title>
    <style>
        body { font-family: Arial, sans-serif; margin: 0; padding: 20px; color: #333; }
        .container { max-width: 600px; margin: 0 auto; }
        .header { background-color: #2980b9; color: white; padding: 10px 20px; border-radius: 5px 5px 0 0; }
        .content { padding: 20px; border: 1px solid #ddd; border-top: none; border-radius: 0 0 5px 5px; }
        .section { margin-bottom: 20px; }
        .section-title { font-weight: bold; margin-bottom: 10px; border-bottom: 1px solid #eee; padding-bottom: 5px; }
        .incident { margin-bottom: 15px; padding-left: 15px; }
        .service { margin-bottom: 5px; }
        .status-operational { color: #27ae60; }
        .status-degraded { color: #f39c12; }
        .status-outage { color: #e74c3c; }
        .status-maintenance { color: #3498db; }
        .footer { margin-top: 20px; font-size: 12px; color: #777; }
    </style>
</head>
<body>
    <div class="container">
        <div class="header">
            <h2>Jamf Status Report</h2>
        </div>
        <div class="content">
            <div class="section">
                <div class="section-title">Current Status</div>
                {{if .HasActiveOutages}}
                <div class="status-outage">There are {{len .ActiveOutages}} active outage(s)</div>
                {{else}}
                <div class="status-operational">All systems operational</div>
                {{end}}
            </div>

            {{if .HasActiveOutages}}
            <div class="section">
                <div class="section-title">Active Outages</div>
                {{range .ActiveOutages}}
                <div class="incident">
                    <div><strong>{{.Title}}</strong> - {{.Status}}</div>
                    <div>Affected: {{join .AffectedRegions ", "}}</div>
                    <div>Impact: {{.Impact}}</div>
                </div>
                {{end}}
            </div>
            {{end}}

            {{if .PlannedMaintenance}}
            <div class="section">
                <div class="section-title">Planned Maintenance</div>
                {{range .PlannedMaintenance}}
                <div class="incident">
                    <div><strong>{{.Title}}</strong> - {{.Status}}</div>
                    <div>Scheduled: {{formatTime .PublishedAt}}</div>
                    <div>Affected: {{join .AffectedRegions ", "}}</div>
                </div>
                {{end}}
            </div>
            {{end}}

            <div class="section">
                <div class="section-title">Service Status</div>
                {{range .Services}}
                <div class="service">
                    <strong>{{.Name}}:</strong> <span class="status-{{lower .Status}}">{{.Status}}</span>
                </div>
                {{end}}
            </div>

            <div class="footer">
                Last updated: {{now}}<br>
                This is an automated message from the Jamf Status Monitor.
            </div>
        </div>
    </div>
</body>
</html>
`
	return template.Must(template.New("report_html").Funcs(standardFuncMap()).Parse(content))
}

func defaultTextSummaryTemplate() *template.Template {
	content := `
Jamf Status Summary ({{now}})
{{if .HasActiveOutages}}
⚠️ ACTIVE OUTAGES: {{len .ActiveOutages}}
{{else}}
✅ ALL SYSTEMS OPERATIONAL
{{end}}

{{if .PlannedMaintenance}}
🔧 UPCOMING MAINTENANCE: {{len .PlannedMaintenance}}
{{end}}
`
	return template.Must(template.New("summary_text").Funcs(standardFuncMap()).Parse(content))
}
