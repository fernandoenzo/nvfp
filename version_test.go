package main

import (
	"bytes"
	"testing"
)

func TestRootCmd_VersionFlag(t *testing.T) {
	for _, arg := range []string{"--version", "-v"} {
		t.Run(arg, func(t *testing.T) {
			defer func() { versionFlag = false }()

			cmd := newRootCmd()
			var buf bytes.Buffer
			cmd.SetOut(&buf)
			cmd.SetArgs([]string{arg})

			if err := cmd.Execute(); err != nil {
				t.Fatalf("Execute(%s) error: %v", arg, err)
			}
			if got := buf.String(); got != versionMessage {
				t.Errorf("%s output = %q, want %q", arg, got, versionMessage)
			}
		})
	}
}
