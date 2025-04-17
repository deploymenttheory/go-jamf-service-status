package monitor

import (
	"fmt"
	"os"
	"strings"

	"github.com/deploymenttheory/go-jamf-service-status/jamfstatus/pkg/core"
)

// NagiosStatusCode represents Nagios status codes
type NagiosStatusCode int

const (
	NagiosOK       NagiosStatusCode = 0
	NagiosWarning  NagiosStatusCode = 1
	NagiosCritical NagiosStatusCode = 2
	NagiosUnknown  NagiosStatusCode = 3
)

// NagiosCheck performs a Nagios-compatible check of Jamf status
type NagiosCheck struct {
	WarningThreshold  int
	CriticalThreshold int
	MonitoredRegions  []string
	MonitoredServices []string
}

// NewNagiosCheck creates a new Nagios check with default thresholds
func NewNagiosCheck() *NagiosCheck {
	return &NagiosCheck{
		WarningThreshold:  0, // Any outage is a warning
		CriticalThreshold: 1, // More than 1 outage is critical
		MonitoredRegions:  []string{},
		MonitoredServices: []string{},
	}
}

// CheckStatus performs a Nagios-compatible check of a status report
func (n *NagiosCheck) CheckStatus(report *core.StatusReport) (NagiosStatusCode, string) {
	if report == nil {
		return NagiosUnknown, "UNKNOWN - Failed to retrieve Jamf status"
	}

	// Count relevant outages
	relevantOutages := n.countRelevantOutages(report.ActiveOutages)

	// Determine status code
	var statusCode NagiosStatusCode
	if relevantOutages == 0 {
		statusCode = NagiosOK
	} else if relevantOutages <= n.WarningThreshold {
		statusCode = NagiosWarning
	} else if relevantOutages <= n.CriticalThreshold {
		statusCode = NagiosCritical
	} else {
		statusCode = NagiosCritical
	}

	// Create status message
	var message string
	switch statusCode {
	case NagiosOK:
		message = "OK - Jamf services operational"
	case NagiosWarning:
		message = fmt.Sprintf("WARNING - %d Jamf service issue(s) detected", relevantOutages)
	case NagiosCritical:
		message = fmt.Sprintf("CRITICAL - %d Jamf service outage(s) detected", relevantOutages)
	default:
		message = "UNKNOWN - Unable to determine Jamf status"
	}

	// Add details if we have outages
	if relevantOutages > 0 {
		message += "\n"
		for i, outage := range report.ActiveOutages {
			if n.isRelevantOutage(outage) {
				message += fmt.Sprintf("%d. %s - %s - Affected: %s\n",
					i+1,
					outage.Title,
					outage.Status,
					strings.Join(outage.AffectedRegions, ", "))
			}
		}
	}

	// Add service status for monitored services
	if len(n.MonitoredServices) > 0 {
		message += "\nService Status:\n"
		for _, service := range report.Services {
			if n.isMonitoredService(service.Name) {
				message += fmt.Sprintf("- %s: %s\n", service.Name, service.Status)
			}
		}
	}

	return statusCode, message
}

// ExecCheck runs a Nagios check and exits with the appropriate status code
func (n *NagiosCheck) ExecCheck(report *core.StatusReport) {
	statusCode, message := n.CheckStatus(report)

	// Print message
	fmt.Println(message)

	// Exit with the appropriate status code
	os.Exit(int(statusCode))
}

// countRelevantOutages counts the outages that match the criteria
func (n *NagiosCheck) countRelevantOutages(outages []core.StatusIncident) int {
	count := 0
	for _, outage := range outages {
		if n.isRelevantOutage(outage) {
			count++
		}
	}
	return count
}

// isRelevantOutage checks if an outage is relevant to this monitor
func (n *NagiosCheck) isRelevantOutage(outage core.StatusIncident) bool {
	// If we have no region filters, all outages are relevant
	if len(n.MonitoredRegions) == 0 {
		return true
	}

	// Check if any monitored region is affected
	for _, monitoredRegion := range n.MonitoredRegions {
		for _, affectedRegion := range outage.AffectedRegions {
			if monitoredRegion == affectedRegion {
				return true
			}
		}
	}

	return false
}

// isMonitoredService checks if a service is monitored
func (n *NagiosCheck) isMonitoredService(serviceName string) bool {
	// If we have no service filters, all services are monitored
	if len(n.MonitoredServices) == 0 {
		return true
	}

	// Check if service is in the monitored list
	for _, monitored := range n.MonitoredServices {
		if monitored == serviceName {
			return true
		}
	}

	return false
}

// SetMonitoredRegions sets the regions to monitor
func (n *NagiosCheck) SetMonitoredRegions(regions []string) {
	n.MonitoredRegions = regions
}

// SetMonitoredServices sets the services to monitor
func (n *NagiosCheck) SetMonitoredServices(services []string) {
	n.MonitoredServices = services
}

// SetWarningThreshold sets the warning threshold
func (n *NagiosCheck) SetWarningThreshold(threshold int) {
	n.WarningThreshold = threshold
}

// SetCriticalThreshold sets the critical threshold
func (n *NagiosCheck) SetCriticalThreshold(threshold int) {
	n.CriticalThreshold = threshold
}
