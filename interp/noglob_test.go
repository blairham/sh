// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// noglob switches off the *filesystem* half and nothing else.
func TestNoGlobStopsPathnameExpansionOnly(t *testing.T) {
	dir := fileDir(t, "a.txt", "b.txt")
	inDir := func(r *Runner) { r.Dir = dir }
	for _, tc := range []struct{ src, want string }{
		{`echo *.txt`, "a.txt b.txt"},
		{`set -o noglob; echo *.txt`, "*.txt"},
		{`set -o noglob; set +o noglob; echo *.txt`, "a.txt b.txt"},
		// The result of an expansion reaches the same stage by another route.
		{`set -o noglob; x=*.txt; echo $x`, "*.txt"},
		// Matching is not expansion, so a pattern still matches.
		{`set -o noglob; case a.txt in *.txt) echo match;; *) echo no;; esac`, "match"},
		{`set -o noglob; [[ a.txt == *.txt ]] && echo match`, "match"},
		// A pattern that matches nothing is unaffected either way.
		{`set -o noglob; echo *.none`, "*.none"},
	} {
		if out, _ := run(t, tc.src, inDir); strings.TrimSpace(out) != tc.want {
			t.Errorf("%s = %q, want %q", tc.src, strings.TrimSpace(out), tc.want)
		}
	}
}

// Whether `-f` is the short spelling of it. Three shells say yes; the fourth
// uses `-f` for something that does not touch globbing, so a script that
// writes `set -f` there still globs.
func TestSetFTurnsOffGlobbingIsAnAxis(t *testing.T) {
	dir := fileDir(t, "a.txt")
	answer := func(a Answer) func(*Runner) {
		return func(r *Runner) {
			r.Dir = dir
			s := *r.Semantics
			s.SetFTurnsOffGlobbing = a
			r.Semantics = &s
		}
	}
	for _, tc := range []struct {
		a    Answer
		src  string
		want string
	}{
		{Yes, `set -f; echo *.txt`, "*.txt"},
		{No, `set -f; echo *.txt`, "a.txt"},
		{Yes, `set -f; set +f; echo *.txt`, "a.txt"},
		// The long name is not the axis: it means the same either way, which
		// is what makes it the spelling a portable script uses.
		{Yes, `set -o noglob; echo *.txt`, "*.txt"},
		{No, `set -o noglob; echo *.txt`, "*.txt"},
		{Unspecified, `set -o noglob; echo *.txt`, "*.txt"},
	} {
		if out, _ := run(t, tc.src, answer(tc.a)); strings.TrimSpace(out) != tc.want {
			t.Errorf("%v: %s = %q, want %q", tc.a, tc.src, strings.TrimSpace(out), tc.want)
		}
	}
}

// The long spelling covers every option this shell has, and refuses a name it
// does not — rather than accepting it and doing nothing, which would let a
// script believe it had asked for something.
func TestSetONamesEveryOption(t *testing.T) {
	// Each option is checked by what it *does*, since a name accepted and
	// ignored would look identical to one that worked.
	for _, tc := range []struct {
		src     string
		stopped bool
	}{
		{`set -o errexit; false; echo reached`, true},
		{`set -o nounset; echo "${nosuch}"; echo reached`, true},
		{`set +o errexit; false; echo reached`, false},
	} {
		out, _ := run(t, tc.src, nil)
		if got := strings.Contains(out, "reached"); got == tc.stopped {
			t.Errorf("%s: got %q", tc.src, out)
		}
	}
	// noclobber refuses the redirection and says so; it does not end the
	// script, so the file keeps what the first one put there.
	out, _ := run(t, `set -o noclobber; echo a > f; echo b > f; cat f`, func(r *Runner) { r.Dir = t.TempDir() })
	if !strings.Contains(out, "exists") || !strings.Contains(out, "a\n") {
		t.Errorf("noclobber gave %q, want a refusal and the first write kept", out)
	}
	// An unknown name is reported rather than accepted, which is what stops a
	// script believing it asked for something. The script carries on, as it
	// does in the panel.
	out, _ = run(t, `set -o nosuchopt; echo after`, nil)
	if !strings.Contains(out, "nosuchopt") {
		t.Errorf("an unknown name gave %q, want it named", out)
	}
}
