package main

import (
	"fmt"
	"os"
	"path/filepath"
)

func getCacheDir() (string, error) {
	// On Windows: %LOCALAPPDATA%\nvfp (cached data stays on the machine)
	localAppData := os.Getenv("LOCALAPPDATA")
	if localAppData != "" {
		return filepath.Join(localAppData, "nvfp"), nil
	}
	// Fallback for non-Windows (testing)
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".cache", "nvfp"), nil
}

// getNvidiaAppDir returns the NVIDIA App backend directory.
func getNvidiaAppDir() (string, error) {
	localAppData := os.Getenv("LOCALAPPDATA")
	if localAppData == "" {
		return "", fmt.Errorf("LOCALAPPDATA not set")
	}
	dir := filepath.Join(localAppData, "NVIDIA Corporation", "NVIDIA App", "NvBackend")
	if _, err := os.Stat(dir); err != nil {
		return "", fmt.Errorf("NVIDIA App dir not found (is NVIDIA App installed?): %w", err)
	}
	return dir, nil
}

// getFingerprintDBPath returns the path of the working fingerprint.db without
// requiring the file to exist, so a restore can recreate it.
func getFingerprintDBPath() (string, error) {
	nvidiaAppDir, err := getNvidiaAppDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(nvidiaAppDir,
		"ApplicationOntology", "data", "fingerprint.db"), nil
}

// findFingerprintDB returns the path of the working fingerprint.db used by the
// NVIDIA App ontology engine.
func findFingerprintDB() (string, error) {
	path, err := getFingerprintDBPath()
	if err != nil {
		return "", err
	}
	if _, err := os.Stat(path); err != nil {
		return "", fmt.Errorf("fingerprint.db not found (is NVIDIA App installed?): %w", err)
	}
	return path, nil
}

// findDAOFingerprintDB returns the path of the pristine fingerprint.db stored
// under the NVIDIA App DAO directory.
func findDAOFingerprintDB() (string, error) {
	nvidiaAppDir, err := getNvidiaAppDir()
	if err != nil {
		return "", err
	}
	daoDir := filepath.Join(nvidiaAppDir, "DAO")
	entries, err := os.ReadDir(daoDir)
	if err != nil {
		return "", fmt.Errorf("reading DAO directory (is NVIDIA App installed?): %w", err)
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		dbPath := filepath.Join(daoDir, entry.Name(), "fingerprint.db")
		if _, err := os.Stat(dbPath); err == nil {
			return dbPath, nil
		}
	}
	return "", fmt.Errorf("fingerprint.db not found under %s", daoDir)
}
