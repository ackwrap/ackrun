package handler

import (
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ackwrap/ackrun/internal/logging"
	"github.com/ackwrap/ackrun/internal/model"
	"github.com/ackwrap/ackrun/internal/service"
	"github.com/ackwrap/ackrun/internal/store"
	"github.com/gin-gonic/gin"
)

func TestClashProxySelection(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, status := range []int{http.StatusNoContent, http.StatusBadRequest} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			group := "香港 / Proxy?#%"
			requestBody := `{"name":"synthetic-node"}`
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodPut || r.URL.EscapedPath() != "/proxies/"+url.PathEscape(group) {
					t.Errorf("unexpected upstream target: %s %s", r.Method, r.URL.EscapedPath())
				}
				if r.Header.Get("Authorization") != "Bearer synthetic-secret" {
					t.Error("core authorization was not applied")
				}
				body, _ := io.ReadAll(r.Body)
				if string(body) != requestBody {
					t.Error("selection body changed")
				}
				w.WriteHeader(status)
				if status != http.StatusNoContent {
					_, _ = io.WriteString(w, `{"message":"Must be a Selector"}`)
				}
			}))
			defer upstream.Close()
			db, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			settings := service.NewSettingsService(db)
			parsedURL, _ := url.Parse(upstream.URL)
			if err := settings.SetExperimentalSettings(&model.ExperimentalSettings{ClashAPIEnabled: true, ClashAPIPort: parsedURL.Port(), ClashAPISecret: "synthetic-secret"}); err != nil {
				t.Fatal(err)
			}
			h := NewClashProxyHandler(settings)
			router := gin.New()
			router.PUT("/api/v1/clash/proxies/*name", h.Proxy)
			response := httptest.NewRecorder()
			request := httptest.NewRequest(http.MethodPut, "/api/v1/clash/proxies/"+url.PathEscape(group), strings.NewReader(requestBody))
			request.Header.Set("Content-Type", "application/json")
			request.Header.Set("Authorization", "Bearer synthetic-manager-token")
			router.ServeHTTP(response, request)
			if response.Code != status {
				t.Fatalf("status = %d, want %d", response.Code, status)
			}
			if status == http.StatusBadRequest && !strings.Contains(response.Body.String(), "Must be a Selector") {
				t.Fatal("core error was not preserved")
			}
			logs := logging.ListToolLogs(1)
			if len(logs) != 1 || logs[0].Tag != "core.proxy_selection" {
				t.Fatal("missing selection result log")
			}
			if strings.Contains(logs[0].Message, "synthetic-secret") || strings.Contains(logs[0].Message, "synthetic-manager-token") {
				t.Fatal("selection log exposed credentials")
			}
		})
	}
}
