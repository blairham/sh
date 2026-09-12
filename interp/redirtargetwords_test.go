// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// A redirection target that comes to more than one word.
//
// Two readings and then a third. Under the ordinary-word reading it is an
// ambiguous redirect, because neither word says where to write. Under the
// other reading it used to be joined into one filename with the spaces left
// in — and that is only half right: the shell that does not split a target
// makes **one redirection per word**, which its fan-out and fan-in then join.
//
// Measured 2026-09-12 on zsh 5.9.2, with `printf 'a\n' >f; printf 'b\n' >g`:
//
//	v=(f g); cat <$v                    a then b
//	v=(a b); echo hi >$v                both files
//	cat <p?          (two matches)      both files
//	e="f g"; cat <$e                    no such file or directory: f g
//	unsetopt multios; v=(f g); cat <$v  no such file or directory: f g
//
// The last two are the controls, and the second of them is what says the fan
// and not the expansion is what makes two: turn the joining option off and
// the joined filename comes back, which is what this shell did always (#1792).
//
// So the axis that allows several is the fan's own — RedirectsUseEveryTarget
// — asked after RedirectTargetIsAnOrdinaryWord has said the target is not
// split, and only where the target actually came to more than one word.
func severalWords(fans Answer) func(*Runner) {
	sem := permissive()
	// The reading under which a target is not split into fields. The other
	// answer is the ambiguous-redirect reading, which has no several-words
	// case to fan.
	sem.RedirectTargetIsAnOrdinaryWord = No
	sem.RedirectsUseEveryTarget = fans
	// The array readings the shell with that answer also holds, so that what
	// a row shows is the redirection and not an unanswered expansion.
	sem.ArrayScalarIsTheWholeArray = Yes
	sem.ArrayNameWithoutSubscriptIsTheList = Yes
	return withSem(sem)
}

func TestATargetThatCameToSeveralWordsIsSeveralRedirections(t *testing.T) {
	const src = `printf 'a\n' > f; printf 'b\n' > g; v=(f g); printf "[%s]" "$(cat <$v)"`
	for _, tc := range []struct {
		name string
		fans Answer
		want string
	}{
		{"each word is a redirection, joined", Yes, "[a\nb]"},
		{"the words are one filename", No, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, _ := run(t, src, severalWords(tc.fans))
			if tc.want == "" {
				if !strings.Contains(out, "f g") {
					t.Errorf("got %q, want the joined name `f g` reported", out)
				}
				return
			}
			if out != tc.want {
				t.Errorf("got %q, want %q", out, tc.want)
			}
		})
	}
}

// The writing half, which is the one that loses output where the reading half
// only reads less.
func TestATargetThatCameToSeveralWordsWritesToEachOfThem(t *testing.T) {
	dir := t.TempDir()
	sem := permissive()
	sem.RedirectTargetIsAnOrdinaryWord = No
	sem.RedirectsUseEveryTarget = Yes
	sem.ArrayScalarIsTheWholeArray = Yes
	sem.ArrayNameWithoutSubscriptIsTheList = Yes
	_, st := run(t, `v=(a b); echo hi >$v`, func(r *Runner) {
		r.Semantics, r.Dir = &sem, dir
	})
	if st != 0 {
		t.Fatalf("status %d", st)
	}
	for _, name := range []string{"a", "b"} {
		if got := readFile(t, dir, name); got != "hi\n" {
			t.Errorf("%s = %q, want the line in both files", name, got)
		}
	}
}

// A pattern reaches the same place, which is what says the rule is about the
// words and not about arrays — and it is also the row that says a target is
// matched at all under this reading: a single match opens the file it named.
func TestAPatternTargetIsMatchedAndMayComeToSeveralWords(t *testing.T) {
	for _, tc := range []struct {
		name, src, want string
	}{
		{"two matches", `printf 'a\n' > p1; printf 'b\n' > p2; printf "[%s]" "$(cat <p?)"`, "[a\nb]"},
		{"one match", `printf 'a\n' > p1; printf "[%s]" "$(cat <p?)"`, "[a]"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := run(t, tc.src, severalWords(Yes))
			if out != tc.want || st != 0 {
				t.Errorf("got %q (status %d), want %q at 0", out, st, tc.want)
			}
		})
	}
}

// A scalar holding a space is still one name, under both answers. This is the
// control the joined reading used to be right about, and a change that
// reached for the *fields* view instead of the words view would break it.
func TestAScalarHoldingASpaceIsStillOneName(t *testing.T) {
	for _, fans := range []Answer{Yes, No} {
		out, _ := run(t, `e="f g"; cat <$e`, severalWords(fans))
		if !strings.Contains(out, "f g") {
			t.Errorf("%v: got %q, want the one name `f g`", fans, out)
		}
	}
}

// The fan is asked only where the target actually came to several words, so a
// Runner that has not answered it still runs an ordinary redirection under
// the unsplit reading.
func TestTheFanIsNotAskedAboutAnOrdinaryTarget(t *testing.T) {
	sem := permissive()
	sem.RedirectTargetIsAnOrdinaryWord = No
	sem.RedirectsUseEveryTarget = Unspecified
	sem.ArrayScalarIsTheWholeArray = Yes
	sem.ArrayNameWithoutSubscriptIsTheList = Yes

	out, st := run(t, `printf 'a\n' > f; e=f; printf "[%s]" "$(cat <$e)"`, withSem(sem))
	if want := "[a]"; out != want || st != 0 {
		t.Errorf("one word: got %q (status %d), want %q at 0", out, st, want)
	}

	out, _ = run(t, `printf 'a\n' > f; printf 'b\n' > g; v=(f g); cat <$v`, withSem(sem))
	if !strings.Contains(out, "several words") {
		t.Errorf("several words: %q does not name the axis", out)
	}
}

// A word of the fan that will not open ends the redirection, and the failure
// names *that* word rather than the whole target. Measured: with `f` there
// and `nosuch` not, either order reports `nosuch` and status 1.
func TestAWordOfTheFanThatWillNotOpenEndsIt(t *testing.T) {
	for _, src := range []string{
		`printf 'a\n' > f; v=(nosuch f); cat <$v; printf "st=%s" "$?"`,
		`printf 'a\n' > f; v=(f nosuch); cat <$v; printf "st=%s" "$?"`,
	} {
		out, _ := run(t, src, severalWords(Yes))
		if !strings.Contains(out, "nosuch") {
			t.Errorf("%s: got %q, want the word that failed named", src, out)
		}
		if !strings.HasSuffix(out, "st=1") {
			t.Errorf("%s: got %q, want status 1", src, out)
		}
	}
}
