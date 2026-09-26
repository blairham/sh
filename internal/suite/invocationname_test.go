// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package suite

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"
)

// #4674. The graded binary was copied into the image as `/suiteours`, and the
// name a shell is invoked under is an input to the shell: zsh reads the first
// letter of argv[0], `s` is `sh`, and the whole zsh column was graded under
// `emulate sh` against a reference running as zsh.
//
// Nothing here starts a shell or a container — these are questions about the
// table, in the shape the rest of container_test.go asks them, because what
// they guard against is a run that reports an ordinary-looking figure.

// The rule, over every contained column and every path its reference could
// resolve to: the word our shell finds in argv[0] puts it in the same mode
// the reference's own word puts the reference in.
func TestTheNameTheGradedBinaryIsInvokedUnderStartsNothingTheReferenceIsNotIn(t *testing.T) {
	for _, s := range Ours {
		if !s.Contained() {
			continue
		}
		for _, reference := range s.Lookup {
			if err := CheckInvocationName(s, oursPath(s), reference); err != nil {
				t.Errorf("the %s column copies its binary in as %q and may find its "+
					"reference at %q: %v", s.Name, oursPath(s), reference, err)
			}
		}
	}
}

// And the base name is the column's own, which is [CheckBaseNames]'s question
// rather than this one: normalize compares a line where a shell names itself
// literally, so a copy under a name of the harness's own inflates every such
// line with nothing in the report saying so. Two checks, two nouns, one
// rename that answers both.
func TestTheGradedBinaryIsCopiedInUnderItsColumnsName(t *testing.T) {
	for _, s := range Ours {
		if !s.Contained() {
			continue
		}
		if got := filepath.Base(oursPath(s)); got != s.Dialect {
			t.Errorf("the %s column copies its binary in as %q, whose base name is %q and "+
				"not %q", s.Name, oursPath(s), got, s.Dialect)
		}
	}
}

// The positive control, and the reason the null above is evidence at all: the
// name this harness used until #4674 is refused, and it is refused for the zsh
// column alone.
//
// The four other rows are the measurement, not padding. bash reads `sh` in
// argv[0] by an **exact** match, so `suiteours` never reached it and its
// column was never in POSIX mode; ksh, dash and ash read no letter off argv[0]
// at all. A check that fired on all five would be keyed on the harness's name
// rather than on what the name does.
func TestTheNameThisHarnessUsedIsRefusedForZshAndNoOtherColumn(t *testing.T) {
	refused := map[string]bool{}
	for _, s := range Ours {
		err := CheckInvocationName(s, "/suiteours", s.Lookup[0])
		if err == nil {
			continue
		}
		if !errors.Is(err, ErrNameStartsAnotherShell) {
			t.Errorf("the %s column: err = %v, want it to wrap ErrNameStartsAnotherShell", s.Name, err)
		}
		if !strings.Contains(err.Error(), "suiteours") || !strings.Contains(err.Error(), "sh emulation") {
			t.Errorf("the %s column: err = %v, want the name and the mode it starts", s.Name, err)
		}
		refused[s.Dialect] = true
	}
	if !refused["zsh"] {
		t.Error("`/suiteours` was accepted for the zsh column, so this check cannot have " +
			"produced the refusal #4674 is about")
	}
	for _, d := range []string{"bash", "ksh", "dash", "ash"} {
		if refused[d] {
			t.Errorf("`/suiteours` was refused for the %s column, which reads no letter off "+
				"argv[0] — the check is firing on the harness's name rather than on what "+
				"the name does", d)
		}
	}
}

// The noun, in one pair: **the name the binary is invoked under**, not the
// path it lives at. The directory is the same word in both rows and only the
// last element moves — which is the whole of the fix, since `/suiteours` and
// `/suiteours/zsh` are the same harness putting the same binary in the same
// place.
func TestThePathIsNotTheName(t *testing.T) {
	s, ok := FindOurs("zsh")
	if !ok {
		t.Fatal("no zsh column")
	}
	if err := CheckInvocationName(s, "/suiteours", "/bin/zsh"); err == nil {
		t.Error("`/suiteours` was accepted for the zsh column")
	}
	if err := CheckInvocationName(s, "/suiteours/zsh", "/bin/zsh"); err != nil {
		t.Errorf("`/suiteours/zsh` was refused for the zsh column: %v", err)
	}
}

// The other question read off the same word, and it is a different one: the
// standard's own name is an exact match in every dialect, so a bash graded
// under the name `sh` is in POSIX mode and the `/bin/bash` beside it is not.
//
// Two shells that share the name are not a confound, which is why this is
// asked against the reference rather than against the column's dialect word:
// `/bin/sh` graded against `/bin/sh` is one shell against itself.
func TestTheStandardsOwnNameIsReadToo(t *testing.T) {
	s, ok := FindOurs("bash")
	if !ok {
		t.Fatal("no bash column")
	}
	if err := CheckInvocationName(s, "/tmp/sh", "/bin/bash"); err == nil {
		t.Error("a binary copied to `sh` was accepted against a reference called `bash`, so " +
			"the column would be graded in POSIX mode against a reference that is not")
	}
	if err := CheckInvocationName(s, "/tmp/sh", "/bin/sh"); err != nil {
		t.Errorf("two shells that share the name were refused: %v", err)
	}
	// A path is not a name: the same word one directory up says nothing.
	if err := CheckInvocationName(s, "/tmp/sh/bash", "/bin/bash"); err != nil {
		t.Errorf("a binary at /tmp/sh/bash was refused on its directory's name: %v", err)
	}
}

// A column with no shell of ours behind it has no front end to read the name,
// and refusing one would take a column away over a question it never asks.
func TestAColumnWithNoDialectOfOursIsNotRefused(t *testing.T) {
	if err := CheckInvocationName(Suite{Name: "fish", Dialect: "fish"}, "/suiteours", "/bin/fish"); err != nil {
		t.Errorf("a column with no dialect parts here was refused: %v", err)
	}
}

// And it is asked in [Sweep] rather than in each of the three programs that
// reach it, so a fourth route is covered by having been written at all. The
// directory is empty and never read: the refusal comes before the plan.
func TestSweepRefusesABinaryThatWouldBeGradedInAnotherMode(t *testing.T) {
	s, ok := FindOurs("zsh")
	if !ok {
		t.Fatal("no zsh column")
	}
	_, err := Sweep(context.Background(), s, t.TempDir(), "/suiteours", "/bin/zsh", Options{})
	if !errors.Is(err, ErrNameStartsAnotherShell) {
		t.Fatalf("Sweep err = %v, want it to wrap ErrNameStartsAnotherShell", err)
	}
}
