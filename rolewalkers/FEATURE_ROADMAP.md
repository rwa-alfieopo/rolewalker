# Rolewalkers Feature Roadmap

Features from zenith-tools-infra that can be integrated into rolewalkers, ranked from easiest to hardest.

---

## Already Implemented ✅

- Profile switching with `--kube` flag
- Kubernetes context management (`kube list`, `kube <env>`)
- Port mapping (`port <service> <env>`, `port --list`)
- Tunnel management (`tunnel start/stop/list`)

---

## Features to Implement

### 1. API Key Generator
**Difficulty:** Trivial | **Time:** ~5 min

Simple UUID-like key generation using `crypto/rand`. No external dependencies.

```
rwcli keygen        # Generate 1 key
rwcli keygen 5      # Generate 5 keys
```

Reference: `zenith-tools-infra/scripts/generate-api-key`

---

### 2. SSM Parameter Lookup
**Difficulty:** Easy | **Time:** ~15 min

Fetch AWS SSM parameters. AWS SDK already in go.mod.

```
rwcli ssm get /dev/zenith/database/query/db-write-endpoint
rwcli ssm list /dev/zenith/
rwcli ssm get /prod/zenith/redis/cluster-endpoint --decrypt
```

Reference: Used throughout all tunnel scripts

---

### 3. gRPC Service Tunnel
**Difficulty:** Easy | **Time:** ~15 min

Direct port-forward to k8s services (no socat pod needed).

```
rwcli grpc candidate dev    # Forward localhost:5001 to candidate-microservice-grpc
rwcli grpc job prod         # Forward localhost:5002 to job-microservice-grpc
```

Service ports: candidate=5001, job=5002, client=5003, organisation=5004, user=5006, email=5007, billing=5074, core=5020

Reference: `zenith-tools-infra/scripts/grpc-rw-tunnel`

---

### 4. Database Direct Connect
**Difficulty:** Medium | **Time:** ~30 min

Spawn psql pod in cluster for interactive database access.

```
rwcli db connect dev              # Connect to dev database
rwcli db connect prod --write     # Connect to prod write node
rwcli db connect prod --command   # Connect to command database
```

Reference: `zenith-tools-infra/scripts/db-rw-connect`

---

### 5. Redis Direct Connect
**Difficulty:** Medium | **Time:** ~30 min

Spawn redis-cli pod for interactive Redis access.

```
rwcli redis connect dev     # Connect to dev Redis cluster
rwcli redis connect prod    # Connect to prod Redis cluster
```

Reference: `zenith-tools-infra/scripts/redis-cluster-connect`

---

### 6. MSK/Kafka UI Tunnel
**Difficulty:** Medium | **Time:** ~45 min

Deploy Kafka UI pod with IAM auth for MSK cluster access.

```
rwcli msk ui dev            # Start Kafka UI on localhost:8080
rwcli msk ui prod --port 9090
```

Reference: `zenith-tools-infra/scripts/msk-rw-tunnel`

---

### 7. Maintenance Mode Toggle
**Difficulty:** Medium-High | **Time:** ~1 hour

Toggle maintenance mode via Fastly API.

```
rwcli maintenance --env dev --type api --enable
rwcli maintenance --env prod --type pwa --disable
rwcli maintenance --env prod --type all --enable
```

Reference: `zenith-tools-infra/scripts/maintenance-mode`

---

### 8. HPA Scaling Commands
**Difficulty:** Medium-High | **Time:** ~1 hour

Patch HPA resources for performance testing or scaling.

```
rwcli scale preprod --preset performance
rwcli scale prod --preset normal
rwcli scale dev --service candidate --min 5 --max 10
```

Reference: `zenith-tools-infra/scripts/scale-preprod`, `scale-prod`

---

### 9. Database Backup/Restore
**Difficulty:** Hard | **Time:** ~2 hours

Orchestrate pg_dump/pg_restore operations.

```
rwcli db backup dev --output ./backup.sql
rwcli db restore dev --input ./backup.sql
```

Reference: `zenith-tools-infra/scripts/db-rw-backup`, `db-rw-restore`

---

### 10. Blue-Green Replication Manager
**Difficulty:** Hard | **Time:** ~3+ hours

Complex database replication management with multiple AWS API calls.

```
rwcli replication status dev
rwcli replication switch dev --target blue
```

Reference: `zenith-tools-infra/scripts/blue-green-replication.py`

---

## Recommended Implementation Order

1. **keygen** - Zero dependencies, instant value
2. **ssm** - Foundation for other features
3. **grpc** - Simple port-forward, no pod creation
4. **db connect** - High value for developers
5. **redis connect** - Similar pattern to db connect
6. **msk ui** - More complex pod spec
7. **maintenance** - Requires external API integration
8. **scale** - Kubectl patching
9. **db backup/restore** - Complex orchestration
10. **replication** - Most complex, multiple AWS services
