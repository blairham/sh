// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"path/filepath"
	"strings"
	"testing"
)

// What the nine identity names say about themselves, and what each listing
// writes for them.
//
// The **values** of `$USERNAME` and `$LOGNAME` are deliberately not here: the
// only value this Runner could read is whoever ran the test, and a row that
// passed because it agreed with the maintainer's own login is a row about the
// machine. dialect/zsh/identityvalues_test.go injects a name instead. What is
// asserted here is the type word, the listing row and the presence — none of
// which depends on who is running.
func TestTheNineIdentityNamesDescribeThemselves(t *testing.T) {
	out, st := runZsh(t, t.TempDir(), `
		for n in CPUTYPE MACHTYPE VENDOR OSTYPE ZSH_PATCHLEVEL LOGNAME USERNAME TTY; do
			print -r -- "$n ${(P)+n} ${(tP)n}"
		done
	`)
	if st != 0 {
		t.Fatalf("status %d, output %q", st, out)
	}
	wantWholeLines(t, out,
		"CPUTYPE 1 scalar",
		"MACHTYPE 1 scalar",
		"VENDOR 1 scalar",
		"OSTYPE 1 scalar",
		"ZSH_PATCHLEVEL 1 scalar",
		// The one the shell exports itself, and the one it calls its own.
		// They are next to each other on purpose: three of the four produced
		// names here are *not* the shell's own and this is the one that is,
		// so a rule that made every producer special fails on the line above.
		"LOGNAME 1 scalar-export",
		"USERNAME 1 scalar-special",
		"TTY 1 scalar",
	)
}

// The five stored ones are ordinary scalars a script may write and unset.
func TestTheBuildValuesAreOrdinaryScalars(t *testing.T) {
	out, st := runZsh(t, t.TempDir(), `
		OSTYPE=zz; print -r -- "written=$OSTYPE ${(t)OSTYPE}"
		unset CPUTYPE; print -r -- "unset=${+CPUTYPE}"
	`)
	if st != 0 {
		t.Fatalf("status %d, output %q", st, out)
	}
	wantWholeLines(t, out, "written=zz scalar", "unset=0")
}

// `$ZSH_PATCHLEVEL` and `$ZSH_VERSION` are one claim about this build.
//
// Asserted as a relation rather than as a literal, so that bumping the
// version in one place cannot leave the other behind — which is the failure
// `zshVersion`'s own comment guards against for `--version`.
func TestThePatchLevelIsTheVersionUnderTheReferencesShape(t *testing.T) {
	// The prelude, because `$ZSH_VERSION` is set there: this is a test of the
	// two claims agreeing, so a run where one of them was empty would be
	// comparing against nothing.
	out, st := runZshPrelude(t, t.TempDir(), `print -r -- "$ZSH_PATCHLEVEL|$ZSH_VERSION"`)
	if st != 0 {
		t.Fatalf("status %d, output %q", st, out)
	}
	patch, version, ok := strings.Cut(strings.TrimRight(out, "\n"), "|")
	if !ok || version == "" {
		t.Fatalf("output %q holds no version at all", out)
	}
	if patch != "zsh-"+version {
		t.Errorf("$ZSH_PATCHLEVEL is %q and $ZSH_VERSION is %q, want the one to be `zsh-` and the other", patch, version)
	}
}

// `$TTY` is empty where the shell holds no terminal, which is every case
// here, and it lists as an ordinary empty scalar.
//
// The path a shell *with* a terminal reads is not assertable from a test
// process that has none — which is exactly the shape a check that cannot
// produce a positive takes, so it is not asserted here and is not claimed:
// interp.Runner.TerminalName is where the lookup is, on the package the gate
// already uses to ask a descriptor its name.
func TestTheTerminalNameIsEmptyWithNoTerminal(t *testing.T) {
	out, st := runZsh(t, t.TempDir(), `print -r -- "[$TTY]"; typeset -p TTY`)
	if st != 0 {
		t.Fatalf("status %d, output %q", st, out)
	}
	if out != "[]\ntypeset TTY=''\n" {
		t.Errorf("TTY = %q, want an empty value and an ordinary row", out)
	}
}

// `$ZSH_SCRIPT` is **absent** off a script file rather than empty, and holds
// the path **as the caller wrote it** when there is one.
//
// The distinction is the whole of this one: a name that existed holding
// nothing would make `${ZSH_SCRIPT-nope}` take the wrong arm, and measured on
// zsh 5.9.2 `${+ZSH_SCRIPT}` is 0 under `-c` and on standard input.
func TestTheScriptNameIsAbsentOffAScriptFile(t *testing.T) {
	out, st := runZsh(t, t.TempDir(),
		`print -r -- "there=${+ZSH_SCRIPT} word=[${(t)ZSH_SCRIPT}] default=${ZSH_SCRIPT-nope}"`)
	if st != 0 {
		t.Fatalf("status %d, output %q", st, out)
	}
	wantWholeLines(t, out, "there=0 word=[] default=nope")
}

// And the path it holds is the operand as written, which is the fact a
// resolved path would lose.
func TestTheScriptNameIsThePathAsWritten(t *testing.T) {
	// The path is only a word here: the file is not read, the runner is told
	// what it was called, which is exactly what the front end does.
	path := filepath.Join(t.TempDir(), "s.zsh")
	body := `print -r -- "there=${+ZSH_SCRIPT}"
print -r -- "word=${(t)ZSH_SCRIPT}"
print -r -- "path=$ZSH_SCRIPT"
typeset -p ZSH_SCRIPT
`
	out := runZshScriptFile(t, body, path)
	wantWholeLines(t, out,
		"there=1",
		"word=scalar",
		"path="+path,
		"typeset ZSH_SCRIPT="+path,
	)
}
