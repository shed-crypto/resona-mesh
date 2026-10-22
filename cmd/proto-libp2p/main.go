package main

import (
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"log"
	"time"

	"github.com/libp2p/go-libp2p"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"

	"resonamesh/pkg/crypto"
	"resonamesh/pkg/protocol"
)

func runTransferDemo() error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// 1. Create Receiver Host on dynamic loopback port
	receiver, err := libp2p.New(
		libp2p.ListenAddrStrings("/ip4/127.0.0.1/tcp/0"),
		libp2p.DisableRelay(),
	)
	if err != nil {
		return fmt.Errorf("failed to create receiver host: %w", err)
	}
	defer receiver.Close()

	// 2. Prepare 256 KiB synthetic audio chunk
	chunk := make([]byte, protocol.BlockSize)
	for i := range chunk {
		chunk[i] = byte(i % 251) // deterministic pseudorandom pattern
	}
	expectedHash := crypto.HashChunkHex(chunk)

	transferDone := make(chan error, 1)

	// 3. Register stream handler on Receiver
	receiver.SetStreamHandler(protocol.ProtocolBlock, func(s network.Stream) {
		defer s.Close()

		// Read length prefix (uint32)
		var length uint32
		if err := binary.Read(s, binary.BigEndian, &length); err != nil {
			transferDone <- fmt.Errorf("receiver failed to read length: %w", err)
			return
		}

		if int(length) != protocol.BlockSize {
			transferDone <- fmt.Errorf("unexpected block size %d != %d", length, protocol.BlockSize)
			return
		}

		// Read chunk data
		buf := make([]byte, length)
		if _, err := io.ReadFull(s, buf); err != nil {
			transferDone <- fmt.Errorf("receiver failed to read full payload: %w", err)
			return
		}

		// Verify SHA-256
		if !crypto.VerifyChunk(buf, expectedHash) {
			transferDone <- fmt.Errorf("SHA-256 chunk verification mismatch")
			return
		}

		// Send ack byte
		if _, err := s.Write([]byte{0x01}); err != nil {
			transferDone <- fmt.Errorf("receiver failed to write ack: %w", err)
			return
		}

		transferDone <- nil
	})

	// 4. Create Sender Host
	sender, err := libp2p.New(
		libp2p.ListenAddrStrings("/ip4/127.0.0.1/tcp/0"),
		libp2p.DisableRelay(),
	)
	if err != nil {
		return fmt.Errorf("failed to create sender host: %w", err)
	}
	defer sender.Close()

	// 5. Connect Sender to Receiver
	receiverInfo := peer.AddrInfo{
		ID:    receiver.ID(),
		Addrs: receiver.Addrs(),
	}
	if err := sender.Connect(ctx, receiverInfo); err != nil {
		return fmt.Errorf("sender failed to connect to receiver: %w", err)
	}

	// 6. Open stream and transmit 256 KiB chunk
	s, err := sender.NewStream(ctx, receiver.ID(), protocol.ProtocolBlock)
	if err != nil {
		return fmt.Errorf("failed to open stream: %w", err)
	}
	defer s.Close()

	// Write length prefix
	if err := binary.Write(s, binary.BigEndian, uint32(len(chunk))); err != nil {
		return fmt.Errorf("sender failed to write length: %w", err)
	}

	// Write chunk
	if _, err := s.Write(chunk); err != nil {
		return fmt.Errorf("sender failed to write chunk payload: %w", err)
	}

	// Read ack byte
	ack := make([]byte, 1)
	if _, err := io.ReadFull(s, ack); err != nil {
		return fmt.Errorf("sender failed to read ack: %w", err)
	}

	select {
	case err := <-transferDone:
		if err != nil {
			return err
		}
	case <-ctx.Done():
		return ctx.Err()
	}

	fmt.Printf("[PASS] Successfully transmitted 256 KiB block via libp2p TCP/Noise\n")
	fmt.Printf("       Chunk SHA-256: %s\n", expectedHash)
	return nil
}

func main() {
	if err := runTransferDemo(); err != nil {
		log.Fatalf("Transfer demo failed: %v", err)
	}
}
