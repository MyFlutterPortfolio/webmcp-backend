package auth

import (
	"context"
	"crypto"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"math/big"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"webmcp-backend/internal/domain/core"
)

const (
	maxJWTSize        = 16 << 10
	maxJWKSSize       = 1 << 20
	jwksCacheLifetime = 10 * time.Minute
)

var errInvalidToken = errors.New("invalid bearer token")

// JWKSConfig is the provider-neutral contract for an OIDC issuer. The issuer
// must emit RS256 tokens with sub, organization_id, role, aud and exp claims.
// The HTTP endpoint is cached and refreshed on an unknown key id so key
// rotation does not require a process restart.
type JWKSConfig struct {
	Issuer     string
	Audience   string
	JWKSURL    string
	ClockSkew  time.Duration
	HTTPClient *http.Client
}

type JWKSAuthenticator struct {
	issuer    string
	audience  string
	jwksURL   string
	clockSkew time.Duration
	client    *http.Client

	mu          sync.RWMutex
	keys        map[string]*rsa.PublicKey
	refreshedAt time.Time
}

func NewJWKSAuthenticator(config JWKSConfig) (*JWKSAuthenticator, error) {
	if strings.TrimSpace(config.Issuer) == "" || strings.TrimSpace(config.Audience) == "" || strings.TrimSpace(config.JWKSURL) == "" {
		return nil, errors.New("issuer, audience and JWKS URL are required")
	}
	issuer, err := url.Parse(config.Issuer)
	if err != nil || issuer.Scheme != "https" || issuer.Host == "" || issuer.User != nil || issuer.RawQuery != "" || issuer.Fragment != "" {
		return nil, errors.New("issuer must be an exact HTTPS URL")
	}
	jwks, err := url.Parse(config.JWKSURL)
	if err != nil || (jwks.Scheme != "https" && jwks.Scheme != "http") || jwks.Host == "" || jwks.User != nil || jwks.Fragment != "" {
		return nil, errors.New("JWKS URL must be an absolute HTTP(S) URL")
	}
	if config.ClockSkew <= 0 || config.ClockSkew > 5*time.Minute {
		return nil, errors.New("clock skew must be between 1ns and 5m")
	}
	client := config.HTTPClient
	if client == nil {
		client = &http.Client{
			Timeout: 5 * time.Second,
			CheckRedirect: func(request *http.Request, via []*http.Request) error {
				if len(via) == 0 {
					return nil
				}
				previous := via[len(via)-1].URL
				if request.URL.Scheme != previous.Scheme || request.URL.Host != previous.Host {
					return http.ErrUseLastResponse
				}
				return nil
			},
		}
	}
	return &JWKSAuthenticator{
		issuer: config.Issuer, audience: config.Audience, jwksURL: config.JWKSURL,
		clockSkew: config.ClockSkew, client: client,
		keys: make(map[string]*rsa.PublicKey),
	}, nil
}

func (a *JWKSAuthenticator) Authenticate(ctx context.Context, request *http.Request) (Principal, error) {
	if request == nil {
		return Principal{}, errInvalidToken
	}
	values := request.Header.Values("Authorization")
	if len(values) != 1 || !strings.HasPrefix(values[0], "Bearer ") {
		return Principal{}, errInvalidToken
	}
	token := strings.TrimSpace(strings.TrimPrefix(values[0], "Bearer "))
	if token == "" || len(token) > maxJWTSize || strings.ContainsAny(token, " \t\r\n") {
		return Principal{}, errInvalidToken
	}

	header, claims, signed, signature, err := parseJWT(token)
	if err != nil || header.Algorithm != "RS256" || header.KeyID == "" {
		return Principal{}, errInvalidToken
	}
	key, err := a.publicKey(ctx, header.KeyID)
	if err != nil {
		return Principal{}, errInvalidToken
	}
	digest := sha256.Sum256([]byte(signed))
	if err := rsa.VerifyPKCS1v15(key, crypto.SHA256, digest[:], signature); err != nil {
		return Principal{}, errInvalidToken
	}
	if err := a.validateClaims(claims, time.Now()); err != nil {
		return Principal{}, errInvalidToken
	}
	principal := Principal{
		UserID:         core.ID(claims.Subject),
		OrganizationID: core.ID(claims.OrganizationID),
		Role:           Role(claims.Role),
	}
	if err := principal.Validate(); err != nil {
		return Principal{}, errInvalidToken
	}
	return principal, nil
}

type jwtHeader struct {
	Algorithm string
	KeyID     string
}

type jwtClaims struct {
	Issuer         string
	Audience       []string
	Subject        string
	OrganizationID string
	Role           string
	ExpiresAt      float64
	NotBefore      *float64
	IssuedAt       *float64
}

func parseJWT(token string) (jwtHeader, jwtClaims, string, []byte, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 || parts[0] == "" || parts[1] == "" || parts[2] == "" {
		return jwtHeader{}, jwtClaims{}, "", nil, errInvalidToken
	}
	decode := func(value string) ([]byte, error) {
		return base64.RawURLEncoding.DecodeString(value)
	}
	headerBytes, err := decode(parts[0])
	if err != nil || len(headerBytes) > 4096 {
		return jwtHeader{}, jwtClaims{}, "", nil, errInvalidToken
	}
	var rawHeader map[string]json.RawMessage
	if json.Unmarshal(headerBytes, &rawHeader) != nil {
		return jwtHeader{}, jwtClaims{}, "", nil, errInvalidToken
	}
	algorithm, ok := rawString(rawHeader, "alg")
	if !ok {
		return jwtHeader{}, jwtClaims{}, "", nil, errInvalidToken
	}
	keyID, ok := rawString(rawHeader, "kid")
	if !ok {
		return jwtHeader{}, jwtClaims{}, "", nil, errInvalidToken
	}
	claimBytes, err := decode(parts[1])
	if err != nil || len(claimBytes) > 12<<10 {
		return jwtHeader{}, jwtClaims{}, "", nil, errInvalidToken
	}
	claims, err := decodeClaims(claimBytes)
	if err != nil {
		return jwtHeader{}, jwtClaims{}, "", nil, errInvalidToken
	}
	signature, err := decode(parts[2])
	if err != nil || len(signature) < 64 || len(signature) > 1024 {
		return jwtHeader{}, jwtClaims{}, "", nil, errInvalidToken
	}
	return jwtHeader{Algorithm: algorithm, KeyID: keyID}, claims, parts[0] + "." + parts[1], signature, nil
}

func decodeClaims(data []byte) (jwtClaims, error) {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return jwtClaims{}, err
	}
	issuer, ok := rawString(raw, "iss")
	if !ok {
		return jwtClaims{}, errInvalidToken
	}
	subject, ok := rawString(raw, "sub")
	if !ok {
		return jwtClaims{}, errInvalidToken
	}
	organizationID, ok := rawString(raw, "organization_id")
	if !ok {
		return jwtClaims{}, errInvalidToken
	}
	role, ok := rawString(raw, "role")
	if !ok {
		return jwtClaims{}, errInvalidToken
	}
	audience, ok := rawAudience(raw["aud"])
	if !ok || len(audience) == 0 {
		return jwtClaims{}, errInvalidToken
	}
	expiresAt, ok := rawNumber(raw, "exp")
	if !ok {
		return jwtClaims{}, errInvalidToken
	}
	claims := jwtClaims{Issuer: issuer, Audience: audience, Subject: subject, OrganizationID: organizationID, Role: role, ExpiresAt: expiresAt}
	if value, present := raw["nbf"]; present {
		nbf, err := number(value)
		if err != nil {
			return jwtClaims{}, errInvalidToken
		}
		claims.NotBefore = &nbf
	}
	if value, present := raw["iat"]; present {
		iat, err := number(value)
		if err != nil {
			return jwtClaims{}, errInvalidToken
		}
		claims.IssuedAt = &iat
	}
	return claims, nil
}

func rawString(values map[string]json.RawMessage, key string) (string, bool) {
	value, ok := values[key]
	if !ok {
		return "", false
	}
	var result string
	if json.Unmarshal(value, &result) != nil || strings.TrimSpace(result) == "" {
		return "", false
	}
	return result, true
}

func rawAudience(value json.RawMessage) ([]string, bool) {
	if len(value) == 0 {
		return nil, false
	}
	var one string
	if json.Unmarshal(value, &one) == nil && one != "" {
		return []string{one}, true
	}
	var many []string
	if json.Unmarshal(value, &many) != nil || len(many) == 0 {
		return nil, false
	}
	for _, item := range many {
		if item == "" {
			return nil, false
		}
	}
	return many, true
}

func rawNumber(values map[string]json.RawMessage, key string) (float64, bool) {
	value, ok := values[key]
	if !ok {
		return 0, false
	}
	result, err := number(value)
	return result, err == nil
}

func number(value json.RawMessage) (float64, error) {
	var result json.Number
	if err := json.Unmarshal(value, &result); err != nil {
		return 0, err
	}
	numberValue, err := result.Float64()
	if err != nil || math.IsNaN(numberValue) || math.IsInf(numberValue, 0) {
		return 0, errInvalidToken
	}
	return numberValue, nil
}

func (a *JWKSAuthenticator) validateClaims(claims jwtClaims, now time.Time) error {
	if claims.Issuer != a.issuer || !contains(claims.Audience, a.audience) {
		return errInvalidToken
	}
	nowSeconds := float64(now.UnixNano()) / float64(time.Second)
	clockSkew := a.clockSkew.Seconds()
	if claims.ExpiresAt < nowSeconds-clockSkew {
		return errInvalidToken
	}
	if claims.NotBefore != nil && *claims.NotBefore > nowSeconds+clockSkew {
		return errInvalidToken
	}
	if claims.IssuedAt != nil && *claims.IssuedAt > nowSeconds+clockSkew {
		return errInvalidToken
	}
	return nil
}

func contains(values []string, expected string) bool {
	for _, value := range values {
		if value == expected {
			return true
		}
	}
	return false
}

func (a *JWKSAuthenticator) publicKey(ctx context.Context, keyID string) (*rsa.PublicKey, error) {
	if key, ok := a.cachedKey(keyID); ok {
		return key, nil
	}
	if err := a.refresh(ctx); err != nil {
		return nil, err
	}
	if key, ok := a.cachedKey(keyID); ok {
		return key, nil
	}
	return nil, errInvalidToken
}

func (a *JWKSAuthenticator) cachedKey(keyID string) (*rsa.PublicKey, bool) {
	a.mu.RLock()
	defer a.mu.RUnlock()
	if time.Since(a.refreshedAt) >= jwksCacheLifetime {
		return nil, false
	}
	key, ok := a.keys[keyID]
	return key, ok
}

func (a *JWKSAuthenticator) refresh(ctx context.Context) error {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, a.jwksURL, nil)
	if err != nil {
		return err
	}
	request.Header.Set("Accept", "application/json")
	response, err := a.client.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK || response.ContentLength > maxJWKSSize {
		return fmt.Errorf("JWKS endpoint returned status %d", response.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, maxJWKSSize+1))
	if err != nil {
		return err
	}
	if len(body) > maxJWKSSize {
		return errInvalidToken
	}
	var document struct {
		Keys []struct {
			KeyType   string `json:"kty"`
			KeyID     string `json:"kid"`
			Algorithm string `json:"alg"`
			Use       string `json:"use"`
			Modulus   string `json:"n"`
			Exponent  string `json:"e"`
		} `json:"keys"`
	}
	if err := json.Unmarshal(body, &document); err != nil || len(document.Keys) == 0 {
		return errInvalidToken
	}
	keys := make(map[string]*rsa.PublicKey, len(document.Keys))
	for _, item := range document.Keys {
		if item.KeyType != "RSA" || item.KeyID == "" || (item.Algorithm != "" && item.Algorithm != "RS256") || (item.Use != "" && item.Use != "sig") {
			continue
		}
		modulus, decodeErr := base64.RawURLEncoding.DecodeString(item.Modulus)
		if decodeErr != nil || len(modulus) < 256 || len(modulus) > 1024 {
			continue
		}
		exponentBytes, decodeErr := base64.RawURLEncoding.DecodeString(item.Exponent)
		if decodeErr != nil || len(exponentBytes) == 0 || len(exponentBytes) > 4 {
			continue
		}
		exponent := 0
		for _, value := range exponentBytes {
			exponent = exponent<<8 | int(value)
		}
		if exponent < 3 || exponent%2 == 0 {
			continue
		}
		keys[item.KeyID] = &rsa.PublicKey{N: new(big.Int).SetBytes(modulus), E: exponent}
	}
	if len(keys) == 0 {
		return errInvalidToken
	}
	a.mu.Lock()
	a.keys = keys
	a.refreshedAt = time.Now()
	a.mu.Unlock()
	return nil
}
