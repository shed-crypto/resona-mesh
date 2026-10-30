package p2p

import (
	"context"
	"crypto/rand"
	"encoding/binary"
	"errors"
	"fmt"
	"net"
	"time"
)

const (
	// DefaultSTUNServer is Google's canonical public STUN resolver.
	DefaultSTUNServer = "stun.l.google.com:19302"

	// STUNMagicCookie is the standard RFC 5389 magic cookie value.
	STUNMagicCookie = 0x2112A442

	// STUNBindingRequest is the RFC 5389 Binding Request message type.
	STUNBindingRequest = 0x0001

	// STUNBindingResponse is the RFC 5389 Binding Response message type.
	STUNBindingResponse = 0x0101

	// AttrXORMappedAddress is the RFC 5389 attribute for XOR-encoded public endpoint.
	AttrXORMappedAddress = 0x0020

	// AttrMappedAddress is the legacy RFC 3489 attribute for raw public endpoint.
	AttrMappedAddress = 0x0001
)

var (
	ErrSTUNTimeout   = errors.New("stun: resolution request timed out")
	ErrSTUNMalformed = errors.New("stun: malformed response from STUN server")
)

// ResolvePublicAddr queries a public STUN server to discover the local node's external WAN IP and UDP port.
func ResolvePublicAddr(ctx context.Context, serverAddr string) (*net.UDPAddr, error) {
	if serverAddr == "" {
		serverAddr = DefaultSTUNServer
	}

	var d net.Dialer
	conn, err := d.DialContext(ctx, "udp", serverAddr)
	if err != nil {
		return nil, fmt.Errorf("stun: dial failed: %w", err)
	}
	defer conn.Close()

	if deadline, ok := ctx.Deadline(); ok {
		_ = conn.SetDeadline(deadline)
	} else {
		_ = conn.SetDeadline(time.Now().Add(4 * time.Second))
	}

	// Construct RFC 5389 STUN Binding Request (20 bytes header, 0 bytes body)
	req := make([]byte, 20)
	binary.BigEndian.PutUint16(req[0:2], STUNBindingRequest)
	binary.BigEndian.PutUint16(req[2:4], 0x0000)
	binary.BigEndian.PutUint32(req[4:8], STUNMagicCookie)
	if _, err := rand.Read(req[8:20]); err != nil {
		return nil, fmt.Errorf("stun: failed to generate transaction id: %w", err)
	}

	if _, err := conn.Write(req); err != nil {
		return nil, fmt.Errorf("stun: failed to transmit binding request: %w", err)
	}

	resp := make([]byte, 1024)
	n, err := conn.Read(resp)
	if err != nil {
		return nil, fmt.Errorf("stun: failed reading response: %w", err)
	}

	if n < 20 {
		return nil, ErrSTUNMalformed
	}

	msgType := binary.BigEndian.Uint16(resp[0:2])
	if msgType != STUNBindingResponse {
		return nil, fmt.Errorf("%w: unexpected message type 0x%04x", ErrSTUNMalformed, msgType)
	}

	magicCookie := binary.BigEndian.Uint32(resp[4:8])
	msgLen := int(binary.BigEndian.Uint16(resp[2:4]))
	end := 20 + msgLen
	if end > n {
		end = n
	}

	offset := 20
	for offset+4 <= end {
		attrType := binary.BigEndian.Uint16(resp[offset : offset+2])
		attrLen := int(binary.BigEndian.Uint16(resp[offset+2 : offset+4]))
		offset += 4

		if offset+attrLen > end {
			break
		}

		attrVal := resp[offset : offset+attrLen]
		offset += attrLen

		// 4-byte padding alignment per RFC 5389
		if rem := attrLen % 4; rem != 0 {
			offset += 4 - rem
		}

		if attrType == AttrXORMappedAddress && len(attrVal) >= 8 {
			family := attrVal[1]
			if family == 0x01 { // IPv4
				xport := binary.BigEndian.Uint16(attrVal[2:4])
				port := xport ^ uint16(magicCookie>>16)
				xip := binary.BigEndian.Uint32(attrVal[4:8])
				ipBytes := make([]byte, 4)
				binary.BigEndian.PutUint32(ipBytes, xip^magicCookie)
				return &net.UDPAddr{
					IP:   net.IP(ipBytes),
					Port: int(port),
				}, nil
			}
		}

		if attrType == AttrMappedAddress && len(attrVal) >= 8 {
			family := attrVal[1]
			if family == 0x01 { // IPv4
				port := binary.BigEndian.Uint16(attrVal[2:4])
				ip := net.IP(attrVal[4:8])
				return &net.UDPAddr{
					IP:   ip,
					Port: int(port),
				}, nil
			}
		}
	}

	return nil, fmt.Errorf("%w: response missing mapped address attribute", ErrSTUNMalformed)
}
