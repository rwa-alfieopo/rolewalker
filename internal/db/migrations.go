package db

// migrateV1CreateEnvironments creates the environments table
func migrateV1CreateEnvironments(db *DB) error {
	_, err := db.Exec(`
		CREATE TABLE environments (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT NOT NULL UNIQUE,
			display_name TEXT NOT NULL,
			region TEXT NOT NULL,
			aws_profile TEXT NOT NULL,
			cluster_name TEXT NOT NULL,
			namespace TEXT NOT NULL DEFAULT 'zenith',
			active BOOLEAN NOT NULL DEFAULT 1,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		)
	`)
	return err
}

// migrateV2CreateServices creates the services table
func migrateV2CreateServices(db *DB) error {
	_, err := db.Exec(`
		CREATE TABLE services (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT NOT NULL UNIQUE,
			display_name TEXT NOT NULL,
			service_type TEXT NOT NULL,
			default_remote_port INTEGER NOT NULL,
			description TEXT,
			active BOOLEAN NOT NULL DEFAULT 1,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		)
	`)
	return err
}

// migrateV3CreatePortMappings creates the port_mappings table
func migrateV3CreatePortMappings(db *DB) error {
	_, err := db.Exec(`
		CREATE TABLE port_mappings (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			service_id INTEGER NOT NULL,
			environment_id INTEGER NOT NULL,
			local_port INTEGER NOT NULL,
			remote_port INTEGER NOT NULL,
			description TEXT,
			active BOOLEAN NOT NULL DEFAULT 1,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			FOREIGN KEY (service_id) REFERENCES services(id) ON DELETE CASCADE,
			FOREIGN KEY (environment_id) REFERENCES environments(id) ON DELETE CASCADE,
			UNIQUE(service_id, environment_id)
		)
	`)
	return err
}

// migrateV4CreateScalingPresets creates the scaling_presets table
func migrateV4CreateScalingPresets(db *DB) error {
	_, err := db.Exec(`
		CREATE TABLE scaling_presets (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT NOT NULL UNIQUE,
			display_name TEXT NOT NULL,
			min_replicas INTEGER NOT NULL,
			max_replicas INTEGER NOT NULL,
			description TEXT,
			active BOOLEAN NOT NULL DEFAULT 1,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		)
	`)
	return err
}

// migrateV5CreateAPIEndpoints creates the api_endpoints table
func migrateV5CreateAPIEndpoints(db *DB) error {
	_, err := db.Exec(`
		CREATE TABLE api_endpoints (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT NOT NULL UNIQUE,
			base_url TEXT NOT NULL,
			description TEXT,
			active BOOLEAN NOT NULL DEFAULT 1,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		)
	`)
	return err
}

// migrateV6CreateClusterMappings creates the cluster_mappings table
func migrateV6CreateClusterMappings(db *DB) error {
	_, err := db.Exec(`
		CREATE TABLE cluster_mappings (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			environment_id INTEGER NOT NULL,
			cluster_prefix TEXT NOT NULL,
			cluster_suffix TEXT NOT NULL,
			full_cluster_name TEXT NOT NULL,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			FOREIGN KEY (environment_id) REFERENCES environments(id) ON DELETE CASCADE,
			UNIQUE(environment_id)
		)
	`)
	return err
}

// migrateV7SeedDefaultData seeds the database with default configuration
func migrateV7SeedDefaultData(db *DB) error {
	// Seed environments
	environments := []struct {
		name        string
		displayName string
		region      string
		profile     string
		cluster     string
	}{
		{"snd", "Sandbox", "eu-west-2", "zenith-sandbox", "snd-zenith-eks-cluster"},
		{"dev", "Development", "eu-west-2", "zenith-dev", "dev-zenith-eks-cluster"},
		{"sit", "SIT", "eu-west-2", "zenith-sit", "sit-zenith-eks-cluster"},
		{"preprod", "Pre-Production", "eu-west-2", "zenith-preprod", "preprod-zenith-eks-cluster"},
		{"trg", "TRG", "eu-west-2", "zenith-trg", "trg-zenith-eks-cluster"},
		{"prod", "Production", "eu-west-2", "zenith-live", "prod-zenith-eks-cluster"},
		{"qa", "QA", "eu-west-2", "zenith-qa", "qa-zenith-eks-cluster"},
		{"stage", "Staging", "eu-west-2", "zenith-staging", "stage-zenith-eks-cluster"},
	}

	for _, env := range environments {
		_, err := db.Exec(`
			INSERT INTO environments (name, display_name, region, aws_profile, cluster_name)
			VALUES (?, ?, ?, ?, ?)
		`, env.name, env.displayName, env.region, env.profile, env.cluster)
		if err != nil {
			return err
		}
	}

	// Seed services
	services := []struct {
		name        string
		displayName string
		serviceType string
		remotePort  int
		description string
	}{
		{"db", "Database", "postgresql", 5432, "PostgreSQL database"},
		{"redis", "Redis", "redis", 6379, "Redis cache"},
		{"elasticsearch", "Elasticsearch", "elasticsearch", 9200, "Elasticsearch cluster"},
		{"kafka", "Kafka", "kafka", 9092, "Kafka broker"},
		{"msk", "MSK", "kafka", 9098, "AWS MSK Kafka"},
		{"rabbitmq", "RabbitMQ", "rabbitmq", 443, "RabbitMQ message broker"},
		{"grpc", "gRPC", "grpc", 5001, "gRPC services"},
	}

	for _, svc := range services {
		_, err := db.Exec(`
			INSERT INTO services (name, display_name, service_type, default_remote_port, description)
			VALUES (?, ?, ?, ?, ?)
		`, svc.name, svc.displayName, svc.serviceType, svc.remotePort, svc.description)
		if err != nil {
			return err
		}
	}

	// Seed port mappings
	portMappings := map[string]map[string]int{
		"db": {
			"snd": 5432, "dev": 5433, "sit": 5434, "preprod": 5435,
			"trg": 5437, "prod": 5438, "qa": 5440, "stage": 5442,
		},
		"redis": {
			"snd": 6379, "dev": 6380, "sit": 6381, "preprod": 6382,
			"trg": 6383, "prod": 6384, "qa": 6385, "stage": 6386,
		},
		"elasticsearch": {
			"snd": 9200, "dev": 9200, "sit": 9200, "preprod": 9200,
			"trg": 9200, "prod": 9200, "qa": 9200, "stage": 9200,
		},
		"kafka": {
			"snd": 9092, "dev": 9093, "sit": 9094, "preprod": 9095,
			"trg": 9096, "prod": 9097, "qa": 9098, "stage": 9099,
		},
		"msk": {
			"snd": 8080, "dev": 8081, "sit": 8082, "preprod": 8083,
			"trg": 8084, "prod": 8085, "qa": 8086, "stage": 8087,
		},
		"rabbitmq": {
			"snd": 5672, "dev": 5673, "sit": 5674, "preprod": 5675,
			"trg": 5676, "prod": 5677, "qa": 5678, "stage": 5679,
		},
		"grpc": {
			"snd": 50051, "dev": 50052, "sit": 50053, "preprod": 50054,
			"trg": 50055, "prod": 50056, "qa": 50057, "stage": 50058,
		},
	}

	for serviceName, envPorts := range portMappings {
		for envName, localPort := range envPorts {
			_, err := db.Exec(`
				INSERT INTO port_mappings (service_id, environment_id, local_port, remote_port)
				SELECT s.id, e.id, ?, s.default_remote_port
				FROM services s, environments e
				WHERE s.name = ? AND e.name = ?
			`, localPort, serviceName, envName)
			if err != nil {
				return err
			}
		}
	}

	// Seed gRPC microservice ports
	grpcPorts := map[string]int{
		"candidate": 5001, "job": 5002, "client": 5003, "organisation": 5004,
		"user": 5006, "email": 5007, "billing": 5074, "core": 5020,
	}

	for microservice, port := range grpcPorts {
		_, err := db.Exec(`
			INSERT INTO services (name, display_name, service_type, default_remote_port, description)
			VALUES (?, ?, 'grpc-microservice', ?, ?)
		`, "grpc-"+microservice, microservice+" Microservice", port, "gRPC "+microservice+" microservice")
		if err != nil {
			return err
		}
	}

	// Seed scaling presets
	presets := []struct {
		name        string
		displayName string
		min         int
		max         int
		description string
	}{
		{"normal", "Normal", 2, 10, "Standard scaling for normal operations"},
		{"performance", "Performance", 10, 50, "High performance scaling for peak loads"},
		{"minimal", "Minimal", 1, 3, "Minimal scaling for cost optimization"},
	}

	for _, preset := range presets {
		_, err := db.Exec(`
			INSERT INTO scaling_presets (name, display_name, min_replicas, max_replicas, description)
			VALUES (?, ?, ?, ?, ?)
		`, preset.name, preset.displayName, preset.min, preset.max, preset.description)
		if err != nil {
			return err
		}
	}

	// Seed API endpoints
	_, err := db.Exec(`
		INSERT INTO api_endpoints (name, base_url, description)
		VALUES ('fastly', 'https://api.fastly.com', 'Fastly CDN API')
	`)
	if err != nil {
		return err
	}

	return nil
}
