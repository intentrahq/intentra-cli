package scanner

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/intentrahq/intentra-cli/pkg/models"
)

func TestValidateScanID(t *testing.T) {
	tests := []struct {
		name    string
		id      string
		wantErr bool
	}{
		{"valid alphanumeric", "scan-123_abc", false},
		{"valid with hyphens", "a-b-c", false},
		{"empty", "", true},
		{"path traversal", "../../../etc/passwd", true},
		{"slashes", "scans/evil", true},
		{"dots", "scan..id", true},
		{"spaces", "scan id", true},
		{"special chars", "scan;rm -rf /", true},
		{"too long", string(make([]byte, 129)), true},
		{"max length", string(repeatByte('a', 128)), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateScanID(tt.id)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateScanID(%q) error = %v, wantErr %v", tt.id, err, tt.wantErr)
			}
		})
	}
}

func repeatByte(b byte, n int) []byte {
	result := make([]byte, n)
	for i := range result {
		result[i] = b
	}
	return result
}

func TestSaveScan(t *testing.T) {
	tmpDir := t.TempDir()
	os.Setenv("INTENTRA_CONFIG_DIR", tmpDir)
	defer os.Unsetenv("INTENTRA_CONFIG_DIR")

	scan := &models.Scan{
		ID:   "test-scan-001",
		Tool: "cursor",
	}

	if err := SaveScan(scan); err != nil {
		t.Fatalf("SaveScan failed: %v", err)
	}

	filename := filepath.Join(tmpDir, "scans", "test-scan-001.json")
	data, err := os.ReadFile(filename)
	if err != nil {
		t.Fatalf("failed to read saved scan: %v", err)
	}

	var loaded models.Scan
	if err := json.Unmarshal(data, &loaded); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}
	if loaded.ID != "test-scan-001" {
		t.Errorf("ID = %q, want test-scan-001", loaded.ID)
	}
}

func TestSaveScan_InvalidID(t *testing.T) {
	if err := SaveScan(&models.Scan{ID: "../evil"}); err == nil {
		t.Error("expected error for path traversal ID")
	}
}

func TestLoadScan(t *testing.T) {
	tmpDir := t.TempDir()
	os.Setenv("INTENTRA_CONFIG_DIR", tmpDir)
	defer os.Unsetenv("INTENTRA_CONFIG_DIR")

	scansDir := filepath.Join(tmpDir, "scans")
	if err := os.MkdirAll(scansDir, 0700); err != nil {
		t.Fatalf("MkdirAll failed: %v", err)
	}

	scan := models.Scan{ID: "load-test", Tool: "claude"}
	data, _ := json.MarshalIndent(scan, "", "  ")
	if err := os.WriteFile(filepath.Join(scansDir, "load-test.json"), data, 0600); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	loaded, err := LoadScan("load-test")
	if err != nil {
		t.Fatalf("LoadScan failed: %v", err)
	}
	if loaded.Tool != "claude" {
		t.Errorf("Tool = %q, want claude", loaded.Tool)
	}
}

func TestLoadScan_InvalidID(t *testing.T) {
	_, err := LoadScan("../../../etc/passwd")
	if err == nil {
		t.Error("expected error for path traversal")
	}
}

func TestLoadScan_NotFound(t *testing.T) {
	tmpDir := t.TempDir()
	os.Setenv("INTENTRA_CONFIG_DIR", tmpDir)
	defer os.Unsetenv("INTENTRA_CONFIG_DIR")

	if err := os.MkdirAll(filepath.Join(tmpDir, "scans"), 0700); err != nil {
		t.Fatalf("MkdirAll failed: %v", err)
	}

	_, err := LoadScan("nonexistent")
	if err == nil {
		t.Error("expected error for nonexistent scan")
	}
}

func TestDeleteScan(t *testing.T) {
	tmpDir := t.TempDir()
	os.Setenv("INTENTRA_CONFIG_DIR", tmpDir)
	defer os.Unsetenv("INTENTRA_CONFIG_DIR")

	scansDir := filepath.Join(tmpDir, "scans")
	if err := os.MkdirAll(scansDir, 0700); err != nil {
		t.Fatalf("MkdirAll failed: %v", err)
	}
	if err := os.WriteFile(filepath.Join(scansDir, "del-test.json"), []byte("{}"), 0600); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	if err := DeleteScan("del-test"); err != nil {
		t.Fatalf("DeleteScan failed: %v", err)
	}

	if _, err := os.Stat(filepath.Join(scansDir, "del-test.json")); !os.IsNotExist(err) {
		t.Error("file should have been deleted")
	}
}

func TestDeleteScan_InvalidID(t *testing.T) {
	if err := DeleteScan("../evil"); err == nil {
		t.Error("expected error for path traversal")
	}
}

func TestDeleteScan_Nonexistent(t *testing.T) {
	tmpDir := t.TempDir()
	os.Setenv("INTENTRA_CONFIG_DIR", tmpDir)
	defer os.Unsetenv("INTENTRA_CONFIG_DIR")

	if err := os.MkdirAll(filepath.Join(tmpDir, "scans"), 0700); err != nil {
		t.Fatalf("MkdirAll failed: %v", err)
	}

	if err := DeleteScan("does-not-exist"); err != nil {
		t.Errorf("deleting nonexistent scan should not error, got: %v", err)
	}
}

func TestLoadScans(t *testing.T) {
	tmpDir := t.TempDir()
	os.Setenv("INTENTRA_CONFIG_DIR", tmpDir)
	defer os.Unsetenv("INTENTRA_CONFIG_DIR")

	scansDir := filepath.Join(tmpDir, "scans")
	if err := os.MkdirAll(scansDir, 0700); err != nil {
		t.Fatalf("MkdirAll failed: %v", err)
	}

	for _, id := range []string{"scan-a", "scan-b"} {
		data, _ := json.Marshal(models.Scan{ID: id, Tool: "cursor"})
		if err := os.WriteFile(filepath.Join(scansDir, id+".json"), data, 0600); err != nil {
			t.Fatalf("WriteFile failed: %v", err)
		}
	}

	if err := os.WriteFile(filepath.Join(scansDir, "readme.txt"), []byte("not json"), 0600); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	if err := os.WriteFile(filepath.Join(scansDir, "bad.json"), []byte("{invalid"), 0600); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	scans, err := LoadScans()
	if err != nil {
		t.Fatalf("LoadScans failed: %v", err)
	}
	if len(scans) != 2 {
		t.Errorf("expected 2 scans, got %d", len(scans))
	}
}

func TestLoadScans_EmptyDir(t *testing.T) {
	tmpDir := t.TempDir()
	os.Setenv("INTENTRA_CONFIG_DIR", tmpDir)
	defer os.Unsetenv("INTENTRA_CONFIG_DIR")

	scans, err := LoadScans()
	if err != nil {
		t.Fatalf("LoadScans failed: %v", err)
	}
	if scans != nil {
		t.Errorf("expected nil for nonexistent dir, got %v", scans)
	}
}

func TestLoadEvents(t *testing.T) {
	tmpDir := t.TempDir()
	os.Setenv("INTENTRA_CONFIG_DIR", tmpDir)
	defer os.Unsetenv("INTENTRA_CONFIG_DIR")

	eventsFile := filepath.Join(tmpDir, "events.jsonl")
	lines := `{"hook_type":"afterResponse","normalized_type":"after_response","timestamp":"2025-01-01T00:00:00Z","conversation_id":"c1"}
{"hook_type":"afterTool","normalized_type":"after_tool","timestamp":"2025-01-01T00:00:01Z","conversation_id":"c1"}
{malformed json line}
`
	if err := os.WriteFile(eventsFile, []byte(lines), 0600); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	events, err := LoadEvents()
	if err != nil {
		t.Fatalf("LoadEvents failed: %v", err)
	}
	if len(events) != 2 {
		t.Errorf("expected 2 events (1 malformed skipped), got %d", len(events))
	}
}

func TestLoadEvents_NoFile(t *testing.T) {
	tmpDir := t.TempDir()
	os.Setenv("INTENTRA_CONFIG_DIR", tmpDir)
	defer os.Unsetenv("INTENTRA_CONFIG_DIR")

	events, err := LoadEvents()
	if err != nil {
		t.Fatalf("LoadEvents failed: %v", err)
	}
	if events != nil {
		t.Errorf("expected nil for missing file, got %v", events)
	}
}
