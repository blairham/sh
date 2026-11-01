// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import "testing"

// A pattern that matched nothing in a program's words is the program's
// failure here: written, 1, and the script carries on — where the same
// pattern given to a builtin or a function ends the script (#6524). Measured
// 2026-10-15 on zsh 5.9.2 under `-fc`; the table is on
// interp.Semantics.UnmatchedPatternFailsOnlyTheProgram.
func TestAnUnmatchedPatternInAProgramsWordsIsTheProgramsFailure(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ src, out, errs string }{
		{
			"/nonexistent/prog zz[x]zz; print after $?\n",
			"after 1\n", "zsh:1: no matches found: zz[x]zz\n",
		},
		{
			"/nonexistent/prog zz[x]zz || print or\n",
			"or\n", "zsh:1: no matches found: zz[x]zz\n",
		},
		{
			// Every other expansion runs first, and only the first pattern
			// is named.
			"/nonexistent/prog zz[x]zz zz[y]zz $(print hi >&2); print after $?\n",
			"after 1\n", "hi\nzsh:1: no matches found: zz[x]zz\n",
		},
		{
			// A failure of any other expansion in the words wins.
			"/nonexistent/prog zz[x]zz ${unset_zz?nope}; print after\n",
			"", "zsh:1: unset_zz: nope\n",
		},
		{
			// The redirections are never opened.
			"/nonexistent/prog zz[x]zz >made_zz; print after $?; [[ -e made_zz ]] || print none\n",
			"after 1\nnone\n", "zsh:1: no matches found: zz[x]zz\n",
		},
		// A refusal in a program's redirection target is the program's too,
		// by the route a redirection already takes.
		{
			"/nonexistent/prog a >zz[x]zz; print after $?\n",
			"after 1\n", "zsh:1: no matches found: zz[x]zz\n",
		},
		// A substitution in a later word is its own place: a loop in it
		// refuses its own word list there, and the program is looked for.
		{
			"/nonexistent/prog a $(for i in zz[x]zz; do :; done); print after $?\n",
			"after 127\n",
			"zsh:1: no matches found: zz[x]zz\nzsh:1: no such file or directory: /nonexistent/prog\n",
		},
		// `command` looks for a program, so its words are a program's.
		{
			"command /nonexistent/prog zz[x]zz; print after $?\n",
			"after 1\n", "zsh:1: no matches found: zz[x]zz\n",
		},
		// The controls: a builtin, a function, `builtin`, `exec` — which
		// starts no child — the name itself, and a builtin's redirection
		// target all end the script.
		{"exec /nonexistent/prog zz[x]zz; print after\n", "", "zsh:1: no matches found: zz[x]zz\n"},
		{"print hi >zz[x]zz; print after\n", "", "zsh:1: no matches found: zz[x]zz\n"},
		{"print zz[x]zz; print after\n", "", "zsh:1: no matches found: zz[x]zz\n"},
		{"f() { :; }; f zz[x]zz; print after\n", "", "zsh:1: no matches found: zz[x]zz\n"},
		{"builtin print zz[x]zz; print after\n", "", "zsh:1: no matches found: zz[x]zz\n"},
		{"zz[x]zz a; print after\n", "", "zsh:1: no matches found: zz[x]zz\n"},
	} {
		out, _, errs := runZshSplit(t, t.TempDir(), tc.src)
		if out != tc.out || errs != tc.errs {
			t.Errorf("%q:\n out %q errs %q\nwant %q errs %q", tc.src, out, errs, tc.out, tc.errs)
		}
	}
}

// Every refusal the pathname stage raises in a program's words is the
// program's, not only a miss: a qualifier list that will not read, a
// subscript in one that will not evaluate, and a pattern that will not
// compile (#6576). The first refusal is the one named, whatever its kind.
// Measured 2026-10-15 on zsh 5.9.2 under `-fc`; the table is on
// interp.Semantics.UnmatchedPatternFailsOnlyTheProgram.
func TestARefusedPatternInAProgramsWordsIsTheProgramsFailure(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ src, out, errs string }{
		{"/nonexistent/prog *(/nonexist); print after $?\n", "after 1\n", "zsh:1: missing end of string\n"},
		{"/nonexistent/prog *(Z); print after $?\n", "after 1\n", "zsh:1: unknown file attribute: Z\n"},
		{"/nonexistent/prog *(oZ); print after $?\n", "after 1\n", "zsh:1: unknown sort specifier\n"},
		{"/nonexistent/prog *([1,]); print after $?\n", "after 1\n", "zsh:1: bad math expression: empty string\n"},
		{"/nonexistent/prog *(#q.); print after $?\n", "after 1\n", "zsh:1: unknown file attribute: #\n"},
		{"/nonexistent/prog zz(b; print after $?\n", "after 1\n", "zsh:1: bad pattern: zz(b\n"},
		{
			// Every other expansion runs first.
			"/nonexistent/prog *(Z) $(print hi >&2); print after $?\n",
			"after 1\n", "hi\nzsh:1: unknown file attribute: Z\n",
		},
		// Only the first refusal is named, whichever kind comes first.
		{"/nonexistent/prog zz[x]zz *(Z); print after $?\n", "after 1\n", "zsh:1: no matches found: zz[x]zz\n"},
		{"/nonexistent/prog *(Z) zz[x]zz; print after $?\n", "after 1\n", "zsh:1: unknown file attribute: Z\n"},
		{"/nonexistent/prog zz[x]zz zz(b; print after $?\n", "after 1\n", "zsh:1: no matches found: zz[x]zz\n"},
		{"/nonexistent/prog zz[x]zz *([1,]); print after $?\n", "after 1\n", "zsh:1: no matches found: zz[x]zz\n"},
		// The controls: a builtin's and a function's words end the script.
		{"print *(Z); print after\n", "", "zsh:1: unknown file attribute: Z\n"},
		{"f() { :; }; f zz(b; print after\n", "", "zsh:1: bad pattern: zz(b\n"},
		{"print *([1,]); print after\n", "", "zsh:1: bad math expression: empty string\n"},
		// A redirection target is read in two views, and its refusal is one
		// sentence.
		{": >*(Z); print after\n", "", "zsh:1: unknown file attribute: Z\n"},
		{"/nonexistent/prog <*(Z); print after $?\n", "after 1\n", "zsh:1: unknown file attribute: Z\n"},
	} {
		out, _, errs := runZshSplit(t, t.TempDir(), tc.src)
		if out != tc.out || errs != tc.errs {
			t.Errorf("%q:\n out %q errs %q\nwant %q errs %q", tc.src, out, errs, tc.out, tc.errs)
		}
	}
}
