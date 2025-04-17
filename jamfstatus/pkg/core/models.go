package core

import (
	"time"
)

// StatusIncident represents a parsed incident from the Jamf status feed
type StatusIncident struct {
	Title           string         `json:"title"`
	Link            string         `json:"link"`
	PublishedAt     *time.Time     `json:"published_at"`
	Status          string         `json:"status"`
	Description     string         `json:"description"`
	IsOutage        bool           `json:"is_outage"`
	IsResolved      bool           `json:"is_resolved"`
	AffectedRegions []string       `json:"affected_regions"`
	Updates         []StatusUpdate `json:"updates"`
	// Additional fields for extended functionality
	ID          string            `json:"id"`           // Unique identifier from the feed
	Category    string            `json:"category"`     // Service category affected
	StartTime   *time.Time        `json:"start_time"`   // When the incident started
	EndTime     *time.Time        `json:"end_time"`     // When the incident was resolved (if applicable)
	IsPlanned   bool              `json:"is_planned"`   // Is this a planned maintenance?
	Impact      string            `json:"impact"`       // High, Medium, Low
	ExternalIDs map[string]string `json:"external_ids"` // IDs in external systems (e.g., JIRA)
}

// StatusUpdate represents a single update within an incident
type StatusUpdate struct {
	Timestamp time.Time `json:"timestamp"`
	Status    string    `json:"status"`
	Message   string    `json:"message"`
}

// StatusReport contains a comprehensive view of Jamf status
type StatusReport struct {
	LastChecked      time.Time        `json:"last_checked"`
	ActiveOutages    []StatusIncident `json:"active_outages"`
	RecentIncidents  []StatusIncident `json:"recent_incidents"`
	HasActiveOutages bool             `json:"has_active_outages"`
	// Additional fields for more context
	PlannedMaintenance []StatusIncident   `json:"planned_maintenance"` // Upcoming maintenance
	Services           []ServiceStatus    `json:"services"`            // Status by service
	HistoricalUptime   map[string]float64 `json:"historical_uptime"`   // Calculated uptime by region/service
}

// ServiceStatus provides status for a specific Jamf service
type ServiceStatus struct {
	Name         string     `json:"name"`          // Service name
	Status       string     `json:"status"`        // Status (operational, degraded, outage)
	LastIncident *time.Time `json:"last_incident"` // Last incident time
	Regions      []string   `json:"regions"`       // Affected regions
	Components   []string   `json:"components"`    // Affected components
}

// HealthStatus combines external status with internal health checks
type HealthStatus struct {
	JamfStatus    *StatusReport     `json:"jamf_status"`    // External Jamf status
	LocalChecks   map[string]bool   `json:"local_checks"`   // Results of local health checks
	Connectivity  map[string]string `json:"connectivity"`   // API/endpoint connectivity
	OverallHealth string            `json:"overall_health"` // Overall health assessment
}

// AlertConfig contains configuration for status alerts
type AlertConfig struct {
	Channels       []string `json:"channels"`        // Channels to alert on (slack, email, etc)
	MinSeverity    string   `json:"min_severity"`    // Minimum severity to trigger alert
	Regions        []string `json:"regions"`         // Regions to monitor
	Services       []string `json:"services"`        // Services to monitor
	CheckInterval  int      `json:"check_interval"`  // How often to check in seconds
	ThrottleWindow int      `json:"throttle_window"` // Min time between alerts in seconds
}

// RegionKeywords maps region identifiers to keyword patterns
var RegionKeywords = map[string][]string{
	"US": {
		"us", "us-east", "us-west", "united states", "us standard", "us region",
		"us-east-1", "us-east-2", "us-west-1", "us-west-2",
	},
	"EU": {
		"eu", "eu-central", "eu-west", "europe", "eu standard", "eu region",
		"eu-central-1", "eu-west-1", "eu-west-2", "eu-south-1", "eu-north-1",
	},
	"APAC": {
		"apac", "ap-northeast", "ap-southeast", "asia", "asia pacific", "australia",
		"ap-northeast-1", "ap-northeast-2", "ap-southeast-1", "ap-southeast-2",
	},
	"Global": {
		"all regions", "global", "worldwide", "all instances", "all customers", "all classes",
	},
	"Production": {
		"production", "prod instances", "prod environment", "standard production",
	},
	"Non-Production": {
		"sandbox", "trial", "beta", "non-prod", "non-production", "test",
	},
}
