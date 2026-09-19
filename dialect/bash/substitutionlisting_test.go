// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import (
	"os"
	"strings"
	"testing"
)

// What a listing writes between the `$(` and the `)` — #3801, where the span's
// own characters went back out unchanged and every normalization the rest of
// the listing makes stopped at the parenthesis.
//
// Measured 2026-09-19 on bash 5.3.20 through `declare -f`, `env -i
// PATH=/usr/bin:/bin LC_ALL=C`, standard input on the null device. bash 3.2.57
// answers alike, and so do `export -f` and `type` — the body goes back through
// the same engine on all three routes, and through `--pretty-print` as well.
//
// Every `want` below is that shell's own output, byte for byte.
func TestASubstitutionsBodyIsListedThisShellsWay(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{
			// The row the issue is named for: the spacing inside the body is
			// the listing's and not the script's.
			name: "the spacing inside the body",
			src:  "f() { echo $(a >/dev/null;b); }\ndeclare -f f",
			want: "f () \n{ \n    echo $(a > /dev/null; b)\n}\n",
		},
		{
			// And so is the construct, which is the half that says the body
			// was re-read rather than tidied.
			name: "a construct inside the body",
			src:  "f() { echo \"$(if true; then echo x; fi)\"; }\ndeclare -f f",
			want: "f () \n{ \n    echo \"$(if true; then\n    echo x;\nfi)\"\n}\n",
		},
		{
			// The arrangement inside is neither of the two the listing
			// already had: the statements of a list keep the source's line
			// breaks while what a keyword closes is laid out. Both halves
			// are in this one row.
			name: "a list around a construct",
			src:  "f() { echo $(a; b; if true; then echo x; fi; c); }\ndeclare -f f",
			want: "f () \n{ \n    echo $(a; b; if true; then\n    echo x;\nfi; c)\n}\n",
		},
		{
			// The other half of that, on its own: a newline in the source is
			// a newline in the listing, which one statement to a line cannot
			// say and which is why this is a third arrangement.
			name: "a list written over two lines",
			src:  "f() { echo $(echo one\necho two); }\ndeclare -f f",
			want: "f () \n{ \n    echo $(echo one\necho two)\n}\n",
		},
		{
			name: "a loop",
			src:  "f() { echo $(while true; do echo y; done); }\ndeclare -f f",
			want: "f () \n{ \n    echo $(while true; do\n    echo y;\ndone)\n}\n",
		},
		{
			// A loop over words gives its `do` a line of its own here, as it
			// does outside a substitution — the arrangement inside is this
			// listing's own and not a second one.
			name: "a loop over words",
			src:  "f() { echo $(for i in a b; do echo $i; done); }\ndeclare -f f",
			want: "f () \n{ \n    echo $(for i in a b;\ndo\n    echo $i;\ndone)\n}\n",
		},
		{
			// A `case` arm follows the `in` rather than opening a line, and
			// only the first one does.
			name: "a case with one arm",
			src:  "f() { echo $(case x in a) b;; esac); }\ndeclare -f f",
			want: "f () \n{ \n    echo $(case x in a)\n        b\n    ;;\nesac)\n}\n",
		},
		{
			name: "a case with two",
			src:  "f() { echo $(case x in a) b;; c) d;; esac); }\ndeclare -f f",
			want: "f () \n{ \n    echo $(case x in a)\n        b\n    ;;\n    c)\n        d\n    ;;\nesac)\n}\n",
		},
		{
			// A brace group is *not* laid out, which is what says the split
			// is the closing token rather than the nesting.
			name: "a brace group",
			src:  "f() { echo $({ a; b; }); }\ndeclare -f f",
			want: "f () \n{ \n    echo $({ a; b; })\n}\n",
		},
		{
			// Nor is a subshell — and the blank after the `$(` is
			// load-bearing, because the body now *begins* with the
			// parenthesis the reprint wrote. Without it this is `$((`, which
			// is arithmetic and a different program.
			name: "a subshell",
			src:  "f() { echo $( (a; b) ); }\ndeclare -f f",
			want: "f () \n{ \n    echo $( ( a; b ))\n}\n",
		},
		{
			// A declaration written inside one *is* laid out, statements and
			// all, which is the one place the source's line breaks stop
			// being followed.
			name: "a declaration inside the body",
			src:  "f() { echo $(f2() { a;b; }); }\ndeclare -f f",
			want: "f () \n{ \n    echo $(function f2 () \n{ \n    a;\n    b\n})\n}\n",
		},
		{
			name: "a substitution inside the body",
			src:  "f() { echo $(echo $(inner;x)); }\ndeclare -f f",
			want: "f () \n{ \n    echo $(echo $(inner; x))\n}\n",
		},
		{
			name: "one inside double quotes, twice on a line",
			src:  "f() { echo $(a;b)x$(c;d); }\ndeclare -f f",
			want: "f () \n{ \n    echo $(a; b)x$(c; d)\n}\n",
		},
		{
			// An empty body comes back with nothing between the
			// parentheses, where the characters it holds are a blank.
			name: "an empty body",
			src:  "f() { echo $( ); }\ndeclare -f f",
			want: "f () \n{ \n    echo $()\n}\n",
		},
		{
			// A body carrying a here-document keeps it, which is the row
			// that says the reprint writes a program back rather than a
			// line.
			name: "a here-document inside the body",
			src:  "f() { echo $(a <<E\nt\nE\n); }\ndeclare -f f",
			want: "f () \n{ \n    echo $(a <<E\nt\nE\n)\n}\n",
		},
		{
			// **The control.** A backquoted substitution is the same node
			// with a flag, and this shell writes it back exactly as it was
			// written — measured on the same binary in the same run. A
			// change that reprinted every substitution would pass every row
			// above and fail this one.
			name: "a backquoted substitution is untouched",
			src:  "f() { echo `a >/dev/null;b`; }\ndeclare -f f",
			want: "f () \n{ \n    echo `a >/dev/null;b`\n}\n",
		},
		{
			// The second control, and the reason the re-read is best effort:
			// a substitution's body is not read until the substitution runs,
			// so a program holding one that will not parse is a program this
			// shell lists today. It goes back as it stands rather than
			// taking the listing with it.
			name: "a body that will not parse is written back",
			src:  "f() { echo $(for in); }\ndeclare -f f",
			want: "f () \n{ \n    echo $(for in)\n}\n",
		},
		{
			// The third: a `${…}` span is held unparsed too and is *not*
			// reprinted here. bash does reach inside one — `${v-$(a;b)}`
			// comes back `${v-$(a; b)}` there — and this shell's span has no
			// tree under it to reach. Filed rather than half-built.
			name: "a substitution inside a parameter expansion is not reached",
			src:  "f() { echo ${v-$(a;b)}; }\ndeclare -f f",
			want: "f () \n{ \n    echo ${v-$(a;b)}\n}\n",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := runBash(t, t.TempDir(), tc.src)
			if st != 0 || out != tc.want {
				t.Errorf("out %q status %d, want %q", out, st, tc.want)
			}
		})
	}
}

// `type` is the second route that ships the same listing, and a hook attached
// to one layout would pass the suite above while this one wrote the old text.
func TestASubstitutionsBodyIsListedTheSameWayByType(t *testing.T) {
	const want = "f is a function\nf () \n{ \n    echo $(a | b)\n}\n"
	out, st := runBash(t, t.TempDir(), "f() { echo $(a|b); }\ntype f")
	if st != 0 || out != want {
		t.Errorf("out %q status %d, want %q", out, st, want)
	}
}

// And `export -f` is the third, at its own indent — one space, and the same
// space however deep, since what goes into the environment is not indented to
// be read. It is a separate syntax.Layout, so it needs the hook separately:
// without it a child would be handed the body as the script typed it while
// every row above passed.
//
// The environment is read through a real child, which is what says the text
// reached it: the entry's name is `BASH_FUNC_f%%`, which no parameter
// expansion can spell.
func TestASubstitutionsBodyIsListedTheSameWayByExport(t *testing.T) {
	const env = "/usr/bin/env"
	if _, err := os.Stat(env); err != nil {
		t.Skipf("no %s here: %v", env, err)
	}
	out, st := runBash(t, t.TempDir(),
		"f() { echo $(if true; then a; fi); }\nexport -f f\n"+env)
	if st != 0 {
		t.Fatalf("status %d, output %q", st, out)
	}
	const want = "BASH_FUNC_f%%=() {  echo $(if true; then\n a;\nfi)\n}"
	if !strings.Contains(out, want) {
		t.Errorf("a child's environment carried %q, want it to hold %q", out, want)
	}
}
