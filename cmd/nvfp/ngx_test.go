package main

import (
	"strings"
	"testing"
)

// TestNGXRegistryLayout pins that the leaf passed to CreateKey has no separator,
// so no intermediate key can be invented.
func TestNGXRegistryLayout(t *testing.T) {
	if strings.ContainsAny(ngxKeyName, `\/`) {
		t.Errorf("ngxKeyName = %q carries a separator: CreateKey would create intermediate keys", ngxKeyName)
	}
	if ngxRegKey != nvidiaRegGlobal+`\`+ngxKeyName {
		t.Errorf("ngxRegKey = %q, want it built from %q and %q", ngxRegKey, nvidiaRegGlobal, ngxKeyName)
	}
	if !strings.HasPrefix(ngxRegKey, `SOFTWARE\NVIDIA Corporation\`) {
		t.Errorf("ngxRegKey = %q, want it under SOFTWARE\\NVIDIA Corporation", ngxRegKey)
	}
}
