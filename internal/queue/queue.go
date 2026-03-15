// Package queue provides an encrypted offline scan queue for Intentra.
// When scans cannot be sent (no auth, network failure), they are encrypted
// with AES-256-GCM and persisted to ~/.intentra/queue/. On login or next
// successful send, queued scans are flushed upstream automatically.
package queue

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/atbabers/intentra-cli/internal/auth"
	"github.com/atbabers/intentra-cli/internal/config"
	"github.com/atbabers/intentra-cli/internal/debug"
	"github.com/atbabers/intentra-cli/pkg/models"
)

const (
	maxQueueSize   = 500
	maxAgeHours    = 72
	maxFlushFails  = 10
	fileExtension  = ".scan.enc"
	failsExtension = ".failures"
	queueKeySalt   = "intentra-queue-key-v1"
	queueKeyInfo   = "scan-queue-encryption"
)

var (
	cachedKey    []byte
	cachedKeyErr error
	keyOnce      sync.Once
)

func getQueueKey() ([]byte, error) {
	keyOnce.Do(func() {
		cachedKey, cachedKeyErr = auth.DeriveKey(queueKeySalt, queueKeyInfo)
	})
	return cachedKey, cachedKeyErr
}

// Enqueue encrypts and persists a scan to the offline queue.
func Enqueue(scan *models.Scan) error {
	dir, err := queueDir()
	if err != nil {
		return fmt.Errorf("failed to get queue dir: %w", err)
	}

	if err := enforceQueueLimits(dir); err != nil {
		debug.Warn("queue limit enforcement failed: %v", err)
	}

	plaintext, err := json.Marshal(scan)
	if err != nil {
		return fmt.Errorf("failed to marshal scan: %w", err)
	}

	key, err := getQueueKey()
	if err != nil {
		return fmt.Errorf("failed to derive encryption key: %w", err)
	}

	ciphertext, err := auth.Encrypt(plaintext, key)
	if err != nil {
		return fmt.Errorf("failed to encrypt scan: %w", err)
	}

	filename := scan.ID + fileExtension
	path := filepath.Join(dir, filename)
	tmpPath := path + ".tmp"

	if err := os.WriteFile(tmpPath, ciphertext, 0600); err != nil {
		return fmt.Errorf("failed to write queued scan: %w", err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("failed to finalize queued scan: %w", err)
	}

	debug.Log("Queued scan offline: %s", scan.ID)
	return nil
}

// DequeueAll reads and decrypts all queued scans, returning them with their file paths.
// Caller is responsible for calling Remove after successful send.
func DequeueAll() ([]QueuedScan, error) {
	dir, err := queueDir()
	if err != nil {
		return nil, err
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to read queue dir: %w", err)
	}

	key, err := getQueueKey()
	if err != nil {
		return nil, fmt.Errorf("failed to derive decryption key: %w", err)
	}

	var result []QueuedScan
	cutoff := time.Now().Add(-time.Duration(maxAgeHours) * time.Hour)

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), fileExtension) {
			continue
		}

		path := filepath.Join(dir, entry.Name())

		info, err := entry.Info()
		if err != nil {
			continue
		}
		if info.ModTime().Before(cutoff) {
			os.Remove(path)
			debug.Log("Removed expired queued scan: %s", entry.Name())
			continue
		}

		ciphertext, err := os.ReadFile(path)
		if err != nil {
			debug.Warn("failed to read queued scan %s: %v", entry.Name(), err)
			continue
		}

		plaintext, err := auth.Decrypt(ciphertext, key)
		if err != nil {
			debug.Warn("failed to decrypt queued scan %s: %v", entry.Name(), err)
			continue
		}

		var scan models.Scan
		if err := json.Unmarshal(plaintext, &scan); err != nil {
			debug.Warn("failed to unmarshal queued scan %s: %v", entry.Name(), err)
			continue
		}

		result = append(result, QueuedScan{Scan: &scan, Path: path})
	}

	return result, nil
}

// Remove deletes a queued scan file and its failure counter after successful send.
func Remove(path string) {
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		debug.Warn("failed to remove queued scan %s: %v", path, err)
	}
	os.Remove(failurePath(path))
}

// PendingCount returns the number of scans waiting in the queue.
func PendingCount() int {
	dir, err := queueDir()
	if err != nil {
		return 0
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return 0
	}
	count := 0
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), fileExtension) {
			count++
		}
	}
	return count
}

// QueuedScan pairs a decrypted scan with its file path for cleanup after send.
type QueuedScan struct {
	Scan *models.Scan
	Path string
}

func queueDir() (string, error) {
	dir, err := config.GetDataDir()
	if err != nil {
		return "", err
	}
	qDir := filepath.Join(dir, "queue")
	if err := os.MkdirAll(qDir, 0700); err != nil {
		return "", err
	}
	return qDir, nil
}

// RecordFailure increments the failure counter for a queued scan.
// Returns true if the scan should be removed (exceeded maxFlushFails).
func RecordFailure(scanPath string) bool {
	fp := failurePath(scanPath)
	count := 0

	data, err := os.ReadFile(fp)
	if err == nil {
		_, _ = fmt.Sscanf(string(data), "%d", &count)
	}

	count++
	_ = os.WriteFile(fp, []byte(fmt.Sprintf("%d", count)), 0600)

	if count >= maxFlushFails {
		Remove(scanPath)
		os.Remove(fp)
		return true
	}
	return false
}

func failurePath(scanPath string) string {
	return strings.TrimSuffix(scanPath, fileExtension) + failsExtension
}

func enforceQueueLimits(dir string) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}

	var scanFiles []os.DirEntry
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), fileExtension) {
			scanFiles = append(scanFiles, e)
		}
	}

	if len(scanFiles) <= maxQueueSize {
		return nil
	}

	// Sort by modification time (oldest first) so we evict the oldest scans
	sort.Slice(scanFiles, func(i, j int) bool {
		infoI, errI := scanFiles[i].Info()
		infoJ, errJ := scanFiles[j].Info()
		if errI != nil || errJ != nil {
			return false
		}
		return infoI.ModTime().Before(infoJ.ModTime())
	})

	for i := 0; i < len(scanFiles)-maxQueueSize; i++ {
		os.Remove(filepath.Join(dir, scanFiles[i].Name()))
	}
	return nil
}

