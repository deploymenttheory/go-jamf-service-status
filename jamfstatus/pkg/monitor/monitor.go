package monitor

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/deploymenttheory/go-jamf-service-status/jamfstatus/pkg/alert"
	"github.com/deploymenttheory/go-jamf-service-status/jamfstatus/pkg/client"
	"github.com/deploymenttheory/go-jamf-service-status/jamfstatus/pkg/core"
	"github.com/deploymenttheory/go-jamf-service-status/jamfstatus/pkg/healthcheck"
)

// Monitor represents a status monitoring service
type Monitor struct {
	client          *client.Client
	checkInterval   time.Duration
	alertManager    *alert.AlertManager
	stopChan        chan struct{}
	wg              sync.WaitGroup
	mu              sync.Mutex
	lastStatus      *core.StatusReport
	metricsExporter *PrometheusExporter
	metricsAddr     string
	metricsPath     string
	healthChecker   *healthcheck.ServiceHealthChecker
	verboseLogging  bool
	outageCallback  func([]core.StatusIncident)
	lastError       error
}

// MonitorOption is a functional option for configuring the monitor
type MonitorOption func(*Monitor)

// WithCheckInterval sets the check interval
func WithCheckInterval(interval time.Duration) MonitorOption {
	return func(m *Monitor) {
		m.checkInterval = interval
	}
}

// WithAlertManager sets the alert manager
func WithAlertManager(mgr *alert.AlertManager) MonitorOption {
	return func(m *Monitor) {
		m.alertManager = mgr
	}
}

// WithMetricsEndpoint configures Prometheus metrics
func WithMetricsEndpoint(addr, path string) MonitorOption {
	return func(m *Monitor) {
		m.metricsAddr = addr
		m.metricsPath = path
	}
}

// WithHealthChecker sets the health checker
func WithHealthChecker(checker *healthcheck.ServiceHealthChecker) MonitorOption {
	return func(m *Monitor) {
		m.healthChecker = checker
	}
}

// WithVerboseLogging enables verbose logging
func WithVerboseLogging(verbose bool) MonitorOption {
	return func(m *Monitor) {
		m.verboseLogging = verbose
	}
}

// WithClient sets a custom client
func WithClient(client *client.Client) MonitorOption {
	return func(m *Monitor) {
		m.client = client
	}
}

// WithOutageCallback sets a callback to be called when outages are detected
func WithOutageCallback(callback func([]core.StatusIncident)) MonitorOption {
	return func(m *Monitor) {
		m.outageCallback = callback
	}
}

// NewMonitor creates a new status monitor
func NewMonitor(options ...MonitorOption) *Monitor {
	m := &Monitor{
		client:         client.NewClient(),
		checkInterval:  5 * time.Minute,
		stopChan:       make(chan struct{}),
		metricsAddr:    ":9090",
		metricsPath:    "/metrics",
		healthChecker:  healthcheck.NewServiceHealthChecker(),
		verboseLogging: false,
	}

	for _, opt := range options {
		opt(m)
	}

	return m
}

// Start begins the monitoring process
func (m *Monitor) Start() {
	m.wg.Add(1)

	// Start metrics server if configured
	if m.metricsAddr != "" {
		m.setupMetrics()
	}

	// Main monitoring goroutine
	go func() {
		defer m.wg.Done()

		// Initial check
		m.performCheck()

		ticker := time.NewTicker(m.checkInterval)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				m.performCheck()
			case <-m.stopChan:
				return
			}
		}
	}()

	// Set up signal handling
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-sigChan
		fmt.Println("\nShutting down monitor...")
		m.Stop()
	}()
}

// Stop stops the monitoring process
func (m *Monitor) Stop() {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Only signal once
	select {
	case <-m.stopChan:
		// Already stopped
	default:
		close(m.stopChan)
	}

	// Stop metrics server if running
	if m.metricsExporter != nil {
		m.metricsExporter.Stop()
	}
}

// Wait blocks until monitoring is stopped
func (m *Monitor) Wait() {
	m.wg.Wait()
}

// GetLastStatus returns the last checked status
func (m *Monitor) GetLastStatus() *core.StatusReport {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.lastStatus
}

// GetLastError returns the last error encountered during a check
func (m *Monitor) GetLastError() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.lastError
}

// PerformImmediateCheck performs a status check immediately
func (m *Monitor) PerformImmediateCheck() *core.StatusReport {
	return m.performCheck()
}

// GetHealthStatus returns the current health status
func (m *Monitor) GetHealthStatus() *core.HealthStatus {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.healthChecker != nil {
		return m.healthChecker.GetHealthStatus(m.lastStatus)
	}

	// Create a simple health status if no health checker is configured
	overallHealth := "healthy"
	if m.lastStatus != nil && m.lastStatus.HasActiveOutages {
		overallHealth = "degraded"
	}

	return &core.HealthStatus{
		JamfStatus:    m.lastStatus,
		LocalChecks:   make(map[string]bool),
		Connectivity:  map[string]string{"status_feed": "ok"},
		OverallHealth: overallHealth,
	}
}

// performCheck performs a status check and processes results
func (m *Monitor) performCheck() *core.StatusReport {
	// Create context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Track start time for metrics
	startTime := time.Now()

	// Get status report
	report, err := m.client.GetStatusReport(ctx, 1) // Just check recent incidents

	// Calculate check duration
	checkDuration := time.Since(startTime)

	// Store error
	m.mu.Lock()
	m.lastError = err
	m.mu.Unlock()

	if err != nil {
		fmt.Fprintf(os.Stderr, "Error checking status: %v\n", err)
		return nil
	}

	// Store the status
	m.mu.Lock()
	prevStatus := m.lastStatus
	m.lastStatus = report
	m.mu.Unlock()

	// Display status change if any
	if statusChanged(prevStatus, report) {
		if m.verboseLogging {
			displayStatusChange(prevStatus, report)
		}

		// Call outage callback if configured
		if m.outageCallback != nil && report.HasActiveOutages {
			m.outageCallback(report.ActiveOutages)
		}
	}

	// Log check
	if m.verboseLogging {
		timeStr := time.Now().Format("2006-01-02 15:04:05")
		outageStr := "No outages"
		if report.HasActiveOutages {
			outageStr = fmt.Sprintf("%d active outage(s)", len(report.ActiveOutages))
		}
		fmt.Printf("[%s] Status check: %s\n", timeStr, outageStr)
	}

	// Send alerts if configured
	if m.alertManager != nil && report.HasActiveOutages {
		if err := m.alertManager.ProcessReport(report); err != nil {
			fmt.Fprintf(os.Stderr, "Error sending alerts: %v\n", err)
		}
	}

	// Update metrics if enabled
	if m.metricsExporter != nil {
		m.metricsExporter.UpdateMetrics(report, checkDuration)
	}

	return report
}

// setupMetrics initializes and starts the metrics server
func (m *Monitor) setupMetrics() {
	m.metricsExporter = NewPrometheusExporter()
	if err := m.metricsExporter.Start(m.metricsAddr, m.metricsPath); err != nil {
		fmt.Fprintf(os.Stderr, "Error starting metrics server: %v\n", err)
	}
}

// Helper functions
func statusChanged(prev, curr *core.StatusReport) bool {
	if prev == nil {
		return curr.HasActiveOutages
	}

	// Check if outage status changed
	if prev.HasActiveOutages != curr.HasActiveOutages {
		return true
	}

	// Check if count of active outages changed
	if len(prev.ActiveOutages) != len(curr.ActiveOutages) {
		return true
	}

	// More detailed comparison - check if outage IDs changed
	prevIDs := make(map[string]bool)
	for _, outage := range prev.ActiveOutages {
		prevIDs[outage.ID] = true
	}

	for _, outage := range curr.ActiveOutages {
		if !prevIDs[outage.ID] {
			return true // Found a new outage
		}
	}

	return false
}

func displayStatusChange(prev, curr *core.StatusReport) {
	fmt.Println("\n--- STATUS CHANGE DETECTED ---")

	if prev == nil {
		if curr.HasActiveOutages {
			fmt.Printf("NEW OUTAGE: %d active outage(s) detected\n", len(curr.ActiveOutages))
		} else {
			fmt.Println("INITIAL STATUS: No active outages")
		}
		return
	}

	if !prev.HasActiveOutages && curr.HasActiveOutages {
		fmt.Printf("NEW OUTAGE: %d active outage(s) detected\n", len(curr.ActiveOutages))
		for i, incident := range curr.ActiveOutages {
			fmt.Printf("  %d. %s\n", i+1, incident.Title)
		}
	} else if prev.HasActiveOutages && !curr.HasActiveOutages {
		fmt.Println("RESOLVED: All outages have been resolved")
	} else if len(prev.ActiveOutages) < len(curr.ActiveOutages) {
		fmt.Printf("ADDITIONAL OUTAGE: Now %d active outages\n", len(curr.ActiveOutages))
	} else if len(prev.ActiveOutages) > len(curr.ActiveOutages) {
		fmt.Printf("PARTIAL RESOLUTION: Now %d active outages\n", len(curr.ActiveOutages))
	}

	fmt.Println("---------------------------")
}
