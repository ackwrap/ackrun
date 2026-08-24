package handler

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/ackwrap/ackrun/internal/model"
	"github.com/ackwrap/ackrun/internal/paths"
	"github.com/ackwrap/ackrun/internal/service"
	"github.com/ackwrap/ackrun/internal/store"
)

func TestSSHHostShareHTTPNoStoreAndLimits(t *testing.T) {
	gin.SetMode(gin.TestMode)
	root := t.TempDir()
	db, err := store.Open(filepath.Join(root, "ackwrap.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	svc, err := service.NewSSHHostService(db, &paths.Paths{DataDir: root}, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(svc.Close)
	credential, err := svc.CreateCredential(model.SSHCredentialRequest{
		Name: "share handler credential", AuthType: "password", Secret: "handler-secret",
	})
	if err != nil {
		t.Fatal(err)
	}
	host, err := svc.CreateHost(model.SSHHostRequest{
		Name: "share handler host", Host: "handler.example.invalid", Port: 22, Username: "root",
		CredentialID: credential.ID, ConnectionMode: "direct", TerminalType: "xterm-256color", Enabled: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	handler := NewSSHHostHandler(svc)
	router := gin.New()
	router.POST("/hosts/import", handler.ImportHost)
	router.POST("/hosts/:id/share", handler.ShareHost)

	shared := performSSHHostJSONRequest(t, router, "/hosts/"+strconv.FormatInt(host.ID, 10)+"/share", map[string]string{
		"password": "handler-share-password",
	})
	if shared.Code != http.StatusOK || shared.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("unexpected share response: status=%d cache=%q", shared.Code, shared.Header().Get("Cache-Control"))
	}
	var shareResponse model.SSHHostShareResponse
	if err := json.Unmarshal(shared.Body.Bytes(), &shareResponse); err != nil || shareResponse.Code == "" {
		t.Fatal("share response is missing an encrypted code")
	}

	wrongPassword := performSSHHostJSONRequest(t, router, "/hosts/import", map[string]string{
		"code": shareResponse.Code, "password": "incorrect-password",
	})
	assertSSHHostHTTPError(t, wrongPassword, http.StatusBadRequest, "SSH_SHARE_DECRYPT_FAILED")

	imported := performSSHHostJSONRequest(t, router, "/hosts/import", map[string]string{
		"code": shareResponse.Code, "password": "handler-share-password",
	})
	if imported.Code != http.StatusOK || imported.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("unexpected import response: status=%d cache=%q", imported.Code, imported.Header().Get("Cache-Control"))
	}

	tooLargeCode := performSSHHostJSONRequest(t, router, "/hosts/import", map[string]string{
		"code": strings.Repeat("a", model.MaxSSHHostShareCodeSize+1), "password": "handler-share-password",
	})
	assertSSHHostHTTPError(t, tooLargeCode, http.StatusRequestEntityTooLarge, "SSH_SHARE_TOO_LARGE")

	tooLargeBody := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/hosts/import", strings.NewReader(`{"code":"`+strings.Repeat("a", maxSSHHostImportRequestBody)+`","password":"handler-share-password"}`))
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(tooLargeBody, request)
	assertSSHHostHTTPError(t, tooLargeBody, http.StatusRequestEntityTooLarge, "SSH_REQUEST_TOO_LARGE")
}

func performSSHHostJSONRequest(t *testing.T, router http.Handler, path string, payload any) *httptest.ResponseRecorder {
	t.Helper()
	content, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(content))
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(response, request)
	return response
}

func assertSSHHostHTTPError(t *testing.T, response *httptest.ResponseRecorder, status int, code string) {
	t.Helper()
	if response.Code != status {
		t.Fatalf("expected HTTP %d, got %d", status, response.Code)
	}
	var result model.ErrorResponse
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result.Error.Code != code {
		t.Fatalf("expected error code %s, got %s", code, result.Error.Code)
	}
}
