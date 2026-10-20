// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import (
	"strings"
	"testing"
)

// The process substitution reads a here-document its own text could not feed
// from the lines after the enclosing command, and the command substitution is
// refused instead.
//
// One shell doing two different things with one question, which is why
// syntax.Dialect.HeredocBodyFromAfterTheCommand is an enumeration and not a
// bool. Measured 2026-09-20 against ksh93u+ 2012-08-01, script files under
// `env -i PATH=/usr/bin:/bin LC_ALL=C` with standard input on the null
// device:
//
//	cat <(cat <<EOF) …    `one`, `body`, `after` — silently, status 0
//	echo $(cat <<EOF) …   refused while reading, status 3
//
// The silence is the second half of it: bash 5.3 reads the same shape and
// warns, so the wording is a dialect's and the capability is not. See #3711,
// and #3361 for the refusal.
func TestAProcessSubstitutionIsFedFromAfterTheCommandAndSaysNothing(t *testing.T) {
	for _, c := range []struct {
		name, src, out string
		status         int
	}{
		{
			name:   "the process substitution yields the body, silently",
			src:    "echo one\ncat <(cat <<EOF)\nbody\nEOF\necho after\n",
			out:    "one\nbody\nafter\n",
			status: 0,
		},
		{
			// The control: a document that closes inside the parentheses is
			// neither carried nor refused.
			name:   "a document that closes inside is untouched",
			src:    "v=$(cat <<EOF\ninside\nEOF\n)\necho \"[$v]\"\n",
			out:    "[inside]\n",
			status: 0,
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, status := answersRun(t, c.src)
			if out != c.out || status != c.status {
				t.Errorf("ran %q: out %q status %d, want %q and %d",
					c.src, out, status, c.out, c.status)
			}
		})
	}
}

// TestTheCommandSubstitutionIsStillRefusedWhileReading is the other half of
// the same question and the control that says carrying a body for one
// spelling did not carry it for the other: #3361's refusal is unmoved.
func TestTheCommandSubstitutionIsStillRefusedWhileReading(t *testing.T) {
	got := refuseAsKsh(t, "echo one\necho $(cat <<EOF)\nbody\nEOF\necho after\n")
	want := "here-document not contained within command substitution"
	if !strings.Contains(got, want) {
		t.Errorf("refusal %q, want it to carry %q", got, want)
	}
}
