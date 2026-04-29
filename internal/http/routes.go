package http

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/kongken/urlo/internal/url"
	urlov1 "github.com/kongken/urlo/pkg/proto/urlo/v1"
)

// RegisterRoutes wires the URL shortener HTTP API onto r, backed by svc.
//
// Routes:
//
//	POST   /api/v1/urls              -> Shorten
//	GET    /api/v1/urls/:code        -> Resolve
//	GET    /api/v1/urls/:code/stats  -> GetStats
//	DELETE /api/v1/urls/:code        -> Delete
//	GET    /:code                    -> 302 to the long URL
func RegisterRoutes(r *gin.Engine, svc *url.Service) {
	r.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "healthy"})
	})

	api := r.Group("/api/v1")
	api.POST("/urls", handleShorten(svc))
	api.GET("/urls/:code", handleResolve(svc))
	api.GET("/urls/:code/stats", handleGetStats(svc))
	api.DELETE("/urls/:code", handleDelete(svc))

	r.GET("/:code", handleRedirect(svc))
}

type shortenRequest struct {
	LongURL    string `json:"long_url"`
	CustomCode string `json:"custom_code,omitempty"`
	TTLSeconds int64  `json:"ttl_seconds,omitempty"`
}

func handleShorten(svc *url.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		var body shortenRequest
		if err := c.ShouldBindJSON(&body); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"error":   "invalid_body",
				"message": err.Error(),
			})
			return
		}
		resp, err := svc.Shorten(c.Request.Context(), &urlov1.ShortenRequest{
			LongUrl:    body.LongURL,
			CustomCode: body.CustomCode,
			TtlSeconds: body.TTLSeconds,
		})
		if err != nil {
			writeStatusError(c, err)
			return
		}
		c.JSON(http.StatusCreated, resp.GetLink())
	}
}

func handleResolve(svc *url.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		resp, err := svc.Resolve(c.Request.Context(), &urlov1.ResolveRequest{
			Code: c.Param("code"),
		})
		if err != nil {
			writeStatusError(c, err)
			return
		}
		c.JSON(http.StatusOK, resp.GetLink())
	}
}

func handleGetStats(svc *url.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		resp, err := svc.GetStats(c.Request.Context(), &urlov1.GetStatsRequest{
			Code: c.Param("code"),
		})
		if err != nil {
			writeStatusError(c, err)
			return
		}
		c.JSON(http.StatusOK, resp.GetLink())
	}
}

func handleDelete(svc *url.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		if _, err := svc.Delete(c.Request.Context(), &urlov1.DeleteRequest{
			Code: c.Param("code"),
		}); err != nil {
			writeStatusError(c, err)
			return
		}
		c.Status(http.StatusNoContent)
	}
}

func handleRedirect(svc *url.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		resp, err := svc.Resolve(c.Request.Context(), &urlov1.ResolveRequest{
			Code: c.Param("code"),
		})
		if err != nil {
			writeStatusError(c, err)
			return
		}
		c.Redirect(http.StatusFound, resp.GetLink().GetLongUrl())
	}
}

// writeStatusError translates a gRPC status error into a JSON HTTP response.
func writeStatusError(c *gin.Context, err error) {
	st, ok := status.FromError(err)
	if !ok {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "internal",
			"message": err.Error(),
		})
		return
	}
	httpCode := codeToHTTP(st.Code())
	c.JSON(httpCode, gin.H{
		"error":   st.Code().String(),
		"message": st.Message(),
	})
}

func codeToHTTP(c codes.Code) int {
	switch c {
	case codes.OK:
		return http.StatusOK
	case codes.InvalidArgument:
		return http.StatusBadRequest
	case codes.NotFound:
		return http.StatusNotFound
	case codes.AlreadyExists:
		return http.StatusConflict
	case codes.PermissionDenied:
		return http.StatusForbidden
	case codes.Unauthenticated:
		return http.StatusUnauthorized
	case codes.ResourceExhausted:
		return http.StatusTooManyRequests
	case codes.FailedPrecondition:
		return http.StatusPreconditionFailed
	case codes.Unavailable:
		return http.StatusServiceUnavailable
	case codes.DeadlineExceeded:
		return http.StatusGatewayTimeout
	default:
		return http.StatusInternalServerError
	}
}
