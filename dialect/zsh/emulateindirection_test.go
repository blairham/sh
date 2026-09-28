// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// `${!name}` — the construct this shell's own grammar refuses and a **mode**
// gives a meaning.
//
// Every `want` below is the reference's own answer, measured on zsh 5.9.2
// (aarch64-apple-darwin25.4.0) at `/opt/homebrew/bin/zsh`, run `-f` over a
// script file whose first line is the `emulate`, 2026-09-28.

// indirectionSetup is the state every row reads, written once so that the rows
// are the expansions and nothing else.
const indirectionSetup = "s=SVAL; SVAL=DEEP; n=s\n" +
	"a=(zero one two)\n" +
	"typeset -A m; m[ka]=va; m[kb]=vb\n" +
	"typeset -A w; w[k]=tgt; tgt=HELLO\n"

// runEmulated puts the mode on a runner the way a script's own `emulate` line
// does — before the text that reads it is parsed — and runs the row.
//
// The mode is applied to the runner rather than written into the source
// because this package's helper parses a script whole: a front end reads a
// file a command at a time and picks the replaced dialect up as it goes, which
// is what driver's run loop is for, and a test of the *table* has no business
// depending on it. The end-to-end route is graded by `make emulate-sweep`.
func runEmulated(t *testing.T, mode, src string) (string, int) {
	t.Helper()
	r := caseListRunner(t)
	applyEmulation(r, mode, false)
	var out bytes.Buffer
	r.Stdout, r.Stderr = &out, &out
	f, err := syntax.Parse(indirectionSetup+src+"\n", *r.Dialect)
	if err != nil {
		// A refusal at the parse is an answer too, and it is the one the
		// modes without the construct used to give. It is not the one they
		// give now — see TestTheRefusedIndirectionIsCarriedPastTheParse.
		return "parse: " + err.Error(), -1
	}
	st, rerr := r.Run(context.Background(), f)
	if rerr != nil {
		return out.String() + "unsupported: " + rerr.Error(), -1
	}
	return out.String(), st
}

var indirectionRows = []struct {
	name string
	src  string
	// ksh is what `emulate ksh` answers. The other two modes refuse every
	// row, which is asserted once rather than repeated per row.
	ksh string
}{
	// The `!` is **dropped** where no subscript is written, which is what
	// makes this neither bash's reading nor ksh93's: `SVAL=DEEP` is there to
	// say it is not an indirection, and the name `s` to say it is not the
	// name.
	{"a scalar", `printf "[%s]" "${!s}"`, "[SVAL]"},
	{"a scalar naming another", `printf "[%s]" "${!n}"`, "[s]"},
	{"a name nothing set", `printf "[%s]" "${!nosuch}"`, "[]"},
	// And it is the **subscript** where one is written, which is where the
	// reading parts from the plain expansion — `${a[0]}` is `zero`.
	{"an element of an array", `printf "[%s]" "${!a[0]}"`, "[0]"},
	{"an element out of range", `printf "[%s]" "${!a[9]}"`, "[9]"},
	{"a subscript that is an expression", `printf "[%s]" "${!a[1+1]}"`, "[2]"},
	{"a key of a table", `printf "[%s]" "${!m[ka]}"`, "[ka]"},
	{"a key the table has not", `printf "[%s]" "${!m[zz]-MISS}"`, "[MISS]"},
	// The listing, and the row that says it is not simply "the subscripts":
	// a table answers with its keys and an ordinary array with its
	// **values**, the flag this stands for having no effect on one.
	{"the listing of a table", `printf "[%s]" "${!m[@]}"`, "[ka][kb]"},
	{"the listing of an array", `printf "[%s]" "${!a[@]}"`, "[zero][one][two]"},
	// An operator acts on what the listing came to, where bash reads the `!`
	// again as an indirection and answers `ELLO`.
	{"an operator after the listing", `printf "[%s]" "${!w[@]}" "${!w[@]#H}"`, "[k][k]"},
}

// TestTheModeDecidesWhatTheIndirectionMeans is the row that closes #4957.
func TestTheModeDecidesWhatTheIndirectionMeans(t *testing.T) {
	for _, row := range indirectionRows {
		t.Run(row.name, func(t *testing.T) {
			out, st := runEmulated(t, "ksh", row.src)
			if out != row.ksh || st != 0 {
				t.Errorf("emulate ksh: out %q status %d, want %q at 0", out, st, row.ksh)
			}
			for _, mode := range []string{"zsh", "sh"} {
				out, st := runEmulated(t, mode, row.src)
				if !strings.Contains(out, "bad substitution") || st == 0 {
					t.Errorf("emulate %s: out %q status %d, want the refusal", mode, out, st)
				}
			}
		})
	}
}

// TestTheIndirectionIsNotThePlainExpansion is the control the table needs:
// without it every row above is equally well explained by "the `!` does
// nothing at all", which is true of the three rows with no subscript and
// false of the rest.
func TestTheIndirectionIsNotThePlainExpansion(t *testing.T) {
	for _, tc := range []struct{ name, written, bang, plain string }{
		{"an element", "a[0]", "[0]", "[zero]"},
		{"a key", "m[ka]", "[ka]", "[va]"},
		{"a table's listing", "m[@]", "[ka][kb]", "[va][vb]"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if out, _ := runEmulated(t, "ksh", `printf "[%s]" "${!`+tc.written+`}"`); out != tc.bang {
				t.Errorf("${!%s} = %q, want %q", tc.written, out, tc.bang)
			}
			if out, _ := runEmulated(t, "ksh", `printf "[%s]" "${`+tc.written+`}"`); out != tc.plain {
				t.Errorf("${%s} = %q, want %q", tc.written, out, tc.plain)
			}
		})
	}
}

// TestTheRefusedIndirectionIsCarriedPastTheParse is *where* the two modes
// without the construct refuse it, and it is the half a mode-keyed table
// cannot get right on its own: a construct the **parser** refuses is refused
// wherever it stands, and one the expander refuses is refused only where this
// shell reads the words.
//
// Measured with `set -n`, which is the only instrument that tells the two
// apart, and the `${9nope}` rows are the control — an expansion this grammar
// has always deferred answers every position the same way.
func TestTheRefusedIndirectionIsCarriedPastTheParse(t *testing.T) {
	for _, tc := range []struct {
		name    string
		src     string
		refused bool
	}{
		{"at the top level", `echo "${!x}"`, true},
		{"inside a group", `{ echo "${!x}"; }`, false},
		{"inside an if", `if true; then echo "${!x}"; fi`, false},
		{"the control, at the top level", `echo "${9nope}"`, true},
		{"the control, inside a group", `{ echo "${9nope}"; }`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, mode := range []string{"zsh", "sh"} {
				out, st := runEmulated(t, mode, "set -n\n"+tc.src)
				if strings.HasPrefix(out, "parse: ") {
					t.Fatalf("emulate %s: %s — the refusal ended the parse", mode, out)
				}
				if refused := st != 0; refused != tc.refused {
					t.Errorf("emulate %s: out %q status %d, want refused=%v",
						mode, out, st, tc.refused)
				}
			}
		})
	}
}

// TestThePrefixListingIsInNoMode is the spelling the mode does **not** give
// back, and it is what says this is a construct rather than a sigil: the same
// `emulate ksh` that reads `${!x}` and `${!m[@]}` refuses `${!ZQ_@}`.
func TestThePrefixListingIsInNoMode(t *testing.T) {
	for _, mode := range []string{"zsh", "sh", "ksh"} {
		t.Run("emulate "+mode, func(t *testing.T) {
			out, st := runEmulated(t, mode, `ZQ_a=1; ZQ_b=2; echo ${!ZQ_@}`)
			if st == 0 || !strings.Contains(out, "bad substitution") {
				t.Errorf("out %q status %d, want the refusal", out, st)
			}
		})
	}
	// And the control, in the one mode that has the construct at all: the
	// indirection beside it reads.
	if out, st := runEmulated(t, "ksh", `printf "[%s]" "${!n}"`); out != "[s]" || st != 0 {
		t.Errorf("the indirection beside it: out %q status %d, want [s] at 0", out, st)
	}
}

// TestTheGrammarTableAnswersEveryMode is grammarTableHoles pointed at the
// table as it now stands, which is the check the seam was built with: a mode
// an axis does not answer keeps whatever the *previous* emulation left, so
// `emulate sh; emulate zsh` would not be the shell it started as.
func TestTheGrammarTableAnswersEveryMode(t *testing.T) {
	if holes := grammarTableHoles(emulationGrammar); len(holes) != 0 {
		t.Errorf("the table has holes: %v", holes)
	}
	// And the axis itself, read off the dialect the emulation leaves: the
	// construct is the mode's and nothing else in the shell moves it.
	for _, tc := range []struct {
		mode string
		has  bool
	}{{"zsh", false}, {"sh", false}, {"ksh", true}, {"csh", false}} {
		r := caseListRunner(t)
		applyEmulation(r, tc.mode, false)
		if got := r.Dialect.ParamIndirection; got != tc.has {
			t.Errorf("emulate %s: ParamIndirection %v, want %v", tc.mode, got, tc.has)
		}
	}
	// A mode taken and put back is the shell it started as, which is what the
	// holes check exists to protect and which nothing else here would catch.
	r := caseListRunner(t)
	applyEmulation(r, "ksh", false)
	applyEmulation(r, "zsh", false)
	if r.Dialect.ParamIndirection {
		t.Error("`emulate ksh` then `emulate zsh` left the construct behind")
	}
}

// And no option name moves it, which is what puts the axis in
// emulationGrammar rather than in setopt.go beside `shglob` and
// `multifuncdef`. Measured against the reference a name and a state at a time
// over the whole of `${(k)options}`; asserted here for the one name that
// would be the obvious candidate.
func TestNoOptionMovesTheIndirection(t *testing.T) {
	src := indirectionSetup + `printf "[%s]" "${!n}"` + "\n"
	for _, name := range []string{"ksharrays", "shglob", "kshglob", "extendedglob"} {
		t.Run(name, func(t *testing.T) {
			for _, on := range []bool{true, false} {
				r := caseListRunner(t)
				applyEmulation(r, "zsh", false)
				if code := setOption(r, name, on); code != 0 {
					t.Fatalf("setting %s answered %d", name, code)
				}
				if r.Dialect.ParamIndirection {
					t.Errorf("setopt %s %v gave the shell's own mode the construct", name, on)
				}
				if _, err := syntax.Parse(src, *r.Dialect); err != nil {
					t.Fatalf("parse: %v", err)
				}
			}
		})
	}
}

// interp is the axis's own home and grades what the construct *means* — see
// interp/indirectsubscriptflag_test.go, whose twenty rows are the same
// measurement. What this dialect has to say is that it answers the axis at
// all, since a dialect that parses the construct and has not chosen reports an
// unanswered axis where a script expands one.
func TestThisDialectAnswersWhatTheIndirectionMeans(t *testing.T) {
	if got := Semantics().IndirectionIsTheSubscriptFlag; got != interp.Yes {
		t.Errorf("IndirectionIsTheSubscriptFlag = %v, want Yes", got)
	}
}
