package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"golang.org/x/crypto/pbkdf2"
)

const (
	BackupBundleVersion = 1
	PBKDF2Iterations    = 600000
	KeySizeBytes        = 32
	SaltSizeBytes       = 16
)

var (
	ErrEmptyPassphrase      = errors.New("crypto: passphrase cannot be empty")
	ErrInvalidBundleVersion = errors.New("crypto: unsupported backup bundle version")
	ErrCorruptBackupBundle  = errors.New("crypto: corrupt or invalid backup bundle")
	ErrPassphraseMismatch   = errors.New("crypto: invalid passphrase or tampered backup payload")
)

type EncryptedKeyBundle struct {
	Version    int    `json:"version"`
	KDF        string `json:"kdf"`
	Iterations int    `json:"iterations"`
	Salt       string `json:"salt"`
	Nonce      string `json:"nonce"`
	Ciphertext string `json:"ciphertext"`
	PubKey     string `json:"pubkey"`
}

// ExportEncryptedKey derives a 256-bit symmetric encryption key using PBKDF2-HMAC-SHA256
// with 600,000 iterations and encrypts the private key using AES-256-GCM.
func ExportEncryptedKey(priv ed25519.PrivateKey, passphrase string) ([]byte, error) {
	if len(priv) != ed25519.PrivateKeySize {
		return nil, ErrInvalidKeyLength
	}
	if passphrase == "" {
		return nil, ErrEmptyPassphrase
	}

	salt := make([]byte, SaltSizeBytes)
	if _, err := io.ReadFull(rand.Reader, salt); err != nil {
		return nil, fmt.Errorf("crypto: failed to generate salt: %w", err)
	}

	derivedKey := pbkdf2.Key([]byte(passphrase), salt, PBKDF2Iterations, KeySizeBytes, sha256.New)

	block, err := aes.NewCipher(derivedKey)
	if err != nil {
		return nil, fmt.Errorf("crypto: failed to create cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("crypto: failed to create gcm: %w", err)
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, fmt.Errorf("crypto: failed to generate nonce: %w", err)
	}

	pubKeyHex := hex.EncodeToString(priv.Public().(ed25519.PublicKey))

	// Additional authenticated data binds the public key to ciphertext
	ciphertext := gcm.Seal(nil, nonce, priv, []byte(pubKeyHex))

	bundle := EncryptedKeyBundle{
		Version:    BackupBundleVersion,
		KDF:        "pbkdf2-sha256",
		Iterations: PBKDF2Iterations,
		Salt:       hex.EncodeToString(salt),
		Nonce:      hex.EncodeToString(nonce),
		Ciphertext: hex.EncodeToString(ciphertext),
		PubKey:     pubKeyHex,
	}

	return json.Marshal(bundle)
}

// ImportEncryptedKey decrypts and validates an encrypted key bundle using the supplied passphrase.
func ImportEncryptedKey(bundleJSON []byte, passphrase string) (ed25519.PrivateKey, error) {
	if passphrase == "" {
		return nil, ErrEmptyPassphrase
	}

	var bundle EncryptedKeyBundle
	if err := json.Unmarshal(bundleJSON, &bundle); err != nil {
		return nil, ErrCorruptBackupBundle
	}

	if bundle.Version != BackupBundleVersion {
		return nil, ErrInvalidBundleVersion
	}

	salt, err := hex.DecodeString(bundle.Salt)
	if err != nil || len(salt) != SaltSizeBytes {
		return nil, ErrCorruptBackupBundle
	}

	nonce, err := hex.DecodeString(bundle.Nonce)
	if err != nil {
		return nil, ErrCorruptBackupBundle
	}

	ciphertext, err := hex.DecodeString(bundle.Ciphertext)
	if err != nil {
		return nil, ErrCorruptBackupBundle
	}

	iterations := bundle.Iterations
	if iterations <= 0 {
		iterations = PBKDF2Iterations
	}

	derivedKey := pbkdf2.Key([]byte(passphrase), salt, iterations, KeySizeBytes, sha256.New)

	block, err := aes.NewCipher(derivedKey)
	if err != nil {
		return nil, fmt.Errorf("crypto: cipher creation failed: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("crypto: gcm creation failed: %w", err)
	}

	if len(nonce) != gcm.NonceSize() {
		return nil, ErrCorruptBackupBundle
	}

	plaintext, err := gcm.Open(nil, nonce, ciphertext, []byte(bundle.PubKey))
	if err != nil {
		return nil, ErrPassphraseMismatch
	}

	if len(plaintext) != ed25519.PrivateKeySize {
		return nil, ErrCorruptBackupBundle
	}

	priv := ed25519.PrivateKey(plaintext)
	derivedPubHex := hex.EncodeToString(priv.Public().(ed25519.PublicKey))
	if derivedPubHex != bundle.PubKey {
		return nil, ErrCorruptBackupBundle
	}

	return priv, nil
}

