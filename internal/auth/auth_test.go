package auth

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func TestCredentialsIsExpired_Valid(t *testing.T) {
	creds := &Credentials{
		AccessToken: "tok",
		ExpiresAt:   time.Now().Add(1 * time.Hour),
	}
	if creds.IsExpired() {
		t.Error("expected credentials to not be expired")
	}
}

func TestCredentialsIsExpired_Expired(t *testing.T) {
	creds := &Credentials{
		AccessToken: "tok",
		ExpiresAt:   time.Now().Add(-1 * time.Minute),
	}
	if !creds.IsExpired() {
		t.Error("expected credentials to be expired")
	}
}

func TestCredentialsIsExpired_WithinBuffer(t *testing.T) {
	// Within 5-minute buffer should count as expired
	creds := &Credentials{
		AccessToken: "tok",
		ExpiresAt:   time.Now().Add(3 * time.Minute),
	}
	if !creds.IsExpired() {
		t.Error("expected credentials within 5-min buffer to be expired")
	}
}

func TestCredentialsIsExpired_JustOutsideBuffer(t *testing.T) {
	creds := &Credentials{
		AccessToken: "tok",
		ExpiresAt:   time.Now().Add(6 * time.Minute),
	}
	if creds.IsExpired() {
		t.Error("expected credentials just outside buffer to not be expired")
	}
}

func TestCredentialsIsValid(t *testing.T) {
	tests := []struct {
		name  string
		creds *Credentials
		want  bool
	}{
		{
			name:  "valid credentials",
			creds: &Credentials{AccessToken: "tok", ExpiresAt: time.Now().Add(1 * time.Hour)},
			want:  true,
		},
		{
			name:  "empty access token",
			creds: &Credentials{AccessToken: "", ExpiresAt: time.Now().Add(1 * time.Hour)},
			want:  false,
		},
		{
			name:  "expired token",
			creds: &Credentials{AccessToken: "tok", ExpiresAt: time.Now().Add(-1 * time.Hour)},
			want:  false,
		},
		{
			name:  "within buffer",
			creds: &Credentials{AccessToken: "tok", ExpiresAt: time.Now().Add(2 * time.Minute)},
			want:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.creds.IsValid()
			if got != tt.want {
				t.Errorf("IsValid() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestCredentialsFromTokenResponse(t *testing.T) {
	before := time.Now()
	resp := &TokenResponse{
		AccessToken:  "access",
		RefreshToken: "refresh",
		IDToken:      "id",
		TokenType:    "Bearer",
		ExpiresIn:    3600,
	}

	creds := CredentialsFromTokenResponse(resp)
	after := time.Now()

	if creds.AccessToken != "access" {
		t.Errorf("AccessToken = %q, want %q", creds.AccessToken, "access")
	}
	if creds.RefreshToken != "refresh" {
		t.Errorf("RefreshToken = %q, want %q", creds.RefreshToken, "refresh")
	}
	if creds.IDToken != "id" {
		t.Errorf("IDToken = %q, want %q", creds.IDToken, "id")
	}
	if creds.TokenType != "Bearer" {
		t.Errorf("TokenType = %q, want %q", creds.TokenType, "Bearer")
	}

	expectedMin := before.Add(3600 * time.Second)
	expectedMax := after.Add(3600 * time.Second)
	if creds.ExpiresAt.Before(expectedMin) || creds.ExpiresAt.After(expectedMax) {
		t.Errorf("ExpiresAt = %v, want between %v and %v", creds.ExpiresAt, expectedMin, expectedMax)
	}
}

func TestEncryptDecrypt(t *testing.T) {
	key, err := generateRandomKey()
	if err != nil {
		t.Fatalf("generateRandomKey() error: %v", err)
	}

	plaintext := []byte("hello, world! this is a test of encryption.")

	ciphertext, err := Encrypt(plaintext, key)
	if err != nil {
		t.Fatalf("Encrypt() error: %v", err)
	}

	if string(ciphertext) == string(plaintext) {
		t.Error("ciphertext should differ from plaintext")
	}

	decrypted, err := Decrypt(ciphertext, key)
	if err != nil {
		t.Fatalf("Decrypt() error: %v", err)
	}

	if string(decrypted) != string(plaintext) {
		t.Errorf("Decrypt() = %q, want %q", string(decrypted), string(plaintext))
	}
}

func TestEncryptDecrypt_WrongKey(t *testing.T) {
	key1, _ := generateRandomKey()
	key2, _ := generateRandomKey()

	ciphertext, err := Encrypt([]byte("secret"), key1)
	if err != nil {
		t.Fatalf("Encrypt() error: %v", err)
	}

	_, err = Decrypt(ciphertext, key2)
	if err == nil {
		t.Error("decrypt with wrong key should fail")
	}
}

func TestWriteReadEncryptedCache(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("INTENTRA_CONFIG_DIR", tmpDir)

	// Reset keyring singleton so it re-opens with test config
	ringOnce = sync.Once{}
	ring = nil
	ringOpenErr = nil

	creds := &Credentials{
		AccessToken:  "test-access-token",
		RefreshToken: "test-refresh-token",
		TokenType:    "Bearer",
		ExpiresAt:    time.Now().Add(1 * time.Hour).Truncate(time.Second),
		Email:        "test@example.com",
	}

	if err := WriteEncryptedCache(creds); err != nil {
		t.Fatalf("WriteEncryptedCache() error: %v", err)
	}

	// Verify file exists
	encFile := filepath.Join(tmpDir, "credentials.enc")
	if _, err := os.Stat(encFile); os.IsNotExist(err) {
		t.Fatal("encrypted cache file should exist")
	}

	loaded, err := ReadEncryptedCache()
	if err != nil {
		t.Fatalf("ReadEncryptedCache() error: %v", err)
	}
	if loaded == nil {
		t.Fatal("ReadEncryptedCache() returned nil")
	}

	if loaded.AccessToken != creds.AccessToken {
		t.Errorf("AccessToken = %q, want %q", loaded.AccessToken, creds.AccessToken)
	}
	if loaded.RefreshToken != creds.RefreshToken {
		t.Errorf("RefreshToken = %q, want %q", loaded.RefreshToken, creds.RefreshToken)
	}
	if loaded.Email != creds.Email {
		t.Errorf("Email = %q, want %q", loaded.Email, creds.Email)
	}
}

func TestDerivedKeyConsistency(t *testing.T) {
	key1, err := DeriveKey(CacheKeySalt, CacheKeyInfo)
	if err != nil {
		t.Fatalf("DeriveKey() error: %v", err)
	}
	key2, err := DeriveKey(CacheKeySalt, CacheKeyInfo)
	if err != nil {
		t.Fatalf("DeriveKey() second call error: %v", err)
	}

	if string(key1) != string(key2) {
		t.Error("DeriveKey() should return consistent results")
	}
	if len(key1) != keySize {
		t.Errorf("key length = %d, want %d", len(key1), keySize)
	}
}

func TestFileLock(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("INTENTRA_CONFIG_DIR", tmpDir)

	release, err := acquireCredentialLock()
	if err != nil {
		t.Fatalf("acquireCredentialLock() error: %v", err)
	}

	lockFile := filepath.Join(tmpDir, lockFileName)
	if _, err := os.Stat(lockFile); os.IsNotExist(err) {
		t.Error("lock file should exist after acquisition")
	}

	release()

	if _, err := os.Stat(lockFile); !os.IsNotExist(err) {
		t.Error("lock file should be removed after release")
	}
}

func TestWithCredentialLock(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("INTENTRA_CONFIG_DIR", tmpDir)

	called := false
	err := WithCredentialLock(func() error {
		called = true
		return nil
	})
	if err != nil {
		t.Fatalf("WithCredentialLock() error: %v", err)
	}
	if !called {
		t.Error("callback should have been called")
	}
}

func TestDeleteEncryptedCache(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("INTENTRA_CONFIG_DIR", tmpDir)

	// Reset keyring singleton
	ringOnce = sync.Once{}
	ring = nil
	ringOpenErr = nil

	creds := &Credentials{
		AccessToken:  "tok",
		RefreshToken: "ref",
		TokenType:    "Bearer",
		ExpiresAt:    time.Now().Add(1 * time.Hour),
	}
	if err := WriteEncryptedCache(creds); err != nil {
		t.Fatalf("WriteEncryptedCache() error: %v", err)
	}

	if err := DeleteEncryptedCache(); err != nil {
		t.Fatalf("DeleteEncryptedCache() error: %v", err)
	}

	loaded, err := ReadEncryptedCache()
	if err != nil {
		// Expected — file was deleted
		return
	}
	if loaded != nil {
		t.Error("ReadEncryptedCache() should return nil after delete")
	}
}

func TestLoadCredentialsFromKeyring_EnvToken(t *testing.T) {
	t.Setenv("INTENTRA_TOKEN", "test-env-token")

	creds, err := LoadCredentialsFromKeyring()
	if err != nil {
		t.Fatalf("LoadCredentialsFromKeyring() error: %v", err)
	}
	if creds == nil {
		t.Fatal("expected non-nil credentials")
	}
	if creds.AccessToken != "test-env-token" {
		t.Errorf("AccessToken = %q, want %q", creds.AccessToken, "test-env-token")
	}
	if creds.TokenType != "Bearer" {
		t.Errorf("TokenType = %q, want %q", creds.TokenType, "Bearer")
	}
	if creds.RefreshToken != "env-token-no-refresh" {
		t.Errorf("RefreshToken = %q, want %q", creds.RefreshToken, "env-token-no-refresh")
	}
	if creds.IsExpired() {
		t.Error("env token credentials should not be expired")
	}
	if !creds.IsValid() {
		t.Error("env token credentials should be valid")
	}
}

func TestGetValidCredentials_EnvToken(t *testing.T) {
	t.Setenv("INTENTRA_TOKEN", "test-env-token")

	creds, err := GetValidCredentials()
	if err != nil {
		t.Fatalf("GetValidCredentials() error: %v", err)
	}
	if creds == nil {
		t.Fatal("expected non-nil credentials from GetValidCredentials with env token")
	}
	if creds.AccessToken != "test-env-token" {
		t.Errorf("AccessToken = %q, want %q", creds.AccessToken, "test-env-token")
	}
}

func TestReadEncryptedCache_NoFile(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("INTENTRA_CONFIG_DIR", tmpDir)

	creds, err := ReadEncryptedCache()
	if err != nil {
		t.Fatalf("ReadEncryptedCache() unexpected error: %v", err)
	}
	if creds != nil {
		t.Error("ReadEncryptedCache() should return nil when no file exists")
	}
}
