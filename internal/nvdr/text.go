package nvdr

import (
	"fmt"
	"unicode/utf16"
)

// toUTF16 writes s into dst as a NUL-terminated UTF-16 string, failing without
// truncating when the field cannot hold it. The terminator is written here, so
// dst needs no prior zeroing and may be reused across calls.
func toUTF16(dst []uint16, s string) error {
	encoded := utf16.Encode([]rune(s))
	if len(encoded) > len(dst)-1 {
		return fmt.Errorf("%q does not fit in an NVAPI unicode string (%d UTF-16 units)", s, len(dst)-1)
	}
	copy(dst, encoded)
	dst[len(encoded)] = 0
	return nil
}

// fromUTF16 decodes a NUL-terminated UTF-16 string.
func fromUTF16(b []uint16) string {
	for i, unit := range b {
		if unit == 0 {
			b = b[:i]
			break
		}
	}
	return string(utf16.Decode(b))
}

// cString decodes a NUL-terminated byte string.
func cString(b []byte) string {
	for i, c := range b {
		if c == 0 {
			b = b[:i]
			break
		}
	}
	return string(b)
}
