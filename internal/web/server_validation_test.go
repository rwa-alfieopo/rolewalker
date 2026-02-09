package web

import "testing"

func TestIsValidAccountID(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected bool
	}{
		{"valid 12 digits", "123456789012", true},
		{"too short", "12345", false},
		{"too long", "1234567890123", false},
		{"with letters", "12345678901a", false},
		{"empty", "", false},
		{"with dashes", "123-456-7890", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isValidAccountID(tt.input)
			if result != tt.expected {
				t.Errorf("isValidAccountID(%q) = %v, want %v", tt.input, result, tt.expected)
			}
		})
	}
}

func TestIsValidAccountName(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected bool
	}{
		{"valid", "Production", true},
		{"empty", "", false},
		{"max length", string(make([]byte, 255)), true},
		{"too long", string(make([]byte, 256)), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isValidAccountName(tt.input)
			if result != tt.expected {
				t.Errorf("isValidAccountName(%q) = %v, want %v", tt.input, result, tt.expected)
			}
		})
	}
}

func TestIsValidURL(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected bool
	}{
		{"https", "https://example.com/start", true},
		{"http", "http://example.com", true},
		{"no scheme", "example.com", false},
		{"ftp", "ftp://example.com", false},
		{"empty", "", false},
		{"just scheme", "https://", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isValidURL(tt.input)
			if result != tt.expected {
				t.Errorf("isValidURL(%q) = %v, want %v", tt.input, result, tt.expected)
			}
		})
	}
}

func TestIsValidRegion(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected bool
	}{
		{"valid", "eu-west-2", true},
		{"empty", "", false},
		{"whitespace only", "   ", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isValidRegion(tt.input)
			if result != tt.expected {
				t.Errorf("isValidRegion(%q) = %v, want %v", tt.input, result, tt.expected)
			}
		})
	}
}

func TestIsValidARN(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected bool
	}{
		{"valid role ARN", "arn:aws:iam::123456789012:role/AdminRole", true},
		{"not an ARN", "not-an-arn", false},
		{"empty", "", false},
		{"arn without role", "arn:aws:s3:::my-bucket", false},
		{"too few parts", "arn:aws:iam", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isValidARN(tt.input)
			if result != tt.expected {
				t.Errorf("isValidARN(%q) = %v, want %v", tt.input, result, tt.expected)
			}
		})
	}
}

func TestIsValidProfileName(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected bool
	}{
		{"valid", "zenith-dev", true},
		{"empty", "", false},
		{"max length", string(make([]byte, 255)), true},
		{"too long", string(make([]byte, 256)), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isValidProfileName(tt.input)
			if result != tt.expected {
				t.Errorf("isValidProfileName(%q) = %v, want %v", tt.input, result, tt.expected)
			}
		})
	}
}

func TestValidateAddAccountRequest(t *testing.T) {
	// Valid request
	req := AddAccountRequest{
		AccountID:   "123456789012",
		AccountName: "Production",
		SSOStartURL: "https://example.awsapps.com/start",
		SSORegion:   "eu-west-2",
	}

	errors := req.Validate()
	if len(errors) != 0 {
		t.Errorf("Expected no errors for valid request, got %d: %v", len(errors), errors)
	}

	// Invalid request
	req.AccountID = "invalid"
	req.AccountName = ""
	errors = req.Validate()
	if len(errors) < 2 {
		t.Errorf("Expected at least 2 errors for invalid request, got %d", len(errors))
	}
}
