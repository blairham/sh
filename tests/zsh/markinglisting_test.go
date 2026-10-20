// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"strings"
	"testing"
)

// `functions -u` and its relatives, measured 2026-09-12 against zsh 5.9.2
// under `-f` with a scratch HOME (#1996).
//
// The letters that *mark* a name for autoloading are, with no operands, a
// listing: the same listing a bare `autoload` writes, narrowed to the names
// holding the mark each letter names.

// markedFixture is a shell with `f1` autoloaded plainly, `f2` autoloaded with
// `-U`, and `g` an ordinary function — three functions that the marking
// letters have to tell apart.
func markedFixture(t *testing.T) string {
	t.Helper()
	fp := fpathDir(t, map[string]string{"f1": "print one", "f2": "print two"})
	return "fpath=(" + fp + ")\nautoload f1\nautoload -U f2\ng() { print g; }\n"
}

const (
	f1Stub = "f1 () {\n\t# undefined\n\tbuiltin autoload -X\n}\n"
	f2Stub = "f2 () {\n\t# undefined\n\tbuiltin autoload -XU\n}\n"
)

// The listing `functions -u` writes is the listing a bare `autoload` writes,
// byte for byte. That is the whole of the first half of #1996: the letter was
// refused as unimplemented while the answer sat one builtin away, so it is
// folded into the population `autoload` already walks rather than written a
// second time.
func TestTheMarkingLetterIsTheListingABareAutoloadWrites(t *testing.T) {
	pre := markedFixture(t)
	bare, _ := runZsh(t, t.TempDir(), pre+"autoload\n")
	if bare != f1Stub+f2Stub {
		t.Fatalf("autoload = %q, want the two stubs", bare)
	}
	for _, word := range []string{"functions -u", "typeset -fu"} {
		out, st := runZsh(t, t.TempDir(), pre+word+"\n")
		if out != bare || st != 0 {
			t.Errorf("%s = %q (status %d), want the bare listing %q", word, out, st, bare)
		}
	}
}

// `-U` is the same listing narrowed to the names whose stub carries that
// letter, and the two letters together are a **union** rather than an
// intersection: `-uU` is `-u`'s answer and not `-U`'s.
func TestTheMarkingLettersNarrowTheListingAndCombineAsAUnion(t *testing.T) {
	pre := markedFixture(t)
	for _, c := range []struct{ word, want string }{
		{"functions -u", f1Stub + f2Stub},
		{"functions -U", f2Stub},
		{"functions -uU", f1Stub + f2Stub},
		{"typeset -fU", f2Stub},
		{"functions", f1Stub + f2Stub + "g () {\n\tprint g\n}\n"},
	} {
		out, st := runZsh(t, t.TempDir(), pre+c.word+"\n")
		if out != c.want || st != 0 {
			t.Errorf("%s = %q (status %d), want %q", c.word, out, st, c.want)
		}
	}
}

// A marking letter under a plus **names** its functions where a minus writes
// them out — and it does so however the `f` letter was signed and whatever
// the last option word was. `typeset -f +U` is the name alone and `functions
// +U -u` names them too, so neither of the two signs an engine already tracks
// answers this: it is the letter's own.
func TestAMarkingLetterUnderAPlusNamesItsFunctions(t *testing.T) {
	pre := markedFixture(t)
	for _, c := range []struct{ word, want string }{
		{"functions +U", "f2\n"},
		{"typeset +fU", "f2\n"},
		{"typeset -f +U", "f2\n"},
		{"functions -u +U", "f1\nf2\n"},
		{"functions +U -u", "f1\nf2\n"},
	} {
		out, st := runZsh(t, t.TempDir(), pre+c.word+"\n")
		if out != c.want || st != 0 {
			t.Errorf("%s = %q (status %d), want %q", c.word, out, st, c.want)
		}
	}
}

// `+u` is not a letter this shell has, though `-u` is and `+U` is. So the
// refused set is strictly narrower than the set that marks and cannot be
// derived from it — see Diagnostics.MarkingLettersUnderPlus.
//
// The wording is the *declaration's* and not `autoload`'s `bad option: -Q`,
// because the line never reached a marking, and the word that complains is
// the word that was written.
func TestAMarkingLetterUnderAPlusIsRefusedByTheOneLetterThatRefusesIt(t *testing.T) {
	pre := markedFixture(t)
	for _, c := range []struct{ word, speaker string }{
		{"functions +u", "functions"},
		{"functions +uU", "functions"},
		{"typeset +fu", "typeset"},
		{"typeset -fu +u f1", "typeset"},
	} {
		out, st := runZsh(t, t.TempDir(), pre+c.word+"\necho st=$?\n")
		want := ":" + c.speaker + ":" + " invalid option(s)"
		if !strings.Contains(out, "invalid option(s)") || !strings.Contains(out, "st=1\n") {
			t.Errorf("%s = %q (status %d), want %q at 1", c.word, out, st, want)
		}
		if !strings.Contains(out, c.speaker+":") {
			t.Errorf("%s = %q, want the complaint named after %s", c.word, out, c.speaker)
		}
		if strings.Contains(out, "f1 () {") || strings.Contains(out, "\nf1\n") {
			t.Errorf("%s = %q, want no listing from a refused line", c.word, out)
		}
	}
	// The letter's *own* last sign is what decides, and not the sign of the
	// last option word: the same two words the other way round mark as usual.
	out, st := runZsh(t, t.TempDir(), pre+"typeset +fu -u nm\necho st=$?\nfunctions -u\n")
	if st != 0 || strings.Contains(out, "invalid option(s)") {
		t.Errorf("typeset +fu -u = %q (status %d), want the marking taken", out, st)
	}
	if !strings.Contains(out, "nm () {") {
		t.Errorf("typeset +fu -u = %q, want nm marked", out)
	}
	// And a plus-signed `u` outside the function table is an ordinary
	// attribute being taken off, which this refusal must not reach.
	out, st = runZsh(t, t.TempDir(), "typeset -u v=abc\ntypeset +u v\nprint $v\n")
	if st != 0 || strings.Contains(out, "invalid option(s)") {
		t.Errorf("typeset +u v = %q (status %d), want no refusal", out, st)
	}
}

// A shell with the notion and no marked functions writes **nothing**, where a
// shell without the notion writes the whole table. An empty narrowing is a
// narrowing and not an absent one, which is the one thing a hook returning a
// slice cannot say by itself.
func TestANarrowedListingWithNothingInItIsEmptyAndNotTheWholeTable(t *testing.T) {
	out, st := runZsh(t, t.TempDir(), "g() { print g; }\nfunctions -u\necho st=$?\n")
	if out != "st=0\n" {
		t.Errorf("functions -u = %q (status %d), want nothing and 0", out, st)
	}
}
