# ResonaMesh Architectural Roadmap & Future Protocol Evolution

> **Status**: Living Architectural Specification  
> **Repository Baseline**: ResonaMesh v0.1.0-alpha  
> **Standards Compliance**: Zero External Tracking, Pure Go, Backward-Compatible Libp2p Protocol  

---

## 1. Protocol Evolution & Backward Compatibility Guarantees

In accordance with ResonaMesh Architectural Standards (Section 9), all wire protocol enhancements and storage schema migrations must maintain strict backward compatibility with legacy nodes and unupdated bootstrap relays.

### 1.1 Non-Destructive Extensibility Architecture
- **JSON Schema Invariance**: Wire messages and serialized manifests (`protocol.Manifest`) utilize non-strict JSON unmarshaling. Unknown fields added in newer versions are ignored by older clients without causing parse failures or stream termination.
- **Self-Describing Manifest Headers**: Every manifest contains explicit schema identification (`schema_version`), payload hash, and Merkle block list. Any future compression or framing changes are explicitly signaled in the manifest headers rather than inferred implicitly.
- **Cryptographic Domain Separation**: New message types use dedicated domain prefixes (e.g., `resonamesh:release:v1`, `resonamesh:tag:v1`) to prevent cross-protocol replay or collision with track manifests (`resonamesh:manifest:v1`).
- **Opportunistic Data Enrichment**: When older nodes retransmit or seed manifests originally authored by newer nodes, they preserve the raw manifest bytes and signature intact, guaranteeing that downstream modern nodes retain all extended metadata.

---

## 2. Milestone 11: Autonomous P2P Client Auto-Updater

### 2.1 Problem Statement
In decentralized and partitioned mesh networks without centralized application stores, client software must be capable of propagating verified binary updates directly through the swarm.

### 2.2 Architectural Design
1. **Release Manifest Specification (`ReleaseManifest`)**:
   - `schema_version`: `"resonamesh:release:v1"`
   - `version`: Target SemVer string (e.g. `"0.2.0"`).
   - `target_os`: Operating system identifier (`windows`, `linux`, `darwin`).
   - `target_arch`: Architecture identifier (`amd64`, `arm64`).
   - `binary_hash`: SHA-256 digest of the complete executable binary.
   - `total_size_bytes`: Total byte size of the binary.
   - `block_size`: 262,144 bytes (256 KiB blocks, standard ResonaMesh CAS block size).
   - `block_hashes`: Ordered SHA-256 Merkle hashes.
   - `release_pubkey`: Hex-encoded Ed25519 public key of the release authority (`ReleasePubKey`).
   - `signature`: Hex-encoded Ed25519 signature computed over `resonamesh:release:v1 || <canonical_release_bytes>`.
2. **Distribution via Content-Addressed BlockStore**:
   - Release binaries are chunked into standard 256 KiB blocks and stored in the local `BlockStore`.
   - The existing `Fetcher` and libp2p stream protocol (`/resonamesh/block/1.0.0`) are reused directly without modifying the wire transport layer.
3. **Gossip Notification**:
   - Nodes gossip new verified release announcements over `/resonamesh/release/1.0.0`.
   - Nodes verify the signature against the embedded immutable `ReleasePubKey` before triggering download.
4. **Atomic In-Place Binary Replacement**:
   - **Windows**: The running executable cannot be deleted directly while running, but it can be renamed. ResonaMesh renames `resonamesh.exe` to `resonamesh.exe.old`, writes the new binary to `resonamesh.exe`, and notifies the user to restart.
   - **POSIX (Linux / macOS)**: Performs an atomic `rename()` or replaces the binary file descriptor.
5. **UI Notification**:
   - Visual notification badge in `NodeStatusPanel.tsx` displaying the available version with a one-click "Apply Update & Restart" action.

---

## 3. Milestone 12: Embedded Synchronized Lyrics & Karaoke Mode

### 3.1 Problem Statement
Music streaming interfaces benefit from real-time lyrical synchronization. In a decentralized network, lyrics must reside within the distributed manifest or be resolvable via deterministic content addressing.

### 3.2 Architectural Design
1. **LRC Format Support**: Standard time-coded format `[mm:ss.xx] lyric text`.
2. **Manifest Extension**:
   - Optional `lyrics` field in `protocol.Manifest` containing raw LRC content or an embedded structured array of `{ timestamp_ms: number, line: string }`.
3. **Player Synchronization**:
   - Real-time timeupdate listener in `AudioPlayerBar.tsx` and dedicated `LyricsDrawer` component.
   - Active line highlighting with smooth automatic vertical scrolling following playback position.

---

## 4. Milestone 13: Decentralized Swarm Reputation & Telemetry

### 4.1 Problem Statement
P2P swarms require mechanisms to identify and deprioritize nodes that serve corrupt chunks, throttle connections, or misreport availability.

### 4.2 Architectural Design
1. **Local Reputation Metric (Zero Gossip Score Attack Surface)**:
   - Reputation is tracked strictly locally by each node to prevent sybil smear campaigns.
   - Metrics tracked per `peer.ID`:
     - Successful chunk deliveries vs verification failures (SHA-256 mismatch).
     - Average response latency and throughput (KiB/s).
     - Availability uptime and keeper replication assistance.
2. **Peer Selection Heuristic**:
   - `Fetcher` sorts candidate peers by local reputation score before dispatching chunk requests.
   - Peers with consecutive checksum failures are quarantined for an exponential backoff period.

---

## 5. Milestone 14: Studio Audio Suite (EQ, Speed, Timer & Fullscreen)

### 5.1 Problem Statement
Audiophiles and casual listeners require precise acoustic equalization, playback speed adjustments for audiobooks/podcasts, sleep timers, and a distraction-free fullscreen listening experience.

### 5.2 Architectural Design
1. **Web Audio API 5-Band Parametric Equalizer**:
   - Integrated into `AudioPlayerBar.tsx` using `BiquadFilterNode` chain:
     - Sub-bass / Low shelf (60 Hz)
     - Low-mid peaking (250 Hz)
     - Midrange peaking (1 kHz)
     - High-mid peaking (4 kHz)
     - Treble / High shelf (12 kHz)
   - Studio presets: Flat, Bass Boost, Acoustic, Electronic, Vocal Boost.
2. **Playback Speed Control**:
   - Variable playback rate (0.5x, 0.75x, 1.0x, 1.25x, 1.5x, 2.0x) with pitch correction preservation.
3. **Sleep Timer**:
   - Configurable countdown timer (15m, 30m, 45m, 60m, end of track) with gradual fade-out volume attenuation.
4. **Studio Fullscreen Now Playing Mode**:
   - Dedicated immersion view displaying high-resolution procedural vinyl artwork, interactive dual-buffer timeline, synchronized lyrics, and dynamic audio visualizer.

---

## 6. Milestone 15: Music Tags, Audio Features & Swarm Radio Recommendations

### 6.1 Problem Statement
When a playlist ends or a user selects a single track, modern music platforms continue playback with stylistically similar music ("Radio Mode"). In a privacy-first P2P network, recommendations must function autonomously on-device without cloud profiling or centralized user tracking.

### 6.2 Architectural Design
1. **Metadata Tag Extension**:
   - Extensible catalog fields: `genre`, `subgenres`, `tags` (e.g. `["ambient", "cyberpunk", "synth"]`), `bpm`, `musical_key`, `mood`.
2. **Zero-Telemetry Local Similarity Engine**:
   - **Jaccard Tag Similarity**: Computes set intersection over union for categorical tags:
     $$J(A, B) = \frac{|A \cap B|}{|A \cup B|}$$
   - **Audio Feature Cosine Similarity**: For numerical features (BPM, energy, valence):
     $$\text{Cosine}(u, v) = \frac{u \cdot v}{\|u\| \|v\|}$$
3. **Swarm Radio Autonomous Queue**:
   - When Radio Mode is enabled, upon nearing the end of the playback queue, the player queries local catalog and known swarm manifests to append the top-ranked candidates with diversity jitter to avoid repetition loops.
