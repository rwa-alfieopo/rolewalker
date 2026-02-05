# Rolewalkers CLI Testing Commands (SND Environment)

All commands should be run from `rolewalkers/rolewalkers` directory using:
```bash
go run ./cmd/rwcli <command>
```

---

## 1. Help & Basic Commands

### Show Help
```bash
go run ./cmd/rwcli help
```
**Expected Output:** Full help text with all available commands and examples

### Generate API Keys
```bash
go run ./cmd/rwcli keygen
```
**Expected Output:** Single 32-character hex string (e.g., `a1b2c3d4e5f6789012345678abcdef01`)

```bash
go run ./cmd/rwcli keygen 3
```
**Expected Output:** Three 32-character hex strings, one per line

---

## 2. AWS Profile Management

### List Profiles
```bash
go run ./cmd/rwcli list
```
**Expected Output:**
```
AWS Profiles:
--------------------------------------------------------------------------------
  zenith-snd [ACTIVE] (SSO: logged in)
    Region: ap-southeast-2
    Account: 123456789012 | Role: AdministratorAccess
  zenith-dev (SSO: not logged in)
    ...
```

### Show Current Profile
```bash
go run ./cmd/rwcli current
```
**Expected Output:**
```
Active Profile: zenith-snd
Default Region: ap-southeast-2
AWS_PROFILE env: zenith-snd
```

### Switch Profile
```bash
go run ./cmd/rwcli switch zenith-snd
```
**Expected Output:**
```
✓ Switched to profile: zenith-snd

To update your current shell session, run:
  PowerShell: $env:AWS_PROFILE = 'zenith-snd'
  Bash/Zsh:   export AWS_PROFILE='zenith-snd'
```

### Switch Profile with Kubernetes Context
```bash
go run ./cmd/rwcli switch zenith-snd --kube
```
**Expected Output:**
```
✓ Switched to profile: zenith-snd
✓ Switched kubectl context: arn:aws:eks:ap-southeast-2:123456789012:cluster/zenith-snd
...
```

---

## 3. SSO Authentication

### SSO Login
```bash
go run ./cmd/rwcli login zenith-snd
```
**Expected Output:**
```
Initiating SSO login for profile: zenith-snd
A browser window will open for authentication...
✓ Successfully logged in to: zenith-snd
```

### SSO Logout
```bash
go run ./cmd/rwcli logout zenith-snd
```
**Expected Output:**
```
✓ Logged out from: zenith-snd
```

### SSO Status
```bash
go run ./cmd/rwcli status
```
**Expected Output:**
```
SSO Profile Status:
------------------------------------------------------------
  zenith-snd [ACTIVE]: ✓ Logged in (expires: 14:30:00)
  zenith-dev: ✗ Not logged in
  ...
```

---

## 4. Kubernetes Context Management

### List Kubernetes Contexts
```bash
go run ./cmd/rwcli kube list
```
**Expected Output:**
```
Available kubectl contexts:
  * arn:aws:eks:ap-southeast-2:123456789012:cluster/zenith-snd (current)
    arn:aws:eks:ap-southeast-2:123456789012:cluster/zenith-dev
    ...
```

### Switch Kubernetes Context
```bash
go run ./cmd/rwcli kube snd
```
**Expected Output:**
```
✓ Switched kubectl context: arn:aws:eks:ap-southeast-2:123456789012:cluster/zenith-snd
```

---

## 5. Port Configuration

### List All Port Mappings
```bash
go run ./cmd/rwcli port --list
```
**Expected Output:**
```
Port Mappings:
Service      | snd   | dev   | sit   | preprod | prod
-------------|-------|-------|-------|---------|------
db           | 15432 | 25432 | 35432 | 45432   | 55432
redis        | 16379 | 26379 | 36379 | 46379   | 56379
...
```

### Get Specific Port
```bash
go run ./cmd/rwcli port db snd
```
**Expected Output:** `15432`

```bash
go run ./cmd/rwcli port redis snd
```
**Expected Output:** `16379`

---

## 6. SSM Parameter Store

### Get SSM Parameter
```bash
go run ./cmd/rwcli ssm get /snd/zenith/database/query/db-write-endpoint
```
**Expected Output:** The parameter value (e.g., database endpoint hostname)

### List SSM Parameters
```bash
go run ./cmd/rwcli ssm list /snd/zenith/
```
**Expected Output:**
```
Parameters under /snd/zenith/:
  /snd/zenith/database/query/db-write-endpoint
  /snd/zenith/database/query/db-read-endpoint
  /snd/zenith/redis/cluster-endpoint
  ...
```

---

## 7. Tunnel Management

### Start Database Tunnel
```bash
go run ./cmd/rwcli tunnel start db snd
```
**Expected Output:**
```
✓ Starting tunnel to db in snd...
  Local port: 15432
  Target: zenith-snd-db.cluster-xxx.ap-southeast-2.rds.amazonaws.com:5432
✓ Tunnel established
```

### Start Redis Tunnel
```bash
go run ./cmd/rwcli tunnel start redis snd
```
**Expected Output:**
```
✓ Starting tunnel to redis in snd...
  Local port: 16379
  Target: zenith-snd-redis.xxx.cache.amazonaws.com:6379
✓ Tunnel established
```

### List Active Tunnels
```bash
go run ./cmd/rwcli tunnel list
```
**Expected Output:**
```
Active Tunnels:
  db (snd)     - localhost:15432 → zenith-snd-db:5432 [PID: 12345]
  redis (snd)  - localhost:16379 → zenith-snd-redis:6379 [PID: 12346]
```

### Stop Specific Tunnel
```bash
go run ./cmd/rwcli tunnel stop db snd
```
**Expected Output:**
```
✓ Stopped tunnel: db (snd)
```

### Stop All Tunnels
```bash
go run ./cmd/rwcli tunnel stop --all
```
**Expected Output:**
```
✓ Stopped all tunnels (2 tunnels terminated)
```

### Cleanup Stale Tunnels
```bash
go run ./cmd/rwcli tunnel cleanup
```
**Expected Output:**
```
✓ Cleaned up 0 stale tunnel entries
```

---

## 8. Database Operations

### Connect to Database (Read Node - Query DB)
```bash
go run ./cmd/rwcli db connect snd
```
**Expected Output:**
```
Connecting to snd query database (read node)...
psql (15.x)
Type "help" for help.

zenith_query=>
```

### Connect to Database (Write Node)
```bash
go run ./cmd/rwcli db connect snd --write
```
**Expected Output:**
```
Connecting to snd query database (write node)...
psql (15.x)
...
```

### Connect to Command Database
```bash
go run ./cmd/rwcli db connect snd --command
```
**Expected Output:**
```
Connecting to snd command database (read node)...
psql (15.x)
...
```

### Backup Database
```bash
go run ./cmd/rwcli db backup snd --output ./snd-backup.sql
```
**Expected Output:**
```
Starting backup of snd query database...
✓ Backup completed: ./snd-backup.sql (15.2 MB)
```

### Backup Schema Only
```bash
go run ./cmd/rwcli db backup snd --output ./snd-schema.sql --schema-only
```
**Expected Output:**
```
Starting schema backup of snd query database...
✓ Backup completed: ./snd-schema.sql (256 KB)
```

### Restore Database
```bash
go run ./cmd/rwcli db restore snd --input ./snd-backup.sql
```
**Expected Output:**
```
⚠ WARNING: This will restore data to snd from ./snd-backup.sql
Are you sure? (yes/no): yes
Restoring to snd query database...
✓ Restore completed successfully
```

### Restore with Auto-Confirm
```bash
go run ./cmd/rwcli db restore snd --input ./snd-backup.sql --clean --yes
```
**Expected Output:**
```
Restoring to snd query database (clean mode)...
✓ Restore completed successfully
```

---

## 9. Redis Operations

### Connect to Redis
```bash
go run ./cmd/rwcli redis connect snd
```
**Expected Output:**
```
Connecting to snd Redis cluster...
zenith-snd-redis.xxx.cache.amazonaws.com:6379>
```

---

## 10. gRPC Port Forwarding

### List gRPC Services
```bash
go run ./cmd/rwcli grpc list
```
**Expected Output:**
```
Available gRPC Services:
  candidate    - localhost:5001 → candidate-microservice-grpc
  job          - localhost:5002 → job-microservice-grpc
  client       - localhost:5003 → client-microservice-grpc
  organisation - localhost:5004 → organisation-microservice-grpc
  user         - localhost:5005 → user-microservice-grpc
  email        - localhost:5006 → email-microservice-grpc
  billing      - localhost:5007 → billing-microservice-grpc
  core         - localhost:5008 → core-microservice-grpc
```

### Forward gRPC Service
```bash
go run ./cmd/rwcli grpc candidate snd
```
**Expected Output:**
```
✓ Port forwarding candidate-microservice-grpc in snd
  localhost:5001 → candidate-microservice-grpc:50051
Press Ctrl+C to stop...
```

---

## 11. MSK (Kafka) Operations

### Start Kafka UI
```bash
go run ./cmd/rwcli msk ui snd
```
**Expected Output:**
```
Starting Kafka UI for snd MSK cluster...
✓ Kafka UI available at: http://localhost:8080
Press Ctrl+C to stop...
```

### Start Kafka UI on Custom Port
```bash
go run ./cmd/rwcli msk ui snd --port 9090
```
**Expected Output:**
```
Starting Kafka UI for snd MSK cluster...
✓ Kafka UI available at: http://localhost:9090
Press Ctrl+C to stop...
```

### Stop Kafka UI
```bash
go run ./cmd/rwcli msk stop snd
```
**Expected Output:**
```
✓ Stopped Kafka UI pod for snd
```

---

## 12. Maintenance Mode (Requires FASTLY_API_TOKEN)

### Check Maintenance Status
```bash
go run ./cmd/rwcli maintenance status snd
```
**Expected Output:**
```
Maintenance Mode Status for snd:
--------------------------------------------------
  API (zenith-snd-api): ✗ Disabled
  PWA (zenith-snd-pwa): ✗ Disabled
```

### Enable API Maintenance
```bash
go run ./cmd/rwcli maintenance snd --type api --enable
```
**Expected Output:**
```
✓ Enabled maintenance mode for API in snd
```

### Disable All Maintenance
```bash
go run ./cmd/rwcli maintenance snd --type all --disable
```
**Expected Output:**
```
✓ Disabled maintenance mode for API in snd
✓ Disabled maintenance mode for PWA in snd
```

---

## 13. HPA Scaling

### List HPAs
```bash
go run ./cmd/rwcli scale list snd
```
**Expected Output:**
```
HPAs in snd:
NAME                          MIN   MAX   CURRENT   DESIRED
candidate-microservice        2     10    3         3
job-microservice              2     10    2         2
client-microservice           2     10    2         2
...
```

### Scale with Preset
```bash
go run ./cmd/rwcli scale snd --preset performance
```
**Expected Output:**
```
Scaling HPAs in snd to performance preset (min: 10, max: 50)...
✓ Scaled candidate-microservice: 10/50
✓ Scaled job-microservice: 10/50
✓ Scaled client-microservice: 10/50
...
✓ All HPAs scaled successfully
```

### Scale Specific Service
```bash
go run ./cmd/rwcli scale snd --service candidate --min 5 --max 15
```
**Expected Output:**
```
✓ Scaled candidate-microservice in snd: min=5, max=15
```

---

## 14. Blue-Green Replication

### Show Replication Status
```bash
go run ./cmd/rwcli replication status snd
```
**Expected Output:**
```
Blue-Green Deployments in snd:
--------------------------------------------------------------------------------
ID: bgd-abc123def456
  Name: zenith-snd-blue-green
  Status: AVAILABLE
  Source: zenith-snd-db-cluster (AVAILABLE)
  Target: zenith-snd-db-cluster-green (AVAILABLE)
  Created: 2024-01-15 10:30:00
```

### Create Blue-Green Deployment
```bash
go run ./cmd/rwcli replication create snd --name test-bg --source zenith-snd-db-cluster
```
**Expected Output:**
```
⚠ This will create a Blue-Green deployment:
  Name: test-bg
  Source: zenith-snd-db-cluster
Are you sure? (yes/no): yes
Creating Blue-Green deployment...
✓ Created deployment: bgd-xyz789abc123
  Status: PROVISIONING
  Monitor with: rwcli replication status snd
```

### Switchover Deployment
```bash
go run ./cmd/rwcli replication switch bgd-abc123def456 --yes
```
**Expected Output:**
```
Initiating switchover for bgd-abc123def456...
✓ Switchover initiated
  Status: SWITCHOVER_IN_PROGRESS
  Monitor with: rwcli replication status snd
```

### Delete Deployment
```bash
go run ./cmd/rwcli replication delete bgd-abc123def456 --yes
```
**Expected Output:**
```
Deleting Blue-Green deployment bgd-abc123def456...
✓ Deployment deleted successfully
```

### Delete with Target Cluster
```bash
go run ./cmd/rwcli replication delete bgd-abc123def456 --delete-target --yes
```
**Expected Output:**
```
Deleting Blue-Green deployment bgd-abc123def456 (including target cluster)...
✓ Deployment and target cluster deleted successfully
```

---

## Quick Test Sequence (SND Only)

Run these commands in order to verify basic functionality:

```bash
# 1. Basic commands
go run ./cmd/rwcli help
go run ./cmd/rwcli keygen

# 2. Profile management
go run ./cmd/rwcli list
go run ./cmd/rwcli current
go run ./cmd/rwcli switch zenith-snd

# 3. SSO (requires browser)
go run ./cmd/rwcli login zenith-snd
go run ./cmd/rwcli status

# 4. Kubernetes
go run ./cmd/rwcli kube list
go run ./cmd/rwcli kube snd

# 5. Ports
go run ./cmd/rwcli port --list
go run ./cmd/rwcli port db snd

# 6. SSM
go run ./cmd/rwcli ssm list /snd/zenith/

# 7. Tunnels
go run ./cmd/rwcli tunnel start db snd
go run ./cmd/rwcli tunnel list
go run ./cmd/rwcli tunnel stop --all

# 8. Scaling
go run ./cmd/rwcli scale list snd

# 9. Replication
go run ./cmd/rwcli replication status snd
```

---

## Error Scenarios

### Invalid Environment
```bash
go run ./cmd/rwcli db connect invalid-env
```
**Expected Output:** `Error: unknown environment: invalid-env`

### Missing Required Arguments
```bash
go run ./cmd/rwcli db backup snd
```
**Expected Output:** `Error: --output is required`

### Unknown Command
```bash
go run ./cmd/rwcli unknown
```
**Expected Output:** `Error: unknown command: unknown`

### SSM Parameter Not Found
```bash
go run ./cmd/rwcli ssm get /nonexistent/path
```
**Expected Output:** `Error: parameter not found: /nonexistent/path`
