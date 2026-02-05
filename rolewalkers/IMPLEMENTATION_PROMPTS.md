# Rolewalkers Feature Implementation Prompts

Use these prompts with Kiro to implement each feature. Copy and paste the prompt for the feature you want to implement.

---

## 1. API Key Generator (keygen)

**Prompt:**
```
Implement an API key generator command for rolewalkers CLI.

Requirements:
1. Add a new "keygen" command to cli/cli.go
2. Generate cryptographically secure random keys using crypto/rand
3. Keys should be 32-character hex strings (16 bytes)

Usage:
- rwcli keygen        # Generate 1 key
- rwcli keygen 5      # Generate 5 keys

Implementation steps:
1. Add "keygen" case to the switch statement in cli.go Run() method
2. Create a keygen() method that:
   - Parses optional count argument (default 1)
   - Uses crypto/rand.Read() to generate random bytes
   - Converts to hex string using hex.EncodeToString()
   - Prints each key on a new line
3. Update showHelp() to include keygen command

Keep it simple - no external dependencies needed. Just use standard library crypto/rand and encoding/hex.
```

---

## 2. SSM Parameter Lookup

**Prompt:**
```
Enhance the SSM functionality in rolewalkers CLI to support direct parameter lookup.

Requirements:
1. Add "ssm" command to cli/cli.go with subcommands: get, list
2. Use the existing aws/ssm.go SSMManager

Usage:
- rwcli ssm get /dev/zenith/database/query/db-write-endpoint
- rwcli ssm get /prod/zenith/redis/cluster-endpoint --decrypt
- rwcli ssm list /dev/zenith/

Implementation steps:
1. Add "ssm" case to cli.go Run() switch statement
2. Create ssm() method that handles subcommands:
   - "get <path>" - calls SSMManager.GetParameter()
   - "list <prefix>" - new method to list parameters by path prefix
3. Add ListParameters() method to aws/ssm.go:
   - Use "aws ssm get-parameters-by-path --path <prefix> --recursive"
   - Parse JSON response and display parameter names
4. The --decrypt flag is already handled by SSMManager (uses --with-decryption)
5. Update showHelp() with ssm commands

The SSMManager already exists in aws/ssm.go - just add the CLI integration and list functionality.
```

---

## 3. gRPC Service Tunnel

**Prompt:**
```
Add gRPC service port-forwarding to rolewalkers CLI.

Requirements:
1. Add "grpc" command to cli/cli.go
2. Use kubectl port-forward to forward to gRPC services in the cluster
3. Support multiple microservices with predefined ports

Service port mappings:
- candidate: 5001
- job: 5002
- client: 5003
- organisation: 5004
- user: 5006
- email: 5007
- billing: 5074
- core: 5020

Usage:
- rwcli grpc candidate dev    # Forward localhost:5001 to candidate-microservice-grpc
- rwcli grpc job prod         # Forward localhost:5002 to job-microservice-grpc
- rwcli grpc list             # List available gRPC services

Implementation steps:
1. Add grpcPorts map in aws/ports.go or a new aws/grpc.go file:
   var grpcPorts = map[string]int{
       "candidate": 5001, "job": 5002, "client": 5003,
       "organisation": 5004, "user": 5006, "email": 5007,
       "billing": 5074, "core": 5020,
   }
2. Add "grpc" case to cli.go Run() switch
3. Create grpc() method that:
   - Validates service name exists in grpcPorts
   - Ensures correct kubectl context (use kubeManager.SwitchContextForEnv)
   - Runs: kubectl port-forward svc/<service>-microservice-grpc <port>:<port> -n zenith
4. Run port-forward in background (similar to tunnel command)
5. Update showHelp()

Service naming pattern: <service>-microservice-grpc (e.g., candidate-microservice-grpc)
```

---

## 4. Database Direct Connect

**Prompt:**
```
Add database direct connect command to rolewalkers CLI.

Requirements:
1. Add "db connect" subcommand to cli/cli.go
2. Spawn a psql pod in the Kubernetes cluster for interactive database access
3. Support read/write nodes and query/command databases

Usage:
- rwcli db connect dev              # Connect to dev query database (read node)
- rwcli db connect prod --write     # Connect to prod write node
- rwcli db connect prod --command   # Connect to command database

Implementation steps:
1. Create aws/database.go with DatabaseManager struct
2. Add Connect() method that:
   - Gets database endpoint from SSM using SSMManager.GetDatabaseEndpoint()
   - Gets database credentials from SSM (password path: /<env>/zenith/database/<dbtype>/db-password)
   - Spawns a temporary pod with psql:
     kubectl run psql-temp-<random> --rm -it --restart=Never \
       --image=postgres:15-alpine \
       --env="PGPASSWORD=<password>" \
       -- psql -h <endpoint> -U zenith_app -d zenith
3. Add "db" case to cli.go with subcommand handling:
   - "connect" -> calls DatabaseManager.Connect()
   - Parse --write and --command flags
4. Update showHelp()

The pod runs interactively (--rm -it) and deletes itself when psql exits.
Database name: zenith
Username: zenith_app
```

---

## 5. Redis Direct Connect

**Prompt:**
```
Add Redis direct connect command to rolewalkers CLI.

Requirements:
1. Add "redis connect" subcommand to cli/cli.go
2. Spawn a redis-cli pod in the Kubernetes cluster for interactive Redis access
3. Get Redis endpoint from SSM

Usage:
- rwcli redis connect dev     # Connect to dev Redis cluster
- rwcli redis connect prod    # Connect to prod Redis cluster

Implementation steps:
1. Create aws/redis.go with RedisManager struct
2. Add Connect() method that:
   - Gets Redis endpoint from SSM: /<env>/zenith/redis/cluster-endpoint
   - Parses endpoint to extract host (remove port if present)
   - Spawns a temporary pod with redis-cli:
     kubectl run redis-temp-<random> --rm -it --restart=Never \
       --image=redis:7-alpine \
       -- redis-cli -h <host> -p 6379 -c
   - The -c flag enables cluster mode
3. Add "redis" case to cli.go (or extend existing if present):
   - "connect <env>" -> calls RedisManager.Connect()
4. Update showHelp()

The pod runs interactively and auto-deletes when redis-cli exits.
Default Redis port: 6379
```

---

## 6. MSK/Kafka UI Tunnel

**Prompt:**
```
Add MSK Kafka UI tunnel command to rolewalkers CLI.

Requirements:
1. Add "msk ui" subcommand to cli/cli.go
2. Deploy a Kafka UI pod with IAM authentication for MSK cluster access
3. Port-forward the UI to localhost

Usage:
- rwcli msk ui dev            # Start Kafka UI on localhost:8080
- rwcli msk ui prod --port 9090
- rwcli msk stop dev          # Stop the Kafka UI pod

Implementation steps:
1. Create aws/msk.go with MSKManager struct
2. Add StartUI() method that:
   - Gets MSK brokers from SSM: /<env>/zenith/msk/brokers-iam-endpoint
   - Creates a Kafka UI deployment pod:
     kubectl run kafka-ui-<env> --restart=Never \
       --image=provectuslabs/kafka-ui:latest \
       --env="KAFKA_CLUSTERS_0_NAME=<env>" \
       --env="KAFKA_CLUSTERS_0_BOOTSTRAPSERVERS=<brokers>" \
       --env="KAFKA_CLUSTERS_0_PROPERTIES_SECURITY_PROTOCOL=SASL_SSL" \
       --env="KAFKA_CLUSTERS_0_PROPERTIES_SASL_MECHANISM=AWS_MSK_IAM" \
       --env="KAFKA_CLUSTERS_0_PROPERTIES_SASL_JAAS_CONFIG=software.amazon.msk.auth.iam.IAMLoginModule required;" \
       --env="KAFKA_CLUSTERS_0_PROPERTIES_SASL_CLIENT_CALLBACK_HANDLER_CLASS=software.amazon.msk.auth.iam.IAMClientCallbackHandler" \
       -n zenith
   - Waits for pod to be ready
   - Port-forwards: kubectl port-forward pod/kafka-ui-<env> <port>:8080 -n zenith
3. Add StopUI() method to delete the pod
4. Add "msk" case to cli.go:
   - "ui <env>" -> StartUI()
   - "stop <env>" -> StopUI()
5. Update showHelp()

Default local port: 8080
Kafka UI container port: 8080
```

---

## 7. Maintenance Mode Toggle

**Prompt:**
```
Add maintenance mode toggle command to rolewalkers CLI.

Requirements:
1. Add "maintenance" command to cli/cli.go
2. Toggle maintenance mode via Fastly API
3. Support different service types (api, pwa, all)

Usage:
- rwcli maintenance --env dev --type api --enable
- rwcli maintenance --env prod --type pwa --disable
- rwcli maintenance --env prod --type all --enable
- rwcli maintenance status dev

Implementation steps:
1. Create aws/maintenance.go with MaintenanceManager struct
2. Store Fastly service IDs (get from SSM or config):
   - API service ID path: /<env>/zenith/fastly/api-service-id
   - PWA service ID path: /<env>/zenith/fastly/pwa-service-id
3. Add Toggle() method that:
   - Gets Fastly API token from environment: FASTLY_API_TOKEN
   - Makes HTTP request to Fastly API:
     PUT https://api.fastly.com/service/<service_id>/version/<version>/snippet/<snippet_name>
   - Or uses Fastly's maintenance mode feature via their API
4. Add Status() method to check current maintenance state
5. Add "maintenance" case to cli.go:
   - Parse --env, --type, --enable/--disable flags
   - Call MaintenanceManager methods
6. Update showHelp()

Requires: FASTLY_API_TOKEN environment variable
Types: api, pwa, all
```

---

## 8. HPA Scaling Commands

**Prompt:**
```
Add HPA scaling commands to rolewalkers CLI.

Requirements:
1. Add "scale" command to cli/cli.go
2. Patch HPA (Horizontal Pod Autoscaler) resources for scaling
3. Support presets and custom scaling

Usage:
- rwcli scale preprod --preset performance   # Scale up for performance testing
- rwcli scale prod --preset normal           # Reset to normal scaling
- rwcli scale dev --service candidate --min 5 --max 10

Presets:
- normal: min=2, max=10
- performance: min=10, max=50
- minimal: min=1, max=3

Implementation steps:
1. Create aws/scaling.go with ScalingManager struct
2. Define preset configurations:
   var presets = map[string]struct{ Min, Max int }{
       "normal":      {2, 10},
       "performance": {10, 50},
       "minimal":     {1, 3},
   }
3. Add Scale() method that:
   - Ensures correct kubectl context
   - Lists HPAs in zenith namespace: kubectl get hpa -n zenith -o json
   - For each HPA (or specific service), patches min/max:
     kubectl patch hpa <name> -n zenith --type=merge \
       -p '{"spec":{"minReplicas":<min>,"maxReplicas":<max>}}'
4. Add ScaleService() for single service scaling
5. Add "scale" case to cli.go:
   - Parse --preset, --service, --min, --max flags
   - Call appropriate ScalingManager method
6. Update showHelp()

HPA naming pattern: <service>-microservice-hpa
```

---

## 9. Database Backup/Restore

**Prompt:**
```
Add database backup and restore commands to rolewalkers CLI.

Requirements:
1. Add "db backup" and "db restore" subcommands to cli/cli.go
2. Orchestrate pg_dump/pg_restore operations via a temporary pod
3. Support local file output/input

Usage:
- rwcli db backup dev --output ./backup.sql
- rwcli db backup dev --output ./backup.sql --schema-only
- rwcli db restore dev --input ./backup.sql
- rwcli db restore dev --input ./backup.sql --clean

Implementation steps:
1. Extend aws/database.go (or create new file) with backup/restore methods
2. Add Backup() method that:
   - Gets database endpoint and credentials from SSM
   - Creates a temporary pod with pg_dump:
     kubectl run pgdump-temp-<random> --rm -i --restart=Never \
       --image=postgres:15-alpine \
       --env="PGPASSWORD=<password>" \
       -- pg_dump -h <endpoint> -U zenith_app -d zenith [--schema-only]
   - Captures stdout and writes to local file
3. Add Restore() method that:
   - Reads local SQL file
   - Creates a temporary pod with psql:
     kubectl run pgrestore-temp-<random> --rm -i --restart=Never \
       --image=postgres:15-alpine \
       --env="PGPASSWORD=<password>" \
       -- psql -h <endpoint> -U zenith_app -d zenith [--clean]
   - Pipes SQL file content to stdin
4. Add to cli.go "db" case:
   - "backup <env>" with --output, --schema-only flags
   - "restore <env>" with --input, --clean flags
5. Update showHelp()

Warning: Add confirmation prompt for restore operations!
```

---

## 10. Blue-Green Replication Manager

**Prompt:**
```
Add blue-green database replication management to rolewalkers CLI.

Requirements:
1. Add "replication" command to cli/cli.go
2. Manage RDS blue-green deployments via AWS API
3. Support status checking and switchover

Usage:
- rwcli replication status dev
- rwcli replication switch dev --target blue
- rwcli replication create dev --source prod-db-cluster

Implementation steps:
1. Create aws/replication.go with ReplicationManager struct
2. Add Status() method that:
   - Uses AWS CLI: aws rds describe-blue-green-deployments --region eu-west-2
   - Parses response to show:
     - Deployment identifier
     - Source cluster
     - Target cluster
     - Status (AVAILABLE, SWITCHOVER_IN_PROGRESS, etc.)
     - Progress percentage
3. Add Switch() method that:
   - Validates deployment exists and is AVAILABLE
   - Executes switchover:
     aws rds switchover-blue-green-deployment \
       --blue-green-deployment-identifier <id> \
       --region eu-west-2
   - Monitors progress until complete
4. Add Create() method for creating new blue-green deployments:
   aws rds create-blue-green-deployment \
     --blue-green-deployment-name <name> \
     --source <source-arn> \
     --region eu-west-2
5. Add "replication" case to cli.go:
   - "status <env>" -> Status()
   - "switch <env> --target <blue|green>" -> Switch()
   - "create <env> --source <cluster>" -> Create()
6. Update showHelp()

Warning: Add confirmation prompts for switch and create operations!
This is a complex operation - include progress monitoring and error handling.
```

---

## Quick Reference: File Locations

| Feature | Main File | Supporting Files |
|---------|-----------|------------------|
| keygen | cli/cli.go | (none needed) |
| ssm | cli/cli.go | aws/ssm.go (extend) |
| grpc | cli/cli.go | aws/grpc.go (new) or aws/ports.go |
| db connect | cli/cli.go | aws/database.go (new) |
| redis connect | cli/cli.go | aws/redis.go (new) |
| msk ui | cli/cli.go | aws/msk.go (new) |
| maintenance | cli/cli.go | aws/maintenance.go (new) |
| scale | cli/cli.go | aws/scaling.go (new) |
| db backup/restore | cli/cli.go | aws/database.go (extend) |
| replication | cli/cli.go | aws/replication.go (new) |

---

## Implementation Tips

1. **Start with keygen** - It's the simplest and helps you understand the CLI pattern
2. **Follow existing patterns** - Look at how tunnel and kube commands are structured
3. **Use SSMManager** - It's already set up for parameter retrieval
4. **Test incrementally** - Build and test each feature before moving to the next
5. **Update help text** - Always add new commands to showHelp()
