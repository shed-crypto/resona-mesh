# ResonaMesh Architectural Roadmap & Implementation Blueprint

> **Status**: Living Architectural & Engineering Specification  
> **Repository Baseline**: ResonaMesh v0.1.0-alpha  
> **Standards Compliance**: Zero External Tracking, Pure Go (`modernc.org/sqlite`), Backward-Compatible Libp2p Protocol  

---

## 0. Current Codebase Implementation Status Matrix

Before modifying any subsystem, consult this matrix to verify what is already implemented in the repository versus what is scheduled for upcoming implementation.

| Subsystem / Feature | Status | Primary Source Files |
| :--- | :--- | :--- |
| **Dual-Stack P2P Host (QUIC + TCP + WS)** | Implemented | `internal/p2p/host.go`, `cmd/resonamesh/main.go` |
| **Shared-Socket STUN & Symmetric NAT Prediction** | Implemented | `internal/p2p/stun.go`, `internal/p2p/host.go` |
| **Direct Audio Track Download & Export (`RFC 6266`)** | Implemented | `internal/api/server.go` (`handleTrackDownload`), `web/src/features/library/TrackCard.tsx`, `web/src/features/player/AudioPlayerBar.tsx` |
| **Cryptographic Governance, Moderator Certs & Key Rotation** | Implemented | `internal/moderation/engine.go`, `pkg/protocol/types.go`, `cmd/resonamesh/main.go` (`keygen`, `sign-rotation`) |
| **Milestone 11: Autonomous P2P Client Auto-Updater** | Implemented | `internal/updater/engine.go`, `internal/updater/version.go`, `internal/p2p/release_protocol.go`, `web/src/features/network/UpdaterModal.tsx` |
| **Priority 0: Smart Relay Orchestration & Dormant Bootstrap** | Implemented | `internal/config/config.go`, `internal/p2p/host.go`, `internal/p2p/pex.go`, `internal/p2p/bootstrap.go`, `internal/p2p/relay_orchestration_test.go` |
| **Priority 1: Milestone 12 — Synchronized Lyrics (`.lrc`)** | Implemented | `pkg/protocol/types.go`, `internal/storage/sqlite.go`, `internal/catalog/catalog.go`, `internal/api/server.go`, `web/src/shared/utils/lrcParser.ts`, `web/src/features/player/LyricsDrawer.tsx` |
| **Priority 2: Milestone 14 — Studio Audio Suite (EQ & Fullscreen)** | Implemented | `web/src/features/player/useAudioEqualizer.ts`, `web/src/features/player/EqualizerDrawer.tsx`, `web/src/features/player/useSleepTimer.ts`, `web/src/features/player/FullscreenPlayerModal.tsx` |
| **Priority 3: Milestone 15 — Metadata Tags & Local Swarm Radio** | Implemented | `internal/catalog/recommendations.go`, `internal/api/server.go`, `web/src/shared/hooks/useAudioPlayer.ts`, `web/src/features/player/AudioPlayerBar.tsx` |
| **Priority 4: Milestone 13 — Local Swarm Reputation & Quarantine** | Implemented | `internal/p2p/reputation.go`, `internal/p2p/fetcher.go`, `internal/api/server.go` |
| **Priority 5: Mesh Topology Map & Bounded 2-Hop PEX Horizon** | Implemented | `internal/p2p/discovery.go`, `internal/p2p/pex.go`, `web/src/features/network/MeshTopologyMap.tsx`, `web/src/features/network/ConnectPeerModal.tsx` |

---

## 1. Protocol Evolution & Backward Compatibility Guarantees

In accordance with ResonaMesh Architectural Standards (Section 9), all wire protocol enhancements and storage schema migrations must maintain strict backward compatibility with legacy nodes and unupdated bootstrap relays.

### 1.1 Non-Destructive Extensibility Architecture
- **JSON Schema Invariance**: Wire messages (`pexRequest`, `pexResponse`, `protocol.Manifest`) utilize standard Go `encoding/json` non-strict unmarshaling. Optional fields tagged with `omitempty` are ignored by legacy nodes without causing parse errors or stream resets.
- **Self-Describing Manifest Headers**: Every manifest contains explicit schema identification (`schema_version`), payload hash, and Merkle block list.
- **Cryptographic Domain Separation**: Every signed entity uses a unique domain prefix (`resonamesh:manifest:v1`, `resonamesh:release:v1`, `resonamesh:modcert:v1`, `resonamesh:keyrot:v1`) defined in `pkg/crypto/crypto.go`.
- **Opportunistic Data Enrichment**: When older nodes retransmit or seed manifests authored by newer nodes, they preserve raw manifest JSON and signatures intact.

---

## 2. Priority 0: Smart Relay Orchestration, Dormant Bootstrap & 2-Hop Mesh Routing

> **Target Branch**: `feature/smart-relay-orchestration`

### 2.1 Current Bottlenecks in Existing Code (Root-Cause Analysis)
1. **Unconditional Relay Reservation on Every Peer (`cmd/resonamesh/main.go`, lines 316–325 & 374–381)**:
   - Currently, inside `p2pHost.Network().Notify(&network.NotifyBundle{ ConnectedF: ... })` and `StartBootstrapLoop`, every client automatically calls `p2pHost.ReserveRelay(resCtx, remotePeer)` on **every** connected peer and bootstrap server, regardless of how many reservations the client already holds.
2. **Continuous Bootstrap Polling (`internal/p2p/bootstrap.go`, lines 244–278)**:
   - `StartBootstrapLoop` re-resolves and dials `BootstrapSeeds` (e.g., the primary Render relay) every `2 * time.Minute` even when the client already has sufficient active P2P swarm connections (`len(h.ConnectedPeers()) >= 3`).
3. **Unbounded Periodic PEX Polling (`cmd/resonamesh/main.go`, lines 401–419 & `internal/p2p/pex.go`, lines 131–158)**:
   - Every 15 seconds, the node queries `pexHandler.QueryPeers` on **all** connected peers, and `QueryPeers` immediately auto-dials every returned peer without checking a maximum connection ceiling (`MaxSwarmPeers`).
4. **Binary Relay Configuration (`internal/p2p/host.go`, lines 141–147)**:
   - `circuitrelay.New(rawHost, circuitrelay.WithInfiniteLimits())` is only enabled when `cfg.EnableRelayService` is `true` (currently restricted to `cfg.Headless` in `cmd/resonamesh/main.go`, line 190). Regular desktop nodes with public IP addresses cannot act as lightweight NAT hole-punching coordinators (`Tier 2`) or opt-in bounded audio transit relays (`Tier 3`).

---

### 2.2 Architectural Solution Overview

#### A. Three-Tier Node Contribution Model
Every ResonaMesh node operates in one of three relay contribution modes (`Config.RelayMode`):
1. **Tier 1: `pex-only` (Universal Default for NAT-Restricted Nodes)**
   - Participates in `/resonamesh/pex/1.0.0` gossip (`PeerExchange`).
   - Does not run a Circuit Relay v2 server (`circuitrelay.New` is not started).
2. **Tier 2: `coordinator` (Default for Publicly Reachable Desktop Nodes)**
   - Automatically activates when `event.EvtLocalReachabilityChanged` reports `network.ReachabilityPublic` or STUN diagnosis (`ProbeAndAdvertiseNAT`) detects a public IP / Full Cone NAT, unless explicitly disabled via `--disable-relay` or `--relay-mode=pex-only`.
   - Runs Circuit Relay v2 strictly constrained to **signaling and DCUtR hole-punch coordination** (zero heavy audio transit):
     - `MaxReservations`: `16` (configurable via `RelayMaxReservations`).
     - `MaxCircuits`: `4` per peer.
     - `RelayLimit.Data`: `256 * 1024` bytes (`256 KiB` — enough for Identify, PEX, CatalogSync, and DCUtR coordination, automatically rejecting multi-megabyte audio block streams).
     - `RelayLimit.Duration`: `2 * time.Minute`.
3. **Tier 3: `transit` (Opt-In High-Capacity Community Relay & Headless Seeds)**
   - Enabled when `--relay-mode=transit` or `cfg.Headless == true`.
   - Runs Circuit Relay v2 permitting fallback audio block streaming (`/resonamesh/block/1.0.0`) when DCUtR hole punching fails between two Symmetric NAT peers:
     - `MaxReservations`: `64` (configurable via `RelayMaxReservations`).
     - `MaxCircuits`: `16` per peer.
     - `RelayLimit.Data`: `16 * 1024 * 1024` bytes (`16 MiB` per circuit, configurable via `RelayDataLimitBytes`; `0` = unlimited for dedicated cloud seeds if desired).
     - `RelayLimit.Duration`: `10 * time.Minute`.

#### B. Dormant Bootstrap Policy ("Render as Last-Resort Spark")
- The primary bootstrap seed (`BootstrapSeeds` / Render) exists solely to ignite initial swarm entry when a client is isolated.
- **Rule 1 (Dormant Seed Polling)**: In `StartBootstrapLoop` (`internal/p2p/bootstrap.go`), if `len(h.ConnectedPeers()) >= cfg.MinSwarmPeers` (default `3`), the periodic ticker **skips** resolving and dialing `BootstrapSeeds` completely.
- **Rule 2 (Seed Connection Priority)**: Bootstrap seeds are placed at the absolute bottom of relay reservation priority. A client only requests a Circuit Relay slot on a bootstrap seed if it has tried all known community relays (`Tier 2` / `Tier 3`) and has `0` active relay reservations.
- **Rule 3 (Proactive Seed Drop / Dispatcher-First)**: Once a client establishes `>= cfg.MinSwarmPeers` direct or community-relayed connections and holds at least `1` community relay reservation, it does not renew its reservation on the bootstrap seed.

#### C. Protecting Primary & 1st-Hop Relays ("Dispatcher-First" & 50% Soft-Cap Edge Push)
To prevent the primary bootstrap seed and its immediate 1st-ring neighbor relays from saturating first:
1. **Root Seed (`Headless == true`) as Pure Dispatcher**:
   - When a client queries `/resonamesh/pex/1.0.0` with `Type: "find_relay"`, if the root seed knows at least one unvisited downstream relay (not in `req.ExcludeIDs`) and `req.Exhausted == false`, the seed sets `AcceptReserve: false` and returns a **randomly shuffled** subset of downstream relays (`Relays []RelayDescriptor`).
   - The root seed only sets `AcceptReserve: true` when no unvisited downstream relays exist in its table or the client sets `req.Exhausted == true` (all downstream candidates failed).
2. **1st-Hop Community Relays (`50% Soft-Cap Edge Push`)**:
   - Each community relay tracks its current utilization ratio:
     $$\text{LoadPct} = \left\lfloor \frac{\text{ActiveReservations} \times 100}{\text{MaxReservations}} \right\rfloor$$
   - **Under 50% Load (`LoadPct < 50`)**: The relay immediately sets `AcceptReserve: true` so the client can reserve a slot without extra network hops.
   - **At or Above 50% Load (`LoadPct >= 50`)**:
     - If the relay knows unvisited downstream relays (relays in its `RelayRegistry` whose `ID` is not in `req.ExcludeIDs`) and `!req.Exhausted`, it enters **Edge Push mode**: it sets `AcceptReserve: false` and returns up to `4` randomly shuffled unvisited downstream relays.
     - If **all** known relays of this node are already in `req.ExcludeIDs` (meaning the client reached the end of a branch or cycle) or `req.Exhausted == true`, the relay unlocks its **last-50% reserve capacity** and returns `AcceptReserve: true` (as long as `ActiveReservations < MaxReservations`).

#### D. Graph Cycles (Loops): Preserve Mesh Cycles, Prevent Client Loops via `ExcludeIDs`
- **Why Relay Graph Cycles Must NEVER Be Severed**:
  - P2P nodes are volatile; home relays can disconnect at any moment. If relays deleted links upon detecting a cycle (`R1 -> R2 -> R3 -> R1`), the relay topology would degenerate into a brittle **Tree**. In a tree, a single relay disconnect partitions the network. Keeping cyclic links (a **Mesh** topology) guarantees multiple redundant paths around any dead relay.
- **How Clients Avoid Walking in Circles (`visited` Set + `ExcludeIDs`)**:
  - During a relay discovery walk, the client maintains a local per-session map: `visited := map[peer.ID]bool{selfID: true}`.
  - Whenever the client queries a relay `R_current`, it sends `ExcludeIDs: keys(visited)` in `pexRequest`.
  - `R_current` filters its known `primaryRelays` and `backupRelays`, stripping out any relay ID present in `req.ExcludeIDs` before selecting referrals.
  - Consequently, even if `R1 <-> R2 <-> R3 <-> R1` forms a tight triangle, a client starting at `R1` visits `R2` (with `ExcludeIDs: [R1]`), `R2` refers it only to `R3`, and at `R3` (with `ExcludeIDs: [R1, R2]`), `R3` sees that `R1` and `R2` are already visited. `R3` either refers the client to a new branch `R4` or, if no `R4` exists, allocates one of its own reserve slots (`AcceptReserve: true`).
  - If `R3` is 100% full (`ActiveReservations == MaxReservations`) and has no unvisited branches, it returns `AcceptReserve: false, Relays: []`. The client simply pops the next unvisited candidate from its local BFS/DFS queue (backtracking) without ever querying the same relay twice.

#### E. 2-Hop Neighborhood Memory (`NextHopIDs`) for Instant Zero-Downtime Failover
- **How a Relay Learns About Other Relays**:
  1. From incoming and outgoing PEX exchanges (`pexResponse.Relays`).
  2. From connected peers whose multiaddrs are confirmed public and which advertise relay capability (`Tier 2` or `Tier 3`).
- **Two-Level Bounded Memory Table (`RelayRegistry`)**:
  Each node maintains a bounded in-memory `RelayRegistry` (consuming `< 16 KiB` RAM):
  1. **`primaryRelays` (1-Hop Relays, max `16` entries)**:
     - Relays directly connected or recently verified by this node.
     - Each `RelayDescriptor` stores `NextHopIDs []peer.ID` (up to `4` relay IDs that the 1-hop relay is connected to) and `NextHopAddrs map[peer.ID][]string`.
  2. **`backupRelays` (2-Hop Relays, max `32` entries)**:
     - Relays standing one hop behind the node's `primaryRelays`.
- **Automatic Promotion on Relay Disconnect (Zero-Downtime Healing)**:
  - When `primaryRelay` `R2` disconnects (`DisconnectedF` in `cmd/resonamesh/main.go`) or fails a dial/stream attempt (`RecordRelayFailure(R2)`):
    1. `RelayRegistry.HandlePeerDisconnected(R2)` looks up `R2.NextHopIDs` (e.g., `[R5, R6]`).
    2. It immediately promotes `R5` and `R6` from `backupRelays` into `primaryRelays` and schedules a background lightweight PEX handshake to verify them.
    3. Any client asking for a referral 1 millisecond after `R2` dies is seamlessly routed to `R5` or `R6`.
- **Zero-Overhead Health Tracking (Passive Eviction + Lazy Refresh)**:
  - Relays **never** ping each other in high-frequency background loops.
  - Liveness is tracked passively via:
    1. Libp2p connection state (`ConnectedF` / `DisconnectedF`).
    2. Dial outcome feedback during regular `15s–60s` PEX exchanges (`FailureCount`; evicted after `2` consecutive failures or `StaleTTL = 15 * time.Minute` without activity).

---

### 2.3 Exact Step-by-Step Code Implementation Guide (For Priority 0)

#### Step 1: Update `internal/config/config.go` & `.env.example`
1. Add the following constants and fields to `Config` in `internal/config/config.go`:
   ```go
   const (
       RelayModePEXOnly     = "pex-only"
       RelayModeCoordinator = "coordinator"
       RelayModeTransit     = "transit"
   )
   ```
   Add to `type Config struct`:
   ```go
   RelayMode            string
   RelayMaxReservations int
   RelayMaxCircuits     int
   RelayDataLimitBytes  int64
   RelayDurationSec     int
   MinSwarmPeers        int
   MaxSwarmPeers        int
   ```
2. Set defaults in `DefaultConfig()`:
   ```go
   RelayMode:            RelayModeCoordinator,
   RelayMaxReservations: 16,
   RelayMaxCircuits:     4,
   RelayDataLimitBytes:  256 * 1024, // 256 KiB signaling limit for coordinator mode
   RelayDurationSec:     120,        // 2 minutes
   MinSwarmPeers:        3,
   MaxSwarmPeers:        32,
   ```
3. Parse environment variables in `Load()`:
   - `RESONAMESH_RELAY_MODE` (`pex-only`, `coordinator`, `transit`)
   - `RESONAMESH_RELAY_MAX_RESERVATIONS` (`int`)
   - `RESONAMESH_RELAY_DATA_LIMIT_BYTES` (`int64`)
   - `RESONAMESH_MIN_SWARM_PEERS` (`int`)
   - `RESONAMESH_MAX_SWARM_PEERS` (`int`)
4. Parse CLI flags in `Load()`:
   - `--relay-mode`
   - `--relay-max-reservations`
   - `--relay-data-limit`
   - `--min-swarm-peers`
   - `--max-swarm-peers`
5. In `Validate()`:
   - Validate `c.RelayMode` is one of `"pex-only"`, `"coordinator"`, `"transit"`.
   - Ensure `c.MinSwarmPeers >= 1` and `c.MaxSwarmPeers >= c.MinSwarmPeers`.
6. Document all new variables in `.env.example`.

#### Step 2: Update `internal/p2p/host.go` for Tiered Circuit Relay v2 Limits
1. Extend `HostConfig` in `internal/p2p/host.go`:
   ```go
   type HostConfig struct {
       ListenAddr           string
       ListenAddrs          []string
       PrivateKey           ed25519.PrivateKey
       DisableRelay         bool
       EnableRelayService   bool
       RelayMode            string
       RelayMaxReservations int
       RelayMaxCircuits     int
       RelayDataLimitBytes  int64
       RelayDurationSec     int
       EnableNATPortMap     bool
       HTTPHandler          http.Handler
   }
   ```
2. Add helper method ` EnableDynamicRelayService(cfg HostConfig) error` on `*Host` (protected by `sync.Mutex` so it is idempotent):
   - If `cfg.DisableRelay` or `cfg.RelayMode == "pex-only"`, do not start `circuitrelay.New`.
   - Build `circuitrelay.Resources`:
     ```go
     rc := circuitrelay.DefaultResources()
     if cfg.RelayMaxReservations > 0 {
         rc.MaxReservations = cfg.RelayMaxReservations
     }
     if cfg.RelayMaxCircuits > 0 {
         rc.MaxCircuits = cfg.RelayMaxCircuits
     }
     ```
   - Build `circuitrelay.Option` slice:
     - If `cfg.RelayMode == "transit"` and `cfg.RelayDataLimitBytes <= 0`: use `circuitrelay.WithResources(rc)` with `circuitrelay.WithInfiniteLimits()`.
     - Otherwise construct `limit := &circuitrelay.RelayLimit{ Data: cfg.RelayDataLimitBytes, Duration: time.Duration(cfg.RelayDurationSec) * time.Second }` and call `circuitrelay.New(h.raw, circuitrelay.WithResources(rc), circuitrelay.WithLimit(limit))`.
   - If `cfg.EnableRelayService` is true at startup, call `h.EnableDynamicRelayService(cfg)` inside `NewHost`.
   - If `cfg.RelayMode == "coordinator"` or `"transit"` on a non-headless node, call `h.EnableDynamicRelayService(hostConfig)` when STUN (`ProbeAndAdvertiseNAT`) confirms a public IP / Full Cone NAT or when `event.EvtLocalReachabilityChanged` emits `network.ReachabilityPublic`.

#### Step 3: Implement `RelayRegistry`, 2-Hop Backup Promotion & Smart Referral in `internal/p2p/pex.go`
1. Define wire structures in `internal/p2p/pex.go` (100% backward-compatible with existing `pexRequest`/`pexResponse`):
   ```go
   const (
       MaxPrimaryRelays    = 16
       MaxBackupRelays     = 32
       MaxNextHopsPerRelay = 4
       MaxReferralSample   = 4
       RelayStaleTTL       = 15 * time.Minute
   )

   type RelayDescriptor struct {
       ID           peer.ID             `json:"id"`
       Addrs        []string            `json:"addrs"`
       Mode         string              `json:"mode"`                     // "coordinator", "transit", "seed"
       LoadPct      int                 `json:"load_pct"`                 // 0..100
       NextHopIDs   []peer.ID           `json:"next_hop_ids,omitempty"`   // 2-hop relay IDs
       NextHopAddrs map[string][]string `json:"next_hop_addrs,omitempty"` // peerID -> multiaddr strings
       LastSeen     int64               `json:"last_seen"`
       Failures     int                 `json:"-"`
   }

   type pexRequest struct {
       Type       string           `json:"type"`                  // "get_peers" or "find_relay"
       ExcludeIDs []peer.ID        `json:"exclude_ids,omitempty"` // Visited relays in current client search
       Exhausted  bool             `json:"exhausted,omitempty"`   // Client exhausted all downstream referrals
       SelfRelay  *RelayDescriptor `json:"self_relay,omitempty"`  // Optional announcement if caller is an active relay
   }

   type pexResponse struct {
       Peers         []peer.AddrInfo   `json:"peers"`
       Relays        []RelayDescriptor `json:"relays,omitempty"`
       AcceptReserve bool              `json:"accept_reserve,omitempty"`
   }
   ```
2. Add `RelayRegistry` state to `PeerExchange`:
   - Track `primaryRelays map[peer.ID]*RelayDescriptor` (capped at `MaxPrimaryRelays = 16`) and `backupRelays map[peer.ID]*RelayDescriptor` (capped at `MaxBackupRelays = 32`), guarded by `sync.RWMutex`.
   - Track local relay status: `localRelayMode string`, `isSeedNode bool`, `maxReservations int`, `activeReservations atomic.Int32`, `maxSwarmPeers int`.
3. Implement **2-Hop Failover (`OnPeerDisconnected(id peer.ID)`)**:
   - When `id` is present in `primaryRelays`:
     1. Read its `NextHopIDs` and `NextHopAddrs`.
     2. Delete `id` from `primaryRelays`.
     3. For each `hopID` in `NextHopIDs`: if `hopID != pe.host.ID()` and `hopID` is not already in `primaryRelays` (and `len(primaryRelays) < MaxPrimaryRelays`), promote `hopID` from `backupRelays` (or construct from `NextHopAddrs`) into `primaryRelays`.
4. Implement **Dispatcher-First & 50% Soft-Cap (`shouldAcceptReservation(req pexRequest, unvisitedRelays []RelayDescriptor) bool`)**:
   - If `pe.disableRelay` or `pe.localRelayMode == "pex-only"`: return `false`.
   - Current load percentage: `loadPct := int(pe.activeReservations.Load()) * 100 / max(1, pe.maxReservations)`.
   - If `loadPct >= 100`: return `false`.
   - If `pe.isSeedNode`:
     - Return `len(unvisitedRelays) == 0 || req.Exhausted` (Root seed never takes reservations while unvisited downstream relays exist, unless the client exhausted them all).
   - For regular relays (`"coordinator"` or `"transit"`):
     - If `loadPct < 50`: return `true`.
     - If `len(unvisitedRelays) > 0 && !req.Exhausted`: return `false` (Edge Push: redirect deeper into the swarm).
     - Return `true` (Last-resort allocation from the remaining 50% reserve when all downstream paths in the cycle/branch were already visited).
5. Implement **Client-Side Cycle-Free Relay Search (`FindAndReserveRelay(ctx context.Context, seedIDs []peer.ID) (peer.ID, error)`)**:
   - Maintain `visited := map[peer.ID]bool{pe.host.ID(): true}` and a candidate queue `queue []RelayDescriptor`.
   - Seed `queue` with known non-seed `primaryRelays` first, `backupRelays` second, and `seedIDs` (Render) **last**.
   - Pop candidate `cand` from `queue`:
     - If `visited[cand.ID]`, skip.
     - Mark `visited[cand.ID] = true`.
     - Send `pexRequest{Type: "find_relay", ExcludeIDs: mapKeys(visited)}` to `cand.ID`.
     - Append any returned `resp.Relays` (not in `visited`) to the **front** of `queue` (so community referrals are tried before falling back to `seedIDs` at the back of the queue).
     - If `resp.AcceptReserve == true`: call `pe.host.ReserveRelay(ctx, cand.ID)` and return `cand.ID, nil`.
   - If the entire `queue` is exhausted without securing a reservation, make one final pass with `Exhausted: true` on the last reachable relay or seed so it unlocks its emergency reserve slot.

#### Step 4: Enforce Dormant Bootstrap & Peer Caps in `internal/p2p/bootstrap.go` & `cmd/resonamesh/main.go`
1. In `internal/p2p/bootstrap.go`:
   - Add `minSwarmPeers int` parameter (or check via callback/config) in `StartBootstrapLoop`.
   - Inside the periodic `for` loop in `StartBootstrapLoop`:
     ```go
     if len(h.ConnectedPeers()) >= minSwarmPeers {
         slog.Debug("Swarm peer threshold satisfied, keeping bootstrap seed dormant",
             "connected_peers", len(h.ConnectedPeers()),
             "min_swarm_peers", minSwarmPeers,
         )
         continue
     }
     ```
2. In `internal/p2p/pex.go` (`QueryPeers`):
   - Check `if pe.maxSwarmPeers > 0 && len(pe.host.ConnectedPeers()) >= pe.maxSwarmPeers` before auto-dialing discovered peers. Even when the connection cap is reached, still ingest `resp.Relays` into `RelayRegistry` so 1-hop and 2-hop relay tables stay fresh without opening extra TCP/QUIC connections.
3. In `cmd/resonamesh/main.go`:
   - Remove the unconditional `p2pHost.ReserveRelay` goroutines from `ConnectedF` and `StartBootstrapLoop`.
   - Replace them with `pexHandler.StartRelayOrchestrator(ctx, bootstrapPeerIDs)` which maintains 1–2 active relay reservations using `FindAndReserveRelay` and calls `pexHandler.OnPeerDisconnected(remotePeer)` inside `DisconnectedF`.

#### Step 5: Mandatory Unit & Simulation Tests for Priority 0 (`internal/p2p/relay_orchestration_test.go`)
Write deterministic in-memory tests covering:
1. `TestSeedDispatcherFirst`: Verify a headless seed with 1 known community relay returns `AcceptReserve: false` and refers the client to the community relay; verify it returns `AcceptReserve: true` when `Exhausted: true`.
2. `TestRelaySoftCapEdgePush`: Verify a relay below 50% load returns `AcceptReserve: true`, and at `>= 50%` load returns `AcceptReserve: false` with shuffled unvisited referrals.
3. `TestCyclicRelayGraphNoInfiniteLoop`: Construct a 3-relay cycle (`R1 <-> R2 <-> R3 <-> R1`), all at 60% load. Verify a client walking `R1 -> R2 -> R3` passes `ExcludeIDs: [Client, R1, R2]` to `R3`, causing `R3` to see zero unvisited referrals and immediately grant a reserve slot (`AcceptReserve: true`) without looping back to `R1`, while `R1, R2, R3` preserve their cyclic mesh links.
4. `TestTwoHopFailoverPromotion`: Connect `R1` to `R2` where `R2` advertises `NextHopIDs: [R3]`. Simulate `R2` disconnecting (`OnPeerDisconnected(R2)`) and assert `R3` is immediately promoted into `R1`'s `primaryRelays`.
5. `TestDormantBootstrapSkipsSeedWhenSwarmHealthy`: Verify `StartBootstrapLoop` does not contact the HTTP/WS bootstrap seed when `ConnectedPeers() >= MinSwarmPeers`.

---

## 3. Priority 1: Milestone 12 — Embedded Synchronized Lyrics & Karaoke Mode

> **Status**: Implemented & Verified in `feature/synced-lyrics`, merged to `develop`

> **Target Branch**: `feature/synced-lyrics`

### 3.1 Problem Statement
Modern music platforms (such as YouTube Music) provide real-time synchronized lyrics that auto-scroll with playback and allow clicking any lyric line to seek directly to that timestamp. In ResonaMesh, lyrics must work offline and travel with the track metadata or local catalog without external API dependencies.

### 3.2 Exact Code Implementation Specification

#### Step 1: Protocol & Storage Schema (`pkg/protocol/types.go`, `internal/storage/migrations.go`, `internal/catalog/catalog.go`)
1. In `pkg/protocol/types.go`:
   - Add optional field to `Manifest`:
     ```go
     Lyrics string `json:"lyrics,omitempty"` // Optional raw LRC or plain text lyrics
     ```
   - **Backward Compatibility Note**: In `pkg/protocol/payloads.go` (`CanonicalManifestPayload`), if `m.Lyrics == ""` (legacy tracks), the canonical signing string remains 100% identical to v0.1.0-alpha. If `m.Lyrics != ""`, include `|lyrics:<sha256_hex>` or store user-editable lyrics in SQLite `tracks.lyrics` so local or publisher lyrics work seamlessly. To guarantee 100% signature compatibility with existing signed manifests, keep `CanonicalManifestPayload` unchanged for v1 and allow `Manifest.Lyrics` as an optional transport metadata field alongside a dedicated SQLite column `lyrics TEXT NOT NULL DEFAULT ''`.
2. In `internal/storage/migrations.go`:
   - Add migration incrementing schema version to add `lyrics TEXT NOT NULL DEFAULT ''` to the `tracks` table and update the FTS5 index or track queries.
3. In `internal/catalog/catalog.go`:
   - Add `Lyrics string `json:"lyrics,omitempty"`` to `TrackRecord`.
   - Update `UpsertTrack`, `GetTrack`, `ListTracks` and add `UpdateTrackLyrics(ctx context.Context, contentID string, lyrics string) error`.
4. In `internal/api/server.go`:
   - Support optional `lyrics` form field in `POST /api/v1/tracks/import`.
   - Add `PUT /api/v1/tracks/{id}/lyrics` accepting `{"lyrics": "[00:12.50] First line..."}` so users can attach or edit `.lrc` lyrics on any track.

#### Step 2: Frontend LRC Parser & Interactive Lyrics View (`web/src/shared/utils/lrcParser.ts` & `web/src/features/player/LyricsDrawer.tsx`)
1. Create `web/src/shared/utils/lrcParser.ts`:
   - Parse standard LRC line timestamps `[mm:ss.xx]` or `[mm:ss.xxx]` and optional enhanced word timestamps `<mm:ss.xx>word`:
     ```ts
     export interface LyricWord {
       timeSec: number;
       text: string;
     }
     export interface LyricLine {
       timeSec: number;
       text: string;
       words?: LyricWord[];
     }
     export interface ParsedLyrics {
       isSynced: boolean;
       lines: LyricLine[];
     }
     ```
   - Binary search helper `findActiveLyricIndex(lines: LyricLine[], currentTimeSec: number): number` returning the index of the active line (`lines[i].timeSec <= currentTimeSec < lines[i+1].timeSec`).
2. Create `web/src/features/player/LyricsDrawer.tsx`:
   - Slide-over / expandable studio drawer styled with `#0b0d10` / `#14171d` glassmorphism.
   - Auto-scrolls the active line to the vertical center using `scrollIntoView({ behavior: 'smooth', block: 'center' })`.
   - Clicking any synced line invokes `onSeek(line.timeSec)`, jumping playback immediately to that lyric line.
   - Includes an inline "Edit / Paste .LRC" modal/mode calling `PUT /api/v1/tracks/{id}/lyrics` so users can paste or upload `.lrc` files directly in the player.
3. Wire a `Mic2` / `AlignLeft` Lucide toggle button into `web/src/features/player/AudioPlayerBar.tsx`.

---

## 4. Priority 2: Milestone 14 — Studio Audio Suite (5-Band EQ, Speed, Sleep Timer & Fullscreen Mode)

> **Status**: Implemented & Verified in `feature/studio-audio-suite`, merged to `develop`
> **Target Branch**: `feature/studio-audio-suite`

### 4.1 Exact Code Implementation Specification

#### Step 1: Web Audio API 5-Band Parametric Equalizer (`web/src/features/player/useAudioEqualizer.ts`)
1. Attach a singleton `AudioContext` and `MediaElementAudioSourceNode` to the `<audio>` element inside `AudioPlayerBar.tsx` (using a WeakMap or ref guard so `createMediaElementSource` is called at most once per `<audio>` DOM element).
2. Create 5 cascaded `BiquadFilterNode` stages connected in series (`source -> band60 -> band250 -> band1k -> band4k -> band12k -> destination`):
   - `60 Hz`: `type = "lowshelf"`
   - `250 Hz`: `type = "peaking"`, `Q = 1.0`
   - `1000 Hz`: `type = "peaking"`, `Q = 1.0`
   - `4000 Hz`: `type = "peaking"`, `Q = 1.0`
   - `12000 Hz`: `type = "highshelf"`
3. Each band gain is adjustable from `-12 dB` to `+12 dB`, persisted in `localStorage` under `resonamesh_eq_state`.
4. Built-in presets:
   - `Flat`: `[0, 0, 0, 0, 0]`
   - `Bass Boost`: `[6, 4, 0, 0, 0]`
   - `Vocal Presence`: `[-2, 0, 4, 3, 1]`
   - `Electronic / Club`: `[5, 2, -1, 3, 5]`
   - `Acoustic / Warm`: `[3, 2, 0, 1, 3]`

#### Step 2: Playback Speed & Sleep Timer (`web/src/features/player/AudioPlayerBar.tsx`)
1. **Playback Rate**:
   - Selector supporting `0.5x`, `0.75x`, `1.0x`, `1.25x`, `1.5x`, `2.0x`.
   - Sets `audioRef.current.playbackRate = rate` and `audioRef.current.preservesPitch = true`.
2. **Sleep Timer**:
   - Options: `Off`, `15 min`, `30 min`, `45 min`, `60 min`, `End of Track`.
   - During the final 10 seconds before timer expiration, smoothly ramps volume down to `0` before pausing playback and restoring the user's nominal volume slider state.

#### Step 3: Fullscreen Studio Now Playing View (`web/src/features/player/FullscreenPlayerModal.tsx`)
1. Triggered via `Maximize2` button in `AudioPlayerBar.tsx` (or keyboard shortcut `F`).
2. Split-pane layout:
   - **Left Pane**: Large procedural vinyl/cover art, track title, artist, license badge, real-time Web Audio `AnalyserNode` frequency bar spectrum, and P2P swarm chunk buffer bar.
   - **Right Pane**: Embedded interactive `LyricsDrawer` (if lyrics exist) or Up Next Queue + 5-Band Equalizer rack.

---

## 5. Priority 3: Milestone 15 — Music Metadata Tags & Local Swarm Radio

> **Status**: Implemented & Verified in `feature/swarm-radio`, merged to `develop`
> **Target Branch**: `feature/swarm-radio`

### 5.1 Exact Code Implementation Specification
1. **Database & Manifest Fields (`internal/storage/migrations.go`, `internal/catalog/catalog.go`, `pkg/protocol/types.go`)**:
   - Add optional metadata fields to `Manifest` and `TrackRecord`:
     - `Genre string `json:"genre,omitempty"``
     - `Tags []string `json:"tags,omitempty"`` (stored as JSON array string in SQLite `tracks.tags_json`)
     - `BPM int `json:"bpm,omitempty"``
     - `Mood string `json:"mood,omitempty"``
2. **Local Similarity Engine (`internal/catalog/recommendations.go`)**:
   - Given a seed track $A$ and candidate track $B$ from the local catalog:
     - **Tag & Genre Jaccard Similarity** ($S_{\text{tags}} \in [0, 1]$):
       $$J(T_A, T_B) = \frac{|T_A \cap T_B|}{|T_A \cup T_B|}$$
       where $T_X$ is the normalized lowercase set of `{genre, mood, artist} ∪ tags`.
     - **Tempo Proximity Score** ($S_{\text{bpm}} \in [0, 1]$):
       If both tracks have `BPM > 0`:
       $$S_{\text{bpm}} = \max\left(0, 1 - \frac{|\text{BPM}_A - \text{BPM}_B|}{60}\right)$$
       (accounting for double/half time by also testing $2 \times \text{BPM}$ and $0.5 \times \text{BPM}$).
     - **Composite Score**:
       $$\text{Score}(A, B) = 0.75 \cdot J(T_A, T_B) + 0.25 \cdot S_{\text{bpm}}$$
3. **API & Auto-Queue (`internal/api/server.go` & `web/src/App.tsx`)**:
   - Expose `GET /api/v1/tracks/{id}/recommendations?limit=10`.
   - Add a "Swarm Radio" toggle button (`Radio` Lucide icon) in `AudioPlayerBar.tsx`. When active and playback finishes the current track, automatically fetch the top unplayed recommendation and start streaming it.

---

## 6. Priority 4: Milestone 13 — Decentralized Swarm Reputation & Peer Quarantine

> **Status**: Implemented & Verified in `feature/swarm-reputation`, merged to `develop`
> **Target Branch**: `feature/swarm-reputation`

### 6.1 Exact Code Implementation Specification
1. **Local Reputation Registry (`internal/p2p/reputation.go`)**:
   - Create `PeerReputationTracker` with thread-safe per-peer stats (`map[peer.ID]*PeerTransferStats`):
     - `BlocksVerified int64`
     - `HashMismatchCount int64`
     - `StreamErrors int64`
     - `AvgLatencyMs float64` (exponential moving average $\alpha = 0.2$)
     - `QuarantinedUntil time.Time`
   - **Scoring Formula**:
     $$\text{Score}(p) = 100 + \min(50, 2 \cdot \text{BlocksVerified}) - 15 \cdot \text{StreamErrors} - 100 \cdot \text{HashMismatchCount} - \frac{\text{AvgLatencyMs}}{50}$$
   - **Quarantine Rule**:
     - Any SHA-256 checksum mismatch in `RequestBlock` (`internal/p2p/protocol.go`) immediately quarantines that `peer.ID` for `10 * time.Minute` (doubling on repeat offenses).
     - Three consecutive stream timeouts quarantine the peer for `1 * time.Minute`.
2. **Integration into `Fetcher` (`internal/p2p/fetcher.go`)**:
   - In `Fetcher.prioritizedSwarmPeers()`:
     - Filter out any `peer.ID` where `reputation.IsQuarantined(pid, time.Now()) == true`.
     - Within each connection tier (`directP2P`, `relayedP2P`, `wsRelays`), sort candidate peers in descending order of `reputation.Score(pid)`.
   - In `Fetcher.FetchBlock()`:
     - Measure round-trip duration of `RequestBlock`. On success, call `reputation.RecordSuccess(peerID, elapsed, len(data))`. On hash mismatch or timeout, call `reputation.RecordFailure(peerID, isHashMismatch)`.
