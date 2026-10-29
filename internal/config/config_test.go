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
	if !cfg.NATPortMapEnabled {
		t.Errorf("expected default NATPortMapEnabled to be true, got %v", cfg.NATPortMapEnabled)
	}
	if len(cfg.BootstrapSeeds) != 0 {
		t.Errorf("expected default BootstrapSeeds to be empty, got %v", cfg.BootstrapSeeds)
	}
}

func TestSecurityRejectionNonLoopback(t *testing.T) {
	insecureArgs := []string{"--http", "0.0.0.0:8080"}
	_, err := config.Load("", insecureArgs)
	if err != config.ErrInsecureHTTPBind {
		t.Fatalf("expected ErrInsecureHTTPBind when binding to 0.0.0.0, got: %v", err)
	}
}

func TestOptInNonLoopbackForContainers(t *testing.T) {
	// CLI flag opt-in
	args := []string{"--http", "0.0.0.0:8080", "--allow-insecure-http"}
	cfg, err := config.Load("", args)
	if err != nil {
		t.Fatalf("expected successful bind with explicit --allow-insecure-http, got: %v", err)
	}
	if cfg.HTTPAddr != "0.0.0.0:8080" {
		t.Errorf("expected 0.0.0.0:8080, got %s", cfg.HTTPAddr)
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
	flags := []string{"--http", "127.0.0.1:9090", "--p2p", "6001", "--nat-port-map=false"}
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
	if cfgWithFlags.NATPortMapEnabled {
		t.Errorf("expected CLI flag override false for NATPortMapEnabled, got %v", cfgWithFlags.NATPortMapEnabled)
	}
}

func TestBootstrapSeedsConfiguration(t *testing.T) {
	// 1. Env variable with comma-separated list
	t.Setenv("RESONAMESH_BOOTSTRAP_SEEDS", "https://gist.github.com/test/raw, /ip4/127.0.0.1/tcp/4001/p2p/12D3KooWtest")
	cfgEnv, err := config.Load("", nil)
	if err != nil {
		t.Fatalf("failed to load config with env seeds: %v", err)
	}
	if len(cfgEnv.BootstrapSeeds) != 2 {
		t.Fatalf("expected 2 bootstrap seeds from env, got %d", len(cfgEnv.BootstrapSeeds))
	}
	if cfgEnv.BootstrapSeeds[0] != "https://gist.github.com/test/raw" {
		t.Errorf("unexpected seed 0: %s", cfgEnv.BootstrapSeeds[0])
	}
	if cfgEnv.BootstrapSeeds[1] != "/ip4/127.0.0.1/tcp/4001/p2p/12D3KooWtest" {
		t.Errorf("unexpected seed 1: %s", cfgEnv.BootstrapSeeds[1])
	}

	// 2. CLI flag override
	cliArgs := []string{"--bootstrap", `["https://override.gist.com/raw"]`}
	cfgCLI, err := config.Load("", cliArgs)
	if err != nil {
		t.Fatalf("failed to load config with CLI seeds: %v", err)
	}
	if len(cfgCLI.BootstrapSeeds) != 1 || cfgCLI.BootstrapSeeds[0] != "https://override.gist.com/raw" {
		t.Fatalf("expected CLI flag override, got: %+v", cfgCLI.BootstrapSeeds)
	}
}

func TestHeadlessConfiguration(t *testing.T) {
	// 1. Env variable
	t.Setenv("RESONAMESH_HEADLESS", "true")
	cfgEnv, err := config.Load("", nil)
	if err != nil {
		t.Fatalf("failed to load config with env: %v", err)
	}
	if !cfgEnv.Headless {
		t.Errorf("expected Headless=true from env")
	}

	// 2. CLI flag
	t.Setenv("RESONAMESH_HEADLESS", "false")
	cfgCLI, err := config.Load("", []string{"--headless"})
	if err != nil {
		t.Fatalf("failed to load config with CLI flag: %v", err)
	}
	if !cfgCLI.Headless {
		t.Errorf("expected Headless=true from CLI flag")
	}
}

func TestSTUNServersConfiguration(t *testing.T) {
	// 1. Default embedded pool
	cfgDef, err := config.Load("", nil)
	if err != nil {
		t.Fatalf("failed to load default config: %v", err)
	}
	if len(cfgDef.STUNServers) == 0 || cfgDef.STUNServers[0] != "stun.l.google.com:19302" {
		t.Fatalf("expected default STUN servers, got: %+v", cfgDef.STUNServers)
	}

	// 2. Env variable override
	t.Setenv("RESONAMESH_STUN_SERVERS", "custom.stun.org:3478, stun.cloudflare.com:3478")
	cfgEnv, err := config.Load("", nil)
	if err != nil {
		t.Fatalf("failed to load config with env STUN: %v", err)
	}
	if len(cfgEnv.STUNServers) != 2 || cfgEnv.STUNServers[0] != "custom.stun.org:3478" {
		t.Fatalf("expected env STUN override, got: %+v", cfgEnv.STUNServers)
	}

	// 3. CLI flag override
	cliArgs := []string{"--stun-servers", "cli.stun.local:19302"}
	cfgCLI, err := config.Load("", cliArgs)
	if err != nil {
		t.Fatalf("failed to load config with CLI STUN: %v", err)
	}
	if len(cfgCLI.STUNServers) != 1 || cfgCLI.STUNServers[0] != "cli.stun.local:19302" {
		t.Fatalf("expected CLI STUN override, got: %+v", cfgCLI.STUNServers)
	}
}

func TestModeratorKeyCLIOverride(t *testing.T) {
	cliArgs := []string{"--moderator-key", "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"}
	cfg, err := config.Load("", cliArgs)
	if err != nil {
		t.Fatalf("failed to load config: %v", err)
	}
	if cfg.TrustedModeratorPubKey != "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef" {
		t.Fatalf("expected CLI moderator key override, got: %s", cfg.TrustedModeratorPubKey)
	}
}





func TestReleasePubKeyResolution(t *testing.T) {
	// 1. Default should be empty
	cfgDefault, err := config.Load("", []string{})
	if err != nil {
		t.Fatal(err)
	}
	if cfgDefault.ReleasePubKey != "" {
		t.Fatalf("expected empty default release pubkey, got: %s", cfgDefault.ReleasePubKey)
	}

	// 2. Env var
	t.Setenv("RESONAMESH_RELEASE_PUBKEY", "aaaa111122223333aaaa111122223333aaaa111122223333aaaa111122223333")
	cfgEnv, err := config.Load("", []string{})
	if err != nil {
		t.Fatal(err)
	}
	if cfgEnv.ReleasePubKey != "aaaa111122223333aaaa111122223333aaaa111122223333aaaa111122223333" {
		t.Fatalf("expected env release pubkey, got: %s", cfgEnv.ReleasePubKey)
	}

	// 3. CLI flag override
	cliArgs := []string{"--release-pubkey", "bbbb111122223333bbbb111122223333bbbb111122223333bbbb111122223333"}
	cfgCLI, err := config.Load("", cliArgs)
	if err != nil {
		t.Fatal(err)
	}
	if cfgCLI.ReleasePubKey != "bbbb111122223333bbbb111122223333bbbb111122223333bbbb111122223333" {
		t.Fatalf("expected CLI release pubkey override, got: %s", cfgCLI.ReleasePubKey)
	}
}
