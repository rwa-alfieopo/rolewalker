package services

import (
	"fmt"
	"rolewalkers/aws"
)

// AWSService provides AWS operations to the frontend
type AWSService struct {
	configManager   *aws.ConfigManager
	ssoManager      *aws.SSOManager
	profileSwitcher *aws.ProfileSwitcher
}

// NewAWSService creates a new AWS service
func NewAWSService() (*AWSService, error) {
	cm, err := aws.NewConfigManager()
	if err != nil {
		return nil, err
	}

	sm, err := aws.NewSSOManager()
	if err != nil {
		return nil, err
	}

	ps, err := aws.NewProfileSwitcher()
	if err != nil {
		return nil, err
	}

	return &AWSService{
		configManager:   cm,
		ssoManager:      sm,
		profileSwitcher: ps,
	}, nil
}

// ProfileInfo contains profile information for the frontend
type ProfileInfo struct {
	Name         string `json:"name"`
	Region       string `json:"region"`
	SSOStartURL  string `json:"ssoStartUrl"`
	SSORegion    string `json:"ssoRegion"`
	SSOAccountID string `json:"ssoAccountId"`
	SSORoleName  string `json:"ssoRoleName"`
	IsSSO        bool   `json:"isSso"`
	IsActive     bool   `json:"isActive"`
	IsLoggedIn   bool   `json:"isLoggedIn"`
}

// GetProfiles returns all AWS profiles
func (s *AWSService) GetProfiles() ([]ProfileInfo, error) {
	profiles, err := s.configManager.GetProfiles()
	if err != nil {
		return nil, err
	}

	result := make([]ProfileInfo, len(profiles))
	for i, p := range profiles {
		result[i] = ProfileInfo{
			Name:         p.Name,
			Region:       p.Region,
			SSOStartURL:  p.SSOStartURL,
			SSORegion:    p.SSORegion,
			SSOAccountID: p.SSOAccountID,
			SSORoleName:  p.SSORoleName,
			IsSSO:        p.IsSSO,
			IsActive:     p.IsActive,
			IsLoggedIn:   s.ssoManager.IsLoggedIn(p.Name),
		}
	}

	return result, nil
}

// GetActiveProfile returns the currently active profile
func (s *AWSService) GetActiveProfile() string {
	return s.configManager.GetActiveProfile()
}

// SwitchProfile switches to a different AWS profile
func (s *AWSService) SwitchProfile(profileName string) error {
	return s.profileSwitcher.SwitchProfile(profileName)
}

// Login initiates SSO login for a profile
func (s *AWSService) Login(profileName string) error {
	return s.ssoManager.Login(profileName)
}

// Logout clears SSO session for a profile
func (s *AWSService) Logout(profileName string) error {
	return s.ssoManager.Logout(profileName)
}

// IsLoggedIn checks if SSO session is valid
func (s *AWSService) IsLoggedIn(profileName string) bool {
	return s.ssoManager.IsLoggedIn(profileName)
}

// GetSSOProfiles returns only SSO-enabled profiles
func (s *AWSService) GetSSOProfiles() ([]ProfileInfo, error) {
	profiles, err := s.ssoManager.GetSSOProfiles()
	if err != nil {
		return nil, err
	}

	result := make([]ProfileInfo, len(profiles))
	for i, p := range profiles {
		result[i] = ProfileInfo{
			Name:         p.Name,
			Region:       p.Region,
			SSOStartURL:  p.SSOStartURL,
			SSORegion:    p.SSORegion,
			SSOAccountID: p.SSOAccountID,
			SSORoleName:  p.SSORoleName,
			IsSSO:        p.IsSSO,
			IsActive:     p.IsActive,
			IsLoggedIn:   s.ssoManager.IsLoggedIn(p.Name),
		}
	}

	return result, nil
}

// GetStartURLs returns unique SSO start URLs
func (s *AWSService) GetStartURLs() ([]string, error) {
	return s.ssoManager.GetStartURLs()
}

// GetDefaultRegion returns the default region
func (s *AWSService) GetDefaultRegion() string {
	return s.profileSwitcher.GetDefaultRegion()
}

// GetShellExport returns shell export commands for a profile
func (s *AWSService) GetShellExport(profileName string, shell string) (string, error) {
	return s.profileSwitcher.GenerateShellExport(profileName, shell)
}

// ValidateProfile validates a profile configuration
func (s *AWSService) ValidateProfile(profileName string) error {
	return s.ssoManager.ValidateProfile(profileName)
}

// GetProfilesByStartURL groups profiles by SSO start URL
func (s *AWSService) GetProfilesByStartURL() (map[string][]ProfileInfo, error) {
	grouped, err := s.ssoManager.GetProfilesByStartURL()
	if err != nil {
		return nil, err
	}

	result := make(map[string][]ProfileInfo)
	for url, profiles := range grouped {
		infos := make([]ProfileInfo, len(profiles))
		for i, p := range profiles {
			infos[i] = ProfileInfo{
				Name:         p.Name,
				Region:       p.Region,
				SSOStartURL:  p.SSOStartURL,
				SSORegion:    p.SSORegion,
				SSOAccountID: p.SSOAccountID,
				SSORoleName:  p.SSORoleName,
				IsSSO:        p.IsSSO,
				IsActive:     p.IsActive,
				IsLoggedIn:   s.ssoManager.IsLoggedIn(p.Name),
			}
		}
		result[url] = infos
	}

	return result, nil
}

// RefreshCredentials refreshes credentials for a profile
func (s *AWSService) RefreshCredentials(profileName string) error {
	return s.ssoManager.RefreshCredentials(profileName)
}

// GetCredentialExpiry returns credential expiry time
func (s *AWSService) GetCredentialExpiry(profileName string) (string, error) {
	expiry, err := s.ssoManager.GetCredentialExpiry(profileName)
	if err != nil {
		return "", err
	}
	return expiry.Format("2006-01-02 15:04:05"), nil
}

// LoginAndSwitch logs in and switches to a profile
func (s *AWSService) LoginAndSwitch(profileName string) error {
	if err := s.ssoManager.Login(profileName); err != nil {
		return fmt.Errorf("login failed: %w", err)
	}

	if err := s.profileSwitcher.SwitchProfile(profileName); err != nil {
		return fmt.Errorf("switch failed: %w", err)
	}

	return nil
}
