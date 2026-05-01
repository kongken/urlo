// Package auth provides Google ID-token verification and a JWT-cookie
// session for the urlo HTTP API.
//
// The flow:
//
//  1. Frontend obtains a Google ID token via Google Identity Services and
//     POSTs it to /api/v1/auth/google.
//  2. The handler calls Verifier.Verify, then issues a session JWT via
//     Sessions.Issue and writes it as an HttpOnly cookie.
//  3. Subsequent requests include the cookie; Middleware decodes it and
//     puts the *User on the gin.Context.
package auth

import (
	"context"
	"errors"
	"time"
)

// User is the authenticated principal extracted from a verified Google
// ID token (or a session JWT).
type User struct {
	Sub   string `json:"sub"`             // stable Google user id
	Email string `json:"email,omitempty"` // verified email
	Name  string `json:"name,omitempty"`
}

// Verifier verifies a Google-issued ID token.
type Verifier interface {
	Verify(ctx context.Context, idToken string) (*User, error)
}

// ErrInvalidToken is returned by Verify / DecodeSession on bad input.
var ErrInvalidToken = errors.New("auth: invalid token")

// Sessions issues and decodes signed session tokens.
type Sessions interface {
	Issue(u *User, ttl time.Duration) (string, error)
	Decode(token string) (*User, error)
}
