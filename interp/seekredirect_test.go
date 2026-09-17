// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// `<#((expr))` and `>#((expr))` move where a descriptor next reads or writes
// (#3034). The grammar flag is what says the construct exists, so these name
// the flag and never a shell.

func runSeek(t *testing.T, src string) (string, int) {
	t.Helper()
	return runGrammar(t, src, func(d *syntax.Dialect) { d.SeekRedirect = true }, nil)
}

// The read side, the write side, and an expression that reads a parameter.
func TestSeekRedirectMovesTheDescriptor(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{
			"back to the start and on again",
			`printf abcdefghij > f
exec 3< f
read -n4 v <&3; printf '[%s]' "$v"
exec 3<#((0))
read -n2 v <&3; printf '[%s]' "$v"
exec 3<#((6))
read -n2 v <&3; printf '[%s]\n' "$v"`,
			"[abcd][ab][gh]\n",
		},
		{
			"the offset is an expression over parameters",
			`printf abcdefghij > f
exec 3< f
n=2
exec 3<#((n * 3 + 1))
read -n2 v <&3; printf '[%s]\n' "$v"`,
			"[hi]\n",
		},
		{
			"the write side overwrites in place",
			`printf 0123456789 > g
exec 4<> g
exec 4>#((3))
printf XY >&4
exec 4>&-
cat g; printf '\n'`,
			"012XY56789\n",
		},
		{
			// The position belongs to the open file rather than to the
			// command, so it is not put back when the command ends.
			"a seek on a command outlives it",
			`printf abcdefghij > f
exec 3< f
read -n2 v <&3; printf '[%s]' "$v"
{ read -n2 v <&3; printf '[%s]' "$v"; } 3<#((6))
read -n2 v <&3; printf '[%s]\n' "$v"`,
			"[ab][gh][ij]\n",
		},
		{
			// Past the end is not an error: the read after it finds nothing
			// and reports 1.
			"past the end",
			`printf abcdefghij > f
exec 3< f
exec 3<#((100)); printf 'seek=%s ' "$?"
read -n2 v <&3; printf 'read=%s [%s]\n' "$?" "$v"`,
			"seek=0 read=1 []\n",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := runSeek(t, tc.src)
			if out != tc.want || st != 0 {
				t.Errorf("got %q (status %d), want %q at 0", out, st, tc.want)
			}
		})
	}
}

// CUR and EOF stand for the position and the size, and only inside the
// expression: a parameter of those names keeps its value, and an assignment
// the expression makes still reaches the shell.
func TestSeekRedirectBindsCurAndEof(t *testing.T) {
	const src = `printf abcdefghij > f
exec 3< f
read -n4 v <&3
CUR=99; EOF=99
exec 3<#((CUR))
read -n2 v <&3; printf '[%s]' "$v"
exec 3<#((EOF-3))
read -n2 v <&3; printf '[%s]' "$v"
printf '[%s:%s]' "$CUR" "$EOF"
exec 3<#((zz=6))
read -n2 v <&3; printf '[%s:%s]\n' "$v" "$zz"`
	out, st := runSeek(t, src)
	const want = "[ef][hi][99:99][gh:6]\n"
	if out != want || st != 0 {
		t.Errorf("got %q (status %d), want %q at 0", out, st, want)
	}
}

// What the operators refuse, and that each refusal is its own sentence: a
// number nothing is open at, a stream with no position, and an offset before
// the start.
func TestSeekRedirectRefusals(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{"a descriptor nothing is open at", `exec 6<#((0))`, "6: Bad file descriptor"},
		{"a stream with no position", `printf x | { exec 0<#((0)); }`, "0: not seekable"},
		{
			"an offset before the start",
			"printf abcdefghij > f\nexec 3< f\nexec 3<#((-1))",
			"-1: invalid seek offset",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, _ := runSeek(t, tc.src)
			if !strings.Contains(out, tc.want) {
				t.Errorf("got %q, want it to carry %q", out, tc.want)
			}
		})
	}
}

// The three refusals are three sentences and not one, which a dialect can
// word separately. Read through the vector rather than through any preset.
func TestSeekRefusalsAreSeparateWordings(t *testing.T) {
	setup := func(r *Runner) {
		d := Diagnostics{
			SeekDescriptorNotOpen:   "NOTOPEN %[1]s %[2]s",
			SeekStreamHasNoPosition: "NOPOS %[1]s",
			SeekOffsetRefused:       "BADOFF %[1]s",
		}
		if r.Diagnostics != nil {
			d = *r.Diagnostics
			d.SeekDescriptorNotOpen = "NOTOPEN %[1]s %[2]s"
			d.SeekStreamHasNoPosition = "NOPOS %[1]s"
			d.SeekOffsetRefused = "BADOFF %[1]s"
		}
		r.Diagnostics = &d
	}
	for _, tc := range []struct{ src, want string }{
		{`exec 6<#((0))`, "NOTOPEN 6 Bad file descriptor"},
		{`printf x | { exec 0<#((0)); }`, "NOPOS 0"},
		{"printf abcdefghij > f\nexec 3< f\nexec 3<#((-1))", "BADOFF -1"},
	} {
		out, _ := runGrammar(t, tc.src, func(d *syntax.Dialect) { d.SeekRedirect = true }, setup)
		if !strings.Contains(out, tc.want) {
			t.Errorf("%q: got %q, want it to carry %q", tc.src, out, tc.want)
		}
	}
}
