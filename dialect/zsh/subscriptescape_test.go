// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"strings"
	"testing"
)

// A key read as written keeps a backslash before most characters and drops it
// before nine (#5227). Measured 2026-09-30 on zsh 5.9.2 (`-f`, a script file
// under `env -i PATH=/usr/bin:/bin LC_ALL=C`), one key per character, each
// stored as `h[a\Cb]=z` and listed back: `$`, a backquote, `\`, `(`, `)`, `[`,
// `]`, `{` and `}` store as `aCb`, and every other character tried stores as
// `a\Cb`. bash 5.3.20 and ksh93u+ drop every one of them, so on the nine the
// panel agrees, and the rest are this shell's own reading (#1103).
var keyEscapes = []struct {
	c    string
	kept bool
}{
	{"$", false},
	{"`", false},
	{`\`, false},
	{"(", false},
	{")", false},
	{"[", false},
	{"]", false},
	{"{", false},
	{"}", false},
	// The controls: a backslash the same reading keeps, so a shell that
	// dropped every backslash would fail here and not above.
	{`"`, true},
	{"x", true},
	{"*", true},
	{"#", true},
	{"~", true},
	{"!", true},
	{"'", true},
	{" ", true},
	{"=", true},
	{"@", true},
}

// shellQuote writes s as one single-quoted word.
func shellQuote(s string) string { return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'" }

func TestAKeysBackslashIsDroppedBeforeNineCharacters(t *testing.T) {
	dir := t.TempDir()
	for _, e := range keyEscapes {
		want := "a" + e.c + "b"
		if e.kept {
			want = `a\` + e.c + "b"
		}
		t.Run("store "+e.c, func(t *testing.T) {
			src := "typeset -A h\nh[a\\" + e.c + "b]=z\nfor k in \"${(@k)h}\"; do print -rn -- \"[$k]\"; done\n"
			if out, st := runZsh(t, dir, src); out != "["+want+"]" || st != 0 {
				t.Errorf("h[a\\%sb]=z stored %q at %d, want [%s]", e.c, out, st, want)
			}
		})
		// The read, against both spellings planted by a route that is not
		// a subscript, so the two sides cannot agree by sharing a mistake.
		t.Run("read "+e.c, func(t *testing.T) {
			src := "typeset -A h\nh=(" + shellQuote("a"+e.c+"b") + " bare " + shellQuote(`a\`+e.c+"b") + " kept)\n" +
				"v=${h[a\\" + e.c + "b]}\nprint -rn -- \"$v\"\n"
			wantRead := "bare"
			if e.kept {
				wantRead = "kept"
			}
			if out, st := runZsh(t, dir, src); out != wantRead || st != 0 {
				t.Errorf("${h[a\\%sb]} = %q at %d, want %q", e.c, out, st, wantRead)
			}
		})
	}
}

// The round trips the issue was found through: a key written by a route that
// never was a subscript, read back through one. Each of these found nothing
// while store and read both kept the backslash, because the two agreed with
// each other and with no other route.
func TestAKeyStoredElsewhereIsFoundThroughASubscript(t *testing.T) {
	dir := t.TempDir()
	for _, c := range []struct{ name, src, want string }{
		{"the alias table", `alias 'a$b'=z; print -rn -- "${aliases[a\$b]}"`, "z"},
		{"a parameter's value as the key", `typeset -A h; k='a$b'; h[$k]=1; print -rn -- "${h[a\$b]}"`, "1"},
		{"a subscript beside an array literal", `typeset -A h; h=('a$b' L); h[a\$b]+=S; print -rn -- "${#h} ${h[a\$b]}"`, "1 LS"},
		{"a doubled backslash and a dollar", `typeset -A h; h[a\\\$b]=z; for k in "${(@k)h}"; do print -rn -- "[$k]"; done`, `[a\$b]`},
		{"a leading dollar", `typeset -A h; h[\$x]=1; for k in "${(@k)h}"; do print -rn -- "[$k]"; done`, `[$x]`},
	} {
		t.Run(c.name, func(t *testing.T) {
			if out, st := runZsh(t, dir, c.src+"\n"); out != c.want || st != 0 {
				t.Errorf("got %q at %d, want %q", out, st, c.want)
			}
		})
	}
}
