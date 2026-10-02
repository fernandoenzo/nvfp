package nvdr

import (
	"strings"
	"testing"
)

func TestToUTF16(t *testing.T) {
	tests := []struct {
		name string
		size int
		in   string
		want []uint16 // written prefix, without the terminating NUL
	}{
		{
			name: "plain string",
			size: 8,
			in:   "abc",
			want: []uint16{'a', 'b', 'c'},
		},
		{
			name: "empty string writes only the terminator",
			size: 4,
			in:   "",
			want: nil,
		},
		{
			name: "fits exactly at the limit",
			size: 4,
			in:   "abc",
			want: []uint16{'a', 'b', 'c'},
		},
		{
			name: "2047 units fit in an NVAPI string",
			size: 2048,
			in:   strings.Repeat("a", 2047),
			want: nil, // checked separately: the prefix is too long to list here
		},
		{
			name: "surrogate pair counts as two units",
			size: 3,
			in:   "\U0001F600",
			want: []uint16{0xD83D, 0xDE00},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// dst is deliberately zeroed here to keep the case independent of
			// the terminator; toUTF16 writes it either way.
			dst := make([]uint16, tt.size)
			if err := toUTF16(dst, tt.in); err != nil {
				t.Fatalf("toUTF16(%q) into %d units: %v", tt.in, tt.size, err)
			}
			if got := fromUTF16(dst); got != tt.in {
				t.Errorf("round trip = %q, want %q", got, tt.in)
			}
			if tt.want != nil {
				for i, want := range tt.want {
					if dst[i] != want {
						t.Errorf("dst[%d] = %#04x, want %#04x", i, dst[i], want)
					}
				}
			}
		})
	}

	t.Run("writes the terminator", func(t *testing.T) {
		dst := []uint16{0xFFFF, 0xFFFF, 0xFFFF}
		if err := toUTF16(dst, "a"); err != nil {
			t.Fatal(err)
		}
		if dst[0] != 'a' || dst[1] != 0 || dst[2] != 0xFFFF {
			t.Errorf("dst = %v, want [a 0 ffff]", dst)
		}
	})

	t.Run("terminates a reused buffer that was not zeroed", func(t *testing.T) {
		// The old contract required the caller to zero dst. A recycled scratch
		// buffer full of leftovers used to leak into the string.
		dst := []uint16{'X', 'Y', 'Z', 'W', 'V', 'U', 'T', 'S'}
		if err := toUTF16(dst, "abc"); err != nil {
			t.Fatal(err)
		}
		if got := fromUTF16(dst); got != "abc" {
			t.Errorf("fromUTF16 = %q, want %q", got, "abc")
		}
	})

	failures := []struct {
		name string
		size int
		in   string
	}{
		{name: "one past the limit", size: 3, in: "abc"},
		{name: "2048 units do not fit in an NVAPI string", size: 2048, in: strings.Repeat("a", 2048)},
		{name: "surrogate pair one past the limit", size: 2, in: "\U0001F600"},
	}
	for _, tt := range failures {
		t.Run(tt.name, func(t *testing.T) {
			dst := make([]uint16, tt.size)
			for i := range dst {
				dst[i] = 0xFFFF
			}
			err := toUTF16(dst, tt.in)
			if err == nil {
				t.Fatalf("toUTF16(...) into %d units: no error", tt.size)
			}
			if !strings.Contains(err.Error(), "does not fit") {
				t.Errorf("unexpected error: %v", err)
			}
			for i := range dst {
				if dst[i] != 0xFFFF {
					t.Fatalf("truncated write into dst: %v", dst)
				}
			}
		})
	}
}

func TestFromUTF16(t *testing.T) {
	tests := []struct {
		name string
		in   []uint16
		want string
	}{
		{
			name: "stops at the first NUL",
			in:   []uint16{'a', 'b', 0, 'c'},
			want: "ab",
		},
		{
			name: "consumes a full buffer without NUL",
			in:   []uint16{'a', 'b', 'c'},
			want: "abc",
		},
		{
			name: "empty buffer",
			in:   nil,
			want: "",
		},
		{
			name: "leading NUL",
			in:   []uint16{0, 'a'},
			want: "",
		},
		{
			name: "decodes surrogate pairs",
			in:   []uint16{0xD83D, 0xDE00, 0},
			want: "\U0001F600",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := fromUTF16(tt.in); got != tt.want {
				t.Errorf("fromUTF16(%v) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestCString(t *testing.T) {
	tests := []struct {
		name string
		in   []byte
		want string
	}{
		{
			name: "stops at the first NUL",
			in:   []byte("ab\x00cd"),
			want: "ab",
		},
		{
			name: "consumes a full buffer without NUL",
			in:   []byte("abc"),
			want: "abc",
		},
		{
			name: "empty buffer",
			in:   nil,
			want: "",
		},
		{
			name: "leading NUL",
			in:   []byte{0, 'a'},
			want: "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := cString(tt.in); got != tt.want {
				t.Errorf("cString(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}
