// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"bytes"
	"context"
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// runIndirect runs src in a dialect that has `${!x}`, which the core does not:
// it is bash's and ksh93's, and the two shells without it refuse the whole
// family rather than reading it as something else.
func runIndirect(t *testing.T, src string, setup func(*Runner)) string {
	t.Helper()
	d := syntax.Core()
	d.ParamIndirection = true
	f, err := syntax.Parse(src, d)
	if err != nil {
		t.Fatalf("parse %q: %v", src, err)
	}
	var out bytes.Buffer
	s := PosixSemantics()
	r := newTestRunner(t, &Runner{Stdout: &out, Stderr: &out, Dialect: &d, Semantics: &s})
	if setup != nil {
		setup(r)
	}
	if _, err := r.Run(context.Background(), f); err != nil {
		t.Fatal(err)
	}
	return strings.TrimSpace(out.String())
}

// `${!prefix@}` yields the names, not any value — and the two spellings differ
// exactly as `$@` and `$*` do.
func TestTheNamesWithAPrefix(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{`ZQ_a=1; ZQ_b=2; echo ${!ZQ_@}`, "ZQ_a ZQ_b"},
		{`ZQ_a=1; ZQ_b=2; printf "[%s]" "${!ZQ_@}"`, "[ZQ_a][ZQ_b]"},
		{`ZQ_a=1; ZQ_b=2; printf "[%s]" "${!ZQ_*}"`, "[ZQ_a ZQ_b]"},
		// Sorted, because a map has no order to inherit and the same script
		// must not print its names differently on different runs.
		{`ZQ_b=2; ZQ_a=1; echo ${!ZQ_@}`, "ZQ_a ZQ_b"},
		// Nothing matching is empty rather than an error.
		{`echo "[${!ZQNOSUCH_@}]"`, "[]"},
		// The prefix is a prefix, not a pattern.
		{`ZQ_a=1; XZQ_b=2; echo ${!ZQ_@}`, "ZQ_a"},
		// A name the shell produces is listed too, since it can be read.
		{`echo "${!ZQNOSUCH_@}x"`, "x"},
	} {
		if out := runIndirect(t, tc.src, nil); out != tc.want {
			t.Errorf("%s = %q, want %q", tc.src, out, tc.want)
		}
	}
}

// A name `unset` took away is not listed, because it cannot be read either.
func TestUnsetNamesAreNotListed(t *testing.T) {
	out := runIndirect(t, `ZQ_a=1; ZQ_b=2; unset ZQ_a; echo "[${!ZQ_@}]"`, nil)
	if got, want := out, "[ZQ_b]"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

// The names come from everywhere a lookup would find one, so a variable the
// shell inherited is listed beside one it set.
func TestInheritedNamesAreListed(t *testing.T) {
	withEnv := func(r *Runner) { r.Env = []string{"ZQ_env=1"} }
	out := runIndirect(t, `ZQ_set=2; echo ${!ZQ_@}`, withEnv)
	if got, want := out, "ZQ_env ZQ_set"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

// A produced parameter is listed, and stops being listed once `unset` takes
// it away — which is the one path where that has to be checked. A name the
// shell set is deleted outright, and one it inherited is already filtered out
// of the environment, so the producer's table is the only place a removed
// name survives to be listed by mistake.
func TestAProducedNameIsListedUntilItIsUnset(t *testing.T) {
	produce := func(r *Runner) {
		r.SetDynamic("ZQ_made", func(*Runner) string { return "x" })
	}
	if got, want := runIndirect(t, `ZQ_set=1; echo "[${!ZQ_@}]"`, produce), "[ZQ_made ZQ_set]"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
	if got, want := runIndirect(t, `ZQ_set=1; unset ZQ_made; echo "[${!ZQ_@}]"`, produce), "[ZQ_set]"; got != want {
		t.Errorf("after unset: got %q, want %q", got, want)
	}
}
