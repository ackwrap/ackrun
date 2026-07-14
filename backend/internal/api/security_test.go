package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func newSecurityTestRouter(token string) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(SecurityMiddleware(token))
	router.GET("/", func(c *gin.Context) { c.String(http.StatusOK, "ui") })
	router.GET("/api/v1/runtime", func(c *gin.Context) { c.String(http.StatusOK, "api") })
	return router
}

func TestSecurityMiddlewareAllowsLocalMode(t *testing.T) {
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/v1/runtime", nil)
	newSecurityTestRouter("").ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusOK)
	}
}

func TestSecurityMiddlewareRequiresToken(t *testing.T) {
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/v1/runtime", nil)
	newSecurityTestRouter("secret").ServeHTTP(recorder, request)

	if recorder.Code != http.StatusUnauthorized || !strings.Contains(recorder.Body.String(), "UNAUTHORIZED") {
		t.Fatalf("response = %d %s, want unauthorized error", recorder.Code, recorder.Body.String())
	}
}

func TestSecurityMiddlewareAcceptsBearerToken(t *testing.T) {
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/v1/runtime", nil)
	request.Header.Set("Authorization", "Bearer secret")
	newSecurityTestRouter("secret").ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusOK)
	}
}

func TestSecurityMiddlewareBootstrapsCookieAndRemovesToken(t *testing.T) {
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/?access_token=secret&tab=nodes", nil)
	newSecurityTestRouter("secret").ServeHTTP(recorder, request)

	if recorder.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusSeeOther)
	}
	if location := recorder.Header().Get("Location"); location != "/?tab=nodes" {
		t.Fatalf("Location = %q, want token-free redirect", location)
	}
	cookies := recorder.Result().Cookies()
	if len(cookies) != 1 || !cookies[0].HttpOnly || cookies[0].SameSite != http.SameSiteStrictMode {
		t.Fatalf("cookies = %#v, want HttpOnly strict cookie", cookies)
	}
}

func TestSecurityMiddlewareAcceptsSessionCookie(t *testing.T) {
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/v1/runtime", nil)
	request.AddCookie(&http.Cookie{Name: apiTokenCookie, Value: "secret"})
	newSecurityTestRouter("secret").ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusOK)
	}
}
