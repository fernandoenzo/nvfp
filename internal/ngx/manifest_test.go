package ngx

// Tests for the manifest model and its serializer: the only code in the package
// that knows the file format.

import (
	"encoding/binary"
	"strings"
	"testing"
	"unicode/utf16"
)

// realistic is what NVIDIA writes: CRLF, blank lines between sections, several
// sections this package does not manage, and one section we do.
const realistic = "[dlisr]\r\napp_E658703 = 1.0.0\r\n\r\n" +
	"[dlss]\r\napp_E658703 = 310.4.0\r\n\r\n" +
	"[force_add_update]\r\napp_B9D48D0 = dlss\r\napp_E99B5EC = dlss\r\n\r\n" +
	"[sl_sdk_0]\r\napp_E658703 = 2.14.0\r\n\r\n" +
	"[sl_common_0]\r\napp_E658703 = 2.14.0\r\n"

func TestManifestRoundTripIsByteIdentical(t *testing.T) {
	doc, err := parseManifest([]byte(realistic))
	if err != nil {
		t.Fatalf("parseManifest: %v", err)
	}
	if got := string(doc.bytes()); got != realistic {
		t.Errorf("round-trip is not byte-identical:\n got %q\nwant %q", got, realistic)
	}
}

func TestManifestKeepsOpaqueLinesAndSpacing(t *testing.T) {
	weird := "; a comment\r\n[dlss]\r\nweird_line_without_equals\r\n  indented = yes\r\n\r\n[sl_sdk_0]\r\napp_E658703=2.14.0\r\n"
	doc, err := parseManifest([]byte(weird))
	if err != nil {
		t.Fatalf("parseManifest: %v", err)
	}
	if got := string(doc.bytes()); got != weird {
		t.Errorf("opaque lines not preserved:\n got %q\nwant %q", got, weird)
	}
}

func TestManifestSectionLookup(t *testing.T) {
	doc, _ := parseManifest([]byte(realistic))
	if doc.section("sl_sdk_0") == nil {
		t.Error("sl_sdk_0 not found")
	}
	if doc.section("nope") != nil {
		t.Error("a missing section must return nil")
	}
	value, ok := doc.section("force_add_update").get("app_E99B5EC")
	if !ok || value != "dlss" {
		t.Errorf("get(app_E99B5EC) = %q,%v want dlss,true", value, ok)
	}
	if _, ok := doc.section("dlss").get("missing_key"); ok {
		t.Error("get of a missing key must report false")
	}
}

func TestManifestSetInPlaceAndAppend(t *testing.T) {
	doc, _ := parseManifest([]byte(realistic))
	sl := doc.section("sl_common_0")
	if !sl.set("app_E658703", "2.14.3") {
		t.Fatal("set should report a change")
	}
	if sl.set("app_E658703", "2.14.3") {
		t.Error("set with the same value must report no change")
	}
	if got := string(doc.bytes()); !strings.Contains(got, "[sl_common_0]\r\napp_E658703 = 2.14.3\r\n") {
		t.Errorf("value not updated in place:\n%s", got)
	}

	// A new section is appended after a blank-line separator, like NVIDIA's.
	added := doc.appendSection("sl_reflex_0")
	added.set("app_E658703", "2.14.3")
	got := string(doc.bytes())
	if !strings.HasSuffix(got, "\r\n\r\n[sl_reflex_0]\r\napp_E658703 = 2.14.3\r\n") {
		t.Errorf("appended section is not separated by a blank line:\n%q", got[len(got)-70:])
	}
}

func TestManifestDecodesEveryToleratedEncoding(t *testing.T) {
	const want = "[dlss]\r\napp_E658703 = 310.4.0\r\n"
	inputs := map[string]string{
		"UTF-8":     "[dlss]\r\napp_E658703 = 310.4.0\r\n",
		"UTF-8 BOM": "\xEF\xBB\xBF[dlss]\r\napp_E658703 = 310.4.0\r\n",
		"LF only":   "[dlss]\napp_E658703 = 310.4.0\n",
		"CR only":   "[dlss]\rapp_E658703 = 310.4.0\r",
		"UTF-16LE":  "\xFF\xFE" + string(encodeUTF16(want, binary.LittleEndian)),
		"UTF-16BE":  "\xFE\xFF" + string(encodeUTF16(want, binary.BigEndian)),
	}
	for name, input := range inputs {
		doc, err := parseManifest([]byte(input))
		if err != nil {
			t.Errorf("%s: %v", name, err)
			continue
		}
		if doc.section("dlss") == nil {
			t.Errorf("%s: section not found", name)
		}
		// Whatever went in, exactly one canonical form comes out.
		if got := string(doc.bytes()); got != want {
			t.Errorf("%s: output = %q, want %q", name, got, want)
		}
	}
}

func TestManifestRefusesNULWithoutBOM(t *testing.T) {
	if _, err := parseManifest([]byte("[dlss]\x00\x00garbage")); err == nil {
		t.Error("a NUL-filled file with no BOM must be refused, not guessed")
	}
}

func TestSectionHeaderTolerance(t *testing.T) {
	for line, want := range map[string]string{
		"[dlss]":          "dlss",
		"  [dlss]  ":      "dlss",
		"\uFEFF[dlss]":    "dlss",
		"[force_add_upd]": "force_add_upd",
	} {
		got, ok := sectionHeader(line)
		if !ok || got != want {
			t.Errorf("sectionHeader(%q) = %q,%v want %q", line, got, ok, want)
		}
	}
	for _, line := range []string{"[]", "[a]b", "app_x = [not a section]", "[]]", "no header"} {
		if _, ok := sectionHeader(line); ok {
			t.Errorf("sectionHeader(%q) should not match", line)
		}
	}
}

// encodeUTF16 encodes s as UTF-16 in the given byte order, without a BOM.
func encodeUTF16(s string, order binary.ByteOrder) []byte {
	codes := utf16.Encode([]rune(s))
	out := make([]byte, 0, len(codes)*2)
	for _, code := range codes {
		if order == binary.BigEndian {
			out = binary.BigEndian.AppendUint16(out, code)
			continue
		}
		out = binary.LittleEndian.AppendUint16(out, code)
	}
	return out
}
