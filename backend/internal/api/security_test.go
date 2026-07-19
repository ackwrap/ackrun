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
	router.GET("/api/v1/rules/subscriptions/:id/content", func(c *gin.Context) { c.String(http.StatusOK, "rules") })
	router.GET("/api/v1/rules/geo/rule-sets/:tag/content", func(c *gin.Context) { c.String(http.StatusOK, "rules") })
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
	cookies := recorder.Result().Cookies()
	if len(cookies) != 1 || cookies[0].Name != apiTokenCookie || !cookies[0].HttpOnly || cookies[0].MaxAge != apiTokenCookieMaxAge {
		t.Fatalf("bearer session cookies = %#v", cookies)
	}
}

func TestSecurityMiddlewareDoesNotAcceptTokenOnUIURL(t *testing.T) {
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/?access_token=secret&tab=nodes", nil)
	newSecurityTestRouter("secret").ServeHTTP(recorder, request)

	if recorder.Code != http.StatusSeeOther || recorder.Header().Get("Location") != "/?tab=nodes" || len(recorder.Result().Cookies()) != 0 {
		t.Fatalf("response = %d, location = %q, cookies = %#v; UI query token must only be removed", recorder.Code, recorder.Header().Get("Location"), recorder.Result().Cookies())
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

func TestSecurityMiddlewareRequiresQueryTokenForLoopbackRuleSetContent(t *testing.T) {
	paths := []string{
		"/api/v1/rules/subscriptions/1/content",
		"/api/v1/rules/geo/rule-sets/geosite-google/content",
	}
	for _, path := range paths {
		recorder := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodGet, path, nil)
		request.RemoteAddr = "127.0.0.1:12345"
		newSecurityTestRouter("secret").ServeHTTP(recorder, request)
		if recorder.Code != http.StatusUnauthorized {
			t.Fatalf("%s status = %d, want %d", path, recorder.Code, http.StatusUnauthorized)
		}

		recorder = httptest.NewRecorder()
		request = httptest.NewRequest(http.MethodGet, path+"?access_token=secret", nil)
		request.RemoteAddr = "127.0.0.1:12345"
		newSecurityTestRouter("secret").ServeHTTP(recorder, request)
		if recorder.Code != http.StatusOK {
			t.Fatalf("%s authenticated status = %d, want %d", path, recorder.Code, http.StatusOK)
		}
	}
}

func TestSecurityMiddlewareRejectsQueryTokenOnOtherAPIPaths(t *testing.T) {
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/v1/runtime?access_token=secret", nil)
	newSecurityTestRouter("secret").ServeHTTP(recorder, request)
	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusUnauthorized)
	}
}

func TestSecurityMiddlewareAcceptsCookieWhenAnotherBearerHeaderIsPresent(t *testing.T) {
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/v1/runtime", nil)
	request.Header.Set("Authorization", "Bearer downstream-secret")
	request.AddCookie(&http.Cookie{Name: apiTokenCookie, Value: "secret"})
	newSecurityTestRouter("secret").ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusOK)
	}
}
