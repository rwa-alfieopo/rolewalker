package utils

import (
	"os"
	"regexp"
	"strings"
)

// Package-level compiled regexes (Fix 5: avoid recompiling on every call)
var (
	reNonAlphanumDash = regexp.MustCompile(`[^a-zA-Z0-9-]`)
	reNonLabelChar    = regexp.MustCompile(`[^a-zA-Z0-9._-]`)
)

// SanitizeUsername removes non-alphanumeric characters from username
// and limits length to 20 characters for use in pod names
func SanitizeUsername(username string) string {
	result := reNonAlphanumDash.ReplaceAllString(username, "")
	if len(result) > 20 {
		result = result[:20]
	}
	return result
}

// SanitizeLabelValue sanitizes a string to be a valid Kubernetes label value
// Label values must be 63 characters or less and consist of alphanumeric characters, '-', '_' or '.'
func SanitizeLabelValue(value string) string {
	// Replace @ with 'at' and other special chars with '-'
	value = strings.ReplaceAll(value, "@", "at")
	value = reNonLabelChar.ReplaceAllString(value, "-")
	// Trim to 63 characters max
	if len(value) > 63 {
		value = value[:63]
	}
	return value
}

// GetCurrentUsername retrieves the current username from environment variables
// Returns "unknown" if no username is found
func GetCurrentUsername() string {
	username := SanitizeLabelValue(os.Getenv("USER"))
	if username == "" {
		username = SanitizeLabelValue(os.Getenv("USERNAME"))
	}
	if username == "" {
		username = "unknown"
	}
	return username
}

// GetCurrentUsernamePodSafe retrieves the current username sanitized for pod names
// Returns "unknown" if no username is found
func GetCurrentUsernamePodSafe() string {
	username := SanitizeUsername(os.Getenv("USER"))
	if username == "" {
		username = SanitizeUsername(os.Getenv("USERNAME"))
	}
	if username == "" {
		username = "unknown"
	}
	return username
}

// GetCurrentEmail retrieves the current user email from environment variables
// Returns "unknown" if no email is found
func GetCurrentEmail() string {
	email := SanitizeLabelValue(os.Getenv("EMAIL"))
	if email == "" {
		email = "unknown"
	}
	return email
}
