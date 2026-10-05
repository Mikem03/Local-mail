package config

import (
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setRequiredPostgresEnvironment(t *testing.T) {
	t.Helper()
	t.Setenv("POSTGRES_USER", "mail-user")
	t.Setenv("POSTGRES_PASSWORD", "mail-password")
	t.Setenv("POSTGRES_DB", "mail-db")
}

func TestLoad_RequiresCredentials(t *testing.T) {
	for _, variable := range []string{"POSTGRES_USER", "POSTGRES_PASSWORD", "POSTGRES_DB"} {
		t.Run(variable, func(t *testing.T) {
			setRequiredPostgresEnvironment(t)
			t.Setenv(variable, "")

			cfg, err := Load()

			require.Error(t, err)
			assert.Empty(t, cfg)
			assert.ErrorContains(t, err, variable)
		})
	}
}

func TestLoad_UsesLocalDevelopmentDefaults(t *testing.T) {
	setRequiredPostgresEnvironment(t)
	t.Setenv("POSTGRES_HOST", "")
	t.Setenv("POSTGRES_PORT", "")
	t.Setenv("POSTGRES_SSLMODE", "")

	cfg, err := Load()

	require.NoError(t, err)
	assert.Equal(t, Config{
		PostgresUser:     "mail-user",
		PostgresPassword: "mail-password",
		PostgresDatabase: "mail-db",
		PostgresHost:     "localhost",
		PostgresPort:     5432,
		PostgresSSLMode:  "disable",
	}, cfg)
}

func TestLoad_UsesConfiguredConnectionSettings(t *testing.T) {
	setRequiredPostgresEnvironment(t)
	t.Setenv("POSTGRES_HOST", "db.internal")
	t.Setenv("POSTGRES_PORT", "55432")
	t.Setenv("POSTGRES_SSLMODE", "require")

	cfg, err := Load()

	require.NoError(t, err)
	assert.Equal(t, "db.internal", cfg.PostgresHost)
	assert.Equal(t, uint16(55432), cfg.PostgresPort)
	assert.Equal(t, "require", cfg.PostgresSSLMode)
}

func TestLoad_RejectsInvalidPorts(t *testing.T) {
	invalidPorts := []string{"0", "-1", "not-a-port", "65536"}
	for _, port := range invalidPorts {
		t.Run(port, func(t *testing.T) {
			setRequiredPostgresEnvironment(t)
			t.Setenv("POSTGRES_PORT", port)

			cfg, err := Load()

			require.Error(t, err)
			assert.Empty(t, cfg)
			assert.ErrorContains(t, err, "POSTGRES_PORT")
		})
	}
}

func TestDatabaseURL_EscapesCredentialsAndDatabaseName(t *testing.T) {
	cfg := Config{
		PostgresUser:     "mail@user",
		PostgresPassword: "p@ss:/?# word",
		PostgresDatabase: "mail/db",
		PostgresHost:     "localhost",
		PostgresPort:     5432,
		PostgresSSLMode:  "require",
	}

	parsed, err := url.Parse(cfg.DatabaseURL())

	require.NoError(t, err)
	assert.Equal(t, "postgres", parsed.Scheme)
	assert.Equal(t, "localhost:5432", parsed.Host)
	assert.Equal(t, "mail@user", parsed.User.Username())
	password, hasPassword := parsed.User.Password()
	assert.True(t, hasPassword)
	assert.Equal(t, "p@ss:/?# word", password)
	assert.Equal(t, "/mail/db", parsed.Path)
	assert.Equal(t, "require", parsed.Query().Get("sslmode"))
}
