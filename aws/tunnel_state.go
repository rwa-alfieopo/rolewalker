package aws

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// TunnelInfo represents an active tunnel
type TunnelInfo struct {
	ID          string    `json:"id"`
	Service     string    `json:"service"`
	Environment string    `json:"environment"`
	PodName     string    `json:"pod_name"`
	LocalPort   int       `json:"local_port"`
	RemoteHost  string    `json:"remote_host"`
	RemotePort  int       `json:"remote_port"`
	StartedAt   time.Time `json:"started_at"`
	PID         int       `json:"pid,omitempty"` // port-forward process ID
}

// TunnelState manages the state of active tunnels
type TunnelState struct {
	Tunnels  map[string]*TunnelInfo `json:"tunnels"`
	filePath string
	mu       sync.RWMutex
}

// NewTunnelState creates a new tunnel state manager
func NewTunnelState() (*TunnelState, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("failed to get home directory: %w", err)
	}

	stateDir := filepath.Join(homeDir, ".rolewalkers")
	if err := os.MkdirAll(stateDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create state directory: %w", err)
	}

	ts := &TunnelState{
		Tunnels:  make(map[string]*TunnelInfo),
		filePath: filepath.Join(stateDir, "tunnels.json"),
	}

	// Load existing state
	ts.load()

	return ts, nil
}

// load reads the state from disk
func (ts *TunnelState) load() error {
	ts.mu.Lock()
	defer ts.mu.Unlock()

	data, err := os.ReadFile(ts.filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	return json.Unmarshal(data, ts)
}

// save writes the state to disk
func (ts *TunnelState) save() error {
	data, err := json.MarshalIndent(ts, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(ts.filePath, data, 0644)
}

// Add adds a tunnel to the state
func (ts *TunnelState) Add(tunnel *TunnelInfo) error {
	ts.mu.Lock()
	defer ts.mu.Unlock()

	ts.Tunnels[tunnel.ID] = tunnel
	return ts.save()
}

// Remove removes a tunnel from the state
func (ts *TunnelState) Remove(id string) error {
	ts.mu.Lock()
	defer ts.mu.Unlock()

	delete(ts.Tunnels, id)
	return ts.save()
}

// Get returns a tunnel by ID
func (ts *TunnelState) Get(id string) *TunnelInfo {
	ts.mu.RLock()
	defer ts.mu.RUnlock()

	return ts.Tunnels[id]
}

// GetByServiceEnv returns a tunnel by service and environment
func (ts *TunnelState) GetByServiceEnv(service, env string) *TunnelInfo {
	ts.mu.RLock()
	defer ts.mu.RUnlock()

	id := fmt.Sprintf("%s-%s", service, env)
	return ts.Tunnels[id]
}

// List returns all active tunnels
func (ts *TunnelState) List() []*TunnelInfo {
	ts.mu.RLock()
	defer ts.mu.RUnlock()

	tunnels := make([]*TunnelInfo, 0, len(ts.Tunnels))
	for _, t := range ts.Tunnels {
		tunnels = append(tunnels, t)
	}
	return tunnels
}

// Clear removes all tunnels from state
func (ts *TunnelState) Clear() error {
	ts.mu.Lock()
	defer ts.mu.Unlock()

	ts.Tunnels = make(map[string]*TunnelInfo)
	return ts.save()
}

// GenerateTunnelID creates a unique tunnel ID
func GenerateTunnelID(service, env string) string {
	return fmt.Sprintf("%s-%s", service, env)
}
