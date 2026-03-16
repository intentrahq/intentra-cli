package scanner

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/intentrahq/intentra-cli/internal/config"
	"github.com/intentrahq/intentra-cli/pkg/models"
)

// validScanIDPattern validates scan IDs to prevent path traversal attacks.
// Only allows alphanumeric characters, underscores, and hyphens.
var validScanIDPattern = regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)

// ErrInvalidScanID is returned when a scan ID contains invalid characters.
var ErrInvalidScanID = errors.New("invalid scan ID: must contain only alphanumeric characters, underscores, and hyphens")

// LoadEvents reads all events from events.jsonl.
func LoadEvents() ([]models.Event, error) {
	eventsFile, err := config.GetEventsFile()
	if err != nil {
		return nil, fmt.Errorf("failed to determine events path: %w", err)
	}

	f, err := os.Open(eventsFile)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	defer f.Close()

	const maxEvents = 10000
	malformedCount := 0
	var events []models.Event
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 64*1024), 10*1024*1024)
	for scanner.Scan() {
		var event models.Event
		if err := json.Unmarshal(scanner.Bytes(), &event); err != nil {
			malformedCount++
			continue
		}
		events = append(events, event)
		if len(events) >= maxEvents {
			break
		}
	}
	if malformedCount > 0 {
		fmt.Fprintf(os.Stderr, "Warning: skipped %d malformed events\n", malformedCount)
	}

	return events, scanner.Err()
}

// validateScanID checks if a scan ID is safe to use in file paths.
func validateScanID(id string) error {
	if id == "" {
		return ErrInvalidScanID
	}
	if len(id) > 128 {
		return errors.New("invalid scan ID: exceeds maximum length of 128 characters")
	}
	if !validScanIDPattern.MatchString(id) {
		return ErrInvalidScanID
	}
	return nil
}

// SaveScan writes a scan to the scans directory.
func SaveScan(scan *models.Scan) error {
	if err := validateScanID(scan.ID); err != nil {
		return err
	}

	scansDir, err := config.GetScansDir()
	if err != nil {
		return fmt.Errorf("failed to determine scans path: %w", err)
	}
	if err := os.MkdirAll(scansDir, 0700); err != nil {
		return err
	}

	filename := filepath.Join(scansDir, scan.ID+".json")
	data, err := json.MarshalIndent(scan, "", "  ")
	if err != nil {
		return err
	}

	// Use 0600 for user-only read/write
	return os.WriteFile(filename, data, 0600)
}

// LoadScans reads all scans from the scans directory.
func LoadScans() ([]models.Scan, error) {
	scansDir, err := config.GetScansDir()
	if err != nil {
		return nil, fmt.Errorf("failed to determine scans path: %w", err)
	}

	entries, err := os.ReadDir(scansDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var scans []models.Scan
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}

		data, err := os.ReadFile(filepath.Join(scansDir, entry.Name()))
		if err != nil {
			continue
		}

		var scan models.Scan
		if err := json.Unmarshal(data, &scan); err != nil {
			continue
		}
		scans = append(scans, scan)
	}

	return scans, nil
}

// LoadScan reads a single scan by ID.
func LoadScan(id string) (*models.Scan, error) {
	if err := validateScanID(id); err != nil {
		return nil, err
	}

	scansDir, err := config.GetScansDir()
	if err != nil {
		return nil, fmt.Errorf("failed to determine scans path: %w", err)
	}
	filename := filepath.Join(scansDir, id+".json")

	absFilename, err := filepath.Abs(filename)
	if err != nil {
		return nil, err
	}
	absScansDir, err := filepath.Abs(scansDir)
	if err != nil {
		return nil, err
	}
	relPath, err := filepath.Rel(absScansDir, absFilename)
	if err != nil || strings.HasPrefix(relPath, "..") {
		return nil, ErrInvalidScanID
	}

	data, err := os.ReadFile(filename)
	if err != nil {
		return nil, err
	}

	var scan models.Scan
	if err := json.Unmarshal(data, &scan); err != nil {
		return nil, err
	}

	return &scan, nil
}

// DeleteScan removes a scan file by ID.
func DeleteScan(id string) error {
	if err := validateScanID(id); err != nil {
		return err
	}

	scansDir, err := config.GetScansDir()
	if err != nil {
		return fmt.Errorf("failed to determine scans path: %w", err)
	}
	filename := filepath.Join(scansDir, id+".json")

	absFilename, err := filepath.Abs(filename)
	if err != nil {
		return err
	}
	absScansDir, err := filepath.Abs(scansDir)
	if err != nil {
		return err
	}
	relPath, err := filepath.Rel(absScansDir, absFilename)
	if err != nil || strings.HasPrefix(relPath, "..") {
		return ErrInvalidScanID
	}

	err = os.Remove(filename)
	if err != nil && !os.IsNotExist(err) {
		return err
	}

	return nil
}
