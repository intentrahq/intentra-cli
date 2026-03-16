package auth

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/user"
	"path/filepath"

	"github.com/intentrahq/intentra-cli/internal/config"
	"github.com/intentrahq/intentra-cli/internal/device"
	"golang.org/x/crypto/hkdf"
)

const (
	encryptedCacheVersion = 1
	nonceSize             = 12
	keySize               = 32

	// CacheKeySalt and CacheKeyInfo provide domain separation for credential encryption keys.
	CacheKeySalt = "intentra-cache-key-v1"
	CacheKeyInfo = "credential-encryption"
)

func getEncryptedCacheFile() (string, error) {
	dir, err := config.GetConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "credentials.enc"), nil
}

func getCacheKeyFile() (string, error) {
	dir, err := config.GetConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, ".cache-key"), nil
}

func WriteEncryptedCache(creds *Credentials) error {
	if err := config.EnsureDirectories(); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	key, err := GetOrCreateCacheKey()
	if err != nil {
		return fmt.Errorf("failed to get encryption key: %w", err)
	}

	plaintext, err := json.Marshal(creds)
	if err != nil {
		return fmt.Errorf("failed to marshal credentials: %w", err)
	}

	ciphertext, err := Encrypt(plaintext, key)
	if err != nil {
		return fmt.Errorf("failed to encrypt credentials: %w", err)
	}

	data := make([]byte, 1+len(ciphertext))
	data[0] = encryptedCacheVersion
	copy(data[1:], ciphertext)

	cacheFile, err := getEncryptedCacheFile()
	if err != nil {
		return fmt.Errorf("failed to determine cache path: %w", err)
	}
	tempFile := cacheFile + ".tmp"
	if err := os.WriteFile(tempFile, data, 0600); err != nil {
		return fmt.Errorf("failed to write encrypted cache: %w", err)
	}

	if err := os.Rename(tempFile, cacheFile); err != nil {
		os.Remove(tempFile)
		return fmt.Errorf("failed to rename encrypted cache: %w", err)
	}

	return nil
}

func ReadEncryptedCache() (*Credentials, error) {
	cacheFile, err := getEncryptedCacheFile()
	if err != nil {
		return nil, fmt.Errorf("failed to determine cache path: %w", err)
	}
	data, err := os.ReadFile(cacheFile)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to read encrypted cache: %w", err)
	}

	if len(data) < 2 {
		return nil, fmt.Errorf("encrypted cache too short")
	}

	version := data[0]
	if version != encryptedCacheVersion {
		return nil, fmt.Errorf("unsupported encrypted cache version: %d", version)
	}

	key, err := ReadCacheKey()
	if err != nil {
		return nil, fmt.Errorf("failed to read cache key: %w", err)
	}

	plaintext, err := Decrypt(data[1:], key)
	if err != nil {
		return nil, fmt.Errorf("failed to decrypt credentials: %w", err)
	}

	var creds Credentials
	if err := json.Unmarshal(plaintext, &creds); err != nil {
		return nil, fmt.Errorf("failed to unmarshal credentials: %w", err)
	}

	return &creds, nil
}

func DeleteEncryptedCache() error {
	cacheFile, err := getEncryptedCacheFile()
	if err != nil {
		return fmt.Errorf("failed to determine cache path: %w", err)
	}
	if err := os.Remove(cacheFile); err != nil && !os.IsNotExist(err) {
		return err
	}

	keyFile, err := getCacheKeyFile()
	if err != nil {
		return fmt.Errorf("failed to determine key path: %w", err)
	}
	if err := os.Remove(keyFile); err != nil && !os.IsNotExist(err) {
		return err
	}

	return nil
}

// ReadCacheKey reads the encryption key from keyring or derives one from hardware ID.
func ReadCacheKey() ([]byte, error) {
	// Try keyring first (matches GetOrCreateCacheKey write path)
	kr, err := openKeyring()
	if err == nil {
		item, err := kr.Get(cacheKeyKey)
		if err == nil && len(item.Data) == keySize {
			return item.Data, nil
		}
	}

	// Fall back to file
	keyFile, err := getCacheKeyFile()
	if err != nil {
		return nil, fmt.Errorf("failed to determine key path: %w", err)
	}
	key, err := os.ReadFile(keyFile)
	if err != nil {
		if os.IsNotExist(err) {
			return DeriveKey(CacheKeySalt, CacheKeyInfo)
		}
		return nil, err
	}

	if len(key) != keySize {
		return DeriveKey(CacheKeySalt, CacheKeyInfo)
	}

	return key, nil
}

// Encrypt encrypts plaintext with AES-256-GCM using the provided key.
func Encrypt(plaintext, key []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}

	nonce := make([]byte, nonceSize)
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, err
	}

	ciphertext := gcm.Seal(nonce, nonce, plaintext, nil)
	return ciphertext, nil
}

// Decrypt decrypts AES-256-GCM ciphertext using the provided key.
func Decrypt(data, key []byte) ([]byte, error) {
	if len(data) < nonceSize {
		return nil, fmt.Errorf("ciphertext too short")
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}

	nonce := data[:nonceSize]
	ciphertext := data[nonceSize:]

	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, err
	}

	return plaintext, nil
}

func generateRandomKey() ([]byte, error) {
	key := make([]byte, keySize)
	if _, err := io.ReadFull(rand.Reader, key); err != nil {
		return nil, err
	}
	return key, nil
}

// DeriveKey derives an encryption key from machine ID and username using HKDF.
// The salt and info parameters provide domain separation between different uses.
func DeriveKey(salt, info string) ([]byte, error) {
	machineID, err := device.GetRawHardwareID()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: using fallback machine ID for key derivation: %v\n", err)
		machineID = "fallback-machine-id"
	}

	currentUser, err := user.Current()
	username := "unknown"
	if err == nil {
		username = currentUser.Username
	}

	ikm := []byte(machineID + "|" + username)

	hkdfReader := hkdf.New(sha256.New, ikm, []byte(salt), []byte(info))
	key := make([]byte, keySize)
	if _, err := io.ReadFull(hkdfReader, key); err != nil {
		return nil, fmt.Errorf("failed to derive key: %w", err)
	}

	return key, nil
}

