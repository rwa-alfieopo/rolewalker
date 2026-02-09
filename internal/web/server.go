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
	"net/url"
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

type Server struct {
	port         int
	dbRepo       *db.ConfigRepository
	roleSwitcher *aws.RoleSwitcher
	ssoManager   *aws.SSOManager
	kubeManager  *aws.KubeManager
	logger       *slog.Logger
	authToken    string // Bearer token generated at startup for API auth
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

type ValidationError struct {
	Field   string `json:"field"`
	Message string `json:"message"`
}

type ValidationErrorResponse struct {
	Error  string             `json:"error"`
	Fields []ValidationError  `json:"fields,omitempty"`
}

// Validation helper functions
func isValidAccountID(accountID string) bool {
	// Account ID must be exactly 12 digits
	if len(accountID) != 12 {
		return false
	}
	for _, c := range accountID {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}

func isValidAccountName(name string) bool {
	// Non-empty and max 255 characters
	return len(name) > 0 && len(name) <= 255
}

func isValidURL(urlStr string) bool {
	u, err := url.Parse(urlStr)
	if err != nil {
		return false
	}
	// Must be a valid HTTP/HTTPS URL with a host
	return (u.Scheme == "http" || u.Scheme == "https") && u.Host != ""
}

func isValidRegion(region string) bool {
	// Non-empty region
	return len(strings.TrimSpace(region)) > 0
}

func isValidRoleName(name string) bool {
	// Non-empty and max 255 characters
	return len(name) > 0 && len(name) <= 255
}

func isValidARN(arn string) bool {
	// ARN format: arn:partition:service:region:account-id:resource
	// For IAM roles: arn:aws:iam::account-id:role/role-name
	if !strings.HasPrefix(arn, "arn:") {
		return false
	}
	parts := strings.Split(arn, ":")
	if len(parts) < 6 {
		return false
	}
	// Check if it contains "role" in the resource part
	return strings.Contains(arn, "role")
}

func isValidProfileName(name string) bool {
	// Non-empty and max 255 characters
	return len(name) > 0 && len(name) <= 255
}

func validateAddAccountRequest(req struct {
	AccountID   string `json:"account_id"`
	AccountName string `json:"account_name"`
	SSOStartURL string `json:"sso_start_url"`
	SSORegion   string `json:"sso_region"`
	Description string `json:"description"`
}) []ValidationError {
	var errors []ValidationError

	if !isValidAccountID(req.AccountID) {
		errors = append(errors, ValidationError{
			Field:   "account_id",
			Message: "Account ID must be exactly 12 digits",
		})
	}

	if !isValidAccountName(req.AccountName) {
		errors = append(errors, ValidationError{
			Field:   "account_name",
			Message: "Account name must be non-empty and max 255 characters",
		})
	}

	if req.SSOStartURL != "" && !isValidURL(req.SSOStartURL) {
		errors = append(errors, ValidationError{
			Field:   "sso_start_url",
			Message: "SSO start URL must be a valid HTTP/HTTPS URL",
		})
	}

	if !isValidRegion(req.SSORegion) {
		errors = append(errors, ValidationError{
			Field:   "sso_region",
			Message: "SSO region must be non-empty",
		})
	}

	return errors
}

func validateAddRoleRequest(req struct {
	AccountID   int    `json:"account_id"`
	RoleName    string `json:"role_name"`
	RoleARN     string `json:"role_arn"`
	ProfileName string `json:"profile_name"`
	Region      string `json:"region"`
	Description string `json:"description"`
}) []ValidationError {
	var errors []ValidationError

	if !isValidRoleName(req.RoleName) {
		errors = append(errors, ValidationError{
			Field:   "role_name",
			Message: "Role name must be non-empty and max 255 characters",
		})
	}

	if req.RoleARN != "" && !isValidARN(req.RoleARN) {
		errors = append(errors, ValidationError{
			Field:   "role_arn",
			Message: "Role ARN must be in valid ARN format (arn:aws:iam::account-id:role/role-name)",
		})
	}

	if !isValidProfileName(req.ProfileName) {
		errors = append(errors, ValidationError{
			Field:   "profile_name",
			Message: "Profile name must be non-empty and max 255 characters",
		})
	}

	if !isValidRegion(req.Region) {
		errors = append(errors, ValidationError{
			Field:   "region",
			Message: "Region must be non-empty",
		})
	}

	return errors
}

func validateSwitchSessionRequest(req struct {
	ProfileName string `json:"profile_name"`
}) []ValidationError {
	var errors []ValidationError

	if !isValidProfileName(req.ProfileName) {
		errors = append(errors, ValidationError{
			Field:   "profile_name",
			Message: "Profile name must be non-empty and max 255 characters",
		})
	}

	return errors
}

func validateLoginRoleRequest(req struct {
	ProfileName string `json:"profile_name"`
}) []ValidationError {
	var errors []ValidationError

	if !isValidProfileName(req.ProfileName) {
		errors = append(errors, ValidationError{
			Field:   "profile_name",
			Message: "Profile name must be non-empty and max 255 characters",
		})
	}

	return errors
}

func validateImportConfigRequest(req struct {
	Profiles []map[string]string `json:"profiles"`
}) []ValidationError {
	var errors []ValidationError

	if len(req.Profiles) == 0 {
		errors = append(errors, ValidationError{
			Field:   "profiles",
			Message: "Profiles list must not be empty",
		})
		return errors
	}

	// Validate each profile
	for i, profile := range req.Profiles {
		profileName := profile["name"]
		if profileName == "" {
			errors = append(errors, ValidationError{
				Field:   fmt.Sprintf("profiles[%d].name", i),
				Message: "Profile name must be non-empty",
			})
		} else if !isValidProfileName(profileName) {
			errors = append(errors, ValidationError{
				Field:   fmt.Sprintf("profiles[%d].name", i),
				Message: "Profile name must be max 255 characters",
			})
		}
	}

	return errors
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

func NewServer(port int, dbRepo *db.ConfigRepository, roleSwitcher *aws.RoleSwitcher) *Server {
	ssoManager, _ := aws.NewSSOManager()
	kubeManager := aws.NewKubeManager()

	// Generate a random bearer token for API authentication
	tokenBytes := make([]byte, 32)
	if _, err := crypto_rand.Read(tokenBytes); err != nil {
		// Fallback: use a timestamp-based token (less secure but functional)
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
	mux := http.NewServeMux()

	// API endpoints (wrapped with auth + recovery + logging middleware)
	mux.HandleFunc("GET /api/accounts", s.RecoveryMiddleware(s.logRequest(s.requireAuth(s.handleGetAccounts))))
	mux.HandleFunc("POST /api/accounts", s.RecoveryMiddleware(s.logRequest(s.requireAuth(s.handleAddAccount))))
	mux.HandleFunc("GET /api/accounts/{id}/roles", s.RecoveryMiddleware(s.logRequest(s.requireAuth(s.handleGetRoles))))
	mux.HandleFunc("POST /api/roles", s.RecoveryMiddleware(s.logRequest(s.requireAuth(s.handleAddRole))))
	mux.HandleFunc("GET /api/session/active", s.RecoveryMiddleware(s.logRequest(s.requireAuth(s.handleGetActiveSession))))
	mux.HandleFunc("POST /api/session/switch", s.RecoveryMiddleware(s.logRequest(s.requireAuth(s.handleSwitchSession))))
	mux.HandleFunc("GET /api/session/login-status/{profileName}", s.RecoveryMiddleware(s.logRequest(s.requireAuth(s.handleGetLoginStatus))))
	mux.HandleFunc("POST /api/session/login", s.RecoveryMiddleware(s.logRequest(s.requireAuth(s.handleLoginRole))))
	mux.HandleFunc("GET /api/config/import", s.RecoveryMiddleware(s.logRequest(s.requireAuth(s.handleImportConfig))))
	mux.HandleFunc("POST /api/config/import", s.RecoveryMiddleware(s.logRequest(s.requireAuth(s.handleImportConfig))))

	// Serve static files using embedded or disk filesystem
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

	addr := fmt.Sprintf("localhost:%d", s.port)
	server := &http.Server{
		Addr:         addr,
		Handler:      s.securityHeaders(mux),
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// Graceful shutdown on Ctrl+C / SIGTERM — releases DB lock cleanly
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

// securityHeaders adds basic security headers to all responses
func (s *Server) securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self' 'unsafe-inline'; style-src 'self' 'unsafe-inline'; img-src 'self' data:")
		w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
		next.ServeHTTP(w, r)
	})
}

// requireAuth validates the bearer token or query param token for API requests
func (s *Server) requireAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		token := ""
		// Check Authorization header first
		if auth := r.Header.Get("Authorization"); strings.HasPrefix(auth, "Bearer ") {
			token = strings.TrimPrefix(auth, "Bearer ")
		}
		// Fall back to query parameter
		if token == "" {
			token = r.URL.Query().Get("token")
		}
		if token != s.authToken {
			s.writeError(w, http.StatusUnauthorized, "Unauthorized")
			return
		}
		next(w, r)
	}
}

func (s *Server) getWebDir() (http.FileSystem, bool) {
	// Try current directory first
	if _, err := os.Stat("web"); err == nil {
		return http.Dir("web"), false
	}
	// Try relative to executable
	ex, err := os.Executable()
	if err == nil {
		dir := filepath.Dir(ex)
		webPath := filepath.Join(dir, "web")
		if _, err := os.Stat(webPath); err == nil {
			return http.Dir(webPath), false
		}
	}
	// Use embedded filesystem as fallback
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

func (s *Server) RecoveryMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if err := recover(); err != nil {
				stack := make([]byte, 4096)
				n := runtime.Stack(stack, false)
				s.logger.Error("Panic recovered",
					"error", err,
					"method", r.Method,
					"path", r.URL.Path,
					"stack", string(stack[:n]),
				)
				http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			}
		}()
		next(w, r)
	}
}

func (s *Server) logRequest(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		
		// Create a response writer wrapper to capture status code
		wrapped := &responseWriter{ResponseWriter: w, statusCode: http.StatusOK}
		
		// Call the handler
		next(wrapped, r)
		
		// Log the request
		duration := time.Since(start).Milliseconds()
		s.logger.Info("API request",
			"method", r.Method,
			"path", r.URL.Path,
			"status", wrapped.statusCode,
			"duration_ms", duration,
		)
	}
}

type responseWriter struct {
	http.ResponseWriter
	statusCode int
}

func (rw *responseWriter) WriteHeader(code int) {
	rw.statusCode = code
	rw.ResponseWriter.WriteHeader(code)
}

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

	r.Body = http.MaxBytesReader(w, r.Body, 1<<20) // 1MB max
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.writeError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	// Validate request
	if validationErrors := validateAddAccountRequest(req); len(validationErrors) > 0 {
		s.writeValidationError(w, validationErrors)
		return
	}

	if err := s.dbRepo.WithContext(r.Context()).AddAWSAccount(req.AccountID, req.AccountName, req.SSOStartURL, req.SSORegion, req.Description); err != nil {
		s.logger.Error("Database error", "operation", "AddAWSAccount", "account_id", req.AccountID, "error", err)
		s.writeError(w, http.StatusInternalServerError, "Internal server error")
		return
	}

	s.logger.Info("Sensitive operation: account added", "account_id", req.AccountID, "account_name", req.AccountName)

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

	repo := s.dbRepo.WithContext(r.Context())

	// Get the account to find its AWS account ID
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

	r.Body = http.MaxBytesReader(w, r.Body, 1<<20) // 1MB max
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.writeError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	// Validate request
	if validationErrors := validateAddRoleRequest(req); len(validationErrors) > 0 {
		s.writeValidationError(w, validationErrors)
		return
	}

	if err := s.dbRepo.WithContext(r.Context()).AddAWSRole(req.AccountID, req.RoleName, req.RoleARN, req.ProfileName, req.Region, req.Description); err != nil {
		s.logger.Error("Database error", "operation", "AddAWSRole", "account_id", req.AccountID, "role_name", req.RoleName, "error", err)
		s.writeError(w, http.StatusInternalServerError, "Internal server error")
		return
	}

	s.logger.Info("Sensitive operation: role added", "account_id", req.AccountID, "role_name", req.RoleName, "profile_name", req.ProfileName)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]string{"status": "created"})
}

func (s *Server) handleGetActiveSession(w http.ResponseWriter, r *http.Request) {
	session, role, account, err := s.dbRepo.WithContext(r.Context()).GetActiveSession()
	if err != nil {
		s.logger.Error("Database error", "operation", "GetActiveSession", "error", err)
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

	r.Body = http.MaxBytesReader(w, r.Body, 1<<20) // 1MB max
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.writeError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	// Validate request
	if validationErrors := validateSwitchSessionRequest(req); len(validationErrors) > 0 {
		s.writeValidationError(w, validationErrors)
		return
	}

	if err := s.roleSwitcher.SwitchRole(req.ProfileName); err != nil {
		s.logger.Error("Role switch failed", "profile_name", req.ProfileName, "error", err)
		s.writeError(w, http.StatusInternalServerError, "Internal server error")
		return
	}

	s.logger.Info("Sensitive operation: session switched", "profile_name", req.ProfileName)

	// Try to switch Kubernetes context if available
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

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "switched", "kube_status": kubeStatus})
}

func (s *Server) handleLoginRole(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ProfileName string `json:"profile_name"`
	}

	r.Body = http.MaxBytesReader(w, r.Body, 1<<20) // 1MB max
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.writeError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	// Validate request
	if validationErrors := validateLoginRoleRequest(req); len(validationErrors) > 0 {
		s.writeValidationError(w, validationErrors)
		return
	}

	if s.ssoManager == nil {
		s.logger.Error("SSO manager not initialized", "profile_name", req.ProfileName)
		s.writeError(w, http.StatusInternalServerError, "Internal server error")
		return
	}

	// Validate the profile exists
	if err := s.ssoManager.ValidateProfile(req.ProfileName); err != nil {
		s.logger.Error("Profile validation failed", "profile_name", req.ProfileName, "error", err)
		s.writeError(w, http.StatusBadRequest, "Invalid profile")
		return
	}

	// Initiate SSO login
	if err := s.ssoManager.Login(req.ProfileName); err != nil {
		s.logger.Error("SSO login failed", "profile_name", req.ProfileName, "error", err)
		s.writeError(w, http.StatusInternalServerError, "Internal server error")
		return
	}

	// After successful login, switch to the role
	if err := s.roleSwitcher.SwitchRole(req.ProfileName); err != nil {
		s.logger.Error("Role switch after login failed", "profile_name", req.ProfileName, "error", err)
		s.writeError(w, http.StatusInternalServerError, "Internal server error")
		return
	}

	s.logger.Info("Sensitive operation: user logged in and switched role", "profile_name", req.ProfileName)

	// Try to switch Kubernetes context if available
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

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"profiles": profiles,
	})
}

func (s *Server) executeImportConfig(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Profiles []map[string]string `json:"profiles"`
	}

	r.Body = http.MaxBytesReader(w, r.Body, 1<<20) // 1MB max
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.writeError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	// Validate request
	if validationErrors := validateImportConfigRequest(req); len(validationErrors) > 0 {
		s.writeValidationError(w, validationErrors)
		return
	}

	imported := 0
	skipped := 0
	errors := []string{}

	repo := s.dbRepo.WithContext(r.Context())

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
		account, err := repo.GetAWSAccount(accountID)
		if err != nil || account == nil {
			// Account doesn't exist, create it
			// Extract environment name from profile (e.g., "prod-admin" -> "prod")
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
			
			// Fetch the newly created account
			account, err = repo.GetAWSAccount(accountID)
			if err != nil || account == nil {
				s.logger.Error("Created account not retrievable", "profile_name", profileName, "account_id", accountID)
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
		existingRole, _ := repo.GetRoleByProfileName(profileName)
		if existingRole != nil {
			skipped++
			continue
		}

		// Create role
		if err := repo.AddAWSRole(account.ID, roleName, roleARN, profileName, region, "Imported from AWS config"); err != nil {
			// Check if it's a duplicate role name error
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
