package auth

import (
	"context"
	"crypto"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"strings"
	"sync"
	"time"
)

func verifyRS256(pub *rsa.PublicKey, signed, sig []byte) error {
	h := sha256.Sum256(signed)
	return rsa.VerifyPKCS1v15(pub, crypto.SHA256, h[:], sig)
}

// GoogleVerifier verifies Google-issued ID tokens by fetching Google's
// public JWKs and validating signature + standard claims (iss, aud, exp).
//
// It does no token caching beyond the JWK set itself.
type GoogleVerifier struct {
	clientID string
	jwksURL  string
	http     *http.Client

	mu      sync.RWMutex
	keys    map[string]*rsa.PublicKey
	expires time.Time
}

const defaultJWKSURL = "https://www.googleapis.com/oauth2/v3/certs"

func NewGoogleVerifier(clientID string) *GoogleVerifier {
	return &GoogleVerifier{
		clientID: clientID,
		jwksURL:  defaultJWKSURL,
		http:     &http.Client{Timeout: 10 * time.Second},
		keys:     map[string]*rsa.PublicKey{},
	}
}

func (v *GoogleVerifier) Verify(ctx context.Context, idToken string) (*User, error) {
	parts := strings.Split(idToken, ".")
	if len(parts) != 3 {
		return nil, ErrInvalidToken
	}
	headerJSON, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return nil, fmt.Errorf("%w: header b64", ErrInvalidToken)
	}
	var hdr struct {
		Alg string `json:"alg"`
		Kid string `json:"kid"`
	}
	if err := json.Unmarshal(headerJSON, &hdr); err != nil {
		return nil, fmt.Errorf("%w: header json", ErrInvalidToken)
	}
	if hdr.Alg != "RS256" {
		return nil, fmt.Errorf("%w: unsupported alg %q", ErrInvalidToken, hdr.Alg)
	}

	key, err := v.getKey(ctx, hdr.Kid)
	if err != nil {
		return nil, err
	}
	signingInput := []byte(parts[0] + "." + parts[1])
	sig, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		return nil, fmt.Errorf("%w: sig b64", ErrInvalidToken)
	}
	if err := verifyRS256(key, signingInput, sig); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidToken, err)
	}

	payloadJSON, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, fmt.Errorf("%w: payload b64", ErrInvalidToken)
	}
	var claims struct {
		Iss           string `json:"iss"`
		Aud           string `json:"aud"`
		Sub           string `json:"sub"`
		Email         string `json:"email"`
		EmailVerified bool   `json:"email_verified"`
		Name          string `json:"name"`
		Exp           int64  `json:"exp"`
	}
	if err := json.Unmarshal(payloadJSON, &claims); err != nil {
		return nil, fmt.Errorf("%w: payload json", ErrInvalidToken)
	}
	if claims.Iss != "https://accounts.google.com" && claims.Iss != "accounts.google.com" {
		return nil, fmt.Errorf("%w: bad issuer %q", ErrInvalidToken, claims.Iss)
	}
	if v.clientID != "" && claims.Aud != v.clientID {
		return nil, fmt.Errorf("%w: audience mismatch", ErrInvalidToken)
	}
	if claims.Exp == 0 || time.Now().Unix() >= claims.Exp {
		return nil, fmt.Errorf("%w: expired", ErrInvalidToken)
	}
	if claims.Sub == "" {
		return nil, fmt.Errorf("%w: missing sub", ErrInvalidToken)
	}
	return &User{Sub: claims.Sub, Email: claims.Email, Name: claims.Name}, nil
}

func (v *GoogleVerifier) getKey(ctx context.Context, kid string) (*rsa.PublicKey, error) {
	v.mu.RLock()
	if time.Now().Before(v.expires) {
		if k, ok := v.keys[kid]; ok {
			v.mu.RUnlock()
			return k, nil
		}
	}
	v.mu.RUnlock()

	v.mu.Lock()
	defer v.mu.Unlock()
	if time.Now().Before(v.expires) {
		if k, ok := v.keys[kid]; ok {
			return k, nil
		}
	}
	if err := v.refreshLocked(ctx); err != nil {
		return nil, err
	}
	k, ok := v.keys[kid]
	if !ok {
		return nil, fmt.Errorf("%w: unknown kid %q", ErrInvalidToken, kid)
	}
	return k, nil
}

func (v *GoogleVerifier) refreshLocked(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, v.jwksURL, nil)
	if err != nil {
		return err
	}
	resp, err := v.http.Do(req)
	if err != nil {
		return fmt.Errorf("fetch jwks: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("fetch jwks: status %d", resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	var set struct {
		Keys []struct {
			Kid string `json:"kid"`
			Kty string `json:"kty"`
			N   string `json:"n"`
			E   string `json:"e"`
		} `json:"keys"`
	}
	if err := json.Unmarshal(body, &set); err != nil {
		return fmt.Errorf("parse jwks: %w", err)
	}
	keys := make(map[string]*rsa.PublicKey, len(set.Keys))
	for _, k := range set.Keys {
		if k.Kty != "RSA" {
			continue
		}
		nBytes, err := base64.RawURLEncoding.DecodeString(k.N)
		if err != nil {
			continue
		}
		eBytes, err := base64.RawURLEncoding.DecodeString(k.E)
		if err != nil {
			continue
		}
		var e int
		switch len(eBytes) {
		case 3:
			e = int(uint32(eBytes[0])<<16 | uint32(eBytes[1])<<8 | uint32(eBytes[2]))
		case 4:
			e = int(binary.BigEndian.Uint32(eBytes))
		default:
			continue
		}
		keys[k.Kid] = &rsa.PublicKey{
			N: new(big.Int).SetBytes(nBytes),
			E: e,
		}
	}
	if len(keys) == 0 {
		return errors.New("jwks: no usable keys")
	}
	v.keys = keys
	v.expires = time.Now().Add(1 * time.Hour)
	return nil
}
