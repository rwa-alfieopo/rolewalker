# Rolewalkers Integration Prompts

These are ready-to-use prompts for Kiro to implement each integration from zenith-tools-infra.

---

## 1. API Key Generator

**Difficulty:** Trivial | **Estimated Time:** 5 minutes

```
Add a new CLI command `rwcli keygen` that generates API keys in UUID-like format.

Requirements:
- Add a "keygen" command to cli/cli.go
- Generate keys in format: xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx (hex tokens)
- Accept optional count argument: `rwcli keygen 5` generates 5 keys
- Use crypto/rand for secure random generation
- Output one key per line

Example usage:
  rwcli keygen        # Generate 1 key
  rwcli keygen 3      # Generate 3 keys

Reference format from zenith-tools-infra:
  secrets.token_hex(4) + '-' + secrets.token_hex(2) + '-' + secrets.token_hex(2) + '-' + secrets.token_hex(2) + '-' + secrets.token_hex(6)
```

---

## 2. SSM Parameter Lookup

**Difficulty:** Easy | **Estimated Time:** 15 minutes

```
Add a new CLI command `rwcli ssm` to fetch AWS SSM parameters.

Requirements:
- Add "ssm" command with subcommands: get, list
- `rwcli ssm get <parameter-name>` - fetch and display a parameter value
- `rwcli ssm get <parameter-name> --decrypt` - fetch with decryption (default: true)
- `rwcli ssm list <path-prefix>` - list parameters under a path
- Use AWS SDK for Go v2 (already in go.mod)
- Respect the current AWS profile set by rolewalkers
- Handle errors gracefully (parameter not found, access denied, etc.)

Example usage:
  rwcli ssm get /dev/zenith/database/query/db-write-endpoint
  rwcli ssm list /dev/zenith/
  rwcli ssm get /prod/zenith/redis/cluster-endpoint --decrypt

Create new file: aws/ssm.go for SSM operations
```

---

## 3. Environment Port Mapping

**Difficulty:** Easy | **Estimated Time:** 10 minutes

```
Add a new CLI command `rwcli port` that returns standard local ports for services per environment.

Requirements:
- Add "port" command to cli/cli.go
- Create a port mapping configuration in aws/ports.go
- Support services: db, redis, elasticsearch, kafka, rabbitmq, grpc
- Support environments: snd, dev, sit, preprod, trg, prod, qa, stage

Port mappings (from zenith-tools-infra):
  Database:
    snd=5432, dev=5433, sit=5434, preprod=5435/5436, trg=5437, prod=5438/5439, qa=5440/5441, stage=5442/5443
  Redis:
    snd=6379, dev=6380, sit=6381, preprod=6382, trg=6383, prod=6384, qa=6385, stage=6386
  Elasticsearch:
    default=9200

Example usage:
  rwcli port db dev          # Output: 5433
  rwcli port redis prod      # Output: 6384
  rwcli port --list          # Show all port mappings
```

---

## 4. Kubernetes Context Sync

**Difficulty:** Medium | **Estimated Time:** 30 minutes

```
Extend the profile switch command to optionally sync Kubernetes context.

Requirements:
- Add --kube flag to `rwcli switch` command
- When --kube is set, also switch kubectl context to matching EKS cluster
- Detect EKS cluster name pattern: {env}-zenith-eks-cluster
- Use exec to run `kubectl config use-context` 
- Add `rwcli kube` command to manually switch k8s context
- Add `rwcli kube list` to show available contexts

Context naming patterns from zenith-tools-infra:
  - ARN format: arn:aws:eks:eu-west-2:{account}:cluster/{env}-zenith-eks-cluster
  - Simple format: zenith-{env}

Example usage:
  rwcli switch zenith-dev --kube    # Switch AWS profile AND k8s context
  rwcli kube dev                    # Switch only k8s context
  rwcli kube list                   # List available k8s contexts

Create new file: aws/kubernetes.go for k8s context operations
```

---

## 5. Tunnel Management

**Difficulty:** Medium-High | **Estimated Time:** 1 hour

```
Add tunnel management commands to create secure tunnels to AWS resources via Kubernetes.

Requirements:
- Add `rwcli tunnel` command with subcommands: start, stop, list
- Support tunnel types: db, redis, elasticsearch, kafka, msk, rabbitmq, grpc
- Create tunnels using kubectl to spawn socat pods in tunnel-access namespace
- Auto-assign local ports based on environment (use port mapping from integration #3)
- Track active tunnels in a local state file (~/.rolewalkers/tunnels.json)
- Handle cleanup on interrupt (Ctrl+C)

Commands:
  rwcli tunnel start db dev       # Start database tunnel to dev
  rwcli tunnel start redis prod   # Start redis tunnel to prod  
  rwcli tunnel stop db dev        # Stop specific tunnel
  rwcli tunnel stop --all         # Stop all tunnels
  rwcli tunnel list               # Show active tunnels

Implementation details from zenith-tools-infra:
- Pod naming: {service}tunnel-{username}-{random}
- Image for socat: alpine/socat
- Command: socat tcp-listen:{port},fork,reuseaddr tcp:{host}:{port}
- Fetch endpoints from SSM: /{env}/zenith/{service}/cluster-endpoint

Create new files:
- aws/tunnel.go - tunnel operations
- aws/tunnel_state.go - state management

Dependencies:
- Requires integration #2 (SSM) for endpoint lookup
- Requires integration #3 (ports) for port mapping
```

---

## Implementation Order

Recommended sequence for implementing these integrations:

1. **keygen** - Standalone, no dependencies, quick win
2. **port** - Standalone, needed by tunnel
3. **ssm** - Standalone, needed by tunnel
4. **kube** - Enhances existing switch command
5. **tunnel** - Depends on ssm and port

Each prompt above is self-contained and can be given directly to Kiro.
