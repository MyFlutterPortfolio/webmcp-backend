package auth

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"webmcp-backend/internal/domain/core"
)

const guestTokenPrefix = "guest"
const maxGuestAuthorizationBytes = 8192

type GuestAuthenticator struct {
	secret         []byte
	userID         core.ID
	organizationID core.ID
	businessID     core.ID
	goalID         core.ID
	ttl            time.Duration
}

type guestClaims struct {
	Subject        string `json:"sub"`
	OrganizationID string `json:"org"`
	BusinessID     string `json:"business"`
	GoalID         string `json:"goal"`
	IssuedAt       int64  `json:"iat"`
	ExpiresAt      int64  `json:"exp"`
}

func NewGuestAuthenticator(secret string, userID, organizationID, businessID, goalID core.ID, ttl time.Duration) (*GuestAuthenticator, error) {
	if len(secret) < 32 || ttl < time.Minute || ttl > 30*time.Minute {
		return nil, errors.New("guest secret or token TTL is invalid")
	}
	if err := userID.Validate("user_id"); err != nil {
		return nil, err
	}
	if err := organizationID.Validate("organization_id"); err != nil {
		return nil, err
	}
	if err := businessID.Validate("business_id"); err != nil {
		return nil, err
	}
	if err := goalID.Validate("goal_id"); err != nil {
		return nil, err
	}
	return &GuestAuthenticator{secret: []byte(secret), userID: userID, organizationID: organizationID, businessID: businessID, goalID: goalID, ttl: ttl}, nil
}

func (a *GuestAuthenticator) IssueGuestToken() (string, time.Time, error) {
	now := time.Now().UTC()
	claims := guestClaims{Subject: string(a.userID), OrganizationID: string(a.organizationID), BusinessID: string(a.businessID), GoalID: string(a.goalID), IssuedAt: now.Unix(), ExpiresAt: now.Add(a.ttl).Unix()}
	payload, err := json.Marshal(claims)
	if err != nil {
		return "", time.Time{}, err
	}
	encoded := base64.RawURLEncoding.EncodeToString(payload)
	signature := a.sign(encoded)
	return guestTokenPrefix + "." + encoded + "." + signature, now.Add(a.ttl), nil
}

func (a *GuestAuthenticator) GuestBusinessID() string { return string(a.businessID) }
func (a *GuestAuthenticator) GuestGoalID() string     { return string(a.goalID) }

func (a *GuestAuthenticator) Authenticate(_ context.Context, request *http.Request) (Principal, error) {
	if request == nil {
		return Principal{}, errors.New("guest token is invalid")
	}
	values := request.Header.Values("Authorization")
	if len(values) != 1 || len(values[0]) > maxGuestAuthorizationBytes || !strings.HasPrefix(values[0], "Bearer ") {
		return Principal{}, errors.New("guest token is invalid")
	}
	parts := strings.Split(strings.TrimSpace(strings.TrimPrefix(values[0], "Bearer ")), ".")
	if len(parts) != 3 || parts[0] != guestTokenPrefix || parts[1] == "" || parts[2] == "" || !hmac.Equal([]byte(parts[2]), []byte(a.sign(parts[1]))) {
		return Principal{}, errors.New("guest token is invalid")
	}
	data, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil || len(data) > 2048 {
		return Principal{}, errors.New("guest token is invalid")
	}
	var claims guestClaims
	if json.Unmarshal(data, &claims) != nil || claims.Subject != string(a.userID) || claims.OrganizationID != string(a.organizationID) || claims.BusinessID != string(a.businessID) || claims.GoalID != string(a.goalID) {
		return Principal{}, errors.New("guest token is invalid")
	}
	now := time.Now().Unix()
	if claims.IssuedAt <= 0 || claims.ExpiresAt <= claims.IssuedAt || claims.ExpiresAt < now || claims.IssuedAt > now+30 {
		return Principal{}, errors.New("guest token is invalid")
	}
	principal := Principal{UserID: a.userID, OrganizationID: a.organizationID, Role: RoleViewer, Guest: true, GuestBusinessID: a.businessID, GuestGoalID: a.goalID}
	if err := principal.Validate(); err != nil {
		return Principal{}, errors.New("guest token is invalid")
	}
	return principal, nil
}

func (a *GuestAuthenticator) sign(payload string) string {
	hash := hmac.New(sha256.New, a.secret)
	_, _ = hash.Write([]byte(payload))
	return base64.RawURLEncoding.EncodeToString(hash.Sum(nil))
}

type chainedAuthenticator []Authenticator

func ChainAuthenticators(authenticators ...Authenticator) Authenticator {
	valid := make(chainedAuthenticator, 0, len(authenticators))
	for _, authenticator := range authenticators {
		if authenticator != nil {
			valid = append(valid, authenticator)
		}
	}
	return valid
}

func (c chainedAuthenticator) Authenticate(ctx context.Context, request *http.Request) (Principal, error) {
	for _, authenticator := range c {
		if principal, err := authenticator.Authenticate(ctx, request); err == nil {
			return principal, nil
		}
	}
	return Principal{}, errors.New("authentication failed")
}
