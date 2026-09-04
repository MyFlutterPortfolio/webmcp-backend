package config

import (
	"testing"
	"time"
)

func TestLoadUsesManagedRuntimePortWhenExplicitAddressIsAbsent(t *testing.T) {
	t.Setenv("WEBMCP_HTTP_ADDR", "")
	t.Setenv("PORT", "4317")
	t.Setenv("WEBMCP_ENV", "development")
	config, err := Load()
	if err != nil {
		t.Fatalf("expected runtime port to be accepted: %v", err)
	}
	if config.HTTPAddr != "0.0.0.0:4317" {
		t.Fatalf("unexpected managed runtime address: %q", config.HTTPAddr)
	}
}

func TestLoadRejectsInvalidManagedRuntimePort(t *testing.T) {
	t.Setenv("WEBMCP_HTTP_ADDR", "")
	t.Setenv("PORT", "not-a-port")
	if _, err := Load(); err == nil {
		t.Fatal("expected invalid runtime port to be rejected")
	}
}

func TestValidateProductionRequiresExactOrigin(t *testing.T) {
	c := Config{
		Environment:       "production",
		HTTPAddr:          ":8080",
		DatabaseURL:       "postgres://user:pass@localhost:5432/db?sslmode=verify-full",
		ShutdownTimeout:   1,
		RequestTimeout:    1,
		MaxBodyBytes:      1,
		DBMaxConns:        1,
		ReadHeaderTimeout: 1,
		WriteTimeout:      1,
		IdleTimeout:       1,
	}
	if err := c.Validate(); err == nil {
		t.Fatal("expected production origin validation error")
	}
	c.AllowedOrigins = []string{"*"}
	if err := c.Validate(); err == nil {
		t.Fatal("expected wildcard origin validation error")
	}
	c.AllowedOrigins = []string{"https://app.example.com"}
	c.JWTIssuer = "https://id.example.com/tenant"
	c.JWTAudience = "webmcp-api"
	c.JWKSURL = "https://id.example.com/tenant/.well-known/jwks.json"
	c.JWTClockSkew = time.Minute
	if err := c.Validate(); err != nil {
		t.Fatalf("expected valid production config, got %v", err)
	}
}

func TestSplitCSVTrimsEmptyValues(t *testing.T) {
	got := splitCSV(" https://a.example , ,https://b.example")
	if len(got) != 2 || got[0] != "https://a.example" || got[1] != "https://b.example" {
		t.Fatalf("unexpected origins: %#v", got)
	}
}

func TestValidateAuthenticationRequiresCompleteSecureConfiguration(t *testing.T) {
	c := validProductionConfig()
	c.JWKSURL = ""
	if err := c.Validate(); err == nil {
		t.Fatal("expected incomplete authentication configuration to be rejected")
	}
	c = validProductionConfig()
	c.JWKSURL = "http://id.example.com/jwks"
	if err := c.Validate(); err == nil {
		t.Fatal("expected insecure production JWKS URL to be rejected")
	}
}

func TestValidateProductionRejectsInsecureDatabaseTransport(t *testing.T) {
	c := validProductionConfig()
	c.DatabaseURL = "postgres://user:pass@localhost:5432/db?sslmode=disable"
	if err := c.Validate(); err == nil {
		t.Fatal("expected insecure production database URL to be rejected")
	}
}

func TestValidateRejectsNonOriginCORSValues(t *testing.T) {
	c := validProductionConfig()
	c.AllowedOrigins = []string{"https://app.example.com/path"}
	if err := c.Validate(); err == nil {
		t.Fatal("expected CORS path to be rejected")
	}
}

func validProductionConfig() Config {
	return Config{
		Environment: "production", HTTPAddr: ":8080",
		DatabaseURL:    "postgres://user:pass@localhost:5432/db?sslmode=verify-full",
		AllowedOrigins: []string{"https://app.example.com"},
		JWTIssuer:      "https://id.example.com/tenant", JWTAudience: "webmcp-api", JWKSURL: "https://id.example.com/tenant/.well-known/jwks.json", JWTClockSkew: time.Minute,
		ShutdownTimeout: 1, RequestTimeout: 1, MaxBodyBytes: 1, DBMaxConns: 1,
		ReadHeaderTimeout: 1, WriteTimeout: 1, IdleTimeout: 1,
	}
}
