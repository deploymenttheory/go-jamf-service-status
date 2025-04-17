package template

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/deploymenttheory/go-jamf-service-status/jamfstatus/pkg/core"
)

// ReportFormat defines the format for status reports
type ReportFormat string

const (
	FormatText     ReportFormat = "text"
	FormatHTML     ReportFormat = "html"
	FormatJSON     ReportFormat = "json"
	FormatMarkdown ReportFormat = "markdown"
)

// ReportGenerator handles generating status reports in various formats
type ReportGenerator struct {
	Templates *TemplateEngine
}

// NewReportGenerator creates a new report generator
func NewReportGenerator() *ReportGenerator {
	return &ReportGenerator{
		Templates: NewTemplateEngine(),
	}
}

// SetTemplateEngine sets a custom template engine
func (g *ReportGenerator) SetTemplateEngine(engine *TemplateEngine) {
	g.Templates = engine
}

// GenerateOutageReport creates a report for active outages
func (g *ReportGenerator) GenerateOutageReport(incidents []core.StatusIncident, format ReportFormat) (string, error) {
	if len(incidents) == 0 {
		return "No active outages detected.", nil
	}

	switch format {
	case FormatText:
		return g.Templates.RenderOutage("outage_text", incidents)
	case FormatHTML:
		return g.Templates.RenderOutage("outage_html", incidents)
	case FormatJSON:
		return convertToJSON(incidents)
	case FormatMarkdown:
		return g.generateMarkdownOutage(incidents)
	default:
		return "", fmt.Errorf("unsupported format: %s", format)
	}
}

// GenerateStatusReport creates a full status report
func (g *ReportGenerator) GenerateStatusReport(report *core.StatusReport, format ReportFormat) (string, error) {
	switch format {
	case FormatText:
		return g.Templates.RenderReport("report_text", report)
	case FormatHTML:
		return g.Templates.RenderReport("report_html", report)
	case FormatJSON:
		return convertToJSON(report)
	case FormatMarkdown:
		return g.generateMarkdownReport(report)
	default:
		return "", fmt.Errorf("unsupported format: %s", format)
	}
}

// GenerateSummaryReport creates a brief summary report
func (g *ReportGenerator) GenerateSummaryReport(report *core.StatusReport) (string, error) {
	return g.Templates.RenderReport("summary_text", report)
}

// AddCustomTemplate adds a custom template for report generation
func (g *ReportGenerator) AddCustomTemplate(name string, templateContent string) error {
	return g.Templates.AddTemplate(name, templateContent)
}

// generateMarkdownOutage creates a markdown formatted outage report
func (g *ReportGenerator) generateMarkdownOutage(incidents []core.StatusIncident) (string, error) {
	var sb strings.Builder

	sb.WriteString("# Jamf Status Alert: " + fmt.Sprintf("%d", len(incidents)) + " Active Outage(s)\n\n")

	for _, incident := range incidents {
		sb.WriteString("## " + incident.Title + "\n\n")
		sb.WriteString("**Status:** " + incident.Status + "\n\n")
		sb.WriteString("**Affected:** " + strings.Join(incident.AffectedRegions, ", ") + "\n\n")
		sb.WriteString("**Impact:** " + incident.Impact + "\n\n")

		if incident.Description != "" {
			sb.WriteString(incident.Description + "\n\n")
		}

		sb.WriteString("---\n\n")
	}

	sb.WriteString("*Last updated: " + time.Now().Format(time.RFC1123) + "*\n")

	return sb.String(), nil
}

// generateMarkdownReport creates a markdown formatted status report
func (g *ReportGenerator) generateMarkdownReport(report *core.StatusReport) (string, error) {
	var sb strings.Builder

	sb.WriteString("# Jamf Status Report\n\n")

	// Current Status
	sb.WriteString("## Current Status\n\n")
	if report.HasActiveOutages {
		sb.WriteString("⚠️ **There are " + fmt.Sprintf("%d", len(report.ActiveOutages)) + " active outage(s)**\n\n")
	} else {
		sb.WriteString("✅ **All systems operational**\n\n")
	}

	// Active Outages
	if report.HasActiveOutages {
		sb.WriteString("## Active Outages\n\n")
		for _, incident := range report.ActiveOutages {
			sb.WriteString("### " + incident.Title + "\n\n")
			sb.WriteString("**Status:** " + incident.Status + "\n\n")
			sb.WriteString("**Affected:** " + strings.Join(incident.AffectedRegions, ", ") + "\n\n")
			sb.WriteString("**Impact:** " + incident.Impact + "\n\n")
		}
	}

	// Planned Maintenance
	if len(report.PlannedMaintenance) > 0 {
		sb.WriteString("## Planned Maintenance\n\n")
		for _, maintenance := range report.PlannedMaintenance {
			sb.WriteString("### " + maintenance.Title + "\n\n")
			sb.WriteString("**Status:** " + maintenance.Status + "\n\n")

			timeStr := "N/A"
			if maintenance.PublishedAt != nil {
				timeStr = maintenance.PublishedAt.Format(time.RFC1123)
			}
			sb.WriteString("**Scheduled:** " + timeStr + "\n\n")

			sb.WriteString("**Affected:** " + strings.Join(maintenance.AffectedRegions, ", ") + "\n\n")
		}
	}

	// Service Status
	sb.WriteString("## Service Status\n\n")
	for _, service := range report.Services {
		statusIcon := "✅"
		if service.Status == "degraded" {
			statusIcon = "⚠️"
		} else if service.Status == "outage" {
			statusIcon = "❌"
		} else if service.Status == "maintenance" {
			statusIcon = "🔧"
		}

		sb.WriteString("- **" + service.Name + ":** " + statusIcon + " " +
			strings.Title(service.Status) + "\n")
	}

	sb.WriteString("\n*Last updated: " + report.LastChecked.Format(time.RFC1123) + "*\n")

	return sb.String(), nil
}

// Helper functions

// convertToJSON converts a struct to JSON string
func convertToJSON(data interface{}) (string, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetIndent("", "  ")
	if err := enc.Encode(data); err != nil {
		return "", fmt.Errorf("error encoding to JSON: %w", err)
	}
	return buf.String(), nil
}
