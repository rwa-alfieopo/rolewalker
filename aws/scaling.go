package aws

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"rolewalkers/internal/db"
	"strings"
)

// ScalingManager handles HPA scaling operations
type ScalingManager struct {
	kubeManager     *KubeManager
	profileSwitcher *ProfileSwitcher
	configRepo      *db.ConfigRepository
	namespace       string
}

// ScalingPreset defines min/max replicas for a preset
type ScalingPreset struct {
	Min int
	Max int
}

// HPAInfo represents HPA metadata from kubectl
type HPAInfo struct {
	Metadata struct {
		Name string `json:"name"`
	} `json:"metadata"`
	Spec struct {
		MinReplicas int `json:"minReplicas"`
		MaxReplicas int `json:"maxReplicas"`
	} `json:"spec"`
}

// HPAList represents the kubectl get hpa output
type HPAList struct {
	Items []HPAInfo `json:"items"`
}

// NewScalingManager creates a new ScalingManager instance
func NewScalingManager() *ScalingManager {
	ps, err := NewProfileSwitcher()
	if err != nil {
		fmt.Fprintf(os.Stderr, "⚠ Profile switcher init failed: %v\n", err)
	}
	database, err := db.NewDB()
	var repo *db.ConfigRepository
	if err != nil {
		fmt.Fprintf(os.Stderr, "⚠ Database init failed: %v\n", err)
	} else {
		repo = db.NewConfigRepository(database)
	}
	return &ScalingManager{
		kubeManager:     NewKubeManager(),
		profileSwitcher: ps,
		configRepo:      repo,
		namespace:       "zenith",
	}
}

// ValidEnvironments returns the list of valid environments
func (sm *ScalingManager) ValidEnvironments() []string {
	if sm.configRepo != nil {
		envs, err := sm.configRepo.GetAllEnvironments()
		if err == nil {
			names := make([]string, len(envs))
			for i, e := range envs {
				names[i] = e.Name
			}
			return names
		}
	}
	return []string{"snd", "dev", "sit", "preprod", "trg", "prod", "qa", "stage"}
}

// ValidPresets returns the list of valid preset names
func (sm *ScalingManager) ValidPresets() []string {
	if sm.configRepo != nil {
		presets, err := sm.configRepo.GetAllScalingPresets()
		if err == nil {
			names := make([]string, len(presets))
			for i, p := range presets {
				names[i] = p.Name
			}
			return names
		}
	}
	return []string{"normal", "performance", "minimal"}
}

// Scale applies a preset to all HPAs in the environment
func (sm *ScalingManager) Scale(env, presetName string) error {
	var preset ScalingPreset
	
	if sm.configRepo != nil {
		dbPreset, err := sm.configRepo.GetScalingPreset(presetName)
		if err == nil {
			preset = ScalingPreset{Min: dbPreset.MinReplicas, Max: dbPreset.MaxReplicas}
		} else {
			return fmt.Errorf("invalid preset: %s (valid: %s)", presetName, strings.Join(sm.ValidPresets(), ", "))
		}
	} else {
		// Fallback to hardcoded presets
		presets := map[string]ScalingPreset{
			"normal":      {Min: 2, Max: 10},
			"performance": {Min: 10, Max: 50},
			"minimal":     {Min: 1, Max: 3},
		}
		var ok bool
		preset, ok = presets[presetName]
		if !ok {
			return fmt.Errorf("invalid preset: %s (valid: %s)", presetName, strings.Join(sm.ValidPresets(), ", "))
		}
	}

	if !sm.isValidEnv(env) {
		return fmt.Errorf("invalid environment: %s (valid: %s)", env, strings.Join(sm.ValidEnvironments(), ", "))
	}

	// Switch to correct kubectl context
	if err := sm.kubeManager.SwitchContextForEnvWithProfile(env, sm.profileSwitcher); err != nil {
		return fmt.Errorf("failed to switch kubectl context: %w", err)
	}

	ctx, _ := sm.kubeManager.GetCurrentContext()
	fmt.Printf("Using kubectl context: %s\n", ctx)

	// Get all HPAs
	hpas, err := sm.listHPAs()
	if err != nil {
		return fmt.Errorf("failed to list HPAs: %w", err)
	}

	if len(hpas) == 0 {
		return fmt.Errorf("no HPAs found in namespace %s", sm.namespace)
	}

	fmt.Printf("Scaling %d HPAs to preset '%s' (min=%d, max=%d)...\n", len(hpas), presetName, preset.Min, preset.Max)

	// Patch each HPA
	var errors []string
	for _, hpa := range hpas {
		if err := sm.patchHPA(hpa.Metadata.Name, preset.Min, preset.Max); err != nil {
			errors = append(errors, fmt.Sprintf("%s: %v", hpa.Metadata.Name, err))
		} else {
			fmt.Printf("  ✓ %s\n", hpa.Metadata.Name)
		}
	}

	if len(errors) > 0 {
		return fmt.Errorf("some HPAs failed to scale:\n  %s", strings.Join(errors, "\n  "))
	}

	fmt.Printf("\n✓ Successfully scaled all HPAs to '%s' preset\n", presetName)
	return nil
}

// ScaleService scales a specific service's HPA
func (sm *ScalingManager) ScaleService(env, service string, min, max int) error {
	if !sm.isValidEnv(env) {
		return fmt.Errorf("invalid environment: %s (valid: %s)", env, strings.Join(sm.ValidEnvironments(), ", "))
	}

	if min < 0 || max < 0 {
		return fmt.Errorf("min and max must be non-negative")
	}

	if min > max {
		return fmt.Errorf("min (%d) cannot be greater than max (%d)", min, max)
	}

	// Switch to correct kubectl context
	if err := sm.kubeManager.SwitchContextForEnvWithProfile(env, sm.profileSwitcher); err != nil {
		return fmt.Errorf("failed to switch kubectl context: %w", err)
	}

	ctx, _ := sm.kubeManager.GetCurrentContext()
	fmt.Printf("Using kubectl context: %s\n", ctx)

	// Build HPA name from service name
	hpaName := sm.buildHPAName(service)

	// Verify HPA exists
	if !sm.hpaExists(hpaName) {
		return fmt.Errorf("HPA '%s' not found in namespace %s", hpaName, sm.namespace)
	}

	// Patch the HPA
	if err := sm.patchHPA(hpaName, min, max); err != nil {
		return fmt.Errorf("failed to scale %s: %w", hpaName, err)
	}

	fmt.Printf("✓ Scaled %s to min=%d, max=%d\n", hpaName, min, max)
	return nil
}

// ListHPAs returns formatted list of HPAs and their current scaling
func (sm *ScalingManager) ListHPAs(env string) (string, error) {
	if !sm.isValidEnv(env) {
		return "", fmt.Errorf("invalid environment: %s (valid: %s)", env, strings.Join(sm.ValidEnvironments(), ", "))
	}

	// Switch to correct kubectl context
	if err := sm.kubeManager.SwitchContextForEnvWithProfile(env, sm.profileSwitcher); err != nil {
		return "", fmt.Errorf("failed to switch kubectl context: %w", err)
	}

	hpas, err := sm.listHPAs()
	if err != nil {
		return "", err
	}

	if len(hpas) == 0 {
		return fmt.Sprintf("No HPAs found in namespace %s", sm.namespace), nil
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("HPAs in %s namespace:\n", sm.namespace))
	sb.WriteString(strings.Repeat("-", 60) + "\n")
	sb.WriteString(fmt.Sprintf("%-40s %s\n", "NAME", "MIN/MAX"))
	sb.WriteString(strings.Repeat("-", 60) + "\n")

	for _, hpa := range hpas {
		sb.WriteString(fmt.Sprintf("%-40s %d/%d\n", hpa.Metadata.Name, hpa.Spec.MinReplicas, hpa.Spec.MaxReplicas))
	}

	return sb.String(), nil
}

func (sm *ScalingManager) listHPAs() ([]HPAInfo, error) {
	cmd := exec.Command("kubectl", "get", "hpa", "-n", sm.namespace, "-o", "json")
	var out bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("kubectl error: %s", stderr.String())
	}

	var hpaList HPAList
	if err := json.Unmarshal(out.Bytes(), &hpaList); err != nil {
		return nil, fmt.Errorf("failed to parse HPA list: %w", err)
	}

	return hpaList.Items, nil
}

func (sm *ScalingManager) patchHPA(name string, min, max int) error {
	patch := fmt.Sprintf(`{"spec":{"minReplicas":%d,"maxReplicas":%d}}`, min, max)

	cmd := exec.Command("kubectl", "patch", "hpa", name, "-n", sm.namespace, "--type=merge", "-p", patch)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%s", stderr.String())
	}

	return nil
}

func (sm *ScalingManager) hpaExists(name string) bool {
	cmd := exec.Command("kubectl", "get", "hpa", name, "-n", sm.namespace)
	return cmd.Run() == nil
}

func (sm *ScalingManager) buildHPAName(service string) string {
	// If already has -hpa suffix, use as-is
	if strings.HasSuffix(service, "-hpa") {
		return service
	}
	// If already has -microservice suffix, just add -hpa
	if strings.HasSuffix(service, "-microservice") {
		return service + "-hpa"
	}
	// Otherwise, build full name: <service>-microservice-hpa
	return service + "-microservice-hpa"
}

func (sm *ScalingManager) isValidEnv(env string) bool {
	for _, e := range sm.ValidEnvironments() {
		if e == env {
			return true
		}
	}
	return false
}
