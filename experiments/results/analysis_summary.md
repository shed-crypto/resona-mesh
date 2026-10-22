# ResonaMesh Research Experiments — Statistical Summary (E1, E2, E3)

> **Evaluation Suite:** G5 Integrated Acceptance & Research Benchmarks  
> **Environment:** In-Memory Loopback P2P Swarm Topology  
> **Methodology:** ≥ 20 independent trials per streaming mode, ≥ 10 trials per failure scenario

---

## 1. Experiment E1: Streaming Delivery Comparison

Comparing three progressive audio transfer strategies:
- **B0 (Baseline Full Download):** Complete track payload must be transferred before playback begins.
- **B1 (Sequential On-Demand):** Chunks requested one-by-one as playback advances without lookahead buffer.
- **B2 (Bounded Prefetch Window):** Foreground chunk requested immediately with up to 4 prefetch chunks pipelined in background.

### Time to First Audio (TTFA in ms)
| Mode | Samples | Min (ms) | Median (ms) | Mean (ms) | p95 (ms) | Max (ms) |
|---|---|---|---|---|---|---|
| **B0 (Full Download)** | 20 | 43.0 | 50.5 | 53.1 | 75.0 | 75.0 |
| **B1 (Sequential Demand)** | 20 | 4.0 | 6.0 | 8.8 | 21.0 | 21.0 |
| **B2 (Bounded Prefetch)** | 20 | 5.0 | 6.0 | 7.9 | 24.0 | 24.0 |

### Playback Stall Duration (ms)
| Mode | Samples | Min (ms) | Median (ms) | Mean (ms) | p95 (ms) | Max (ms) |
|---|---|---|---|---|---|---|
| **B1 (Sequential Demand)** | 20 | 0.0 | 0.0 | 0.4 | 8.0 | 8.0 |
| **B2 (Bounded Prefetch)** | 20 | 0.0 | 0.0 | 0.0 | 0.0 | 0.0 |

**Key Finding:** Bounded prefetching (B2) achieves near-instant TTFA (median 6.0 ms vs 50.5 ms for B0, an improvement of **88.1%%**) while completely eliminating intermediate buffering stalls under standard network conditions.

---

## 2. Experiment E2: Source Failure & Replica Failover

Evaluates swarm playback resilience when the primary audio publisher abruptly disconnects mid-stream:
- **Topology:** Node A (Publisher, drops), Node B (Listener), Node C (Autonomous Keeper Replica).

| Metric | Samples | Min (ms) | Median (ms) | Mean (ms) | p95 (ms) | Max (ms) |
|---|---|---|---|---|---|---|
| **Failover Latency** | 12 | 3.0 | 4.0 | 3.8 | 5.0 | 5.0 |
| **Stall Duration** | 12 | 3.0 | 8.5 | 9.6 | 19.0 | 19.0 |

**Key Finding:** When the primary stream publisher terminates unexpectedly, the swarm fetcher detects peer loss within timeout bounds and transitions seamlessly to Keeper replica Node C with a median failover latency of **4.0 ms**.

---

## 3. Experiment E3: Network Partition & Policy Convergence

Evaluates store-and-forward moderation decision convergence across network partitions:
- **Scenario:** Cluster split into segments {A, B} and {C, D}. Moderator D issues a cryptographic BLOCK decision in segment 2. Network connectivity is subsequently restored.

| Metric | Samples | Min (ms) | Median (ms) | Mean (ms) | p95 (ms) | Max (ms) |
|---|---|---|---|---|---|---|
| **Decision Sync Latency** | 12 | 0.0 | 0.5 | 0.5 | 1.0 | 1.0 |

**Key Finding:** Reconnecting nodes achieve 100%% policy convergence within **0.5 ms** median latency, applying monotonic sequence decisions and immediately revoking unauthorized audio distribution.
