package client

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/deploymenttheory/go-jamf-service-status/jamfstatus/pkg/core"
	"github.com/mmcdole/gofeed"
)

// Client represents a Jamf status client
type Client struct {
	// HTTP client settings
	BaseURL    string
	UserAgent  string
	Timeout    time.Duration
	HTTPClient *http.Client

	// Parser settings
	Parser     *gofeed.Parser
	Translator gofeed.Translator
}

// ClientOption allows for functional options to configure the client
type ClientOption func(*Client)

// NewClient creates a new Jamf status client with default settings
func NewClient(options ...ClientOption) *Client {
	// Default client configuration
	c := &Client{
		BaseURL:    "https://status.jamf.com/history.rss",
		UserAgent:  "JamfStatus/1.0",
		Timeout:    30 * time.Second,
		HTTPClient: &http.Client{Timeout: 30 * time.Second},
	}

	// Initialize the feed parser
	c.Parser = gofeed.NewParser()
	c.Parser.UserAgent = c.UserAgent

	// Apply all provided options
	for _, option := range options {
		option(c)
	}

	return c
}

// WithBaseURL sets the base URL for the status feed
func WithBaseURL(url string) ClientOption {
	return func(c *Client) {
		c.BaseURL = url
	}
}

// WithUserAgent sets the User-Agent header for requests
func WithUserAgent(userAgent string) ClientOption {
	return func(c *Client) {
		c.UserAgent = userAgent
		if c.Parser != nil {
			c.Parser.UserAgent = userAgent
		}
	}
}

// WithTimeout sets the timeout for HTTP requests
func WithTimeout(timeout time.Duration) ClientOption {
	return func(c *Client) {
		c.Timeout = timeout
		c.HTTPClient.Timeout = timeout
	}
}

// WithHTTPClient sets a custom HTTP client
func WithHTTPClient(httpClient *http.Client) ClientOption {
	return func(c *Client) {
		c.HTTPClient = httpClient
	}
}

// WithTranslator sets a custom feed translator
func WithTranslator(translator gofeed.Translator) ClientOption {
	return func(c *Client) {
		c.Translator = translator
		if c.Parser != nil && translator != nil {
			c.Parser.RSSTranslator = translator
		}
	}
}

// GetStatusReport fetches and processes the status feed
func (c *Client) GetStatusReport(ctx context.Context, daysToCheck int) (*core.StatusReport, error) {
	// Create a new context with timeout if none is provided
	if ctx == nil {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(context.Background(), c.Timeout)
		defer cancel()
	}

	// Parse the feed with context
	feed, err := c.Parser.ParseURLWithContext(c.BaseURL, ctx)
	if err != nil {
		return nil, fmt.Errorf("error parsing feed: %w", err)
	}

	// Validate feed data
	if feed == nil || len(feed.Items) == 0 {
		return nil, errors.New("feed contains no items")
	}

	// Process feed items into incidents
	incidents, err := c.processIncidents(feed.Items)
	if err != nil {
		return nil, fmt.Errorf("error processing incidents: %w", err)
	}

	// Filter active outages and recent incidents
	activeOutages := filterActiveOutages(incidents)
	recentIncidents := filterRecentIncidents(incidents, daysToCheck)
	plannedMaintenance := filterPlannedMaintenance(incidents)

	// Build service status map
	serviceStatus := buildServiceStatus(incidents)

	// Create the report
	report := &core.StatusReport{
		LastChecked:        time.Now(),
		ActiveOutages:      activeOutages,
		RecentIncidents:    recentIncidents,
		HasActiveOutages:   len(activeOutages) > 0,
		PlannedMaintenance: plannedMaintenance,
		Services:           serviceStatus,
		HistoricalUptime:   calculateHistoricalUptime(incidents),
	}

	return report, nil
}

// ImmediateCheck performs a quick check for active outages with minimal processing
func (c *Client) ImmediateCheck(ctx context.Context) (bool, error) {
	// Create a context with short timeout for quick checks
	if ctx == nil {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
	}

	// Parse the feed with context
	feed, err := c.Parser.ParseURLWithContext(c.BaseURL, ctx)
	if err != nil {
		return false, fmt.Errorf("error parsing feed: %w", err)
	}

	// Quick check for outages without full processing
	for _, item := range feed.Items {
		// Skip resolved incidents
		if isResolved(item.Description) {
			continue
		}

		// Check if the title indicates an outage
		if isOutageTitle(item.Title) {
			return true, nil // Found active outage
		}
	}

	return false, nil // No active outages
}

// GetHealthStatus combines status report with additional health checks
func (c *Client) GetHealthStatus(ctx context.Context, healthChecks map[string]func() bool) (*core.HealthStatus, error) {
	// Get the status report
	report, err := c.GetStatusReport(ctx, 1) // Just recent incidents
	if err != nil {
		return nil, err
	}

	// Run health checks
	checkResults := make(map[string]bool)
	for name, check := range healthChecks {
		checkResults[name] = check()
	}

	// Check API connectivity
	connectivity := make(map[string]string)
	connectivity["status_feed"] = "ok" // We already fetched the feed

	// Additional connectivity checks could be added here

	// Determine overall health
	overallHealth := "healthy"
	if report.HasActiveOutages {
		overallHealth = "degraded"
	}

	// Check if any local checks failed
	for _, result := range checkResults {
		if !result {
			overallHealth = "unhealthy"
			break
		}
	}

	return &core.HealthStatus{
		JamfStatus:    report,
		LocalChecks:   checkResults,
		Connectivity:  connectivity,
		OverallHealth: overallHealth,
	}, nil
}

// Helper functions
func (c *Client) processIncidents(items []*gofeed.Item) ([]core.StatusIncident, error) {
	// Implementation to process feed items into incidents
	// This would include the HTML parsing and status extraction logic
	// ...

	// Placeholder for now
	return []core.StatusIncident{}, nil
}

func filterActiveOutages(incidents []core.StatusIncident) []core.StatusIncident {
	var activeOutages []core.StatusIncident
	for _, incident := range incidents {
		if incident.IsOutage && !incident.IsResolved {
			activeOutages = append(activeOutages, incident)
		}
	}
	return activeOutages
}

func filterRecentIncidents(incidents []core.StatusIncident, days int) []core.StatusIncident {
	var recentIncidents []core.StatusIncident
	cutoff := time.Now().AddDate(0, 0, -days)

	for _, incident := range incidents {
		if incident.PublishedAt != nil && incident.PublishedAt.After(cutoff) {
			recentIncidents = append(recentIncidents, incident)
		}
	}
	return recentIncidents
}

func filterPlannedMaintenance(incidents []core.StatusIncident) []core.StatusIncident {
	var planned []core.StatusIncident
	for _, incident := range incidents {
		if incident.IsPlanned && !incident.IsResolved {
			planned = append(planned, incident)
		}
	}
	return planned
}

func buildServiceStatus(incidents []core.StatusIncident) []core.ServiceStatus {
	// Implementation to build status by service
	// This would analyze incidents and create a status for each service
	// ...

	// Placeholder for now
	return []core.ServiceStatus{}
}

func calculateHistoricalUptime(incidents []core.StatusIncident) map[string]float64 {
	// Implementation to calculate uptime percentages
	// ...

	// Placeholder for now
	return map[string]float64{}
}

func isResolved(description string) bool {
	// Quick check if the incident is resolved
	// Implementation would check the status in the description
	// ...

	// Placeholder for now
	return false
}

func isOutageTitle(title string) bool {
	// Implementation to check if the title indicates an outage
	// ...

	// Placeholder for now
	return false
}
