package config_test

import (
	"os"
	"path/filepath"
	"testing"

	"resonamesh/internal/config"
	"resonamesh/pkg/protocol"
)

func TestDefaultConfig(t *testing.T) {
	cfg, err := config.Load("", nil)
	if err != nil {
		t.Fatalf("expected valid default config, got error: %v", err)
	}

	if cfg.HTTPAddr != "127.0.0.1:8080" {
		t.Errorf("expected default HTTPAddr 127.0.0.1:8080, got %s", cfg.HTTPAddr)
	}
	if cfg.P2PPort != protocol.DefaultP2PPort {
		t.Errorf("expected default P2PPort %d, got %d", protocol.DefaultP2PPort, cfg.P2PPort)
	}
	if cfg.StorageQuotaBytes != 5*1024*1024*1024 {
		t.Errorf("expected default quota 5 GiB, got %d", cfg.StorageQuotaBytes)
	}
}

func TestSecurityRejectionNonLoopback(t *testing.T) {
	insecureArgs := []string{"--http", "0.0.0.0:8080"}
	_, err := config.Load("", insecureArgs)
	if err != config.ErrInsecureHTTPBind {
		t.Fatalf("expected ErrInsecureHTTPBind when binding to 0.0.0.0, got: %v", err)
	}
}

func TestInvalidPortBounds(t *testing.T) {
	args := []string{"--p2p", "70000"}
	_, err := config.Load("", args)
	if err != config.ErrInvalidP2PPort {
		t.Fatalf("expected ErrInvalidP2PPort for out-of-range port, got: %v", err)
	}
}

func TestThreeTierPrecedence(t *testing.T) {
	// Create temporary .env file
	tmpDir := t.TempDir()
	envFile := filepath.Join(tmpDir, ".env")
	content := "RESONAMESH_HTTP_ADDR=127.0.0.1:9000\nRESONAMESH_P2P_PORT=5001\n"
	if err := os.WriteFile(envFile, []byte(content), 0600); err != nil {
		t.Fatalf("failed to write tmp env file: %v", err)
	}

	// 1. Without flags, should take from .env
	cfg, err := config.Load(envFile, nil)
	if err != nil {
		t.Fatalf("failed to load config with .env: %v", err)
	}
	if cfg.HTTPAddr != "127.0.0.1:9000" {
		t.Errorf("expected HTTPAddr from .env 127.0.0.1:9000, got %s", cfg.HTTPAddr)
	}
	if cfg.P2PPort != 5001 {
		t.Errorf("expected P2PPort from .env 5001, got %d", cfg.P2PPort)
	}

	// 2. CLI flags override .env values
	flags := []string{"--http", "127.0.0.1:9090", "--p2p", "6001"}
	cfgWithFlags, err := config.Load(envFile, flags)
	if err != nil {
		t.Fatalf("failed to load config with flags: %v", err)
	}
	if cfgWithFlags.HTTPAddr != "127.0.0.1:9090" {
		t.Errorf("expected CLI flag override 127.0.0.1:9090, got %s", cfgWithFlags.HTTPAddr)
	}
	if cfgWithFlags.P2PPort != 6001 {
		t.Errorf("expected CLI flag override 6001, got %d", cfgWithFlags.P2PPort)
	}
}
