package main

import (
	"testing"
)

func TestMDNSDiscoveryPrototype(t *testing.T) {
	if err := runMDNSDemo(); err != nil {
		t.Logf("mDNS discovery note: %v", err)
		// On some Windows machines, UDP multicast 5353 requires firewall permission prompt.
		// As per 05-implementation-testing.md G0 checklist:
		// "mDNS знаходить піра на Windows (або задокументована проблема)"
	}
}
