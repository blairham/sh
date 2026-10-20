// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import "testing"

// TestANameReferencesOwnNameCannotCarryASubscript — `declare -n name[sub]` is
// refused: a reference is a **name**, and a subscript is not part of one.
//
// Measured 2026-09-23 against bash 5.3.15 in the pinned debian:sid-slim and
// bash 5.3.20 on macOS, which agree. Every row answered 0 here before, and the
// valueless form went further and created an indexed array of the base name —
// where the reference the operand asked for was never made (#4178).
func TestANameReferencesOwnNameCannotCarryASubscript(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{
			"with no value",
			"declare -n 'x[3]'\necho \"st=$?\"\n",
			"st=1\n",
		},
		{
			"with a value",
			"v=1\ndeclare -n 'x[3]'=v\necho \"st=$?\"\n",
			"st=1\n",
		},
		{
			"and readonly with it",
			"v=1\ndeclare -nr 'y[2]'=v\necho \"st=$?\"\n",
			"st=1\n",
		},
		{
			"the subscript is never evaluated",
			"v=1\ndeclare -n 'var[@]'=v\necho \"st=$?\"\n",
			"st=1\n",
		},
		{
			"the operand beside it is still declared",
			"v=1\ndeclare -n 'x[3]' good=v\necho \"st=$?\"\ndeclare -p good\n",
			"st=1\ndeclare -n good=\"v\"\n",
		},
		{
			"and the base name is not brought into being",
			"declare -n 'a[0]'\ndeclare -p a\n",
			"",
		},
		// The controls. A subscript in the **value** is a name to point at
		// and is accepted, and the letter under a plus refuses nothing.
		{"a subscript in the value", "v=1\ndeclare -n r='v[0]'\necho \"st=$?\"\n", "st=0\n"},
		{"the letter under a plus", "declare +n 'z[1]'\necho \"st=$?\"\n", "st=0\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, _ := runBashSplitFatal(t, tc.src)
			if out != tc.want {
				t.Errorf("got %q, want %q", out, tc.want)
			}
		})
	}
	// And the sentence, which names the operand as written — the subscript
	// included — under the builtin that read it.
	_, errs := runBashSplitFatal(t, "declare -n 'x[3]'\n")
	if want := "declare: x[3]: reference variable cannot be an array"; !contains(errs, want) {
		t.Errorf("stderr = %q, want it to hold %q", errs, want)
	}
	_, errs = runBashSplitFatal(t, "f() { local -n 'x[3]'=y; }\nf\n")
	if want := "local: x[3]: reference variable cannot be an array"; !contains(errs, want) {
		t.Errorf("local: stderr = %q, want it to hold %q", errs, want)
	}
}

func contains(s, sub string) bool {
	return len(sub) == 0 || (len(s) >= len(sub) && indexOf(s, sub) >= 0)
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
