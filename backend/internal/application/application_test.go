package application

import (
	"bytes"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/ackwrap/ackrun/internal/paths"
)

func testApplication(t *testing.T) *Application {
	t.Helper()
	dataDir := t.TempDir()
	t.Setenv("ACKWRAP_DATA_DIR", dataDir)
	t.Setenv("ACKWRAP_BINARY_DIR", filepath.Join(dataDir, "bin"))
	app, err := New(Options{Paths: paths.Default()})
	if err != nil {
		t.Fatal(err)
	}
	return app
}

func TestApplicationLifecycleIsIdempotent(t *testing.T) {
	app := testApplication(t)
	if err := app.Start(); err != nil {
		t.Fatal(err)
	}
	if err := app.Start(); err != nil {
		t.Fatalf("second Start() error = %v", err)
	}
	app.PrepareShutdown()
	app.PrepareShutdown()
	if err := app.Start(); !errors.Is(err, ErrClosed) {
		t.Fatalf("Start() after shutdown error = %v, want %v", err, ErrClosed)
	}
	if err := app.Close(); err != nil {
		t.Fatal(err)
	}
	if err := app.Close(); err != nil {
		t.Fatalf("second Close() error = %v", err)
	}
}

func TestApplicationServesEmbeddedSPAAndAssets(t *testing.T) {
	app := testApplication(t)
	defer app.Close()

	indexRecorder := httptest.NewRecorder()
	app.Handler().ServeHTTP(indexRecorder, httptest.NewRequest(http.MethodGet, "/nodes", nil))
	if indexRecorder.Code != http.StatusOK || !strings.Contains(indexRecorder.Body.String(), `<div id="root"></div>`) {
		t.Fatalf("SPA response = %d %s", indexRecorder.Code, indexRecorder.Body.String())
	}
	assetRecorder := httptest.NewRecorder()
	app.Handler().ServeHTTP(assetRecorder, httptest.NewRequest(http.MethodGet, "/favicon.png", nil))
	if assetRecorder.Code != http.StatusOK || assetRecorder.Body.Len() == 0 || !strings.HasPrefix(assetRecorder.Header().Get("Content-Type"), "image/png") {
		t.Fatalf("asset response = %d type=%q size=%d", assetRecorder.Code, assetRecorder.Header().Get("Content-Type"), assetRecorder.Body.Len())
	}
	apiRecorder := httptest.NewRecorder()
	app.Handler().ServeHTTP(apiRecorder, httptest.NewRequest(http.MethodGet, "/api/v1/missing", nil))
	if apiRecorder.Code != http.StatusNotFound || !strings.Contains(apiRecorder.Body.String(), "NOT_FOUND") {
		t.Fatalf("API fallback = %d %s", apiRecorder.Code, apiRecorder.Body.String())
	}
}

func TestAccessTokenRedactingWriter(t *testing.T) {
	var output bytes.Buffer
	writer := accessTokenRedactingWriter{Writer: &output}
	input := []byte("GET /?access_token=secret-value HTTP/1.1\n")
	if written, err := writer.Write(input); err != nil || written != len(input) {
		t.Fatalf("Write() = %d, %v", written, err)
	}
	if got := output.String(); strings.Contains(got, "secret-value") || !strings.Contains(got, "access_token=[REDACTED]") {
		t.Fatalf("access log was not redacted: %q", got)
	}
}

func TestGinAccessLogRedactsQueryToken(t *testing.T) {
	for _, requestPath := range []string{
		"/?access_token=secret-value",
		"/?access%5Ftoken=secret-value",
		"/?%61%63%63%65%73%73%5f%74%6f%6b%65%6e=secret-value",
	} {
		t.Run(requestPath, func(t *testing.T) {
			gin.SetMode(gin.TestMode)
			var output bytes.Buffer
			router := gin.New()
			router.Use(gin.LoggerWithWriter(accessTokenRedactingWriter{Writer: &output}))
			router.GET("/", func(c *gin.Context) { c.Status(http.StatusNoContent) })

			recorder := httptest.NewRecorder()
			router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, requestPath, nil))
			if got := output.String(); strings.Contains(got, "secret-value") || !strings.Contains(got, "[REDACTED]") {
				t.Fatalf("Gin access log was not redacted: %q", got)
			}
		})
	}
}
