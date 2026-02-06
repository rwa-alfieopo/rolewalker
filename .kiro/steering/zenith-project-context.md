---
inclusion: auto
---

# Zenith Project Context

This steering file provides context about the Zenith project and its AWS/Kubernetes configuration behavior.

## Project Overview

This is the **rolewalkers (rwcli)** tool - an AWS Profile & SSO Manager specifically designed for the Zenith project infrastructure.

## Automatic Kubernetes Context Switching

**IMPORTANT BEHAVIOR**: When switching AWS accounts/profiles, the tool automatically:
1. **Updates the kubeconfig** by running `aws eks update-kubeconfig` to fetch the latest cluster configuration
2. **Switches the kubectl context** to match the environment

This ensures you always have the latest kubeconfig aligned with your logged-in AWS account.

### How It Works

1. When you run `rwcli switch <profile>` or `rwcli login <profile>`, the tool:
   - Switches the AWS profile configuration
   - **Automatically runs `aws eks update-kubeconfig`** to fetch/update the cluster configuration
   - **Automatically switches the kubectl context** to match the environment
   - Updates both AWS and Kubernetes configurations in a single command

2. To skip automatic Kubernetes context switching, use:
   ```bash
   rwcli switch <profile> --no-kube
   ```

### Workflow Example

```bash
# Step 1: Switch AWS profile
rwcli switch zenith-dev

# Behind the scenes, the tool automatically:
# 1. Switches AWS profile to zenith-dev
# 2. Runs: aws eks update-kubeconfig --name dev-zenith-eks-cluster --region eu-west-2
# 3. Switches kubectl context to the updated cluster

# You're now ready to use kubectl with the correct cluster!
kubectl get pods
```

### Environment Mapping

The tool maps AWS profiles to Kubernetes clusters using this convention:

| AWS Profile       | Kubernetes Cluster           | Environment |
|-------------------|------------------------------|-------------|
| zenith-qa         | qa-zenith-eks-cluster        | QA          |
| zenith-dev        | dev-zenith-eks-cluster       | Development |
| zenith-live       | prod-zenith-eks-cluster      | Production  |
| zenith-sandbox    | snd-zenith-eks-cluster       | Sandbox     |
| zenith-staging    | stage-zenith-eks-cluster     | Staging     |

### Kubernetes Context Format

Kubernetes contexts are stored in ARN format:
```
arn:aws:eks:<region>:<account-id>:cluster/<cluster-name>
```

Example:
```
arn:aws:eks:eu-west-2:226075141250:cluster/snd-zenith-eks-cluster
```

## Code Implementation Notes

### Key Files

- **cli/cli.go**: Contains the `switchProfile()` function that handles both AWS and Kubernetes switching
- **aws/kubernetes.go**: Contains `KubeManager` with methods:
  - `SwitchContextForEnv()`: Automatically finds and switches to the matching Kubernetes context
  - `FindContextForEnv()`: Maps AWS profile names to Kubernetes cluster names
  - `GetCurrentContext()`: Gets the current kubectl context
  - `GetCurrentNamespace()`: Gets the current kubectl namespace

### Profile Switching Logic

```go
func (c *CLI) switchProfile(profileName string, skipKube bool) error {
    // 1. Switch AWS profile
    if err := c.profileSwitcher.SwitchProfile(profileName); err != nil {
        return err
    }

    // 2. Automatically update kubeconfig and switch context (unless --no-kube flag is used)
    if !skipKube {
        if err := c.kubeManager.SwitchContextForEnv(profileName); err != nil {
            // Non-fatal: warn but don't fail
            fmt.Printf("⚠ Failed to switch kubectl context: %v\n", err)
        } else {
            ctx, _ := c.kubeManager.GetCurrentContext()
            fmt.Printf("✓ Switched kubectl context: %s\n", ctx)
        }
    }

    return nil
}
```

### Kubeconfig Update Logic

The `SwitchContextForEnv` method in `KubeManager`:
1. Tries to find an existing kubectl context for the environment
2. If not found, automatically runs `aws eks update-kubeconfig` to fetch the cluster configuration
3. Switches to the updated context

```go
func (km *KubeManager) SwitchContextForEnv(env string) error {
    // Try to find existing context
    contextName, err := km.FindContextForEnv(env)
    if err != nil {
        // Context not found, update kubeconfig from AWS
        if updateErr := km.UpdateKubeconfig(clusterName, region); updateErr != nil {
            return fmt.Errorf("failed to update kubeconfig: %w", updateErr)
        }
        
        // Try to find context again after update
        contextName, err = km.FindContextForEnv(env)
        if err != nil {
            return fmt.Errorf("context still not found: %w", err)
        }
    }

    return km.SwitchContext(contextName)
}
```

## Zenith-Specific Configuration

### AWS SSO Configuration

All Zenith profiles use AWS SSO with:
- **SSO Start URL**: `https://d-9c67711d98.awsapps.com/start/`
- **SSO Region**: `eu-west-2`
- **Role**: `AdministratorAccess`

### Region

All Zenith environments use:
- **Default Region**: `eu-west-2` (Europe - London)

## Usage Examples

### Switch with automatic Kubernetes context update (default)
```bash
rwcli switch zenith-dev
# Output:
# ✓ Switched to profile: zenith-dev
# ✓ Switched kubectl context: arn:aws:eks:eu-west-2:611914608941:cluster/dev-zenith-eks-cluster
```

### Switch AWS only (skip Kubernetes)
```bash
rwcli switch zenith-dev --no-kube
# Output:
# ✓ Switched to profile: zenith-dev
```

### Check current context
```bash
rwcli current
# Output:
# Current Context:
# ------------------------------------------------------------
# AWS Profile:     zenith-dev
# AWS Region:      eu-west-2
# Account ID:      611914608941
# Account Name:    Dev
# Kube Cluster:    arn:aws:eks:eu-west-2:611914608941:cluster/dev-zenith-eks-cluster
# Kube Namespace:  default
```

## Important Notes for Development

1. **This behavior is Zenith-specific**: The automatic Kubernetes switching is designed specifically for the Zenith project's infrastructure setup.

2. **Cluster naming convention**: The tool expects clusters to follow the pattern: `<env>-zenith-eks-cluster`

3. **Non-fatal Kubernetes errors**: If Kubernetes context switching fails, the tool will warn but won't fail the AWS profile switch. This ensures AWS operations can continue even if kubectl is not configured.

4. **Environment variable precedence**: The `AWS_PROFILE` environment variable will override the config file setting. The tool warns about this when detected.

5. **SSO URL format**: The SSO start URL must NOT include the `/#` fragment that appears in browser URLs. Use only: `https://d-9c67711d98.awsapps.com/start/`

## Troubleshooting

### Kubernetes context not switching
- Ensure `kubectl` is installed and configured
- Verify you have access to the EKS cluster
- Check that the cluster name follows the expected pattern
- Use `rwcli kube list` to see available contexts

### AWS profile switch works but kubectl fails
- This is expected behavior if kubectl is not configured
- The tool will show a warning but continue
- Configure kubectl access separately if needed

### SSO login fails
- Ensure the SSO start URL doesn't have `/#` at the end
- Clear SSO cache: `rm -rf ~/.aws/sso/cache/*.json` (except kiro-auth-token.json)
- Update AWS CLI: `brew upgrade awscli`
- Verify the SSO start URL is correct in `~/.aws/config`
