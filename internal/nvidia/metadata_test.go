package nvidia

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestUpdateMetadataSHA256(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a test data file
	dataPath := filepath.Join(tmpDir, "fingerprint.db")
	dataContent := []byte("test fingerprint data")
	if err := os.WriteFile(dataPath, dataContent, 0o644); err != nil {
		t.Fatalf("writing test data: %v", err)
	}

	// Create metadata.json
	metadataPath := filepath.Join(tmpDir, "metadata.json")
	meta := MetadataJSON{
		Files: map[string]FileInfo{
			"other.db": {SHA256: "abc123", Size: 100},
		},
	}
	metaData, _ := json.MarshalIndent(meta, "", "  ")
	if err := os.WriteFile(metadataPath, metaData, 0o644); err != nil {
		t.Fatalf("writing metadata: %v", err)
	}

	// Update SHA256
	if err := UpdateMetadataSHA256(metadataPath, dataPath); err != nil {
		t.Fatalf("UpdateMetadataSHA256 failed: %v", err)
	}

	// Verify
	loaded, err := loadMetadata(metadataPath)
	if err != nil {
		t.Fatalf("loadMetadata failed: %v", err)
	}

	fp, ok := loaded.Files["fingerprint.db"]
	if !ok {
		t.Fatal("fingerprint.db not found in metadata")
	}
	if fp.Size != int64(len(dataContent)) {
		t.Errorf("expected size %d, got %d", len(dataContent), fp.Size)
	}
	if fp.SHA256 == "" {
		t.Error("SHA256 should not be empty")
	}

	// Check that existing entry is preserved
	other, ok := loaded.Files["other.db"]
	if !ok {
		t.Error("other.db should still be in metadata")
	} else if other.SHA256 != "abc123" {
		t.Errorf("other.db SHA256 changed: got %s, want abc123", other.SHA256)
	}
}

func TestUpdateMetadataSHA256NewFile(t *testing.T) {
	tmpDir := t.TempDir()

	dataPath := filepath.Join(tmpDir, "fingerprint.db")
	dataContent := []byte("test data")
	if err := os.WriteFile(dataPath, dataContent, 0o644); err != nil {
		t.Fatalf("writing test data: %v", err)
	}

	metadataPath := filepath.Join(tmpDir, "metadata.json")

	// metadata.json doesn't exist yet — should be created
	if err := UpdateMetadataSHA256(metadataPath, dataPath); err != nil {
		t.Fatalf("UpdateMetadataSHA256 failed: %v", err)
	}

	if _, err := os.Stat(metadataPath); err != nil {
		t.Fatalf("metadata.json not created: %v", err)
	}

	loaded, err := loadMetadata(metadataPath)
	if err != nil {
		t.Fatalf("loadMetadata failed: %v", err)
	}

	fp, ok := loaded.Files["fingerprint.db"]
	if !ok {
		t.Fatal("fingerprint.db not found in metadata")
	}
	if fp.Size != int64(len(dataContent)) {
		t.Errorf("expected size %d, got %d", len(dataContent), fp.Size)
	}
}