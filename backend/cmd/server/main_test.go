package main

import (
	"context"
	"errors"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestLoadServerConfigUsesDefaultListenAddressWithToken(t *testing.T) {
	t.Setenv("ACKWRAP_LISTEN_ADDR", "")
	t.Setenv("ACKWRAP_API_TOKEN", "secret")

	config, err := loadServerConfig()
	if err != nil {
		t.Fatalf("loadServerConfig() error = %v", err)
	}
	if config.ListenAddr != defaultListenAddr {
		t.Fatalf("ListenAddr = %q, want %q", config.ListenAddr, defaultListenAddr)
	}
}

func TestLoadServerConfigRequiresTokenForNonLoopback(t *testing.T) {
	t.Setenv("ACKWRAP_LISTEN_ADDR", "0.0.0.0:8080")
	t.Setenv("ACKWRAP_API_TOKEN", "")

	_, err := loadServerConfig()
	if err == nil || !strings.Contains(err.Error(), "ACKWRAP_API_TOKEN") {
		t.Fatalf("loadServerConfig() error = %v, want token requirement", err)
	}
}

func TestLoadServerConfigAllowsNonLoopbackWithToken(t *testing.T) {
	t.Setenv("ACKWRAP_LISTEN_ADDR", "0.0.0.0:8080")
	t.Setenv("ACKWRAP_API_TOKEN", "secret")

	config, err := loadServerConfig()
	if err != nil {
		t.Fatalf("loadServerConfig() error = %v", err)
	}
	if config.APIToken != "secret" {
		t.Fatalf("APIToken = %q, want configured token", config.APIToken)
	}
}

func TestLoadServerConfigRejectsInvalidAddress(t *testing.T) {
	t.Setenv("ACKWRAP_LISTEN_ADDR", "localhost")
	t.Setenv("ACKWRAP_API_TOKEN", "")

	if _, err := loadServerConfig(); err == nil {
		t.Fatal("loadServerConfig() error = nil, want invalid address error")
	}
}

func TestStartHTTPServerAndCoreSkipsCoreWhenListenFails(t *testing.T) {
	occupied, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer occupied.Close()

	restored := false
	started := false
	server := &http.Server{Addr: occupied.Addr().String(), Handler: http.NewServeMux()}
	if _, err := startHTTPServerAndCore(server, func() { restored = true }, func() error {
		started = true
		return nil
	}); err == nil {
		t.Fatal("expected occupied listener error")
	}
	if restored || started {
		t.Fatal("core restore or auto-start ran before the HTTP listener was available")
	}
}

func TestStartHTTPServerAndCoreStartsCoreAfterListen(t *testing.T) {
	restored := make(chan struct{}, 1)
	started := make(chan struct{}, 1)
	server := &http.Server{Addr: "127.0.0.1:0", Handler: http.NewServeMux()}
	serverErrors, err := startHTTPServerAndCore(server, func() { restored <- struct{}{} }, func() error {
		<-restored
		started <- struct{}{}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for core auto-start")
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := server.Shutdown(shutdownCtx); err != nil {
		t.Fatal(err)
	}
	if err := <-serverErrors; !errors.Is(err, http.ErrServerClosed) {
		t.Fatalf("server error = %v, want %v", err, http.ErrServerClosed)
	}
}
