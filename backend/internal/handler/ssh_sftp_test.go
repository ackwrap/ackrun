package handler

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/ackwrap/ackrun/internal/paths"
	"github.com/ackwrap/ackrun/internal/service"
	"github.com/ackwrap/ackrun/internal/store"
)

func TestSSHSFTPHTTPRequiresSessionTokenAndEnforcesUploadLimit(t *testing.T) {
	gin.SetMode(gin.TestMode)
	root := t.TempDir()
	db, err := store.Open(filepath.Join(root, "ackwrap.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	svc, err := service.NewSSHHostService(db, &paths.Paths{DataDir: root}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer svc.Close()
	handler := NewSSHHostHandler(svc)
	router := gin.New()
	router.GET("/sessions/:sessionID/sftp", handler.ListSFTP)
	router.POST("/sessions/:sessionID/sftp/upload", handler.UploadSFTP)

	request := httptest.NewRequest(http.MethodGet, "/sessions/missing/sftp?path=.", nil)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusNotFound || !strings.Contains(response.Body.String(), "SSH_SESSION_NOT_FOUND") {
		t.Fatalf("missing SFTP token response: status=%d body=%s", response.Code, response.Body.String())
	}

	request = httptest.NewRequest(http.MethodGet, "/sessions/missing/sftp?path=.", nil)
	request.Header.Set(sshSFTPTokenHeader, "wrong-token")
	response = httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusNotFound || !strings.Contains(response.Body.String(), "SSH_SESSION_NOT_FOUND") {
		t.Fatalf("invalid SFTP session response: status=%d body=%s", response.Code, response.Body.String())
	}

	request = httptest.NewRequest(http.MethodPost, "/sessions/missing/sftp/upload?path=file", http.NoBody)
	request.Header.Set(sshSFTPTokenHeader, "token")
	request.ContentLength = maxSSHSFTPUploadBody + 1
	response = httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusRequestEntityTooLarge || !strings.Contains(response.Body.String(), "SSH_SFTP_UPLOAD_TOO_LARGE") {
		t.Fatalf("oversized SFTP upload response: status=%d body=%s", response.Code, response.Body.String())
	}
}
