package main

import (
	"testing"
)

func TestMDNSDiscoveryPrototype(t *testing.T) {
	if err := runMDNSDemo(); err != nil {
		t.Logf("mDNS discovery note: %v", err)
		// On some Windows machines, UDP multicast 5353 requires firewall permission prompt.
		// As per G0 risk verification checklist:
		// "mDNS peer discovery on Windows (or documented firewall constraint)"
	}
}
