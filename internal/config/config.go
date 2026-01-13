package config

import (
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/ilyakaznacheev/cleanenv"
)

type Config struct {
	Env         string `yaml:"env" env-default:"local"`
	*HTTPServer `yaml:"http_server"`
	Routes      map[string]*ProxyRoute `yaml:"routes"`
	*Websocket  `yaml:"websocket"`
}

type HTTPServer struct {
	Address     string        `yaml:"address" env-default:"localhost:8080"`
	Timeout     time.Duration `yaml:"timeout" env-default:"4s"`
	IdleTimeout time.Duration `yaml:"idle_timeout" env-default:"60s"`
	*Cors       `yaml:"cors"`
}

type Cors struct {
	AllowedOrigins     []string `yaml:"allowed_origins"`
	AllowedMethods     []string `yaml:"allowed_methods"`
	AllowedHeaders     []string `yaml:"allowed_headers"`
	ExposedHeaders     []string `yaml:"exposed_headers"`
	AllowCredentials   bool     `yaml:"allow_credentials" env-default:"false"`
	MaxAge             int      `yaml:"max_age" env-default:"300"`
	OptionsPassthrough bool     `yaml:"options" env-default:"false"`
	Debug              bool     `yaml:"debug" env-default:"false"`
}

type ProxyRoute struct {
	Topic            string   `yaml:"topic" env-required:"true"`
	RequiredScope    string   `yaml:"required_scope"`
	AdditionalScopes []string `yaml:"additional_scopes"`
}

type Websocket struct {
	HandshakeTimeout  time.Duration `yaml:"handshake_timeout"`
	ReadBufferSize    int           `yaml:"read_buffer_size"`
	WriteBufferSize   int           `yaml:"write_buffer_size"`
	Subprotocols      []string      `yaml:"subprotocols"`
	EnableCompression bool          `yaml:"enable_compression" env-default:"false"`
}

func MustLoad() *Config {
	configPath := os.Getenv("CONFIG_PATH")
	if configPath == "" {
		log.Fatal("CONFIG_PATH is not set")
	}

	// check if file exists
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		fullConfigPath, _ := filepath.Abs(configPath)
		log.Fatalf("config file does not exists by path \"%s\"", fullConfigPath)
	}

	var cfg Config
	if err := cleanenv.ReadConfig(configPath, &cfg); err != nil {
		log.Fatalf("can not read config: %s", err)
	}

	return &cfg
}
