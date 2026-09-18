// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import (
	"context"
	"strings"
	"testing"

	"github.com/blairham/sh/internal/dialecttest"
	"github.com/blairham/sh/interp"
)

// This shell's option **namespace** reads two spellings its roster does not
// publish, and it reads them on every route into the namespace: the `set`
// builtin's operand, the invocation's `-o`, and the `--name` word.
//
// Separators come out of a name (`err-exit`, `err_exit`, `glob-star`), and a
// `no` in front of a roster name is that name off (`noerrexit`). Measured
// 2026-09-18 on ksh93u+ 2012-08-01 under
// `env -i PATH=/usr/bin:/bin LC_ALL=C`, with `HOME` and `ENV` pointed at an
// empty directory so no startup file could move a row.
//
// Two axes rather than one, because the two are independently falsifiable and
// the panel could have had either alone: Semantics.OptionNamespaceIgnoresSeparators
// and Semantics.OptionNamespaceTakesANoPrefix. They compose, which is the row
// `no_err_exit` is here for.

// namespaceRunner builds a shell that has read nothing yet, with one buffer
// for both streams, so a caller can read what an invocation said before
// anything ran on the same shell.
func namespaceRunner(t *testing.T) (*interp.Runner, *strings.Builder) {
	t.Helper()
	said := &strings.Builder{}
	r := preset.Runner(dialecttest.Base{
		Stdout: said, Stderr: said, Name: "ksh",
		Dir: t.TempDir(), Env: []string{"PATH=/usr/bin:/bin"},
	})
	r.Invocation = "ksh"
	return r, said
}

// namespaceListing runs `set -o` on a shell an invocation has already spoken
// to, and hands back only what the listing itself wrote.
func namespaceListing(t *testing.T, r *interp.Runner, said *strings.Builder) string {
	t.Helper()
	before := said.Len()
	if _, err := r.Run(context.Background(), preset.Parse(t, "set -o\n")); err != nil {
		t.Fatalf("set -o: %v", err)
	}
	return said.String()[before:]
}

// listedState reads one row out of a `set -o` listing.
func listedState(t *testing.T, listing, name string) string {
	t.Helper()
	for _, line := range strings.Split(listing, "\n") {
		f := strings.Fields(line)
		if len(f) == 2 && f[0] == name {
			return f[1]
		}
	}
	t.Fatalf("no %q row in\n%s", name, listing)
	return ""
}

// The spellings, and the state each of them leaves behind. The listing is the
// assertion rather than a status, because a name accepted and acted on by
// nothing is the defect #2884 records: every row here names a roster row and
// says which way it went.
func TestTheOptionNamespaceReadsFoldedAndNegatedNames(t *testing.T) {
	for _, c := range []struct{ written, row, want string }{
		// The name as written still wins, which is what the fold is tried
		// second for.
		{"errexit", "errexit", "on"},
		{"globstar", "globstar", "on"},
		// Separators out of the name, in both spellings and mixed.
		{"err-exit", "errexit", "on"},
		{"err_exit", "errexit", "on"},
		{"glob-star", "globstar", "on"},
		{"e-r-r_e-x-i-t", "errexit", "on"},
		// A `no` in front of a roster name is that name off, and the roster
		// still holds only the positive.
		{"noerrexit", "errexit", "off"},
		{"noglobstar", "globstar", "off"},
		{"noallexport", "allexport", "off"},
		{"nokeyword", "keyword", "off"},
		{"nobgnice", "bgnice", "off"},
		// And the two fold together.
		{"no_err_exit", "errexit", "off"},
		{"no-err-exit", "errexit", "off"},
		// The roster's own name for a state the substrate stores negated is
		// `clobber`, and the `no` spelling of it is the one every other
		// column lists. Both reach the same row from opposite directions.
		{"noclobber", "clobber", "off"},
		{"clobber", "clobber", "on"},
	} {
		t.Run(c.written, func(t *testing.T) {
			// The builtin's operand.
			out, st := answersRun(t, "set -o "+c.written+"\nset -o\n")
			if st != 0 {
				t.Fatalf("set -o %s ended at %d: %q", c.written, st, out)
			}
			if got := listedState(t, out, c.row); got != c.want {
				t.Errorf("set -o %s left %s %s, want %s", c.written, c.row, got, c.want)
			}
			// The invocation's `-o`, on a shell that has read nothing.
			r, said := namespaceRunner(t)
			if st := r.SetNamedOption(c.written, true); st != 0 || said.String() != "" {
				t.Fatalf("-o %s said %q at %d, want it taken in silence", c.written, said, st)
			}
			if got := listedState(t, namespaceListing(t, r, said), c.row); got != c.want {
				t.Errorf("-o %s left %s %s, want %s", c.written, c.row, got, c.want)
			}
			// And the `--name` word, which is the same namespace by a third
			// door: the three must not be able to disagree.
			r, said = namespaceRunner(t)
			if st := r.SetLongOption(c.written); st != 0 || said.String() != "" {
				t.Fatalf("--%s said %q at %d, want it taken in silence", c.written, said, st)
			}
			if got := listedState(t, namespaceListing(t, r, said), c.row); got != c.want {
				t.Errorf("--%s left %s %s, want %s", c.written, c.row, got, c.want)
			}
		})
	}
}

// The plus sign composes with the `no` the way the sign and the sense compose
// everywhere else in this shell: `set +o noerrexit` turns errexit **on**.
func TestANegatedNameUnderThePlusSignIsTheOptionOn(t *testing.T) {
	out, st := answersRun(t, "set -e\nset +o noerrexit\nset -o\n")
	if st != 0 {
		t.Fatalf("ended at %d: %q", st, out)
	}
	if got := listedState(t, out, "errexit"); got != "on" {
		t.Errorf("set +o noerrexit left errexit %s, want on", got)
	}
}

// What the fold does **not** reach, which is what makes it a fold and not a
// normalization. Each row is a refusal in real ksh93, and each is a different
// reason:
//
//   - case is untouched, so `ERREXIT` and `RC` are words this shell has never
//     heard of;
//   - a word the fold leaves unrecognizable is still unrecognizable, which is
//     what `no_profile` says — it is not `profile` and there is no `profile`;
//   - and the `no` comes off **once**, against the roster. This shell lists
//     `clobber` where the rest of the panel lists `noclobber`, so
//     `nonoclobber` uncovers a word the listing does not hold and is refused —
//     even though `noclobber` on its own is a spelling the same shell takes.
func TestTheFoldStopsWhereTheShellStopsIt(t *testing.T) {
	for _, word := range []string{
		"ERREXIT", "Err-Exit", "no_profile", "nonoclobber", "noxyzzy", "no",
	} {
		t.Run(word, func(t *testing.T) {
			out, st := answersRun(t, "set -o "+word+"\n")
			if st != 2 {
				t.Errorf("set -o %s ended at %d, want 2: %q", word, st, out)
			}
			if want := "set: " + word + ": bad option(s)"; !strings.Contains(out, want) {
				t.Errorf("set -o %s said %q, want %q in it", word, out, want)
			}
		})
	}
}

// A refusal echoes the word as it was written and not the name the fold
// arrived at, and `login_shell` is where the two differ: the name a script
// may not move is the roster's, and the word the reader typed is theirs.
// Measured — `set -o login-shell` is `set: login-shell: bad option(s)`.
func TestARefusalEchoesTheWordAsItWasWritten(t *testing.T) {
	for _, word := range []string{"login_shell", "login-shell", "loginshell", "nologin_shell"} {
		t.Run(word, func(t *testing.T) {
			out, st := answersRun(t, "set -o "+word+"\n")
			if st != 2 {
				t.Errorf("set -o %s ended at %d, want 2: %q", word, st, out)
			}
			if want := "set: " + word + ": bad option(s)"; !strings.Contains(out, want) {
				t.Errorf("set -o %s said %q, want %q in it", word, out, want)
			}
		})
	}
}
