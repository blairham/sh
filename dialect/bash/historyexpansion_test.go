// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import (
	"context"
	"strings"
	"testing"

	"github.com/blairham/sh/internal/dialecttest"
)

// runOptionScript runs src and reports what reached standard output and error.
func runOptionScript(t *testing.T, src string) (string, string) {
	t.Helper()
	f := preset.Parse(t, src)
	var out, errs strings.Builder
	r := preset.Runner(dialecttest.Base{Stdout: &out, Stderr: &errs})
	if _, rerr := r.Run(context.Background(), f); rerr != nil {
		t.Fatalf("run %q: %v", src, rerr)
	}
	return out.String(), errs.String()
}

// `set -H` and `set -o histexpand` are two spellings of one request, and the
// failure this holds off is the one #2426 names: a script that reached the
// state the long way being told `not implemented` while the letter granted it.
// bash lists both, so both have to move the same switch and neither may
// complain.
func TestTheLetterAndTheNameAreOneRequest(t *testing.T) {
	for _, src := range []string{"set -H\n", "set -o histexpand\n"} {
		out, errs := runOptionScript(t, src+"set -o\n")
		if errs != "" {
			t.Errorf("%q complained: %q", src, errs)
		}
		if got := listed(out, "histexpand"); got != "on" {
			t.Errorf("%q left histexpand %q, want on", src, got)
		}
	}
	// And off again, through either spelling.
	for _, src := range []string{"set -H\nset +H\n", "set -H\nset +o histexpand\n"} {
		out, errs := runOptionScript(t, src+"set -o\n")
		if errs != "" {
			t.Errorf("%q complained: %q", src, errs)
		}
		if got := listed(out, "histexpand"); got != "off" {
			t.Errorf("%q left histexpand %q, want off", src, got)
		}
	}
}

// listed reads one row out of a `set -o` listing.
func listed(out, name string) string {
	for line := range strings.SplitSeq(out, "\n") {
		n, state, ok := strings.Cut(strings.TrimSpace(line), "\t")
		if ok && strings.TrimSpace(n) == name {
			return strings.TrimSpace(state)
		}
	}
	return ""
}

// `set -o history` is the *other* state and is listed separately, because it
// is: measured on bash 5.3.20, `set -o` lists `history` and `histexpand` as
// two rows and a script with only the first expands nothing. Both were one
// refused name here until #3093.
func TestRecordingAndExpandingAreTwoStates(t *testing.T) {
	out, errs := runOptionScript(t, "set -o history\nset -o\n")
	if errs != "" {
		t.Fatalf("complained: %q", errs)
	}
	// The one that was asked for is on and the one that was not is off, which
	// is what says they are two switches rather than one under two names.
	if got := listed(out, "history"); got != "on" {
		t.Errorf("history is %q, want on", got)
	}
	if got := listed(out, "histexpand"); got != "off" {
		t.Errorf("histexpand is %q, want off", got)
	}
}
