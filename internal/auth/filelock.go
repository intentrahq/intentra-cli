package auth

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"syscall"
	"time"

	"github.com/intentrahq/intentra-cli/internal/config"
)

const (
	lockFileName     = "credentials.lock"
	lockTimeout      = 10 * time.Second
	lockStaleAge     = 30 * time.Second
	lockPollInterval = 50 * time.Millisecond
)

func getLockFile() (string, error) {
	dir, err := config.GetConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, lockFileName), nil
}

func acquireCredentialLock() (func(), error) {
	lockFile, err := getLockFile()
	if err != nil {
		return nil, fmt.Errorf("failed to determine lock path: %w", err)
	}

	if err := config.EnsureDirectories(); err != nil {
		return nil, fmt.Errorf("failed to create config directory: %w", err)
	}

	deadline := time.Now().Add(lockTimeout)

	for time.Now().Before(deadline) {
		if tryCleanStaleLock(lockFile) {
			file, err := os.OpenFile(lockFile, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
			if err == nil {
				pid := os.Getpid()
				if _, err := file.WriteString(fmt.Sprintf("%d\n%d", pid, time.Now().UnixMilli())); err != nil {
					file.Close()
					os.Remove(lockFile)
					return nil, fmt.Errorf("failed to write lock file: %w", err)
				}
				file.Close()

				release := func() {
					if err := os.Remove(lockFile); err != nil && !os.IsNotExist(err) {
						fmt.Fprintf(os.Stderr, "Warning: failed to release lock file %s: %v\n", lockFile, err)
					}
				}
				return release, nil
			}
		}

		time.Sleep(lockPollInterval)
	}

	return nil, fmt.Errorf("timeout acquiring credential lock")
}

func tryCleanStaleLock(lockFile string) bool {
	info, err := os.Stat(lockFile)
	if os.IsNotExist(err) {
		return true
	}
	if err != nil {
		return false
	}

	if time.Since(info.ModTime()) > lockStaleAge {
		os.Remove(lockFile)
		return true
	}

	data, err := os.ReadFile(lockFile)
	if err != nil {
		return false
	}

	lines := string(data)
	var pid int
	var lockTime int64
	_, _ = fmt.Sscanf(lines, "%d\n%d", &pid, &lockTime)

	// Auto-expire locks older than 60 seconds
	if lockTime > 0 && time.Now().UnixMilli()-lockTime > 60000 {
		os.Remove(lockFile)
		return true
	}

	if pid > 0 && !isProcessRunning(pid) {
		os.Remove(lockFile)
		return true
	}

	return false
}

func isProcessRunning(pid int) bool {
	process, err := os.FindProcess(pid)
	if err != nil {
		return false
	}

	err = process.Signal(syscall.Signal(0))
	if err == nil {
		return true
	}
	// Permission denied means the process exists but belongs to another user.
	if errors.Is(err, os.ErrPermission) {
		return true
	}
	return false
}

func WithCredentialLock(fn func() error) error {
	release, err := acquireCredentialLock()
	if err != nil {
		return err
	}
	defer release()

	return fn()
}

