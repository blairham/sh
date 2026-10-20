// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"strings"
	"testing"

	"github.com/blairham/sh/dialect/zsh"
	"github.com/blairham/sh/interp"
)

// A `( … )` is a process of its own here, so a redirection target that will
// not expand costs the parentheses and not the script — where the same target
// on a group, a builtin, a function or a loop ends this shell at 1.
//
// Measured 2026-09-26 on `/opt/homebrew/bin/zsh -f` — `zsh 5.9.2
// (aarch64-apple-darwin25.4.0)`, `not a Go executable` by `go version -m` —
// over a script file under `env -i PATH=/usr/bin:/bin` with a scratch HOME,
// with the failure on the redirection's **own line** so that a give-up that
// costs the *line* and one that costs the *shell* are told apart by a marker
// after a `;` rather than by a marker on the next line, which both readings
// print.
//
//	( echo RAN ) > $(( 1/0 ))     reference   before
//	rest of its own line runs     yes         no
//	next line runs, at st=1       yes         no
//	`||` catches it               yes         no
//	`> "${u:=made}"` leaves u     unset       made
//
// The group row is the control and is fatal in both, so this is the
// parentheses and not "we are fatal about everything" (#4695).

func TestASubshellsFailedTargetCostsOnlyTheParenthesesHere(t *testing.T) {
	t.Parallel()
	t.Run("the rest of its own line runs", func(t *testing.T) {
		t.Parallel()
		out, st := runZsh(t, t.TempDir(), "( echo RAN ) > $(( 1/0 )) ; echo SAME\necho NEXT\n")
		if strings.Contains(out, "RAN") {
			t.Errorf("out = %q, want the subshell left unrun", out)
		}
		if !strings.Contains(out, "SAME") || !strings.Contains(out, "NEXT") {
			t.Errorf("out = %q (status %d), want the rest of the line and the next line", out, st)
		}
	})
	t.Run("the next line reads 1", func(t *testing.T) {
		t.Parallel()
		out, st := runZsh(t, t.TempDir(), "( echo RAN ) > $(( 1/0 ))\necho \"after st=$?\"\n")
		if !strings.Contains(out, "after st=1") || st != 0 {
			t.Errorf("out = %q (status %d), want the script alive with 1 behind it", out, st)
		}
	})
	t.Run("|| catches it", func(t *testing.T) {
		t.Parallel()
		out, _ := runZsh(t, t.TempDir(), "( echo RAN ) > $(( 1/0 )) || echo CAUGHT\n")
		if !strings.Contains(out, "CAUGHT") {
			t.Errorf("out = %q, want the failure caught", out)
		}
	})
	t.Run("the write the target made is lost", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		out, _ := runZsh(t, dir, "unset u\n( : ) > \"${u:=made}\"\necho \"u=[${u-unset}]\"\n")
		if !strings.Contains(out, "u=[unset]") {
			t.Errorf("out = %q, want the assignment confined to the subshell", out)
		}
	})
}

// The control the issue names, re-measured rather than quoted: a **group**
// with the same target ends this shell, so the difference is the parentheses.
// A builtin, a function and a loop are the same side of that line.
func TestThisShellsOwnCommandsStillEndOverAFailedTargetHere(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ name, cmd string }{
		{"a group", "{ echo RAN; }"},
		{"a special builtin", ":"},
		{"a regular builtin", "echo RAN"},
		{"a function", "f"},
		{"a loop", "for i in 1; do echo RAN; done"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			src := "f() { echo RAN; }\n" + tc.cmd + " > $(( 1/0 )) ; echo SAME\necho NEXT\n"
			out, st := runZsh(t, t.TempDir(), src)
			if strings.Contains(out, "SAME") || strings.Contains(out, "NEXT") {
				t.Errorf("out = %q, want the shell ended over it", out)
			}
			if st != 1 {
				t.Errorf("status = %d, want 1", st)
			}
		})
	}
}

// **The noun is the parentheses, not "a child".** These three are children of
// this shell without being a `( … )`, and every one of them contains the same
// failure here — they did before this change and they do after, because what
// contains them is the pipeline, the background job and the substitution, not
// the axis. A rule keyed on "a child" would be right about these rows by
// accident and wrong about ksh93's, which keeps the write a subshell's target
// makes and loses the one a command's makes.
func TestAChildThatIsNotParenthesesWasAlreadyContainedHere(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ name, src string }{
		{"a group in a pipeline", "{ echo RAN; } > $(( 1/0 )) | cat ; echo SAME\necho NEXT\n"},
		{"a background group", "{ echo RAN; } > $(( 1/0 )) & wait\necho NEXT\n"},
		{"a group in a substitution", "v=$( { echo RAN; } > $(( 1/0 )) ) ; echo SAME\necho NEXT\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			out, _ := runZsh(t, t.TempDir(), tc.src)
			if !strings.Contains(out, "NEXT") {
				t.Errorf("out = %q, want the script alive", out)
			}
			if strings.Contains(out, "RAN") {
				t.Errorf("out = %q, want the command left unrun", out)
			}
		})
	}
}

// And the parentheses reach wherever they are written.
func TestASubshellsFailedTargetReachesAFunctionBodyAndANestingHere(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ name, src, want string }{
		{
			"inside a function",
			"g() { ( echo RAN ) > $(( 1/0 )); echo AFTERINFUNC; }\ng\necho NEXT\n",
			"AFTERINFUNC",
		},
		{
			"inside another subshell",
			"( ( echo RAN ) > $(( 1/0 )) )\necho NEXT\n",
			"NEXT",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			out, _ := runZsh(t, t.TempDir(), tc.src)
			if strings.Contains(out, "RAN") {
				t.Errorf("out = %q, want the subshell left unrun", out)
			}
			if !strings.Contains(out, tc.want) || !strings.Contains(out, "NEXT") {
				t.Errorf("out = %q, want %q and the script alive", out, tc.want)
			}
		})
	}
}

func TestZshSaysASubshellsTargetExpandsInTheSubshell(t *testing.T) {
	t.Parallel()
	if got, want := zsh.Semantics().RedirectTargetOnASubshellExpandsInTheSubshell, interp.Yes; got != want {
		t.Errorf("RedirectTargetOnASubshellExpandsInTheSubshell = %v, want %v", got, want)
	}
}
