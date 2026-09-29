// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"regexp"
	"strings"
	"testing"
)

// A process substitution written as a **redirection's target** does not have its
// body traced (#5089).
//
// This engine traced every substitution's body, so `print x > >(read v; print
// B)` wrote `@ read v` and `@ print B` where the reference writes neither.
//
// Measured 2026-09-29 on zsh 5.9.2 (`/opt/homebrew/bin/zsh`; `go version -m`
// reports *not a Go executable*, so the reference is that shell and not another
// build of this one), script files under `env -i PATH=/usr/bin:/bin` with a
// scratch HOME and standard input on the null device, three runs of each row.
//
// **The rule is the substitution's place, not its direction**, which is the
// issue's framing corrected — see TestOneCommandDecidesItByPlaceAndNotDirection,
// which is the row that settles it.
var traceLine = regexp.MustCompile(`(?m)^@ .*$`)

// tracedLines is the `@ `-prefixed lines of a run's standard error.
//
// This harness has no PATH, so `sleep 0` is traced and then not found; the
// diagnostic that follows is the fixture's and not the shell's answer.
func tracedLines(t *testing.T, dir, src string) []string {
	t.Helper()
	_, _, errs := runZshSplit(t, dir, "PS4='@ '\nsetopt xtrace\n"+src)
	return traceLine.FindAllString(errs, -1)
}

func TestARedirectionTargetsBodyIsNotTraced(t *testing.T) {
	dir := t.TempDir()
	// Each of these has no body line at all, so the whole sequence is
	// deterministic and is asserted as one.
	for _, tc := range []struct {
		name, src string
		want      []string
	}{
		{"a reading substitution as a target", "read v < <(print A)\n:\n", []string{"@ read v", "@ :"}},
		{"and with a command that reads it", "cat < <(print A)\n:\n", []string{"@ cat", "@ :"}},
		{"a writing substitution as a target", "print x > >(read v)\n:\n", []string{"@ print x", "@ :"}},
		{
			"with more than one command in the body",
			"print x > >(read v; print B)\n:\n",
			[]string{"@ print x", "@ :"},
		},
		{"under a different operator", "print x >> >(read v)\n:\n", []string{"@ print x", "@ :"}},
		{"with a command that writes nothing", "true > >(read v)\n:\n", []string{"@ true", "@ :"}},
		{"inside a function", "f(){ read v < <(print A) }\nf\n:\n", []string{"@ f", "@ read v", "@ :"}},
		{
			"and inside a loop's own redirection",
			"while read v < <(print A); do break; done\n:\n",
			[]string{"@ read v", "@ break", "@ :"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := tracedLines(t, dir, tc.src)
			if strings.Join(got, "\n") != strings.Join(tc.want, "\n") {
				t.Errorf("traced %q, want %q", got, tc.want)
			}
		})
	}
}

// A substitution written as a **word** is still traced, in either direction.
//
// Presence rather than position: where the body's line falls among the parent's
// is #5088 and is a race in the reference, so asserting it here would be
// asserting the other issue.
func TestAWordSubstitutionsBodyIsStillTraced(t *testing.T) {
	dir := t.TempDir()
	for _, tc := range []struct{ name, src, want string }{
		// A reading substitution whose body nothing waits for is **not**
		// asserted here: whether its line is out by the time the script ends
		// is #5088's race and cannot be pinned, so a row for bare
		// `: <(print A)` would flake. The reading direction is covered by the
		// row below, where the command opens the pipe, and by
		// TestOneCommandDecidesItByPlaceAndNotDirection.
		{"reading, with a command that reads it", "cat <(print A) > /dev/null\n:\n", "@ print A"},
		// The writing direction **as a word** is traced too, which is what
		// refutes reading the rule as `>(…)` against `<(…)`.
		{"writing", ": >(read v)\n:\n", "@ read v"},
		{"writing, more than one command", ": >(read v; print B) < /dev/null\n:\n", "@ print B"},
		{"and the file spelling", ": =(print A)\n:\n", "@ print A"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := tracedLines(t, dir, tc.src)
			if !strings.Contains(strings.Join(got, "\n"), tc.want) {
				t.Errorf("traced %q, want it to carry %q", got, tc.want)
			}
		})
	}
}

// **One command settles it.** Two reading substitutions, one a word and one a
// redirection's target: the word's body is traced and the target's is not, so
// the rule cannot be about the direction and cannot be about the command.
func TestOneCommandDecidesItByPlaceAndNotDirection(t *testing.T) {
	dir := t.TempDir()
	got := strings.Join(tracedLines(t, dir, "cat <(print A) < <(print B) > /dev/null\n:\n"), "\n")
	if !strings.Contains(got, "@ print A") {
		t.Errorf("traced %q, want the word's body traced", got)
	}
	if strings.Contains(got, "@ print B") {
		t.Errorf("traced %q, want the target's body silent", got)
	}
}

// A substitution nested **inside** a silenced body is silent too, which is the
// row that says the sink travels with the body rather than being decided again
// for each substitution.
func TestASubstitutionInsideASilencedBodyIsSilentToo(t *testing.T) {
	dir := t.TempDir()
	got := strings.Join(tracedLines(t, dir, "read v < <(: <(print INNER))\n:\n"), "\n")
	if strings.Contains(got, "INNER") {
		t.Errorf("traced %q, want the nested body silent as well", got)
	}
	if !strings.Contains(got, "@ read v") {
		t.Errorf("traced %q, want the command itself still traced", got)
	}
}

// The option is **on** inside a silenced body, which is what makes this a
// question about where the lines go rather than about the option: a body that
// asks answers `ON`, and one that traces its own line by hand is heard.
func TestTheSilencedBodyStillHasTheOptionOn(t *testing.T) {
	dir := t.TempDir()
	out, st := runZsh(t, dir,
		"setopt xtrace\nprint x > >(read v; [[ -o xtrace ]] && print ON || print OFF)\n")
	if !strings.Contains(out, "ON") || st != 0 {
		t.Errorf("out %q status %d, want the body to answer ON", out, st)
	}
}
