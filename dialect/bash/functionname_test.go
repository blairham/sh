// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import "testing"

// TestAListedFunctionNameIsBareOrTakesTheKeyword — this shell writes a
// function's name back **bare** in a body listing, whatever is in it, and
// reaches for the keyword in the one shape where a bare `name ()` header would
// not read back as a definition: a name holding an assignment.
//
// Measured 2026-09-23 on bash 5.3.20, each name defined with the keyword form
// and listed with `declare -f`. This shell quoted seven of these before #4174,
// zsh's answer having been applied to every dialect — `'a=2' () ` where the
// reference writes `function a=2 () `, and `'~x' () ` where it writes `~x () `.
//
// `=x` is the control that makes it the assignment rather than the character:
// nothing stands before the `=` there, so the word is not an assignment and the
// header is bare. See interp.FunctionListingNameSpelling for the panel.
func TestAListedFunctionNameIsBareOrTakesTheKeyword(t *testing.T) {
	for _, tc := range []struct{ name, want string }{
		// The assignment shapes, which take the keyword.
		{"a=2", "function a=2 () \n{ \n    :\n}\n"},
		{"x=", "function x= () \n{ \n    :\n}\n"},
		{"a=b=c", "function a=b=c () \n{ \n    :\n}\n"},
		// The control: no name in front of the `=`, so no assignment.
		{"=x", "=x () \n{ \n    :\n}\n"},
		// And everything else bare, including the seven that used to be
		// quoted here.
		{"a[b", "a[b () \n{ \n    :\n}\n"},
		{"[x", "[x () \n{ \n    :\n}\n"},
		{"a}b", "a}b () \n{ \n    :\n}\n"},
		{"a^b", "a^b () \n{ \n    :\n}\n"},
		{"~x", "~x () \n{ \n    :\n}\n"},
		{"a#b", "a#b () \n{ \n    :\n}\n"},
		{"a*b", "a*b () \n{ \n    :\n}\n"},
		{"a?b", "a?b () \n{ \n    :\n}\n"},
		// And the ordinary ones, which were right all along and are the rows
		// that say this is a spelling and not a rewrite.
		{"11111", "11111 () \n{ \n    :\n}\n"},
		{"f-g", "f-g () \n{ \n    :\n}\n"},
		{"a.b", "a.b () \n{ \n    :\n}\n"},
		{"ok", "ok () \n{ \n    :\n}\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, errs := runBashSplit(t, "function "+tc.name+" { :; }\ndeclare -f\n")
			if out != tc.want {
				t.Errorf("got %q, want %q", out, tc.want)
			}
			if errs != "" {
				t.Errorf("stderr = %q, want nothing", errs)
			}
		})
	}
}
