// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package dialect_test

import (
	"strings"
	"testing"

	"github.com/blairham/sh/dialect/bash"
	"github.com/blairham/sh/dialect/dash"
	"github.com/blairham/sh/dialect/ksh"
	"github.com/blairham/sh/dialect/zsh"
	"github.com/blairham/sh/internal/dialecttest"
)

// presets is the four dialects' real vectors, which is what makes the rows
// below evidence: a test inside interp can only build a synthetic Semantics
// and would pass against a live dialect bug.
var presets = map[string]dialecttest.Preset{
	"bash": {Name: "bash", Dialect: bash.Dialect, Semantics: bash.Semantics, Diagnostics: bash.Diagnostics, Apply: bash.Apply},
	"dash": {Name: "dash", Dialect: dash.Dialect, Semantics: dash.Semantics, Diagnostics: dash.Diagnostics, Apply: dash.Apply},
	"ksh":  {Name: "ksh", Dialect: ksh.Dialect, Semantics: ksh.Semantics, Diagnostics: ksh.Diagnostics, Apply: ksh.Apply},
	"zsh":  {Name: "zsh", Dialect: zsh.Dialect, Semantics: zsh.Semantics, Diagnostics: zsh.Diagnostics, Apply: zsh.Apply},
}

// What each dialect says when `read` is given a word that is not a name, whole
// line and status, against the sentence its own binary printed (#1440).
//
// Measured 2026-09-07 with `printf 'X Y Z\n' | { read 1bad; echo "st=$?"; }`:
//
//	bash 5.3.15  bash: line 1: read: `1bad': not a valid identifier   1
//	bash-as-sh   sh: line 1: read: `1bad': not a valid identifier     1
//	bash 3.2.57  bash: line 0: read: `1bad': not a valid identifier   1
//	dash         dash: 1: read: 1bad: bad variable name               2
//	ksh93u+      ksh: read: 1bad: invalid variable name               1
//	zsh 5.9.2    zsh:1: not an identifier: 1bad                       1
//
// Whole lines and not substrings: "not an identifier" is a substring of three
// of these, and a Contains over it cannot see a wording that has grown a
// prefix — which is the half of this bug that cost an hour, the missing
// sentence rather than the missing refusal.
func TestEachDialectRefusesAReadNameInItsOwnWords(t *testing.T) {
	for _, c := range []struct {
		dialect string
		want    string
		status  int
	}{
		{"bash", "bash: line 1: read: `1bad': not a valid identifier\nst=1\n", 0},
		{"dash", "dash: 1: read: 1bad: bad variable name\nst=2\n", 0},
		{"ksh", "ksh: read: 1bad: invalid variable name\nst=1\n", 0},
		// zsh stops, so the `st=` line never runs and the status is the
		// pipeline's own.
		{"zsh", "zsh:1: not an identifier: 1bad\n", 1},
	} {
		t.Run(c.dialect, func(t *testing.T) {
			p := presets[c.dialect]
			out, st, err := p.Combined(t, dialecttest.Base{}, `printf 'X Y Z\n' | { read 1bad; echo "st=$?"; }`)
			if err != nil {
				t.Fatal(err)
			}
			if out != c.want || st != c.status {
				t.Errorf("said %q status %d, want %q at %d", out, st, c.want, c.status)
			}
		})
	}
}

// zsh alone ends the script here. The other three report it and carry on,
// which is what `read` not being a special builtin buys them — dash and ksh93
// stop for `export 1x` and go on past this.
func TestOnlyZshEndsTheScriptOnAReadName(t *testing.T) {
	for _, c := range []struct {
		dialect string
		stops   bool
	}{{"bash", false}, {"dash", false}, {"ksh", false}, {"zsh", true}} {
		t.Run(c.dialect, func(t *testing.T) {
			out, _, err := presets[c.dialect].Combined(t, dialecttest.Base{}, `read 1bad; echo after`)
			if err != nil {
				t.Fatal(err)
			}
			if stopped := !strings.Contains(out, "after"); stopped != c.stops {
				t.Errorf("said %q, want stops=%v", out, c.stops)
			}
		})
	}
}

// Whether the line is read before the operand is judged, which the input is
// the only witness to. bash 5.3 and ksh93 leave the line for the next reader;
// dash and zsh have eaten it.
//
// zsh's answer is measured in a subshell — `printf 'AAA\nBBB\n' | { (read
// 1bad); cat; }` prints only BBB — because the refusal is fatal there and a
// subshell is the only way to have a reader left to ask.
func TestWhichDialectsJudgeAReadNameBeforeReading(t *testing.T) {
	for _, c := range []struct {
		dialect string
		want    string
	}{
		{"bash", "next=[AAA]"},
		{"ksh", "next=[AAA]"},
		{"dash", "next=[BBB]"},
	} {
		t.Run(c.dialect, func(t *testing.T) {
			out, _, err := presets[c.dialect].Combined(t, dialecttest.Base{},
				`printf 'AAA\nBBB\n' | { read 1bad; read next; echo "next=[$next]"; }`)
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(out, c.want) {
				t.Errorf("said %q, want %q", out, c.want)
			}
		})
	}
	out, _, err := presets["zsh"].Combined(t, dialecttest.Base{},
		`printf 'AAA\nBBB\n' | { (read 1bad); read next; echo "next=[$next]"; }`)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "next=[BBB]") {
		t.Errorf("zsh said %q, want next=[BBB]", out)
	}
}

// zsh fills `$1` from `read 1`, where the same shell refuses `export 1` and
// the other three refuse the operand outright.
func TestOnlyZshTakesAPositionalWhereReadWantsAName(t *testing.T) {
	for _, c := range []struct {
		dialect string
		want    string
	}{
		{"bash", "st=1 one=[]"},
		{"dash", "st=2 one=[]"},
		{"ksh", "st=1 one=[]"},
		{"zsh", "st=0 one=[X Y Z]"},
	} {
		t.Run(c.dialect, func(t *testing.T) {
			out, _, err := presets[c.dialect].Combined(t, dialecttest.Base{},
				`printf 'X Y Z\n' | { read 1; echo "st=$? one=[$1]"; }`)
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(out, c.want) {
				t.Errorf("said %q, want %q", out, c.want)
			}
		})
	}
}

// `read "v?Name: "` is ksh93's spelling of `read -p`, and zsh took it. Both
// read into v; bash and dash have no such form and refuse the whole word as a
// name. Where the mark stands alone the two that have the form part company:
// ksh93 refuses the empty name it is left with — naming the empty word — and
// zsh reads into REPLY, which is the shape the idiom is usually written in.
func TestTheReadPromptOperandIsTwoDialectsAndOneOfThemIsStricter(t *testing.T) {
	for _, c := range []struct {
		dialect      string
		named, alone string
	}{
		{"bash", "bash: line 1: read: `v?p': not a valid identifier", "bash: line 1: read: `?p': not a valid identifier"},
		{"dash", "dash: 1: read: v?p: bad variable name", "dash: 1: read: ?p: bad variable name"},
		{"ksh", "st=0 v=[X Y Z]", "ksh: read: : invalid variable name"},
		{"zsh", "st=0 v=[X Y Z]", "st=0 R=[X Y Z]"},
	} {
		t.Run(c.dialect, func(t *testing.T) {
			p := presets[c.dialect]
			out, _, err := p.Combined(t, dialecttest.Base{}, `printf 'X Y Z\n' | { read "v?p"; echo "st=$? v=[$v]"; }`)
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(out, c.named) {
				t.Errorf("named: said %q, want %q", out, c.named)
			}
			out, _, err = p.Combined(t, dialecttest.Base{}, `printf 'X Y Z\n' | { read "?p"; echo "st=$? R=[$REPLY]"; }`)
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(out, c.alone) {
				t.Errorf("alone: said %q, want %q", out, c.alone)
			}
		})
	}
}
