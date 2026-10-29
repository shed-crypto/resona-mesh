package p2p

import (
	"context"
	"encoding/binary"
	"fmt"
	"net"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"resonamesh/pkg/crypto"
)

func writeMockSTUNResponse(pc net.PacketConn, req []byte, addr net.Addr, mappedIP string, mappedPort uint16) {
	resp := make([]byte, 32)
	binary.BigEndian.PutUint16(resp[0:2], STUNBindingResponse)
	binary.BigEndian.PutUint16(resp[2:4], 12) // Attribute length: 12 bytes
	binary.BigEndian.PutUint32(resp[4:8], STUNMagicCookie)
	copy(resp[8:20], req[8:20]) // Echo transaction ID

	// XOR-MAPPED-ADDRESS (type: 0x0020, len: 8)
	binary.BigEndian.PutUint16(resp[20:22], AttrXORMappedAddress)
	binary.BigEndian.PutUint16(resp[22:24], 8)
	resp[24] = 0x00 // Reserved
	resp[25] = 0x01 // IPv4 family

	xport := mappedPort ^ uint16(STUNMagicCookie>>16)
	binary.BigEndian.PutUint16(resp[26:28], xport)

	expectedIP := net.ParseIP(mappedIP).To4()
	expectedIPUint := binary.BigEndian.Uint32(expectedIP)
	binary.BigEndian.PutUint32(resp[28:32], expectedIPUint^STUNMagicCookie)

	_, _ = pc.WriteTo(resp, addr)
}

func TestSTUNResolverMockServer(t *testing.T) {
	pc, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to start mock UDP listener: %v", err)
	}
	defer pc.Close()

	serverAddr := pc.LocalAddr().String()

	go func() {
		buf := make([]byte, 1024)
		n, addr, err := pc.ReadFrom(buf)
		if err != nil || n < 20 {
			return
		}
		writeMockSTUNResponse(pc, buf[:n], addr, "203.0.113.195", 54321)
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	udpAddr, err := ResolvePublicAddr(ctx, serverAddr)
	if err != nil {
		t.Fatalf("ResolvePublicAddr failed: %v", err)
	}

	if udpAddr.IP.String() != "203.0.113.195" {
		t.Errorf("expected IP 203.0.113.195, got %s", udpAddr.IP.String())
	}
	if udpAddr.Port != 54321 {
		t.Errorf("expected port 54321, got %d", udpAddr.Port)
	}
}

func TestSTUNResolverFallback(t *testing.T) {
	pc, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to start mock UDP listener: %v", err)
	}
	defer pc.Close()

	workingAddr := pc.LocalAddr().String()

	go func() {
		buf := make([]byte, 1024)
		n, addr, err := pc.ReadFrom(buf)
		if err != nil || n < 20 {
			return
		}
		writeMockSTUNResponse(pc, buf[:n], addr, "198.51.100.1", 4001)
	}()

	origServers := DefaultSTUNServers
	defer func() { DefaultSTUNServers = origServers }()

	DefaultSTUNServers = []string{"127.0.0.1:1", workingAddr}

	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	defer cancel()

	udpAddr, err := ResolvePublicAddr(ctx, "")
	if err != nil {
		t.Fatalf("expected fallback to succeed, got error: %v", err)
	}
	if udpAddr.IP.String() != "198.51.100.1" {
		t.Errorf("expected IP 198.51.100.1, got %s", udpAddr.IP.String())
	}
	if udpAddr.Port != 4001 {
		t.Errorf("expected port 4001, got %d", udpAddr.Port)
	}
}

func TestSharedQUICSocketSTUNAndMappedPortAdvertising(t *testing.T) {
	pc, err := net.ListenPacket("udp4", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to start mock STUN listener: %v", err)
	}
	defer pc.Close()

	var observedClientPort atomic.Int32
	const simulatedExternalPort = 61432

	go func() {
		buf := make([]byte, 1024)
		for {
			n, addr, err := pc.ReadFrom(buf)
			if err != nil {
				return
			}
			if n < 20 {
				continue
			}
			if uaddr, ok := addr.(*net.UDPAddr); ok {
				observedClientPort.Store(int32(uaddr.Port))
			}
			writeMockSTUNResponse(pc, buf[:n], addr, "203.0.113.77", simulatedExternalPort)
		}
	}()

	_, priv, err := crypto.GenerateKeyPair()
	if err != nil {
		t.Fatal(err)
	}

	h, err := NewHost(HostConfig{
		ListenAddrs: []string{
			"/ip4/127.0.0.1/tcp/0",
			"/ip4/127.0.0.1/udp/0/quic-v1",
		},
		PrivateKey: priv,
	})
	if err != nil {
		t.Fatalf("failed to create host: %v", err)
	}
	defer h.Close()

	_, boundQUICPort, err := h.SharedUDPPacketConn()
	if err != nil {
		t.Fatalf("expected SharedUDPPacketConn to succeed: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	diag, err := h.ProbeAndAdvertiseNAT(ctx, []string{pc.LocalAddr().String()})
	if err != nil {
		t.Fatalf("ProbeAndAdvertiseNAT failed: %v", err)
	}

	// 1. Verify STUN packet actually originated from the exact UDP port bound by QUIC-v1
	if int(observedClientPort.Load()) != boundQUICPort {
		t.Fatalf("expected STUN packet to originate from bound QUIC UDP port %d, got %d", boundQUICPort, observedClientPort.Load())
	}

	// 2. Verify external NAT-mapped port (61432) is advertised instead of internal port
	if diag.PrimaryPort != simulatedExternalPort {
		t.Fatalf("expected primary mapped port %d, got %d", simulatedExternalPort, diag.PrimaryPort)
	}

	expectedMA := fmt.Sprintf("/ip4/203.0.113.77/udp/%d/quic-v1", simulatedExternalPort)
	found := false
	for _, a := range h.Addrs() {
		if a.String() == expectedMA {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected host.Addrs() to advertise mapped QUIC multiaddr %s, got %v", expectedMA, h.Addrs())
	}
}

func TestDiagnoseNATSymmetricPortPrediction(t *testing.T) {
	pc1, err := net.ListenPacket("udp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer pc1.Close()

	pc2, err := net.ListenPacket("udp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer pc2.Close()

	go func() {
		buf := make([]byte, 1024)
		if n, addr, err := pc1.ReadFrom(buf); err == nil && n >= 20 {
			writeMockSTUNResponse(pc1, buf[:n], addr, "198.51.100.42", 50010)
		}
	}()
	go func() {
		buf := make([]byte, 1024)
		if n, addr, err := pc2.ReadFrom(buf); err == nil && n >= 20 {
			writeMockSTUNResponse(pc2, buf[:n], addr, "198.51.100.42", 50012)
		}
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	diag, err := DiagnoseNAT(ctx, nil, []string{pc1.LocalAddr().String(), pc2.LocalAddr().String()})
	if err != nil {
		t.Fatalf("DiagnoseNAT failed: %v", err)
	}

	if diag.NATType != NATTypeSymmetric {
		t.Fatalf("expected NATTypeSymmetric, got %s", diag.NATType)
	}
	if diag.Delta != 2 {
		t.Fatalf("expected port delta +2, got %d", diag.Delta)
	}

	// Candidate ports should include [50012, 50010, 50014, 50016, 50018]
	portsStr := fmt.Sprintf("%v", diag.CandidatePorts)
	for _, expected := range []string{"50010", "50012", "50014", "50016", "50018"} {
		if !strings.Contains(portsStr, expected) {
			t.Errorf("expected candidate ports %s to contain predicted port %s", portsStr, expected)
		}
	}
}
