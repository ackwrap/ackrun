package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestLoadServiceURLDefaultsToHighLoopbackPort(t *testing.T) {
	serviceURL, err := loadServiceURL("")
	if err != nil {
		t.Fatal(err)
	}
	if got := serviceURL.String(); got != defaultServiceURL {
		t.Fatalf("service URL = %q, want %q", got, defaultServiceURL)
	}
}

func TestLoadServiceURLRejectsRemoteAndCredentialedURLs(t *testing.T) {
	for _, value := range []string{
		"https://127.0.0.1:18080",
		"http://192.168.1.10:18080",
		"http://user:pass@127.0.0.1:18080",
		"http://127.0.0.1:18080/admin",
	} {
		t.Run(value, func(t *testing.T) {
			if _, err := loadServiceURL(value); err == nil {
				t.Fatalf("loadServiceURL(%q) error = nil", value)
			}
		})
	}
}

func TestProbeServiceValidatesAckwrapRuntimeResponse(t *testing.T) {
	requestPath := make(chan string, 1)
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		requestPath <- request.URL.Path
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{"status":"running","version":"test"}`))
	}))
	defer server.Close()

	serviceURL, err := loadServiceURL(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	app := &App{serviceURL: serviceURL, httpClient: server.Client()}
	if err := app.probeService(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got := <-requestPath; got != "/api/v1/runtime" {
		t.Fatalf("request path = %q", got)
	}
}

func TestProbeServiceRejectsUnrelatedHTTPServer(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{"status":"unrelated"}`))
	}))
	defer server.Close()

	serviceURL, err := loadServiceURL(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	app := &App{serviceURL: serviceURL, httpClient: server.Client()}
	if err := app.probeService(context.Background()); err == nil {
		t.Fatal("probeService() error = nil")
	}
}
