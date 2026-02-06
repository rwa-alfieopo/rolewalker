package aws

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// PromptComponent represents a component that can be shown in the shell prompt
type PromptComponent string

const (
	PromptTime   PromptComponent = "time"
	PromptFolder PromptComponent = "folder"
	PromptAWS    PromptComponent = "aws"
	PromptK8s    PromptComponent = "k8s"
	PromptGit    PromptComponent = "git"
)

// AllPromptComponents returns all available prompt components
func AllPromptComponents() []PromptComponent {
	return []PromptComponent{PromptTime, PromptFolder, PromptAWS, PromptK8s, PromptGit}
}

// PromptManager handles shell prompt customization
type PromptManager struct{}

// NewPromptManager creates a new PromptManager
func NewPromptManager() *PromptManager {
	return &PromptManager{}
}

// DetectShell returns the current shell type
func (pm *PromptManager) DetectShell() string {
	if shell := os.Getenv("SHELL"); shell != "" {
		if strings.Contains(shell, "zsh") {
			return "zsh"
		}
		if strings.Contains(shell, "bash") {
			return "bash"
		}
	}
	if os.Getenv("PSModulePath") != "" {
		return "powershell"
	}
	if runtime.GOOS == "windows" {
		return "powershell"
	}
	return "bash"
}

// GetShellProfilePath returns the profile path for the detected shell
func (pm *PromptManager) GetShellProfilePath(shell string) (string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("failed to get home directory: %w", err)
	}

	switch shell {
	case "zsh":
		return filepath.Join(homeDir, ".zshrc"), nil
	case "bash":
		return filepath.Join(homeDir, ".bashrc"), nil
	case "powershell", "pwsh":
		paths := []string{
			filepath.Join(homeDir, "Documents", "PowerShell", "Microsoft.PowerShell_profile.ps1"),
			filepath.Join(homeDir, "Documents", "WindowsPowerShell", "Microsoft.PowerShell_profile.ps1"),
		}
		for _, p := range paths {
			if _, err := os.Stat(p); err == nil {
				return p, nil
			}
		}
		return paths[0], nil
	default:
		return "", fmt.Errorf("unsupported shell: %s", shell)
	}
}

// InstallPrompt writes the prompt function into the shell profile
func (pm *PromptManager) InstallPrompt(shell string, components []PromptComponent) error {
	profilePath, err := pm.GetShellProfilePath(shell)
	if err != nil {
		return err
	}

	// Read existing content
	content, _ := os.ReadFile(profilePath)

	// Remove old rw prompt block if present
	cleaned := pm.removePromptBlock(string(content))

	// Generate new prompt block
	promptBlock := pm.generatePromptBlock(shell, components)

	// Write back
	newContent := cleaned + promptBlock
	if err := os.MkdirAll(filepath.Dir(profilePath), 0755); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}
	if err := os.WriteFile(profilePath, []byte(newContent), 0644); err != nil {
		return fmt.Errorf("failed to write profile: %w", err)
	}

	return nil
}

// RemovePrompt removes the rw prompt block from the shell profile
func (pm *PromptManager) RemovePrompt(shell string) error {
	profilePath, err := pm.GetShellProfilePath(shell)
	if err != nil {
		return err
	}

	content, err := os.ReadFile(profilePath)
	if err != nil {
		return fmt.Errorf("failed to read profile: %w", err)
	}

	cleaned := pm.removePromptBlock(string(content))
	return os.WriteFile(profilePath, []byte(cleaned), 0644)
}

const promptBlockStart = "# >>> rw prompt >>>"
const promptBlockEnd = "# <<< rw prompt <<<"

func (pm *PromptManager) removePromptBlock(content string) string {
	startIdx := strings.Index(content, promptBlockStart)
	if startIdx == -1 {
		return content
	}
	endIdx := strings.Index(content, promptBlockEnd)
	if endIdx == -1 {
		return content
	}
	endIdx += len(promptBlockEnd)
	// Also remove trailing newline
	if endIdx < len(content) && content[endIdx] == '\n' {
		endIdx++
	}
	return content[:startIdx] + content[endIdx:]
}

func (pm *PromptManager) generatePromptBlock(shell string, components []PromptComponent) string {
	switch shell {
	case "zsh":
		return pm.generateZshPrompt(components)
	case "bash":
		return pm.generateBashPrompt(components)
	case "powershell", "pwsh":
		return pm.generatePowerShellPrompt(components)
	default:
		return ""
	}
}

func (pm *PromptManager) generateZshPrompt(components []PromptComponent) string {
	var parts []string
	for _, c := range components {
		switch c {
		case PromptTime:
			parts = append(parts, `'%F{cyan}%T%f'`)  // %T = HH:MM:SS
		case PromptFolder:
			parts = append(parts, `'%F{blue}%1~%f'`)  // %1~ = current dir
		case PromptAWS:
			parts = append(parts, `'"${_rw_aws}"'`)
		case PromptK8s:
			parts = append(parts, `'"${_rw_k8s}"'`)
		case PromptGit:
			parts = append(parts, `'"${_rw_git}"'`)
		}
	}

	promptExpr := strings.Join(parts, `' '`)

	return fmt.Sprintf(`
%s
# Shell prompt managed by rw - do not edit manually
setopt PROMPT_SUBST

_rw_prompt_info() {
  _rw_aws=""
  _rw_k8s=""
  _rw_git=""

  # AWS profile
  local aws_profile="${AWS_PROFILE:-$(aws configure get profile 2>/dev/null)}"
  if [[ -n "$aws_profile" ]]; then
    _rw_aws="%%F{yellow}☁ ${aws_profile}%%f"
  fi

  # Kubernetes context/namespace
  local k8s_ctx
  k8s_ctx=$(kubectl config current-context 2>/dev/null)
  if [[ -n "$k8s_ctx" ]]; then
    # Shorten ARN-style context names
    k8s_ctx="${k8s_ctx##*/}"
    local k8s_ns
    k8s_ns=$(kubectl config view --minify --output 'jsonpath={..namespace}' 2>/dev/null)
    k8s_ns="${k8s_ns:-default}"
    _rw_k8s="%%F{magenta}⎈ ${k8s_ctx}/${k8s_ns}%%f"
  fi

  # Git branch
  local git_branch
  git_branch=$(git symbolic-ref --short HEAD 2>/dev/null || git rev-parse --short HEAD 2>/dev/null)
  if [[ -n "$git_branch" ]]; then
    _rw_git="%%F{green} ${git_branch}%%f"
  fi
}

precmd_functions+=(_rw_prompt_info)

PROMPT=$'\n'%s$'\n'"%%F{white}❯%%f "
%s
`, promptBlockStart, promptExpr, promptBlockEnd)
}

func (pm *PromptManager) generateBashPrompt(components []PromptComponent) string {
	var parts []string
	for _, c := range components {
		switch c {
		case PromptTime:
			parts = append(parts, `"\[\e[36m\]\t\[\e[0m\]"`)  // \t = HH:MM:SS
		case PromptFolder:
			parts = append(parts, `"\[\e[34m\]\W\[\e[0m\]"`)  // \W = current dir
		case PromptAWS:
			parts = append(parts, `"${_rw_aws}"`)
		case PromptK8s:
			parts = append(parts, `"${_rw_k8s}"`)
		case PromptGit:
			parts = append(parts, `"${_rw_git}"`)
		}
	}

	promptExpr := strings.Join(parts, `" "`)

	return fmt.Sprintf(`
%s
# Shell prompt managed by rw - do not edit manually
_rw_prompt_info() {
  _rw_aws=""
  _rw_k8s=""
  _rw_git=""

  # AWS profile
  local aws_profile="${AWS_PROFILE:-$(aws configure get profile 2>/dev/null)}"
  if [[ -n "$aws_profile" ]]; then
    _rw_aws="\[\e[33m\]☁ ${aws_profile}\[\e[0m\]"
  fi

  # Kubernetes context/namespace
  local k8s_ctx
  k8s_ctx=$(kubectl config current-context 2>/dev/null)
  if [[ -n "$k8s_ctx" ]]; then
    k8s_ctx="${k8s_ctx##*/}"
    local k8s_ns
    k8s_ns=$(kubectl config view --minify --output 'jsonpath={..namespace}' 2>/dev/null)
    k8s_ns="${k8s_ns:-default}"
    _rw_k8s="\[\e[35m\]⎈ ${k8s_ctx}/${k8s_ns}\[\e[0m\]"
  fi

  # Git branch
  local git_branch
  git_branch=$(git symbolic-ref --short HEAD 2>/dev/null || git rev-parse --short HEAD 2>/dev/null)
  if [[ -n "$git_branch" ]]; then
    _rw_git="\[\e[32m\] ${git_branch}\[\e[0m\]"
  fi

  PS1="\n"%s"\n\[\e[37m\]❯\[\e[0m\] "
}

PROMPT_COMMAND="_rw_prompt_info${PROMPT_COMMAND:+;$PROMPT_COMMAND}"
%s
`, promptBlockStart, promptExpr, promptBlockEnd)
}

func (pm *PromptManager) generatePowerShellPrompt(components []PromptComponent) string {
	var parts []string
	for _, c := range components {
		switch c {
		case PromptTime:
			parts = append(parts, `Write-Host (Get-Date -Format "HH:mm:ss") -ForegroundColor Cyan -NoNewline`)
		case PromptFolder:
			parts = append(parts, `Write-Host (Split-Path -Leaf (Get-Location)) -ForegroundColor Blue -NoNewline`)
		case PromptAWS:
			parts = append(parts, `$awsProfile = $env:AWS_PROFILE; if ($awsProfile) { Write-Host "☁ $awsProfile" -ForegroundColor Yellow -NoNewline }`)
		case PromptK8s:
			parts = append(parts, `$k8sCtx = kubectl config current-context 2>$null; if ($k8sCtx) { $k8sCtx = ($k8sCtx -split '/')[-1]; $k8sNs = kubectl config view --minify -o 'jsonpath={..namespace}' 2>$null; if (-not $k8sNs) { $k8sNs = "default" }; Write-Host "⎈ $k8sCtx/$k8sNs" -ForegroundColor Magenta -NoNewline }`)
		case PromptGit:
			parts = append(parts, `$gitBranch = git symbolic-ref --short HEAD 2>$null; if ($gitBranch) { Write-Host " $gitBranch" -ForegroundColor Green -NoNewline }`)
		}
	}

	// Add space separators between parts
	var body string
	for i, p := range parts {
		if i > 0 {
			body += "    Write-Host ' ' -NoNewline\n"
		}
		body += "    " + p + "\n"
	}

	return fmt.Sprintf(`
%s
# Shell prompt managed by rw - do not edit manually
function prompt {
    Write-Host ""
%s    Write-Host ""
    Write-Host "❯ " -NoNewline -ForegroundColor White
    return " "
}
%s
`, promptBlockStart, body, promptBlockEnd)
}
