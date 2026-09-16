// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import (
	"strings"
	"testing"
)

// A plain `unset NAME` reaching the *function* table — #3205, and this
// dialect's alone on the whole panel.
//
// Measured 2026-09-16 with every script ending in a call whose status is
// printed, both streams captured separately, and a marker after it so the run
// says whether the script continued. bash 5.3.20, the same binary under
// argv[0] `sh` and bash 3.2.57 all answer 127 to the call after `unset b`;
// zsh 5.9.2, ksh93u+ 2012-08-01, dash 0.5.12 and BusyBox ash 1.37.0 all run
// the function and answer 0. Every column continued, so the status is the
// whole of the answer and there is nothing invisible in the table.
//
// Here rather than in the substrate for the reason the attribute tests beside
// this file give: what the substrate holds is two tables and one route
// between them, and whether the route exists at all is
// Semantics.UnsetReachesTheFunctionTable.
//
// Every want below is bytes from bash 5.3.20 unless a comment says otherwise.

// The headline, and the shape the issue was filed from: a helper taken off
// with a plain `unset` really is gone.
func TestAPlainUnsetRemovesTheFunction(t *testing.T) {
	out, st := runBash(t, t.TempDir(),
		"b() { printf 'orig\\n'; }\nunset b\nprintf 'st=%s\\n' \"$?\"\n"+
			"b 2>/dev/null\nprintf 'call=%s\\n' \"$?\"\n")
	const want = "st=0\ncall=127\n"
	if out != want || st != 0 {
		t.Errorf("= %q (status %d), want %q", out, st, want)
	}
}

// One table per call, and the parameter table first. A name that is both
// takes two `unset`s: the first the variable, the second the function.
func TestThePlainSpellingTakesOneTablePerCall(t *testing.T) {
	out, st := runBash(t, t.TempDir(),
		"c() { printf 'c fn\\n'; }\nc=val\n"+
			"unset c\nprintf 'c=[%s]\\n' \"${c-UNSET}\"\n"+
			"c 2>/dev/null\nprintf 'first=%s\\n' \"$?\"\n"+
			"unset c\nc 2>/dev/null\nprintf 'second=%s\\n' \"$?\"\n")
	const want = "c=[UNSET]\nc fn\nfirst=0\nsecond=127\n"
	if out != want || st != 0 {
		t.Errorf("= %q (status %d), want %q", out, st, want)
	}
}

// A declaration is a parameter even with no value in it, so it takes the turn
// the value would have taken. All eight of these leave the function standing
// after one `unset` in bash 5.3.20, and reading only the *value* removed it
// on the first in every one of them.
func TestAValuelessDeclarationTakesTheTurn(t *testing.T) {
	for _, decl := range []string{
		"declare f", "declare -i f", "declare -a f", "declare -A f",
		"declare -l f", "declare -u f", "declare -x f", "export f",
		"f=", "f=(a b)", "local f",
	} {
		t.Run(decl, func(t *testing.T) {
			body := "f() { printf 'f fn\\n'; }\n" + decl + "\n" +
				"unset f\nf 2>/dev/null\nprintf 'one=%s\\n' \"$?\"\n" +
				"unset f\nf 2>/dev/null\nprintf 'two=%s\\n' \"$?\"\n"
			src := body
			if strings.HasPrefix(decl, "local ") {
				// `local` needs a call around it, and the whole sequence
				// goes inside so the shadow is the one being unset.
				src = "wrap() {\n" + body + "}\nwrap\n"
			}
			out, st := runBash(t, t.TempDir(), src)
			const want = "f fn\none=0\ntwo=127\n"
			if out != want || st != 0 {
				t.Errorf("%s: = %q (status %d), want %q", decl, out, st, want)
			}
		})
	}
}

// `-v` names the parameter namespace and reaches no function: `unset -v f`
// leaves `f` callable where the plain spelling does not. The control is the
// point — without it a shell that ignored the letter would pass every row
// above.
func TestTheVLetterReachesNoFunction(t *testing.T) {
	out, st := runBash(t, t.TempDir(),
		"f() { printf 'f fn\\n'; }\nunset -v f\nprintf 'st=%s\\n' \"$?\"\n"+
			"f 2>/dev/null\nprintf 'call=%s\\n' \"$?\"\n")
	const want = "st=0\nf fn\ncall=0\n"
	if out != want || st != 0 {
		t.Errorf("= %q (status %d), want %q", out, st, want)
	}
}

// A name reference is resolved and does not itself count: the parameter that
// decides is the one at the far end.
func TestAReferenceIsFollowedBeforeTheFunctionTable(t *testing.T) {
	for _, c := range []struct{ name, decl, want string }{
		{"aimed at a parameter", "v=1\ndeclare -n n=v\n", "n fn\ncall=0\n"},
		{"aimed at nothing", "declare -n n=nowhere\n", "call=127\n"},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, st := runBash(t, t.TempDir(),
				"n() { printf 'n fn\\n'; }\n"+c.decl+
					"unset n\nn 2>/dev/null\nprintf 'call=%s\\n' \"$?\"\n")
			if out != c.want || st != 0 {
				t.Errorf("= %q (status %d), want %q", out, st, c.want)
			}
		})
	}
}

// The freeze is still the freeze. `readonly -f b` refuses the plain spelling
// in the same sentence it refuses `unset -f b`, at 1, with the body still
// there and the script carrying on — which is what makes the refusal #3206
// implemented reachable at all.
func TestAFrozenFunctionRefusesThePlainUnset(t *testing.T) {
	out, st := runBash(t, t.TempDir(),
		"b() { printf 'orig\\n'; }\nreadonly -f b\n"+
			"unset b\nprintf 'st=%s\\n' \"$?\"\n"+
			"b 2>/dev/null\nprintf 'call=%s\\n' \"$?\"\nprintf 'CONTINUED\\n'\n")
	const want = "bash: line 3: unset: b: cannot unset: readonly function\nst=1\norig\ncall=0\nCONTINUED\n"
	if out != want || st != 0 {
		t.Errorf("= %q (status %d), want %q", out, st, want)
	}
}

// A frozen *parameter* answers first and the function behind it is untouched:
// the refusal names the variable, and one `unset` does not become two.
func TestAFrozenParameterAnswersFirst(t *testing.T) {
	out, st := runBash(t, t.TempDir(),
		"f() { printf 'f fn\\n'; }\nreadonly f=rv\n"+
			"unset f\nprintf 'st=%s\\n' \"$?\"\n"+
			"f 2>/dev/null\nprintf 'call=%s\\n' \"$?\"\n")
	const want = "bash: line 3: unset: f: cannot unset: readonly variable\nst=1\nf fn\ncall=0\n"
	if out != want || st != 0 {
		t.Errorf("= %q (status %d), want %q", out, st, want)
	}
	if strings.Contains(out, "readonly function") {
		t.Errorf("= %q, want the function table never reached", out)
	}
}

// And a name with neither a parameter nor a function is the quiet 0 it has
// always been, in every column.
func TestANameWithNeitherIsStillQuiet(t *testing.T) {
	out, st := runBash(t, t.TempDir(), "unset nothing_at_all_zz\nprintf 'st=%s\\n' \"$?\"\n")
	const want = "st=0\n"
	if out != want || st != 0 {
		t.Errorf("= %q (status %d), want %q", out, st, want)
	}
}
