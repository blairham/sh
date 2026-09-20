// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"strings"
	"testing"
)

// A `$( … )` an autoload file never closes is refused by the **load**, so the
// name is left an autoload stub and the script that called it carries on
// (#3895).
//
// A substitution body that does not parse is fatal in this dialect, and this
// engine reads bodies at expansion rather than with the line. In an autoload
// file that cost the whole outer script: the failure surfaced inside the
// running function and the abandonment escaped the call.
//
// Measured 2026-09-20 on zsh 5.9.2, `env -i -u FPATH PATH=/usr/bin:/bin
// LC_ALL=C` over a script file — `-u FPATH` matters, since an inherited one
// replaces the default and the probe then measures the host's zsh:
//
//	./fns/g                     zsh writes                     call
//	v=$(echo hi; for)           BEFORE, parse errors, AFTER    1
//	echo ok / v=$(echo hi; for) BEFORE, parse errors, AFTER    1
//
// `ok` is unwritten in the second, which is what says zsh ran no command of
// the body: the file was refused before the name was ever defined. This shell
// wrote BEFORE and `ok` and then stopped, at status 1.
func TestAnAutoloadFileWithAnUnclosedSubstitutionIsNotAFunction(t *testing.T) {
	call := func(t *testing.T, body string) (string, int) {
		t.Helper()
		fp := fpathDir(t, map[string]string{"g": body})
		return runZsh(t, t.TempDir(), "fpath=("+fp+")\nautoload -Uz g\nprint BEFORE\ng\nprint \"call=$?\"\nprint AFTER\n")
	}

	t.Run("the outer script reaches the line after the call", func(t *testing.T) {
		out, st := call(t, "v=$(echo hi; for)\n")
		if st != 0 {
			t.Errorf("status = %d, want 0 — the script ran to its end", st)
		}
		for _, w := range []string{"BEFORE\n", "call=1\n", "AFTER\n"} {
			if !strings.Contains(out, w) {
				t.Errorf("output = %q, want %q in it", out, w)
			}
		}
	})

	t.Run("no command of the body runs", func(t *testing.T) {
		// The half a boundary at the *call* could not have fixed: zsh parses
		// the file before it defines anything, so the `print ok` in front of
		// the failing line never runs either.
		out, st := call(t, "print ok\nv=$(echo hi; for)\n")
		if strings.Contains(out, "ok\n") {
			t.Errorf("output = %q, want no command of the body to have run", out)
		}
		if st != 0 || !strings.Contains(out, "AFTER\n") {
			t.Errorf("output = %q (status %d), want the script to reach AFTER at 0", out, st)
		}
	})

	t.Run("the name is left an autoload stub", func(t *testing.T) {
		// zsh answers `g is an autoload shell function` after the failed
		// call and reports the same failure on the next one, so the refusal
		// did not define anything.
		fp := fpathDir(t, map[string]string{"g": "v=$(echo hi; for)\n"})
		out, st := runZsh(t, t.TempDir(), "fpath=("+fp+")\nautoload -Uz g\ng\nwhence -v g\nprint AFTER\n")
		if st != 0 || !strings.Contains(out, "g is an autoload shell function") || !strings.Contains(out, "AFTER\n") {
			t.Errorf("output = %q (status %d), want the stub still there and AFTER reached", out, st)
		}
	})

	t.Run("the shapes that swallow the closing parenthesis", func(t *testing.T) {
		// Every row measured on zsh 5.9.2 the same way: BEFORE, the parse
		// errors, `call=1`, AFTER, and `ok` never written. The last is the
		// nesting — it is the shape of each body and not of the outermost.
		for _, body := range []string{
			"v=$(for)", "v=$(echo hi; for)", "v=$(for x in a)",
			"v=$(select)", "v=$(foreach)", "v=$(repeat)",
			"v=$(case)", "v=$(case x in)",
			"v=$(do)", "v=$(then)", "v=$(fi)", "v=$(esac)", "v=$(done)",
			"v=$(echo $(for))",
		} {
			out, st := call(t, "print ok\n"+body+"\n")
			if st != 0 || strings.Contains(out, "ok\n") || !strings.Contains(out, "AFTER\n") {
				t.Errorf("%s: output = %q (status %d), want the file refused and AFTER reached", body, out, st)
			}
		}
	})
}

// And the shapes zsh *does* read at expansion are left where they were, which
// is what says this is the parse moment rather than a boundary at the call.
//
// Measured the same day and the same way. Both of these write `ok` in real
// zsh — so a command of the body ran, so the file parsed — and then end the
// whole script over the failure, exactly as this shell already did. A
// containment boundary on the autoload call path would have made them carry
// on where zsh does not.
func TestAnAutoloadFileWhoseSubstitutionIsReadAtExpansionStillEndsTheScript(t *testing.T) {
	for _, body := range []string{
		"print ok\nv=$(if)\n",          // `)` ends the substitution; the body fails later
		"print ok\nv=`echo hi; for`\n", // the older spelling's scan has no grammar in it
	} {
		fp := fpathDir(t, map[string]string{"g": body})
		out, st := runZsh(t, t.TempDir(), "fpath=("+fp+")\nautoload -Uz g\nprint BEFORE\ng\nprint AFTER\n")
		if st == 0 || strings.Contains(out, "AFTER\n") {
			t.Errorf("%q: output = %q (status %d), want the script to end before AFTER", body, out, st)
		}
		if !strings.Contains(out, "ok\n") {
			t.Errorf("%q: output = %q, want the body to have run up to the failure", body, out)
		}
	}
}

// The three neighbors that already behaved, asserted here so that the load's
// refusal is not read as a blanket loosening of what a substitution parse
// failure costs.
//
// `source` and `eval` catch it because this dialect makes borrowed text the
// boundary, and a plain syntax error in an autoload file was already a
// refused file. Each measured on zsh 5.9.2: the script reaches its last line
// in all three.
func TestTheRoutesThatAlreadyCarriedOnStillDo(t *testing.T) {
	t.Run("a sourced file", func(t *testing.T) {
		dir := t.TempDir()
		fp := fpathDir(t, map[string]string{"inc": "v=$(echo hi; for)\n"})
		out, st := runZsh(t, dir, "print BEFORE\nsource "+fp+"/inc\nprint \"s=$?\"\nprint AFTER\n")
		if st != 0 || !strings.Contains(out, "AFTER\n") {
			t.Errorf("output = %q (status %d), want the script to reach AFTER at 0", out, st)
		}
	})

	t.Run("eval of the same text", func(t *testing.T) {
		out, st := runZsh(t, t.TempDir(), "print BEFORE\neval 'v=$(echo hi; for)'\nprint \"e=$?\"\nprint AFTER\n")
		if st != 0 || !strings.Contains(out, "AFTER\n") {
			t.Errorf("output = %q (status %d), want the script to reach AFTER at 0", out, st)
		}
	})

	t.Run("an autoload file with a plain syntax error", func(t *testing.T) {
		fp := fpathDir(t, map[string]string{"g": "if\n"})
		out, st := runZsh(t, t.TempDir(), "fpath=("+fp+")\nautoload -Uz g\nprint BEFORE\ng\nprint AFTER\n")
		if st != 0 || !strings.Contains(out, "AFTER\n") {
			t.Errorf("output = %q (status %d), want the script to reach AFTER at 0", out, st)
		}
	})

	t.Run("a function file that parses is unaffected", func(t *testing.T) {
		fp := fpathDir(t, map[string]string{"g": "print ok\nv=$(print -r -- hi)\nprint -r -- \"v=$v\"\n"})
		out, st := runZsh(t, t.TempDir(), "fpath=("+fp+")\nautoload -Uz g\ng\nprint AFTER\n")
		want := "ok\nv=hi\nAFTER\n"
		if st != 0 || out != want {
			t.Errorf("output = %q (status %d), want %q at 0", out, st, want)
		}
	})
}
