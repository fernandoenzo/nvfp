package nvdr

import (
	"path"
	"strings"

	"github.com/fernandoenzo/set"
)

// profileLookupAttempts returns the strings to try with FindApplicationByName
// for one fingerprint candidate. The driver stores whatever spelling it was
// handed, so the fingerprint's `<DriverProfile>` (which routinely uses Windows
// backslashes, e.g. `End\Binaries\Win64\game.exe`) has to be retried with
// forward slashes, and the bare file name is tried too because some entries are
// registered without their directory. Forward slashes are used on every
// platform: the driver database is the Windows one even when this runs on
// another OS to read it.
func profileLookupAttempts(candidate string) []string {
	slashed := strings.ReplaceAll(candidate, `\`, "/")
	attempts := []string{candidate, slashed}
	if base := path.Base(slashed); base != slashed {
		attempts = append(attempts, base)
	}
	seen := set.New[string](len(attempts) * 2)
	var unique []string
	for _, a := range attempts {
		for _, variant := range []string{a, strings.ToLower(a)} {
			if variant == "" || seen.Contains(variant) {
				continue
			}
			seen.Add(variant)
			unique = append(unique, variant)
		}
	}
	return unique
}
