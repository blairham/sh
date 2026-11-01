// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"strings"
	"testing"
)

// TestSpanTextJoinsAsABuilderDoes holds spanText to the strings.Builder it
// stands in for: the same text and length after every write, whatever mix of
// strings and bytes arrives, including a byte that is not ASCII (#6353).
func TestSpanTextJoinsAsABuilderDoes(t *testing.T) {
	t.Parallel()
	type write struct {
		s    string
		b    byte
		byte bool
	}
	for _, seq := range [][]write{
		{},
		{{s: ""}},
		{{s: "abc"}},
		{{s: "ab"}, {s: "cd"}},
		{{s: "ab"}, {s: ""}, {s: "cd"}, {s: "ef"}},
		{{b: '\\', byte: true}, {s: "x"}},
		{{s: "a"}, {b: 0xe9, byte: true}, {s: "b"}},
		{{b: 0xff, byte: true}},
		{{s: ""}, {b: 'q', byte: true}},
	} {
		var want strings.Builder
		var got spanText
		for _, w := range seq {
			if w.byte {
				want.WriteByte(w.b)
				got.addByte(w.b)
			} else {
				want.WriteString(w.s)
				got.WriteString(w.s)
			}
			if got.String() != want.String() || got.Len() != want.Len() {
				t.Fatalf("%v: got %q (%d), want %q (%d)", seq, got.String(), got.Len(), want.String(), want.Len())
			}
		}
	}
}
