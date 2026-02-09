package web

import (
	"context"
	crypto_rand "crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"

	"rolewalkers/aws"
	"rolewalkers/internal/db"
	webAssets "rolewalkers/web"
)

// Server is the web UI HTTP server.
type Server struct {
	port         int
	dbRepo       *db.ConfigRepository
	roleSwitcher *aws.RoleSwitcher
	ssoManager   *aws.SSOManager
	kubeManager  *aws.KubeManager
	logger       *slog.Logger
	authToken    string

	// middleware closures (initialised in Start)
	recover  func(http.HandlerFunc) http.HandlerFunc
	logReq   func(http.HandlerFunc) http.HandlerFunc
	authWrap func(http.HandlerFunc) http.HandlerFunc
}

// Response types

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
	Active        bool             `json:"active"`
	Role          *RoleResponse    `json:"role,omitempty"`
	Account       *AccountResponse `json:"account,omitempty"`
	KubeContext   string           `json:"kube_context,omitempty"`
	KubeNamespace string           `json:"kube_namespace,omitempty"`
}

type ErrorResponse struct {
	Error string `json:"error"`
}

func NewServer(port int, dbRepo *db.ConfigRepository, roleSwitcher *aws.RoleSwitcher) *Server {
	cm, _ := aws.NewConfigManager()
	ssoManager, _ := aws.NewSSOManager(cm)
	kubeManager := aws.NewKubeManager()

	tokenBytes := make([]byte, 32)
	if _, err := crypto_rand.Read(tokenBytes); err != nil {
		tokenBytes = []byte(fmt.Sprintf("%d", time.Now().UnixNano()))
	}
	token := fmt.Sprintf("%x", tokenBytes)

	return &Server{
		port:         port,
		dbRepo:       dbRepo,
		roleSwitcher: roleSwitcher,
		ssoManager:   ssoManager,
		kubeManager:  kubeManager,
		logger:       slog.Default(),
		authToken:    token,
	}
}

func (s *Server) Start() error {
	// Initialise middleware closures once
	s.recover = RecoveryMiddleware(s.logger)
	s.logReq = RequestLogger(s.logger)
	s.authWrap = BearerAuth(s.authToken, s.writeError)

	mux := http.NewServeMux()
	s.registerRoutes(mux)

	addr := fmt.Sprintf("localhost:%d", s.port)
	server := &http.Server{
		Addr:         addr,
		Handler:      SecurityHeaders(mux),
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)

	go func() {
		<-stop
		s.logger.Info("Shutting down web server...")
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		server.Shutdown(ctx)
	}()

	s.logger.Info("Starting web server", "address", addr)
	s.logger.Info("API auth token", "token", s.authToken)
	s.openBrowser(fmt.Sprintf("http://%s?token=%s", addr, s.authToken))

	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return err
	}
	s.logger.Info("Web server stopped")
	return nil
}

// api wraps a handler with recovery + logging + auth middleware.
func (s *Server) api(h http.HandlerFunc) http.HandlerFunc {
	return s.recover(s.logReq(s.authWrap(h)))
}

func (s *Server) registerRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/accounts", s.api(s.handleGetAccounts))
	mux.HandleFunc("POST /api/accounts", s.api(s.handleAddAccount))
	mux.HandleFunc("GET /api/accounts/{id}/roles", s.api(s.handleGetRoles))
	mux.HandleFunc("POST /api/roles", s.api(s.handleAddRole))
	mux.HandleFunc("GET /api/session/active", s.api(s.handleGetActiveSession))
	mux.HandleFunc("POST /api/session/switch", s.api(s.handleSwitchSession))
	mux.HandleFunc("GET /api/session/login-status/{profileName}", s.api(s.handleGetLoginStatus))
	mux.HandleFunc("POST /api/session/login", s.api(s.handleLoginRole))
	mux.HandleFunc("GET /api/config/import", s.api(s.handleImportConfig))
	mux.HandleFunc("POST /api/config/import", s.api(s.handleImportConfig))

	webFS, _ := s.getWebDir()
	fileServer := http.FileServer(webFS)
	mux.HandleFunc("GET /{path...}", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" {
			f, err := webFS.Open("/index.html")
			if err != nil {
				http.Error(w, "index.html not found", http.StatusNotFound)
				return
			}
			defer f.Close()
			stat, _ := f.Stat()
			http.ServeContent(w, r, "index.html", stat.ModTime(), f.(io.ReadSeeker))
			return
		}
		fileServer.ServeHTTP(w, r)
	})
}

// --- Handlers ---

func (s *Server) handleGetAccounts(w http.ResponseWriter, r *http.Request) {
	repo := s.dbRepo.WithContext(r.Context())
	accounts, err := repo.GetAllAWSAccounts()
	if err != nil {
		s.logger.Error("Database error", "operation", "GetAllAWSAccounts", "error", err)
		s.writeError(w, http.StatusInternalServerError, "Internal server error")
		return
	}

	response := make([]AccountResponse, len(accounts))
	for i, acc := range accounts {
		response[i] = AccountResponse{
			ID: acc.ID, AccountID: acc.AccountID,
			AccountName: acc.AccountName, Description: acc.Description.String,
		}
	}
	s.writeJSON(w, http.StatusOK, response)
}

func (s *Server) handleAddAccount(w http.ResponseWriter, r *http.Request) {
	var req AddAccountRequest
	if !s.decodeBody(w, r, &req) {
		return
	}
	if errs := req.Validate(); len(errs) > 0 {
		s.writeValidationError(w, errs)
		return
	}
	if err := s.dbRepo.WithContext(r.Context()).AddAWSAccount(req.AccountID, req.AccountName, req.SSOStartURL, req.SSORegion, req.Description); err != nil {
		s.logger.Error("Database error", "operation", "AddAWSAccount", "account_id", req.AccountID, "error", err)
		s.writeError(w, http.StatusInternalServerError, "Internal server error")
		return
	}
	s.logger.Info("Sensitive operation: account added", "account_id", req.AccountID, "account_name", req.AccountName)
	s.writeJSON(w, http.StatusCreated, map[string]string{"status": "created"})
}

func (s *Server) handleGetRoles(w http.ResponseWriter, r *http.Request) {
	accountIDStr := r.PathValue("id")
	accountID, err := strconv.Atoi(accountIDStr)
	if err != nil {
		s.writeError(w, http.StatusBadRequest, "Invalid account ID")
		return
	}

	repo := s.dbRepo.WithContext(r.Context())
	accounts, err := repo.GetAllAWSAccounts()
	if err != nil {
		s.logger.Error("Database error", "operation", "GetAllAWSAccounts", "error", err)
		s.writeError(w, http.StatusInternalServerError, "Internal server error")
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

	roles, err := repo.GetRolesByAccount(awsAccountID)
	if err != nil {
		s.logger.Error("Database error", "operation", "GetRolesByAccount", "account_id", awsAccountID, "error", err)
		s.writeError(w, http.StatusInternalServerError, "Internal server error")
		return
	}

	response := make([]RoleResponse, len(roles))
	for i, role := range roles {
		response[i] = RoleResponse{
			ID: role.ID, AccountID: role.AccountID, RoleName: role.RoleName,
			ProfileName: role.ProfileName, Region: role.Region, Description: role.Description.String,
		}
	}
	s.writeJSON(w, http.StatusOK, response)
}

func (s *Server) handleAddRole(w http.ResponseWriter, r *http.Request) {
	var req AddRoleRequest
	if !s.decodeBody(w, r, &req) {
		return
	}
	if errs := req.Validate(); len(errs) > 0 {
		s.writeValidationError(w, errs)
		return
	}
	if err := s.dbRepo.WithContext(r.Context()).AddAWSRole(req.AccountID, req.RoleName, req.RoleARN, req.ProfileName, req.Region, req.Description); err != nil {
		s.logger.Error("Database error", "operation", "AddAWSRole", "account_id", req.AccountID, "role_name", req.RoleName, "error", err)
		s.writeError(w, http.StatusInternalServerError, "Internal server error")
		return
	}
	s.logger.Info("Sensitive operation: role added", "account_id", req.AccountID, "role_name", req.RoleName, "profile_name", req.ProfileName)
	s.writeJSON(w, http.StatusCreated, map[string]string{"status": "created"})
}

func (s *Server) handleGetActiveSession(w http.ResponseWriter, r *http.Request) {
	session, role, account, err := s.dbRepo.WithContext(r.Context()).GetActiveSession()
	if err != nil || session == nil || role == nil || account == nil {
		if err != nil {
			s.logger.Error("Database error", "operation", "GetActiveSession", "error", err)
		}
		s.writeJSON(w, http.StatusOK, SessionResponse{Active: false})
		return
	}

	response := SessionResponse{
		Active: true,
		Role: &RoleResponse{
			ID: role.ID, AccountID: role.AccountID, RoleName: role.RoleName,
			ProfileName: role.ProfileName, Region: role.Region, Description: role.Description.String,
		},
		Account: &AccountResponse{
			ID: account.ID, AccountID: account.AccountID,
			AccountName: account.AccountName, Description: account.Description.String,
		},
	}
	if s.kubeManager != nil {
		if ctx, err := s.kubeManager.GetCurrentContext(); err == nil {
			response.KubeContext = ctx
		}
		response.KubeNamespace = s.kubeManager.GetCurrentNamespace()
	}
	s.writeJSON(w, http.StatusOK, response)
}

func (s *Server) handleSwitchSession(w http.ResponseWriter, r *http.Request) {
	var req SwitchSessionRequest
	if !s.decodeBody(w, r, &req) {
		return
	}
	if errs := req.Validate(); len(errs) > 0 {
		s.writeValidationError(w, errs)
		return
	}
	if err := s.roleSwitcher.SwitchRole(req.ProfileName); err != nil {
		s.logger.Error("Role switch failed", "profile_name", req.ProfileName, "error", err)
		s.writeError(w, http.StatusInternalServerError, "Internal server error")
		return
	}
	s.logger.Info("Sensitive operation: session switched", "profile_name", req.ProfileName)

	kubeStatus := "skipped"
	if s.kubeManager != nil {
		env := s.extractEnvName(req.ProfileName)
		if err := s.kubeManager.SwitchContextForEnv(env); err == nil {
			kubeStatus = "switched"
		} else {
			kubeStatus = "failed"
			s.logger.Error("Kubernetes context switch failed", "profile_name", req.ProfileName, "env", env, "error", err)
		}
	}
	s.writeJSON(w, http.StatusOK, map[string]string{"status": "switched", "kube_status": kubeStatus})
}

func (s *Server) handleLoginRole(w http.ResponseWriter, r *http.Request) {
	var req LoginRoleRequest
	if !s.decodeBody(w, r, &req) {
		return
	}
	if errs := req.Validate(); len(errs) > 0 {
		s.writeValidationError(w, errs)
		return
	}
	if s.ssoManager == nil {
		s.logger.Error("SSO manager not initialized", "profile_name", req.ProfileName)
		s.writeError(w, http.StatusInternalServerError, "Internal server error")
		return
	}
	if err := s.ssoManager.ValidateProfile(req.ProfileName); err != nil {
		s.logger.Error("Profile validation failed", "profile_name", req.ProfileName, "error", err)
		s.writeError(w, http.StatusBadRequest, "Invalid profile")
		return
	}
	if err := s.ssoManager.Login(req.ProfileName); err != nil {
		s.logger.Error("SSO login failed", "profile_name", req.ProfileName, "error", err)
		s.writeError(w, http.StatusInternalServerError, "Internal server error")
		return
	}
	if err := s.roleSwitcher.SwitchRole(req.ProfileName); err != nil {
		s.logger.Error("Role switch after login failed", "profile_name", req.ProfileName, "error", err)
		s.writeError(w, http.StatusInternalServerError, "Internal server error")
		return
	}
	s.logger.Info("Sensitive operation: user logged in and switched role", "profile_name", req.ProfileName)

	kubeStatus := "skipped"
	if s.kubeManager != nil {
		env := s.extractEnvName(req.ProfileName)
		if err := s.kubeManager.SwitchContextForEnv(env); err == nil {
			kubeStatus = "switched"
		} else {
			kubeStatus = "failed"
			s.logger.Error("Kubernetes context switch failed after login", "profile_name", req.ProfileName, "env", env, "error", err)
		}
	}
	s.writeJSON(w, http.StatusOK, map[string]string{"status": "logged_in_and_switched", "kube_status": kubeStatus})
}

func (s *Server) handleGetLoginStatus(w http.ResponseWriter, r *http.Request) {
	profileName := r.PathValue("profileName")
	loggedIn := false
	if s.ssoManager != nil {
		loggedIn = s.ssoManager.IsLoggedIn(profileName)
	}
	s.writeJSON(w, http.StatusOK, map[string]interface{}{"profile_name": profileName, "logged_in": loggedIn})
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
		s.logger.Error("Failed to get home directory", "error", err)
		s.writeError(w, http.StatusInternalServerError, "Internal server error")
		return
	}
	configPath := filepath.Join(homeDir, ".aws", "config")
	content, err := os.ReadFile(configPath)
	if err != nil {
		s.logger.Error("Failed to read AWS config", "path", configPath, "error", err)
		s.writeError(w, http.StatusInternalServerError, "Internal server error")
		return
	}
	profiles := s.parseAWSConfig(string(content))
	s.writeJSON(w, http.StatusOK, map[string]interface{}{"profiles": profiles})
}

func (s *Server) executeImportConfig(w http.ResponseWriter, r *http.Request) {
	var req ImportConfigRequest
	if !s.decodeBody(w, r, &req) {
		return
	}
	if errs := req.Validate(); len(errs) > 0 {
		s.writeValidationError(w, errs)
		return
	}

	imported, skipped := 0, 0
	var errors []string
	repo := s.dbRepo.WithContext(r.Context())

	for _, profile := range req.Profiles {
		profileName := profile["name"]
		if profileName == "" || profileName == "default" {
			continue
		}

		accountID := profile["sso_account_id"]
		if accountID == "" {
			if roleARN := profile["role_arn"]; roleARN != "" {
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

		account, err := repo.GetAWSAccount(accountID)
		if err != nil || account == nil {
			envName := s.extractEnvName(profileName)
			ssoStartURL := profile["sso_start_url"]
			ssoRegion := profile["sso_region"]
			if ssoRegion == "" {
				ssoRegion = "eu-west-2"
			}
			if err := repo.AddAWSAccount(accountID, envName, ssoStartURL, ssoRegion, "Imported from AWS config"); err != nil {
				s.logger.Error("Failed to create account during import", "profile_name", profileName, "account_id", accountID, "error", err)
				errors = append(errors, fmt.Sprintf("Profile %s: failed to create account", profileName))
				continue
			}
			account, err = repo.GetAWSAccount(accountID)
			if err != nil || account == nil {
				s.logger.Error("Created account not retrievable", "profile_name", profileName, "account_id", accountID)
				errors = append(errors, fmt.Sprintf("Profile %s: account created but could not be retrieved", profileName))
				continue
			}
		}

		roleName := profile["sso_role_name"]
		if roleName == "" {
			if roleARN := profile["role_arn"]; roleARN != "" {
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

		existingRole, _ := repo.GetRoleByProfileName(profileName)
		if existingRole != nil {
			skipped++
			continue
		}

		if err := repo.AddAWSRole(account.ID, roleName, roleARN, profileName, region, "Imported from AWS config"); err != nil {
			if strings.Contains(err.Error(), "UNIQUE constraint failed") {
				skipped++
				continue
			}
			s.logger.Error("Failed to create role during import", "profile_name", profileName, "account_id", account.ID, "role_name", roleName, "error", err)
			errors = append(errors, fmt.Sprintf("Profile %s: failed to create role", profileName))
			continue
		}
		imported++
	}

	s.logger.Info("Sensitive operation: config imported", "imported", imported, "skipped", skipped, "errors", len(errors))
	s.writeJSON(w, http.StatusOK, map[string]interface{}{"imported": imported, "skipped": skipped, "errors": errors})
}

// --- Helpers ---

func (s *Server) extractEnvName(profileName string) string {
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
			if strings.HasPrefix(profileName, "sso-session") {
				currentProfile = nil
				continue
			}
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

func (s *Server) getWebDir() (http.FileSystem, bool) {
	if _, err := os.Stat("web"); err == nil {
		return http.Dir("web"), false
	}
	ex, err := os.Executable()
	if err == nil {
		dir := filepath.Dir(ex)
		webPath := filepath.Join(dir, "web")
		if _, err := os.Stat(webPath); err == nil {
			return http.Dir(webPath), false
		}
	}
	embedded, err := fs.Sub(webAssets.Assets, ".")
	if err != nil {
		s.logger.Error("Failed to create embedded sub-filesystem", "error", err)
		return http.Dir("web"), false
	}
	return http.FS(embedded), true
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
		if err := cmd.Run(); err != nil {
			s.logger.Warn("Failed to open browser", "error", err)
		}
	}
}

func (s *Server) writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		s.logger.Error("Failed to encode JSON response", "error", err)
	}
}

func (s *Server) writeError(w http.ResponseWriter, statusCode int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	resp := ErrorResponse{}
	if statusCode >= 500 {
		resp.Error = "Internal server error"
	} else {
		resp.Error = message
	}
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		s.logger.Error("Failed to encode error response", "error", err)
	}
}

func (s *Server) writeValidationError(w http.ResponseWriter, validationErrors []ValidationError) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusBadRequest)
	if err := json.NewEncoder(w).Encode(ValidationErrorResponse{
		Error:  "Validation failed",
		Fields: validationErrors,
	}); err != nil {
		s.logger.Error("Failed to encode validation error response", "error", err)
	}
}

func (s *Server) decodeBody(w http.ResponseWriter, r *http.Request, v interface{}) bool {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	if err := json.NewDecoder(r.Body).Decode(v); err != nil {
		s.writeError(w, http.StatusBadRequest, "Invalid request body")
		return false
	}
	return true
}
