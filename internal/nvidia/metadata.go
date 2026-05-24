package nvidia

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// MetadataJSON represents the structure of a DAO metadata.json file.
type MetadataJSON struct {
	Files map[string]FileInfo `json:"files"`
}

// FileInfo represents a file entry in metadata.json.
type FileInfo struct {
	SHA256 string `json:"sha256"`
	Size   int64  `json:"size"`
}

// UpdateMetadataSHA256 computes the SHA256 hash of the given file and updates
// the metadata.json with the new hash and file size.
func UpdateMetadataSHA256(metadataPath string, dataPath string) error {
	hashStr, size, err := computeFileHash(dataPath)
	if err != nil {
		return err
	}

	meta, err := loadMetadata(metadataPath)
	if err != nil {
		return fmt.Errorf("loading metadata: %w", err)
	}

	filename := filepath.Base(dataPath)
	if meta.Files == nil {
		meta.Files = make(map[string]FileInfo)
	}
	meta.Files[filename] = FileInfo{
		SHA256: hashStr,
		Size:   size,
	}
	return saveMetadata(metadataPath, meta)
}

// computeFileHash returns the SHA256 hex string and file size for the given path.
func computeFileHash(path string) (string, int64, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", 0, fmt.Errorf("reading %s: %w", path, err)
	}
	hash := sha256.Sum256(data)
	info, err := os.Stat(path)
	if err != nil {
		return "", 0, fmt.Errorf("stat %s: %w", path, err)
	}
	return hex.EncodeToString(hash[:]), info.Size(), nil
}

func loadMetadata(path string) (*MetadataJSON, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &MetadataJSON{Files: make(map[string]FileInfo)}, nil
		}
		return nil, err
	}
	var meta MetadataJSON
	if err := json.Unmarshal(data, &meta); err != nil {
		return nil, err
	}
	if meta.Files == nil {
		meta.Files = make(map[string]FileInfo)
	}
	return &meta, nil
}

func saveMetadata(path string, meta *MetadataJSON) error {
	data, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}