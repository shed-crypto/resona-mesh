package p2p

import (
	"context"
	"encoding/binary"
	"net"
	"testing"
	"time"
)

func TestSTUNResolverMockServer(t *testing.T) {
	// Start mock UDP server
	pc, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to start mock UDP listener: %v", err)
	}
	defer pc.Close()

	serverAddr := pc.LocalAddr().String()

	// Server goroutine to answer with RFC 5389 XOR-MAPPED-ADDRESS
	go func() {
		buf := make([]byte, 1024)
		n, addr, err := pc.ReadFrom(buf)
		if err != nil || n < 20 {
			return
		}

		resp := make([]byte, 32)
		binary.BigEndian.PutUint16(resp[0:2], STUNBindingResponse)
		binary.BigEndian.PutUint16(resp[2:4], 12) // Attribute length: 12 bytes
		binary.BigEndian.PutUint32(resp[4:8], STUNMagicCookie)
		copy(resp[8:20], buf[8:20]) // Echo transaction ID

		// XOR-MAPPED-ADDRESS (type: 0x0020, len: 8)
		binary.BigEndian.PutUint16(resp[20:22], AttrXORMappedAddress)
		binary.BigEndian.PutUint16(resp[22:24], 8)
		resp[24] = 0x00 // Reserved
		resp[25] = 0x01 // IPv4 family

		// Expected IP: 203.0.113.195, Port: 54321
		expectedPort := uint16(54321)
		xport := expectedPort ^ uint16(STUNMagicCookie>>16)
		binary.BigEndian.PutUint16(resp[26:28], xport)

		expectedIP := net.ParseIP("203.0.113.195").To4()
		expectedIPUint := binary.BigEndian.Uint32(expectedIP)
		binary.BigEndian.PutUint32(resp[28:32], expectedIPUint^STUNMagicCookie)

		_, _ = pc.WriteTo(resp, addr)
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
