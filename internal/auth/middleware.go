package auth

import (
	"context"
	"net/http"

	"github.com/gin-gonic/gin"
)

const (
	contextUserKey = "auth.user"
)

// Middleware reads the session cookie and, if valid, attaches the *User to
// the request context. With required=true, requests without a valid
// session are rejected with 401.
func Middleware(sessions Sessions, cookieName string, required bool) gin.HandlerFunc {
	return func(c *gin.Context) {
		var u *User
		if cookie, err := c.Cookie(cookieName); err == nil && cookie != "" {
			if user, derr := sessions.Decode(cookie); derr == nil {
				u = user
			}
		}
		if u == nil && required {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"error":   "unauthenticated",
				"message": "login required",
			})
			return
		}
		if u != nil {
			c.Set(contextUserKey, u)
			ctx := WithUser(c.Request.Context(), u)
			c.Request = c.Request.WithContext(ctx)
		}
		c.Next()
	}
}

type ctxKey struct{}

// WithUser returns a copy of ctx carrying u.
func WithUser(ctx context.Context, u *User) context.Context {
	return context.WithValue(ctx, ctxKey{}, u)
}

// FromContext returns the User on ctx, or nil if absent.
func FromContext(ctx context.Context) *User {
	u, _ := ctx.Value(ctxKey{}).(*User)
	return u
}

// FromGin returns the User attached to a gin.Context by Middleware.
func FromGin(c *gin.Context) *User {
	v, ok := c.Get(contextUserKey)
	if !ok {
		return nil
	}
	u, _ := v.(*User)
	return u
}
