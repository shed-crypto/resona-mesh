package audio_test

import (
	"bytes"
	"testing"

	"resonamesh/internal/audio"
	"resonamesh/pkg/protocol"
)

func TestChunkerExactMultiple(t *testing.T) {
	// 512 KiB = exactly 2 chunks of 256 KiB
	data := make([]byte, 2*protocol.BlockSize)
	for i := range data {
		data[i] = byte(i % 256)
	}

	chunker := audio.NewChunker()
	chunks, contentID, totalBytes, err := chunker.Split(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("expected successful chunking, got error: %v", err)
	}

	if totalBytes != int64(len(data)) {
		t.Fatalf("expected total bytes %d, got %d", len(data), totalBytes)
	}

	if len(chunks) != 2 {
		t.Fatalf("expected 2 chunks, got %d", len(chunks))
	}

	if len(chunks[0].Data) != protocol.BlockSize || len(chunks[1].Data) != protocol.BlockSize {
		t.Fatalf("expected both chunks to be 256 KiB, got %d and %d", len(chunks[0].Data), len(chunks[1].Data))
	}

	if len(contentID) != 64 {
		t.Fatalf("expected 64-char hex content ID, got: %s", contentID)
	}
}

func TestChunkerPartialTrailingChunk(t *testing.T) {
	// 600 KiB = 256 + 256 + 88 KiB (3 chunks)
	totalExpected := int64(600 * 1024)
	data := make([]byte, totalExpected)

	chunker := audio.NewChunker()
	chunks, contentID, totalBytes, err := chunker.Split(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("expected successful chunking, got error: %v", err)
	}

	if totalBytes != totalExpected {
		t.Fatalf("expected total bytes %d, got %d", totalExpected, totalBytes)
	}

	if len(chunks) != 3 {
		t.Fatalf("expected 3 chunks, got %d", len(chunks))
	}

	if len(chunks[2].Data) != 88*1024 {
		t.Fatalf("expected trailing chunk size 88 KiB, got %d", len(chunks[2].Data))
	}

	if chunks[0].Index != 0 || chunks[1].Index != 1 || chunks[2].Index != 2 {
		t.Fatal("chunk indices are not monotonically sequential")
	}

	if len(contentID) != 64 {
		t.Fatalf("expected 64-char content ID, got: %s", contentID)
	}
}

func TestChunkerEmptyStream(t *testing.T) {
	chunker := audio.NewChunker()
	_, _, _, err := chunker.Split(bytes.NewReader(nil))
	if err != audio.ErrEmptyAudioStream {
		t.Fatalf("expected ErrEmptyAudioStream for empty reader, got: %v", err)
	}
}
