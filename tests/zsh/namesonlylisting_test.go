// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"strings"
	"testing"

	"github.com/blairham/sh/dialect/zsh"
	"github.com/blairham/sh/interp"
)

// The **names-only** shape of a listing: `typeset +f`, `functions +`, a bare
// `typeset +` and a plus-signed attribute letter. Measured 2026-09-10 against
// zsh 5.9.2 at /opt/homebrew/bin/zsh, `-f` with no startup files and a
// scrubbed environment.
//
// One sign, one meaning, four surfaces. What made this a P1 rather than a
// cosmetic listing fault is that the symptom is on *stdout*: a shell snapshot
// collects a session's functions with
//
//	typeset +f | grep -vE '^_[^_]' | while read func; do
//	  typeset -f "$func" >> "$SNAPSHOT_FILE"
//	done
//
// so bodies coming back where names were asked for make `$func` a *line of a
// function body*, and the loop then asks `typeset -f` about it. Against a
// real `~/.zshrc` that captured 75 functions of 303 and wrote 110 diagnostics
// against zsh's own 27 — and said nothing at all about it (#1576).
//
// The pattern forms of every row here are in typesetmatching_test.go; the two
// halves are one question and share one walk, which is the point of them
// being tested apart: `typeset +fm '_*'` was right for a merge while
// `typeset +f` was wrong, because the letters were read in the `-m` path
// alone.

// `typeset +f` is the function names, one bare name a line, with an operand
// and without one alike.
func TestTypesetPlusFNamesTheFunctions(t *testing.T) {
	const src = "f() { echo x; }\ng() { :; }\n"
	for _, c := range []struct{ name, line, want string }{
		{"with no operand", "typeset +f", "f\ng\n"},
		{"with an operand", "typeset +f f", "f\n"},
		// The sign of the *last* `f` letter decides, not the sign of the
		// last option word: these two lines differ in nothing else.
		{"the last f letter wins, plus", "typeset -f +f f", "f\n"},
		{"the last f letter wins, minus", "typeset +f -f f", "f () {\n\techo x\n}\n"},
		// `-p` alongside changes nothing: the letters already mean print.
		{"a print letter alongside", "typeset +fp f", "f\n"},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, st := runZsh(t, t.TempDir(), src+c.line)
			if out != c.want || st != 0 {
				t.Errorf("%s = %q (status %d), want %q", c.line, out, st, c.want)
			}
		})
	}
}

// A name nobody defined is silent at 1, and the 1 stands however many other
// names printed — the same answer `typeset -f` gives, which is what makes the
// two signs one builtin rather than two.
func TestTypesetPlusFAnswersOneForANameItDoesNotHold(t *testing.T) {
	out, st := runZsh(t, t.TempDir(), "f() { :; }\ntypeset +f f nosuch g")
	if out != "f\n" || st != 1 {
		t.Errorf("typeset +f f nosuch g = %q (status %d), want %q at 1", out, st, "f\n")
	}
}

// The name is written **raw**. The body listing quotes a name that needs it —
// `'a b' () {` — and this one does not: measured, `function "a b" { :; }`
// lists as the two bare characters and a space under both spellings of the
// names-only listing. A list of names is not a program that reads back.
func TestANamesOnlyListingDoesNotQuoteTheName(t *testing.T) {
	const src = "function \"a b\" { :; }\n"
	for _, line := range []string{"typeset +f", "functions +"} {
		out, st := runZsh(t, t.TempDir(), src+line)
		if out != "a b\n" || st != 0 {
			t.Errorf("%s = %q (status %d), want %q", line, out, st, "a b\n")
		}
	}
	// And the body listing, which is the discriminator: a fix that reached
	// this one too would have quieted the row above by making both wrong.
	out, _ := runZsh(t, t.TempDir(), src+"typeset -f")
	if !strings.HasPrefix(out, "'a b' () {") {
		t.Errorf("typeset -f = %q, want the quoted header", out)
	}
}

// `functions` spells no `f` letter, so the sign rides on the option word —
// and a sign on its own *is* an option word here. `functions +` names every
// function where `functions -` writes them all out.
func TestFunctionsTakesItsSignFromTheWord(t *testing.T) {
	const src = "f() { echo x; }\n"
	for _, c := range []struct{ name, line, want string }{
		{"a plus names them", "functions +", "f\n"},
		{"a plus with an operand", "functions + f", "f\n"},
		{"a minus writes them", "functions -", "f () {\n\techo x\n}\n"},
		// Not the reading `-m` gets, which is measured and is why the two
		// are set in different places: under the pattern letter the plus
		// writes the body exactly as the minus does.
		{"the pattern letter is not the same question", "functions +m 'f*'", "f () {\n\techo x\n}\n"},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, st := runZsh(t, t.TempDir(), src+c.line)
			if out != c.want || st != 0 {
				t.Errorf("%s = %q (status %d), want %q", c.line, out, st, c.want)
			}
		})
	}
}

// Over parameters the sign picks between the same two listings it picks
// between under `-m`: the value under a minus, the attribute words and the
// bare name under a plus. A bare sign is the whole table.
func TestABareSignIsTheWholeTablesListing(t *testing.T) {
	if got := zsh.Semantics().SignAloneIsAnOptionWord; got != interp.Yes {
		t.Errorf("SignAloneIsAnOptionWord = %v, want Yes", got)
	}
	const src = "qa=1\ntypeset -i qb=2\ntypeset -a qc=(x y)\n"
	for _, c := range []struct {
		name, line string
		want       []string
	}{
		// The values, and the attribute words in front of them — the same
		// listing the bare word writes.
		{"a minus adds nothing to the bare listing", "typeset -", []string{"qa=1\n", "integer qb=2\n", "array qc=( x y )\n"}},
		// The attribute words and the name, and no `=` anywhere on the row.
		{"a plus leaves the values off", "typeset +", []string{"qa\n", "integer qb\n", "array qc\n"}},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, st := runZsh(t, t.TempDir(), src+c.line)
			if st != 0 {
				t.Fatalf("%s: status = %d, out %q", c.line, st, out)
			}
			for _, want := range c.want {
				if !containsLine(out, want) {
					t.Errorf("%s = %q, want the line %q", c.line, out, want)
				}
			}
		})
	}
	// The contents and not the shape: the plus form must not carry a value
	// on any row it wrote for these names, which is the assertion a count of
	// lines cannot make.
	out, _ := runZsh(t, t.TempDir(), src+"typeset +")
	for _, unwanted := range []string{"qa=1\n", "integer qb=2\n", "array qc=( x y )\n"} {
		if containsLine(out, unwanted) {
			t.Errorf("typeset + = %q, want no value on %q", out, unwanted)
		}
	}
}

// A plus-signed attribute letter with no pattern is the filter that letter is
// under `-m`, over the whole table: the matching names alone, with no
// attribute words in front of them.
func TestAPlusSignedAttributeLetterFiltersTheWholeTable(t *testing.T) {
	const src = "qa=1\nexport qb=2\ntypeset -i qc=3\ntypeset -r qd=4\n"
	for _, c := range []struct {
		name, line     string
		want, unwanted []string
	}{
		{"exported", "typeset +x", []string{"qb\n"}, []string{"qa\n", "qc\n", "integer qc\n"}},
		{"integer", "typeset +i", []string{"qc\n"}, []string{"qa\n", "qb\n"}},
		{"readonly", "typeset +r", []string{"qd\n"}, []string{"qa\n", "qb\n"}},
		// Either letter and not both, which is measured rather than the
		// reading a filter invites — and it takes two letters to see, since
		// one letter answers the same under both readings.
		{"two letters are a union", "typeset +xi", []string{"qb\n", "qc\n"}, []string{"qa\n", "qd\n"}},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, st := runZsh(t, t.TempDir(), src+c.line)
			if st != 0 {
				t.Fatalf("%s: status = %d, out %q", c.line, st, out)
			}
			for _, want := range c.want {
				if !containsLine(out, want) {
					t.Errorf("%s = %q, want the line %q", c.line, out, want)
				}
			}
			for _, no := range c.unwanted {
				if containsLine(out, no) {
					t.Errorf("%s = %q, want no line %q", c.line, out, no)
				}
			}
		})
	}
}

// `-g` is the letter that says the fallback is the bare listing rather than
// silence: it says where a declaration *lands* rather than what a name
// carries, so there is nothing for it to filter on and it drops out under
// either sign. `+gx` writing what `+x` writes is the same fact from the other
// side, and is what keeps `-g` from being read as "a letter, therefore a
// filter that matches nothing".
func TestTheGlobalLetterFiltersNothing(t *testing.T) {
	const src = "qa=1\nexport qb=2\n"
	for _, line := range []string{"typeset -g", "typeset +g"} {
		out, st := runZsh(t, t.TempDir(), src+line)
		if st != 0 {
			t.Fatalf("%s: status = %d, out %q", line, st, out)
		}
		for _, want := range []string{"qa=1\n", "qb=2\n"} {
			if !containsLine(out, want) {
				t.Errorf("%s = %q, want the line %q — the whole table, values and all", line, out, want)
			}
		}
	}
	out, st := runZsh(t, t.TempDir(), src+"typeset +gx")
	if st != 0 {
		t.Fatalf("typeset +gx: status = %d, out %q", st, out)
	}
	if !containsLine(out, "qb\n") || containsLine(out, "qa\n") {
		t.Errorf("typeset +gx = %q, want the exported name alone", out)
	}
}

// `typeset -T` and `+T` with nothing to tie are the same listing under the
// same rule: both halves of every tie, and the sign says whether the values
// come with them.
func TestTheTieListingTakesTheSign(t *testing.T) {
	const src = "typeset -T SCA sca=(p q)\n"
	out, st := runZsh(t, t.TempDir(), src+"typeset +T")
	if st != 0 {
		t.Fatalf("typeset +T: status = %d, out %q", st, out)
	}
	for _, want := range []string{"SCA\n", "sca\n"} {
		if !containsLine(out, want) {
			t.Errorf("typeset +T = %q, want the line %q", out, want)
		}
	}
	if strings.Contains(out, "=") {
		t.Errorf("typeset +T = %q, want no value on any row", out)
	}
	minus, _ := runZsh(t, t.TempDir(), src+"typeset -T")
	if !containsLine(minus, "SCA=p:q\n") {
		t.Errorf("typeset -T = %q, want the scalar's value on its row", minus)
	}
}

// The `z` letter, which is in this builtin's table and in none of the other
// four declaration words. Measured 2026-09-10: zsh 5.9.2 takes `typeset -z`
// and `typeset +z` at 0, and answers `bad option: -z` to `local -z`,
// `integer -z`, `float -z`, `export -z` and `readonly -z`.
//
// It was 68 of the 83 excess diagnostics the snapshot above wrote: a plugin
// loader's own functions are named `+zi-log` and the like, so a *body* line
// beginning `+zi-` reaching `typeset` is read as an option bundle and the
// first letter of it is this one.
func TestTheZLetterIsTypesetsAlone(t *testing.T) {
	for _, line := range []string{"typeset -z", "typeset +z", "typeset -fz", "declare -z"} {
		out, st := runZsh(t, t.TempDir(), line)
		if out != "" || st != 0 {
			t.Errorf("%s = %q (status %d), want silence at 0", line, out, st)
		}
	}
	// Taken and doing nothing is not the same as taken and listing: the
	// letter selects an attribute no name here carries, so the listing is
	// empty where `-g` on the same table writes every name.
	out, st := runZsh(t, t.TempDir(), "qa=1\ntypeset -z qa qb")
	if out != "" || st != 0 {
		t.Errorf("typeset -z qa qb = %q (status %d), want silence at 0", out, st)
	}
	// And it declares, exactly as a bare `typeset qb` would.
	out, st = runZsh(t, t.TempDir(), "typeset -z qb\ntypeset -p qb")
	if out != "typeset qb=''\n" || st != 0 {
		t.Errorf("typeset -z qb = %q (status %d), want the name declared", out, st)
	}
	for _, line := range []string{"local -z", "integer -z", "export -z", "readonly -z"} {
		out, st := runZsh(t, t.TempDir(), "f() { "+line+"; }\nf")
		if st == 0 || !strings.Contains(out, "bad option: -z") {
			t.Errorf("%s = %q (status %d), want the letter refused", line, out, st)
		}
	}
}
