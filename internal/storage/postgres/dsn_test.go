package postgres

import (
	"net/url"
	"testing"

	"github.com/OCAP2/extension/v5/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildDSN_PlainConfig(t *testing.T) {
	dsn := buildDSN(config.PostgresConfig{
		Host:     "db.example.com",
		Port:     "5432",
		Username: "postgres",
		Password: "secret",
		Database: "ocap",
	})

	u, err := url.Parse(dsn)
	require.NoError(t, err)
	assert.Equal(t, "postgres", u.Scheme)
	assert.Equal(t, "db.example.com:5432", u.Host)
	assert.Equal(t, "/ocap", u.Path)
	assert.Equal(t, "sslmode=disable", u.RawQuery)
	assert.Equal(t, "postgres", u.User.Username())
	pw, ok := u.User.Password()
	assert.True(t, ok)
	assert.Equal(t, "secret", pw)
}

func TestBuildDSN_EncodesSpecialChars(t *testing.T) {
	// Spaces, single quotes, '@', and '/' in the password used to break the
	// keyword/value DSN form. With the URL form they round-trip cleanly.
	cfg := config.PostgresConfig{
		Host:     "db.example.com",
		Port:     "5432",
		Username: "user@corp",
		Password: `p ass'w@rd/`,
		Database: "ocap",
	}
	dsn := buildDSN(cfg)

	u, err := url.Parse(dsn)
	require.NoError(t, err)
	assert.Equal(t, "user@corp", u.User.Username())
	pw, ok := u.User.Password()
	assert.True(t, ok)
	assert.Equal(t, `p ass'w@rd/`, pw)
}

func TestBuildDSN_IPv6Host(t *testing.T) {
	dsn := buildDSN(config.PostgresConfig{
		Host:     "::1",
		Port:     "5432",
		Username: "postgres",
		Password: "secret",
		Database: "ocap",
	})
	u, err := url.Parse(dsn)
	require.NoError(t, err)
	// net.JoinHostPort wraps IPv6 in brackets so the port stays unambiguous.
	assert.Equal(t, "[::1]:5432", u.Host)
}
