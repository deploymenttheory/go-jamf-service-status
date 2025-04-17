package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/deploymenttheory/go-jamf-service-status/jamfstatus/pkg/alert"
	"github.com/deploymenttheory/go-jamf-service-status/jamfstatus/pkg/client"
	"github.com/deploymenttheory/go-jamf-service-status/jamfstatus/pkg/core"
	"github.com/deploymenttheory/go-jamf-service-status/jamfstatus/pkg/healthcheck"
	"github.com/deploymenttheory/go-jamf-service-status/jamfstatus/pkg/monitor"
	"github.com/deploymenttheory/go-jamf-service-status/jamfstatus/pkg/template"
)

func main() {
	// Parse command line flags
	checkIntervalMinutes := flag.Int("interval", 5, "Check interval in minutes")
	checkOnlyFlag := flag.Bool("check", false, "Only check for active outages, exit with status code")
	outputFormat := flag.String("format", "text", "Output format (text, json, html, markdown)")
	slackWebhook := flag.String("slack-webhook", "", "Slack webhook URL for notifications")
	teamsWebhook := flag.String("teams-webhook", "", "Microsoft Teams webhook URL for notifications")
	emailRecipient := flag.String("email", "", "Email address for notifications")
	smtpHost := flag.String("smtp-host", "", "SMTP server host")
	smtpPort := flag.Int("smtp-port", 587, "SMTP server port")
	smtpUser := flag.String("smtp-user", "", "SMTP username")
	smtpPass := flag.String("smtp-pass", "", "SMTP password")
	prometheusAddr := flag.String("metrics-addr", "", "Address to expose Prometheus metrics (e.g. :9090)")
	verbose := flag.Bool("verbose", false, "Enable verbose logging")
	nagiosMode := flag.Bool("nagios", false, "Run in Nagios/Icinga compatible check mode")
	monitoredRegions := flag.String("regions", "", "Comma-separated list of regions to monitor")
	monitoredServices := flag.String("services", "", "Comma-separated list of services to monitor")
	version := flag.Bool("version", false, "Print version information and exit")

	flag.Parse()

	// Print version information if requested
	if *version {
		fmt.Println("Jamf Status Monitor v1.0.0")
		os.Exit(0)
	}

	// Create client with default options
	jamfClient := client.NewClient()

	// Run a single check if check-only mode is enabled
	if *checkOnlyFlag {
		runSingleCheck(jamfClient, *outputFormat, *nagiosMode, *monitoredRegions, *monitoredServices)
		return
	}

	// Set up alert manager if notification options are provided
	var alertMgr *alert.AlertManager
	if *slackWebhook != "" || *teamsWebhook != "" || *emailRecipient != "" {
		alertMgr = setupAlerts(*slackWebhook, *teamsWebhook, *emailRecipient, *smtpHost, *smtpPort, *smtpUser, *smtpPass)
	}

	// Create health checker
	healthChecker := healthcheck.NewServiceHealthChecker()

	// Add basic health checks
	healthChecker.AddCheck(healthcheck.NewEndpointCheck("jamf-status", "https://status.jamf.com", 10*time.Second))

	// Parse monitored regions and services
	var regions []string
	var services []string
	if *monitoredRegions != "" {
		regions = strings.Split(*monitoredRegions, ",")
	}
	if *monitoredServices != "" {
		services = strings.Split(*monitoredServices, ",")
	}

	// Create monitor options
	options := []monitor.MonitorOption{
		monitor.WithCheckInterval(time.Duration(*checkIntervalMinutes) * time.Minute),
		monitor.WithVerboseLogging(*verbose),
		monitor.WithHealthChecker(healthChecker),
		monitor.WithClient(jamfClient),
	}

	// Add alert manager if configured
	if alertMgr != nil {
		options = append(options, monitor.WithAlertManager(alertMgr))
	}

	// Add metrics endpoint if configured
	if *prometheusAddr != "" {
		options = append(options, monitor.WithMetricsEndpoint(*prometheusAddr, "/metrics"))
	}

	// Create and start monitor
	statusMonitor := monitor.NewMonitor(options...)
	statusMonitor.Start()

	fmt.Println("Jamf Status Monitor started")
	fmt.Printf("Checking status every %d minutes\n", *checkIntervalMinutes)

	// Set up signal handling for clean shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// Wait for signal
	<-sigChan
	fmt.Println("\nShutting down...")

	// Stop monitor and wait for completion
	statusMonitor.Stop()
	statusMonitor.Wait()
}

// runSingleCheck performs a single status check and exits
func runSingleCheck(jamfClient *client.Client, format string, nagiosMode bool, regionsStr, servicesStr string) {
	// Create context for the check
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Get the status report
	report, err := jamfClient.GetStatusReport(ctx, 1)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error checking status: %v\n", err)
		os.Exit(2)
	}

	// If nagios mode is enabled, run as a Nagios check
	if nagiosMode {
		nagiosCheck := monitor.NewNagiosCheck()

		// Set monitored regions if provided
		if regionsStr != "" {
			nagiosCheck.SetMonitoredRegions(strings.Split(regionsStr, ","))
		}

		// Set monitored services if provided
		if servicesStr != "" {
			nagiosCheck.SetMonitoredServices(strings.Split(servicesStr, ","))
		}

		// Run check and exit
		nagiosCheck.ExecCheck(report)
		return
	}

	// Otherwise, display the status based on format
	switch strings.ToLower(format) {
	case "json":
		displayJSON(report)
	case "html":
		displayHTML(report)
	case "markdown":
		displayMarkdown(report)
	default:
		displayText(report)
	}

	// Exit with appropriate status code
	if report.HasActiveOutages {
		os.Exit(1) // Outage detected
	}
	os.Exit(0) // No outages
}

// setupAlerts configures alert channels
func setupAlerts(slackWebhook, teamsWebhook, emailRecipient, smtpHost string, smtpPort int, smtpUser, smtpPass string) *alert.AlertManager {
	alertMgr := alert.NewAlertManager()

	// Configure Slack alerts if webhook is provided
	if slackWebhook != "" {
		slackAlert := alert.NewSlackAlert()
		alertMgr.RegisterAlert("slack", slackAlert)
		alertMgr.SetConfig("slack", "webhook_url", slackWebhook)
		fmt.Println("Configured Slack alerts")
	}

	// Configure Teams alerts if webhook is provided
	if teamsWebhook != "" {
		teamsAlert := alert.NewTeamsAlert()
		alertMgr.RegisterAlert("teams", teamsAlert)
		alertMgr.SetConfig("teams", "webhook_url", teamsWebhook)
		fmt.Println("Configured Microsoft Teams alerts")
	}

	// Configure email alerts if recipient is provided
	if emailRecipient != "" && smtpHost != "" {
		emailAlert := alert.NewEmailAlert()
		emailAlert.Configure(smtpHost, smtpPort, smtpUser, smtpPass, "jamf-status@example.com")

		alertMgr.RegisterAlert("email", emailAlert)
		alertMgr.SetConfig("email", "recipient", emailRecipient)
		alertMgr.SetConfig("email", "smtp_host", smtpHost)
		alertMgr.SetConfig("email", "smtp_port", fmt.Sprintf("%d", smtpPort))

		if smtpUser != "" && smtpPass != "" {
			alertMgr.SetConfig("email", "smtp_username", smtpUser)
			alertMgr.SetConfig("email", "smtp_password", smtpPass)
		}

		fmt.Println("Configured email alerts")
	}

	return alertMgr
}

// Display functions for different output formats
func displayText(report *core.StatusReport) {
	fmt.Println("=== JAMF STATUS MONITOR ===")
	fmt.Printf("Last Checked: %s\n\n", report.LastChecked.Format(time.RFC1123))

	// Display active outages first
	if report.HasActiveOutages {
		fmt.Println("🚨 ACTIVE OUTAGES DETECTED 🚨")
		fmt.Printf("Found %d active outage(s)\n\n", len(report.ActiveOutages))

		for i, incident := range report.ActiveOutages {
			fmt.Printf("%d. %s - %s\n", i+1, incident.Title, incident.Status)
			fmt.Printf("   Affected: %s\n", strings.Join(incident.AffectedRegions, ", "))
			fmt.Printf("   Impact: %s\n", incident.Impact)
			if incident.Description != "" {
				fmt.Printf("   Description: %s\n", incident.Description)
			}
			fmt.Println()
		}
	} else {
		fmt.Println("✅ NO ACTIVE OUTAGES DETECTED")
	}

	// Display planned maintenance
	if len(report.PlannedMaintenance) > 0 {
		fmt.Println("\n--- PLANNED MAINTENANCE ---")
		for i, maintenance := range report.PlannedMaintenance {
			fmt.Printf("%d. %s - %s\n", i+1, maintenance.Title, maintenance.Status)
			if maintenance.PublishedAt != nil {
				fmt.Printf("   Scheduled: %s\n", maintenance.PublishedAt.Format(time.RFC1123))
			}
			fmt.Printf("   Affected: %s\n", strings.Join(maintenance.AffectedRegions, ", "))
			fmt.Println()
		}
	}

	// Display service status
	fmt.Println("\n--- SERVICE STATUS ---")
	for _, service := range report.Services {
		status := "✅ Operational"
		if service.Status == "degraded" {
			status = "⚠️ Degraded"
		} else if service.Status == "outage" {
			status = "❌ Outage"
		} else if service.Status == "maintenance" {
			status = "🔧 Maintenance"
		}

		fmt.Printf("%s: %s\n", service.Name, status)
	}
}

func displayJSON(report *core.StatusReport) {
	json, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error generating JSON: %v\n", err)
		return
	}

	fmt.Println(string(json))
}

func displayHTML(report *core.StatusReport) {
	// Create template engine
	templateEngine := template.NewTemplateEngine()

	// Render report
	html, err := templateEngine.RenderReport("report_html", report)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error generating HTML: %v\n", err)
		return
	}

	fmt.Println(html)
}

func displayMarkdown(report *core.StatusReport) {
	// Create report generator
	reportGen := template.NewReportGenerator()

	// Generate markdown report
	markdown, err := reportGen.GenerateStatusReport(report, template.FormatMarkdown)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error generating markdown: %v\n", err)
		return
	}

	fmt.Println(markdown)
}
