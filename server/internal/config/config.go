package config

import (
	"flag"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/ilyakaznacheev/cleanenv"
)

type Config struct {
	Env                string `yaml:"env" env:"ENV" env-required:"true"`
	UploadsPath        string `yaml:"uploads_path" env:"UPLOADS_PATH" env-required:"true"`
	TwitchClientId     string `yaml:"twitch_client_id" env:"TWITCH_CLIENT_ID" env-required:"true"`
	TwitchClientSecret string `yaml:"twitch_client_secret" env:"TWITCH_CLIENT_SECRET" env-required:"true"`
	Database           `yaml:"database"`
	HTTPServer         `yaml:"http_server"`
	Clients            ClientsConfig `yaml:"clients"`
	AppSecret          string        `yaml:"app_secret" env:"APP_SECRET" env-required:"true"`
}

type Database struct {
	Host       string `yaml:"host" env:"HOST" env-default:"localhost"`
	Port       int    `yaml:"port" env:"PORT" env-required:"true"`
	UsernameDB string `yaml:"username-db" env:"USERNAMEDB" env-required:"true"`
	Password   string `yaml:"password" env:"PASSWORD"`
	DBName     string `yaml:"dbname" env:"DBNAME" env-default:"games"`
}

type HTTPServer struct {
	Address     string        `yaml:"address" env-default:"localhost:8080"`
	Timeout     time.Duration `yaml:"timeout" env-default:"4s"`
	IdleTimeout time.Duration `yaml:"idle_timeout" env-default:"60s"`
	Cors        []string      `yaml:"cors" env-default:"[http://localhost:3000]"`
}

type Client struct {
	Address      string        `yaml:"address" env-required:"true"`
	Timeout      time.Duration `yaml:"timeout" env-required:"true"`
	RetriesCount int           `yaml:"retries_count" env-required:"true"`
	Insecure     bool          `yaml:"insecure" env-required:"true"`
}

type ClientsConfig struct {
	SSO Client `yaml:"sso"`
}

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

func (cfg *Database) GetDSN() string {
	dsn := fmt.Sprintf(
		"%s:%s@tcp(%s:%d)/%s?parseTime=true",
		cfg.UsernameDB,
		cfg.Password,
		cfg.Host,
		cfg.Port,
		cfg.DBName,
	)

	log.Print(dsn)

	return dsn
}
