package updater

import (
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"resonamesh/pkg/crypto"
	"resonamesh/pkg/protocol"
)

var (
	ErrInvalidManifest     = errors.New("updater: invalid release manifest")
	ErrUntrustedReleaseKey = errors.New("updater: untrusted release authority public key")
	ErrArchitectureMismatch = errors.New("updater: target os or architecture mismatch")
	ErrVersionNotNewer     = errors.New("updater: release version is not newer than current")
	ErrNoReleaseAvailable  = errors.New("updater: no release available to download")
	ErrNotReadyToApply     = errors.New("updater: release binary not ready to apply")
	ErrIntegrityCheckFailed = errors.New("updater: assembled binary checksum does not match manifest")
)

type UpdateStatus string

const (
	StatusIdle        UpdateStatus = "idle"
	StatusAvailable   UpdateStatus = "available"
	StatusDownloading UpdateStatus = "downloading"
	StatusReady       UpdateStatus = "ready_to_restart"
	StatusError       UpdateStatus = "error"
)

// State describes the current runtime status and progress of the updater.
type State struct {
	Status           UpdateStatus              `json:"status"`
	CurrentVersion   string                    `json:"current_version"`
	AvailableRelease *protocol.ReleaseManifest `json:"available_release,omitempty"`
	ProgressPct      float64                   `json:"progress_pct"`
	DownloadedBlocks int                       `json:"downloaded_blocks"`
	TotalBlocks      int                       `json:"total_blocks"`
	ErrorMessage     string                    `json:"error_message,omitempty"`
	StagedPath       string                    `json:"staged_path,omitempty"`
}

// BlockProvider provides access to content-addressed block storage.
type BlockProvider interface {
	Get(hash string) ([]byte, error)
	Put(hash string, data []byte) error
	Has(hash string) bool
}

// RemoteBlockFetcher allows pulling missing release blocks from swarm peers.
type RemoteBlockFetcher interface {
	FetchBlock(ctx context.Context, contentID string, blockIndex int, blockHash string) ([]byte, error)
}

// Config provides dependencies for the auto-updater engine.
type Config struct {
	CurrentVersion   string
	ExecutablePath   string
	ReleasePubKey    string
	DataDir          string
	BlockStore       BlockProvider
	RemoteFetcher    RemoteBlockFetcher
}

// Engine coordinates release discovery, chunk downloading, verification, and atomic executable swapping.
type Engine struct {
	cfg Config
	mu  sync.RWMutex

	status           UpdateStatus
	availableRelease *protocol.ReleaseManifest
	progressPct      float64
	downloadedBlocks int
	totalBlocks      int
	errorMessage     string
	stagedPath       string
}

// NewEngine constructs and initializes the auto-updater.
func NewEngine(cfg Config) (*Engine, error) {
	if cfg.CurrentVersion == "" {
		cfg.CurrentVersion = "0.1.0-alpha"
	}
	if cfg.ExecutablePath == "" {
		exe, err := os.Executable()
		if err == nil {
			cfg.ExecutablePath = exe
		}
	}
	if cfg.DataDir == "" {
		cfg.DataDir = "./data"
	}

	updatesDir := filepath.Join(cfg.DataDir, "updates")
	if err := os.MkdirAll(updatesDir, 0700); err != nil {
		return nil, fmt.Errorf("updater: failed to initialize updates staging directory: %w", err)
	}

	return &Engine{
		cfg:    cfg,
		status: StatusIdle,
	}, nil
}

// SetRemoteFetcher attaches a peer block fetcher once P2P host is online.
func (e *Engine) SetRemoteFetcher(rf RemoteBlockFetcher) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.cfg.RemoteFetcher = rf
}

// State returns a snapshot of the current update state.
func (e *Engine) State() State {
	e.mu.RLock()
	defer e.mu.RUnlock()

	return State{
		Status:           e.status,
		CurrentVersion:   e.cfg.CurrentVersion,
		AvailableRelease: e.availableRelease,
		ProgressPct:      e.progressPct,
		DownloadedBlocks: e.downloadedBlocks,
		TotalBlocks:      e.totalBlocks,
		ErrorMessage:     e.errorMessage,
		StagedPath:       e.stagedPath,
	}
}

// VerifyManifest performs cryptographic verification and architecture checks on a release manifest.
func (e *Engine) VerifyManifest(rel *protocol.ReleaseManifest) error {
	if rel == nil {
		return ErrInvalidManifest
	}
	if rel.SchemaVersion != protocol.SchemaReleaseV1 {
		return fmt.Errorf("%w: unsupported schema %q", ErrInvalidManifest, rel.SchemaVersion)
	}

	// Platform architecture check
	targetOS := strings.ToLower(strings.TrimSpace(rel.TargetOS))
	targetArch := strings.ToLower(strings.TrimSpace(rel.TargetArch))
	if targetOS != runtime.GOOS || targetArch != runtime.GOARCH {
		return fmt.Errorf("%w: manifest specifies %s/%s, local platform is %s/%s",
			ErrArchitectureMismatch, targetOS, targetArch, runtime.GOOS, runtime.GOARCH)
	}

	// Trusted release key verification
	expectedPubKey := strings.ToLower(strings.TrimSpace(e.cfg.ReleasePubKey))
	manifestPubKey := strings.ToLower(strings.TrimSpace(rel.ReleasePubKey))
	if expectedPubKey != "" && !strings.EqualFold(expectedPubKey, manifestPubKey) {
		return fmt.Errorf("%w: manifest key %s does not match trusted key %s",
			ErrUntrustedReleaseKey, manifestPubKey, expectedPubKey)
	}

	pubBytes, err := crypto.HexToPublicKey(manifestPubKey)
	if err != nil {
		return fmt.Errorf("%w: invalid public key hex: %v", ErrInvalidManifest, err)
	}

	sigBytes, err := hex.DecodeString(rel.Signature)
	if err != nil {
		return fmt.Errorf("%w: invalid signature hex: %v", ErrInvalidManifest, err)
	}

	canonicalPayload := protocol.CanonicalReleasePayload(rel)
	if !crypto.VerifyWithDomain(pubBytes, crypto.DomainReleaseManifest, []byte(canonicalPayload), sigBytes) {
		return fmt.Errorf("%w: cryptographic signature check failed", ErrInvalidManifest)
	}

	return nil
}

// ProcessAnnouncedRelease verifies and registers an announced release from the swarm.
func (e *Engine) ProcessAnnouncedRelease(rel *protocol.ReleaseManifest) (bool, error) {
	if err := e.VerifyManifest(rel); err != nil {
		return false, err
	}

	// Version check: only proceed if announced release is newer than current running binary
	cmp := CompareVersions(rel.Version, e.cfg.CurrentVersion)
	if cmp <= 0 {
		return false, ErrVersionNotNewer
	}

	e.mu.Lock()
	defer e.mu.Unlock()

	// If already ready or newer release already staged, evaluate
	if e.availableRelease != nil {
		if CompareVersions(rel.Version, e.availableRelease.Version) <= 0 {
			return false, nil
		}
	}

	e.availableRelease = rel
	e.status = StatusAvailable
	e.progressPct = 0
	e.downloadedBlocks = 0
	e.totalBlocks = len(rel.BlockHashes)
	e.errorMessage = ""

	slog.Info("New verified release manifest accepted", "version", rel.Version, "os", rel.TargetOS, "arch", rel.TargetArch)
	return true, nil
}

// DownloadRelease downloads and verifies all constituent blocks, assembling the staged binary.
func (e *Engine) DownloadRelease(ctx context.Context) error {
	e.mu.Lock()
	if e.availableRelease == nil {
		e.mu.Unlock()
		return ErrNoReleaseAvailable
	}
	rel := e.availableRelease
	e.status = StatusDownloading
	e.progressPct = 0
	e.downloadedBlocks = 0
	e.totalBlocks = len(rel.BlockHashes)
	e.errorMessage = ""
	e.mu.Unlock()

	stagedFilename := fmt.Sprintf("resonamesh-%s-%s-%s.staged", rel.Version, rel.TargetOS, rel.TargetArch)
	if runtime.GOOS == "windows" {
		stagedFilename += ".exe"
	}
	stagedPath := filepath.Join(e.cfg.DataDir, "updates", stagedFilename)

	tempFile, err := os.OpenFile(stagedPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0755)
	if err != nil {
		e.setError(fmt.Sprintf("failed to create staged binary file: %v", err))
		return err
	}
	defer tempFile.Close()

	hasher := sha256.New()
	writer := io.MultiWriter(tempFile, hasher)

	for idx, blockHash := range rel.BlockHashes {
		select {
		case <-ctx.Done():
			e.setError("download canceled")
			return ctx.Err()
		default:
		}

		var blockData []byte
		// 1. Check local BlockStore
		if e.cfg.BlockStore != nil {
			if e.cfg.BlockStore.Has(blockHash) {
				data, err := e.cfg.BlockStore.Get(blockHash)
				if err == nil && crypto.VerifyChunk(data, blockHash) {
					blockData = data
				}
			}
		}

		// 2. Fetch remotely if not cached locally
		if blockData == nil {
			if e.cfg.RemoteFetcher == nil {
				err := fmt.Errorf("block %s not in local store and remote fetcher not configured", blockHash)
				e.setError(err.Error())
				return err
			}

			data, err := e.cfg.RemoteFetcher.FetchBlock(ctx, rel.BinaryHash, idx, blockHash)
			if err != nil {
				e.setError(fmt.Sprintf("failed to fetch block %d (%s): %v", idx, blockHash, err))
				return err
			}
			blockData = data

			// Store for seeding to other nodes
			if e.cfg.BlockStore != nil {
				_ = e.cfg.BlockStore.Put(blockHash, blockData)
			}
		}

		if _, err := writer.Write(blockData); err != nil {
			e.setError(fmt.Sprintf("failed to write chunk payload to disk: %v", err))
			return err
		}

		e.mu.Lock()
		e.downloadedBlocks++
		if e.totalBlocks > 0 {
			e.progressPct = float64(e.downloadedBlocks) / float64(e.totalBlocks) * 100.0
		}
		e.mu.Unlock()
	}

	if err := tempFile.Sync(); err != nil {
		e.setError(fmt.Sprintf("failed to flush staged binary: %v", err))
		return err
	}

	// Verify complete binary hash
	actualDigest := hex.EncodeToString(hasher.Sum(nil))
	if !strings.EqualFold(actualDigest, rel.BinaryHash) {
		_ = os.Remove(stagedPath)
		err := fmt.Errorf("%w: expected %s, got %s", ErrIntegrityCheckFailed, rel.BinaryHash, actualDigest)
		e.setError(err.Error())
		return err
	}

	e.mu.Lock()
	e.status = StatusReady
	e.stagedPath = stagedPath
	e.progressPct = 100.0
	e.mu.Unlock()

	slog.Info("Release binary assembled and cryptographically verified",
		"version", rel.Version, "staged_path", stagedPath, "size_bytes", rel.TotalSizeBytes)
	return nil
}

// ApplyUpdate executes the atomic replacement of the running executable.
func (e *Engine) ApplyUpdate() error {
	e.mu.Lock()
	defer e.mu.Unlock()

	if e.status != StatusReady || e.stagedPath == "" {
		return ErrNotReadyToApply
	}

	targetExe := e.cfg.ExecutablePath
	if targetExe == "" {
		return errors.New("updater: target executable path is empty")
	}

	// 1. Cross-platform atomic replacement
	backupPath := targetExe + ".old"
	_ = os.Remove(backupPath)

	// Rename running binary to .old (permitted on Windows NTFS and POSIX)
	if err := os.Rename(targetExe, backupPath); err != nil {
		return fmt.Errorf("updater: failed to move existing executable to backup: %w", err)
	}

	// Move staged binary to target executable
	if err := os.Rename(e.stagedPath, targetExe); err != nil {
		// Rollback attempt
		_ = os.Rename(backupPath, targetExe)
		return fmt.Errorf("updater: failed to replace target executable: %w (rollback performed)", err)
	}

	// Ensure executable permissions on POSIX
	_ = os.Chmod(targetExe, 0755)

	slog.Info("Autonomous atomic binary update applied successfully", "executable", targetExe, "backup", backupPath)
	return nil
}

// Rollback restores the previous executable backup if an update fails to launch.
func (e *Engine) Rollback() error {
	e.mu.Lock()
	defer e.mu.Unlock()

	targetExe := e.cfg.ExecutablePath
	backupPath := targetExe + ".old"

	if _, err := os.Stat(backupPath); err != nil {
		return fmt.Errorf("updater: no backup executable found to restore: %w", err)
	}

	_ = os.Remove(targetExe)
	if err := os.Rename(backupPath, targetExe); err != nil {
		return fmt.Errorf("updater: failed to restore backup executable: %w", err)
	}

	slog.Info("Executable rollback successfully completed", "restored_path", targetExe)
	return nil
}

// CreateRelease reads an executable binary, splits it into 256 KiB blocks, saves them to BlockStore, and signs the manifest.
func (e *Engine) CreateRelease(
	binaryPath string,
	version string,
	releaseNotes string,
	privKey ed25519.PrivateKey,
) (*protocol.ReleaseManifest, error) {
	file, err := os.Open(binaryPath)
	if err != nil {
		return nil, fmt.Errorf("updater: failed to open source binary: %w", err)
	}
	defer file.Close()

	stat, err := file.Stat()
	if err != nil {
		return nil, fmt.Errorf("updater: failed to stat source binary: %w", err)
	}

	hasher := sha256.New()
	blockHashes := make([]string, 0)
	buf := make([]byte, protocol.BlockSize)

	for {
		n, err := file.Read(buf)
		if n > 0 {
			chunk := buf[:n]
			hasher.Write(chunk)

			blockSum := sha256.Sum256(chunk)
			blockHash := hex.EncodeToString(blockSum[:])
			blockHashes = append(blockHashes, blockHash)

			if e.cfg.BlockStore != nil {
				if err := e.cfg.BlockStore.Put(blockHash, chunk); err != nil {
					return nil, fmt.Errorf("updater: failed to cache release block in blockstore: %w", err)
				}
			}
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("updater: read error while chunking binary: %w", err)
		}
	}

	binaryDigest := hex.EncodeToString(hasher.Sum(nil))
	pubKey := privKey.Public().(ed25519.PublicKey)
	pubHex := crypto.PublicKeyToHex(pubKey)

	manifest := &protocol.ReleaseManifest{
		SchemaVersion:  protocol.SchemaReleaseV1,
		Version:        version,
		TargetOS:       runtime.GOOS,
		TargetArch:     runtime.GOARCH,
		BinaryHash:     binaryDigest,
		TotalSizeBytes: stat.Size(),
		BlockSize:      protocol.BlockSize,
		BlockHashes:    blockHashes,
		ReleasePubKey:  pubHex,
		PublishedAt:    time.Now().Unix(),
		ReleaseNotes:   releaseNotes,
	}

	canonicalPayload := protocol.CanonicalReleasePayload(manifest)
	sig := crypto.SignWithDomain(privKey, crypto.DomainReleaseManifest, []byte(canonicalPayload))
	manifest.Signature = hex.EncodeToString(sig)

	return manifest, nil
}

func (e *Engine) setError(msg string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.status = StatusError
	e.errorMessage = msg
	slog.Error("Updater encountered error", "error", msg)
}
