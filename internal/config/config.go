package config

import (
	"fmt"
	"net"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

// Config contains only process-level configuration. Secrets are intentionally
// not loaded into a client-facing package or returned by any API endpoint.
type Config struct {
	Environment         string
	HTTPAddr            string
	DatabaseURL         string
	DBMinConns          int32
	DBMaxConns          int32
	ShutdownTimeout     time.Duration
	RequestTimeout      time.Duration
	MaxBodyBytes        int64
	AllowedOrigins      []string
	ReadHeaderTimeout   time.Duration
	WriteTimeout        time.Duration
	IdleTimeout         time.Duration
	JWTIssuer           string
	JWTAudience         string
	JWKSURL             string
	JWTClockSkew        time.Duration
	GuestDemoEnabled    bool
	GuestDemoSecret     string
	GuestUserID         string
	GuestOrganizationID string
	GuestBusinessID     string
	GuestGoalID         string
	GuestTokenTTL       time.Duration
}

func Load() (Config, error) {
	address, err := listenAddress()
	if err != nil {
		return Config{}, err
	}
	c := Config{
		Environment:         getString("WEBMCP_ENV", "development"),
		HTTPAddr:            address,
		DatabaseURL:         strings.TrimSpace(os.Getenv("WEBMCP_DATABASE_URL")),
		DBMinConns:          int32(getInt64("WEBMCP_DB_MIN_CONNS", 2)),
		DBMaxConns:          int32(getInt64("WEBMCP_DB_MAX_CONNS", 10)),
		ShutdownTimeout:     getDuration("WEBMCP_SHUTDOWN_TIMEOUT", 10*time.Second),
		RequestTimeout:      getDuration("WEBMCP_REQUEST_TIMEOUT", 15*time.Second),
		MaxBodyBytes:        getInt64("WEBMCP_MAX_BODY_BYTES", 1<<20),
		AllowedOrigins:      splitCSV(os.Getenv("WEBMCP_ALLOWED_ORIGINS")),
		ReadHeaderTimeout:   5 * time.Second,
		WriteTimeout:        30 * time.Second,
		IdleTimeout:         60 * time.Second,
		JWTIssuer:           strings.TrimSpace(os.Getenv("WEBMCP_JWT_ISSUER")),
		JWTAudience:         strings.TrimSpace(os.Getenv("WEBMCP_JWT_AUDIENCE")),
		JWKSURL:             strings.TrimSpace(os.Getenv("WEBMCP_JWKS_URL")),
		JWTClockSkew:        getDuration("WEBMCP_JWT_CLOCK_SKEW", 30*time.Second),
		GuestDemoEnabled:    getBool("WEBMCP_GUEST_DEMO_ENABLED", false),
		GuestDemoSecret:     strings.TrimSpace(os.Getenv("WEBMCP_GUEST_DEMO_SECRET")),
		GuestUserID:         strings.TrimSpace(os.Getenv("WEBMCP_GUEST_DEMO_USER_ID")),
		GuestOrganizationID: strings.TrimSpace(os.Getenv("WEBMCP_GUEST_DEMO_ORGANIZATION_ID")),
		GuestBusinessID:     strings.TrimSpace(os.Getenv("WEBMCP_GUEST_DEMO_BUSINESS_ID")),
		GuestGoalID:         strings.TrimSpace(os.Getenv("WEBMCP_GUEST_DEMO_GOAL_ID")),
		GuestTokenTTL:       getDuration("WEBMCP_GUEST_DEMO_TOKEN_TTL", 15*time.Minute),
	}

	if err := c.Validate(); err != nil {
		return Config{}, err
	}
	return c, nil
}

// listenAddress keeps local development bound to loopback while honoring the
// port injected by managed runtimes such as Railway, Render and Cloud Run.
// WEBMCP_HTTP_ADDR remains the explicit override for local and self-hosted
// deployments.
func listenAddress() (string, error) {
	if configured := strings.TrimSpace(os.Getenv("WEBMCP_HTTP_ADDR")); configured != "" {
		return configured, nil
	}
	if port := strings.TrimSpace(os.Getenv("PORT")); port != "" {
		if parsed, err := strconv.Atoi(port); err == nil && parsed >= 1 && parsed <= 65535 {
			return net.JoinHostPort("0.0.0.0", port), nil
		}
		return "", fmt.Errorf("PORT must be an integer between 1 and 65535")
	}
	return "127.0.0.1:8080", nil
}

func (c Config) Validate() error {
	if c.Environment != "development" && c.Environment != "test" && c.Environment != "staging" && c.Environment != "production" {
		return fmt.Errorf("WEBMCP_ENV must be development, test, staging, or production")
	}
	if c.HTTPAddr == "" {
		return fmt.Errorf("WEBMCP_HTTP_ADDR must not be empty")
	}
	if _, _, err := net.SplitHostPort(c.HTTPAddr); err != nil {
		return fmt.Errorf("WEBMCP_HTTP_ADDR must be a valid host:port: %w", err)
	}
	if c.ShutdownTimeout <= 0 || c.RequestTimeout <= 0 || c.ReadHeaderTimeout <= 0 || c.WriteTimeout <= 0 || c.IdleTimeout <= 0 {
		return fmt.Errorf("server timeouts must be positive")
	}
	if c.MaxBodyBytes <= 0 || c.MaxBodyBytes > 32<<20 {
		return fmt.Errorf("WEBMCP_MAX_BODY_BYTES must be between 1 and 33554432")
	}
	if c.DBMinConns < 0 || c.DBMaxConns <= 0 || c.DBMinConns > c.DBMaxConns {
		return fmt.Errorf("database pool bounds are invalid")
	}
	if (c.Environment == "staging" || c.Environment == "production") && c.DatabaseURL == "" {
		return fmt.Errorf("WEBMCP_DATABASE_URL is required in staging and production")
	}
	if (c.Environment == "staging" || c.Environment == "production") && len(c.AllowedOrigins) == 0 {
		return fmt.Errorf("WEBMCP_ALLOWED_ORIGINS is required in staging and production")
	}
	if err := validateAuthentication(c); err != nil {
		return err
	}
	if err := validateGuestDemo(c); err != nil {
		return err
	}
	for _, origin := range c.AllowedOrigins {
		if err := validateOrigin(origin); err != nil {
			return fmt.Errorf("WEBMCP_ALLOWED_ORIGINS contains invalid origin %q: %w", origin, err)
		}
	}
	if c.Environment == "staging" || c.Environment == "production" {
		if err := validateProductionDatabaseURL(c.DatabaseURL, c.Environment == "production"); err != nil {
			return err
		}
	}
	return nil
}

func (c Config) AuthConfigured() bool {
	return c.JWTIssuer != "" || c.JWTAudience != "" || c.JWKSURL != ""
}

func (c Config) GuestDemoConfigured() bool { return c.GuestDemoEnabled }

func validateAuthentication(c Config) error {
	configured := c.AuthConfigured()
	if (c.Environment == "staging" || c.Environment == "production") && !configured && !c.GuestDemoConfigured() {
		return fmt.Errorf("WEBMCP_JWT_ISSUER, WEBMCP_JWT_AUDIENCE and WEBMCP_JWKS_URL are required in staging and production")
	}
	if !configured {
		return nil
	}
	if c.JWTIssuer == "" || c.JWTAudience == "" || c.JWKSURL == "" {
		return fmt.Errorf("WEBMCP_JWT_ISSUER, WEBMCP_JWT_AUDIENCE and WEBMCP_JWKS_URL must be configured together")
	}
	issuer, err := url.Parse(c.JWTIssuer)
	if err != nil || issuer.Scheme != "https" || issuer.Host == "" || issuer.User != nil || issuer.RawQuery != "" || issuer.Fragment != "" {
		return fmt.Errorf("WEBMCP_JWT_ISSUER must be an exact HTTPS issuer URL")
	}
	jwks, err := url.Parse(c.JWKSURL)
	if err != nil || (jwks.Scheme != "https" && !(c.Environment == "development" || c.Environment == "test")) || jwks.Host == "" || jwks.User != nil || jwks.Fragment != "" {
		return fmt.Errorf("WEBMCP_JWKS_URL must be an HTTPS URL (HTTP is allowed only in development and test)")
	}
	if c.JWTClockSkew <= 0 || c.JWTClockSkew > 5*time.Minute {
		return fmt.Errorf("WEBMCP_JWT_CLOCK_SKEW must be between 1ns and 5m")
	}
	return nil
}

func validateGuestDemo(c Config) error {
	if !c.GuestDemoEnabled {
		return nil
	}
	if len(c.GuestDemoSecret) < 32 {
		return fmt.Errorf("WEBMCP_GUEST_DEMO_SECRET must contain at least 32 characters when guest demo is enabled")
	}
	for key, value := range map[string]string{"WEBMCP_GUEST_DEMO_USER_ID": c.GuestUserID, "WEBMCP_GUEST_DEMO_ORGANIZATION_ID": c.GuestOrganizationID, "WEBMCP_GUEST_DEMO_BUSINESS_ID": c.GuestBusinessID, "WEBMCP_GUEST_DEMO_GOAL_ID": c.GuestGoalID} {
		if len(value) < 3 || len(value) > 200 {
			return fmt.Errorf("%s must be between 3 and 200 characters when guest demo is enabled", key)
		}
	}
	if c.GuestTokenTTL < time.Minute || c.GuestTokenTTL > 30*time.Minute {
		return fmt.Errorf("WEBMCP_GUEST_DEMO_TOKEN_TTL must be between 1m and 30m")
	}
	return nil
}

func validateOrigin(origin string) error {
	if origin == "" || origin == "*" {
		return fmt.Errorf("origins must be exact HTTP(S) origins")
	}
	parsed, err := url.Parse(origin)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" || parsed.User != nil || parsed.Path != "" || parsed.RawQuery != "" || parsed.Fragment != "" {
		return fmt.Errorf("origin must be an exact http or https origin")
	}
	return nil
}

func validateProductionDatabaseURL(databaseURL string, requireVerifiedTLS bool) error {
	parsed, err := url.Parse(databaseURL)
	if err != nil || (parsed.Scheme != "postgres" && parsed.Scheme != "postgresql") || parsed.Host == "" || parsed.Path == "" || parsed.User == nil {
		return fmt.Errorf("WEBMCP_DATABASE_URL must be a valid PostgreSQL URL")
	}
	switch sslmode := strings.ToLower(parsed.Query().Get("sslmode")); sslmode {
	case "verify-full":
		return nil
	case "require", "verify-ca":
		if !requireVerifiedTLS {
			return nil
		}
		return fmt.Errorf("production WEBMCP_DATABASE_URL must use sslmode=verify-full")
	default:
		return fmt.Errorf("WEBMCP_DATABASE_URL must require TLS with sslmode=verify-full (production) or sslmode=require/verify-ca (staging)")
	}
}

func getString(key, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return fallback
}

func getDuration(key string, fallback time.Duration) time.Duration {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	duration, err := time.ParseDuration(value)
	if err != nil {
		return -1
	}
	return duration
}

func getInt64(key string, fallback int64) int64 {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	parsed, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		return -1
	}
	return parsed
}

func getBool(key string, fallback bool) bool {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	parsed, err := strconv.ParseBool(value)
	if err != nil {
		return false
	}
	return parsed
}

func splitCSV(value string) []string {
	parts := strings.Split(value, ",")
	origins := make([]string, 0, len(parts))
	for _, part := range parts {
		if origin := strings.TrimSpace(part); origin != "" {
			origins = append(origins, origin)
		}
	}
	return origins
}
