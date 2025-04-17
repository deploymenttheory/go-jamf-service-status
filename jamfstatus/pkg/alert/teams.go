package alert

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"text/template"
	"time"

	"github.com/deploymenttheory/go-jamf-service-status/jamfstatus/pkg/core"
)

// TeamsAlert represents a Microsoft Teams alert implementation
type TeamsAlert struct {
	BaseAlert
	Client         *http.Client
	ThrottleWindow time.Duration
}

// NewTeamsAlert creates a new Teams alert with default templates
func NewTeamsAlert() *TeamsAlert {
	// Initialize with default templates
	templates := make(map[string]*template.Template)

	// Template for outage alerts
	outageTemplate := `
Jamf Status Alert: {{len .}} Active Outage(s)
{{range .}}
* {{.Title}} - {{.Status}}
  Affected: {{join .AffectedRegions ", "}}
  Impact: {{.Impact}}
  {{if .Description}}{{.Description}}{{end}}
{{end}}
Last updated: {{now}}
`

	// Template for status reports
	reportTemplate := `
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
{{range .Services}}* {{.Name}}: {{.Status}}
{{end}}

Last updated: {{now}}
`

	// Create templates with custom functions
	templates["default"] = template.Must(template.New("outage").Funcs(template.FuncMap{
		"join": strings.Join,
		"now": func() string {
			return time.Now().Format(time.RFC1123)
		},
	}).Parse(outageTemplate))

	templates["report"] = template.Must(template.New("report").Funcs(template.FuncMap{
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
	}).Parse(reportTemplate))

	return &TeamsAlert{
		BaseAlert: BaseAlert{
			Templates: templates,
		},
		Client: &http.Client{
			Timeout: 10 * time.Second,
		},
		ThrottleWindow: 30 * time.Minute,
	}
}

// GetTemplates returns the templates used by this alert
func (a *TeamsAlert) GetTemplates() map[string]*template.Template {
	return a.Templates
}

// Send sends an alert about the given incidents
func (a *TeamsAlert) Send(incidents []core.StatusIncident, config map[string]string) error {
	// Skip if throttling is active
	if time.Since(a.LastSent) < a.ThrottleWindow {
		return nil
	}

	// Skip if no incidents
	if len(incidents) == 0 {
		return nil
	}

	// Render template
	templateName := "default"
	if tn, ok := config["template"]; ok && a.Templates[tn] != nil {
		templateName = tn
	}

	var buf bytes.Buffer
	if err := a.Templates[templateName].Execute(&buf, incidents); err != nil {
		return fmt.Errorf("error executing template: %w", err)
	}

	// Check for Teams webhook
	if webhook, ok := config["webhook_url"]; ok {
		if err := a.sendToTeams(webhook, buf.String(), "Jamf Status Alert"); err != nil {
			return err
		}
	} else {
		return fmt.Errorf("no webhook_url found in configuration")
	}

	a.LastSent = time.Now()
	return nil
}

// SendReport sends a full status report
func (a *TeamsAlert) SendReport(report *core.StatusReport, config map[string]string) error {
	// Skip if throttling is active
	if time.Since(a.LastSent) < a.ThrottleWindow {
		return nil
	}

	// Render template
	templateName := "report"
	if tn, ok := config["report_template"]; ok && a.Templates[tn] != nil {
		templateName = tn
	}

	var buf bytes.Buffer
	if err := a.Templates[templateName].Execute(&buf, report); err != nil {
		return fmt.Errorf("error executing template: %w", err)
	}

	// Check for Teams webhook
	if webhook, ok := config["webhook_url"]; ok {
		if err := a.sendToTeams(webhook, buf.String(), "Jamf Status Report"); err != nil {
			return err
		}
	} else {
		return fmt.Errorf("no webhook_url found in configuration")
	}

	a.LastSent = time.Now()
	return nil
}

// sendToTeams sends an alert to a Microsoft Teams webhook
func (a *TeamsAlert) sendToTeams(webhookURL, message, title string) error {
	// Determine card color based on message content
	themeColor := "0078D7" // Default blue
	if strings.Contains(message, "Outage") {
		themeColor = "FF0000" // Red for outages
	} else if strings.Contains(message, "All systems operational") {
		themeColor = "00FF00" // Green for all-clear
	}

	// Prepare Teams message payload
	payload := map[string]interface{}{
		"@type":      "MessageCard",
		"@context":   "https://schema.org/extensions",
		"summary":    title,
		"themeColor": themeColor,
		"title":      title,
		"sections": []map[string]interface{}{
			{
				"text": message,
			},
		},
	}

	// Convert to JSON
	jsonPayload, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("error marshaling Teams payload: %w", err)
	}

	// Send to webhook
	req, err := http.NewRequest("POST", webhookURL, bytes.NewBuffer(jsonPayload))
	if err != nil {
		return fmt.Errorf("error creating Teams request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := a.Client.Do(req)
	if err != nil {
		return fmt.Errorf("error sending Teams alert: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("teams returned non-OK response: %s", resp.Status)
	}

	return nil
}

// SetTemplate allows setting a custom template
func (a *TeamsAlert) SetTemplate(name, templateContent string) error {
	tmpl, err := template.New(name).Funcs(template.FuncMap{
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
	}).Parse(templateContent)

	if err != nil {
		return fmt.Errorf("error parsing template: %w", err)
	}

	a.Templates[name] = tmpl
	return nil
}
