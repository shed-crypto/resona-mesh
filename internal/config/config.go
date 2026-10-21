package config

import (
	"bufio"
	"errors"
	"flag"
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"

	"resonamesh/pkg/protocol"
)

var (
	ErrInsecureHTTPBind = errors.New("config: HTTP address must bind to loopback (127.0.0.1) for security")
	ErrInvalidP2PPort   = errors.New("config: P2P port must be between 1 and 65535")
	ErrInvalidQuota     = errors.New("config: storage quota must be greater than zero")
)

type Config struct {
	HTTPAddr               string
	P2PPort                int
	DataDir                string
	StorageQuotaBytes      int64
	KeeperEnabled          bool
	TrustedModeratorPubKey string
	LogLevel               string
}

// DefaultConfig returns safe, preconfigured embedded fallback settings.
func DefaultConfig() *Config {
	return &Config{
		HTTPAddr:               fmt.Sprintf("127.0.0.1:%d", protocol.DefaultHTTPPort),
		P2PPort:                protocol.DefaultP2PPort,
		DataDir:                "./data",
		StorageQuotaBytes:      5 * 1024 * 1024 * 1024, // 5 GiB
		KeeperEnabled:          false,
		TrustedModeratorPubKey: "",
		LogLevel:               "INFO",
	}
}

// Load loads configuration adhering strictly to the three-tier precedence:
// 1. CLI Flags (highest)
// 2. Environment Variables / .env file (intermediate)
// 3. Embedded Defaults (fallback)
func Load(envPath string, args []string) (*Config, error) {
	cfg := DefaultConfig()

	// 1. Load from .env file if it exists
	if envPath != "" {
		_ = loadDotEnv(envPath)
	}

	// 2. Read from process environment
	if val := os.Getenv("RESONAMESH_HTTP_ADDR"); val != "" {
		cfg.HTTPAddr = val
	}
	if val := os.Getenv("RESONAMESH_P2P_PORT"); val != "" {
		if p, err := strconv.Atoi(val); err == nil {
			cfg.P2PPort = p
		}
	}
	if val := os.Getenv("RESONAMESH_DATA_DIR"); val != "" {
		cfg.DataDir = val
	}
	if val := os.Getenv("RESONAMESH_STORAGE_QUOTA_BYTES"); val != "" {
		if q, err := strconv.ParseInt(val, 10, 64); err == nil {
			cfg.StorageQuotaBytes = q
		}
	}
	if val := os.Getenv("RESONAMESH_KEEPER_ENABLED"); val != "" {
		cfg.KeeperEnabled = strings.EqualFold(val, "true") || val == "1"
	}
	if val := os.Getenv("RESONAMESH_TRUSTED_MODERATOR_PUBKEY"); val != "" {
		cfg.TrustedModeratorPubKey = val
	}
	if val := os.Getenv("RESONAMESH_LOG_LEVEL"); val != "" {
		cfg.LogLevel = strings.ToUpper(val)
	}

	// 3. Override with CLI Flags if provided
	if len(args) > 0 {
		fs := flag.NewFlagSet("resonamesh", flag.ContinueOnError)
		flagHTTP := fs.String("http", "", "Local HTTP listen address (loopback)")
		flagP2P := fs.Int("p2p", 0, "P2P swarm listen port")
		flagData := fs.String("data", "", "Data directory path")
		flagQuota := fs.Int64("quota", 0, "Storage quota in bytes")
		flagKeeper := fs.Bool("keeper", false, "Enable automatic background replication")

		if err := fs.Parse(args); err != nil {
			return nil, err
		}

		if *flagHTTP != "" {
			cfg.HTTPAddr = *flagHTTP
		}
		if *flagP2P != 0 {
			cfg.P2PPort = *flagP2P
		}
		if *flagData != "" {
			cfg.DataDir = *flagData
		}
		if *flagQuota > 0 {
			cfg.StorageQuotaBytes = *flagQuota
		}
		if *flagKeeper {
			cfg.KeeperEnabled = true
		}
	}

	// 4. Validate configuration bounds
	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	return cfg, nil
}

// Validate verifies that settings conform to security and networking boundaries.
func (c *Config) Validate() error {
	host, _, err := net.SplitHostPort(c.HTTPAddr)
	if err != nil {
		return fmt.Errorf("config: invalid HTTP address: %w", err)
	}

	// Enforce loopback binding: 04-security-moderation.md §3 forbids 0.0.0.0
	if host != "127.0.0.1" && host != "localhost" && host != "::1" {
		return ErrInsecureHTTPBind
	}

	if c.P2PPort < 1 || c.P2PPort > 65535 {
		return ErrInvalidP2PPort
	}

	if c.StorageQuotaBytes <= 0 {
		return ErrInvalidQuota
	}

	return nil
}

// loadDotEnv parses key-value pairs from a .env file and sets them into process env if not already set.
func loadDotEnv(filepath string) error {
	f, err := os.Open(filepath)
	if err != nil {
		return err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) == 2 {
			k := strings.TrimSpace(parts[0])
			v := strings.TrimSpace(parts[1])
			v = strings.Trim(v, `"'`)
			if _, exists := os.LookupEnv(k); !exists {
				_ = os.Setenv(k, v)
			}
		}
	}
	return scanner.Err()
}
