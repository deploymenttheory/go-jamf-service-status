package core

import (
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/mmcdole/gofeed"
	"github.com/mmcdole/gofeed/rss"
	"golang.org/x/net/html"
)

// JamfStatusTranslator is a custom translator for Jamf status feed
type JamfStatusTranslator struct {
	defaultTranslator *gofeed.DefaultRSSTranslator
}

// NewJamfStatusTranslator creates a new custom translator
func NewJamfStatusTranslator() *JamfStatusTranslator {
	t := &JamfStatusTranslator{}
	t.defaultTranslator = &gofeed.DefaultRSSTranslator{}
	return t
}

// Translate implements the gofeed.Translator interface
func (t *JamfStatusTranslator) Translate(feed interface{}) (*gofeed.Feed, error) {
	rssFeed, found := feed.(*rss.Feed)
	if !found {
		return nil, fmt.Errorf("feed did not match expected type of *rss.Feed")
	}

	// Use the default translator for basic conversion
	f, err := t.defaultTranslator.Translate(rssFeed)
	if err != nil {
		return nil, err
	}

	// Process maintenanceEndDate extension if needed
	for i, item := range f.Items {
		if ext, ok := rssFeed.Items[i].Extensions["maintenanceEndDate"]; ok && len(ext) > 0 {
			// Store this information in the item's custom fields
			if item.Custom == nil {
				item.Custom = make(map[string]string)
			}
			item.Custom["maintenanceEndDate"] = "true"
		}
	}

	return f, nil
}

// ParseIncidents extracts structured data from feed items
func ParseIncidents(items []*gofeed.Item) []StatusIncident {
	var incidents []StatusIncident

	for _, item := range items {
		// Extract status updates from description
		updates := ExtractStatusUpdates(item.Description)

		// Get current status and determine if resolved
		currentStatus := ""
		isResolved := false
		description := ""

		if len(updates) > 0 {
			currentStatus = updates[0].Status
			description = updates[0].Message
			isResolved = IsResolvedStatus(currentStatus)
		}

		// Check if this is a maintenance item that's completed
		if item.Custom != nil {
			if _, ok := item.Custom["maintenanceEndDate"]; ok {
				isResolved = true
				if currentStatus == "" {
					currentStatus = "Completed"
				}
			}
		}

		// Determine if this is an outage based on title
		isOutage := IsOutageTitle(item.Title)

		// Don't consider scheduled maintenance as an outage if it's resolved
		if IsScheduledMaintenance(item.Title) && isResolved {
			isOutage = false
		}

		// Extract affected regions
		affectedRegions := DetectAffectedRegions(item.Title, item.Description)

		// Determine if this is a planned maintenance
		isPlanned := IsScheduledMaintenance(item.Title)

		// Determine the impact level
		impact := DetermineImpact(item.Title, item.Description)

		// Create a unique ID from title and published date
		id := CreateIncidentID(item)

		var startTime *time.Time
		var endTime *time.Time

		// Set start time to published time
		startTime = item.PublishedParsed

		// If resolved, set end time to the time of the last update
		if isResolved && len(updates) > 0 {
			lastUpdate := updates[len(updates)-1].Timestamp
			endTime = &lastUpdate
		}

		incident := StatusIncident{
			Title:           item.Title,
			Link:            item.Link,
			PublishedAt:     item.PublishedParsed,
			Status:          currentStatus,
			Description:     description,
			IsOutage:        isOutage,
			IsResolved:      isResolved,
			AffectedRegions: affectedRegions,
			Updates:         updates,
			ID:              id,
			Category:        DetermineCategory(item.Title),
			StartTime:       startTime,
			EndTime:         endTime,
			IsPlanned:       isPlanned,
			Impact:          impact,
			ExternalIDs:     make(map[string]string),
		}

		incidents = append(incidents, incident)
	}

	return incidents
}

// ExtractStatusUpdates parses the HTML description to extract updates
func ExtractStatusUpdates(description string) []StatusUpdate {
	var updates []StatusUpdate

	// Use the standard HTML parser
	doc, err := html.Parse(strings.NewReader(description))
	if err != nil {
		return updates
	}

	// Find all paragraph elements
	var paragraphs []*html.Node
	var findParagraphs func(*html.Node)
	findParagraphs = func(n *html.Node) {
		if n.Type == html.ElementNode && n.Data == "p" {
			paragraphs = append(paragraphs, n)
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			findParagraphs(c)
		}
	}
	findParagraphs(doc)

	// Process each paragraph for status updates
	for _, p := range paragraphs {
		var timeStr, status, message string

		// Find the <small> tag for timestamp
		var small *html.Node
		for c := p.FirstChild; c != nil; c = c.NextSibling {
			if c.Type == html.ElementNode && c.Data == "small" {
				small = c
				break
			}
		}

		// Find the <strong> tag for status
		var strong *html.Node
		for c := p.FirstChild; c != nil; c = c.NextSibling {
			if c.Type == html.ElementNode && c.Data == "strong" {
				strong = c
				break
			}
		}

		// Extract timestamp text
		if small != nil {
			timeStr = RenderNode(small)
		}

		// Extract status text
		if strong != nil {
			status = RenderNode(strong)
		}

		// Extract message (everything after the strong tag)
		var messageBuilder strings.Builder
		var pastStrong bool
		for c := p.FirstChild; c != nil; c = c.NextSibling {
			if pastStrong {
				messageBuilder.WriteString(RenderNode(c))
			}
			if c == strong {
				pastStrong = true
			}
		}
		message = strings.TrimSpace(messageBuilder.String())

		// Remove leading dash if present
		if strings.HasPrefix(message, "- ") {
			message = message[2:]
		} else if strings.HasPrefix(message, "-") {
			message = message[1:]
		}

		// Skip if we couldn't extract key information
		if timeStr == "" || status == "" {
			continue
		}

		// Parse the timestamp
		timestamp, err := ParseTimestamp(timeStr)
		if err != nil {
			timestamp = time.Now() // Fallback to current time
		}

		update := StatusUpdate{
			Timestamp: timestamp,
			Status:    status,
			Message:   message,
		}

		updates = append(updates, update)
	}

	return updates
}

// RenderNode extracts text content from an HTML node
func RenderNode(n *html.Node) string {
	if n.Type == html.TextNode {
		return n.Data
	}

	var result strings.Builder
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		result.WriteString(RenderNode(c))
	}

	return result.String()
}

// IsResolvedStatus checks if the status indicates resolution
func IsResolvedStatus(status string) bool {
	status = strings.ToLower(status)
	resolvedKeywords := []string{"resolved", "completed", "fixed"}

	for _, keyword := range resolvedKeywords {
		if strings.Contains(status, keyword) {
			return true
		}
	}

	return false
}

// IsOutageTitle determines if an incident title suggests an outage
func IsOutageTitle(title string) bool {
	title = strings.ToLower(title)
	outageKeywords := []string{"outage", "down", "unavailable", "degraded", "issue", "incident", "misconfiguration", "disruption"}

	for _, keyword := range outageKeywords {
		if strings.Contains(title, keyword) {
			return true
		}
	}

	return false
}

// IsScheduledMaintenance determines if an incident is scheduled maintenance
func IsScheduledMaintenance(title string) bool {
	title = strings.ToLower(title)
	keywords := []string{"upgrade", "maintenance", "scheduled", "planned", "update"}

	for _, keyword := range keywords {
		if strings.Contains(title, keyword) {
			return true
		}
	}

	return false
}

// DetectAffectedRegions analyzes text to identify affected regions
func DetectAffectedRegions(title, description string) []string {
	combinedText := strings.ToLower(title + " " + description)
	var detectedRegions []string
	regionsMap := make(map[string]bool)

	// Extract regions from title (e.g., "for us-west-2")
	regionPattern := regexp.MustCompile(`\s+for\s+([a-z0-9-]+)`)
	matches := regionPattern.FindStringSubmatch(strings.ToLower(title))
	if len(matches) >= 2 {
		regionCode := matches[1]
		for region, keywords := range RegionKeywords {
			for _, keyword := range keywords {
				if keyword == regionCode {
					regionsMap[region] = true
					break
				}
			}
		}
	}

	// Check for each region keyword in the combined text
	for region, keywords := range RegionKeywords {
		for _, keyword := range keywords {
			if strings.Contains(combinedText, keyword) {
				regionsMap[region] = true
				break
			}
		}
	}

	// If no specific regions found, assume Global
	if len(regionsMap) == 0 {
		return []string{"Global"}
	}

	// Convert map to slice
	for region := range regionsMap {
		detectedRegions = append(detectedRegions, region)
	}

	return detectedRegions
}

// ParseTimestamp converts the Jamf status timestamp format to time.Time
func ParseTimestamp(timeStr string) (time.Time, error) {
	timeStr = strings.TrimSpace(timeStr)
	currentYear := time.Now().Year()

	// Format: "Apr 16, 20:32 UTC"
	fullTimeStr := fmt.Sprintf("%s %d", timeStr, currentYear)

	// Try a few different formats
	formats := []string{
		"Jan 2, 15:04 UTC 2006",
		"Jan 2, 15:04:05 UTC 2006",
		"January 2, 15:04 UTC 2006",
	}

	for _, format := range formats {
		t, err := time.Parse(format, fullTimeStr)
		if err == nil {
			return t, nil
		}
	}

	return time.Time{}, fmt.Errorf("could not parse time: %s", timeStr)
}

// FilterActiveOutages returns incidents that are outages and not resolved
func FilterActiveOutages(incidents []StatusIncident) []StatusIncident {
	var activeOutages []StatusIncident

	for _, incident := range incidents {
		if incident.IsOutage && !incident.IsResolved {
			activeOutages = append(activeOutages, incident)
		}
	}

	return activeOutages
}

// FilterRecentIncidents returns incidents within the last X days
func FilterRecentIncidents(incidents []StatusIncident, days int) []StatusIncident {
	var recentIncidents []StatusIncident
	cutoff := time.Now().AddDate(0, 0, -days)

	for _, incident := range incidents {
		if incident.PublishedAt != nil && incident.PublishedAt.After(cutoff) {
			recentIncidents = append(recentIncidents, incident)
		}
	}

	return recentIncidents
}

// FilterPlannedMaintenance returns incidents that are planned maintenance and not resolved
func FilterPlannedMaintenance(incidents []StatusIncident) []StatusIncident {
	var planned []StatusIncident
	for _, incident := range incidents {
		if incident.IsPlanned && !incident.IsResolved {
			planned = append(planned, incident)
		}
	}
	return planned
}

// DetermineCategory extracts the service category from incident title
func DetermineCategory(title string) string {
	// Map of keywords to categories
	categoryMap := map[string]string{
		"cloud":       "Cloud Services",
		"api":         "API Services",
		"login":       "Authentication",
		"auth":        "Authentication",
		"mdm":         "Device Management",
		"admin":       "Administration",
		"inventory":   "Inventory",
		"app catalog": "App Catalog",
		"self help":   "Self Service",
		"database":    "Database",
	}

	title = strings.ToLower(title)

	// Check for each category keyword
	for keyword, category := range categoryMap {
		if strings.Contains(title, keyword) {
			return category
		}
	}

	// Default category if none detected
	return "General"
}

// DetermineImpact analyzes incident to assign impact level
func DetermineImpact(title, description string) string {
	// Combine title and description for analysis
	text := strings.ToLower(title + " " + description)

	// Check for high impact keywords
	highImpactKeywords := []string{"major outage", "complete outage", "unavailable", "all regions"}
	for _, keyword := range highImpactKeywords {
		if strings.Contains(text, keyword) {
			return "High"
		}
	}

	// Check for medium impact keywords
	mediumImpactKeywords := []string{"degraded", "intermittent", "slow performance"}
	for _, keyword := range mediumImpactKeywords {
		if strings.Contains(text, keyword) {
			return "Medium"
		}
	}

	// Check for low impact keywords
	lowImpactKeywords := []string{"minor", "partial", "isolated"}
	for _, keyword := range lowImpactKeywords {
		if strings.Contains(text, keyword) {
			return "Low"
		}
	}

	// Default to Medium impact if no keywords match
	return "Medium"
}

// CreateIncidentID generates a unique ID for an incident
func CreateIncidentID(item *gofeed.Item) string {
	// Start with a simple hash of the title
	id := fmt.Sprintf("%s", item.GUID)

	// If GUID is empty, create an ID from title and date
	if id == "" {
		timestamp := "unknown"
		if item.PublishedParsed != nil {
			timestamp = item.PublishedParsed.Format("20060102150405")
		}
		id = fmt.Sprintf("incident-%s-%s", timestamp, strings.ReplaceAll(strings.ToLower(item.Title), " ", "-"))
	}

	return id
}

// BuildServiceStatus categorizes incidents by service
func BuildServiceStatus(incidents []StatusIncident) []ServiceStatus {
	// Group incidents by service category
	serviceMap := make(map[string]*ServiceStatus)

	for _, incident := range incidents {
		if _, exists := serviceMap[incident.Category]; !exists {
			serviceMap[incident.Category] = &ServiceStatus{
				Name:    incident.Category,
				Status:  "operational", // Default to operational
				Regions: make([]string, 0),
			}
		}

		// Update service status based on incident
		service := serviceMap[incident.Category]

		// Add regions (deduplicating)
		regionMap := make(map[string]bool)
		for _, region := range service.Regions {
			regionMap[region] = true
		}
		for _, region := range incident.AffectedRegions {
			regionMap[region] = true
		}
		service.Regions = make([]string, 0, len(regionMap))
		for region := range regionMap {
			service.Regions = append(service.Regions, region)
		}

		// Update last incident time if newer
		if incident.PublishedAt != nil && (service.LastIncident == nil ||
			incident.PublishedAt.After(*service.LastIncident)) {
			service.LastIncident = incident.PublishedAt
		}

		// Update status based on incident (if not resolved)
		if !incident.IsResolved {
			if incident.IsOutage {
				if incident.Impact == "High" {
					service.Status = "outage"
				} else {
					service.Status = "degraded"
				}
			} else if service.Status == "operational" && incident.IsPlanned {
				service.Status = "maintenance"
			}
		}
	}

	// Convert map to slice
	services := make([]ServiceStatus, 0, len(serviceMap))
	for _, service := range serviceMap {
		services = append(services, *service)
	}

	return services
}

// CalculateHistoricalUptime analyzes historical incidents to calculate uptime percentages
func CalculateHistoricalUptime(incidents []StatusIncident, days int) map[string]float64 {
	// Initialize uptime map by region
	uptime := make(map[string]float64)
	regions := make(map[string]bool)

	// Calculate start time for analysis
	startTime := time.Now().AddDate(0, 0, -days)
	totalDurationHours := float64(days) * 24

	// Identify all regions in incidents
	for _, incident := range incidents {
		for _, region := range incident.AffectedRegions {
			regions[region] = true
		}
	}

	// Initialize uptime map with 100% for all regions
	for region := range regions {
		uptime[region] = 100.0
	}

	// Calculate downtime for each region
	for _, incident := range incidents {
		// Skip incidents that are too old
		if incident.PublishedAt == nil || incident.PublishedAt.Before(startTime) {
			continue
		}

		// Skip non-outage incidents
		if !incident.IsOutage {
			continue
		}

		// Calculate outage duration
		var endTime time.Time
		if incident.EndTime != nil {
			endTime = *incident.EndTime
		} else if incident.IsResolved {
			// If resolved but no end time set, use the first update with resolved status
			endTime = time.Now()
			for _, update := range incident.Updates {
				if IsResolvedStatus(update.Status) {
					endTime = update.Timestamp
					break
				}
			}
		} else {
			// If not resolved, use current time
			endTime = time.Now()
		}

		// Use start time
		startTimeIncident := startTime
		if incident.StartTime != nil && incident.StartTime.After(startTime) {
			startTimeIncident = *incident.StartTime
		}

		// Calculate duration in hours
		duration := endTime.Sub(startTimeIncident).Hours()

		// Cap at the total analysis period
		if duration > totalDurationHours {
			duration = totalDurationHours
		}

		// Adjust uptime for each affected region
		for _, region := range incident.AffectedRegions {
			downtimePercentage := (duration / totalDurationHours) * 100

			// Adjust the impact based on the incident severity
			switch incident.Impact {
			case "High":
				downtimePercentage = downtimePercentage * 1.0 // 100% impact
			case "Medium":
				downtimePercentage = downtimePercentage * 0.5 // 50% impact
			case "Low":
				downtimePercentage = downtimePercentage * 0.1 // 10% impact
			}

			// Update uptime
			uptime[region] -= downtimePercentage

			// Ensure uptime doesn't go below 0
			if uptime[region] < 0 {
				uptime[region] = 0
			}
		}
	}

	return uptime
}
