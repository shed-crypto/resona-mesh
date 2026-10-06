package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/libp2p/go-libp2p"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/p2p/discovery/mdns"
)

type discoveryNotifee struct {
	targetPeerID  peer.ID
	discoveredChan chan peer.AddrInfo
}

func (n *discoveryNotifee) HandlePeerFound(pi peer.AddrInfo) {
	if pi.ID == n.targetPeerID {
		select {
		case n.discoveredChan <- pi:
		default:
		}
	}
}

func runMDNSDemo() error {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	const serviceTag = "resonamesh-discovery-test"

	// 1. Create Node A on dynamic loopback port
	nodeA, err := libp2p.New(
		libp2p.ListenAddrStrings("/ip4/127.0.0.1/tcp/0"),
		libp2p.DisableRelay(),
	)
	if err != nil {
		return fmt.Errorf("failed to create node A: %w", err)
	}
	defer nodeA.Close()

	// 2. Create Node B on dynamic loopback port
	nodeB, err := libp2p.New(
		libp2p.ListenAddrStrings("/ip4/127.0.0.1/tcp/0"),
		libp2p.DisableRelay(),
	)
	if err != nil {
		return fmt.Errorf("failed to create node B: %w", err)
	}
	defer nodeB.Close()

	discovered := make(chan peer.AddrInfo, 1)
	notifee := &discoveryNotifee{
		targetPeerID:  nodeA.ID(),
		discoveredChan: discovered,
	}

	// 3. Register mDNS service on Node A (broadcaster)
	serviceA := mdns.NewMdnsService(nodeA, serviceTag, &discoveryNotifee{
		targetPeerID:  nodeB.ID(),
		discoveredChan: make(chan peer.AddrInfo, 1),
	})
	if err := serviceA.Start(); err != nil {
		return fmt.Errorf("failed to start mDNS on node A: %w", err)
	}
	defer serviceA.Close()

	// 4. Register mDNS service on Node B (listener)
	serviceB := mdns.NewMdnsService(nodeB, serviceTag, notifee)
	if err := serviceB.Start(); err != nil {
		return fmt.Errorf("failed to start mDNS on node B: %w", err)
	}
	defer serviceB.Close()

	select {
	case pi := <-discovered:
		fmt.Printf("[PASS] mDNS Peer Discovery successful on loopback!\n")
		fmt.Printf("       Discovered PeerID: %s\n", pi.ID)
		fmt.Printf("       Multiaddresses: %v\n", pi.Addrs)
		return nil
	case <-ctx.Done():
		return fmt.Errorf("mDNS discovery timed out after 15s (may require Windows Firewall UDP 5353 allow rule)")
	}
}

func main() {
	if err := runMDNSDemo(); err != nil {
		log.Fatalf("mDNS demo failed: %v", err)
	}
}
