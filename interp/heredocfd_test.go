// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// A here-document is opened on the descriptor it was written for, and not on
// standard input whatever the number says.
//
// Measured 2026-09-13 on bash 5.3.15, bash 3.2.57, bash invoked as `sh`,
// ksh93u+, zsh 5.9.2, dash and BusyBox ash: every column agrees on every row
// it has, so this is a correction and not an axis (#2743).
//
// Two of the rows are *silent* when it is wrong — a command whose input the
// shell has quietly replaced prints text nobody asked for, at status 0, with
// nothing on standard error — which is why each is written so that the wrong
// answer differs from the right one in what is printed rather than only in
// what is not.

// `read` is the probe rather than `cat`, so the assertions are about this
// shell and not about a program on the machine running the test.
func runHeredocFd(t *testing.T, src string) (string, int) {
	t.Helper()
	sem := PosixSemantics()
	return run(t, src, func(r *Runner) { r.Semantics = &sem })
}

// The loud row: the shape `while read -r l <&3; do … done 3<<X` is written
// with. A shell that ignores the number answers `bad file descriptor` here.
func TestAHereDocumentOpensTheDescriptorItNames(t *testing.T) {
	out, st := runHeredocFd(t, "{ read -r l <&3; echo \"got=[$l]\"; } 3<<X\nthree\nX\n")
	if out != "got=[three]\n" || st != 0 {
		t.Errorf("got %q at %d, want the document readable on 3", out, st)
	}
}

// The silent row: with the body on 3, the command's own input is untouched,
// so a `read` of standard input finds nothing and fails.
func TestAHereDocumentOnANumberLeavesStandardInputAlone(t *testing.T) {
	out, st := runHeredocFd(t, "read -r l 3<<X\nthree\nX\necho \"st=$? l=[$l]\"\n")
	if out != "st=1 l=[]\n" || st != 0 {
		t.Errorf("got %q at %d, want standard input still empty and the `read` to fail", out, st)
	}
}

// The second silent row, and the one that says the two documents are held
// apart rather than the last one winning: the command reads the one written
// for its input.
func TestTwoHereDocumentsGoToTheirOwnDescriptors(t *testing.T) {
	out, st := runHeredocFd(t, "{ read -r a; read -r b <&3; echo \"a=[$a] b=[$b]\"; } <<P 3<<Q\npee\nP\nqueue\nQ\n")
	if out != "a=[pee] b=[queue]\n" || st != 0 {
		t.Errorf("got %q at %d, want each document on the descriptor it was written for", out, st)
	}
}

// A here-string shares the branch and so shared the defect.
func TestAHereStringOpensTheDescriptorItNames(t *testing.T) {
	out, st := runHeredocFd(t, "{ read -r l <&3; echo \"got=[$l]\"; } 3<<<'string'\n")
	if out != "got=[string]\n" || st != 0 {
		t.Errorf("got %q at %d, want the line readable on 3", out, st)
	}
}

// `exec` is the one command whose redirections outlive it, and a document is
// no different: the descriptor is still open at the next command.
func TestAHereDocumentOpenedByExecOutlivesIt(t *testing.T) {
	out, st := runHeredocFd(t, "exec 3<<X\nkept\nX\nread -r l <&3\necho \"got=[$l]\"\n")
	if out != "got=[kept]\n" || st != 0 {
		t.Errorf("got %q at %d, want the document still open after the `exec`", out, st)
	}
}

// And the variable-named spelling allocates a number and hands it over, the
// same way `{v}<file` does — including the axis that says whether it lasts.
func TestAHereDocumentThroughAVariableNamedDescriptor(t *testing.T) {
	out, st := runHeredocFd(t, "exec {v}<<X\nnamed\nX\nread -r l <&\"$v\"\necho \"v=$v got=[$l]\"\n")
	if !strings.HasSuffix(out, " got=[named]\n") || st != 0 {
		t.Errorf("got %q at %d, want the name to hold a descriptor the document is on", out, st)
	}
	if strings.HasPrefix(out, "v= ") {
		t.Errorf("got %q, want the name to have received a number", out)
	}
}

// The number is given back when the command ends, exactly as an opened file's
// is — `exec` above is the exception and this is the rule.
//
// The document has two lines and only the first is read, so a descriptor that
// leaked past the command would still have something to give: reading nothing
// is what a *closed* number gives and an exhausted one gives too, and a probe
// that could not tell them apart would pass whether or not the table was ever
// saved.
func TestAHereDocumentsDescriptorIsTakenBackWithTheCommand(t *testing.T) {
	out, st := runHeredocFd(t, "{ read -r l <&3; echo \"in=[$l]\"; } 3<<X\nthree\nfour\nX\nread -r m <&3\necho \"after=$? m=[$m]\"\n")
	if !strings.HasPrefix(out, "in=[three]\n") {
		t.Fatalf("got %q at %d, want the document read inside the command", out, st)
	}
	if !strings.Contains(out, "after=1 m=[]\n") {
		t.Errorf("got %q at %d, want 3 closed again once the command it belonged to had ended — `four` is still in the document, so a leaked descriptor would have handed it over", out, st)
	}
}
