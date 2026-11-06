# ResonaMesh

**Decentralized P2P Audio Streaming Application with Local Autonomy**

ResonaMesh is a peer-to-peer audio streaming and distribution platform engineered to operate reliably in local networks and fragmented environments without requiring an active global internet connection or centralized streaming servers. When WAN connectivity is available, the mesh automatically extends across NAT boundaries using STUN-resolved endpoints, UDP hole punching (DCUtR), and circuit relay fallback.

Developed as a Bachelor's Thesis in Software Engineering at the Faculty of Information Technology, Taras Shevchenko National University of Kyiv.

---

## Key Capabilities

- **Autonomous Local Mesh**: Operates across local area networks (LAN) using mDNS peer discovery and direct multiaddr dialing without cloud dependencies.
- **WAN NAT Traversal**: Extends beyond LAN using RFC 5389 STUN resolution, UPnP/NAT-PMP port mapping, QUIC-v1 UDP transport, DCUtR coordinated hole punching, and Circuit Relay v2 fallback.
- **Range-Based Audio Streaming**: Chunks audio into 256 KiB blocks, verified via cryptographic SHA-256 hashes and signed with Ed25519 creator keys. Supports instant scrubbing via HTTP Range requests.
- **Embedded Local Catalog**: Stores track metadata and peer routing indexes in an embedded pure-Go SQLite database with instant full-text search (FTS5) supporting multilingual Unicode queries.
- **Custom Cover Art**: Upload and serve per-track cover images with automatic MIME detection and browser caching, falling back to procedurally generated neon vinyl artwork derived from content SHA-256 hashes.
- **Playlist Management**: Create, reorder, and manage playlists entirely offline. Export playlists as shareable P2P JSON manifests for 1-click import by other mesh peers.
- **Playback Queue**: Full queue management with Next/Previous navigation, Shuffle mode, Repeat mode, and automatic track advancement.
- **Privacy-Preserving Recommendations**: In-browser recommendation engine computes artist affinity, trust-weighted scoring, and discovery bias using only local listening telemetry. Zero data leaves the node.
- **Cryptographic Content Moderation**: Ed25519-signed BLOCK and RESTORE decisions propagated across the mesh via a dedicated wire protocol with abuse report outboxes and receipt verification.
- **Single-Binary Delivery**: Compiles the entire Go backend and React frontend into a standalone executable (`resonamesh.exe`) using Go 1.16+ `//go:embed`.
- **Media Session Integration**: Synchronizes playback state with native OS media controls and lock-screen widgets via the Web Media Session API.

---

## Architectural Overview

```
                   +---------------------------------------------+
                   |         Web Frontend (React / Vite)         |
                   |     - Dark DAW-grade Studio UI              |
                   |     - Dual-buffer timeline (P2P vs Play)    |
                   |     - Media Session API controller          |
                   |     - Playlists, Cover Art, Recommendations |
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
|  |  - Cover Art Serving  |  |  - Block Verifier     |  |  - Quota Manager      |  |
|  +-----------------------+  +-----------+-----------+  +-----------------------+  |
|                                         |                                         |
|  +-----------------------+              v              +-----------------------+  |
|  | internal/moderation   |  +-------------------------+|   internal/importer   |  |
|  | - Policy Engine       |  |      internal/p2p       ||  - MP3 Chunking       |  |
|  | - Outbox / Receipts   |  |  - go-libp2p Host       ||  - Manifest Signing   |  |
|  | - Decision Broadcast  |  |  - TCP + QUIC-v1 + WS   ||  - Cover Art Ingest   |  |
|  +-----------------------+  |  - Noise Encryption      |+-----------------------+  |
|                             |  - mDNS + PEX Discovery  |                          |
|                             |  - STUN + DCUtR + Relay  |                          |
|                             |  - Block Exchange Wire   |                          |
|                             |  - Catalog Syncer        |                          |
|                             |  - Keeper Replicator     |                          |
|                             +------------+-------------+                          |
+-----------------------------------------|-----------------------------------------+
                                          | libp2p (/resonamesh/*/1.0.0)
                                          v
                                +-------------------+
                                |     P2P Swarm     |
                                |  LAN + WAN Peers  |
                                +-------------------+
```

---

## NAT Traversal Strategy

ResonaMesh implements a multi-tier connectivity waterfall to maximize direct peer-to-peer connections across network boundaries:

| Priority | Mechanism | Protocol | Scope |
| :--- | :--- | :--- | :--- |
| 1 | **mDNS Discovery** | Multicast DNS | LAN only |
| 2 | **Direct Dial** | TCP + QUIC-v1 | Public IP / port-forwarded peers |
| 3 | **STUN Resolution** | RFC 5389 (Google, Cloudflare) | Discovers public reflexive IP:port |
| 4 | **UPnP / NAT-PMP** | Router IGD protocol | Automatic port forwarding on home routers |
| 5 | **DCUtR Hole Punching** | `/libp2p/dcutr` | RTT-synchronized simultaneous UDP dial through relay signaling |
| 6 | **Circuit Relay v2** | `/libp2p/circuit/relay/0.2.0/hop` | Last resort: data flows through a public relay peer |

The system prioritizes peers with public IPs or open NATs as relay candidates. Cloud relay instances (e.g. Render deployments) serve as a fallback when no community relays are available.

---

## Recommendation Engine & Swarm Radio

ResonaMesh features a fully autonomous, privacy-preserving music recommendation and auto-queue engine (`internal/catalog/recommendations.go`). All similarity computations occur strictly on the local node without transmitting listening telemetry or graph queries over the network.

Given a seed track $A$ and a catalog candidate track $B$, the engine computes a normalized composite similarity score $\text{Score}(A, B) \in [0, 1]$:

$$\text{Score}(A, B) = 0.75 \cdot J(T_A, T_B) + 0.25 \cdot S_{\text{bpm}}$$

### 1. Tag & Genre Jaccard Similarity ($J(T_A, T_B)$)
Each track is indexed by a normalized lowercase set of semantic descriptors: $T_X = \{\text{genre}_X, \text{mood}_X, \text{artist}_X\} \cup \text{tags}_X$. Set similarity is evaluated via the Jaccard coefficient:

$$J(T_A, T_B) = \frac{|T_A \cap T_B|}{|T_A \cup T_B|}$$

### 2. Harmonic Tempo Proximity ($S_{\text{bpm}}$)
When tempo metadata ($\text{BPM} > 0$) is available on both tracks, tempo proximity is calculated while accounting for musical double-time and half-time harmonic transitions:

$$\Delta_{\text{effective}} = \min\left( |\text{BPM}_A - \text{BPM}_B|, \; |\text{BPM}_A - 2 \cdot \text{BPM}_B|, \; |\text{BPM}_A - 0.5 \cdot \text{BPM}_B| \right)$$

$$S_{\text{bpm}} = \max\left(0, \; 1 - \frac{\Delta_{\text{effective}}}{60}\right)$$

When the queue is depleted and "Swarm Radio" is active, the audio player automatically queries `GET /api/v1/tracks/{id}/recommendations`, queues the highest-ranking unplayed track, and continues continuous playback from the local library or mesh peers.

---

## Technology Stack

| Layer | Technology | Rationale |
| :--- | :--- | :--- |
| **Backend Runtime** | Go 1.27+ | High concurrency, low memory footprint, single-binary compilation. |
| **P2P Networking** | `go-libp2p` v0.50 | TCP, QUIC-v1, WebSocket transports; Noise encryption; DCUtR hole punching; Circuit Relay v2. |
| **Storage & Search** | `modernc.org/sqlite` | Pure Go implementation (no CGO required; seamless Windows/Linux compilation). |
| **Cryptography** | Standard Go `crypto/ed25519`, `crypto/sha256` | Content verification, author authenticity, tamper resistance. |
| **Frontend** | React 19, TypeScript, Vite | Modern component architecture, fast build pipeline, strict typing. |
| **Observability** | Standard Go `log/slog` | Structured JSON and text logging without third-party logging overhead. |

---

## Quick Start

### Prerequisites

- **Go**: 1.27 or higher
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

### 2. Code Coverage
Run coverage analysis across all packages:
```bash
go test -cover ./...
```
Core domain packages maintain 63-100% statement coverage (average ~78%), with `pkg/protocol` at 100% and `internal/audio` at 97.7%.

### 3. Empirical Research Experiments (E1 — E3)
Executes scientific trials and outputs raw CSV metrics to `experiments/results/`:
```bash
# Run benchmark experiments (E1, E2, E3)
go run ./cmd/benchmark

# Generate statistical summary analysis
go run ./experiments/analysis/analyze.go
```
- **E1:** Full download (B0) vs Sequential demand (B1) vs Bounded prefetch (B2) (20+ trials per mode).
- **E2:** Source failure and Keeper replica failover latency (10+ trials).
- **E3:** Network partition healing and cryptographic moderation convergence (10+ trials).

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
│   ├── audio/               # Audio chunking, SHA-256 block hashing
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

## Contributing

1. Fork the repository and create a feature branch from `develop`.
2. Ensure all tests pass: `go test ./...`
3. Ensure the frontend builds cleanly: `cd web && npm run build`
4. Follow [Conventional Commits](https://www.conventionalcommits.org/) for all commit messages.
5. Open a pull request targeting `develop`.

---

## License

This project is licensed under the GNU General Public License v3.0 — see the [LICENSE](LICENSE) file for details.
