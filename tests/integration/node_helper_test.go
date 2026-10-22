package integration_test

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"resonamesh/internal/api"
	"resonamesh/internal/catalog"
	"resonamesh/internal/config"
	"resonamesh/internal/importer"
	"resonamesh/internal/moderation"
	"resonamesh/internal/p2p"
	"resonamesh/internal/player"
	"resonamesh/internal/storage"
	"resonamesh/pkg/crypto"
	"resonamesh/pkg/protocol"
)

type blockStoreProvider struct {
	bs *storage.BlockStore
}

func (p *blockStoreProvider) GetBlock(_ context.Context, _ string, _ int, blockHash string) ([]byte, error) {
	return p.bs.Get(blockHash)
}

// NodeOptions defines customizable parameters for an in-memory integration test node.
type NodeOptions struct {
	StorageQuotaBytes int64
	ModeratorPubKey   ed25519.PublicKey
	ModeratorPrivKey  ed25519.PrivateKey
	KeeperEnabled     bool
}

// TestNode encapsulates a fully wired autonomous ResonaMesh node running in-memory or on loopback.
type TestNode struct {
	T            *testing.T
	Dir          string
	DB           *storage.DB
	BlockStore   *storage.BlockStore
	Catalog      *catalog.Catalog
	QuotaManager *storage.QuotaManager
	Host         *p2p.Host
	Importer     *importer.Importer
	Fetcher      *p2p.Fetcher
	Streamer     *player.Streamer
	ModEngine    *moderation.Engine
	ModHandler   *p2p.ModerationHandler
	ProtoHandler *p2p.ProtocolHandler
	Syncer       *p2p.CatalogSyncer
	Keeper       *p2p.Keeper
	PrivKey      ed25519.PrivateKey
	PubKey       ed25519.PublicKey
	APIServer    *api.Server
	HTTPServer   *httptest.Server
	HTTPClient   *http.Client
	BaseURL      string
	cancel       context.CancelFunc
}

// NewTestNode creates and initializes a single isolated ResonaMesh node.
func NewTestNode(t *testing.T, opts NodeOptions) *TestNode {
	t.Helper()
	dir := t.TempDir()

	ctx, cancel := context.WithCancel(context.Background())

	pubKey, privKey, err := crypto.GenerateKeyPair()
	if err != nil {
		t.Fatalf("failed to generate node key pair: %v", err)
	}

	dbPath := filepath.Join(dir, "catalog.db")
	db, err := storage.Open(dbPath)
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}

	bs, err := storage.NewBlockStore(dir)
	if err != nil {
		t.Fatalf("failed to create blockstore: %v", err)
	}

	cat := catalog.New(db, bs)

	quotaBytes := opts.StorageQuotaBytes
	if quotaBytes <= 0 {
		quotaBytes = 100 * 1024 * 1024 // default 100 MiB for tests
	}
	activeTracker := storage.NewActiveTracker()
	qm := storage.NewQuotaManager(quotaBytes, dir, db, bs, activeTracker)

	h, err := p2p.NewHost(p2p.HostConfig{
		ListenAddr:   "/ip4/127.0.0.1/tcp/0",
		PrivateKey:   privKey,
		DisableRelay: true,
	})
	if err != nil {
		t.Fatalf("failed to create libp2p host: %v", err)
	}

	modModeratorKeyHex := ""
	if len(opts.ModeratorPubKey) > 0 {
		modModeratorKeyHex = crypto.PublicKeyToHex(opts.ModeratorPubKey)
	}
	modCfg := moderation.Config{
		NetworkID:              "resonamesh-test",
		PolicyID:               "policy-test",
		PolicyEpoch:            1,
		TrustedModeratorPubKey: modModeratorKeyHex,
	}
	modEngine, err := moderation.NewEngine(db, modCfg, privKey)
	if err != nil {
		t.Fatalf("failed to initialize moderation engine: %v", err)
	}

	fetcher := p2p.NewFetcher(h, bs, cat)
	fetcher.SetQuotaManager(qm)
	fetcher.SetBlockChecker(modEngine)

	streamer := player.NewStreamer(cat, bs)
	streamer.SetActiveTracker(activeTracker)
	streamer.SetQuotaManager(qm)
	streamer.SetBlockChecker(modEngine)
	streamer.SetFetcher(fetcher)

	imp := importer.NewImporter(bs, db)
	imp.SetQuotaManager(qm)
	imp.SetBlockChecker(modEngine)

	protoHandler := p2p.NewProtocolHandler(h, &blockStoreProvider{bs: bs})
	protoHandler.SetBlockChecker(modEngine)

	syncer, err := p2p.NewCatalogSyncer(h, cat)
	if err != nil {
		t.Fatalf("failed to create catalog syncer: %v", err)
	}

	modHandler := p2p.NewModerationHandler(h, modEngine, opts.ModeratorPrivKey)
	modHandler.Start(ctx)

	cfg := &config.Config{
		HTTPAddr:          "127.0.0.1:0",
		DataDir:           dir,
		StorageQuotaBytes: quotaBytes,
		KeeperEnabled:     opts.KeeperEnabled,
	}

	apiServer := api.NewServer(cfg, cat, imp, streamer, privKey)
	apiServer.SetModerationEngine(modEngine, opts.ModeratorPrivKey)
	apiServer.SetQuotaManager(qm)
	apiServer.SetP2PHost(h)
	apiServer.SetSyncer(syncer)

	httpTestServer := httptest.NewServer(apiServer.Handler())

	keeperCfg := p2p.DefaultKeeperConfig()
	keeperCfg.Enabled = opts.KeeperEnabled
	keeper := p2p.NewKeeper(keeperCfg, cat, fetcher, streamer, qm)

	node := &TestNode{
		T:            t,
		Dir:          dir,
		DB:           db,
		BlockStore:   bs,
		Catalog:      cat,
		QuotaManager: qm,
		Host:         h,
		Importer:     imp,
		Fetcher:      fetcher,
		Streamer:     streamer,
		ModEngine:    modEngine,
		ModHandler:   modHandler,
		ProtoHandler: protoHandler,
		Syncer:       syncer,
		Keeper:       keeper,
		PrivKey:      privKey,
		PubKey:       pubKey,
		APIServer:    apiServer,
		HTTPServer:   httpTestServer,
		HTTPClient:   httpTestServer.Client(),
		BaseURL:      httpTestServer.URL,
		cancel:       cancel,
	}

	t.Cleanup(func() {
		node.Close()
	})

	return node
}

// Close gracefully releases all node resources.
func (n *TestNode) Close() {
	if n.cancel != nil {
		n.cancel()
	}
	if n.ModHandler != nil {
		n.ModHandler.Stop()
	}
	if n.HTTPServer != nil {
		n.HTTPServer.Close()
	}
	if n.Host != nil {
		_ = n.Host.Close()
	}
	if n.DB != nil {
		_ = n.DB.Close()
	}
}

// Connect direct dials another test node over loopback TCP.
func (n *TestNode) Connect(other *TestNode) {
	n.T.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := n.Host.Connect(ctx, other.Host.AddrInfo()); err != nil {
		n.T.Fatalf("failed to connect node to peer: %v", err)
	}
}

// ImportSyntheticTrack generates and imports a multi-chunk track for testing.
func (n *TestNode) ImportSyntheticTrack(title string, sizeBytes int) *protocol.Manifest {
	n.T.Helper()
	ctx := context.Background()

	audioData := make([]byte, sizeBytes)
	titleSeed := 0
	for _, ch := range title {
		titleSeed += int(ch)
	}
	for i := range audioData {
		audioData[i] = byte((i*31 + titleSeed*17) % 256)
	}

	params := importer.ImportParams{
		Title:        title,
		Artist:       "Synthetic Performer",
		Description:  "Deterministic audio fixture for testing",
		DurationMs:   int64(sizeBytes / 24),
		BitrateKbps:  192,
		LicenseCode:  protocol.LicenseCCBY40,
		PublisherKey: n.PrivKey,
		AudioData:    bytes.NewReader(audioData),
	}

	m, err := n.Importer.Import(ctx, params)
	if err != nil {
		n.T.Fatalf("import of %q failed: %v", title, err)
	}
	return m
}
