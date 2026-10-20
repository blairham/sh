// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"strings"
	"testing"
)

// A function defined with redirections, said back.
//
// `A04redirect.ztst` stops twice on this, three chunks apart: once on
// `which redirfn` and once on `print $functions[redirfn]`. They are the same
// fact — **a definition's redirections are part of the function** — reached
// through two surfaces, and each needed its own half of the fix.
//
// Measured 2026-09-29 on zsh 5.9.2. The listing half is a descriptor rule
// and not a redirection rule: what was wrong was `>&2` in the body coming
// back as `1>&2`, which is bash's normalization and not this shell's. See
// syntax.Layout.RedirectDescriptor for the three-column grid.
func TestAListingSaysBackTheDefinitionsRedirections(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{
			"the chunk",
			"redirfn() {\n  local var\n  read var\n  print I want to tell you about $var\n" +
				"  print Also, this might be an error >&2\n} <input2 >output2 2>&1\nwhich redirfn\n",
			"redirfn () {\n\tlocal var\n\tread var\n\tprint I want to tell you about $var\n" +
				"\tprint Also, this might be an error >&2\n} < input2 > output2 2>&1\n",
		},
		// Where the redirection is attached is the distinction that decides
		// what a listing shows, so each attachment point has a row. The
		// call's redirection is not the function's and must not appear.
		{"on the definition", "f() { print x; } >out\nwhich f\n", "f () {\n\tprint x\n} > out\n"},
		{"in the body", "f() { print x >out; }\nwhich f\n", "f () {\n\tprint x > out\n}\n"},
		{"on the call, which is not the function's", "f() { print x; }\nf >out\nwhich f\n", "f () {\n\tprint x\n}\n"},
		{"on both", "f() { print x >inner; } >outer\nwhich f\n", "f () {\n\tprint x > inner\n} > outer\n"},
		// Several, in written order, with the spacing the listing gives.
		{"several", "f() { print x; } <a >b 2>&1 3>c\nwhich f\n", "f () {\n\tprint x\n} < a > b 2>&1 3> c\n"},
		{"a here-string", "f() { read v; } <<<hi\nwhich f\n", "f () {\n\tread v\n} <<< hi\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st, errs := runZshSplit(t, t.TempDir(), tc.src)
			if out != tc.want || st != 0 || errs != "" {
				t.Errorf("out %q status %d err %q, want %q", out, st, errs, tc.want)
			}
		})
	}
}

// The descriptor rule the listing turns on: a descriptor equal to the
// operator's own default is left off, whether the script wrote it or not.
//
// This is the half that was wrong, and it was wrong in bash's direction —
// the printer wrote `1>&2` for a body that said `>&2`, because it had one
// unconditional rule measured on bash. Measured here on zsh 5.9.2 with
// `which`:
//
//	written    listed
//	>&2        >&2        1>&2   ->  >&2
//	<&3        <&3        0<&3   ->  <&3
//	2>&1       2>&1              (not the default, so it stays)
//	1>out      > out
//	<&-        <&-        (the operator is not rewritten here either)
func TestTheListingLeavesOffADefaultDescriptor(t *testing.T) {
	for _, tc := range []struct{ written, want string }{
		{"print x >&2", "print x >&2"},
		{"print x 1>&2", "print x >&2"},
		{"read v <&3", "read v <&3"},
		{"read v 0<&3", "read v <&3"},
		{"print x 2>&1", "print x 2>&1"},
		{"print x 3>&1", "print x 3>&1"},
		{"print x >&-", "print x >&-"},
		{"read v <&-", "read v <&-"},
		{"read v 0<&-", "read v <&-"},
		{"print x 1>out", "print x > out"},
		{"read v 0<in", "read v < in"},
		{"print x 1>>out", "print x >> out"},
		{"print x 2>out", "print x 2> out"},
		// Not a plain number, so never the default and never touched.
		{"print x >&$fd", "print x >&$fd"},
		{"print x {v}>out", "print x {v}> out"},
	} {
		t.Run(tc.written, func(t *testing.T) {
			out, st, _ := runZshSplit(t, t.TempDir(), "f() { "+tc.written+"; }\nwhich f\n")
			want := "f () {\n\t" + tc.want + "\n}\n"
			if out != want || st != 0 {
				t.Errorf("out %q status %d, want %q", out, st, want)
			}
		})
	}
}

// `$functions[name]` is the second surface, and it needs the braces.
//
// The value is the body *between* the braces — except where the definition
// carries redirections, which follow the closing brace and would have
// nowhere to go without it. Measured 2026-09-29: with redirections the value
// opens `{` and ends `} < in > out 2>&1`; without them it is the body alone.
func TestTheFunctionsParameterKeepsTheBracesForRedirections(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{
			"with redirections, braced",
			"f() {\n  local var\n  print hi >&2\n} <in >out 2>&1\nprint $functions[f]\n",
			"{\n\tlocal var\n\tprint hi >&2\n} < in > out 2>&1\n",
		},
		{
			"without them, the body alone",
			"f() { print plain; }\nprint $functions[f]\n",
			"\tprint plain\n",
		},
		{
			"a redirection in the body is not one on the definition",
			"f() { print x >out; }\nprint $functions[f]\n",
			"\tprint x > out\n",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st, _ := runZshSplit(t, t.TempDir(), tc.src)
			if out != tc.want || st != 0 {
				t.Errorf("out %q status %d, want %q", out, st, tc.want)
			}
		})
	}
	// And the value reads back as a function, which is what the braces are
	// for: assigning it to another name gives a function with the same
	// redirections, nested the way the reference nests it.
	t.Run("it reads back as a function", func(t *testing.T) {
		out, st, _ := runZshSplit(t, t.TempDir(),
			"f() { print hi; } <in >out\nfunctions[g]=$functions[f]\nwhich g\n")
		if st != 0 || !strings.Contains(out, "< in > out") {
			t.Errorf("out %q status %d, want the redirections to survive the round trip", out, st)
		}
	})
}
