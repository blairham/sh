// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"bytes"
	"context"
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

func optRun(t *testing.T, tweak func(*Semantics), dg Diagnostics, src string) (string, int) {
	t.Helper()
	f, err := syntax.Parse(src, syntax.Core())
	if err != nil {
		t.Fatalf("parse %q: %v", src, err)
	}
	var buf bytes.Buffer
	sem := permissive()
	sem.BadOptionToSpecialBuiltinFatal = No
	if tweak != nil {
		tweak(&sem)
	}
	r := newTestRunner(t, &Runner{Stdout: &buf, Stderr: &buf, Semantics: &sem, Diagnostics: &dg, Name: "testsh"})
	st, rerr := r.Run(context.Background(), f)
	if rerr != nil {
		t.Fatalf("run %q: %v", src, rerr)
	}
	return buf.String(), st
}

// The bug: an option a builtin does not have was dropped without a word, so
// `export -Q x=1` exported x and said nothing about the -Q. A typo, or a shell
// whose options are not the ones the script was written for, went unnoticed.
func TestAnOptionABuiltinDoesNotHaveIsRefused(t *testing.T) {
	for _, name := range []string{"export", "readonly", "unset"} {
		out, st := optRun(t, nil, Diagnostics{}, name+` -Q x=1; echo "st=$?"; echo "[${x-unset}]"`)
		if !strings.Contains(out, "-Q") {
			t.Errorf("%s: said %q, want the option named", name, out)
		}
		if !strings.Contains(out, "st=2") {
			t.Errorf("%s: said %q, want status 2", name, out)
		}
		// And it did not go on to do the work.
		if strings.Contains(out, "[1]") {
			t.Errorf("%s: said %q, want the command not to have run", name, out)
		}
		_ = st
	}
}

// The options they do have still work, which is the half a refusal could break.
func TestTheOptionsTheyDoHaveStillWork(t *testing.T) {
	for _, c := range []struct{ src, want string }{
		{`x=1; export x; unset -v x; echo "[${x-unset}]"`, "[unset]"},
		{`export -- y=2; echo "[$y]"`, "[2]"},
		{`unset -- z; echo "st=$?"`, "st=0"},
		{`readonly -- w=3; echo "[$w]"`, "[3]"},
		// A bare `-` reaches the operands and is refused *as a name* where
		// the dialect keeps it, which is three of the four. Eating it is
		// the fourth's answer and has a test of its own — this one names an
		// answer rather than assuming the rule, which is what #204 was.
		{`unset -`, "`-'"},
	} {
		if out, _ := optRun(t, nil, Diagnostics{}, c.src); !strings.Contains(out, c.want) {
			t.Errorf("%s: said %q, want %q", c.src, out, c.want)
		}
	}
}

// Where the dialect says a special builtin's failure is fatal, the script stops
// there rather than carrying on with the next command.
func TestABadOptionCanEndTheScript(t *testing.T) {
	out, _ := optRun(t, func(s *Semantics) { s.BadOptionToSpecialBuiltinFatal = Yes },
		Diagnostics{}, `export -Q x; echo after`)
	if strings.Contains(out, "after") {
		t.Errorf("said %q, want the script to have stopped", out)
	}
	out, _ = optRun(t, func(s *Semantics) { s.BadOptionToSpecialBuiltinFatal = No },
		Diagnostics{}, `export -Q x; echo after`)
	if !strings.Contains(out, "after") {
		t.Errorf("said %q, want the script to carry on", out)
	}
}

// The usage line is per builtin and only two of the four print one.
func TestTheUsageLineFollowsWhereTheDialectPrintsOne(t *testing.T) {
	dg := Diagnostics{
		BuiltinBadOption: "%[1]s: %[2]s: invalid option",
		BuiltinUsage:     map[string]string{"export": "export: usage: export [-fn]"},
	}
	out, _ := optRun(t, nil, dg, `export -Q x`)
	if !strings.Contains(out, "export: usage: export [-fn]") {
		t.Errorf("said %q, want the usage line", out)
	}
	// A builtin with no entry gets none, rather than another's.
	out, _ = optRun(t, nil, dg, `unset -Q x`)
	if strings.Contains(out, "usage") {
		t.Errorf("said %q, want no usage line for a builtin with no entry", out)
	}
	// And with no map at all, none.
	out, _ = optRun(t, nil, Diagnostics{BuiltinBadOption: "%[1]s: %[2]s: invalid option"}, `export -Q x`)
	if strings.Contains(out, "usage") {
		t.Errorf("said %q, want no usage line", out)
	}
}

// TestOnlyASpecialBuiltinEndsTheScriptOverABadOption. Every caller of this
// path was a special builtin until `wait` was not, so the check had never
// been reached — and was wrong the moment it was.
func TestOnlyASpecialBuiltinEndsTheScriptOverABadOption(t *testing.T) {
	fatal := func(s *Semantics) { s.BadOptionToSpecialBuiltinFatal = Yes }

	// `export` is special: the script ends.
	out, _ := optRun(t, fatal, Diagnostics{}, `export -q; echo after`)
	if strings.Contains(out, "after") {
		t.Errorf("a special builtin: got %q, want the script to stop", out)
	}

	// `wait` is not, and the same answer must not end it.
	out, _ = optRun(t, fatal, Diagnostics{}, `wait -x; echo after`)
	if !strings.Contains(out, "after") {
		t.Errorf("a builtin that is not special: got %q, want the script to carry on", out)
	}
}

// TestWhichPartOfABadOptionIsNamed is three answers to the same word, and
// `--version` is what tells them apart.
func TestWhichPartOfABadOptionIsNamed(t *testing.T) {
	for _, tc := range []struct {
		name   string
		naming BadOptionName
		src    string
		want   string
	}{
		{
			// The character after the first dash, which is the substrate's
			// own and two dialects'.
			"the character after the first dash",
			BadOptionFirstCharacter, `unset --version`, "unset: --: bad",
		},
		{
			"the whole word",
			BadOptionWholeWord, `unset --version`, "unset: --version: bad",
		},
		{
			// Every dash skipped, then the first letter it does not know.
			// `v` is one of unset's options, so it is consumed and `e` is
			// the one named — which no rule about *the word* could give.
			"the first letter it does not know",
			BadOptionFirstUnknownLetter, `unset --version`, "unset: -e: bad",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dg := Diagnostics{
				BuiltinBadOption: "%[1]s: %[2]s: bad",
				BadOptionNaming:  tc.naming,
			}
			out, _ := optRun(t, func(s *Semantics) { s.BadNameToUnsetFatal = No }, dg, tc.src)
			if !strings.Contains(out, tc.want) {
				t.Errorf("got %q, want %q in it", out, tc.want)
			}
		})
	}
}

// TestASingleLetterIsNamedTheSameWayByAll, which is why this needed a word
// with two dashes to measure at all: `-q` is `-q` under every rule.
func TestASingleLetterIsNamedTheSameWayByAll(t *testing.T) {
	for _, naming := range []BadOptionName{
		BadOptionFirstCharacter, BadOptionWholeWord, BadOptionFirstUnknownLetter,
	} {
		dg := Diagnostics{BuiltinBadOption: "%[1]s: %[2]s: bad", BadOptionNaming: naming}
		out, _ := optRun(t, func(s *Semantics) { s.BadNameToUnsetFatal = No }, dg, `unset -q x`)
		if !strings.Contains(out, "unset: -q: bad") {
			t.Errorf("%v: got %q, want -q named", naming, out)
		}
	}
}

// TestAnOptionIsRefusedOrSaidToBeMissing, which are different answers to
// different situations and must not be confused: one is "this shell has no
// such option", the other is "this dialect has it and this shell does not".
func TestAnOptionIsRefusedOrSaidToBeMissing(t *testing.T) {
	dg := Diagnostics{
		BuiltinBadOption:           "%[1]s: %[2]s: bad",
		UnimplementedOptionLetters: map[string]string{"read": "s"},
	}
	noFatal := func(s *Semantics) { s.BadOptionToSpecialBuiltinFatal = No }

	out, _ := optRun(t, noFatal, dg, `read -s v </dev/null`)
	if !strings.Contains(out, "not implemented yet") {
		t.Errorf("an option the dialect has: got %q", out)
	}
	if strings.Contains(out, "bad") {
		t.Errorf("an option the dialect has: got %q, want it not called unknown", out)
	}

	out, _ = optRun(t, noFatal, dg, `read -q v </dev/null`)
	if !strings.Contains(out, "read: -q: bad") {
		t.Errorf("an option nobody has: got %q", out)
	}
}

// TestTheMissingListIsCheckedAgainstTheOffendingLetterOnly. `--version`
// contains `e`, `r` and `s`; if the list were checked against every letter in
// the word rather than against the one being named, a word that happens to
// contain a real option's letter would be called missing.
func TestTheMissingListIsCheckedAgainstTheOffendingLetterOnly(t *testing.T) {
	dg := Diagnostics{
		BuiltinBadOption:           "%[1]s: %[2]s: bad",
		UnimplementedOptionLetters: map[string]string{"read": "s"},
	}
	out, _ := optRun(t, func(s *Semantics) { s.BadOptionToSpecialBuiltinFatal = No },
		dg, `read --version v </dev/null`)
	if !strings.Contains(out, "read: --: bad") {
		t.Errorf("got %q, want the named letter refused rather than the word searched", out)
	}
}

// TestWholeWordNamingIsOnlyForDoubleDashWords: the whole-word answer to
// BadOptionNaming applies to a word that begins with `--` and to nothing
// else. A single-dash bundle names the letter the walk stopped on, the same
// as everyone — measured with `read -rx`, which every shell in the panel
// answers with `-x` and never with `-rx`.
func TestWholeWordNamingIsOnlyForDoubleDashWords(t *testing.T) {
	dg := Diagnostics{
		BadOptionNaming:  BadOptionWholeWord,
		BuiltinBadOption: "%[1]s: %[2]s: bad",
	}
	noFatal := func(s *Semantics) { s.BadOptionToSpecialBuiltinFatal = No }

	out, _ := optRun(t, noFatal, dg, `unset -vQ x`)
	if !strings.Contains(out, "unset: -Q: bad") {
		t.Errorf("a bundle: got %q, want the letter named alone", out)
	}

	out, _ = optRun(t, noFatal, dg, `unset --version`)
	if !strings.Contains(out, "unset: --version: bad") {
		t.Errorf("a -- word: got %q, want the word kept whole", out)
	}
}
