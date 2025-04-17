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

// SlackAlert represents a Slack alert implementation
type SlackAlert struct {
	BaseAlert
	Client         *http.Client
	ThrottleWindow time.Duration
}

// NewSlackAlert creates a new Slack alert with default templates
func NewSlackAlert() *SlackAlert {
	// Initialize with default templates
	templates := make(map[string]*template.Template)

	// Template for outage alerts
	outageTemplate := `
:warning: *Jamf Status Alert: {{len .}} Active Outage(s)* :warning:
{{range .}}
• *{{.Title}}* - {{.Status}}
  *Affected:* {{join .AffectedRegions ", "}}
  *Impact:* {{.Impact}}
  {{if .Description}}{{.Description}}{{end}}
{{end}}
Last updated: {{now}}
`

	// Template for status reports
	reportTemplate := `
:information_source: *Jamf Status Report* :information_source:

*Current Status:*
{{if .HasActiveOutages}}:warning: There are {{len .ActiveOutages}} active outage(s){{else}}:white_check_mark: All systems operational{{end}}

{{if .HasActiveOutages}}*Active Outages:*
{{range .ActiveOutages}}• *{{.Title}}* - {{.Status}}
  *Affected:* {{join .AffectedRegions ", "}}
  *Impact:* {{.Impact}}
{{end}}
{{end}}

{{if .PlannedMaintenance}}*Planned Maintenance:*
{{range .PlannedMaintenance}}• *{{.Title}}* - {{.Status}}
  *Scheduled:* {{formatTime .PublishedAt}}
  *Affected:* {{join .AffectedRegions ", "}}
{{end}}
{{end}}

*Service Status:*
{{range .Services}}• *{{.Name}}*: {{serviceStatus .Status}}
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
		"serviceStatus": func(status string) string {
			switch strings.ToLower(status) {
			case "operational":
				return ":green_circle: Operational"
			case "degraded":
				return ":large_orange_circle: Degraded"
			case "outage":
				return ":red_circle: Outage"
			case "maintenance":
				return ":wrench: Maintenance"
			default:
				return status
			}
		},
	}).Parse(reportTemplate))

	return &SlackAlert{
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
func (a *SlackAlert) GetTemplates() map[string]*template.Template {
	return a.Templates
}

// Send sends an alert about the given incidents
func (a *SlackAlert) Send(incidents []core.StatusIncident, config map[string]string) error {
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

	// Check for Slack webhook
	if webhook, ok := config["webhook_url"]; ok {
		if err := a.sendToSlack(webhook, buf.String()); err != nil {
			return err
		}
	} else {
		return fmt.Errorf("no webhook_url found in configuration")
	}

	a.LastSent = time.Now()
	return nil
}

// SendReport sends a full status report
func (a *SlackAlert) SendReport(report *core.StatusReport, config map[string]string) error {
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

	// Check for Slack webhook
	if webhook, ok := config["webhook_url"]; ok {
		if err := a.sendToSlack(webhook, buf.String()); err != nil {
			return err
		}
	} else {
		return fmt.Errorf("no webhook_url found in configuration")
	}

	a.LastSent = time.Now()
	return nil
}

// sendToSlack sends an alert to a Slack webhook
func (a *SlackAlert) sendToSlack(webhookURL string, message string) error {
	// Prepare Slack message payload
	payload := map[string]interface{}{
		"text": message,
	}

	// Add blocks for better formatting
	blocks := []map[string]interface{}{
		{
			"type": "section",
			"text": map[string]string{
				"type": "mrkdwn",
				"text": message,
			},
		},
	}
	payload["blocks"] = blocks

	// Convert to JSON
	jsonPayload, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("error marshaling Slack payload: %w", err)
	}

	// Send to webhook
	req, err := http.NewRequest("POST", webhookURL, bytes.NewBuffer(jsonPayload))
	if err != nil {
		return fmt.Errorf("error creating Slack request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := a.Client.Do(req)
	if err != nil {
		return fmt.Errorf("error sending Slack alert: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("slack returned non-OK response: %s", resp.Status)
	}

	return nil
}

// SetTemplate allows setting a custom template
func (a *SlackAlert) SetTemplate(name, templateContent string) error {
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
		"serviceStatus": func(status string) string {
			switch strings.ToLower(status) {
			case "operational":
				return ":green_circle: Operational"
			case "degraded":
				return ":large_orange_circle: Degraded"
			case "outage":
				return ":red_circle: Outage"
			case "maintenance":
				return ":wrench: Maintenance"
			default:
				return status
			}
		},
	}).Parse(templateContent)

	if err != nil {
		return fmt.Errorf("error parsing template: %w", err)
	}

	a.Templates[name] = tmpl
	return nil
}
