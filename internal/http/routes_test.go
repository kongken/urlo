package http

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/kongken/urlo/internal/url"
)

func newTestRouter() (*gin.Engine, *url.Service) {
	gin.SetMode(gin.TestMode)
	svc := url.NewService(url.Options{BaseURL: "https://urlo.example"})
	r := gin.New()
	RegisterRoutes(r, svc)
	return r, svc
}

func doJSON(t *testing.T, r http.Handler, method, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var buf bytes.Buffer
	if body != nil {
		if err := json.NewEncoder(&buf).Encode(body); err != nil {
			t.Fatalf("encode body: %v", err)
		}
	}
	req := httptest.NewRequest(method, path, &buf)
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)
	return rr
}

func decode[T any](t *testing.T, rr *httptest.ResponseRecorder) T {
	t.Helper()
	var out T
	if err := json.Unmarshal(rr.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode body %q: %v", rr.Body.String(), err)
	}
	return out
}

type linkResp struct {
	Code       string `json:"code"`
	LongURL    string `json:"long_url"`
	ShortURL   string `json:"short_url"`
	VisitCount int64  `json:"visit_count"`
}

func TestHealth(t *testing.T) {
	r, _ := newTestRouter()
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
}

func TestShortenAndResolve(t *testing.T) {
	r, _ := newTestRouter()

	rr := doJSON(t, r, http.MethodPost, "/api/v1/urls", map[string]any{
		"long_url": "https://example.com/foo",
	})
	if rr.Code != http.StatusCreated {
		t.Fatalf("Shorten: status=%d body=%s", rr.Code, rr.Body.String())
	}
	created := decode[linkResp](t, rr)
	if created.Code == "" {
		t.Fatal("expected non-empty code")
	}
	if created.ShortURL != "https://urlo.example/"+created.Code {
		t.Errorf("short_url=%q", created.ShortURL)
	}

	rr = doJSON(t, r, http.MethodGet, "/api/v1/urls/"+created.Code, nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("Resolve: status=%d body=%s", rr.Code, rr.Body.String())
	}
	got := decode[linkResp](t, rr)
	if got.LongURL != "https://example.com/foo" {
		t.Errorf("long_url=%q", got.LongURL)
	}
	if got.VisitCount != 1 {
		t.Errorf("visit_count=%d, want 1", got.VisitCount)
	}
}

func TestShortenInvalidBody(t *testing.T) {
	r, _ := newTestRouter()

	cases := []struct {
		name string
		body any
		want int
	}{
		{"missing long_url", map[string]any{}, http.StatusBadRequest},
		{"bad url", map[string]any{"long_url": "not a url"}, http.StatusBadRequest},
		{"bad custom_code", map[string]any{"long_url": "https://x.test", "custom_code": "no/slash"}, http.StatusBadRequest},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rr := doJSON(t, r, http.MethodPost, "/api/v1/urls", tc.body)
			if rr.Code != tc.want {
				t.Errorf("status=%d body=%s, want %d", rr.Code, rr.Body.String(), tc.want)
			}
		})
	}
}

func TestShortenDuplicateCustomCode(t *testing.T) {
	r, _ := newTestRouter()

	doJSON(t, r, http.MethodPost, "/api/v1/urls", map[string]any{
		"long_url":    "https://a.test",
		"custom_code": "abc",
	})
	rr := doJSON(t, r, http.MethodPost, "/api/v1/urls", map[string]any{
		"long_url":    "https://b.test",
		"custom_code": "abc",
	})
	if rr.Code != http.StatusConflict {
		t.Errorf("status=%d, want 409", rr.Code)
	}
}

func TestStatsDoesNotIncrementVisit(t *testing.T) {
	r, _ := newTestRouter()
	rr := doJSON(t, r, http.MethodPost, "/api/v1/urls", map[string]any{
		"long_url": "https://x.test",
	})
	created := decode[linkResp](t, rr)

	rr = doJSON(t, r, http.MethodGet, "/api/v1/urls/"+created.Code+"/stats", nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("stats: status=%d body=%s", rr.Code, rr.Body.String())
	}
	stats := decode[linkResp](t, rr)
	if stats.VisitCount != 0 {
		t.Errorf("visit_count=%d, want 0", stats.VisitCount)
	}
}

func TestDelete(t *testing.T) {
	r, _ := newTestRouter()
	rr := doJSON(t, r, http.MethodPost, "/api/v1/urls", map[string]any{
		"long_url": "https://x.test",
	})
	created := decode[linkResp](t, rr)

	rr = doJSON(t, r, http.MethodDelete, "/api/v1/urls/"+created.Code, nil)
	if rr.Code != http.StatusNoContent {
		t.Fatalf("delete: status=%d body=%s", rr.Code, rr.Body.String())
	}

	rr = doJSON(t, r, http.MethodGet, "/api/v1/urls/"+created.Code, nil)
	if rr.Code != http.StatusNotFound {
		t.Errorf("resolve after delete: status=%d", rr.Code)
	}

	rr = doJSON(t, r, http.MethodDelete, "/api/v1/urls/"+created.Code, nil)
	if rr.Code != http.StatusNotFound {
		t.Errorf("delete twice: status=%d", rr.Code)
	}
}

func TestRedirect(t *testing.T) {
	r, _ := newTestRouter()
	rr := doJSON(t, r, http.MethodPost, "/api/v1/urls", map[string]any{
		"long_url": "https://example.com/target",
	})
	created := decode[linkResp](t, rr)

	req := httptest.NewRequest(http.MethodGet, "/"+created.Code, nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusFound {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	if loc := rec.Header().Get("Location"); loc != "https://example.com/target" {
		t.Errorf("Location=%q", loc)
	}
}

func TestRedirectNotFound(t *testing.T) {
	r, _ := newTestRouter()
	req := httptest.NewRequest(http.MethodGet, "/nope", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Errorf("status=%d", rec.Code)
	}
}
