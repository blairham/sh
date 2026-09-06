// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"testing"

	"github.com/blairham/sh/syntax"
)

// Printing a here-document back and running both halves.
//
// The corpus round trip beside this one covers the cases the corpus happens to
// hold, and every here-document in it has a delimiter — which is why a body
// that ran to the end of the input survived until #962. These are named cases
// for the two shapes where the printer had to supply a newline the input did
// not, and they *run* both programs rather than re-parsing the second: both
// defects produced text that parses, so a re-parse check passes against the
// very thing under test.
//
// `cat <<END\nbody` came back as `cat << END\nbodyEND\n`, which the whole
// panel runs as `bodyEND`; `cat <<A <<B` came back with a blank line opening
// B's body, and `cat <<A <<B` reads B.
func TestPrintingAHereDocumentKeepsWhatItSaid(t *testing.T) {
	for _, c := range []struct {
		name string
		src  string
		want string
		why  string
	}{
		{
			name: "a body whose last line never ended",
			src:  "cat <<END\nbody",
			want: "body",
			why:  "the delimiter joined the body's last line and the command printed `bodyEND`",
		},
		{
			name: "the tab-stripping operator",
			src:  "cat <<-END\n\tbody",
			want: "body",
			why:  "`<<-` reached the same code and had the same defect",
		},
		{
			name: "a second body on the same command",
			src:  "cat <<A <<B\na\nA\nb\nB\n",
			want: "b\n",
			why:  "the last here-document is the one read, and a newline written before its body made the output a blank line and then `b`",
		},
		{
			name: "a second body whose last line never ended",
			src:  "cat <<A <<B\na\nA\nb",
			want: "b",
			why:  "both halves at once",
		},
		{
			name: "a delimiter that did arrive",
			src:  "cat <<END\nbody\nEND\n",
			want: "body\n",
			why:  "the control: the ordinary shape was never wrong and must stay right",
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			f, err := syntax.Parse(c.src, corpusGrammar())
			if err != nil {
				t.Fatalf("parse %q: %v", c.src, err)
			}
			printed := syntax.Print(f)

			wantOut, wantStatus := runUnderBash(t, c.src)
			gotOut, gotStatus := runUnderBash(t, printed)
			// What it said, pinned as a value rather than only against
			// itself: a printer that dropped the body entirely would make
			// both sides agree on nothing at all. A body that ran to the end
			// of the input ends without a newline here, which is what dash,
			// ksh93 and zsh do — bash adds one, and that is a question about
			// the body and not about printing it.
			if wantOut != c.want {
				t.Fatalf("%q printed %q, want %q — %s", c.src, wantOut, c.want, c.why)
			}
			if gotOut != wantOut || gotStatus != wantStatus {
				t.Errorf("printing changed what it does\n  from:   %q\n  gave:   %q\n  before: %q (%d)\n  after:  %q (%d)\n%s",
					c.src, printed, wantOut, wantStatus, gotOut, gotStatus, c.why)
			}
		})
	}
}
