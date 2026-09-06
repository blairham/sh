// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import "testing"

// `zformat`, measured against zsh 5.9.2 (2026-09-06) with a scratch HOME and
// no startup files. Every want is a byte-for-byte transcript of what the real
// builtin wrote for the same snippet.
//
// The values are bracketed in the snippets rather than printed bare, because
// almost everything this builtin decides is *padding*: a check on the visible
// text alone passes for a formatter that ignored every width in the string.

func runZformatCases(t *testing.T, cases []zparseoptsCase) {
	t.Helper()
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			out, st := runZsh(t, t.TempDir(), tc.snippet)
			if out != tc.want || st != tc.status {
				t.Errorf("%s\n = %q (status %d)\nwant %q", tc.snippet, out, st, tc.want)
			}
		})
	}
}

// `-f`: `%c` substitution and the field widths around it.
func TestZformatSubstitutesAndPads(t *testing.T) {
	runZformatCases(t, []zparseoptsCase{{
		name:    "two-specifiers",
		snippet: `zformat -f R "%c-%d" c:hi d:there; echo "st=$? [$R]"`,
		want:    "st=0 [hi-there]\n",
	}, {
		name:    "a-minimum-width-pads-to-the-right",
		snippet: `zformat -f R "[%10c]" c:hi; echo "st=$? [$R]"`,
		want:    "st=0 [[hi        ]]\n",
	}, {
		name:    "a-negative-minimum-pads-to-the-left",
		snippet: `zformat -f R "[%-10c]" c:hi; echo "st=$? [$R]"`,
		want:    "st=0 [[        hi]]\n",
	}, {
		name:    "a-maximum-truncates",
		snippet: `zformat -f R "[%.1c]" c:hello; echo "st=$? [$R]"`,
		want:    "st=0 [[h]]\n",
	}, {
		// Both at once, which is the case that says the maximum is applied
		// before the minimum pads: `hello` cut to `he`, then padded to five.
		name:    "and-both-together-truncate-then-pad",
		snippet: `zformat -f R "[%5.2c]" c:hello; echo "st=$? [$R]"`,
		want:    "st=0 [[he   ]]\n",
	}, {
		name:    "a-doubled-percent-is-a-literal-one",
		snippet: `zformat -f R "100%%"; echo "st=$? [$R]"`,
		want:    "st=0 [100%]\n",
	}, {
		// And it takes a width like anything else.
		name:    "which-takes-a-width-of-its-own",
		snippet: `zformat -f R "[%-5%]"; echo "st=$? [$R]"`,
		want:    "st=0 [[    %]]\n",
	}, {
		// A `%` naming nothing is text, not an error and not an empty
		// string — the specifier set is the caller's, so this shell has no
		// standing to call one wrong.
		name:    "an-unnamed-specifier-is-left-as-written",
		snippet: `zformat -f R "a%xb" c:1; echo "st=$? [$R]"`,
		want:    "st=0 [a%xb]\n",
	}, {
		name:    "width-and-all",
		snippet: `zformat -f R "[%5x]" c:1; echo "st=$? [$R]"`,
		want:    "st=0 [[%5x]]\n",
	}, {
		name:    "a-trailing-percent-too",
		snippet: `zformat -f R "ab%"; echo "st=$? [$R]"`,
		want:    "st=0 [ab%]\n",
	}, {
		name:    "an-empty-value-is-a-value",
		snippet: `zformat -f R "[%c]" c:; echo "st=$? [$R]"`,
		want:    "st=0 [[]]\n",
	}, {
		name:    "a-repeated-specification-is-the-last-one",
		snippet: `zformat -f R "%c" c:1 c:2; echo "st=$? [$R]"`,
		want:    "st=0 [2]\n",
	}})
}

// The ternary, and the two different questions `-f` and `-F` ask about it.
func TestZformatTernaryAsksArithmeticUnderDashFAndWidthUnderDashF(t *testing.T) {
	runZformatCases(t, []zparseoptsCase{{
		name:    "the-manuals-example",
		snippet: `zformat -f R "The answer is '%3(c.yes.no)'." c:3; echo "st=$? [$R]"`,
		want:    "st=0 [The answer is 'yes'.]\n",
	}, {
		name:    "the-test-number-defaults-to-zero",
		snippet: `zformat -f R "%(c.y.n)" c:0; echo "st=$? [$R]"`,
		want:    "st=0 [y]\n",
	}, {
		name:    "and-may-be-written-after-the-parenthesis",
		snippet: `zformat -f R "%(3c.y.n)" c:3; echo "st=$? [$R]"`,
		want:    "st=0 [y]\n",
	}, {
		// Written on both sides, the one before wins — which is only
		// visible because the two numbers disagree here.
		name:    "the-one-before-it-wins-when-both-are-written",
		snippet: `zformat -f R "%1(2c.y.n)" c:2; echo "st=$? [$R]"`,
		want:    "st=0 [n]\n",
	}, {
		// The value is an *expression*, not a number: this is the case a
		// strconv.Atoi would answer `n` to.
		name:    "the-value-is-an-arithmetic-expression",
		snippet: `zformat -f R "%2(c.y.n)" "c:1+1"; echo "st=$? [$R]"`,
		want:    "st=0 [y]\n",
	}, {
		name:    "and-a-word-is-worth-zero-in-one",
		snippet: `zformat -f R "%2(c.y.n)" "c:x"; echo "st=$? [$R]"`,
		want:    "st=0 [n]\n",
	}, {
		name:    "the-delimiter-is-whatever-follows-the-specifier",
		snippet: `zformat -f R "%(cXyXn)" c:0; echo "st=$? [$R]"`,
		want:    "st=0 [y]\n",
	}, {
		name:    "either-half-may-hold-escapes-of-its-own",
		snippet: `zformat -f R "%(c.%d.no)" c:0 d:deep; echo "st=$? [$R]"`,
		want:    "st=0 [deep]\n",
	}, {
		name:    "and-a-parenthesis-is-escaped-with-a-percent",
		snippet: `zformat -f R "%(c.y.a%)b)" c:1; echo "st=$? [$R]"`,
		want:    "st=0 [a)b]\n",
	}, {
		// An expression that will not *evaluate* is not a zero. It is
		// reported as the shell — no `zformat:` in the location, because the
		// expression failed and not the command holding it — and it stops the
		// script, so the `echo` after it never runs. A builtin that read it
		// as 0 would choose the true text and hand its caller a plausible
		// answer to a question that failed.
		name:    "an-expression-that-will-not-evaluate-stops-the-script",
		snippet: `zformat -f R "%0(c.y.n)" "c:1/0" 2>&1; echo "st=$? [$R]"`,
		want:    "zsh:1: division by zero\n",
		status:  1,
	}, {
		name:    "and-so-does-one-that-will-not-parse",
		snippet: `zformat -f R "%0(c.y.n)" "c:1+" 2>&1; echo "st=$? [$R]"`,
		want:    "zsh:1: bad math expression: operand expected at end of string\n",
		status:  1,
	}, {
		name:    "an-unclosed-ternary-is-a-refusal",
		snippet: `zformat -f R "%(c.y" c:0 2>&1; echo "st=$? [$R]"`,
		want:    "zsh:zformat:1: malformed format string\nst=1 []\n",
	}, {
		name:    "dash-f-capital-asks-whether-there-is-a-value",
		snippet: `zformat -F R "%(c.y.n)" c:hello; echo "st=$? [$R]"`,
		want:    "st=0 [y]\n",
	}, {
		name:    "an-empty-one-does-not-count",
		snippet: `zformat -F R "%(c.y.n)" c:; echo "st=$? [$R]"`,
		want:    "st=0 [n]\n",
	}, {
		// The boundary, in both directions, because a strict comparison and
		// a loose one differ nowhere else: three characters against a test
		// of three is *false*.
		name:    "a-test-number-is-a-width-it-must-exceed",
		snippet: `zformat -F R "[%3(c.y.n)][%3(d.y.n)]" c:abc d:abcd; echo "st=$? [$R]"`,
		want:    "st=0 [[n][y]]\n",
	}, {
		name:    "and-a-negative-one-is-a-width-it-must-not",
		snippet: `zformat -F R "[%-3(c.y.n)][%-3(d.y.n)]" c:abc d:abcd; echo "st=$? [$R]"`,
		want:    "st=0 [[y][n]]\n",
	}})
}

// `-a`: aligning a list into columns.
func TestZformatAlignsAList(t *testing.T) {
	runZformatCases(t, []zparseoptsCase{{
		name:    "the-separators-line-up",
		snippet: `zformat -a A " -- " "foo:bar" "longer:baz" "nocolon"; echo "st=$? ${(j:~:)A}"`,
		want:    "st=0 foo    -- bar~longer -- baz~nocolon\n",
	}, {
		// The rule that is easy to miss: neither a string without a colon
		// nor one whose right half is empty takes part in deciding the
		// column, so the separator here sits at `x`'s width and not at
		// `foo`'s.
		name:    "an-empty-right-half-loses-its-colon-and-its-vote",
		snippet: `zformat -a A " -- " "foo:" "x:y"; echo "st=$? ${(j:~:)A}"`,
		want:    "st=0 foo~x -- y\n",
	}, {
		name:    "only-the-first-colon-splits",
		snippet: `zformat -a A "=" "a:b:c"; echo "st=$? [${(j:~:)A}]"`,
		want:    "st=0 [a=b:c]\n",
	}, {
		name:    "and-an-escaped-one-does-not",
		snippet: `zformat -a A " " "a\:b:c"; echo "st=$? [${(j:~:)A}]"`,
		want:    "st=0 [a:b c]\n",
	}, {
		// The escape is undone even where nothing split — this string has no
		// unescaped colon at all and still comes back with a plain one. A
		// no-colon branch that wrote the string back as given would answer
		// `a\:b` here and look right in every other row.
		name:    "an-escaped-colon-with-no-real-one-is-still-unescaped",
		snippet: `zformat -a A "-" "a\:b" "xx:y"; echo "st=$? [${(j:~:)A}]"`,
		want:    "st=0 [a:b~xx-y]\n",
	}, {
		name:    "an-empty-separator-still-aligns",
		snippet: `zformat -a A "" "ab:c" "d:e"; echo "st=$? [${(j:~:)A}]"`,
		want:    "st=0 [abc~d e]\n",
	}, {
		name:    "nothing-to-align",
		snippet: `zformat -a A "-"; echo "st=$? [${(j:~:)A}]"`,
		want:    "st=0 []\n",
	}})
}

// The argument model, which is the one part of this builtin a reading of the
// synopsis gets wrong: exactly one option word, and everything after it is
// data however it is spelled.
func TestZformatReadsExactlyOneOptionWord(t *testing.T) {
	runZformatCases(t, []zparseoptsCase{{
		name:    "a-second-letter-becomes-the-parameter-name",
		snippet: `zformat -f -F R "%(c.y.n)" c: 2>&1; echo "st=$? [$R]"`,
		want:    "zsh:zformat:1: invalid argument: %(c.y.n)\nst=1 []\n",
	}, {
		name:    "a-stack-is-not-an-option-word-at-all",
		snippet: `zformat -fa R "x" 2>&1; echo "st=$?"`,
		want:    "zsh:zformat:1: invalid argument: -fa\nst=1\n",
	}, {
		name:    "and-neither-is-no-letter",
		snippet: `zformat R "%c" c:1 2>&1; echo "st=$? [$R]"`,
		want:    "zsh:zformat:1: invalid argument: R\nst=1 []\n",
	}, {
		name:    "a-double-dash-lets-the-letter-through",
		snippet: `zformat -- -f R "%c" c:1; echo "st=$? [$R]"`,
		want:    "st=0 [1]\n",
	}, {
		name:    "an-unknown-single-letter-is-a-bad-letter",
		snippet: `zformat -Z R x 2>&1; echo "st=$?"`,
		want:    "zsh:zformat:1: invalid option: -Z\nst=1\n",
	}, {
		// The count is checked before the letters, which is why two words
		// never reach a complaint about one.
		name:    "under-three-words-is-a-count-and-not-a-letter",
		snippet: `zformat -f R 2>&1; echo "st=$?"; zformat -a A 2>&1; echo "st=$?"; zformat 2>&1; echo "st=$?"`,
		want: "zsh:zformat:1: not enough arguments\nst=1\n" +
			"zsh:zformat:1: not enough arguments\nst=1\n" +
			"zsh:zformat:1: not enough arguments\nst=1\n",
	}, {
		name:    "a-specification-needs-its-colon",
		snippet: `zformat -f R "%c" cvalue 2>&1; echo "st=$? [$R]"`,
		want:    "zsh:zformat:1: invalid argument: cvalue\nst=1 []\n",
	}, {
		// One byte, not one character: a multibyte specifier is refused.
		name:    "and-a-specifier-is-one-byte",
		snippet: `zformat -f R "%ab" ab:x 2>&1; echo "st=$? [$R]"; zformat -f R "%é" "é:x" 2>&1; echo "st=$?"`,
		want: "zsh:zformat:1: invalid argument: ab:x\nst=1 []\n" +
			"zsh:zformat:1: invalid argument: é:x\nst=1\n",
	}})
}
