package web

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"

	"rolewalkers/aws"
	"rolewalkers/internal/db"
)

type Server struct {
	port         int
	dbRepo       *db.ConfigRepository
	roleSwitcher *aws.RoleSwitcher
	ssoManager   *aws.SSOManager
	kubeManager  *aws.KubeManager
}

type AccountResponse struct {
	ID          int    `json:"id"`
	AccountID   string `json:"account_id"`
	AccountName string `json:"account_name"`
	Description string `json:"description"`
}

type RoleResponse struct {
	ID          int    `json:"id"`
	AccountID   int    `json:"account_id"`
	RoleName    string `json:"role_name"`
	ProfileName string `json:"profile_name"`
	Region      string `json:"region"`
	Description string `json:"description"`
}

type SessionResponse struct {
	Active         bool              `json:"active"`
	Role           *RoleResponse     `json:"role,omitempty"`
	Account        *AccountResponse  `json:"account,omitempty"`
	KubeContext    string            `json:"kube_context,omitempty"`
	KubeNamespace  string            `json:"kube_namespace,omitempty"`
}

type ErrorResponse struct {
	Error string `json:"error"`
}

func NewServer(port int, dbRepo *db.ConfigRepository, roleSwitcher *aws.RoleSwitcher) *Server {
	ssoManager, _ := aws.NewSSOManager()
	kubeManager := aws.NewKubeManager()
	return &Server{
		port:         port,
		dbRepo:       dbRepo,
		roleSwitcher: roleSwitcher,
		ssoManager:   ssoManager,
		kubeManager:  kubeManager,
	}
}

func (s *Server) Start() error {
	mux := http.NewServeMux()

	// API endpoints
	mux.HandleFunc("GET /api/accounts", s.handleGetAccounts)
	mux.HandleFunc("POST /api/accounts", s.handleAddAccount)
	mux.HandleFunc("GET /api/accounts/{id}/roles", s.handleGetRoles)
	mux.HandleFunc("POST /api/roles", s.handleAddRole)
	mux.HandleFunc("GET /api/session/active", s.handleGetActiveSession)
	mux.HandleFunc("POST /api/session/switch", s.handleSwitchSession)
	mux.HandleFunc("GET /api/session/login-status/{profileName}", s.handleGetLoginStatus)
	mux.HandleFunc("POST /api/session/login", s.handleLoginRole)
	mux.HandleFunc("GET /api/config/import", s.handleImportConfig)
	mux.HandleFunc("POST /api/config/import", s.handleImportConfig)

	// Serve static files
	webDir := s.getWebDir()
	
	if webDir == "embedded" {
		// Embedded filesystem not available, serve from disk
		webDir = "web"
	}
	
	// Use disk filesystem
	mux.HandleFunc("GET /", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" {
			http.ServeFile(w, r, filepath.Join(webDir, "index.html"))
			return
		}
		http.FileServer(http.Dir(webDir)).ServeHTTP(w, r)
	})

	addr := fmt.Sprintf("localhost:%d", s.port)
	fmt.Printf("Starting web server on http://%s\n", addr)

	// Open browser
	s.openBrowser(fmt.Sprintf("http://%s", addr))

	return http.ListenAndServe(addr, mux)
}

func (s *Server) getWebDir() string {
	// Try current directory first
	if _, err := os.Stat("web"); err == nil {
		return "web"
	}
	// Try relative to executable
	ex, err := os.Executable()
	if err == nil {
		dir := filepath.Dir(ex)
		if _, err := os.Stat(filepath.Join(dir, "web")); err == nil {
			return filepath.Join(dir, "web")
		}
	}
	// Use embedded filesystem as fallback
	return "embedded"
}

func (s *Server) openBrowser(url string) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	case "linux":
		cmd = exec.Command("xdg-open", url)
	case "windows":
		cmd = exec.Command("cmd", "/c", "start", url)
	}
	if cmd != nil {
		cmd.Run()
	}
}

func (s *Server) handleGetAccounts(w http.ResponseWriter, r *http.Request) {
	accounts, err := s.dbRepo.GetAllAWSAccounts()
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	response := make([]AccountResponse, len(accounts))
	for i, acc := range accounts {
		response[i] = AccountResponse{
			ID:          acc.ID,
			AccountID:   acc.AccountID,
			AccountName: acc.AccountName,
			Description: acc.Description.String,
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

func (s *Server) handleAddAccount(w http.ResponseWriter, r *http.Request) {
	var req struct {
		AccountID   string `json:"account_id"`
		AccountName string `json:"account_name"`
		SSOStartURL string `json:"sso_start_url"`
		SSORegion   string `json:"sso_region"`
		Description string `json:"description"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.writeError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	if err := s.dbRepo.AddAWSAccount(req.AccountID, req.AccountName, req.SSOStartURL, req.SSORegion, req.Description); err != nil {
		s.writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]string{"status": "created"})
}

func (s *Server) handleGetRoles(w http.ResponseWriter, r *http.Request) {
	accountIDStr := r.PathValue("id")
	accountID, err := strconv.Atoi(accountIDStr)
	if err != nil {
		s.writeError(w, http.StatusBadRequest, "Invalid account ID")
		return
	}

	// Get the account to find its AWS account ID
	accounts, err := s.dbRepo.GetAllAWSAccounts()
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	var awsAccountID string
	for _, acc := range accounts {
		if acc.ID == accountID {
			awsAccountID = acc.AccountID
			break
		}
	}

	if awsAccountID == "" {
		s.writeError(w, http.StatusNotFound, "Account not found")
		return
	}

	roles, err := s.dbRepo.GetRolesByAccount(awsAccountID)
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	response := make([]RoleResponse, len(roles))
	for i, role := range roles {
		response[i] = RoleResponse{
			ID:          role.ID,
			AccountID:   role.AccountID,
			RoleName:    role.RoleName,
			ProfileName: role.ProfileName,
			Region:      role.Region,
			Description: role.Description.String,
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

func (s *Server) handleAddRole(w http.ResponseWriter, r *http.Request) {
	var req struct {
		AccountID   int    `json:"account_id"`
		RoleName    string `json:"role_name"`
		RoleARN     string `json:"role_arn"`
		ProfileName string `json:"profile_name"`
		Region      string `json:"region"`
		Description string `json:"description"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.writeError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	if err := s.dbRepo.AddAWSRole(req.AccountID, req.RoleName, req.RoleARN, req.ProfileName, req.Region, req.Description); err != nil {
		s.writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]string{"status": "created"})
}

func (s *Server) handleGetActiveSession(w http.ResponseWriter, r *http.Request) {
	session, role, account, err := s.dbRepo.GetActiveSession()
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(SessionResponse{Active: false})
		return
	}

	if session == nil || role == nil || account == nil {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(SessionResponse{Active: false})
		return
	}

	response := SessionResponse{
		Active: true,
		Role: &RoleResponse{
			ID:          role.ID,
			AccountID:   role.AccountID,
			RoleName:    role.RoleName,
			ProfileName: role.ProfileName,
			Region:      role.Region,
			Description: role.Description.String,
		},
		Account: &AccountResponse{
			ID:          account.ID,
			AccountID:   account.AccountID,
			AccountName: account.AccountName,
			Description: account.Description.String,
		},
	}

	// Try to get Kubernetes context info
	if s.kubeManager != nil {
		if context, err := s.kubeManager.GetCurrentContext(); err == nil {
			response.KubeContext = context
		}
		response.KubeNamespace = s.kubeManager.GetCurrentNamespace()
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

func (s *Server) handleSwitchSession(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ProfileName string `json:"profile_name"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.writeError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	if err := s.roleSwitcher.SwitchRole(req.ProfileName); err != nil {
		s.writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	// Try to switch Kubernetes context if available
	kubeStatus := "skipped"
	if s.kubeManager != nil {
		env := s.extractEnvName(req.ProfileName)
		if err := s.kubeManager.SwitchContextForEnv(env); err == nil {
			kubeStatus = "switched"
		} else {
			kubeStatus = "failed"
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "switched", "kube_status": kubeStatus})
}

func (s *Server) handleLoginRole(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ProfileName string `json:"profile_name"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.writeError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	if s.ssoManager == nil {
		s.writeError(w, http.StatusInternalServerError, "SSO manager not initialized")
		return
	}

	// Validate the profile exists
	if err := s.ssoManager.ValidateProfile(req.ProfileName); err != nil {
		s.writeError(w, http.StatusBadRequest, fmt.Sprintf("Invalid profile: %v", err))
		return
	}

	// Initiate SSO login
	if err := s.ssoManager.Login(req.ProfileName); err != nil {
		s.writeError(w, http.StatusInternalServerError, fmt.Sprintf("SSO login failed: %v", err))
		return
	}

	// After successful login, switch to the role
	if err := s.roleSwitcher.SwitchRole(req.ProfileName); err != nil {
		s.writeError(w, http.StatusInternalServerError, fmt.Sprintf("Failed to switch role after login: %v", err))
		return
	}

	// Try to switch Kubernetes context if available
	kubeStatus := "skipped"
	if s.kubeManager != nil {
		env := s.extractEnvName(req.ProfileName)
		if err := s.kubeManager.SwitchContextForEnv(env); err == nil {
			kubeStatus = "switched"
		} else {
			kubeStatus = "failed"
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "logged_in_and_switched", "kube_status": kubeStatus})
}

func (s *Server) handleGetLoginStatus(w http.ResponseWriter, r *http.Request) {
	profileName := r.PathValue("profileName")
	
	if s.ssoManager == nil {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"profile_name": profileName,
			"logged_in":    false,
		})
		return
	}
	
	isLoggedIn := s.ssoManager.IsLoggedIn(profileName)
	
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"profile_name": profileName,
		"logged_in":    isLoggedIn,
	})
}

func (s *Server) handleImportConfig(w http.ResponseWriter, r *http.Request) {
	if r.Method == "GET" {
		s.previewImportConfig(w, r)
	} else if r.Method == "POST" {
		s.executeImportConfig(w, r)
	}
}

func (s *Server) previewImportConfig(w http.ResponseWriter, r *http.Request) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, "Failed to get home directory")
		return
	}

	configPath := filepath.Join(homeDir, ".aws", "config")
	content, err := os.ReadFile(configPath)
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, "Failed to read AWS config")
		return
	}

	profiles := s.parseAWSConfig(string(content))

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"profiles": profiles,
	})
}

func (s *Server) executeImportConfig(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Profiles []map[string]string `json:"profiles"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.writeError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	imported := 0
	skipped := 0
	errors := []string{}

	for _, profile := range req.Profiles {
		profileName := profile["name"]
		if profileName == "" || profileName == "default" {
			continue
		}

		// Extract account ID from sso_account_id, role_arn, or profile name
		accountID := profile["sso_account_id"]
		
		if accountID == "" {
			// Try to extract from role ARN
			roleARN := profile["role_arn"]
			if roleARN != "" {
				parts := strings.Split(roleARN, ":")
				if len(parts) >= 5 {
					accountID = parts[4]
				}
			}
		}

		if accountID == "" {
			errors = append(errors, fmt.Sprintf("Profile %s: no account ID found in config", profileName))
			continue
		}

		// Get or create account
		account, err := s.dbRepo.GetAWSAccount(accountID)
		if err != nil || account == nil {
			// Account doesn't exist, create it
			// Extract environment name from profile (e.g., "prod-admin" -> "prod")
			envName := s.extractEnvName(profileName)
			
			ssoStartURL := profile["sso_start_url"]
			ssoRegion := profile["sso_region"]
			if ssoRegion == "" {
				ssoRegion = "eu-west-2"
			}
			
			if err := s.dbRepo.AddAWSAccount(accountID, envName, ssoStartURL, ssoRegion, "Imported from AWS config"); err != nil {
				errors = append(errors, fmt.Sprintf("Profile %s: failed to create account - %v", profileName, err))
				continue
			}
			
			// Fetch the newly created account
			account, err = s.dbRepo.GetAWSAccount(accountID)
			if err != nil || account == nil {
				errors = append(errors, fmt.Sprintf("Profile %s: account created but could not be retrieved", profileName))
				continue
			}
		}

		// Extract role name from sso_role_name or role ARN
		roleName := profile["sso_role_name"]
		if roleName == "" {
			roleARN := profile["role_arn"]
			if roleARN != "" {
				parts := strings.Split(roleARN, "/")
				if len(parts) > 0 {
					roleName = parts[len(parts)-1]
				}
			}
		}
		if roleName == "" {
			roleName = "Role"
		}

		region := profile["region"]
		if region == "" {
			region = "eu-west-2"
		}

		roleARN := profile["role_arn"]

		// Check if role already exists
		existingRole, _ := s.dbRepo.GetRoleByProfileName(profileName)
		if existingRole != nil {
			skipped++
			continue
		}

		// Create role
		if err := s.dbRepo.AddAWSRole(account.ID, roleName, roleARN, profileName, region, "Imported from AWS config"); err != nil {
			// Check if it's a duplicate role name error
			if strings.Contains(err.Error(), "UNIQUE constraint failed") {
				skipped++
				continue
			}
			errors = append(errors, fmt.Sprintf("Profile %s: failed to create role - %v", profileName, err))
			continue
		}

		imported++
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"imported": imported,
		"skipped":  skipped,
		"errors":  errors,
	})
}

func (s *Server) extractEnvName(profileName string) string {
	// Use the full profile name as the environment name
	// e.g., "zenith-sandbox" stays "zenith-sandbox", "zenith-qa" stays "zenith-qa"
	return profileName
}

func (s *Server) parseAWSConfig(content string) []map[string]string {
	var profiles []map[string]string
	lines := strings.Split(content, "\n")
	var currentProfile map[string]string

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			if currentProfile != nil {
				profiles = append(profiles, currentProfile)
			}
			profileName := strings.TrimPrefix(strings.TrimSuffix(line, "]"), "[")
			
			// Skip sso-session sections
			if strings.HasPrefix(profileName, "sso-session") {
				currentProfile = nil
				continue
			}
			
			// Remove "profile " prefix if present
			profileName = strings.TrimPrefix(profileName, "profile ")
			currentProfile = map[string]string{"name": profileName}
			continue
		}

		if currentProfile != nil && strings.Contains(line, "=") {
			parts := strings.SplitN(line, "=", 2)
			key := strings.TrimSpace(parts[0])
			value := strings.TrimSpace(parts[1])
			currentProfile[key] = value
		}
	}

	if currentProfile != nil {
		profiles = append(profiles, currentProfile)
	}

	return profiles
}

func (s *Server) writeError(w http.ResponseWriter, statusCode int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	json.NewEncoder(w).Encode(ErrorResponse{Error: message})
}
