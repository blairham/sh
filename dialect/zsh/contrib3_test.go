// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// zcalc, zmathfuncdef and promptinit against rows measured from zsh 5.9.2
// with its own copies; the header of each testdata file says how. Nothing
// here reads zsh's function files: the probe called them.

func TestZcalcAnswersWhatZshAnswers(t *testing.T) {
	// One directory for every row: a subtest's own would be named after the
	// row, and a row is shell. HOME is where nothing is, as it was measured.
	dir := t.TempDir()
	for _, row := range contribRows(t, "zcalc.tsv") {
		t.Run(row[0], func(t *testing.T) {
			out, _ := runZshOnPath(t, dir, "HOME=/nonexistent/home\nfpath=("+shippedFunctionDir(t)+")\n"+
				"autoload -Uz zcalc\nzmodload zsh/mathfunc\n"+row[0]+"\nprint -r -- st=$?\n")
			if got := strings.ReplaceAll(out, "\n", "~"); got != row[1] {
				t.Errorf("output %q, want %q", got, row[1])
			}
		})
	}
}

// How many arguments a body takes, read off `functions -M`, and what a call
// with four of them gives. Measured 2026-10-04 against zsh 5.9.2 with its own
// zmathfuncdef, under -f with standard input on the null device.
func TestZmathfuncdefCountsTheArgumentsZshCounts(t *testing.T) {
	const wrong = "wrong number of arguments: f(1,2,3,4)"
	for _, tc := range []struct{ body, counts, call string }{
		{`$1*$1`, "1 1", wrong},
		{`$3`, "0 0", wrong},
		{`$1+$2`, "2 2", wrong},
		{`${1:-5}*2`, "0 1", wrong},
		{`$1+${2:-3}`, "1 2", wrong},
		{`$1+${3:-1}`, "1 1", wrong},
		{`$1+$2+${3:-0}+${4:-0}`, "2 4", "10"},
		{`$2+$1`, "2 2", wrong},
		{`${1}`, "1 1", wrong},
		{`$#`, "0 0", wrong},
		{`$1$2`, "2 2", wrong},
		{`PI`, "0 0", wrong},
		{`$10`, "0 0", wrong},
		{`$12+$2`, "0 0", wrong},
		{`${1:-2}+$2`, "0 1", wrong},
		{`$1+${2:-1}+$3`, "1 2", wrong},
		{`${1}+$10`, "1 1", wrong},
	} {
		t.Run(tc.body, func(t *testing.T) {
			out, _ := runZshOnPath(t, t.TempDir(), "fpath=("+shippedFunctionDir(t)+")\n"+
				"autoload -Uz zmathfuncdef\nzmathfuncdef f '"+tc.body+"'\nfunctions -M\nprint -r -- $(( f(1,2,3,4) ))\n")
			lines := strings.SplitN(out, "\n", 2)
			if want := "functions -M f " + tc.counts + " zsh_math_func_f"; lines[0] != want {
				t.Errorf("declared %q, want %q", lines[0], want)
			}
			if len(lines) < 2 || !strings.Contains(lines[1], tc.call) {
				t.Errorf("then %q, want it to hold %q", out, tc.call)
			}
		})
	}
}

// Defining, calling, removing, and the refusals. The removal of a function
// that is not there is unfunction's own complaint, at status 0, as in zsh;
// the usage is this file's own words, so only its stream and status are
// held to zsh's.
func TestZmathfuncdefDefinesAndRemoves(t *testing.T) {
	out, _ := runZshOnPath(t, t.TempDir(), "fpath=("+shippedFunctionDir(t)+")\n"+
		"autoload -Uz zmathfuncdef\n"+
		"zmathfuncdef sq '$1*$1'; print -r -- $(( sq(4) ))\n"+
		"zmathfuncdef sq; print -r -- st=$? ${+functions[zsh_math_func_sq]}\n"+
		"functions -M\n"+
		"zmathfuncdef 2bad 1 2>/dev/null; print -r -- st=$?\n"+
		"zmathfuncdef a b c 2>/dev/null; print -r -- st=$?\n"+
		"zmathfuncdef a b c 2>&1 >/dev/null | wc -l | tr -d ' '\n")
	if want := "16\nst=0 0\nst=1\nst=1\n1\n"; out != want {
		t.Errorf("got %q, want %q", out, want)
	}
}

// With no arguments every math function is listed in a form that defines it
// again. zsh's own lists nothing (measured: two functions defined, nothing
// printed), so this follows the manual, on the is-at-least precedent: the
// listing, run in a fresh shell, gives back the same functions.
func TestZmathfuncdefListsInAFormThatRestores(t *testing.T) {
	dir := t.TempDir()
	listing, _ := runZshOnPath(t, dir, "fpath=("+shippedFunctionDir(t)+")\n"+
		"autoload -Uz zmathfuncdef\nzmathfuncdef sq '$1*$1'\nzmathfuncdef add '$1+${2:-10}'\nzmathfuncdef\n")
	if err := os.WriteFile(filepath.Join(dir, "restore"), []byte(listing), 0o644); err != nil {
		t.Fatal(err)
	}
	out, _ := runZshOnPath(t, dir, "source ./restore\nprint -r -- $(( sq(3) )) $(( add(1) )) $(( add(1,2) ))\n")
	if want := "9 11 3\n"; out != want {
		t.Errorf("restored from %q: got %q, want %q", listing, out, want)
	}
}

// promptinit with three themes of the test's own first on $fpath, so every
// row is about the prompt command and none about which themes a copy ships.
func TestPromptinitAnswersWhatZshAnswers(t *testing.T) {
	themes := t.TempDir()
	for name, body := range map[string]string{
		"prompt_tiny_setup": `prompt_tiny_help() { print -r -- "tiny: help text" }
prompt_tiny_setup() {
	PS1="tiny${1:+-$1}> "
	RPS1='[r]'
	prompt_opts=( cr percent subst )
	add-zsh-hook precmd prompt_tiny_precmd
	prompt_cleanup 'print -r -- tiny-cleanup'
}
prompt_tiny_precmd() { : }
prompt_tiny_preview() { print -r -- "tiny preview $*" }
prompt_tiny_setup "$@"
`,
		"prompt_zap_setup": `prompt_zap_setup() {
	PS1='zap> '
	prompt_opts=( bang )
}
prompt_zap_setup "$@"
`,
		"prompt_loud_setup": `print -r -- "file body runs ${funcstack[1]}"
prompt_loud_help() { print -r -- "loud help, PS1=$PS1" }
prompt_loud_setup() {
	print -r -- "setup runs $*"
	PS1='loud> '
	add-zsh-hook precmd prompt_loud_precmd
}
prompt_loud_precmd() { : }
prompt_loud_setup "$@"
`,
	} {
		if err := os.WriteFile(filepath.Join(themes, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	dir := t.TempDir()
	for _, row := range contribRows(t, "promptinit.tsv") {
		t.Run(row[0], func(t *testing.T) {
			out, _ := runZshOnPath(t, dir, "HOME=/nonexistent/home\nTERM=dumb\nfpath=("+themes+" "+shippedFunctionDir(t)+")\n"+
				"autoload -Uz promptinit add-zsh-hook; promptinit\n"+row[0]+"\n")
			if got := strings.ReplaceAll(out, "\n", "~"); got != row[1] {
				t.Errorf("output %q, want %q", got, row[1])
			}
		})
	}
}
