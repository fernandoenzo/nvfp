package nvdr

import (
	"path"
	"strings"

	"github.com/fernandoenzo/set"
)

// profileLookupAttempts returns the spellings of a candidate to try with
// FindApplicationByName: as written, forward-slashed, and the bare file name,
// each lowercased too.
func profileLookupAttempts(candidate string) []string {
	slashed := strings.ReplaceAll(candidate, `\`, "/")
	attempts := []string{candidate, slashed}
	if base := path.Base(slashed); base != slashed && base != "." && base != "/" {
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
