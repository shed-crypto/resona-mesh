package main

import (
	"context"
	"crypto/ed25519"
	"fmt"
	"log/slog"
	"net"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/multiformats/go-multiaddr"

	"resonamesh/internal/api"
	"resonamesh/internal/catalog"
	"resonamesh/internal/config"
	"resonamesh/internal/importer"
	"resonamesh/internal/moderation"
	"resonamesh/internal/p2p"
	"resonamesh/internal/player"
	"resonamesh/internal/storage"
	"resonamesh/pkg/crypto"
)

var (
	AppVersion            = "0.1.0-alpha"
	AppName               = "ResonaMesh"
	DefaultBootstrapSeeds = ""
)

func main() {
	for _, arg := range os.Args[1:] {
		if arg == "--version" || arg == "-v" {
			fmt.Printf("%s v%s\n", AppName, AppVersion)
			os.Exit(0)
		}
	}

	cfg, err := config.Load(".env", os.Args[1:])
	if err != nil {
		fmt.Fprintf(os.Stderr, "Configuration error: %v\n", err)
		os.Exit(1)
	}

	if len(cfg.BootstrapSeeds) == 0 && DefaultBootstrapSeeds != "" {
		for _, s := range strings.Split(DefaultBootstrapSeeds, ",") {
			s = strings.TrimSpace(s)
			if s != "" {
				cfg.BootstrapSeeds = append(cfg.BootstrapSeeds, s)
			}
		}
	}

	logLevel := slog.LevelInfo
	if cfg.LogLevel == "DEBUG" {
		logLevel = slog.LevelDebug
	}

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: logLevel,
	}))
	slog.SetDefault(logger)

	slog.Info("Starting ResonaMesh Node",
		"app", AppName,
		"version", AppVersion,
		"http_addr", cfg.HTTPAddr,
		"p2p_port", cfg.P2PPort,
		"data_dir", cfg.DataDir,
		"quota_bytes", cfg.StorageQuotaBytes,
		"keeper_enabled", cfg.KeeperEnabled,
		"nat_port_map", cfg.NATPortMapEnabled,
		"headless", cfg.Headless,
	)

	// Context with OS interrupt cancellation for graceful shutdown
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// Ensure data directory exists
	if err := os.MkdirAll(cfg.DataDir, 0700); err != nil {
		slog.Error("Failed to initialize data directory", "path", cfg.DataDir, "error", err)
		os.Exit(1)
	}

	// 1. Load or generate persistent node cryptographic identity
	keyPath := filepath.Join(cfg.DataDir, "node_key.ed25519")
	privKey, err := crypto.LoadOrCreatePrivateKey(keyPath)
	if err != nil {
		slog.Error("Failed to initialize node identity key", "path", keyPath, "error", err)
		os.Exit(1)
	}
	pubKey := privKey.Public().(ed25519.PublicKey)
	slog.Info("Node identity initialized", "pubkey", crypto.PublicKeyToHex(pubKey))

	// 2. Open SQLite database with automatic migrations
	dbPath := filepath.Join(cfg.DataDir, "catalog.db")
	db, err := storage.Open(dbPath)
	if err != nil {
		slog.Error("Failed to initialize SQLite database", "path", dbPath, "error", err)
		os.Exit(1)
	}
	defer db.Close()

	// 3. Initialize content-addressed sharded blockstore
	bs, err := storage.NewBlockStore(cfg.DataDir)
	if err != nil {
		slog.Error("Failed to initialize blockstore", "path", cfg.DataDir, "error", err)
		os.Exit(1)
	}

	// Startup storage reconciliation pass (orphan cleanup, temp purging)
	if _, err := storage.Recover(ctx, cfg.DataDir, db, bs); err != nil {
		slog.Warn("Storage recovery completed with notices", "error", err)
	}

	// 4. Instantiate storage quota manager & domain services
	activeTracker := storage.NewActiveTracker()
	qm := storage.NewQuotaManager(cfg.StorageQuotaBytes, cfg.DataDir, db, bs, activeTracker)

	// Initialize Moderation Policy Engine
	modEngine, err := moderation.NewEngine(db, moderation.Config{
		NetworkID:              "resonamesh-main",
		PolicyID:               "policy-default",
		PolicyEpoch:            1,
		TrustedModeratorPubKey: cfg.TrustedModeratorPubKey,
	}, privKey)
	if err != nil {
		slog.Error("Failed to initialize moderation engine", "error", err)
		os.Exit(1)
	}

	cat := catalog.New(db, bs)
	imp := importer.NewImporter(bs, db)
	imp.SetQuotaManager(qm)
	imp.SetBlockChecker(modEngine)

	str := player.NewStreamer(cat, bs)
	str.SetActiveTracker(activeTracker)
	str.SetQuotaManager(qm)
	str.SetBlockChecker(modEngine)

	// 5. Initialize HTTP REST & streaming server
	apiServer := api.NewServer(cfg, cat, imp, str, privKey)

	// 6. Initialize P2P Host and Swarm engine
	hostConfig := p2p.HostConfig{
		PrivateKey:         privKey,
		DisableRelay:       cfg.DisableRelay,
		EnableRelayService: cfg.Headless && !cfg.DisableRelay,
		EnableNATPortMap:   cfg.NATPortMapEnabled,
	}

	if cfg.Headless {
		// In headless container/cloud relay mode (e.g. Render), single-port co-hosting is enabled:
		// Libp2p WebSocket transport binds to the HTTP port and upgrades P2P streams, while routing
		// HTTP browser requests (Status Dashboard) to the apiServer HTTP handler.
		h, portStr, err := net.SplitHostPort(cfg.HTTPAddr)
		if err == nil {
			hostConfig.ListenAddrs = []string{
				fmt.Sprintf("/ip4/0.0.0.0/tcp/%d", cfg.P2PPort),
				fmt.Sprintf("/ip4/0.0.0.0/udp/%d/quic-v1", cfg.P2PPort),
				fmt.Sprintf("/ip4/%s/tcp/%s/ws", h, portStr),
			}
			hostConfig.HTTPHandler = apiServer.Handler()
		} else {
			hostConfig.ListenAddrs = []string{
				fmt.Sprintf("/ip4/0.0.0.0/tcp/%d", cfg.P2PPort),
				fmt.Sprintf("/ip4/0.0.0.0/udp/%d/quic-v1", cfg.P2PPort),
			}
		}
	} else {
		hostConfig.ListenAddrs = []string{
			fmt.Sprintf("/ip4/0.0.0.0/tcp/%d", cfg.P2PPort),
			fmt.Sprintf("/ip4/0.0.0.0/udp/%d/quic-v1", cfg.P2PPort),
		}
	}

	p2pHost, err := p2p.NewHost(hostConfig)
	if err != nil {
		slog.Error("Failed to initialize libp2p host", "error", err)
		os.Exit(1)
	}
	defer p2pHost.Close()

	peerTracker := p2p.NewPeerTracker()

	// 7. Register P2P block exchange wire protocol
	blockHandler := p2p.NewProtocolHandler(p2pHost, &catalogBlockProvider{bs: bs})
	blockHandler.SetBlockChecker(modEngine)

	// 8. Register P2P manifest synchronization protocol
	syncer, err := p2p.NewCatalogSyncer(p2pHost, cat)
	if err != nil {
		slog.Error("Failed to initialize catalog syncer", "error", err)
		os.Exit(1)
	}

	// Register P2P Peer Exchange (PEX) wire protocol for swarm relay discovery
	pexHandler := p2p.NewPeerExchange(p2pHost, peerTracker)
	pexHandler.SetDisableRelay(cfg.DisableRelay)

	// Register universal swarm network connection listener
	p2pHost.Network().Notify(&network.NotifyBundle{
		ConnectedF: func(n network.Network, c network.Conn) {
			remotePeer := c.RemotePeer()
			remoteAddr := c.RemoteMultiaddr()
			slog.Info("Swarm peer connection established", "peer_id", remotePeer.String(), "addr", remoteAddr.String())
			peerTracker.RecordPeer(peer.AddrInfo{ID: remotePeer, Addrs: []multiaddr.Multiaddr{remoteAddr}}, true)

			// Automatically sync catalog manifests with newly connected peer
			go func() {
				syncCtx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
				defer cancel()
				if count, err := syncer.SyncFromPeer(syncCtx, remotePeer); err != nil {
					slog.Debug("Catalog sync finished with notice", "peer_id", remotePeer.String(), "err", err)
				} else if count > 0 {
					slog.Info("Successfully synced manifests from connected peer", "peer_id", remotePeer.String(), "imported_tracks", count)
				}
			}()

			// Attempt circuit relay reservation so remote NAT peers can reach us via this peer
			if !cfg.DisableRelay {
				go func() {
					resCtx, cancelRes := context.WithTimeout(context.Background(), 8*time.Second)
					defer cancelRes()
					if res, err := p2pHost.ReserveRelay(resCtx, remotePeer); err == nil && res != nil {
						slog.Info("Active circuit relay reservation confirmed", "relay_peer", remotePeer.String())
					}
				}()
			}

			// Automatically query PEX to discover other swarm members (e.g. routed through relay)
			go func() {
				pexCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
				defer cancel()
				if discovered, err := pexHandler.QueryPeers(pexCtx, remotePeer); err == nil && len(discovered) > 0 {
					slog.Info("Discovered and connected to new peers via PEX", "count", len(discovered))
				}
			}()
		},
		DisconnectedF: func(n network.Network, c network.Conn) {
			remotePeer := c.RemotePeer()
			if len(n.ConnsToPeer(remotePeer)) == 0 {
				peerTracker.SetConnected(remotePeer, false)
				slog.Info("Swarm peer disconnected", "peer_id", remotePeer.String())
			}
		},
	})

	// Register P2P moderation wire protocol
	modHandler := p2p.NewModerationHandler(p2pHost, modEngine, nil)
	modHandler.Start(ctx)
	defer modHandler.Stop()

	// 9. Start mDNS peer discovery and tracker
	discovery := p2p.NewDiscoveryService(p2pHost, peerTracker, p2p.DefaultMDNSServiceTag)
	discovery.SetOnPeerConnected(func(peerID peer.ID) {
		slog.Info("mDNS peer connected, triggering auto-sync of catalog manifests", "peer_id", peerID.String())
		syncCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		if count, err := syncer.SyncFromPeer(syncCtx, peerID); err != nil {
			slog.Debug("Auto catalog sync with peer finished with notice", "peer_id", peerID.String(), "err", err)
		} else if count > 0 {
			slog.Info("Successfully synced manifests from swarm peer", "peer_id", peerID.String(), "imported_tracks", count)
		}
	})
	if err := discovery.Start(); err != nil {
		slog.Warn("Failed to start mDNS peer discovery", "error", err)
	}
	defer discovery.Close()

	// 9b. Start WAN dynamic bootstrap resolver and periodic reconnect loop
	if len(cfg.BootstrapSeeds) > 0 {
		slog.Info("Starting dynamic bootstrap resolver", "seeds_count", len(cfg.BootstrapSeeds))
		p2p.StartBootstrapLoop(ctx, p2pHost, peerTracker, cfg.BootstrapSeeds, 2*time.Minute, func(peerID peer.ID) {
			slog.Info("WAN bootstrap peer connected, triggering auto-sync of catalog manifests", "peer_id", peerID.String())

			// Attempt circuit relay reservation on bootstrap peer
			if !cfg.DisableRelay {
				go func() {
					resCtx, cancelRes := context.WithTimeout(context.Background(), 8*time.Second)
					defer cancelRes()
					if res, err := p2pHost.ReserveRelay(resCtx, peerID); err == nil && res != nil {
						slog.Info("Active circuit relay reservation confirmed on bootstrap relay", "relay_peer", peerID.String())
					}
				}()
			}

			syncCtx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
			defer cancel()
			if count, err := syncer.SyncFromPeer(syncCtx, peerID); err != nil {
				slog.Debug("Auto catalog sync with bootstrap peer finished with notice", "peer_id", peerID.String(), "err", err)
			} else if count > 0 {
				slog.Info("Successfully synced manifests from bootstrap peer", "peer_id", peerID.String(), "imported_tracks", count)
			}

			// Query bootstrap peer for other swarm peers via PEX
			pexCtx, cancelPex := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancelPex()
			if discovered, err := pexHandler.QueryPeers(pexCtx, peerID); err == nil && len(discovered) > 0 {
				slog.Info("Discovered additional peers from bootstrap relay via PEX", "count", len(discovered))
			}
		})
	}

	// 9c. Periodic PEX discovery loop for continuous dynamic swarm convergence
	go func() {
		ticker := time.NewTicker(15 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				for _, p := range p2pHost.ConnectedPeers() {
					pexCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
					if discovered, err := pexHandler.QueryPeers(pexCtx, p); err == nil && len(discovered) > 0 {
						slog.Info("Discovered and connected to swarm peers via periodic PEX", "count", len(discovered), "via", p.String())
					}
					cancel()
				}
			}
		}
	}()

	// 10. Wire swarm fetcher into audio streamer
	fetcher := p2p.NewFetcher(p2pHost, bs, cat)
	fetcher.SetQuotaManager(qm)
	fetcher.SetBlockChecker(modEngine)
	str.SetFetcher(fetcher)

	// 11. Initialize Keeper background replication service
	keeper := p2p.NewKeeper(p2p.KeeperConfig{
		Enabled:      cfg.KeeperEnabled,
		Interval:     30 * time.Second,
		TargetCopies: 2,
	}, cat, fetcher, str, qm)
	if err := keeper.Start(ctx); err != nil {
		slog.Warn("Failed to start keeper service", "error", err)
	}
	defer keeper.Stop()

	// 12. Wire P2P dependencies and launch HTTP server if not co-hosted
	apiServer.SetPeerTracker(peerTracker)
	apiServer.SetSyncer(syncer)
	apiServer.SetQuotaManager(qm)
	apiServer.SetP2PHost(p2pHost)
	apiServer.SetModerationEngine(modEngine, nil)

	serverErrors := make(chan error, 1)
	if !cfg.Headless || hostConfig.HTTPHandler == nil {
		go func() {
			if err := apiServer.Start(); err != nil {
				serverErrors <- err
			}
		}()
	} else {
		slog.Info("Headless relay co-hosting HTTP and WebSocket on unified port", "http_addr", cfg.HTTPAddr)
	}

	slog.Info("ResonaMesh node running. Awaiting shutdown signal...",
		"http", cfg.HTTPAddr,
		"p2p_peer_id", p2pHost.ID().String(),
	)

	select {
	case err := <-serverErrors:
		slog.Error("HTTP API server failed", "error", err)
	case <-ctx.Done():
		slog.Info("Shutdown signal received. Commencing graceful teardown...")
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := apiServer.Shutdown(shutdownCtx); err != nil {
		slog.Error("HTTP server shutdown error", "error", err)
	}

	slog.Info("ResonaMesh node shutdown completed cleanly.")
}

type catalogBlockProvider struct {
	bs *storage.BlockStore
}

func (p *catalogBlockProvider) GetBlock(_ context.Context, _ string, _ int, blockHash string) ([]byte, error) {
	return p.bs.Get(blockHash)
}

