package auth

import (
	"context"
	"errors"
	"net/http"
)

var ErrNotConfigured = errors.New("authenticator is not configured")

type Authenticator interface {
	Authenticate(context.Context, *http.Request) (Principal, error)
}
