// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import "testing"

// `local -T` ties the two names, as `typeset -T` and `declare -T` do (#5095).
//
// It declared two ordinary names and left them untied. The letter was in the
// word's own option set and was read into the flags; what was missing is that
// `local`'s loop never reached the `-T` dispatch, which is #5074's shape one
// letter along — the same word, for the same reason, and fixed the same way:
// the three shapes the letter has are folded into one helper both words call
// rather than copied into the second.
//
// Measured 2026-09-28 on zsh 5.9.2 (`/opt/homebrew/bin/zsh`; `go version -m`
// reports *not a Go executable*, so the reference is that shell and not
// another build of this one), script files under `env -i PATH=/usr/bin:/bin`
// with a scratch HOME and standard input on the null device.
//
// **Both halves are graded.** A tie that makes the names track each other
// while leaving the attribute off would pass the value rows and fail the type
// rows, so every word gets both.
func TestLocalTiesTheTwoNames(t *testing.T) {
	dir := t.TempDir()
	for _, word := range []string{"local", "typeset", "declare"} {
		for _, tc := range []struct{ name, body, want string }{
			{"the scalar joins the array", `print -r -- "[$IN]"`, "[a:b]"},
			{"the array is itself", `print -r -- "[${in[@]}]"`, "[a b]"},
			{
				"both carry the attribute", `print -r -- "${(t)IN} / ${(t)in}"`,
				"scalar-local-tied / array-local-tied",
			},
			{"a write to the scalar reaches the array", "IN=p:q\n" + `print -r -- "[${in[@]}]"`, "[p q]"},
			{"and a write to the array reaches the scalar", "in=(p q)\n" + `print -r -- "[$IN]"`, "[p:q]"},
		} {
			t.Run(word+": "+tc.name, func(t *testing.T) {
				src := "f(){ " + word + " -T IN in=(a b); " + tc.body + " }\nf\n"
				if out, st := runZsh(t, dir, src); out != tc.want+"\n" || st != 0 {
					t.Errorf("out %q status %d, want %q at 0", out, st, tc.want+"\n")
				}
			})
		}
	}
}

// The letter's other two shapes, which the same helper answers.
//
// A refusal and a listing, and both were silence at 0 under this word before:
// `local +T A a` took the line and did nothing where the reference refuses,
// and `local -T` with no operands wrote this word's own listing where the
// reference writes the tie listing.
func TestLocalCarriesTheLettersOtherShapes(t *testing.T) {
	dir := t.TempDir()
	for _, tc := range []struct {
		name, src, want string
		status          int
	}{
		{
			"the plus form with operands is a refusal",
			"f(){ local +T A a }\nf\n", "f:local: use unset to remove tied variables\n", 1,
		},
		{
			"one operand is a refusal",
			"f(){ local -T A }\nf\n", "f:local: -T requires names of scalar and array\n", 1,
		},
		// The listing forms are compared against `typeset`'s own inside one
		// run, because what they write is the whole tie table and that is the
		// shell's, not this test's. **Both taken inside a function**, because
		// one of the names in that table is tied to the evaluation context
		// and reads differently at the top level — a comparison across the
		// two says `differ` for a reason that is nothing to do with the
		// word.
		{
			"the bare form is the tie listing",
			"typeset -T A a=(x y)\nf(){ local -T }\ng(){ typeset -T }\n" +
				`[[ "$(f)" == "$(g)" ]] && print same || print differ` + "\n",
			"same\n", 0,
		},
		{
			"and the plus form is its names",
			"typeset -T A a=(x y)\nf(){ local +T }\ng(){ typeset +T }\n" +
				`[[ "$(f)" == "$(g)" ]] && print same || print differ` + "\n",
			"same\n", 0,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if out, st := runZsh(t, dir, tc.src); out != tc.want || st != tc.status {
				t.Errorf("out %q status %d, want %q at %d", out, st, tc.want, tc.status)
			}
		})
	}
}

// A tie takes a **new** cell, and the new cell is empty.
//
// The shadow this path took made a binding and left it reading whatever the
// outer name held, which no other declaration word does — so a local tie over
// an outer `S=v` read `v` inside the call where the reference reads nothing.
// It was invisible while `local -T` did nothing at all, and the row that
// shows it is the same one under `typeset`, which had it all along.
func TestATieOverAnOuterNameStartsEmpty(t *testing.T) {
	dir := t.TempDir()
	for _, word := range []string{"local", "typeset", "declare"} {
		t.Run(word, func(t *testing.T) {
			src := "S=v\nf(){ " + word + ` -T S s; print -r -- "in=[$S]" }` + "\n" +
				"f\n" + `print -r -- "out=[$S]"` + "\n"
			const want = "in=[]\nout=[v]\n"
			if out, st := runZsh(t, dir, src); out != want || st != 0 {
				t.Errorf("out %q status %d, want %q at 0", out, st, want)
			}
		})
	}
	// And a half the line does give a value to is not emptied under it.
	t.Run("a value on the scalar survives", func(t *testing.T) {
		src := "f(){ local -T IN=x:y in; " + `print -r -- "[${in[@]}]"` + " }\nf\n"
		if out, st := runZsh(t, dir, src); out != "[x y]\n" || st != 0 {
			t.Errorf("out %q status %d, want %q at 0", out, st, "[x y]\n")
		}
	})
}

// The export letter asks for `-g` as well, under the words that answer yes to
// it — and `local` is not one of them.
//
// The exemption used to be structural: the function that asks this says
// `local` never reaches it because `biLocal` has a loop of its own. Both
// words reach one tie helper now, so the loop no longer says which word is
// which and the caller does — which is exactly the kind of thing a fold can
// lose, so it has rows.
func TestTheExportLetterReachesPastTheCallUnderTypesetAndNotLocal(t *testing.T) {
	dir := t.TempDir()
	for _, tc := range []struct{ name, word, want string }{
		{"typeset reaches past it", "typeset", "[x:y] 1"},
		{"declare reaches past it", "declare", "[x:y] 1"},
		{"local does not", "local", "[] 0"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			src := "f(){ " + tc.word + " -xT A a=(x y) }\nf\n" + `print -r -- "[$A] ${+A}"` + "\n"
			if out, st := runZsh(t, dir, src); out != tc.want+"\n" || st != 0 {
				t.Errorf("out %q status %d, want %q at 0", out, st, tc.want+"\n")
			}
		})
	}
	// Without the letter every word is local, which is the control that says
	// the letter is what moves it.
	for _, word := range []string{"typeset", "declare", "local"} {
		t.Run("without the letter: "+word, func(t *testing.T) {
			src := "f(){ " + word + " -T A a=(x y) }\nf\n" + `print -r -- "[$A] ${+A}"` + "\n"
			if out, st := runZsh(t, dir, src); out != "[] 0\n" || st != 0 {
				t.Errorf("out %q status %d, want %q at 0", out, st, "[] 0\n")
			}
		})
	}
}
