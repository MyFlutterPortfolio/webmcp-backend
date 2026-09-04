package auth

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"math/big"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestJWKSAuthenticatorAcceptsValidTenantToken(t *testing.T) {
	key, authenticator, closeServer := testAuthenticator(t, "key-1")
	defer closeServer()
	token := signedToken(t, key, "key-1", map[string]any{
		"iss": "https://issuer.example.com/tenant", "aud": "webmcp-api", "sub": "user-1",
		"organization_id": "org-1", "role": "operator", "exp": time.Now().Add(time.Minute).Unix(),
	})
	request := httptest.NewRequest(http.MethodGet, "/api/v1/businesses/b-1/snapshot", nil)
	request.Header.Set("Authorization", "Bearer "+token)

	principal, err := authenticator.Authenticate(request.Context(), request)
	if err != nil {
		t.Fatalf("expected valid token, got %v", err)
	}
	if principal.UserID != "user-1" || principal.OrganizationID != "org-1" || principal.Role != RoleOperator {
		t.Fatalf("unexpected principal: %#v", principal)
	}
}

func TestJWKSAuthenticatorRejectsInvalidClaimsAndAuthorization(t *testing.T) {
	key, authenticator, closeServer := testAuthenticator(t, "key-1")
	defer closeServer()
	baseClaims := map[string]any{
		"iss": "https://issuer.example.com/tenant", "aud": "webmcp-api", "sub": "user-1",
		"organization_id": "org-1", "role": "operator", "exp": time.Now().Add(time.Minute).Unix(),
	}
	tests := []struct {
		name   string
		header string
		claims map[string]any
	}{
		{name: "missing bearer", claims: baseClaims},
		{name: "wrong issuer", header: "Bearer ", claims: withClaim(baseClaims, "iss", "https://other.example.com")},
		{name: "expired", header: "Bearer ", claims: withClaim(baseClaims, "exp", time.Now().Add(-time.Minute).Unix())},
		{name: "unknown role", header: "Bearer ", claims: withClaim(baseClaims, "role", "admin")},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodGet, "/", nil)
			if test.header != "" {
				token := signedToken(t, key, "key-1", test.claims)
				request.Header.Set("Authorization", test.header+token)
			}
			if _, err := authenticator.Authenticate(request.Context(), request); err == nil {
				t.Fatal("expected token rejection")
			}
		})
	}

	duplicate := httptest.NewRequest(http.MethodGet, "/", nil)
	token := signedToken(t, key, "key-1", baseClaims)
	duplicate.Header.Add("Authorization", "Bearer "+token)
	duplicate.Header.Add("Authorization", "Bearer "+token)
	if _, err := authenticator.Authenticate(duplicate.Context(), duplicate); err == nil {
		t.Fatal("expected duplicate authorization headers to be rejected")
	}
}

func TestJWKSAuthenticatorRefreshesForRotatedKey(t *testing.T) {
	first, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	second, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	rotated := false
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		key := first
		kid := "key-1"
		if rotated {
			key, kid = second, "key-2"
		}
		_, _ = writer.Write(jwksJSON(t, key, kid))
	}))
	defer server.Close()
	authenticator, err := NewJWKSAuthenticator(JWKSConfig{Issuer: "https://issuer.example.com/tenant", Audience: "webmcp-api", JWKSURL: server.URL, ClockSkew: time.Minute})
	if err != nil {
		t.Fatal(err)
	}
	firstToken := signedToken(t, first, "key-1", validClaims())
	firstRequest := httptest.NewRequest(http.MethodGet, "/", nil)
	firstRequest.Header.Set("Authorization", "Bearer "+firstToken)
	if _, err := authenticator.Authenticate(firstRequest.Context(), firstRequest); err != nil {
		t.Fatalf("first key was rejected: %v", err)
	}

	rotated = true
	rotatedToken := signedToken(t, second, "key-2", validClaims())
	rotatedRequest := httptest.NewRequest(http.MethodGet, "/", nil)
	rotatedRequest.Header.Set("Authorization", "Bearer "+rotatedToken)
	if _, err := authenticator.Authenticate(rotatedRequest.Context(), rotatedRequest); err != nil {
		t.Fatalf("rotated key was not loaded after unknown kid: %v", err)
	}
}

func testAuthenticator(t *testing.T, kid string) (*rsa.PrivateKey, *JWKSAuthenticator, func()) {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		_, _ = writer.Write(jwksJSON(t, key, kid))
	}))
	authenticator, err := NewJWKSAuthenticator(JWKSConfig{Issuer: "https://issuer.example.com/tenant", Audience: "webmcp-api", JWKSURL: server.URL, ClockSkew: time.Minute})
	if err != nil {
		server.Close()
		t.Fatal(err)
	}
	return key, authenticator, server.Close
}

func signedToken(t *testing.T, key *rsa.PrivateKey, kid string, claims map[string]any) string {
	t.Helper()
	header := map[string]string{"alg": "RS256", "kid": kid, "typ": "JWT"}
	encode := func(value any) string {
		data, err := json.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		return base64.RawURLEncoding.EncodeToString(data)
	}
	signed := encode(header) + "." + encode(claims)
	digest := sha256.Sum256([]byte(signed))
	signature, err := rsa.SignPKCS1v15(rand.Reader, key, crypto.SHA256, digest[:])
	if err != nil {
		t.Fatal(err)
	}
	return signed + "." + base64.RawURLEncoding.EncodeToString(signature)
}

func jwksJSON(t *testing.T, key *rsa.PrivateKey, kid string) []byte {
	t.Helper()
	encode := func(data []byte) string { return base64.RawURLEncoding.EncodeToString(data) }
	document := map[string]any{"keys": []map[string]string{{
		"kty": "RSA", "kid": kid, "alg": "RS256", "use": "sig",
		"n": encode(key.PublicKey.N.Bytes()), "e": encode(big.NewInt(int64(key.PublicKey.E)).Bytes()),
	}}}
	data, err := json.Marshal(document)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func validClaims() map[string]any {
	return map[string]any{"iss": "https://issuer.example.com/tenant", "aud": []string{"webmcp-api"}, "sub": "user-1", "organization_id": "org-1", "role": "operator", "exp": time.Now().Add(time.Minute).Unix()}
}

func withClaim(claims map[string]any, key string, value any) map[string]any {
	copy := make(map[string]any, len(claims)+1)
	for claim, item := range claims {
		copy[claim] = item
	}
	copy[key] = value
	return copy
}
