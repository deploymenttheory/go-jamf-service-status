package healthcheck

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/deploymenttheory/go-jamf-service-status/jamfstatus/pkg/core"
)

// HealthCheck defines the interface for health checks
type HealthCheck interface {
	// Check performs the health check and returns the result
	Check() bool

	// Name returns the name of the health check
	Name() string

	// Description returns a description of the health check
	Description() string
}

// EndpointCheck checks the availability of an HTTP endpoint
type EndpointCheck struct {
	name        string
	url         string
	timeout     time.Duration
	client      *http.Client
	description string
}

// NewEndpointCheck creates a new endpoint health check
func NewEndpointCheck(name, url string, timeout time.Duration) *EndpointCheck {
	return &EndpointCheck{
		name:        name,
		url:         url,
		timeout:     timeout,
		client:      &http.Client{Timeout: timeout},
		description: fmt.Sprintf("Checks if endpoint %s is accessible", url),
	}
}

// Check performs the endpoint check
func (c *EndpointCheck) Check() bool {
	ctx, cancel := context.WithTimeout(context.Background(), c.timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "GET", c.url, nil)
	if err != nil {
		return false
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()

	return resp.StatusCode >= 200 && resp.StatusCode < 300
}

// Name returns the name of the health check
func (c *EndpointCheck) Name() string {
	return c.name
}

// Description returns a description of the health check
func (c *EndpointCheck) Description() string {
	return c.description
}

// CompositeCheck combines multiple health checks
type CompositeCheck struct {
	name        string
	checks      []HealthCheck
	description string
	strategy    string // "all" or "any"
}

// NewCompositeCheck creates a new composite health check
func NewCompositeCheck(name string, checks []HealthCheck, strategy string) *CompositeCheck {
	description := fmt.Sprintf("Composite check that requires %s underlying checks to pass", strategy)

	return &CompositeCheck{
		name:        name,
		checks:      checks,
		description: description,
		strategy:    strategy,
	}
}

// Check performs all underlying health checks
func (c *CompositeCheck) Check() bool {
	if len(c.checks) == 0 {
		return true
	}

	if c.strategy == "any" {
		// Only need one check to pass
		for _, check := range c.checks {
			if check.Check() {
				return true
			}
		}
		return false
	}

	// Default strategy: all checks must pass
	for _, check := range c.checks {
		if !check.Check() {
			return false
		}
	}
	return true
}

// Name returns the name of the health check
func (c *CompositeCheck) Name() string {
	return c.name
}

// Description returns a description of the health check
func (c *CompositeCheck) Description() string {
	return c.description
}

// FunctionCheck is a health check that calls a function
type FunctionCheck struct {
	name        string
	checkFunc   func() bool
	description string
}

// NewFunctionCheck creates a new function health check
func NewFunctionCheck(name string, description string, checkFunc func() bool) *FunctionCheck {
	return &FunctionCheck{
		name:        name,
		checkFunc:   checkFunc,
		description: description,
	}
}

// Check calls the check function
func (c *FunctionCheck) Check() bool {
	return c.checkFunc()
}

// Name returns the name of the health check
func (c *FunctionCheck) Name() string {
	return c.name
}

// Description returns a description of the health check
func (c *FunctionCheck) Description() string {
	return c.description
}

// ServiceHealthChecker runs a set of health checks alongside the Jamf status
type ServiceHealthChecker struct {
	checks map[string]HealthCheck
}

// NewServiceHealthChecker creates a new service health checker
func NewServiceHealthChecker() *ServiceHealthChecker {
	return &ServiceHealthChecker{
		checks: make(map[string]HealthCheck),
	}
}

// AddCheck adds a health check to the service
func (s *ServiceHealthChecker) AddCheck(check HealthCheck) {
	s.checks[check.Name()] = check
}

// RemoveCheck removes a health check from the service
func (s *ServiceHealthChecker) RemoveCheck(name string) {
	delete(s.checks, name)
}

// RunChecks runs all health checks and returns the results
func (s *ServiceHealthChecker) RunChecks() map[string]bool {
	results := make(map[string]bool)

	for name, check := range s.checks {
		results[name] = check.Check()
	}

	return results
}

// GetHealthStatus creates a comprehensive health status
func (s *ServiceHealthChecker) GetHealthStatus(jamfStatus *core.StatusReport) *core.HealthStatus {
	// Run all checks
	checkResults := s.RunChecks()

	// Determine connectivity status
	connectivity := make(map[string]string)

	// Set default connectivity status for status feed
	connectivity["status_feed"] = "ok"

	// Determine overall health
	overallHealth := "healthy"

	// Check if Jamf has any active outages
	if jamfStatus != nil && jamfStatus.HasActiveOutages {
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
		JamfStatus:    jamfStatus,
		LocalChecks:   checkResults,
		Connectivity:  connectivity,
		OverallHealth: overallHealth,
	}
}

// GetChecks returns all registered health checks
func (s *ServiceHealthChecker) GetChecks() map[string]HealthCheck {
	return s.checks
}
