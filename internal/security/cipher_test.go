package security

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCipherRoundTrip(t *testing.T) {
	keyPath := filepath.Join(t.TempDir(), "secrets.key")
	cipher, err := NewCipher(keyPath)
	if err != nil {
		t.Fatalf("NewCipher: %v", err)
	}
	info, err := os.Stat(keyPath)
	if err != nil {
		t.Fatalf("stat key: %v", err)
	}
	if info.Size() != 32 {
		t.Fatalf("expected 32-byte key, got %d", info.Size())
	}

	cases := []string{"", "password", "sk-1234567890", "中文密钥-🔐"}
	for _, plain := range cases {
		encrypted, err := cipher.Encrypt(plain)
		if err != nil {
			t.Fatalf("encrypt %q: %v", plain, err)
		}
		decrypted, err := cipher.Decrypt(encrypted)
		if err != nil {
			t.Fatalf("decrypt %q: %v", plain, err)
		}
		if decrypted != plain {
			t.Fatalf("roundtrip mismatch: %q != %q", decrypted, plain)
		}
	}

	// 另一把密钥无法解密。
	other, err := NewCipher(filepath.Join(t.TempDir(), "other.key"))
	if err != nil {
		t.Fatalf("other cipher: %v", err)
	}
	encrypted, _ := cipher.Encrypt("secret")
	if _, err := other.Decrypt(encrypted); err == nil {
		t.Fatal("expected decrypt failure with different key")
	}
}

func TestCipherLoadsExistingKey(t *testing.T) {
	keyPath := filepath.Join(t.TempDir(), "secrets.key")
	first, err := NewCipher(keyPath)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	encrypted, err := first.Encrypt("persisted")
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	second, err := NewCipher(keyPath)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	plain, err := second.Decrypt(encrypted)
	if err != nil || plain != "persisted" {
		t.Fatalf("expected persisted key to decrypt, got %q err=%v", plain, err)
	}
}
