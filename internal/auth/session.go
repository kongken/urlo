package auth

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

// HMACSessions issues and verifies HS256-signed JWTs with the urlo session
// claims. Implemented inline to avoid pulling a JWT dependency for such
// a small surface area.
type HMACSessions struct {
	secret []byte
}

func NewHMACSessions(secret string) (*HMACSessions, error) {
	if len(secret) < 16 {
		return nil, errors.New("auth: session secret must be at least 16 chars")
	}
	return &HMACSessions{secret: []byte(secret)}, nil
}

type sessionClaims struct {
	Sub   string `json:"sub"`
	Email string `json:"email,omitempty"`
	Name  string `json:"name,omitempty"`
	Exp   int64  `json:"exp"`
	Iat   int64  `json:"iat"`
}

func (s *HMACSessions) Issue(u *User, ttl time.Duration) (string, error) {
	if u == nil || u.Sub == "" {
		return "", errors.New("auth: user.sub is required")
	}
	if ttl <= 0 {
		ttl = 7 * 24 * time.Hour
	}
	now := time.Now()
	claims := sessionClaims{
		Sub:   u.Sub,
		Email: u.Email,
		Name:  u.Name,
		Iat:   now.Unix(),
		Exp:   now.Add(ttl).Unix(),
	}
	header := `{"alg":"HS256","typ":"JWT"}`
	payload, err := json.Marshal(claims)
	if err != nil {
		return "", err
	}
	h := base64.RawURLEncoding.EncodeToString([]byte(header))
	p := base64.RawURLEncoding.EncodeToString(payload)
	signing := h + "." + p
	mac := hmac.New(sha256.New, s.secret)
	mac.Write([]byte(signing))
	sig := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	return signing + "." + sig, nil
}

func (s *HMACSessions) Decode(token string) (*User, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return nil, ErrInvalidToken
	}
	signing := parts[0] + "." + parts[1]
	mac := hmac.New(sha256.New, s.secret)
	mac.Write([]byte(signing))
	expected := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	if !hmac.Equal([]byte(expected), []byte(parts[2])) {
		return nil, ErrInvalidToken
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, ErrInvalidToken
	}
	var c sessionClaims
	if err := json.Unmarshal(payload, &c); err != nil {
		return nil, ErrInvalidToken
	}
	if c.Exp == 0 || time.Now().Unix() >= c.Exp {
		return nil, fmt.Errorf("%w: expired", ErrInvalidToken)
	}
	if c.Sub == "" {
		return nil, ErrInvalidToken
	}
	return &User{Sub: c.Sub, Email: c.Email, Name: c.Name}, nil
}

