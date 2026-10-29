package p2p

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"resonamesh/pkg/crypto"

	libp2pcrypto "github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/peer"
)

func generateTestPeerID(t *testing.T) peer.ID {
	t.Helper()
	pub, _, err := crypto.GenerateKeyPair()
	if err != nil {
		t.Fatal(err)
	}
	lp2pPub, err := libp2pcrypto.UnmarshalEd25519PublicKey(pub)
	if err != nil {
		t.Fatal(err)
	}
	pid, err := peer.IDFromPublicKey(lp2pPub)
	if err != nil {
		t.Fatal(err)
	}
	return pid
}

func TestSeedDispatcherFirst(t *testing.T) {
	_, privSeed, _ := crypto.GenerateKeyPair()
	_, privClient, _ := crypto.GenerateKeyPair()

	seedHost, err := NewHost(HostConfig{
		ListenAddr:   "/ip4/127.0.0.1/tcp/0",
		PrivateKey:   privSeed,
		DisableRelay: false,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer seedHost.Close()

	seedPEX := NewPeerExchange(seedHost, nil)
	seedPEX.SetRelayConfig("transit", true, 16, 3, 32) // isSeedNode = true

	// Register a known downstream community relay on the seed
	dummyRelayID := generateTestPeerID(t)
	seedPEX.RegisterRelay(RelayDescriptor{
		ID:       dummyRelayID,
		Addrs:    []string{"/ip4/198.51.100.1/tcp/4001"},
		Mode:     "coordinator",
		LoadPct:  20,
		LastSeen: time.Now().Unix(),
	})

	clientHost, err := NewHost(HostConfig{
		ListenAddr:   "/ip4/127.0.0.1/tcp/0",
		PrivateKey:   privClient,
		DisableRelay: false,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer clientHost.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := clientHost.Connect(ctx, seedHost.AddrInfo()); err != nil {
		t.Fatalf("failed to connect client to seed: %v", err)
	}

	// 1. Client queries seed: because seed knows downstream relay, it should reject reservation (Dispatcher-First)
	stream1, err := clientHost.NewControlStream(ctx, seedHost.ID(), PEXProtocolID)
	if err != nil {
		t.Fatal(err)
	}
	req1 := pexRequest{
		Type:       "find_relay",
		ExcludeIDs: []peer.ID{clientHost.ID()},
		Exhausted:  false,
	}
	if err := json.NewEncoder(stream1).Encode(req1); err != nil {
		t.Fatal(err)
	}
	var resp1 pexResponse
	if err := json.NewDecoder(stream1).Decode(&resp1); err != nil {
		t.Fatal(err)
	}
	stream1.Close()

	if resp1.AcceptReserve {
		t.Fatalf("expected seed node to reject reservation (Dispatcher-First), but AcceptReserve was true")
	}
	if len(resp1.Relays) == 0 || resp1.Relays[0].ID != dummyRelayID {
		t.Fatalf("expected seed to refer downstream relay %s, got: %+v", dummyRelayID, resp1.Relays)
	}

	// 2. Client queries seed with Exhausted: true (downstream candidates were unreachable)
	stream2, err := clientHost.NewControlStream(ctx, seedHost.ID(), PEXProtocolID)
	if err != nil {
		t.Fatal(err)
	}
	req2 := pexRequest{
		Type:       "find_relay",
		ExcludeIDs: []peer.ID{clientHost.ID(), dummyRelayID},
		Exhausted:  true,
	}
	if err := json.NewEncoder(stream2).Encode(req2); err != nil {
		t.Fatal(err)
	}
	var resp2 pexResponse
	if err := json.NewDecoder(stream2).Decode(&resp2); err != nil {
		t.Fatal(err)
	}
	stream2.Close()

	if !resp2.AcceptReserve {
		t.Fatalf("expected seed node to accept reservation when client Exhausted=true, but AcceptReserve was false")
	}
}

func TestRelaySoftCapEdgePush(t *testing.T) {
	_, privRelay, _ := crypto.GenerateKeyPair()
	_, privClient, _ := crypto.GenerateKeyPair()

	relayHost, err := NewHost(HostConfig{
		ListenAddr:   "/ip4/127.0.0.1/tcp/0",
		PrivateKey:   privRelay,
		DisableRelay: false,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer relayHost.Close()

	relayPEX := NewPeerExchange(relayHost, nil)
	relayPEX.SetRelayConfig("coordinator", false, 10, 3, 32) // maxReservations = 10

	downstreamID := generateTestPeerID(t)
	relayPEX.RegisterRelay(RelayDescriptor{
		ID:       downstreamID,
		Addrs:    []string{"/ip4/203.0.113.1/tcp/4001"},
		Mode:     "coordinator",
		LoadPct:  10,
		LastSeen: time.Now().Unix(),
	})

	clientHost, err := NewHost(HostConfig{
		ListenAddr:   "/ip4/127.0.0.1/tcp/0",
		PrivateKey:   privClient,
		DisableRelay: false,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer clientHost.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := clientHost.Connect(ctx, relayHost.AddrInfo()); err != nil {
		t.Fatal(err)
	}

	// Case 1: Relay under 50% load (active = 3 / 10 = 30%) -> accepts reservation immediately
	relayPEX.SetActiveReservations(3)
	s1, err := clientHost.NewControlStream(ctx, relayHost.ID(), PEXProtocolID)
	if err != nil {
		t.Fatal(err)
	}
	_ = json.NewEncoder(s1).Encode(pexRequest{Type: "find_relay"})
	var r1 pexResponse
	_ = json.NewDecoder(s1).Decode(&r1)
	s1.Close()

	if !r1.AcceptReserve {
		t.Fatalf("expected relay under 50%% load to accept reservation, got AcceptReserve=false")
	}

	// Case 2: Relay at >= 50% load (active = 6 / 10 = 60%) with unvisited downstream relays -> Edge Push redirect
	relayPEX.SetActiveReservations(6)
	s2, err := clientHost.NewControlStream(ctx, relayHost.ID(), PEXProtocolID)
	if err != nil {
		t.Fatal(err)
	}
	_ = json.NewEncoder(s2).Encode(pexRequest{
		Type:       "find_relay",
		ExcludeIDs: []peer.ID{clientHost.ID()},
		Exhausted:  false,
	})
	var r2 pexResponse
	_ = json.NewDecoder(s2).Decode(&r2)
	s2.Close()

	if r2.AcceptReserve {
		t.Fatalf("expected relay at 60%% load to redirect client (Edge Push), but AcceptReserve was true")
	}
	if len(r2.Relays) == 0 || r2.Relays[0].ID != downstreamID {
		t.Fatalf("expected referral to downstream relay %s, got: %+v", downstreamID, r2.Relays)
	}

	// Case 3: Relay at 60% load, but client has already visited all downstream relays (ExcludeIDs includes downstream)
	s3, err := clientHost.NewControlStream(ctx, relayHost.ID(), PEXProtocolID)
	if err != nil {
		t.Fatal(err)
	}
	_ = json.NewEncoder(s3).Encode(pexRequest{
		Type:       "find_relay",
		ExcludeIDs: []peer.ID{clientHost.ID(), downstreamID},
		Exhausted:  false,
	})
	var r3 pexResponse
	_ = json.NewDecoder(s3).Decode(&r3)
	s3.Close()

	if !r3.AcceptReserve {
		t.Fatalf("expected relay to unlock emergency reserve when downstream is exhausted, got AcceptReserve=false")
	}
}

func TestCyclicRelayGraphNoInfiniteLoop(t *testing.T) {
	// Simulate 3 relays forming a cycle: R1 <-> R2 <-> R3 <-> R1
	r1ID := generateTestPeerID(t)
	r2ID := generateTestPeerID(t)

	_, privR3, _ := crypto.GenerateKeyPair()
	r3Host, err := NewHost(HostConfig{
		ListenAddr:   "/ip4/127.0.0.1/tcp/0",
		PrivateKey:   privR3,
		DisableRelay: false,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer r3Host.Close()

	r3PEX := NewPeerExchange(r3Host, nil)
	r3PEX.SetRelayConfig("coordinator", false, 10, 3, 32)
	r3PEX.SetActiveReservations(6) // 60% load

	// R3 knows R1 (forming the cycle back to start)
	r3PEX.RegisterRelay(RelayDescriptor{
		ID:       r1ID,
		Addrs:    []string{"/ip4/192.0.2.1/tcp/4001"},
		Mode:     "coordinator",
		LoadPct:  60,
		LastSeen: time.Now().Unix(),
	})

	_, privClient, _ := crypto.GenerateKeyPair()
	clientHost, err := NewHost(HostConfig{
		ListenAddr:   "/ip4/127.0.0.1/tcp/0",
		PrivateKey:   privClient,
		DisableRelay: false,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer clientHost.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := clientHost.Connect(ctx, r3Host.AddrInfo()); err != nil {
		t.Fatal(err)
	}

	// Client has walked R1 -> R2 -> R3. It passes ExcludeIDs: [Client, R1, R2]
	s, err := clientHost.NewControlStream(ctx, r3Host.ID(), PEXProtocolID)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	req := pexRequest{
		Type:       "find_relay",
		ExcludeIDs: []peer.ID{clientHost.ID(), r1ID, r2ID},
		Exhausted:  false,
	}
	if err := json.NewEncoder(s).Encode(req); err != nil {
		t.Fatal(err)
	}

	var resp pexResponse
	if err := json.NewDecoder(s).Decode(&resp); err != nil {
		t.Fatal(err)
	}

	// Even though R3 is at 60% load, its only referral (R1) is in ExcludeIDs.
	// Therefore, instead of looping back to R1, R3 terminates the cycle and grants AcceptReserve=true!
	if !resp.AcceptReserve {
		t.Fatalf("expected R3 to prevent infinite cycle and accept reservation from reserve slots, got false")
	}
	// And verify R3 still kept R1 in its internal table (cycles preserved in mesh graph)
	if len(r3PEX.PrimaryRelays()) == 0 {
		t.Fatalf("R3 should preserve cyclic graph link to R1, but primary relays was empty")
	}
}

func TestTwoHopFailoverPromotion(t *testing.T) {
	_, privA, _ := crypto.GenerateKeyPair()
	hostA, err := NewHost(HostConfig{
		ListenAddr:   "/ip4/127.0.0.1/tcp/0",
		PrivateKey:   privA,
		DisableRelay: false,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer hostA.Close()

	pexA := NewPeerExchange(hostA, nil)

	relayB := generateTestPeerID(t)
	relayC := generateTestPeerID(t)

	// Register RelayB as a primary relay which advertises RelayC as its NextHop (2-hop neighbor)
	pexA.RegisterRelay(RelayDescriptor{
		ID:         relayB,
		Addrs:      []string{"/ip4/198.51.100.2/tcp/4001"},
		Mode:       "coordinator",
		NextHopIDs: []peer.ID{relayC},
		NextHopAddrs: map[string][]string{
			relayC.String(): {"/ip4/198.51.100.3/tcp/4001"},
		},
		LastSeen: time.Now().Unix(),
	})

	// Check initial state: RelayB in primary, RelayC in backup
	if len(pexA.PrimaryRelays()) != 1 || pexA.PrimaryRelays()[0].ID != relayB {
		t.Fatalf("expected RelayB in primary relays, got: %+v", pexA.PrimaryRelays())
	}
	if len(pexA.BackupRelays()) != 1 || pexA.BackupRelays()[0].ID != relayC {
		t.Fatalf("expected RelayC in backup relays, got: %+v", pexA.BackupRelays())
	}

	// Simulate sudden disconnection of RelayB
	pexA.OnPeerDisconnected(relayB)

	// Assert RelayC is immediately promoted to primary relays
	primaries := pexA.PrimaryRelays()
	if len(primaries) != 1 || primaries[0].ID != relayC {
		t.Fatalf("expected RelayC to be promoted to primary relays after RelayB dropped, got: %+v", primaries)
	}
	if len(primaries[0].Addrs) == 0 || primaries[0].Addrs[0] != "/ip4/198.51.100.3/tcp/4001" {
		t.Fatalf("expected RelayC to retain multiaddresses from 2-hop metadata, got: %+v", primaries[0].Addrs)
	}
}

func TestDormantBootstrapSkipsSeedWhenSwarmHealthy(t *testing.T) {
	var seedHitCount atomic.Int32

	// Mock HTTP bootstrap seed server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seedHitCount.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode([]string{})
	}))
	defer server.Close()

	_, privHost, _ := crypto.GenerateKeyPair()
	h, err := NewHost(HostConfig{
		ListenAddr:   "/ip4/127.0.0.1/tcp/0",
		PrivateKey:   privHost,
		DisableRelay: false,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer h.Close()

	// Connect 3 dummy peers so len(ConnectedPeers()) >= minSwarmPeers
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	for i := 0; i < 3; i++ {
		_, privPeer, _ := crypto.GenerateKeyPair()
		pHost, err := NewHost(HostConfig{
			ListenAddr:   "/ip4/127.0.0.1/tcp/0",
			PrivateKey:   privPeer,
			DisableRelay: false,
		})
		if err != nil {
			t.Fatal(err)
		}
		defer pHost.Close()
		if err := h.Connect(ctx, pHost.AddrInfo()); err != nil {
			t.Fatal(err)
		}
	}

	if len(h.ConnectedPeers()) < 3 {
		t.Fatalf("expected at least 3 connected peers, got %d", len(h.ConnectedPeers()))
	}

	loopCtx, loopCancel := context.WithCancel(context.Background())
	defer loopCancel()

	// Launch StartBootstrapLoop with minSwarmPeers = 3
	StartBootstrapLoop(loopCtx, h, nil, []string{server.URL}, 50*time.Millisecond, 3, nil)

	// Wait for a few ticker intervals
	time.Sleep(200 * time.Millisecond)

	// Because connected peers (3) >= minSwarmPeers (3), the seed server MUST NOT be polled!
	hits := seedHitCount.Load()
	if hits != 0 {
		t.Fatalf("expected 0 HTTP hits on bootstrap seed due to Dormant Bootstrap policy, got: %d", hits)
	}
}
