package auth

import (
	"encoding/json"
	"fmt"
	"os"
	"runtime"
	"sync"
	"time"

	"github.com/99designs/keyring"
	"github.com/atbabers/intentra-cli/internal/config"
)

const (
	serviceName    = "intentra"
	credentialsKey = "credentials"
	cacheKeyKey    = "cache-encryption-key"
)

var (
	ring        keyring.Keyring
	ringOpenErr error
	ringOnce    sync.Once
)

func openKeyring() (keyring.Keyring, error) {
	ringOnce.Do(func() {
		backends := getBackendsForPlatform()

		cfg := keyring.Config{
			ServiceName:                    serviceName,
			KeychainTrustApplication:       true,
			KeychainSynchronizable:         false,
			KeychainAccessibleWhenUnlocked: true,
			FileDir:                        func() string { d, _ := config.GetConfigDir(); return d }(),
			FilePasswordFunc:               filePasswordPrompt,
			AllowedBackends:                backends,
		}

		ring, ringOpenErr = keyring.Open(cfg)
	})
	return ring, ringOpenErr
}

func getBackendsForPlatform() []keyring.BackendType {
	if os.Getenv("INTENTRA_NO_KEYCHAIN") != "" {
		return []keyring.BackendType{keyring.FileBackend}
	}

	switch runtime.GOOS {
	case "darwin":
		return []keyring.BackendType{
			keyring.KeychainBackend,
			keyring.FileBackend,
		}
	case "windows":
		return []keyring.BackendType{
			keyring.WinCredBackend,
			keyring.FileBackend,
		}
	default:
		return []keyring.BackendType{
			keyring.SecretServiceBackend,
			keyring.KWalletBackend,
			keyring.KeyCtlBackend,
			keyring.FileBackend,
		}
	}
}

func filePasswordPrompt(prompt string) (string, error) {
	key, err := DeriveKey(CacheKeySalt, CacheKeyInfo)
	if err != nil {
		return "", fmt.Errorf("failed to derive key for file backend: %w", err)
	}
	return string(key[:16]), nil
}

func StoreCredentialsInKeyring(creds *Credentials) error {
	return WithCredentialLock(func() error {
		return storeCredentialsInKeyringUnlocked(creds)
	})
}

func storeCredentialsInKeyringUnlocked(creds *Credentials) error {
	kr, err := openKeyring()
	if err != nil {
		return fmt.Errorf("failed to open keyring: %w", err)
	}

	data, err := json.Marshal(creds)
	if err != nil {
		return fmt.Errorf("failed to marshal credentials: %w", err)
	}

	err = kr.Set(keyring.Item{
		Key:         credentialsKey,
		Label:       "Intentra Credentials",
		Description: "Intentra CLI Auth Creds",
		Data:        data,
	})
	if err != nil {
		return fmt.Errorf("failed to store credentials in keyring: %w", err)
	}

	if err := WriteEncryptedCache(creds); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to write encrypted cache: %v\n", err)
	}

	return nil
}

func LoadCredentialsFromKeyring() (*Credentials, error) {
	if token := os.Getenv("INTENTRA_TOKEN"); token != "" {
		fmt.Fprintf(os.Stderr, "Warning: using INTENTRA_TOKEN environment variable (bypasses secure storage)\n")
		return &Credentials{
			AccessToken:  token,
			TokenType:    "Bearer",
			ExpiresAt:    time.Date(9999, 12, 31, 23, 59, 59, 0, time.UTC),
			RefreshToken: "env-token-no-refresh",
		}, nil
	}

	kr, err := openKeyring()
	if err != nil {
		return ReadEncryptedCache()
	}

	item, err := kr.Get(credentialsKey)
	if err != nil {
		return ReadEncryptedCache()
	}

	var creds Credentials
	if err := json.Unmarshal(item.Data, &creds); err != nil {
		return nil, fmt.Errorf("failed to unmarshal credentials: %w", err)
	}

	return &creds, nil
}

func DeleteCredentialsFromKeyring() error {
	return WithCredentialLock(func() error {
		return deleteCredentialsFromKeyringUnlocked()
	})
}

func deleteCredentialsFromKeyringUnlocked() error {
	kr, err := openKeyring()
	if err == nil {
		_ = kr.Remove(credentialsKey)
	}

	if err := DeleteEncryptedCache(); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to delete encrypted cache: %v\n", err)
	}

	// Clean up legacy cleartext credentials if they exist
	if credFile, err := config.GetCredentialsFile(); err == nil {
		os.Remove(credFile)
	}

	return nil
}

func GetOrCreateCacheKey() ([]byte, error) {
	kr, err := openKeyring()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: keyring unavailable, using derived key: %v\n", err)
		return DeriveKey(CacheKeySalt, CacheKeyInfo)
	}

	item, err := kr.Get(cacheKeyKey)
	if err == nil && len(item.Data) == keySize {
		return item.Data, nil
	}

	key, err := generateRandomKey()
	if err != nil {
		return DeriveKey(CacheKeySalt, CacheKeyInfo)
	}

	_ = kr.Set(keyring.Item{
		Key:         cacheKeyKey,
		Label:       "Intentra Cache Key",
		Description: "Intentra CLI Auth Creds",
		Data:        key,
	})

	return key, nil
}
