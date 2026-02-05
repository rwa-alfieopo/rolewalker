# Rolewalkers Implementation Verification Prompt

Verify the implementation status of all 10 features in the rolewalkers CLI project. For each feature, check if the required files exist, the code is properly integrated, and the functionality works as expected.

## Verification Instructions

For each feature below:
1. Check if the required files exist
2. Verify the CLI command is registered in `cli/cli.go`
3. Confirm the help text is updated in `showHelp()`
4. Test the command works using `go run ./cmd/rwcli <command>` (use `snd` environment only)

---

## Features to Verify

### 1. API Key Generator (keygen)

**Check:**
- [ ] `cli/cli.go` has `keygen` case in Run() switch
- [ ] `keygen()` method exists and uses `crypto/rand`
- [ ] Help text includes keygen command

**Test:**
```bash
cd rolewalkers/rolewalkers
go run ./cmd/rwcli keygen
go run ./cmd/rwcli keygen 3
```

---

### 2. SSM Parameter Lookup

**Check:**
- [ ] `cli/cli.go` has `ssm` case with `get` and `list` subcommands
- [ ] `aws/ssm.go` has `ListParameters()` method
- [ ] Help text includes ssm commands

**Test:**
```bash
go run ./cmd/rwcli ssm get /snd/zenith/database/query/db-write-endpoint
go run ./cmd/rwcli ssm list /snd/zenith/
```

---

### 3. gRPC Service Tunnel

**Check:**
- [ ] `aws/grpc.go` exists with `GRPCPorts` map
- [ ] `cli/cli.go` has `grpc` case
- [ ] Port mappings: candidate=5001, job=5002, client=5003, organisation=5004, user=5006, email=5007, billing=5074, core=5020
- [ ] Help text includes grpc commands

**Test:**
```bash
go run ./cmd/rwcli grpc list
go run ./cmd/rwcli grpc candidate snd
```

---

### 4. Database Direct Connect

**Check:**
- [ ] `aws/database.go` exists with `DatabaseManager` struct
- [ ] `Connect()` method spawns psql pod
- [ ] `cli/cli.go` has `db connect` subcommand
- [ ] Supports `--write` and `--command` flags
- [ ] Help text includes db connect

**Test:**
```bash
go run ./cmd/rwcli db connect snd
```

---

### 5. Redis Direct Connect

**Check:**
- [ ] `aws/redis.go` exists with `RedisManager` struct
- [ ] `Connect()` method spawns redis-cli pod with `-c` (cluster) and `--tls` flags
- [ ] `cli/cli.go` has `redis connect` subcommand
- [ ] Help text includes redis connect

**Test:**
```bash
go run ./cmd/rwcli redis connect snd
```

---

### 6. MSK/Kafka UI Tunnel

**Check:**
- [ ] `aws/msk.go` exists with `MSKManager` struct
- [ ] `StartUI()` method creates Kafka UI pod with IAM auth environment variables
- [ ] `StopUI()` method deletes the pod
- [ ] `cli/cli.go` has `msk ui` and `msk stop` subcommands
- [ ] Supports `--port` flag
- [ ] Help text includes msk commands

**Test:**
```bash
go run ./cmd/rwcli msk ui snd
go run ./cmd/rwcli msk stop snd
```

---

### 7. Maintenance Mode Toggle

**Check:**
- [ ] `aws/maintenance.go` exists with `MaintenanceManager` struct
- [ ] `Toggle()` method uses Fastly API
- [ ] `Status()` method checks current state
- [ ] `cli/cli.go` has `maintenance` command with `--env`, `--type`, `--enable/--disable` flags
- [ ] Supports types: api, pwa, all
- [ ] Help text includes maintenance commands

**Test:**
```bash
go run ./cmd/rwcli maintenance status snd
```

---

### 8. HPA Scaling Commands

**Check:**
- [ ] `aws/scaling.go` exists with `ScalingManager` struct
- [ ] Presets defined: normal (2-10), performance (10-50), minimal (1-3)
- [ ] `Scale()` method patches HPAs
- [ ] `ScaleService()` method for single service
- [ ] `ListHPAs()` method shows current scaling
- [ ] `cli/cli.go` has `scale` command with `--preset`, `--service`, `--min`, `--max` flags
- [ ] Help text includes scale commands

**Test:**
```bash
go run ./cmd/rwcli scale list snd
```

---

### 9. Database Backup/Restore

**Check:**
- [ ] `aws/database.go` has `Backup()` and `Restore()` methods
- [ ] `cli/cli.go` has `db backup` and `db restore` subcommands
- [ ] Backup supports `--output` and `--schema-only` flags
- [ ] Restore supports `--input`, `--clean`, and `--yes` flags
- [ ] Restore has confirmation prompt (unless `--yes`)
- [ ] Help text includes db backup/restore

**Test:**
```bash
go run ./cmd/rwcli db backup snd --output ./test-backup.sql
```

---

### 10. Blue-Green Replication Manager

**Check:**
- [ ] `aws/replication.go` exists with `ReplicationManager` struct
- [ ] `Status()` method uses `aws rds describe-blue-green-deployments`
- [ ] `Switch()` method executes switchover with confirmation
- [ ] `Create()` method creates new deployments with confirmation
- [ ] `Delete()` method deletes deployments with optional `--delete-target` flag
- [ ] `cli/cli.go` has `replication` command with `status`, `switch`, `create`, `delete` subcommands
- [ ] Help text includes replication commands

**Expected Behavior (based on reference: zenith-tools-infra/scripts/blue-green-replication.py):**
- [ ] Status output shows: Deployment name, Identifier, Status, Source cluster, Target cluster, Created time, Tasks
- [ ] Status filters deployments by environment name (case-insensitive match in name or source)
- [ ] Switch requires deployment to be in `AVAILABLE` state before proceeding
- [ ] Switch monitors progress with 10-second polling until completion or 30-minute timeout
- [ ] Switch handles status transitions: `SWITCHOVER_IN_PROGRESS` → `SWITCHOVER_COMPLETED` or `SWITCHOVER_FAILED`
- [ ] Create builds source ARN automatically if only cluster identifier provided
- [ ] Create returns deployment identifier for tracking
- [ ] Delete supports `--delete-target` flag to also delete target cluster
- [ ] All destructive operations (switch, create, delete) require confirmation unless `--yes` flag provided
- [ ] Valid environments: snd, dev, sit, preprod, trg, prod, qa, stage
- [ ] Region hardcoded to `eu-west-2`

**Test:**
```bash
go run ./cmd/rwcli replication status snd
go run ./cmd/rwcli replication --help
```

---

## Output Format

After verification, update this file with the results:

| # | Feature | Status | Notes |
|---|---------|--------|-------|
| 1 | keygen | ✅ | Generates cryptographically secure API keys using crypto/rand |
| 2 | ssm | ✅ | get and list subcommands work, ListParameters() implemented |
| 3 | grpc | ✅ | All 8 services with correct ports (candidate=5001, job=5002, client=5003, organisation=5004, user=5006, email=5007, billing=5074, core=5020) |
| 4 | db connect | ✅ | Supports --write and --command flags, spawns psql pod |
| 5 | redis connect | ✅ | Uses -c (cluster) and --tls flags in redis-cli |
| 6 | msk ui | ✅ | StartUI/StopUI with IAM auth env vars, --port flag supported |
| 7 | maintenance | ✅ | Toggle and Status methods, supports api/pwa/all types |
| 8 | scale | ✅ | Presets (normal=2/10, performance=10/50, minimal=1/3), ScaleService, ListHPAs |
| 9 | db backup/restore | ✅ | Backup with --output/--schema-only, Restore with --input/--clean/--yes |
| 10 | replication | ✅ | status/switch/create/delete subcommands, all confirmations, 10s polling, 30min timeout |

For any feature marked ❌, list the specific missing components that need to be implemented.

---

## Verification Results (February 5, 2026)

All 10 features have been verified and are fully implemented:

### Feature Details

**1. keygen** - Tested with `go run ./cmd/rwcli keygen` and `go run ./cmd/rwcli keygen 3`. Generates 32-character hex keys using crypto/rand.

**2. ssm** - CLI has `ssm` case with `get` and `list` subcommands. `aws/ssm.go` has `ListParameters()` method using `get-parameters-by-path`.

**3. grpc** - `aws/grpc.go` has `GRPCPorts` map with all 8 services. Tested `go run ./cmd/rwcli grpc list` - shows all services with correct ports.

**4. db connect** - `aws/database.go` has `DatabaseManager` with `Connect()` method. CLI supports `--write` and `--command` flags.

**5. redis connect** - `aws/redis.go` has `RedisManager` with `Connect()` method. Uses `-c` (cluster mode) and `--tls` flags.

**6. msk ui** - `aws/msk.go` has `MSKManager` with `StartUI()` and `StopUI()` methods. IAM auth environment variables configured.

**7. maintenance** - `aws/maintenance.go` has `MaintenanceManager` with `Toggle()` and `Status()` methods. Supports api, pwa, all types.

**8. scale** - `aws/scaling.go` has `ScalingManager` with correct presets. `Scale()`, `ScaleService()`, and `ListHPAs()` all implemented.

**9. db backup/restore** - `aws/database.go` has `Backup()` and `Restore()` methods. All flags supported including confirmation prompt.

**10. replication** - `aws/replication.go` has `ReplicationManager` with all methods. Status filters by env, Switch monitors with 10s polling and 30min timeout, Create builds ARN automatically, Delete supports --delete-target.

---

## Important Rules

- Use `go run` for testing, NOT `go build`
- Only test with `snd` environment
- Do not modify any code during verification - only check and report
