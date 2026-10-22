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
                      +---------------------------------------+
                      |       Web Frontend (React / Vite)     |
                      |  - Dark DAW-grade Studio UI           |
                      |  - Dual-buffer timeline (P2P vs Play) |
                      |  - Media Session API controller       |
                      +-------------------+-------------------+
                                          | HTTP / SSE (127.0.0.1:8080)
                                          v
+-----------------------------------------------------------------------------------+
|                        ResonaMesh Core Node (Go Backend)                          |
|                                                                                   |
|  +--------------------+   +---------------------+   +--------------------------+  |
|  |    internal/api    |   |   internal/audio    |   |     internal/storage     |  |
|  | - REST Handlers    |   | - 256 KiB Chunker   |   | - SQLite + FTS5 Catalog  |  |
|  | - SSE Event Stream |   | - Range Streamer    |   | - Disk Block Cache (LRU) |  |
|  +--------------------+   +---------------------+   +--------------------------+  |
|                                     |                                             |
|                                     v                                             |
|                      +------------------------------+                             |
|                      |         internal/p2p         |                             |
|                      |  - go-libp2p Host            |                             |
|                      |  - Noise Handshake & Yamux   |                             |
|                      |  - mDNS LAN Peer Discovery   |                             |
|                      |  - Block Exchange Protocol   |                             |
|                      +--------------+---------------+                             |
+-------------------------------------|---------------------------------------------+
                                      | libp2p (/resonamesh/*/1.0.0)
                                      v
                             +-----------------+
                             |    P2P Swarm    |
                             | Connected Peers |
                             +-----------------+
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
   git clone https://github.com/<username>/resonamesh.git
   cd resonamesh
   ```

2. Initialize your local configuration:
   ```bash
   cp .env.example .env
   ```

3. Build and launch the application:
   ```bash
   # Build the frontend assets
   cd web && npm install && npm run build && cd ..

   # Run the ResonaMesh node
   go run ./cmd/resonamesh
   ```

4. Open your browser at `http://127.0.0.1:8080` to access the ResonaMesh player.

---

## Multi-Node Local Swarm (Testing)

To simulate a multi-peer network locally on a single machine:

```bash
# Terminal 1: Node A (Seeder / Port 8080, P2P 4001)
go run ./cmd/resonamesh --http 127.0.0.1:8080 --p2p 4001 --data ./tmp/node_a

# Terminal 2: Node B (Leecher / Port 8081, P2P 4002)
go run ./cmd/resonamesh --http 127.0.0.1:8081 --p2p 4002 --data ./tmp/node_b
```

Node A and Node B will automatically discover each other via mDNS on the loopback adapter and begin exchanging manifest data.

---

## Repository Layout

```
.
├── cmd/
│   └── resonamesh/          # Application entry point (main.go)
├── internal/
│   ├── api/                 # Local HTTP server, REST endpoints, SSE broker
│   ├── audio/               # Audio chunking, SHA-256 hashing, range streaming
│   ├── p2p/                 # libp2p host configuration, mDNS discovery, protocols
│   └── storage/             # SQLite store, FTS5 catalog, block cache manager
├── pkg/
│   ├── crypto/              # Ed25519 key signing and verification utilities
│   └── protocol/            # Wire protocol descriptors, constants, and packet schemas
├── web/                     # React + TypeScript frontend source code
│   ├── src/
│   │   ├── features/        # Feature-driven slices (player, library, network)
│   │   └── shared/          # Reusable components, hooks, and design system
│   └── vite.config.ts       # Vite bundler configuration
├── .env.example             # Configuration template
├── .gitignore               # Tracked git exclusions
└── README.md                # Project documentation
```

---

## Verification & Testing

ResonaMesh follows strict test-driven and specification-first engineering standards:

```bash
# Run all Go unit and integration tests
go test -v -race ./...

# Run static analysis
go vet ./...

# Typecheck frontend
cd web && npm run build
```

---

## Engineering Standards

- **Commit Format**: Strict [Conventional Commits](https://www.conventionalcommits.org/) (`feat:`, `fix:`, `chore:`, etc.).
- **Code Style**: Idiomatic Go adhering to *Effective Go* and Google Go Style Guide. Zero conversational or AI-tell comments.
- **Branch Strategy**: `main` for release milestones, `develop` for integration, `feature/<name>` for isolated capability development.

---

## License

This project is licensed under the GNU General Public License v3.0 — see the [LICENSE](LICENSE) file for details.

