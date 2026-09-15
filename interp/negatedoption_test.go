// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"context"
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// A dialect may list one of the substrate's options under the *opposite*
// spelling, and may list names `set` will not take at all.
//
// The two seams are Runner.AddNegatedSetOptions and
// Runner.AddImmovableSetOptions, and these tests name them rather than a
// shell: `clobber` stands in for whatever a shell calls the state the
// substrate stores as `noclobber`, and `held` for a name a shell lists and
// refuses. A shell that spells five of the shared options the positive way
// round listed the substrate's five instead, and a listing is a capture
// surface — `eval "$(set +o)"` — so the wrong vocabulary at status 0 is the
// silent kind of wrong answer (#2925).

// negatedRunner is a shell whose listing spells `noclobber` the other way
// round and which lists one name it will not take.
func negatedRunner(t *testing.T, src string, plusLists bool) (string, int) {
	t.Helper()
	var buf strings.Builder
	s := PosixSemantics()
	s.BadSetOptionNameFatal = No
	d := Diagnostics{PlusOListsActive: plusLists}
	r := newTestRunner(t, &Runner{
		Stdout: &buf, Stderr: &buf, Semantics: &s, Diagnostics: &d, Name: "sh",
	})
	r.AddSetOptions("held")
	r.AddImmovableSetOptions("held")
	r.AddNegatedSetOptions(map[string]string{"clobber": "noclobber"})
	f, err := syntax.Parse(src, syntax.Core())
	if err != nil {
		t.Fatal(err)
	}
	st, rerr := r.Run(context.Background(), f)
	if rerr != nil {
		t.Fatal(rerr)
	}
	return buf.String(), st
}

// The listing writes the negated spelling, with the state the other way up,
// and the substrate's own name is gone from it.
func TestANegatedNameReplacesTheSubstratesInTheListing(t *testing.T) {
	out, st := negatedRunner(t, "set -o\n", false)
	if st != 0 {
		t.Fatalf("status %d, want 0", st)
	}
	if !strings.Contains(out, "clobber        on\n") {
		t.Errorf("listing = %q, want a `clobber on` row", out)
	}
	for _, line := range strings.Split(out, "\n") {
		if strings.HasPrefix(line, "noclobber") {
			t.Errorf("listing = %q, want no `noclobber` row", out)
		}
	}
}

// And it follows the state, which is what says the row is one option read
// upside down rather than a second one.
func TestANegatedRowFollowsTheStateItNegates(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{"set -o noclobber\nset -o\n", "clobber        off\n"},
		{"set +o clobber\nset -o\n", "clobber        off\n"},
		{"set +o noclobber\nset -o\n", "clobber        on\n"},
		{"set -o clobber\nset -o\n", "clobber        on\n"},
		// Written through one spelling and read back through the other.
		{"set +o clobber\nset -o noclobber\nset -o\n", "clobber        off\n"},
		{"set -o noclobber\nset -o clobber\nset -o\n", "clobber        on\n"},
	} {
		out, st := negatedRunner(t, tc.src, false)
		if st != 0 || !strings.Contains(out, tc.want) {
			t.Errorf("%q:\n got %q at %d\nwant a %q row at 0", tc.src, out, st, tc.want)
		}
	}
}

// The substrate's own name is still a name this shell **has**: only the
// roster changes, and a script writing the shared spelling is not refused.
func TestTheSubstratesSpellingIsStillTaken(t *testing.T) {
	out, st := negatedRunner(t, "set -o noclobber\nprintf 'st=%s\\n' \"$?\"\n", false)
	if out != "st=0\n" || st != 0 {
		t.Errorf("got %q at %d, want %q at 0", out, st, "st=0\n")
	}
}

// `set +o` in the dialect whose line names the states that are *stored*
// writes the negated row's substrate spelling when it is off, and nothing at
// all when it is on. An ordinary row is the other way round.
func TestThePlusOLineNamesTheStoredState(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{"set +o\n", "set --default\n"},
		{"set +o clobber\nset +o\n", "set --default --noclobber\n"},
		{"set -o noclobber\nset +o\n", "set --default --noclobber\n"},
		{"set -o errexit\nset +o\n", "set --default --errexit\n"},
		{"set -o errexit\nset +o clobber\nset +o\n", "set --default --noclobber --errexit\n"},
	} {
		out, st := negatedRunner(t, tc.src, true)
		if out != tc.want || st != 0 {
			t.Errorf("%q:\n got %q at %d\nwant %q at 0", tc.src, out, st, tc.want)
		}
	}
}

// A name declared immovable is listed and refused in both directions, and it
// is left out of the `set +o` line — which is a command, so a name `set`
// would not take has no business in it.
func TestAnImmovableNameIsListedAndRefused(t *testing.T) {
	out, _ := negatedRunner(t, "set -o\n", false)
	if !strings.Contains(out, "held           off\n") {
		t.Errorf("listing = %q, want a `held off` row", out)
	}
	for _, src := range []string{"set -o held\n", "set +o held\n"} {
		out, st := negatedRunner(t, src, false)
		if st == 0 || out == "" {
			t.Errorf("%q: got %q at %d, want a refusal", src, out, st)
		}
	}
	out, st := negatedRunner(t, "set +o\n", true)
	if out != "set --default\n" || st != 0 {
		t.Errorf("got %q at %d, want %q at 0 — an immovable name is not a command",
			out, st, "set --default\n")
	}
}

// A shell that declares neither is untouched, which is the control: the
// substrate's own spelling is what four of the five presets list.
func TestWithoutTheSeamsTheSubstrateSpellingStands(t *testing.T) {
	var buf strings.Builder
	s := PosixSemantics()
	d := Diagnostics{}
	r := newTestRunner(t, &Runner{
		Stdout: &buf, Stderr: &buf, Semantics: &s, Diagnostics: &d, Name: "sh",
	})
	f, err := syntax.Parse("set -o\n", syntax.Core())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := r.Run(context.Background(), f); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "noclobber      off\n") {
		t.Errorf("listing = %q, want the substrate's own `noclobber off` row", buf.String())
	}
	if strings.Contains(buf.String(), "clobber        ") {
		t.Errorf("listing = %q, want no positive spelling", buf.String())
	}
}
