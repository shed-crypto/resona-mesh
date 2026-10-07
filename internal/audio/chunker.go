package audio

import (
	"errors"
	"io"

	"resonamesh/pkg/crypto"
	"resonamesh/pkg/protocol"
)

var (
	ErrEmptyAudioStream = errors.New("audio: audio stream is empty")
)

// Chunk represents a single 256 KiB audio block and its cryptographic identifier.
type Chunk struct {
	Index int
	Hash  string
	Data  []byte
}

// Chunker splits an arbitrary audio data stream into discrete 256 KiB blocks.
type Chunker struct {
	blockSize int
}

// NewChunker creates a chunker with the canonical 256 KiB block size.
func NewChunker() *Chunker {
	return &Chunker{blockSize: protocol.BlockSize}
}

// Split processes an io.Reader, generating chunks and verifying hashes deterministically.
func (c *Chunker) Split(r io.Reader) ([]Chunk, string, int64, error) {
	var chunks []Chunk
	var blockHashes []string
	var totalBytes int64

	buf := make([]byte, c.blockSize)
	index := 0

	for {
		n, err := io.ReadFull(r, buf)
		if n > 0 {
			chunkData := make([]byte, n)
			copy(chunkData, buf[:n])

			hash := crypto.HashChunkHex(chunkData)
			chunks = append(chunks, Chunk{
				Index: index,
				Hash:  hash,
				Data:  chunkData,
			})
			blockHashes = append(blockHashes, hash)
			totalBytes += int64(n)
			index++
		}

		if err != nil {
			if err == io.EOF || err == io.ErrUnexpectedEOF {
				break
			}
			return nil, "", 0, err
		}
	}

	if len(chunks) == 0 {
		return nil, "", 0, ErrEmptyAudioStream
	}

	contentID := crypto.HashContent(blockHashes)
	return chunks, contentID, totalBytes, nil
}
