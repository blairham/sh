// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import "testing"

// A `<<-` said back as `<<`, because by then the mark has nothing left to
// strip.
//
// `A04redirect.ztst` stops on "Two here-documents in a line are shown
// correctly": a function whose body is `cat <<-x <<-y` with tab-indented
// bodies must list as `cat <<x <<y` with the bodies at column 0. The dash
// asks the *reader* to take a leading tab off every body line and off the
// delimiter, and a listing holds the already-stripped body — so writing the
// mark back would ask for a strip that has been done.
//
// Measured 2026-09-29 on zsh 5.9.2, the script written with real tabs
// because the bodies are newline-delimited text: a literal backslash-n in
// the fixture makes a here-document with **no body**, which is a valid
// construct and would not error.
//
// The bodies are already written at column 0 here; the operator was the only
// thing wrong.
func TestAListingDropsTheHereDocumentStripMark(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{
			"the chunk: two of them on one line",
			"f() {\n\tcat <<-x <<-y\n\tfoo\n\tx\n\tbar\n\ty\n}\n",
			"f () {\n  cat <<x <<y\nfoo\nx\nbar\ny\n}\n",
		},
		{
			"one of them",
			"f() {\n\tcat <<-x\n\tfoo\n\tx\n}\n",
			"f () {\n  cat <<x\nfoo\nx\n}\n",
		},
		{
			// The control: a plain `<<` was never going to carry a mark,
			// and it is here so that "drops the mark" cannot be read as
			// "rewrites the operator".
			"a plain here-document is untouched",
			"f() {\n\tcat <<x\nfoo\nx\n}\n",
			"f () {\n  cat <<x\nfoo\nx\n}\n",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st, errs := runZshSplit(t, t.TempDir(), tc.src+"which -x2 f\n")
			if out != tc.want || st != 0 || errs != "" {
				t.Errorf("out %q status %d err %q, want %q", out, st, errs, tc.want)
			}
		})
	}
}

// The listing has to read back as the same function, which is what the
// dropped mark is *for*: `<<-x` over an unindented body would strip nothing
// and mean the same thing, but `eval`ing a listing is how the chunk checks
// itself and the round trip is the assertion that matters.
func TestTheListingReadsBackAsTheSameFunction(t *testing.T) {
	// `read` rather than `cat`: the runner's PATH is a temp directory, so
	// an external command would fail for a reason that has nothing to do
	// with the listing.
	out, st, _ := runZshSplit(t, t.TempDir(),
		"f() {\n\tread v <<-x\n\tfoo\n\tx\n\tprint -r -- $v\n}\n"+
			"eval \"$(which -x2 f)\"\n"+
			"f\n")
	if out != "foo\n" || st != 0 {
		t.Errorf("out %q status %d, want the re-read function to run the same", out, st)
	}
}

// And `$functions[]` is the other surface, which goes through the same
// printer and so gets it for free — asserted rather than assumed, because
// #5127 is the front where the two surfaces needed separate fixes.
func TestTheFunctionsParameterDropsItToo(t *testing.T) {
	out, st, _ := runZshSplit(t, t.TempDir(),
		"f() {\n\tcat <<-x <<-y\n\tfoo\n\tx\n\tbar\n\ty\n}\nprint -r -- $functions[f]\n")
	const want = "\tcat <<x <<y\nfoo\nx\nbar\ny\n"
	if out != want || st != 0 {
		t.Errorf("out %q status %d, want %q", out, st, want)
	}
}
