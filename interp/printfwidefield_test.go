// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import "testing"

// The zero flag is read from the *flags*, not from the whole spec.
//
// A width is made of digits, so `%10000010d` holds a `0` that is not the zero
// flag. Asking `strings.Contains(spec, "0")` zero-pads a field every column
// pads with spaces — which is exactly what the first attempt at #2663 did, and
// what this test is here to keep from coming back.
//
// In-package because it names an unexported split; the behavior it protects is
// pinned from outside by TestAFieldWiderThanFmtRendersIsLaidOutHere, whose
// first row asserts the padding byte.
func TestAZeroInTheWidthIsNotTheZeroFlag(t *testing.T) {
	for _, tc := range []struct {
		name, spec            string
		wantNarrow, wantFlags string
	}{
		{"a zero inside the width", "%10000010", "%", ""},
		{"a real zero flag", "%010000010", "%0", "0"},
		{"a minus, and a zero in the width", "%-10000010", "%-", "-"},
		{"both flags", "%-010000010", "%-0", "-0"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			narrow, flags, width, wide := printfWideField(tc.spec)
			if !wide {
				t.Fatalf("printfWideField(%q) did not report a wide field", tc.spec)
			}
			if narrow != tc.wantNarrow || flags != tc.wantFlags {
				t.Errorf("narrow=%q flags=%q, want %q and %q",
					narrow, flags, tc.wantNarrow, tc.wantFlags)
			}
			if want := printfFmtWidthCeiling + 1; width != want {
				t.Errorf("width = %d, want %d", width, want)
			}
		})
	}
	// A width fmt will render is left entirely alone — the fast path, and the
	// reason this costs nothing on every ordinary conversion.
	if narrow, flags, width, wide := printfWideField("%20"); wide ||
		narrow != "%20" || flags != "" || width != 0 {
		t.Errorf("an ordinary width was taken as wide: %q %q %d %v", narrow, flags, width, wide)
	}
	// And the ceiling itself is not past the ceiling.
	if _, _, _, wide := printfWideField("%10000009"); wide {
		t.Error("the widest field fmt renders was taken as wide")
	}
}
