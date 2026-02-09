package aws

// DefaultEnvironments is the canonical fallback list of valid environments.
// Used when the database is unavailable.
var DefaultEnvironments = []string{"snd", "dev", "sit", "preprod", "trg", "prod", "qa", "stage"}

// DefaultPresets is the canonical fallback list of scaling preset names.
var DefaultPresets = []string{"normal", "performance", "minimal"}

// DefaultServices is the canonical fallback list of service names.
var DefaultServices = "db, redis, elasticsearch, kafka, msk, rabbitmq, grpc"

// DefaultGRPCServices is the canonical fallback list of gRPC microservice names.
var DefaultGRPCServices = "candidate, job, client, organisation, user, email, billing, core"
