package web

import (
	"fmt"
	"net/url"
	"strings"
)

// ValidationError represents a single field validation error.
type ValidationError struct {
	Field   string `json:"field"`
	Message string `json:"message"`
}

// ValidationErrorResponse is the API response for validation failures.
type ValidationErrorResponse struct {
	Error  string            `json:"error"`
	Fields []ValidationError `json:"fields,omitempty"`
}

// --- Validation helpers ---

func isValidAccountID(accountID string) bool {
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
	return len(name) > 0 && len(name) <= 255
}

func isValidURL(urlStr string) bool {
	u, err := url.Parse(urlStr)
	if err != nil {
		return false
	}
	return (u.Scheme == "http" || u.Scheme == "https") && u.Host != ""
}

func isValidRegion(region string) bool {
	return len(strings.TrimSpace(region)) > 0
}

func isValidRoleName(name string) bool {
	return len(name) > 0 && len(name) <= 255
}

func isValidARN(arn string) bool {
	if !strings.HasPrefix(arn, "arn:") {
		return false
	}
	parts := strings.Split(arn, ":")
	if len(parts) < 6 {
		return false
	}
	return strings.Contains(arn, "role")
}

func isValidProfileName(name string) bool {
	return len(name) > 0 && len(name) <= 255
}

// --- Request types ---

// AddAccountRequest is the payload for creating an AWS account.
type AddAccountRequest struct {
	AccountID   string `json:"account_id"`
	AccountName string `json:"account_name"`
	SSOStartURL string `json:"sso_start_url"`
	SSORegion   string `json:"sso_region"`
	Description string `json:"description"`
}

// Validate returns validation errors for the request.
func (r AddAccountRequest) Validate() []ValidationError {
	var errors []ValidationError
	if !isValidAccountID(r.AccountID) {
		errors = append(errors, ValidationError{Field: "account_id", Message: "Account ID must be exactly 12 digits"})
	}
	if !isValidAccountName(r.AccountName) {
		errors = append(errors, ValidationError{Field: "account_name", Message: "Account name must be non-empty and max 255 characters"})
	}
	if r.SSOStartURL != "" && !isValidURL(r.SSOStartURL) {
		errors = append(errors, ValidationError{Field: "sso_start_url", Message: "SSO start URL must be a valid HTTP/HTTPS URL"})
	}
	if !isValidRegion(r.SSORegion) {
		errors = append(errors, ValidationError{Field: "sso_region", Message: "SSO region must be non-empty"})
	}
	return errors
}

// AddRoleRequest is the payload for creating an AWS role.
type AddRoleRequest struct {
	AccountID   int    `json:"account_id"`
	RoleName    string `json:"role_name"`
	RoleARN     string `json:"role_arn"`
	ProfileName string `json:"profile_name"`
	Region      string `json:"region"`
	Description string `json:"description"`
}

// Validate returns validation errors for the request.
func (r AddRoleRequest) Validate() []ValidationError {
	var errors []ValidationError
	if !isValidRoleName(r.RoleName) {
		errors = append(errors, ValidationError{Field: "role_name", Message: "Role name must be non-empty and max 255 characters"})
	}
	if r.RoleARN != "" && !isValidARN(r.RoleARN) {
		errors = append(errors, ValidationError{Field: "role_arn", Message: "Role ARN must be in valid ARN format (arn:aws:iam::account-id:role/role-name)"})
	}
	if !isValidProfileName(r.ProfileName) {
		errors = append(errors, ValidationError{Field: "profile_name", Message: "Profile name must be non-empty and max 255 characters"})
	}
	if !isValidRegion(r.Region) {
		errors = append(errors, ValidationError{Field: "region", Message: "Region must be non-empty"})
	}
	return errors
}

// SwitchSessionRequest is the payload for switching sessions.
type SwitchSessionRequest struct {
	ProfileName string `json:"profile_name"`
}

// Validate returns validation errors for the request.
func (r SwitchSessionRequest) Validate() []ValidationError {
	var errors []ValidationError
	if !isValidProfileName(r.ProfileName) {
		errors = append(errors, ValidationError{Field: "profile_name", Message: "Profile name must be non-empty and max 255 characters"})
	}
	return errors
}

// LoginRoleRequest is the payload for SSO login.
type LoginRoleRequest struct {
	ProfileName string `json:"profile_name"`
}

// Validate returns validation errors for the request.
func (r LoginRoleRequest) Validate() []ValidationError {
	var errors []ValidationError
	if !isValidProfileName(r.ProfileName) {
		errors = append(errors, ValidationError{Field: "profile_name", Message: "Profile name must be non-empty and max 255 characters"})
	}
	return errors
}

// ImportConfigRequest is the payload for importing AWS config profiles.
type ImportConfigRequest struct {
	Profiles []map[string]string `json:"profiles"`
}

// Validate returns validation errors for the request.
func (r ImportConfigRequest) Validate() []ValidationError {
	var errors []ValidationError
	if len(r.Profiles) == 0 {
		errors = append(errors, ValidationError{Field: "profiles", Message: "Profiles list must not be empty"})
		return errors
	}
	for i, profile := range r.Profiles {
		profileName := profile["name"]
		if profileName == "" {
			errors = append(errors, ValidationError{Field: fmt.Sprintf("profiles[%d].name", i), Message: "Profile name must be non-empty"})
		} else if !isValidProfileName(profileName) {
			errors = append(errors, ValidationError{Field: fmt.Sprintf("profiles[%d].name", i), Message: "Profile name must be max 255 characters"})
		}
	}
	return errors
}
