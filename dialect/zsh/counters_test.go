// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The three counters, and what each says about itself.
//
// `ARGC` is in every row as the control, in the same run: it is the frozen
// produced name these three are modeled on, so a change that moved every one
// of them — or none — moves both columns together, and a listing that broke
// altogether fails the control rather than passing this against an empty
// haystack.
func TestTheThreeCountersAreFrozenProducedIntegers(t *testing.T) {
	out, st := runZsh(t, t.TempDir(), `
		for n in HISTCMD ZSH_SUBSHELL TTYIDLE ARGC; do
			print -r -- "$n ${(P)+n} ${(tP)n} ${(P)n}"
		done
	`)
	if st != 0 {
		t.Fatalf("status %d, output %q", st, out)
	}
	wantWholeLines(t, out,
		"HISTCMD 1 integer-readonly-special 0",
		"ZSH_SUBSHELL 1 integer-readonly-special 0",
		// -1 because a test process has no terminal, which is also the value
		// every script in a pipeline reads. See the interp test that points
		// the arithmetic at a file with a known access time, which is where
		// a positive is watched to fire.
		"TTYIDLE 1 integer-readonly-special -1",
		"ARGC 1 integer-readonly-special 0",
	)
}

// The four listing forms, which the three answer exactly as `$ARGC` does.
func TestTheThreeCountersAnswerTheFourListingForms(t *testing.T) {
	for _, name := range []string{"HISTCMD", "ZSH_SUBSHELL", "TTYIDLE", "ARGC"} {
		t.Run(name, func(t *testing.T) {
			// `typeset -p NAME` writes nothing, at 0.
			out, st := runZsh(t, t.TempDir(), `typeset -p `+name+`; print -r -- "rc=$?"`)
			if out != "rc=0\n" || st != 0 {
				t.Errorf("typeset -p %s = %q (status %d), want no row at 0", name, out, st)
			}
			// And `readonly -p` writes none either.
			out, _ = runZsh(t, t.TempDir(), `readonly -p`)
			for _, line := range strings.Split(out, "\n") {
				if strings.Contains(line, name+"=") {
					t.Errorf("readonly -p wrote %q, want no row for %s", line, name)
				}
			}
			// The two forms that do write the row write it with the letters.
			out, _ = runZsh(t, t.TempDir(), `typeset`)
			if !strings.Contains(out, "integer 10 readonly "+name+"=") {
				t.Errorf("a bare typeset wrote no `integer 10 readonly %s=` row; output %q", name, out)
			}
			out, _ = runZsh(t, t.TempDir(), `readonly`)
			if !strings.Contains(out, name+"=") {
				t.Errorf("a bare readonly wrote no row for %s; output %q", name, out)
			}
			// And an assignment is refused and ends the script.
			out, st = runZsh(t, t.TempDir(), name+`=3; print -r -- unreached`)
			if st == 0 || strings.Contains(out, "unreached") {
				t.Errorf("%s=3 = %q at %d, want a refusal that ends the script", name, out, st)
			}
		})
	}
}

// `$HISTCMD` is the number `%h` draws, from the same place.
//
// One answer under two spellings rather than two counters: the prompt escape
// and the parameter read `fcCurrentEvent`, so a change to the arithmetic
// moves both. Asserted with a list that has something in it — an empty list
// makes both spellings 0 and the row would agree about nothing.
func TestTheHistoryCounterIsTheNumberThePromptDraws(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "h")
	if err := os.WriteFile(path, []byte("one\ntwo\nthree\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	out, st := runZsh(t, dir, `
		print -r -- "empty=$HISTCMD"
		fc -R `+path+`
		print -r -- "loaded=$HISTCMD"
		print -rP -- "escape=%h"
	`)
	if st != 0 {
		t.Fatalf("status %d, output %q", st, out)
	}
	wantWholeLines(t, out, "empty=0", "loaded=3", "escape=3")
}

// `$ZSH_SUBSHELL`, and **the apparatus is in the answer**.
//
// A probe that reads it inside `$( … )` answers 1 because the substitution is
// a subshell, and one that reads it at the top level answers 0 — both
// correct, and a sweep that wrapped every cell in a substitution to keep
// going past a name that might refuse would record the wrong number for this
// name and for nothing else in the sweep. So every row below says which
// apparatus it was taken under, and the top-level row is deliberately not
// wrapped in anything.
//
// The rows are the reference's, measured 2026-09-27 on zsh 5.9.2 under `-f`
// from a script file; [interp.Runner.SubshellDepth] has the grid and
// interp/subshelldepth_test.go grades the constructs.
func TestTheSubshellCounterCountsTheBoundariesTheReferenceCounts(t *testing.T) {
	out, st := runZsh(t, t.TempDir(), `
		print -r -- "toplevel=$ZSH_SUBSHELL"
		( print -r -- "parens=$ZSH_SUBSHELL" )
		( ( print -r -- "nested=$ZSH_SUBSHELL" ) )
		print -r -- "cmdsubst=$( print -r -- $ZSH_SUBSHELL )"
		print -r -- "cmdsubst-parens=$( ( print -r -- $ZSH_SUBSHELL ) )"
		f() { print -r -- "func=$ZSH_SUBSHELL" }
		f
		{ print -r -- "brace=$ZSH_SUBSHELL" }
		eval 'print -r -- "eval=$ZSH_SUBSHELL"'
		( print -r -- "paren-bg=$ZSH_SUBSHELL" ) & wait
		{ ( print -r -- "brace-paren-bg=$ZSH_SUBSHELL" ) } & wait
	`)
	if st != 0 {
		t.Fatalf("status %d, output %q", st, out)
	}
	wantWholeLines(t, out,
		// Taken with no substitution around it, which is the only apparatus
		// that can read the startup value.
		"toplevel=0",
		"parens=1",
		"nested=2",
		// Taken through a command substitution, which is itself the boundary
		// being counted.
		"cmdsubst=1",
		"cmdsubst-parens=2",
		// Three constructs that run in this shell.
		"func=0",
		"brace=0",
		"eval=0",
		// The fork a background job is, which a `( … )` standing as its body
		// does not fork again — and the braces that put one back.
		"paren-bg=1",
		"brace-paren-bg=2",
	)
}
