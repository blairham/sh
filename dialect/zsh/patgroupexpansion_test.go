// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"strings"
	"testing"
)

// An expansion inside a pattern group is read as one, and its **value** is
// the pattern (#1331).
//
// The group was scanned as spans by #1248, so quoting inside one was honored;
// the expansions were left where they had been, as literal text, so
// `[[ wait == ($L) ]]` asked whether `wait` is the two characters `$L` and
// answered no — at status 1, with nothing said anywhere. That is what stopped
// a real plugin manager loading anything at all: its ice parser is one match
// of exactly this shape.
//
// Behavioral rather than a span assertion, and in this package rather than in
// interp, because what an expansion's value is *worth* once it is read is a
// dialect answer and a synthetic Semantics cannot see this preset's.
func TestAnExpansionInsideAPatternGroupIsItsValue(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		// Every spelling of an expansion, each measured against zsh 5.9.2
		// on 2026-09-08.
		{`L=wait; [[ wait == ($L) ]] && echo Y || echo N`, "Y"},
		{`L=wait; [[ wait == (${L}) ]] && echo Y || echo N`, "Y"},
		{`L=wait; [[ wait == (${~L}) ]] && echo Y || echo N`, "Y"},
		{`L=wait; [[ wait == ("$L") ]] && echo Y || echo N`, "Y"},
		{`L=wait; [[ wait == ($(echo wait)) ]] && echo Y || echo N`, "Y"},
		{"L=wait; [[ wait == (`echo wait`) ]] && echo Y || echo N", "Y"},
		{`[[ 4 == ($((2+2))) ]] && echo Y || echo N`, "Y"},
		// Two of them side by side, which is what says the group is a run of
		// spans and not one expansion in a box.
		{`L=wa; M=it; [[ wait == ($L$M) ]] && echo Y || echo N`, "Y"},
		{`L=x; [[ '$'x == ($'\x24'$L) ]] && echo Y || echo N`, "Y"},
		// **The discriminating row.** A pattern built from the source text
		// matches the two characters `$L`; one built from the value does
		// not. `did it match` cannot stand in for it: the row above is `Y`
		// under both readings only if the value happens to be the text.
		{`L=wait; [[ '$L' == ($L) ]] && echo Y || echo N`, "N"},
		{`L=wait; [[ '${L}' == (${L}) ]] && echo Y || echo N`, "N"},
		// Mid-word, nested, and with the group supplying only one arm — the
		// positions #1248 had to measure separately, asked again of the
		// expansion.
		{`L=b; [[ ab == a($L|c) ]] && echo Y || echo N`, "Y"},
		{`L=b; [[ ac == a($L|c) ]] && echo Y || echo N`, "Y"},
		{`L=b; [[ ab == (a(${L}|c)) ]] && echo Y || echo N`, "Y"},
		{`L=b; [[ ax == a($L|c) ]] && echo Y || echo N`, "N"},
		// A `$` that opens nothing is still an ordinary character — and an
		// unset name is still the empty string, so the pattern is `(a)` and
		// the subject is not it. Measured: zsh 5.9.2 says no here too.
		{`[[ 'a$b' == (a$b) ]] && echo Y || echo N`, "N"},
		{`[[ ab == (a$b'b') ]] && echo Y || echo N`, "Y"},
		{`[[ a == (a$b) ]] && echo Y || echo N`, "Y"},
		// Process substitution is the one form a group does not take: `<(`
		// and `>(` are the only spellings whose first byte is also one of
		// the four operators that end a word inside a group, and there the
		// operator wins. zsh 5.9.2 refuses both spellings — `process
		// substitution … cannot be used here` and `number expected` — so
		// reading them here would make two constructs work that the shell
		// does not have. Asked through an `eval`, where a refusal is a
		// status rather than a failure to read the test's own source.
		{`eval '[[ x == (a<(echo x)b) ]]' 2>/dev/null; echo st=$?`, "st=1"},
		{`eval 'print -r -- (a<(echo x)b)' 2>/dev/null; echo st=$?`, "st=1"},
		{`eval '[[ x == (a>(echo x)b) ]]' 2>/dev/null; echo st=$?`, "st=1"},
		// A substitution balances its own parentheses, so the group closes
		// at the last `)` rather than at the one inside the command.
		{`[[ 'a)b' == (a$(printf ')')b) ]] && echo Y || echo N`, "Y"},
		{`[[ 'a(b' == (a$(printf '(')b) ]] && echo Y || echo N`, "Y"},
	} {
		out, _ := answersRun(t, tc.src)
		if got := strings.TrimSpace(out); got != tc.want {
			t.Errorf("%s:\n  said %q\n  want %q", tc.src, got, tc.want)
		}
	}
}

// Whether the metacharacters in that value are live is the axis every other
// expansion meets, asked in the same place rather than a second time for
// groups: this shell answers no, and the `${~ }` flag overrides it.
//
// The alternation is the one the plugin manager's ice list is, so these rows
// are the construct rather than an illustration of it.
func TestAnExpansionInsideAGroupGlobsOnlyWhereTheFlagSaysSo(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{`L='a|b'; [[ a == ($L) ]] && echo Y || echo N`, "N"},
		{`L='a|b'; [[ a == (${~L}) ]] && echo Y || echo N`, "Y"},
		{`L='a|b'; [[ b == (${~L}) ]] && echo Y || echo N`, "Y"},
		// The other direction, and the row that says the first was *literal*
		// rather than merely unmatched: the value matches itself.
		{`L='a|b'; [[ 'a|b' == ($L) ]] && echo Y || echo N`, "Y"},
		{`L='a|b'; [[ 'a|b' == (${~L}) ]] && echo Y || echo N`, "N"},
		{`L='a*'; [[ axx == ($L) ]] && echo Y || echo N`, "N"},
		{`L='a*'; [[ axx == (${~L}) ]] && echo Y || echo N`, "Y"},
		{`L='a*'; [[ 'a*' == ($L) ]] && echo Y || echo N`, "Y"},
		// Quoted, no flag reaches it at all.
		{`L='a|b'; [[ a == ("${~L}") ]] && echo Y || echo N`, "N"},
	} {
		out, _ := answersRun(t, tc.src)
		if got := strings.TrimSpace(out); got != tc.want {
			t.Errorf("%s:\n  said %q\n  want %q", tc.src, got, tc.want)
		}
	}
}

// The construct itself: a backreference group holding an expansion, which is
// what every command of a real plugin manager is parsed by.
//
// The match is asserted as *text*. A row that asks only whether something
// matched passes whichever alternative won, and a group whose bounds are
// wrong still matches — `[][wait][!0]` says each of the three groups took
// the characters written for it.
func TestABackreferenceGroupHoldsAnExpansionsValue(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{`setopt extendedglob; L=wait; [[ xwaity == (#b)*($L)* ]] && print -r -- "[${match[1]}]"`, "[wait]"},
		{
			`setopt extendedglob; L='lucid|wait'; b='wait!0'; ` +
				`[[ $b == (#b)(--|)(${~L})(*) ]] && print -r -- "[${match[1]}][${match[2]}][${match[3]}]" || echo no`,
			"[][wait][!0]",
		},
		{
			`setopt extendedglob; L='lucid|wait'; b='--lucid'; ` +
				`[[ $b == (#b)(--|)(${~L})(*) ]] && print -r -- "[${match[1]}][${match[2]}][${match[3]}]" || echo no`,
			"[--][lucid][]",
		},
		// And the subject that is not an ice at all, which is the row that
		// keeps the pattern from being one that matches everything.
		{`setopt extendedglob; L='lucid|wait'; b='zsh-users/zsh-autosuggestions'; ` +
			`[[ $b == (#b)(--|)(${~L})(*) ]] && print -r -- "[${match[2]}]" || echo no`, "no"},
	} {
		out, _ := answersRun(t, tc.src)
		if got := strings.TrimSpace(out); got != tc.want {
			t.Errorf("%s:\n  said %q\n  want %q", tc.src, got, tc.want)
		}
	}
}

// The same reading on the three routes that are not a condition. #1217 was
// filed after a fix that reached only the route it had been measured on, so
// this is asserted rather than assumed.
//
// The trims are written as the shortest and the longest so the *boundary* is
// visible. A global `//` replacement would pass whatever the group matched,
// because it re-applies until nothing matches, and a count of matches passes
// whether or not the right thing matched.
func TestAGroupsExpansionReachesEveryRoute(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{`L=wait; case xwait in x($L)) echo Y;; *) echo N;; esac`, "Y"},
		{`L=wait; case 'x$L' in x($L)) echo Y;; *) echo N;; esac`, "N"},
		{`L=wait; v=xwaity; print -r -- "[${v#*($L)}][${v%%($L)*}]"`, "[y][x]"},
		{`L=wait; v=xwaity; print -r -- ${v/($L)/Q}`, "xQy"},
		// The filesystem, where the old answer named a pattern nobody wrote:
		// `no matches found: (${L}).zsh`.
		{`: > ice.zsh; : > other.zsh; L=ice; print -r -- (${L}).zsh`, "ice.zsh"},
		{`: > ice.zsh; : > other.zsh; L='ice|other'; print -r -- (${~L}).zsh`, "ice.zsh other.zsh"},
	} {
		out, _ := answersRun(t, tc.src)
		if got := strings.TrimSpace(out); got != tc.want {
			t.Errorf("%s:\n  said %q\n  want %q", tc.src, got, tc.want)
		}
	}
}

// A `|` that arrives from a value is a character and not a choice, even
// though the group around it came from the source.
//
// It is the second half of the same omission: the `|` is the one
// metacharacter a group introduces, so nothing could put one inside a group
// from a value until an expansion inside a group was read as one. The pair of
// names pulls in both directions — the file called `ice|other.zsh` is matched
// and the two named `ice.zsh` and `other.zsh` are not — so the row fails if
// the `|` divides and fails if the expansion never happened.
func TestAnAlternationFromAValueNeedsTheFlag(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{`: > 'ice|other.zsh'; : > ice.zsh; : > other.zsh; L='ice|other'; print -r -- ($L).zsh`, "ice|other.zsh"},
		{
			`: > ice.zsh; : > other.zsh; L='ice|other'; print -r -- ($L).zsh 2>&1 || true`,
			"zsh:1: no matches found: (ice|other).zsh",
		},
		{`: > ice.zsh; : > other.zsh; L='ice|other'; print -r -- (${~L}).zsh`, "ice.zsh other.zsh"},
		// The control: a `|` in a value with no group around it is text
		// however it is read, and must not gain a backslash on the way out.
		{`v='a|b'; print -r -- $v`, "a|b"},
		{`: > 'a|b'; v='a|b'; print -r -- $v`, "a|b"},
		{`: > 'a|b'; v='a|'; print -r -- $v*`, "a|b"},
	} {
		out, _ := answersRun(t, tc.src)
		if got := strings.TrimSpace(out); got != tc.want {
			t.Errorf("%s:\n  said %q\n  want %q", tc.src, got, tc.want)
		}
	}
}
