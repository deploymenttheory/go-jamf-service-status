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

// Alert defines the interface for all alert implementations
type Alert interface {
	// Send sends an alert about the given incidents
	Send(incidents []core.StatusIncident, config map[string]string) error

	// SendReport sends a full status report
	SendReport(report *core.StatusReport, config map[string]string) error

	// GetTemplates returns the templates used by this alert type
	GetTemplates() map[string]*template.Template
}

// BaseAlert contains common functionality for all alert types
type BaseAlert struct {
	Templates map[string]*template.Template
	LastSent  time.Time
}

// OutageAlert represents an alert about an active outage
type OutageAlert struct {
	BaseAlert
	Client         *http.Client
	ThrottleWindow time.Duration
}

// NewOutageAlert creates a new outage alert with default templates
func NewOutageAlert() *OutageAlert {
	// Initialize with default templates
	templates := make(map[string]*template.Template)

	// Simple template for outage alerts
	outageTemplate := `
Jamf Status Alert: {{len .}} Active Outage(s)
{{range .}}
* {{.Title}} - {{.Status}}
  Affected: {{join .AffectedRegions ", "}}
  {{if .Description}}{{.Description}}{{end}}
{{end}}
Last updated: {{now}}
`

	// Create template with custom functions
	templates["default"] = template.Must(template.New("outage").Funcs(template.FuncMap{
		"join": strings.Join,
		"now": func() string {
			return time.Now().Format(time.RFC1123)
		},
	}).Parse(outageTemplate))

	return &OutageAlert{
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
func (a *OutageAlert) GetTemplates() map[string]*template.Template {
	return a.Templates
}

// Send sends an alert about the given incidents
func (a *OutageAlert) Send(incidents []core.StatusIncident, config map[string]string) error {
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

	// Send to appropriate channels
	a.LastSent = time.Now()

	// Check for Slack integration
	if webhook, ok := config["slack_webhook"]; ok {
		if err := a.sendToSlack(webhook, buf.String()); err != nil {
			return err
		}
	}

	// Check for Teams integration
	if webhook, ok := config["teams_webhook"]; ok {
		if err := a.sendToTeams(webhook, buf.String()); err != nil {
			return err
		}
	}

	// Check for email
	if email, ok := config["email"]; ok {
		if err := a.sendEmail(email, "Jamf Status Alert", buf.String(), config); err != nil {
			return err
		}
	}

	return nil
}

// SendReport sends a full status report
func (a *OutageAlert) SendReport(report *core.StatusReport, config map[string]string) error {
	// Implementation would be similar to Send but with different templates
	// and potentially more details from the report
	return nil
}

// sendToSlack sends an alert to a Slack webhook
func (a *OutageAlert) sendToSlack(webhookURL string, message string) error {
	// Prepare Slack message payload
	payload := map[string]interface{}{
		"text": message,
	}

	// Add blocks for better formatting if needed
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

// sendToTeams sends an alert to a Microsoft Teams webhook
func (a *OutageAlert) sendToTeams(webhookURL string, message string) error {
	// Prepare Teams message payload
	payload := map[string]interface{}{
		"@type":      "MessageCard",
		"@context":   "https://schema.org/extensions",
		"summary":    "Jamf Status Alert",
		"themeColor": "0078D7",
		"title":      "Jamf Status Alert",
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

// sendEmail sends an email alert
func (a *OutageAlert) sendEmail(recipient, subject, body string, config map[string]string) error {
	// This would implement SMTP email sending or use a service like SendGrid
	// For simplicity, we'll just log it for now
	fmt.Printf("Would send email to %s with subject '%s'\n", recipient, subject)
	return nil
}

// AlertManager manages multiple alert types and configurations
type AlertManager struct {
	Alerts       map[string]Alert
	Configs      map[string]map[string]string
	CheckHistory map[string]time.Time
}

// NewAlertManager creates a new alert manager
func NewAlertManager() *AlertManager {
	return &AlertManager{
		Alerts:       make(map[string]Alert),
		Configs:      make(map[string]map[string]string),
		CheckHistory: make(map[string]time.Time),
	}
}

// RegisterAlert registers an alert implementation with the manager
func (m *AlertManager) RegisterAlert(name string, alert Alert) {
	m.Alerts[name] = alert
}

// SetConfig sets the configuration for an alert
func (m *AlertManager) SetConfig(alertName, key, value string) {
	if _, ok := m.Configs[alertName]; !ok {
		m.Configs[alertName] = make(map[string]string)
	}
	m.Configs[alertName][key] = value
}

// ProcessReport processes a status report and sends appropriate alerts
func (m *AlertManager) ProcessReport(report *core.StatusReport) error {
	// Track which alerts were sent
	sentAlerts := make(map[string]bool)

	// Process active outages first
	if report.HasActiveOutages {
		for name, alert := range m.Alerts {
			if config, ok := m.Configs[name]; ok {
				if err := alert.Send(report.ActiveOutages, config); err != nil {
					return fmt.Errorf("error sending alert %s: %w", name, err)
				}
				sentAlerts[name] = true
			}
		}
	}

	// Process full reports for any alerts that weren't sent outage notifications
	for name, alert := range m.Alerts {
		if !sentAlerts[name] {
			if config, ok := m.Configs[name]; ok {
				if shouldSendReport(name, m.CheckHistory) {
					if err := alert.SendReport(report, config); err != nil {
						return fmt.Errorf("error sending report %s: %w", name, err)
					}
					m.CheckHistory[name] = time.Now()
				}
			}
		}
	}

	return nil
}

// shouldSendReport determines if a report should be sent based on history
func shouldSendReport(alertName string, history map[string]time.Time) bool {
	// Default interval of 24 hours for reports
	interval := 24 * time.Hour

	lastSent, ok := history[alertName]
	if !ok {
		return true // First time, should send
	}

	return time.Since(lastSent) >= interval
}
