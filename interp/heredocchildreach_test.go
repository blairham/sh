// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

//go:build unix

package interp

import (
	"context"
	"strings"
	"testing"

	"github.com/blairham/sh/syntax"
)

// A here-document has to reach a child that names its descriptor.
//
//	/bin/sh -c 'cat <&3' 3<<X
//	child
//	X
//
// prints `child` in bash 5.3.15, ksh93u+, zsh 5.9.2 and dash. It printed
// `3: Bad file descriptor` here: the body was a reader over text this process
// held, `childFiles` rebuilds a child's table by number and can only answer
// with an *os.File, so the entry answered nil — and a nil is a descriptor
// closed in the child (#2759).
//
// The axis is *which* medium and not whether there is one, so both values are
// asked the same question and both have to answer it.
func TestAHereDocumentReachesAChildThatNamesItsDescriptor(t *testing.T) {
	for _, medium := range []HeredocBodyMedium{HeredocBodyOnAPipe, HeredocBodyInATemporaryFile} {
		t.Run(medium.String(), func(t *testing.T) {
			out := runHeredocMedium(t, medium, "/bin/sh -c 'cat <&3' 3<<X\nchild\nX\necho \"st=$?\"\n")
			if want := "child\nst=0\n"; out != want {
				t.Errorf("out = %q, want %q", out, want)
			}
		})
	}
}

// And the medium is the axis, which a script can tell apart: a reader that
// overshoots and seeks back leaves the rest of the document behind on a file
// and loses it on a pipe.
//
// Measured 2026-09-14 with `exec 3<<X` over two lines, `head -1 <&3` and then
// `cat <&3`: bash 5.3.15 and dash print `line1` and nothing; ksh93 and zsh
// print `line1` and then `line2`. Asked here with an explicit seek rather
// than with `head`, because what `head` does with a block is its own program's
// business and the descriptor is what this is about.
func TestTheMediumAHereDocumentBodyIsOnIsVisibleFromAChild(t *testing.T) {
	// Asked from a *child*, which is the only place `/dev/fd/3` names the
	// document at all: inside this shell 3 is an entry in a table and the
	// process's own descriptor 3 is whatever the runtime happened to put
	// there. The child is handed the table by number, so over there the name
	// means what it says.
	//
	// Measured with the same line on 2026-09-14: bash 5.3.15 and dash answer
	// `pipe`, ksh93u+ and zsh 5.9.2 answer `file`.
	const src = "/bin/sh -c 'if [ -f /dev/fd/3 ]; then echo file; else echo pipe; fi' 3<<X\nline1\nline2\nX\n"
	for _, tc := range []struct {
		medium HeredocBodyMedium
		want   string
	}{
		{HeredocBodyOnAPipe, "pipe\n"},
		{HeredocBodyInATemporaryFile, "file\n"},
	} {
		t.Run(tc.medium.String(), func(t *testing.T) {
			if out := runHeredocMedium(t, tc.medium, src); out != tc.want {
				t.Errorf("out = %q, want %q", out, tc.want)
			}
		})
	}
}

// The text is still the text, whichever medium carries it — the shell's own
// read of the body is the case every script writes and the one a change of
// medium must not touch.
func TestTheBodyItselfIsUnchangedByTheMedium(t *testing.T) {
	for _, medium := range []HeredocBodyMedium{HeredocBodyOnAPipe, HeredocBodyInATemporaryFile} {
		t.Run(medium.String(), func(t *testing.T) {
			out := runHeredocMedium(t, medium, "/bin/cat <<X\nfirst\nsecond\nX\n")
			if want := "first\nsecond\n"; out != want {
				t.Errorf("out = %q, want %q", out, want)
			}
		})
	}
}

func runHeredocMedium(t *testing.T, medium HeredocBodyMedium, src string) string {
	t.Helper()
	d := syntax.Core()
	f, err := syntax.Parse(src, d)
	if err != nil {
		t.Fatal(err)
	}
	sem := PosixSemantics()
	sem.HeredocBody = medium
	out := &strings.Builder{}
	r := newTestRunner(t, &Runner{
		Semantics: &sem, Diagnostics: &Diagnostics{}, Name: "sh",
		Stdout: out, Stderr: out,
	})
	if _, err := r.Run(context.Background(), f); err != nil {
		t.Fatal(err)
	}
	return out.String()
}
