package nvidia

import (
	"path/filepath"
	"testing"

	"github.com/fernandoenzo/nvfp/internal/db"
)

// FuzzPatchGameOutputStaysWellFormed checks the invariant every override key and
// removal must satisfy: once PatchGame reports a patch, the written database must
// parse again. It found the U+022B class of names, which encoding/xml writes and
// then refuses to read back, so the seeds keep that case locked down.
func FuzzPatchGameOutputStaysWellFormed(f *testing.F) {
	f.Add("DLSS_Override_No_OPS", "0", "uwp", "PinnedSLVersion")
	f.Add("<evil>", "x", "uwp", "with space")
	f.Add("ns:foo", "y", "*", "")
	f.Add("", "z", "uwp", "")
	f.Add("ȫ", "0", "uwp", "A")
	f.Fuzz(func(t *testing.T, key, value, version, remove string) {
		fdb, err := ParseFingerprintDB(filepath.Join("testdata", "fingerprint.db"))
		if err != nil {
			t.Fatalf("fixture parse: %v", err)
		}
		game := &db.Game{
			Fingerprint:    "already_uwp_game",
			AppUserModelID: "P!A",
			Versions:       []string{version},
			Overrides:      map[string]string{key: value},
			Remove:         []string{remove},
		}
		if PatchGame(fdb, game).Status != StatusPatched {
			return
		}
		out := filepath.Join(t.TempDir(), "out.db")
		if err := WriteFingerprintDB(fdb, out); err != nil {
			t.Fatalf("write after patched status: %v", err)
		}
		if _, err := ParseFingerprintDB(out); err != nil {
			t.Fatalf("patched output does not re-parse: %v (key=%q remove=%q)", err, key, remove)
		}
	})
}
