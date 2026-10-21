package main

import (
	"testing"
)

func TestBlockTransferPrototype(t *testing.T) {
	if err := runTransferDemo(); err != nil {
		t.Fatalf("expected successful 256 KiB block transfer, got error: %v", err)
	}
}
