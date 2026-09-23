// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"testing"

	. "github.com/blairham/sh/interp"
)

// Whether a colon closes a tilde prefix in an **ordinary word** is an axis with
// three values, and all three are reachable.
//
// Inside an assignment's value a colon closes one in every column, which is
// core behavior and is tildeprefixend_test.go's. This is the word road, where
// the panel parts three ways: never, always, and only where the whole word is
// written plainly.
func TestAColonClosingAnOrdinaryWordsTildePrefixIsAThreeValuedAxis(t *testing.T) {
	for _, tc := range []struct {
		name  string
		reach TildeColonReach
		plain string
		quote string
	}{
		{"never", TildeColonEndsNothing, "[~:x]", `[~:xy]`},
		{"always", TildeColonAlwaysEndsAPrefix, "[/h:x]", `[/h:xy]`},
		{"only a plain word", TildeColonEndsAPrefixInAPlainWord, "[/h:x]", `[~:xy]`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ask := func(s *Semantics) { s.TildeColonEndsAnOrdinaryWordsPrefix = tc.reach }
			if out, st := tildeRun(t, `printf '[%s]' ~:x`, ask); st != 0 || out != tc.plain {
				t.Errorf("~:x: got %q status %d, want %q", out, st, tc.plain)
			}
			// The same word with a quote past the colon, which is the row that
			// separates the third reading from the second.
			if out, st := tildeRun(t, `printf '[%s]' ~:x"y"`, ask); st != 0 || out != tc.quote {
				t.Errorf(`~:x"y": got %q status %d, want %q`, out, st, tc.quote)
			}
		})
	}
	if _, st := tildeRun(t, `printf '[%s]' ~:x`, func(*Semantics) {}); st != 2 {
		t.Errorf("status %d, want the unanswered axis refused", st)
	}
}

// And it is asked only where the two sets of closing bytes come to different
// words, which is what keeps it away from every `~/…` anybody writes.
func TestTheTildeColonAxisIsNotAskedWhereTheSetsAgree(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		// A slash closes the prefix before the colon is reached.
		{"a slash first", `printf '[%s]' ~/m:x`, "[/h/m:x]"},
		{"a plain home", `printf '[%s]' ~/m`, "[/h/m]"},
		// The colon is there and no reading moves the tilde, because the name
		// in front of it is a user nobody has.
		{"a user nobody has", `printf '[%s]' ~chet:x`, "[~chet:x]"},
		// A tilde that does not open the word is ordinary text, and a colon
		// that does not follow one opens nothing.
		{"a tilde inside the word", `printf '[%s]' a~:x`, "[a~:x]"},
		{"a tilde behind a colon", `printf '[%s]' x:~/m`, "[x:~/m]"},
		{"no tilde at all", `printf '[%s]' a:b`, "[a:b]"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if out, st := tildeRun(t, tc.src, func(*Semantics) {}); st != 0 || out != tc.want {
				t.Errorf("%s: got %q status %d, want %q with no question asked", tc.src, out, st, tc.want)
			}
		})
	}
}

// A tilde prefix is the **word's** leading plain text and not the first span's,
// which is what brace expansion makes visible: `~{a,b}` is the two words
// `[~][a]` and `[~][b]`, and the prefix is `~a` — a user nobody has — rather
// than a bare `~` with a letter behind it.
//
// Asserted under a vector that closes at neither a colon nor a quote, so what
// the rows show is the span reading and nothing else. Found beside #4200 and
// folded in here because it is this function's bug: real zsh refuses the user
// and real ksh93 keeps the characters, and this shell answered the home
// directory with an `a` on the end.
func TestATildePrefixIsTheWordsLeadingTextAndNotOneSpans(t *testing.T) {
	ask := func(s *Semantics) {
		s.TildeColonEndsAnOrdinaryWordsPrefix = TildeColonEndsNothing
		s.BraceExpansion = Yes
		s.BraceOutputRereadAsText = No
	}
	for _, tc := range []struct{ name, src, want string }{
		{"a produced name nobody has", `printf '[%s]' ~{a,b}`, "[~a][~b]"},
		{"and one behind a letter", `printf '[%s]' ~x{a,b}`, "[~xa][~xb]"},
		// The prefix still ends where a slash the braces produced stands, and
		// the text behind it is the word's.
		{"a produced slash closes it", `printf '[%s]' ~{/,x}m`, "[/h/m][~xm]"},
		// A word the braces left alone is the ordinary road, unchanged.
		{"no braces at all", `printf '[%s]' ~/m`, "[/h/m]"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if out, st := tildeRun(t, tc.src, ask); st != 0 || out != tc.want {
				t.Errorf("%s: got %q status %d, want %q", tc.src, out, st, tc.want)
			}
		})
	}
}

// The prefix reaching across spans must not swallow what is past it, which is
// the half a reading that only ever rewrote the first span could not get wrong.
func TestAProducedTildePrefixLeavesTheRestOfTheWordAlone(t *testing.T) {
	ask := func(s *Semantics) {
		s.TildeColonEndsAnOrdinaryWordsPrefix = TildeColonEndsNothing
		s.BraceExpansion = Yes
		s.BraceOutputRereadAsText = No
	}
	for _, tc := range []struct{ name, src, want string }{
		{"text behind a produced slash", `printf '[%s]' ~{/a,b}/c`, "[/h/a/c][~b/c]"},
		{"a home and then a colon", `printf '[%s]' ~{/,x}m:z`, "[/h/m:z][~xm:z]"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if out, st := tildeRun(t, tc.src, ask); st != 0 || out != tc.want {
				t.Errorf("%s: got %q status %d, want %q", tc.src, out, st, tc.want)
			}
		})
	}
}

// tildeRun runs a snippet under the standard's preset with the axes these rows
// reach answered, and a home directory that is one character so a row reads as
// the rule rather than as a path.
func tildeRun(t *testing.T, src string, set func(*Semantics)) (string, int) {
	t.Helper()
	return run(t, src+"\n", func(r *Runner) {
		sem := CoreSemantics()
		set(&sem)
		r.Semantics = &sem
		r.Vars = map[string]string{"HOME": "/h"}
	})
}
