# ResonaMesh

**Decentralized P2P Audio Streaming Application with Local Autonomy**

ResonaMesh is a peer-to-peer audio streaming and distribution platform engineered to operate reliably in local networks and fragmented environments without requiring an active global internet connection or centralized streaming servers.

Developed as a Bachelor's Thesis in Software Engineering at the Faculty of Information Technology, Taras Shevchenko National University of Kyiv.

---

## Key Capabilities

- **Autonomous Local Mesh**: Operates across local area networks (LAN) using mDNS peer discovery and direct multiaddr dialing without cloud dependencies.
- **Range-Based Audio Streaming**: Chunks audio into 256 KiB blocks, verified via cryptographic SHA-256 hashes and signed with Ed25519 creator keys. Supports instant scrubbing via HTTP Range requests.
- **Embedded Local Catalog**: Stores track metadata and peer routing indexes in an embedded pure-Go SQLite database with instant full-text search (FTS5).
- **Single-Binary Delivery**: Compiles the entire Go backend and React frontend into a standalone standalone executable (`resonamesh.exe`) using Go 1.16+ `//go:embed`.
- **Media Session Integration**: Synchronizes playback state with native OS media controls and lock-screen widgets via the Web Media Session API.

---

## Architectural Overview

```
                   +---------------------------------------------+
                   |         Web Frontend (React / Vite)         |
                   |     - Dark DAW-grade Studio UI              |
                   |     - Dual-buffer timeline (P2P vs Play)    |
                   |     - Media Session API controller          |
                   +----------------------+----------------------+
                                          | HTTP / SSE (127.0.0.1:8080)
                                          v
+-----------------------------------------------------------------------------------+
|                        ResonaMesh Core Node (Go Backend)                          |
|                                                                                   |
|  +-----------------------+  +-----------------------+  +-----------------------+  |
|  |     internal/api      |  |    internal/audio     |  |   internal/storage    |  |
|  |  - REST API Handlers  |  |  - 256 KiB Chunker    |  |  - SQLite + FTS5 DB   |  |
|  |  - SSE Event Stream   |  |  - Range Streamer     |  |  - Disk Cache (LRU)   |  |
|  +-----------------------+  +-----------+-----------+  +-----------------------+  |
|                                         |                                         |
|                                         v                                         |
|                            +-------------------------+                            |
|                            |      internal/p2p       |                            |
|                            |  - go-libp2p Host       |                            |
|                            |  - Noise & Yamux Conn   |                            |
|                            |  - mDNS Peer Discovery  |                            |
|                            |  - Block Exchange Wire  |                            |
|                            +------------+------------+                            |
+-----------------------------------------|-----------------------------------------+
                                          | libp2p (/resonamesh/*/1.0.0)
                                          v
                                +-------------------+
                                |     P2P Swarm     |
                                |  Connected Peers  |
                                +-------------------+
```

---

## Technology Stack

| Layer | Technology | Rationale |
| :--- | :--- | :--- |
| **Backend Runtime** | Go 1.22+ | High concurrency, low memory footprint, single-binary compilation. |
| **P2P Networking** | `go-libp2p` | Modular transport, Noise encryption, Yamux multiplexing, mDNS discovery. |
| **Storage & Search** | `modernc.org/sqlite` | Pure Go implementation (no CGO required; seamless Windows/Linux compilation). |
| **Cryptography** | Standard Go `crypto/ed25519`, `crypto/sha256` | Content verification, author authenticity, tamper resistance. |
| **Frontend** | React 19, TypeScript, Vite | Modern component architecture, fast build pipeline, strict typing. |
| **Observability** | Standard Go `log/slog` | Structured JSON and text logging without third-party logging overhead. |

---

## Quick Start

### Prerequisites

- **Go**: 1.22 or higher
- **Node.js**: 20.x or higher (for frontend build)
- **Git**

### Installation & Configuration

1. Clone the repository:
   ```bash
   git clone https://github.com/shed-crypto/resona-mesh.git
   cd resona-mesh
   ```


2. Initialize your local configuration:
   ```bash
   cp .env.example .env
   ```

3. Build the single standalone binary:
   ```bash
   # Using GNU Make
   make all

   # Or using PowerShell on Windows
   .\scripts\build.ps1
   ```

4. Run the standalone binary:
   ```bash
   .\resonamesh.exe --http 127.0.0.1:8080 --p2p 4001
   ```

5. Open your browser at `http://127.0.0.1:8080` to access the ResonaMesh player.

---

## Multi-Node Docker Testbed

A 4-node containerized mesh cluster is available under `deploy/compose/`:
- **Node A (Publisher / Seeder):** `http://localhost:8081` (P2P: 4001)
- **Node B (Listener / Consumer):** `http://localhost:8082` (P2P: 4002)
- **Node C (Keeper / Autonomous Replica):** `http://localhost:8083` (P2P: 4003)
- **Node D (Network Governance / Moderator):** `http://localhost:8084` (P2P: 4004)

```bash
# Build and launch all 4 nodes in background
docker compose -f deploy/compose/docker-compose.yml up --build -d

# View live cluster logs
docker compose -f deploy/compose/docker-compose.yml logs -f

# Teardown cluster and wipe test volumes
docker compose -f deploy/compose/docker-compose.yml down -v
```

---

## Acceptance Testing & Research Benchmarks

### 1. Automated Acceptance Test Suite (T01 — T16)
Executes all 16 end-to-end integration tests using in-memory loopback libp2p test hosts:
```bash
go test -v -count=1 ./tests/integration
```
Covers: clean launch, manifest tampering detection, P2P discovery & streaming, multilingual FTS5 catalog search, HTTP Range compliance, cold-cache prefetching, publisher failover, quota & LRU eviction, Keeper replication, abuse report outbox queues, and moderation lifecycles.

### 2. Empirical Research Experiments (E1 — E3)
Executes scientific trials and outputs raw CSV metrics to `experiments/results/`:
```bash
# Run benchmark experiments (E1, E2, E3)
go run ./cmd/benchmark

# Generate statistical summary analysis
go run ./experiments/analysis/analyze.go
```
- **E1:** Full download (B0) vs Sequential demand (B1) vs Bounded prefetch (B2) (≥ 20 trials per mode).
- **E2:** Source failure and Keeper replica failover latency (≥ 10 trials).
- **E3:** Network partition healing and cryptographic moderation convergence (≥ 10 trials).

Detailed findings are documented in [experiments/results/analysis_summary.md](experiments/results/analysis_summary.md) and [docs/THESIS_COMPLIANCE.md](docs/THESIS_COMPLIANCE.md).

---

## Repository Layout

```
.
├── cmd/
│   ├── benchmark/           # E1-E3 benchmark execution CLI
│   └── resonamesh/          # Application entry point (main.go)
├── deploy/
│   └── compose/             # 4-node Docker Compose testbed & Dockerfile
├── docs/                    # Thesis compliance matrix and architecture docs
├── experiments/
│   ├── analysis/            # Statistical aggregation scripts
│   └── results/             # Raw CSV datasets (E1, E2, E3) and summary
├── internal/
│   ├── api/                 # Local HTTP server, REST endpoints, web embed
│   ├── catalog/             # SQLite + FTS5 catalog and availability tracking
│   ├── config/              # CLI and environment variable configuration
│   ├── importer/            # MP3 chunking, licensing, manifest generation
│   ├── moderation/          # Policy engine, outbox, receipt & decision log
│   ├── p2p/                 # libp2p host, wire protocols, syncer, fetcher, keeper
│   ├── player/              # HTTP Range streaming adapter & active tracking
│   └── storage/             # Content-addressed blockstore & LRU quota manager
├── pkg/
│   ├── crypto/              # Ed25519 signatures, domains, SHA-256 chunk hashing
│   └── protocol/            # Wire protocol constants, canonical serializers
├── tests/
│   └── integration/         # T01-T16 automated acceptance test suite
├── web/                     # React 19 + TypeScript DAW-grade frontend
│   └── dist/                # Pre-compiled static assets embedded in Go binary
├── Makefile                 # Automation build targets
├── .env.example             # Configuration template
└── README.md                # Project documentation
```

---

## License

This project is licensed under the GNU General Public License v3.0 — see the [LICENSE](LICENSE) file for details.


