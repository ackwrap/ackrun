package main

import (
	"strings"
	"testing"
)

func TestLoadServerConfigDefaultsToLoopback(t *testing.T) {
	t.Setenv("ACKWRAP_LISTEN_ADDR", "")
	t.Setenv("ACKWRAP_API_TOKEN", "")

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
