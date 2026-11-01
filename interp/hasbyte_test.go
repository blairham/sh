// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"strings"
	"testing"
)

// TestHasByteAnswersAsIndexByteDoes: hasByte's loop and the vector search it
// stands in front of give one answer, on both sides of the length where it
// hands over.
func TestHasByteAnswersAsIndexByteDoes(t *testing.T) {
	t.Parallel()
	for _, s := range []string{
		"", ".", "a", "a.b", "abc", "[", "a[1]", "]", "0123456789abcdef", "0123456789abcde.",
		"0123456789abcdefg", "0123456789abcdef.", strings.Repeat("x", 40) + ".", strings.Repeat("y", 40),
	} {
		for _, c := range []byte{'.', '[', ']', 'x'} {
			if got, want := hasByte(s, c), strings.IndexByte(s, c) >= 0; got != want {
				t.Errorf("hasByte(%q, %q) = %v, want %v", s, c, got, want)
			}
		}
	}
}
