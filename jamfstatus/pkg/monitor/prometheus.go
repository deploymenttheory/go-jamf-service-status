package monitor

import (
	"net/http"
	"time"

	"github.com/deploymenttheory/go-jamf-service-status/jamfstatus/pkg/core"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// PrometheusExporter handles exporting metrics to Prometheus
type PrometheusExporter struct {
	// Metrics
	activeOutages  *prometheus.GaugeVec
	serviceStatus  *prometheus.GaugeVec
	checkDuration  prometheus.Histogram
	uptime         *prometheus.GaugeVec
	lastCheck      prometheus.Gauge
	totalIncidents prometheus.Counter

	registry *prometheus.Registry
	server   *http.Server
}

// NewPrometheusExporter creates a new Prometheus exporter
func NewPrometheusExporter() *PrometheusExporter {
	// Create a new registry
	registry := prometheus.NewRegistry()

	// Define metrics
	activeOutages := prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "jamf_status_active_outages",
			Help: "Number of active outages by region and impact",
		},
		[]string{"region", "impact"},
	)

	serviceStatus := prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "jamf_status_service",
			Help: "Status of Jamf services (0=operational, 1=degraded, 2=outage, 3=maintenance)",
		},
		[]string{"service", "region"},
	)

	checkDuration := prometheus.NewHistogram(
		prometheus.HistogramOpts{
			Name:    "jamf_status_check_duration_seconds",
			Help:    "Duration of status checks in seconds",
			Buckets: prometheus.DefBuckets,
		},
	)

	uptime := prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "jamf_status_uptime_percent",
			Help: "Historical uptime percentage by region",
		},
		[]string{"region"},
	)

	lastCheck := prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "jamf_status_last_check_timestamp",
			Help: "Timestamp of the last status check",
		},
	)

	totalIncidents := prometheus.NewCounter(
		prometheus.CounterOpts{
			Name: "jamf_status_total_incidents",
			Help: "Total number of incidents detected",
		},
	)

	// Register metrics
	registry.MustRegister(activeOutages)
	registry.MustRegister(serviceStatus)
	registry.MustRegister(checkDuration)
	registry.MustRegister(uptime)
	registry.MustRegister(lastCheck)
	registry.MustRegister(totalIncidents)

	return &PrometheusExporter{
		activeOutages:  activeOutages,
		serviceStatus:  serviceStatus,
		checkDuration:  checkDuration,
		uptime:         uptime,
		lastCheck:      lastCheck,
		totalIncidents: totalIncidents,
		registry:       registry,
	}
}

// Start starts the Prometheus HTTP server
func (e *PrometheusExporter) Start(addr string, path string) error {
	// Create handler
	handler := promhttp.HandlerFor(e.registry, promhttp.HandlerOpts{})

	// Create mux
	mux := http.NewServeMux()
	mux.Handle(path, handler)

	// Create server
	e.server = &http.Server{
		Addr:    addr,
		Handler: mux,
	}

	// Start server in a separate goroutine
	go func() {
		if err := e.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			panic(err)
		}
	}()

	return nil
}

// Stop stops the Prometheus HTTP server
func (e *PrometheusExporter) Stop() error {
	if e.server != nil {
		return e.server.Close()
	}
	return nil
}

// UpdateMetrics updates the Prometheus metrics based on the status report
func (e *PrometheusExporter) UpdateMetrics(report *core.StatusReport, duration time.Duration) {
	// Set check duration
	e.checkDuration.Observe(duration.Seconds())

	// Set last check timestamp
	e.lastCheck.Set(float64(time.Now().Unix()))

	// Update active outages
	e.activeOutages.Reset()
	for _, incident := range report.ActiveOutages {
		for _, region := range incident.AffectedRegions {
			e.activeOutages.WithLabelValues(region, incident.Impact).Inc()
		}
		// Increment total incidents
		e.totalIncidents.Inc()
	}

	// Update service status
	e.serviceStatus.Reset()
	for _, service := range report.Services {
		statusValue := 0.0 // Default to operational

		switch service.Status {
		case "degraded":
			statusValue = 1.0
		case "outage":
			statusValue = 2.0
		case "maintenance":
			statusValue = 3.0
		}

		for _, region := range service.Regions {
			e.serviceStatus.WithLabelValues(service.Name, region).Set(statusValue)
		}
	}

	// Update uptime
	for region, value := range report.HistoricalUptime {
		e.uptime.WithLabelValues(region).Set(value)
	}
}

// GetRegistry returns the Prometheus registry
func (e *PrometheusExporter) GetRegistry() *prometheus.Registry {
	return e.registry
}
