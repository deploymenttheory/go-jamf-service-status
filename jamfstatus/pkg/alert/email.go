package alert

import (
	"bytes"
	"fmt"
	"net/smtp"
	"strings"
	"text/template"
	"time"

	"github.com/deploymenttheory/go-jamf-service-status/jamfstatus/pkg/core"
)

// EmailAlert represents an email alert implementation
type EmailAlert struct {
	BaseAlert
	ThrottleWindow time.Duration

	// SMTP settings
	SMTPHost string
	SMTPPort int
	Username string
	Password string
	FromAddr string
}

// NewEmailAlert creates a new email alert with default templates
func NewEmailAlert() *EmailAlert {
	// Initialize with default templates
	templates := make(map[string]*template.Template)

	// HTML template for outage alerts
	outageTemplate := `
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

	// HTML template for status reports
	reportTemplate := `
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

	// Create templates with custom functions
	templates["default"] = template.Must(template.New("outage").Funcs(template.FuncMap{
		"join": strings.Join,
		"now": func() string {
			return time.Now().Format(time.RFC1123)
		},
		"lower": strings.ToLower,
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
		"lower": strings.ToLower,
	}).Parse(reportTemplate))

	return &EmailAlert{
		BaseAlert: BaseAlert{
			Templates: templates,
		},
		ThrottleWindow: 60 * time.Minute, // Less frequent than other alerts
		SMTPHost:       "smtp.example.com",
		SMTPPort:       587,
		FromAddr:       "alerts@example.com",
	}
}

// GetTemplates returns the templates used by this alert
func (a *EmailAlert) GetTemplates() map[string]*template.Template {
	return a.Templates
}

// Send sends an alert about the given incidents
func (a *EmailAlert) Send(incidents []core.StatusIncident, config map[string]string) error {
	// Skip if throttling is active
	if time.Since(a.LastSent) < a.ThrottleWindow {
		return nil
	}

	// Skip if no incidents
	if len(incidents) == 0 {
		return nil
	}

	// Check for required configuration
	recipient, ok := config["recipient"]
	if !ok {
		return fmt.Errorf("no recipient found in configuration")
	}

	// Override SMTP settings from config if provided
	smtpHost := a.SMTPHost
	if host, ok := config["smtp_host"]; ok {
		smtpHost = host
	}

	smtpPort := a.SMTPPort
	if portStr, ok := config["smtp_port"]; ok {
		fmt.Sscanf(portStr, "%d", &smtpPort)
	}

	username := a.Username
	if user, ok := config["smtp_username"]; ok {
		username = user
	}

	password := a.Password
	if pass, ok := config["smtp_password"]; ok {
		password = pass
	}

	fromAddr := a.FromAddr
	if from, ok := config["from_address"]; ok {
		fromAddr = from
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

	// Send email
	subject := fmt.Sprintf("Jamf Status Alert: %d Active Outage(s)", len(incidents))
	if err := a.sendEmail(smtpHost, smtpPort, username, password, fromAddr, recipient, subject, buf.String()); err != nil {
		return err
	}

	a.LastSent = time.Now()
	return nil
}

// SendReport sends a full status report
func (a *EmailAlert) SendReport(report *core.StatusReport, config map[string]string) error {
	// Skip if throttling is active
	if time.Since(a.LastSent) < a.ThrottleWindow {
		return nil
	}

	// Check for required configuration
	recipient, ok := config["recipient"]
	if !ok {
		return fmt.Errorf("no recipient found in configuration")
	}

	// Override SMTP settings from config if provided
	smtpHost := a.SMTPHost
	if host, ok := config["smtp_host"]; ok {
		smtpHost = host
	}

	smtpPort := a.SMTPPort
	if portStr, ok := config["smtp_port"]; ok {
		fmt.Sscanf(portStr, "%d", &smtpPort)
	}

	username := a.Username
	if user, ok := config["smtp_username"]; ok {
		username = user
	}

	password := a.Password
	if pass, ok := config["smtp_password"]; ok {
		password = pass
	}

	fromAddr := a.FromAddr
	if from, ok := config["from_address"]; ok {
		fromAddr = from
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

	// Determine subject based on status
	subject := "Jamf Status Report"
	if report.HasActiveOutages {
		subject = fmt.Sprintf("Jamf Status Alert: %d Active Outage(s)", len(report.ActiveOutages))
	}

	// Send email
	if err := a.sendEmail(smtpHost, smtpPort, username, password, fromAddr, recipient, subject, buf.String()); err != nil {
		return err
	}

	a.LastSent = time.Now()
	return nil
}

// sendEmail sends an email with the given parameters
func (a *EmailAlert) sendEmail(host string, port int, username, password, from, to, subject, body string) error {
	// Check if we have the required settings
	if host == "" || port == 0 {
		return fmt.Errorf("missing SMTP configuration (host=%s, port=%d)", host, port)
	}

	// Set up authentication if credentials are provided
	var auth smtp.Auth
	if username != "" && password != "" {
		auth = smtp.PlainAuth("", username, password, host)
	}

	// Format the email headers
	headers := make(map[string]string)
	headers["From"] = from
	headers["To"] = to
	headers["Subject"] = subject
	headers["MIME-Version"] = "1.0"
	headers["Content-Type"] = "text/html; charset=UTF-8"

	// Build the message
	var message bytes.Buffer
	for key, value := range headers {
		message.WriteString(fmt.Sprintf("%s: %s\r\n", key, value))
	}
	message.WriteString("\r\n")
	message.WriteString(body)

	// Connect to the server and send the email
	addr := fmt.Sprintf("%s:%d", host, port)
	if auth != nil {
		if err := smtp.SendMail(addr, auth, from, []string{to}, message.Bytes()); err != nil {
			return fmt.Errorf("error sending email: %w", err)
		}
	} else {
		// Send without authentication
		client, err := smtp.Dial(addr)
		if err != nil {
			return fmt.Errorf("error connecting to SMTP server: %w", err)
		}
		defer client.Close()

		if err = client.Mail(from); err != nil {
			return fmt.Errorf("error setting sender: %w", err)
		}

		if err = client.Rcpt(to); err != nil {
			return fmt.Errorf("error setting recipient: %w", err)
		}

		w, err := client.Data()
		if err != nil {
			return fmt.Errorf("error creating email data writer: %w", err)
		}

		_, err = w.Write(message.Bytes())
		if err != nil {
			return fmt.Errorf("error writing email data: %w", err)
		}

		err = w.Close()
		if err != nil {
			return fmt.Errorf("error closing email data writer: %w", err)
		}

		err = client.Quit()
		if err != nil {
			return fmt.Errorf("error closing SMTP connection: %w", err)
		}
	}

	return nil
}

// SetTemplate allows setting a custom template
func (a *EmailAlert) SetTemplate(name, templateContent string) error {
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
		"lower": strings.ToLower,
	}).Parse(templateContent)

	if err != nil {
		return fmt.Errorf("error parsing template: %w", err)
	}

	a.Templates[name] = tmpl
	return nil
}

// Configure sets the SMTP configuration for this alert
func (a *EmailAlert) Configure(host string, port int, username, password, fromAddr string) {
	a.SMTPHost = host
	a.SMTPPort = port
	a.Username = username
	a.Password = password
	a.FromAddr = fromAddr
}
