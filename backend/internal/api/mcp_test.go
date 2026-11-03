package api

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/ackwrap/ackrun/internal/model"
	"github.com/ackwrap/ackrun/internal/service"
	"github.com/ackwrap/ackrun/internal/store"
)

func TestMCPHTTPAuthenticationBoundary(t *testing.T) {
	db, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	svc := service.NewMCPService(db)
	issued, err := svc.UpdateSettings(model.MCPSettingsRequest{Enabled: true, GenerateToken: true})
	if err != nil {
		t.Fatal(err)
	}
	router := gin.New()
	router.Use(SecurityMiddleware("synthetic-admin-token"))
	RegisterMCPRoutes(router, svc, "synthetic-admin-token")
	for _, tc := range []struct {
		name, peer, host, origin, token, query, forwarded string
		status                                            int
	}{
		{name: "loopback", peer: "127.0.0.1:1234", host: "localhost:8081", token: issued.Token, status: 405},
		{name: "LAN", peer: "192.168.1.2:1234", host: "192.168.1.1:8080", token: issued.Token, status: 405},
		{name: "IPv6", peer: "[fd00::2]:1234", host: "[fd00::1]:8080", token: issued.Token, status: 405},
		{name: "public peer", peer: "203.0.113.2:1234", host: "127.0.0.1:8081", token: issued.Token, status: 403},
		{name: "domain host", peer: "127.0.0.1:1234", host: "evil.example", token: issued.Token, status: 403},
		{name: "foreign origin", peer: "127.0.0.1:1234", host: "127.0.0.1:8081", origin: "http://evil.example", token: issued.Token, status: 403},
		{name: "same origin", peer: "127.0.0.1:1234", host: "127.0.0.1:8081", origin: "http://127.0.0.1:8081", token: issued.Token, status: 405},
		{name: "no token", peer: "127.0.0.1:1234", host: "127.0.0.1:8081", status: 401},
		{name: "admin token", peer: "127.0.0.1:1234", host: "127.0.0.1:8081", token: "synthetic-admin-token", status: 401},
		{name: "query token", peer: "127.0.0.1:1234", host: "127.0.0.1:8081", query: "?token=synthetic", token: issued.Token, status: 400},
		{name: "forwarded", peer: "127.0.0.1:1234", host: "127.0.0.1:8081", forwarded: "192.168.1.2", token: issued.Token, status: 403},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, path := range []string{"/mcp", "/mcp/"} {
				req := httptest.NewRequest(http.MethodGet, "http://"+tc.host+path+tc.query, nil)
				req.RemoteAddr = tc.peer
				req.Header.Set("Accept", "text/event-stream")
				req.Header.Set("Origin", tc.origin)
				if tc.origin == "" {
					req.Header.Del("Origin")
				}
				if tc.token != "" {
					req.Header.Set("Authorization", "Bearer "+tc.token)
				}
				if tc.forwarded != "" {
					req.Header.Set("X-Forwarded-For", tc.forwarded)
				}
				req.AddCookie(&http.Cookie{Name: apiTokenCookie, Value: "synthetic-admin-token"})
				w := httptest.NewRecorder()
				router.ServeHTTP(w, req)
				if w.Code != tc.status || w.Header().Get("Cache-Control") != "no-store" {
					t.Fatalf("status=%d cache=%q", w.Code, w.Header().Get("Cache-Control"))
				}
			}
		})
	}
	for _, tc := range []struct {
		method, token, origin, body string
		status                      int
	}{
		{http.MethodGet, issued.Token, "", "", 401},
		{http.MethodGet, "synthetic-admin-token", "", "", 200},
		{http.MethodPut, "synthetic-admin-token", "http://evil.example", `{"enabled":false}`, 403},
		{http.MethodPut, "synthetic-admin-token", "", `{"enabled":false}`, 200},
	} {
		req := httptest.NewRequest(tc.method, "http://127.0.0.1:8081/api/v1/mcp/settings", strings.NewReader(tc.body))
		req.Header.Set("Authorization", "Bearer "+tc.token)
		req.Header.Set("Content-Type", "application/json")
		if tc.origin != "" {
			req.Header.Set("Origin", tc.origin)
		}
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		if w.Code != tc.status || strings.Contains(w.Body.String(), issued.Token) {
			t.Fatalf("settings response status=%d", w.Code)
		}
	}
	req := httptest.NewRequest(http.MethodPost, "http://127.0.0.1:8081/mcp", strings.NewReader(`{}`))
	req.RemoteAddr = "127.0.0.1:1234"
	req.Header.Set("Authorization", "Bearer "+issued.Token)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Fatalf("disabled MCP status=%d", w.Code)
	}
}
