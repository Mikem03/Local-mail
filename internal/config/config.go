package config

import (
	"fmt"
	"net"
	"net/url"
	"os"
	"strconv"
)

// Config contains settings shared by the API, worker, and migration command.
type Config struct {
	PostgresUser     string
	PostgresPassword string
	PostgresDatabase string
	PostgresHost     string
	PostgresPort     uint16
	PostgresSSLMode  string
}

func Load() (Config, error) {
	var cfg Config
	var err error

	if cfg.PostgresUser, err = required("POSTGRES_USER"); err != nil {
		return Config{}, err
	}
	if cfg.PostgresPassword, err = required("POSTGRES_PASSWORD"); err != nil {
		return Config{}, err
	}
	if cfg.PostgresDatabase, err = required("POSTGRES_DB"); err != nil {
		return Config{}, err
	}

	cfg.PostgresHost = valueOr("POSTGRES_HOST", "localhost")
	cfg.PostgresSSLMode = valueOr("POSTGRES_SSLMODE", "disable")

	port := valueOr("POSTGRES_PORT", "5432")
	parsedPort, err := strconv.ParseUint(port, 10, 16)
	if err != nil || parsedPort == 0 {
		return Config{}, fmt.Errorf("POSTGRES_PORT must be an integer between 1 and 65535")
	}
	cfg.PostgresPort = uint16(parsedPort)

	return cfg, nil
}

func required(name string) (string, error) {
	value := os.Getenv(name)
	if value == "" {
		return "", fmt.Errorf("required environment variable %s is not set", name)
	}
	return value, nil
}

func valueOr(name, fallback string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}
	return fallback
}

// DatabaseURL returns a properly escaped PostgreSQL connection URL.
func (c Config) DatabaseURL() string {
	u := url.URL{
		Scheme: "postgres",
		User:   url.UserPassword(c.PostgresUser, c.PostgresPassword),
		Host:   net.JoinHostPort(c.PostgresHost, strconv.Itoa(int(c.PostgresPort))),
		Path:   "/" + c.PostgresDatabase,
	}
	query := url.Values{}
	query.Set("sslmode", c.PostgresSSLMode)
	u.RawQuery = query.Encode()
	return u.String()
}
