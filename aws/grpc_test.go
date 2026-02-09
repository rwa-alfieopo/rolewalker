package aws

import "testing"

func TestGetServiceName(t *testing.T) {
	gm := &GRPCManager{}

	tests := []struct {
		service  string
		expected string
	}{
		{"candidate", "candidate-microservice-grpc"},
		{"job", "job-microservice-grpc"},
		{"BILLING", "billing-microservice-grpc"},
	}

	for _, tt := range tests {
		t.Run(tt.service, func(t *testing.T) {
			result := gm.GetServiceName(tt.service)
			if result != tt.expected {
				t.Errorf("GetServiceName(%q) = %q, want %q", tt.service, result, tt.expected)
			}
		})
	}
}
