// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// shortReads is this package's test vector with a `read -t` it can reach, a
// read a trapped signal gives up during a wait, and
// Semantics.ReadEndedShortKeepsOnlyCompleteFields unanswered. A read that asks
// it is refused and one that does not is not. keeps is
// ReadTimeoutKeepsWhatArrived.
func shortReads(keeps Answer) Semantics {
	s := testSemantics()
	s.ReadOptions = "rt:u:"
	s.ReadTimeoutBoundsReadability = No
	s.ReadTimeoutKeepsWhatArrived = keeps
	s.ReadIsAbandonedByATrappedSignal = Yes
	s.ReadAbandonedWhen = ReadAbandonedOnlyDuringAWait
	s.ReadEndedShortKeepsOnlyCompleteFields = Unspecified
	s.SubshellJobTable = SubshellJobsKept
	return s
}

// expiresOn has `first` waiting on a fifo and nothing more coming, so a `read
// -t` of it takes what is there and then reaches its deadline. Nothing races:
// the text is in before the read begins.
func expiresOn(first, read string) string {
	return `mkfifo p; exec 3<>p; a=A b=B c=C; printf '` + first + `' >p; ` +
		read + ` <&3; echo "st=$? a=[$a] b=[$b] c=[$c]"`
}

// givenUpOn has `first` waiting on a fifo and a trapped USR1 a second in, by
// which time the read has taken it and is waiting for more. The second is a
// margin for a loaded runner; `late` comes after the signal, so a read the
// signal found not yet waiting takes it rather than hanging.
func givenUpOn(first, read string) string {
	return `mkfifo p; exec 3<>p; a=A b=B c=C; trap 'echo T' USR1; printf '` + first + `' >p; ` +
		`(sleep 1; kill -USR1 $$; sleep 0.5; echo late >p) & ` +
		read + ` <&3; echo "st=$? a=[$a] b=[$b] c=[$c]"; wait`
}

const completeFieldsQuestion = "a `read` that ends short keeping only the fields whose separator arrived: "

// TestAnExpiredReadAsksAboutItsCompleteFieldsOnlyWhereTheReadingsDiffer: at a
// deadline the question is put only where the dialect keeps nothing there and
// a field ahead of the last name is complete, since everywhere else keeping
// the complete fields and keeping nothing leave the same names. A dialect
// that keeps what arrived is never asked.
func TestAnExpiredReadAsksAboutItsCompleteFieldsOnlyWhereTheReadingsDiffer(t *testing.T) {
	for _, tc := range []struct {
		name  string
		keeps Answer
		src   string
		want  string
	}{
		{"keeping what arrived", Yes, expiresOn("x y ", "read -t 0.05 -r a b c"), "st=1 a=[x] b=[y] c=[]\n"},
		{"one name", No, expiresOn("x y ", "read -t 0.05 -r a"), "st=1 a=[A] b=[B] c=[C]\n"},
		{"no separator yet", No, expiresOn("xy", "read -t 0.05 -r a b"), "st=1 a=[A] b=[B] c=[C]\n"},
		{"blanks only", No, expiresOn("  ", "read -t 0.05 -r a b"), "st=1 a=[A] b=[B] c=[C]\n"},
		{"an escaped blank last", No, expiresOn(`x\\ `, "read -t 0.05 a b"), "st=1 a=[A] b=[B] c=[C]\n"},
		{
			"the end of the input", No, `a=A b=B; printf 'x y ' | { read -t 5 -r a b c; echo "st=$? a=[$a] b=[$b]"; }`,
			"st=1 a=[x] b=[y]\n",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			out, st := run(t, tc.src, withSem(shortReads(tc.keeps)))
			if st != 0 || out != tc.want {
				t.Errorf("out=%q st=%d, want %q and no refusal", out, st, tc.want)
			}
		})
	}
}

// TestAReadThatEndsShortIsAskedAboutItsCompleteFields is the other half: a
// deadline with a complete field ahead of the last name, and any read a
// trapped signal gives up, are where the readings part, and an unanswered axis
// is refused there, by name, at 2.
func TestAReadThatEndsShortIsAskedAboutItsCompleteFields(t *testing.T) {
	for _, tc := range []struct{ name, src, rest string }{
		{"expired", expiresOn("x y", "read -t 0.05 -r a b"), "st=2 a=[A] b=[B] c=[C]\n"},
		{"given up", givenUpOn("ab", "read -r a"), "T\nst=2 a=[A] b=[B] c=[C]\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			out, _ := run(t, tc.src, withSem(shortReads(No)))
			if !strings.Contains(out, completeFieldsQuestion+shellsDisagree) || !strings.HasSuffix(out, tc.rest) {
				t.Errorf("got %q, want the refusal naming the question and then %q", out, tc.rest)
			}
		})
	}
}

// TestEachAnswerToAReadThatEndsShort runs both answers at both endings. Yes
// keeps each field whose separator arrived in its own name and leaves the
// field still arriving, the last name and the names past it as they were. No
// leaves it to the ending: nothing at a deadline that keeps nothing, and what
// arrived at a give-up, the names it does not reach cleared.
func TestEachAnswerToAReadThatEndsShort(t *testing.T) {
	for _, tc := range []struct {
		name   string
		answer Answer
		src    string
		want   string
	}{
		{"yes, expired", Yes, expiresOn("x y ", "read -t 0.05 -r a b c"), "st=1 a=[x] b=[y] c=[C]\n"},
		{"yes, expired, more than names", Yes, expiresOn("x y z ", "read -t 0.05 -r a b"), "st=1 a=[x] b=[B] c=[C]\n"},
		{"yes, expired, an escaped blank", Yes, expiresOn(`x\\ y z `, "read -t 0.05 a b c"), "st=1 a=[x y] b=[z] c=[C]\n"},
		{"yes, given up", Yes, givenUpOn("x y ", "read -r a b c"), "T\nst=1 a=[x] b=[y] c=[C]\n"},
		{"yes, given up, one name", Yes, givenUpOn("ab", "read -r a"), "T\nst=1 a=[A] b=[B] c=[C]\n"},
		{"no, expired", No, expiresOn("x y ", "read -t 0.05 -r a b c"), "st=1 a=[A] b=[B] c=[C]\n"},
		{"no, given up", No, givenUpOn("x y ", "read -r a b c"), "T\nst=1 a=[x] b=[y] c=[]\n"},
		{"no, given up, one name", No, givenUpOn("ab", "read -r a"), "T\nst=1 a=[ab] b=[B] c=[C]\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			sem := shortReads(No)
			sem.ReadEndedShortKeepsOnlyCompleteFields = tc.answer
			if out, st := run(t, tc.src, withSem(sem)); st != 0 || out != tc.want {
				t.Errorf("out=%q st=%d, want %q", out, st, tc.want)
			}
		})
	}
}
