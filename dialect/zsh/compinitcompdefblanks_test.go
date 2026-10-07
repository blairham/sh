// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestCompinitSplitsTheTagLineOnBlanks pins how compinit reads a file's first
// line (#6289): it is split into words as the default IFS splits it, and the
// first word has to be the tag exactly. So a tab separates `#compdef` or
// `#autoload` from what follows as a space does, leading blanks are dropped,
// and the caller's IFS has no say. Measured on zsh 5.9.2 with
// `env -i ... zsh -f`, every row below is what real zsh's compinit leaves,
// including the three that register nothing: a tag with a letter glued to it,
// and a vertical tab or carriage return where a blank would be, neither of
// which the default IFS holds.
func TestCompinitSplitsTheTagLineOnBlanks(t *testing.T) {
	dir := t.TempDir()
	if err := os.Chmod(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	for name, line := range map[string]string{
		"_tab":     "#compdef\tt1\tt2",
		"_multi":   "#compdef\t\tm1 \t m2",
		"_lead":    "  #compdef l1",
		"_leadtab": "\t#compdef l2",
		"_baretab": "#compdef\t",
		"_opt":     "#compdef\t-p\tpp*",
		"_glued":   "#compdefx x1",
		"_vt":      "#compdef\vv1",
		"_cr":      "#compdef\rr1",
		"_atab":    "#autoload\t-z",
		"_alead":   " \t#autoload",
	} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(line+"\nprint in "+name+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	out, st := runShipped(t, `IFS=:
fpath=(`+dir+` $fpath); autoload -Uz compinit; compinit -D -u
IFS=$' \t\n\0'
for k in t1 t2 m1 m2 l1 l2 x1 v1 r1; do print -r -- "$k=${_comps[$k]-unset}"; done
print -r -- "pp*=${_patcomps[pp*]-unset}"
for f in _baretab _glued _vt _cr _atab _alead; do print -r -- "$f=$(whence -w $f)"; done`)
	want := strings.Join([]string{
		"t1=_tab", "t2=_tab", "m1=_multi", "m2=_multi", "l1=_lead", "l2=_leadtab",
		"x1=unset", "v1=unset", "r1=unset",
		"pp*=_opt",
		"_baretab=_baretab: function", "_glued=_glued: none", "_vt=_vt: none",
		"_cr=_cr: none", "_atab=_atab: function", "_alead=_alead: function",
	}, "\n") + "\n"
	if st != 0 || out != want {
		t.Errorf("status %d\ngot:\n%s\nwant:\n%s", st, out, want)
	}
}
