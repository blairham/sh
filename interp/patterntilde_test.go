// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"testing"

	. "github.com/blairham/sh/interp"
)

// A tilde at the front of a pattern is expanded before the word becomes one.
//
// Unanimous across the panel — dash, bash 5.3, bash-as-sh, bash 3.2, ksh93 and
// zsh 5.9.2 all take the `case` arm and all answer `/sub` to the trim, which
// is why this is core and not an axis. The golden record holds the two rows:
// `pattern/a-tilde-at-the-front-of-a-pattern-expands` and
// `pattern/the-text-after-an-expanded-tilde-is-still-a-pattern`.
//
// It was missing entirely. The tilde expanded in every *word* position —
// operands, assignments, redirection targets — and in none of the pattern
// ones, so `[[ $HOME == ~ ]]` was false in a shell whose `[[ ~ == $HOME ]]`
// was true. That asymmetry is what made it hard to see: the left side of the
// comparison and every unary test were right, because they are words.
//
// powerlevel10k tells a global tool version from a local override with
// exactly `[[ ${files[1]:h} == ~ ]]`, so the branch never fired, every version
// read as a local override, and five prompt segments were drawn that the real
// shell does not draw (#2181, #2178).
func TestATildeAtTheFrontOfAPatternIsExpanded(t *testing.T) {
	home := func(r *Runner) {
		r.Vars = map[string]string{"HOME": "/h", "PWD": "/here", "OLDPWD": "/before"}
	}
	for _, tc := range []struct {
		name, src, want string
	}{
		// The pattern side of a comparison, which is the one that was wrong.
		{"a condition's pattern side", `[[ /h == ~ ]] && printf "[Y]" || printf "[N]"`, "[Y]"},
		{"with a tail", `[[ /h/x == ~/x ]] && printf "[Y]" || printf "[N]"`, "[Y]"},
		{"negated", `[[ /h != ~ ]] && printf "[Y]" || printf "[N]"`, "[N]"},
		// The two that were already right, so the pair says what changed.
		{"the value side", `[[ ~ == /h ]] && printf "[Y]" || printf "[N]"`, "[Y]"},
		{"a unary test", `[[ -n ~ ]] && printf "[Y]" || printf "[N]"`, "[Y]"},
		// A `case` arm is the same function and the only spelling every shell
		// in the panel has.
		{"a case arm", `case /h in ~) printf "[Y]";; *) printf "[N]";; esac`, "[Y]"},
		{"a case arm with a tail", `case /h/abc in ~/a*) printf "[Y]";; *) printf "[N]";; esac`, "[Y]"},
		// And the three trims, which reach patternOf by a different caller.
		{"a trim", `x=/h/sub; printf "[%s]" "${x#~}"`, "[/sub]"},
		{"a suffix trim", `x=/h/sub; printf "[%s]" "${x%~/sub}"`, "[]"},
		{"a replacement", `x=/h/sub; printf "[%s]" "${x/~/Z}"`, "[Z/sub]"},

		// The limits. A tilde only expands where a tilde expands anywhere:
		// `~*` names no user, and one that is not at the front of the word is
		// ordinary text.
		{"a tilde naming no user", `case /h in ~*) printf "[Y]";; *) printf "[N]";; esac`, "[N]"},
		{"a tilde inside the word", `case a/h in a~) printf "[Y]";; *) printf "[N]";; esac`, "[N]"},
		{"a quoted tilde", `[[ /h == "~" ]] && printf "[Y]" || printf "[N]"`, "[N]"},
		{"an escaped tilde", `[[ /h == \~ ]] && printf "[Y]" || printf "[N]"`, "[N]"},
		// A tilde that arrives as a *value* is not a written one, which is
		// the split `${~spec}` exists for.
		{"a tilde out of a parameter", `v='~'; [[ /h == $v ]] && printf "[Y]" || printf "[N]"`, "[N]"},

		// `~+` and `~-` are a dialect's answer rather than the core's — the
		// axis is [Semantics.TildePlusMinusExpands] — so a preset that has
		// not answered it leaves them as written here as much as in a word.
		// The shells that do have them are pinned in dialect/zsh.
		{"the working directory, unanswered", `[[ /here == ~+ ]] && printf "[Y]" || printf "[N]"`, "[N]"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := runGrammar(t, tc.src, patternGrammar, home)
			if out != tc.want || st != 0 {
				t.Errorf("%s = %q (status %d), want %q at 0", tc.src, out, st, tc.want)
			}
		})
	}
}

// **The directory is matched as text and the tail after it as a pattern.**
//
// The two halves of one word are worth different things, which is why the
// tilde is expanded into the pattern rather than into the word: a home
// directory holding a metacharacter matches itself and nothing else, while
// the `*` a script wrote after the slash is still live.
//
// Measured on zsh 5.9.2 with `HOME=/tmp/p78home/a*b`, both directories
// present. The panel divides on this half and the split is not built as an
// axis here — bash 3.2 and ksh93 read the directory as a pattern, and bash
// 5.3 keeps the escape character itself in the text, which is #1367's unbuilt
// question rather than this one. docs/spec/grammar/patterns.md records the
// three answers.
func TestTheDirectoryATildeNamesIsMatchedAsText(t *testing.T) {
	home := func(r *Runner) { r.Vars = map[string]string{"HOME": `/h/a*b`} }
	for _, tc := range []struct {
		name, src, want string
	}{
		{"itself", `[[ '/h/a*b' == ~ ]] && printf "[Y]" || printf "[N]"`, "[Y]"},
		{"not what its metacharacter would match", `[[ '/h/axxb' == ~ ]] && printf "[Y]" || printf "[N]"`, "[N]"},
		{"and the tail is still live", `[[ '/h/a*b/abc' == ~/a* ]] && printf "[Y]" || printf "[N]"`, "[Y]"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := runGrammar(t, tc.src, patternGrammar, home)
			if out != tc.want || st != 0 {
				t.Errorf("%s = %q (status %d), want %q at 0", tc.src, out, st, tc.want)
			}
		})
	}
}
