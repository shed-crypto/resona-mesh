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

// DefaultSTUNServer is Google's canonical public STUN resolver.
const DefaultSTUNServer = "stun.l.google.com:19302"

// DefaultSTUNServers defines reliable, public STUN resolvers with automatic fallback.
var DefaultSTUNServers = []string{
	"stun.l.google.com:19302",
	"stun.cloudflare.com:3478",
	"stun1.l.google.com:19302",
	"stun2.l.google.com:19302",
}

// STUNMagicCookie is the standard RFC 5389 magic cookie value.
const STUNMagicCookie = 0x2112A442

// STUNBindingRequest is the RFC 5389 Binding Request message type.
const STUNBindingRequest = 0x0001

// STUNBindingResponse is the RFC 5389 Binding Response message type.
const STUNBindingResponse = 0x0101

// AttrXORMappedAddress is the RFC 5389 attribute for XOR-encoded public endpoint.
const AttrXORMappedAddress = 0x0020

// AttrMappedAddress is the legacy RFC 3489 attribute for raw public endpoint.
const AttrMappedAddress = 0x0001

// NATType classifies the router's UDP port mapping behavior discovered via multi-server STUN.
type NATType string

const (
	NATTypeUnknown             NATType = "unknown"
	NATTypeEndpointIndependent NATType = "endpoint_independent"
	NATTypeSymmetric           NATType = "symmetric"
)

// NATDiagnosis holds the external IP, mapped UDP ports, and predicted ports for NAT traversal.
type NATDiagnosis struct {
	PublicIP       net.IP
	PrimaryPort    int
	SecondaryPort  int
	Delta          int
	NATType        NATType
	CandidatePorts []int
}

var (
	ErrSTUNTimeout   = errors.New("stun: resolution request timed out")
	ErrSTUNMalformed = errors.New("stun: malformed response from STUN server")
)

// ResolvePublicAddr queries a public STUN server to discover the local node's external WAN IP and UDP port.
// If serverAddr is empty, it queries DefaultSTUNServers in order until one responds successfully.
func ResolvePublicAddr(ctx context.Context, serverAddr string) (*net.UDPAddr, error) {
	return ResolvePublicAddrWithPacketConn(ctx, nil, serverAddr)
}

// ResolvePublicAddrWithPacketConn queries a STUN server using an existing net.PacketConn
// (such as the shared libp2p QUIC UDP socket) so the NAT router maps the exact listening port.
func ResolvePublicAddrWithPacketConn(ctx context.Context, pc net.PacketConn, serverAddr string) (*net.UDPAddr, error) {
	if serverAddr != "" {
		return querySingleSTUN(ctx, pc, serverAddr)
	}

	var lastErr error
	for _, srv := range DefaultSTUNServers {
		subCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
		addr, err := querySingleSTUN(subCtx, pc, srv)
		cancel()
		if err == nil && addr != nil {
			return addr, nil
		}
		lastErr = err
	}
	if lastErr != nil {
		return nil, fmt.Errorf("stun: all fallback servers failed: %w", lastErr)
	}
	return nil, errors.New("stun: no STUN servers available")
}

// DiagnoseNAT queries up to two distinct STUN servers over the provided PacketConn to determine
// whether the node is behind an Endpoint-Independent (Cone) NAT or a Symmetric NAT, and computes
// sequential candidate ports when symmetric port allocation is detected.
func DiagnoseNAT(ctx context.Context, pc net.PacketConn, servers []string) (*NATDiagnosis, error) {
	if len(servers) == 0 {
		servers = DefaultSTUNServers
	}

	var resolved []*net.UDPAddr
	var lastErr error

	for _, srv := range servers {
		if len(resolved) >= 2 {
			break
		}
		subCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
		addr, err := querySingleSTUN(subCtx, pc, srv)
		cancel()
		if err != nil {
			lastErr = err
			continue
		}
		if addr != nil {
			resolved = append(resolved, addr)
		}
	}

	if len(resolved) == 0 {
		if lastErr != nil {
			return nil, fmt.Errorf("stun: diagnosis failed on all servers: %w", lastErr)
		}
		return nil, errors.New("stun: no STUN servers available")
	}

	first := resolved[0]
	diag := &NATDiagnosis{
		PublicIP:       first.IP,
		PrimaryPort:    first.Port,
		NATType:        NATTypeEndpointIndependent,
		CandidatePorts: []int{first.Port},
	}

	if len(resolved) >= 2 {
		second := resolved[1]
		diag.SecondaryPort = second.Port
		diag.Delta = second.Port - first.Port
		if second.Port != first.Port {
			diag.NATType = NATTypeSymmetric
			diag.CandidatePorts = []int{second.Port, first.Port}
			if diag.Delta != 0 && diag.Delta >= -32 && diag.Delta <= 32 {
				for step := 1; step <= 3; step++ {
					pred := second.Port + diag.Delta*step
					if pred > 1024 && pred <= 65535 {
						diag.CandidatePorts = append(diag.CandidatePorts, pred)
					}
				}
			}
		}
	}

	return diag, nil
}

func querySingleSTUN(ctx context.Context, sharedConn net.PacketConn, serverAddr string) (*net.UDPAddr, error) {
	if sharedConn == nil {
		var lc net.ListenConfig
		ephemeral, err := lc.ListenPacket(ctx, "udp", ":0")
		if err != nil {
			return nil, fmt.Errorf("stun: listen packet failed: %w", err)
		}
		defer ephemeral.Close()
		sharedConn = ephemeral
	}

	raddr, err := net.ResolveUDPAddr("udp", serverAddr)
	if err != nil {
		return nil, fmt.Errorf("stun: resolve server addr failed: %w", err)
	}

	deadline := time.Now().Add(3 * time.Second)
	if d, ok := ctx.Deadline(); ok {
		deadline = d
	}
	_ = sharedConn.SetReadDeadline(deadline)
	defer func() {
		_ = sharedConn.SetReadDeadline(time.Time{})
	}()

	// Construct RFC 5389 STUN Binding Request (20 bytes header, 0 bytes body)
	req := make([]byte, 20)
	binary.BigEndian.PutUint16(req[0:2], STUNBindingRequest)
	binary.BigEndian.PutUint16(req[2:4], 0x0000)
	binary.BigEndian.PutUint32(req[4:8], STUNMagicCookie)
	txID := req[8:20]
	if _, err := rand.Read(txID); err != nil {
		return nil, fmt.Errorf("stun: failed to generate transaction id: %w", err)
	}

	if _, err := sharedConn.WriteTo(req, raddr); err != nil {
		return nil, fmt.Errorf("stun: failed to transmit binding request: %w", err)
	}

	resp := make([]byte, 1024)
	for {
		n, _, err := sharedConn.ReadFrom(resp)
		if err != nil {
			return nil, fmt.Errorf("stun: failed reading response: %w", err)
		}
		if n < 20 {
			continue
		}
		// Match transaction ID so stray packets on shared socket are ignored
		matchTx := true
		for i := 0; i < 12; i++ {
			if resp[8+i] != txID[i] {
				matchTx = false
				break
			}
		}
		if !matchTx {
			continue
		}
		return parseSTUNBindingResponse(resp[:n], txID)
	}
}

func parseSTUNBindingResponse(resp []byte, txID []byte) (*net.UDPAddr, error) {
	if len(resp) < 20 {
		return nil, ErrSTUNMalformed
	}

	msgType := binary.BigEndian.Uint16(resp[0:2])
	if msgType != STUNBindingResponse {
		return nil, fmt.Errorf("%w: unexpected message type 0x%04x", ErrSTUNMalformed, msgType)
	}

	magicCookie := binary.BigEndian.Uint32(resp[4:8])
	msgLen := int(binary.BigEndian.Uint16(resp[2:4]))
	end := 20 + msgLen
	if end > len(resp) {
		end = len(resp)
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
			xport := binary.BigEndian.Uint16(attrVal[2:4])
			port := xport ^ uint16(magicCookie>>16)
			if family == 0x01 { // IPv4
				xip := binary.BigEndian.Uint32(attrVal[4:8])
				ipBytes := make([]byte, 4)
				binary.BigEndian.PutUint32(ipBytes, xip^magicCookie)
				return &net.UDPAddr{
					IP:   net.IP(ipBytes),
					Port: int(port),
				}, nil
			} else if family == 0x02 && len(attrVal) >= 20 && len(txID) == 12 { // IPv6
				ipBytes := make([]byte, 16)
				var cookieBytes [4]byte
				binary.BigEndian.PutUint32(cookieBytes[:], magicCookie)
				for i := 0; i < 4; i++ {
					ipBytes[i] = attrVal[4+i] ^ cookieBytes[i]
				}
				for i := 0; i < 12; i++ {
					ipBytes[4+i] = attrVal[8+i] ^ txID[i]
				}
				return &net.UDPAddr{
					IP:   net.IP(ipBytes),
					Port: int(port),
				}, nil
			}
		}

		if attrType == AttrMappedAddress && len(attrVal) >= 8 {
			family := attrVal[1]
			port := binary.BigEndian.Uint16(attrVal[2:4])
			if family == 0x01 { // IPv4
				ip := make(net.IP, 4)
				copy(ip, attrVal[4:8])
				return &net.UDPAddr{
					IP:   ip,
					Port: int(port),
				}, nil
			} else if family == 0x02 && len(attrVal) >= 20 { // IPv6
				ip := make(net.IP, 16)
				copy(ip, attrVal[4:20])
				return &net.UDPAddr{
					IP:   ip,
					Port: int(port),
				}, nil
			}
		}
	}

	return nil, fmt.Errorf("%w: response missing mapped address attribute", ErrSTUNMalformed)
}
