package crypto

import (
	"bytes"
	"crypto/ed25519"
	"encoding/json"
	"testing"
)

func TestExportAndImportEncryptedKey(t *testing.T) {
	pub, priv, err := GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair failed: %v", err)
	}

	passphrase := "CorrectHorseBatteryStaple-2026!"

	bundleBytes, err := ExportEncryptedKey(priv, passphrase)
	if err != nil {
		t.Fatalf("ExportEncryptedKey failed: %v", err)
	}

	// Verify JSON structure
	var bundle EncryptedKeyBundle
	if err := json.Unmarshal(bundleBytes, &bundle); err != nil {
		t.Fatalf("invalid json: %v", err)
	}
	if bundle.Version != 1 || bundle.KDF != "pbkdf2-sha256" || bundle.Iterations != 600000 {
		t.Fatalf("unexpected bundle header: %+v", bundle)
	}

	// Successful decryption with correct passphrase
	recoveredPriv, err := ImportEncryptedKey(bundleBytes, passphrase)
	if err != nil {
		t.Fatalf("ImportEncryptedKey failed: %v", err)
	}
	if !bytes.Equal(recoveredPriv, priv) {
		t.Fatalf("recovered key does not match original private key")
	}
	if !bytes.Equal(recoveredPriv.Public().(ed25519.PublicKey), pub) {
		t.Fatalf("recovered key public component does not match original public key")
	}

	// Decryption failure with wrong passphrase
	_, err = ImportEncryptedKey(bundleBytes, "WrongPassword!")
	if err != ErrPassphraseMismatch {
		t.Fatalf("expected ErrPassphraseMismatch, got %v", err)
	}

	// Decryption failure with empty passphrase
	_, err = ImportEncryptedKey(bundleBytes, "")
	if err != ErrEmptyPassphrase {
		t.Fatalf("expected ErrEmptyPassphrase, got %v", err)
	}

	// Decryption failure with tampered ciphertext
	tamperedBundle := bundle
	tamperedCipher := []byte(tamperedBundle.Ciphertext)
	if len(tamperedCipher) > 2 {
		tamperedCipher[0] = 'f'
		tamperedCipher[1] = '0'
		tamperedBundle.Ciphertext = string(tamperedCipher)
		tamperedBytes, _ := json.Marshal(tamperedBundle)
		_, err = ImportEncryptedKey(tamperedBytes, passphrase)
		if err == nil {
			t.Fatalf("expected error on tampered ciphertext, got nil")
		}
	}
}
