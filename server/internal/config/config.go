// internal/config/config.go
// Package config provides application-wide configuration management.
// It supports loading configuration from a YAML file combined with
// environment variable overrides, using the cleanenv library.
//
// Configuration is loaded once at startup via MustLoad, which terminates
// the process immediately if the config file is missing or malformed.
// This fail-fast approach ensures the application never starts in a
// partially configured state.
//
// Configuration priority (highest to lowest):
//  1. Environment variables
//  2. YAML config file values
//  3. Declared env-default values
package config

import (
	"flag"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/ilyakaznacheev/cleanenv"
)

// ============================================================================
// INITIALIZATION
// ============================================================================

// Config is the root configuration structure for the application.
// It is populated from a YAML file and optionally overridden by
// environment variables at startup.
//
// All fields marked env-required:"true" must be present either in
// the config file or as environment variables, or the application
// will fail to start.
type Config struct {
	// Env identifies the runtime environment (e.g. "local", "dev", "prod").
	// Used to adjust logging verbosity and other environment-specific behavior.
	Env string `yaml:"env" env:"ENV" env-required:"true"`

	// UploadsPath is the file system path to the directory where
	// uploaded game images are stored.
	UploadsPath string `yaml:"uploads_path" env:"UPLOADS_PATH" env-required:"true"`

	// TwitchAPI is the base URL for the Twitch IGDB API,
	// used for fetching game metadata.
	TwitchAPI string `yaml:"twitch_api" env:"TWITCH_API" env-required:"true"`

	// TwitchAuthAPI is the base URL for the Twitch OAuth2 token endpoint,
	// used to obtain access tokens for IGDB API requests.
	TwitchAuthAPI string `yaml:"twitch_auth_api" env:"TWITCH_AUTH_API" env-required:"true"`

	// TwitchClientId is the public client identifier for the registered
	// Twitch application, required for IGDB API authentication.
	TwitchClientId string `yaml:"twitch_client_id" env:"TWITCH_CLIENT_ID" env-required:"true"`

	// TwitchClientSecret is the private secret for the registered Twitch
	// application. Must be kept out of version control.
	TwitchClientSecret string `yaml:"twitch_client_secret" env:"TWITCH_CLIENT_SECRET" env-required:"true"`

	// Database holds the MariaDB connection parameters.
	Database `yaml:"database"`

	// HTTPServer holds the HTTP server listen address and timeout settings.
	HTTPServer `yaml:"http_server"`

	// Clients holds configuration for all outbound gRPC service clients.
	Clients ClientsConfig `yaml:"clients"`

	// AppSecret is the secret key used for signing and verifying tokens.
	// Must be kept out of version control.
	AppSecret string `yaml:"app_secret" env:"APP_SECRET" env-required:"true"`

	AppID uint32 `yaml:"app_id" env:"APP_ID" env-required:"true"`
}

// Database holds the connection parameters for the MariaDB database.
// The DSN is assembled from these fields via GetDSN.
type Database struct {
	// Host is the hostname or IP address of the database server.
	// Defaults to "localhost" if not specified.
	Host string `yaml:"host" env:"HOST" env-default:"localhost"`

	// Port is the TCP port on which the database server is listening.
	Port int `yaml:"port" env:"PORT" env-required:"true"`

	// UsernameDB is the database user used to authenticate the connection.
	UsernameDB string `yaml:"username-db" env:"USERNAMEDB" env-required:"true"`

	// Password is the password for the database user.
	// May be empty for local development setups without authentication.
	Password string `yaml:"password" env:"PASSWORD"`

	// DBName is the name of the target database schema.
	// Defaults to "games" if not specified.
	DBName string `yaml:"dbname" env:"DBNAME" env-default:"games"`

	// Driver is the name of the database driver to use.
	Driver string `yaml:"driver" env:"DRIVER" env-required:"true"`
}

// HTTPServer holds the configuration for the application's HTTP server.
type HTTPServer struct {
	// Address is the host:port the HTTP server will listen on.
	// Defaults to "localhost:8080".
	Address string `yaml:"address" env-default:"localhost:8080"`

	// Timeout is the maximum duration for reading a request and writing
	// a response. Defaults to 4 seconds.
	Timeout time.Duration `yaml:"timeout" env-default:"4s"`

	// IdleTimeout is the maximum duration to keep an idle keep-alive
	// connection open. Defaults to 60 seconds.
	IdleTimeout time.Duration `yaml:"idle_timeout" env-default:"60s"`

	// ShutdownTimeout bounds graceful shutdown. It must exceed the longest
	// in-flight handler duration (the IGDB batch import uses 30s) plus a
	// small grace window so active work completes rather than being aborted.
	// Defaults to 35 seconds.
	ShutdownTimeout time.Duration `yaml:"shutdown_timeout" env-default:"35s"`

	// Cors is the list of allowed CORS origins for cross-origin requests.
	// Defaults to ["http://localhost:3000"].
	Cors []string `yaml:"cors" env-default:"[http://localhost:3000]"`
}

// Client holds the connection parameters for a single outbound gRPC client.
type Client struct {
	// Address is the host:port of the remote gRPC service.
	Address string `yaml:"address" env-required:"true"`

	// Timeout is the maximum duration to wait for a response from
	// the remote service before cancelling the request.
	Timeout time.Duration `yaml:"timeout" env-required:"true"`

	// RetriesCount is the number of times a failed request will be
	// retried before returning an error to the caller.
	RetriesCount int `yaml:"retries_count" env-required:"true"`

	// Insecure controls whether the gRPC connection is established
	// without TLS. Should be false in production environments.
	Insecure bool `yaml:"insecure" env-required:"true"`
}

// ClientsConfig groups the configuration for all outbound gRPC clients
// used by the application.
type ClientsConfig struct {
	// SSO is the client configuration for the remote Single Sign-On
	// authentication service.
	SSO Client `yaml:"sso"`
}

// MustLoad reads the application configuration from the YAML file
// specified by the --config command-line flag, merges any environment
// variable overrides, and returns the populated Config.
//
// The function terminates the process via log.Fatal if:
//   - The --config flag is not provided or is empty.
//   - The specified config file does not exist on disk.
//   - The config file cannot be parsed or a required field is missing.
//
// This function is intended to be called once at application startup.
// The "Must" prefix signals that it panics/fatals on failure rather
// than returning an error, following the Go convention for startup
// initialization helpers.
func MustLoad() *Config {
	configPath := flag.String("config", "", "path to config yaml file")
	flag.Parse()
	if *configPath == "" {
		log.Fatal("CONFIG_PATH is not set")
	}

	if _, err := os.Stat(*configPath); os.IsNotExist(err) {
		log.Fatalf("config file does not exist: %s", *configPath)
	}

	var cfg Config

	if err := cleanenv.ReadConfig(*configPath, &cfg); err != nil {
		log.Fatalf("cannot read config: %s - %s", *configPath, err)
	}

	return &cfg
}

// Load reads config from file without terminating the process on error.
// Used in utilities (migrate and others), where error handling is needed.
func Load(configPath string) (*Config, error) {
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		return nil, fmt.Errorf("config file does not exist: %s", configPath)
	}

	var cfg Config
	if err := cleanenv.ReadConfig(configPath, &cfg); err != nil {
		return nil, fmt.Errorf("cannot read config %s: %w", configPath, err)
	}

	return &cfg, nil
}

// ============================================================================
// GETTERS
// ============================================================================

// GetDSN constructs and returns the Data Source Name (DSN) string
// for connecting to the MariaDB database using the go-sql-driver/mysql
// driver format.
//
// The DSN format is:
//
//	username:password@tcp(host:port)/dbname?parseTime=true
//
// The parseTime=true parameter instructs the driver to automatically
// scan TIMESTAMP and DATETIME columns into Go time.Time values rather
// than raw byte slices.
func (cfg *Database) GetDSN() string {
	dsn := fmt.Sprintf(
		"%s:%s@tcp(%s:%d)/%s?parseTime=true",
		cfg.UsernameDB,
		cfg.Password,
		cfg.Host,
		cfg.Port,
		cfg.DBName,
	)

	return dsn
}

// GetTestDSN returns a hardcoded DSN string for connecting to the
// local test database instance.
//
// The test DSN targets:
//   - Host:     localhost:3306
//   - User:     root (no password)
//   - Database: test-games
//
// This function is intended exclusively for use in tests and should
// never be called in production code.
func GetTestDSN() string {
	dsn := "root:@tcp(localhost:3306)/test-games?parseTime=true"

	return dsn
}
