package config

import (
	"bufio"
	"encoding/json"
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
	NATPortMapEnabled      bool
	BootstrapSeeds         []string
	AllowInsecureHTTP      bool
	Headless               bool
	DisableRelay           bool
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
		NATPortMapEnabled:      true,
		AllowInsecureHTTP:      false,
		Headless:               false,
		DisableRelay:           false,
	}
}

// Load loads configuration adhering strictly to the three-tier precedence:
// 1. CLI Flags (highest)
// 2. Environment Variables / .env file (intermediate)
// 3. Embedded Defaults (fallback)
func Load(envPath string, args []string) (*Config, error) {
	cfg := DefaultConfig()

	// 1. Load from local bootstrap configuration file if present
	if data, err := os.ReadFile("bootstrap.local.json"); err == nil {
		var fileSeeds []string
		if err := json.Unmarshal(data, &fileSeeds); err == nil {
			cfg.BootstrapSeeds = fileSeeds
		}
	}

	// 2. Load from .env file if it exists
	if envPath != "" {
		_ = loadDotEnv(envPath)
	}

	// 3. Read from process environment
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
	if val := os.Getenv("RESONAMESH_NAT_PORT_MAP"); val != "" {
		cfg.NATPortMapEnabled = strings.EqualFold(val, "true") || val == "1"
	}
	if val := os.Getenv("RESONAMESH_BOOTSTRAP_SEEDS"); val != "" {
		cfg.BootstrapSeeds = parseSeedsString(val)
	} else if val := os.Getenv("RESONAMESH_BOOTSTRAP_PEERS"); val != "" {
		cfg.BootstrapSeeds = parseSeedsString(val)
	}
	if val := os.Getenv("RESONAMESH_ALLOW_INSECURE_HTTP"); val != "" {
		cfg.AllowInsecureHTTP = strings.EqualFold(val, "true") || val == "1"
	}
	if val := os.Getenv("RESONAMESH_HEADLESS"); val != "" {
		cfg.Headless = strings.EqualFold(val, "true") || val == "1"
	}
	if val := os.Getenv("RESONAMESH_DISABLE_RELAY"); val != "" {
		cfg.DisableRelay = strings.EqualFold(val, "true") || val == "1"
	}

	// 4. Override with CLI Flags if provided
	if len(args) > 0 {
		fs := flag.NewFlagSet("resonamesh", flag.ContinueOnError)
		flagHTTP := fs.String("http", "", "Local HTTP listen address (loopback)")
		flagP2P := fs.Int("p2p", 0, "P2P swarm listen port")
		flagData := fs.String("data", "", "Data directory path")
		flagQuota := fs.Int64("quota", 0, "Storage quota in bytes")
		flagKeeper := fs.Bool("keeper", false, "Enable automatic background replication")
		flagNATPortMap := fs.Bool("nat-port-map", cfg.NATPortMapEnabled, "Enable automatic router port mapping via UPnP / NAT-PMP")
		flagBootstrap := fs.String("bootstrap", "", "Comma-separated bootstrap multiaddrs or HTTP(S) endpoint URLs")
		flagInsecureHTTP := fs.Bool("allow-insecure-http", cfg.AllowInsecureHTTP, "Allow HTTP server to bind to non-loopback addresses (container/server environments)")
		flagHeadless := fs.Bool("headless", cfg.Headless, "Run in headless relay mode with interactive player mutations disabled")
		flagDisableRelay := fs.Bool("disable-relay", cfg.DisableRelay, "Disable Circuit Relay fallback, requiring direct P2P connections only")

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
		if *flagBootstrap != "" {
			cfg.BootstrapSeeds = parseSeedsString(*flagBootstrap)
		}
		fs.Visit(func(f *flag.Flag) {
			switch f.Name {
			case "nat-port-map":
				cfg.NATPortMapEnabled = *flagNATPortMap
			case "allow-insecure-http":
				cfg.AllowInsecureHTTP = *flagInsecureHTTP
			case "headless":
				cfg.Headless = *flagHeadless
			case "disable-relay":
				cfg.DisableRelay = *flagDisableRelay
			}
		})
	}

	// 5. Validate configuration bounds
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

	// Enforce loopback binding: 04-security-moderation.md §3 forbids 0.0.0.0 for desktop UI,
	// but permits explicit opt-in for containerized / headless server deployments.
	if host != "127.0.0.1" && host != "localhost" && host != "::1" {
		if !c.AllowInsecureHTTP {
			return ErrInsecureHTTPBind
		}
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

// parseSeedsString parses a string representing bootstrap seeds, supporting both JSON arrays and comma-separated lists.
func parseSeedsString(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	if strings.HasPrefix(raw, "[") && strings.HasSuffix(raw, "]") {
		var arr []string
		if err := json.Unmarshal([]byte(raw), &arr); err == nil {
			var cleaned []string
			for _, item := range arr {
				item = strings.TrimSpace(item)
				if item != "" {
					cleaned = append(cleaned, item)
				}
			}
			return cleaned
		}
	}

	parts := strings.Split(raw, ",")
	var result []string
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			result = append(result, p)
		}
	}
	return result
}

