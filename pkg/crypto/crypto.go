package crypto

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	DomainManifest = "resonamesh:manifest:v1"
	DomainDecision = "resonamesh:decision:v1"
	DomainReport   = "resonamesh:report:v1"
	DomainReceipt  = "resonamesh:receipt:v1"
	DomainModeratorCert = "resonamesh:mod-cert:v1"
	DomainKeyRotation   = "resonamesh:key-rotation:v1"
)

var (
	ErrInvalidSignature = errors.New("crypto: invalid signature")
	ErrInvalidKeyLength = errors.New("crypto: invalid key length")
	ErrInvalidHash      = errors.New("crypto: chunk hash mismatch")
)

func GenerateKeyPair() (ed25519.PublicKey, ed25519.PrivateKey, error) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, nil, err
	}
	return pub, priv, nil
}

func SignWithDomain(priv ed25519.PrivateKey, domain string, payload []byte) []byte {
	message := append([]byte(domain+":"), payload...)
	return ed25519.Sign(priv, message)
}

func VerifyWithDomain(pub ed25519.PublicKey, domain string, payload []byte, sig []byte) bool {
	if len(pub) != ed25519.PublicKeySize || len(sig) != ed25519.SignatureSize {
		return false
	}
	message := append([]byte(domain+":"), payload...)
	return ed25519.Verify(pub, message, sig)
}

func HashChunk(chunk []byte) [32]byte {
	return sha256.Sum256(chunk)
}

func HashChunkHex(chunk []byte) string {
	sum := HashChunk(chunk)
	return hex.EncodeToString(sum[:])
}

func VerifyChunk(chunk []byte, expectedHex string) bool {
	actual := HashChunkHex(chunk)
	return strings.EqualFold(actual, expectedHex)
}

func HashContent(blockHexHashes []string) string {
	hasher := sha256.New()
	for _, h := range blockHexHashes {
		hasher.Write([]byte(h))
	}
	return hex.EncodeToString(hasher.Sum(nil))
}

func PublicKeyToHex(pub ed25519.PublicKey) string {
	return hex.EncodeToString(pub)
}

func HexToPublicKey(hexStr string) (ed25519.PublicKey, error) {
	bytes, err := hex.DecodeString(hexStr)
	if err != nil {
		return nil, err
	}
	if len(bytes) != ed25519.PublicKeySize {
		return nil, ErrInvalidKeyLength
	}
	return ed25519.PublicKey(bytes), nil
}

func PrivateKeyToHex(priv ed25519.PrivateKey) string {
	return hex.EncodeToString(priv)
}

func HexToPrivateKey(hexStr string) (ed25519.PrivateKey, error) {
	bytes, err := hex.DecodeString(hexStr)
	if err != nil {
		return nil, err
	}
	if len(bytes) != ed25519.PrivateKeySize {
		return nil, ErrInvalidKeyLength
	}
	return ed25519.PrivateKey(bytes), nil
}

func LoadOrCreatePrivateKey(filePath string) (ed25519.PrivateKey, error) {
	if data, err := os.ReadFile(filePath); err == nil {
		content := strings.TrimSpace(string(data))
		priv, err := HexToPrivateKey(content)
		if err == nil {
			return priv, nil
		}
		if len(data) == ed25519.PrivateKeySize {
			return ed25519.PrivateKey(data), nil
		}
		return nil, fmt.Errorf("crypto: invalid key file contents at %s: %w", filePath, err)
	}

	dir := filepath.Dir(filePath)
	if dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0700); err != nil {
			return nil, fmt.Errorf("crypto: failed to create key directory: %w", err)
		}
	}

	_, priv, err := GenerateKeyPair()
	if err != nil {
		return nil, fmt.Errorf("crypto: failed to generate key pair: %w", err)
	}

	hexKey := PrivateKeyToHex(priv)
	if err := os.WriteFile(filePath, []byte(hexKey), 0600); err != nil {
		return nil, fmt.Errorf("crypto: failed to save private key: %w", err)
	}

	return priv, nil
}

