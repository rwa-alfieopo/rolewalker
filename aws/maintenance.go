package aws

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"rolewalkers/internal/db"
	"strings"
	"time"
)

// MaintenanceManager handles Fastly maintenance mode operations
type MaintenanceManager struct {
	apiToken   string
	baseURL    string
	httpClient *http.Client
	configRepo *db.ConfigRepository
}

// MaintenanceStatus represents the current maintenance state
type MaintenanceStatus struct {
	Environment string `json:"environment"`
	ServiceType string `json:"serviceType"`
	ServiceName string `json:"serviceName"`
	Enabled     bool   `json:"enabled"`
	Error       string `json:"error,omitempty"`
}

// fastlyService represents a Fastly service from the API
type fastlyService struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// fastlyServiceDetail represents detailed service info
type fastlyServiceDetail struct {
	Versions []struct {
		Number int  `json:"number"`
		Active bool `json:"active"`
	} `json:"versions"`
}

// fastlyDictionary represents a Fastly dictionary
type fastlyDictionary struct {
	ID string `json:"id"`
}

// fastlyDictionaryItem represents a dictionary item response
type fastlyDictionaryItem struct {
	ItemValue string `json:"item_value"`
}

// NewMaintenanceManager creates a new maintenance manager
func NewMaintenanceManager() *MaintenanceManager {
	database, err := db.NewDB()
	var repo *db.ConfigRepository
	var baseURL string
	
	if err == nil {
		repo = db.NewConfigRepository(database)
		endpoint, err := repo.GetAPIEndpoint("fastly")
		if err == nil {
			baseURL = endpoint.BaseURL
		}
	} else {
		fmt.Fprintf(os.Stderr, "⚠ Database init failed: %v\n", err)
	}
	
	if baseURL == "" {
		baseURL = "https://api.fastly.com"
	}
	
	return &MaintenanceManager{
		apiToken:   os.Getenv("FASTLY_API_TOKEN"),
		baseURL:    baseURL,
		httpClient: &http.Client{Timeout: 30 * time.Second},
		configRepo: repo,
	}
}

// ValidEnvironments returns the list of valid environments
func (mm *MaintenanceManager) ValidEnvironments() []string {
	if mm.configRepo != nil {
		envs, err := mm.configRepo.GetAllEnvironments()
		if err == nil {
			names := make([]string, len(envs))
			for i, e := range envs {
				names[i] = e.Name
			}
			return names
		}
	}
	return []string{"snd", "dev", "sit", "preprod", "trg", "prod"}
}

// ValidServiceTypes returns the list of valid service types
func (mm *MaintenanceManager) ValidServiceTypes() []string {
	return []string{"api", "pwa", "all"}
}

// Toggle enables or disables maintenance mode for a service
func (mm *MaintenanceManager) Toggle(env, serviceType string, enable bool) error {
	if mm.apiToken == "" {
		return fmt.Errorf("FASTLY_API_TOKEN environment variable is not set")
	}

	if !mm.isValidEnv(env) {
		return fmt.Errorf("invalid environment: %s (valid: %s)", env, strings.Join(mm.ValidEnvironments(), ", "))
	}

	if !mm.isValidServiceType(serviceType) {
		return fmt.Errorf("invalid service type: %s (valid: %s)", serviceType, strings.Join(mm.ValidServiceTypes(), ", "))
	}

	if serviceType == "all" {
		if err := mm.toggleService(env, "api", enable); err != nil {
			return err
		}
		return mm.toggleService(env, "pwa", enable)
	}

	return mm.toggleService(env, serviceType, enable)
}

// Status returns the current maintenance status for an environment
func (mm *MaintenanceManager) Status(env string) ([]MaintenanceStatus, error) {
	if mm.apiToken == "" {
		return nil, fmt.Errorf("FASTLY_API_TOKEN environment variable is not set")
	}

	if !mm.isValidEnv(env) {
		return nil, fmt.Errorf("invalid environment: %s (valid: %s)", env, strings.Join(mm.ValidEnvironments(), ", "))
	}

	var statuses []MaintenanceStatus

	for _, svcType := range []string{"api", "pwa"} {
		status := MaintenanceStatus{
			Environment: env,
			ServiceType: svcType,
		}

		enabled, serviceName, err := mm.getMaintenanceStatus(env, svcType)
		if err != nil {
			status.Error = err.Error()
		} else {
			status.Enabled = enabled
			status.ServiceName = serviceName
		}

		statuses = append(statuses, status)
	}

	return statuses, nil
}

func (mm *MaintenanceManager) toggleService(env, serviceType string, enable bool) error {
	// Find service by name pattern
	serviceName, err := mm.findServiceName(env, serviceType)
	if err != nil {
		return fmt.Errorf("failed to find %s service for %s: %w", serviceType, env, err)
	}

	// Get service ID
	serviceID, err := mm.getServiceID(serviceName)
	if err != nil {
		return fmt.Errorf("failed to get service ID: %w", err)
	}

	// Get active version
	activeVersion, err := mm.getActiveVersion(serviceID)
	if err != nil {
		return fmt.Errorf("failed to get active version: %w", err)
	}

	// Get dictionary ID
	dictionaryID, err := mm.getDictionaryID(serviceID, activeVersion)
	if err != nil {
		return fmt.Errorf("failed to get dictionary ID: %w", err)
	}

	// Update maintenance mode
	enableStr := "false"
	if enable {
		enableStr = "true"
	}

	if err := mm.updateMaintenanceMode(serviceID, dictionaryID, enableStr); err != nil {
		return fmt.Errorf("failed to update maintenance mode: %w", err)
	}

	action := "disabled"
	if enable {
		action = "enabled"
	}
	fmt.Printf("✓ Maintenance mode %s for %s %s (%s)\n", action, env, serviceType, serviceName)

	return nil
}

func (mm *MaintenanceManager) getMaintenanceStatus(env, serviceType string) (bool, string, error) {
	serviceName, err := mm.findServiceName(env, serviceType)
	if err != nil {
		return false, "", err
	}

	serviceID, err := mm.getServiceID(serviceName)
	if err != nil {
		return false, serviceName, err
	}

	activeVersion, err := mm.getActiveVersion(serviceID)
	if err != nil {
		return false, serviceName, err
	}

	dictionaryID, err := mm.getDictionaryID(serviceID, activeVersion)
	if err != nil {
		return false, serviceName, err
	}

	value, err := mm.getMaintenanceModeValue(serviceID, dictionaryID)
	if err != nil {
		return false, serviceName, err
	}

	return value == "true", serviceName, nil
}

func (mm *MaintenanceManager) findServiceName(env, serviceType string) (string, error) {
	req, err := http.NewRequestWithContext(context.Background(), "GET", mm.baseURL+"/service", nil)
	if err != nil {
		return "", err
	}
	mm.setHeaders(req)

	resp, err := mm.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 10*1024*1024)) // 10MB max
	if err != nil {
		return "", err
	}

	var services []fastlyService
	if err := json.Unmarshal(body, &services); err != nil {
		return "", err
	}

	// Find service matching pattern: <env>.*<type>
	pattern := strings.ToLower(env)
	typePattern := strings.ToLower(serviceType)

	for _, svc := range services {
		nameLower := strings.ToLower(svc.Name)
		if strings.HasPrefix(nameLower, pattern) && strings.Contains(nameLower, typePattern) {
			return svc.Name, nil
		}
	}

	return "", fmt.Errorf("no service found matching %s %s", env, serviceType)
}

func (mm *MaintenanceManager) getServiceID(serviceName string) (string, error) {
	req, err := http.NewRequestWithContext(context.Background(), "GET", mm.baseURL+"/service/search?name="+url.QueryEscape(serviceName), nil)
	if err != nil {
		return "", err
	}
	mm.setHeaders(req)

	resp, err := mm.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 10*1024*1024)) // 10MB max
	if err != nil {
		return "", err
	}

	var svc fastlyService
	if err := json.Unmarshal(body, &svc); err != nil {
		return "", err
	}

	return svc.ID, nil
}

func (mm *MaintenanceManager) getActiveVersion(serviceID string) (int, error) {
	req, err := http.NewRequestWithContext(context.Background(), "GET", fmt.Sprintf("%s/service/%s", mm.baseURL, serviceID), nil)
	if err != nil {
		return 0, err
	}
	mm.setHeaders(req)

	resp, err := mm.httpClient.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 10*1024*1024)) // 10MB max
	if err != nil {
		return 0, err
	}

	var detail fastlyServiceDetail
	if err := json.Unmarshal(body, &detail); err != nil {
		return 0, err
	}

	for _, v := range detail.Versions {
		if v.Active {
			return v.Number, nil
		}
	}

	return 0, fmt.Errorf("no active version found")
}

func (mm *MaintenanceManager) getDictionaryID(serviceID string, version int) (string, error) {
	req, err := http.NewRequestWithContext(context.Background(), "GET", fmt.Sprintf("%s/service/%s/version/%d/dictionary/MainConfig", mm.baseURL, serviceID, version), nil)
	if err != nil {
		return "", err
	}
	mm.setHeaders(req)

	resp, err := mm.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 10*1024*1024)) // 10MB max
	if err != nil {
		return "", err
	}

	var dict fastlyDictionary
	if err := json.Unmarshal(body, &dict); err != nil {
		return "", err
	}

	return dict.ID, nil
}

func (mm *MaintenanceManager) updateMaintenanceMode(serviceID, dictionaryID, value string) error {
	data := url.Values{}
	data.Set("item_value", value)

	req, err := http.NewRequestWithContext(context.Background(), "PUT",
		fmt.Sprintf("%s/service/%s/dictionary/%s/item/maintenanceMode", mm.baseURL, serviceID, dictionaryID),
		bytes.NewBufferString(data.Encode()))
	if err != nil {
		return err
	}
	mm.setHeaders(req)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := mm.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("API error (%d): %s", resp.StatusCode, string(body))
	}

	return nil
}

func (mm *MaintenanceManager) getMaintenanceModeValue(serviceID, dictionaryID string) (string, error) {
	req, err := http.NewRequestWithContext(context.Background(), "GET",
		fmt.Sprintf("%s/service/%s/dictionary/%s/item/maintenanceMode", mm.baseURL, serviceID, dictionaryID),
		nil)
	if err != nil {
		return "", err
	}
	mm.setHeaders(req)

	resp, err := mm.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 10*1024*1024)) // 10MB max
	if err != nil {
		return "", err
	}

	var item fastlyDictionaryItem
	if err := json.Unmarshal(body, &item); err != nil {
		return "", err
	}

	return item.ItemValue, nil
}

func (mm *MaintenanceManager) setHeaders(req *http.Request) {
	req.Header.Set("Fastly-Key", mm.apiToken)
	req.Header.Set("Accept", "application/json")
}

func (mm *MaintenanceManager) isValidEnv(env string) bool {
	for _, e := range mm.ValidEnvironments() {
		if e == env {
			return true
		}
	}
	return false
}

func (mm *MaintenanceManager) isValidServiceType(t string) bool {
	for _, st := range mm.ValidServiceTypes() {
		if st == t {
			return true
		}
	}
	return false
}
