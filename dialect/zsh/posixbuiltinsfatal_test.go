// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import "testing"

// `posixbuiltins` makes a **special** builtin's failure end a non-interactive
// shell, which is two rules and one option.
//
// `A04redirect.ztst` stops on "failed exec redir, POSIX_BUILTINS" and then,
// three chunks later, on "failed dot, POSIX_BUILTINS". They are the same
// question — POSIX ends the shell over a special builtin that fails — asked
// once about a redirection that will not open and once about a file `.`
// cannot read.
//
// Measured 2026-09-29 on zsh 5.9.2 (`/opt/homebrew/bin/zsh`), script files
// under `env -i PATH=/usr/bin:/bin` with standard input on the null device.
// **The three fields are kept apart deliberately**: the message on standard
// error is byte-identical whether the shell stops or not, so it cannot tell
// the two apart. What carries this rule is the status and whether the next
// command runs.
func TestPosixBuiltinsMakesASpecialBuiltinsFailureEndTheShell(t *testing.T) {
	for _, tc := range []struct {
		name, src, wantOut string
		wantStatus         int
	}{
		{
			"a redirection that will not open, on exec",
			"exec 3< ./no/x\nprint -r -- after\n", "", 1,
		},
		{
			"on the null command, which is special too",
			": 3< ./no/x\nprint -r -- after\n", "", 1,
		},
		{
			"on readonly",
			"readonly 3< ./no/x\nprint -r -- after\n", "", 1,
		},
		{
			"on eval",
			"eval : 3< ./no/x\nprint -r -- after\n", "", 1,
		},
		{
			"an output redirection likewise",
			"exec 3> ./no/x\nprint -r -- after\n", "", 1,
		},
		{
			"and a file `.` cannot read",
			". ./no/x\nprint -r -- after\n", "", 1,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st, _ := runZshSplit(t, t.TempDir(), "setopt posixbuiltins\n"+tc.src)
			if out != tc.wantOut || st != tc.wantStatus {
				t.Errorf("out %q status %d, want %q at %d", out, st, tc.wantOut, tc.wantStatus)
			}
		})
	}
}

// The controls, which are what keep the rule from being "with this option on,
// any failure ends the shell".
//
// Each of these fails in exactly the same words and the script runs on. The
// `command` row is the sharpest: `command set >./no/x` carries on where a
// bare `set >./no/x` does not, so **the prefix is what takes the specialness
// away** and the rule is keyed on the builtin's kind rather than on the word.
func TestWhatPosixBuiltinsDoesNotEnd(t *testing.T) {
	for _, tc := range []struct{ name, src string }{
		{"a builtin that is not special", "print -r -- x 3< ./no/x\nprint -r -- after\n"},
		{"true, likewise", "true 3< ./no/x\nprint -r -- after\n"},
		{"an external command", "/bin/echo hi 3< ./no/x\nprint -r -- after\n"},
		{"a special builtin behind `command`", "command set > ./no/x\nprint -r -- after\n"},
		{"a redirection that opens", "exec 3< /dev/null\nprint -r -- after\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st, _ := runZshSplit(t, t.TempDir(), "setopt posixbuiltins\n"+tc.src)
			if st != 0 {
				t.Errorf("status %d, want the script to run on at 0", st)
			}
			if out == "" {
				t.Errorf("out %q, want the line after the failure to have run", out)
			}
		})
	}
	// A failure inside a subshell ends the subshell and not the script.
	t.Run("a failure inside a subshell", func(t *testing.T) {
		out, st, _ := runZshSplit(t, t.TempDir(),
			"setopt posixbuiltins\n( exec 3< ./no/x ; print -r -- inner )\nprint -r -- after\n")
		if out != "after\n" || st != 0 {
			t.Errorf("out %q status %d, want only `after` at 0 — the subshell ends, the script does not", out, st)
		}
	})
}

// **The option is the key, and the emulation only sets it.**
//
// This pair is the whole reason the rule lives on the option rather than in
// the emulation table, where its redirection half sat until #4436. Each of
// the four emulations' `posixbuiltins` default happens to equal the old
// table's `redirFatal` value, so a grid over `emulate sh|ksh|csh|zsh` agrees
// in all four rows whichever of the two you believe decides — breadth along
// an axis that was never the key.
//
// These two rows hold the emulation fixed and move only the option, and they
// disagree, which is what says the option decides:
//
//	emulate sh                           the script ends, 1
//	emulate sh; unsetopt posixbuiltins   `after`, 0
//	emulate csh                          `after`, 0
//	emulate csh; setopt posixbuiltins    the script ends, 1
func TestTheOptionDecidesAndNotTheEmulation(t *testing.T) {
	for _, tc := range []struct {
		name, src string
		wantEnded bool
	}{
		{"emulate sh, option left on", "emulate sh\n", true},
		{"emulate sh with the option taken off", "emulate sh\nunsetopt posixbuiltins\n", false},
		{"emulate csh, option left off", "emulate csh\n", false},
		{"emulate csh with the option put on", "emulate csh\nsetopt posixbuiltins\n", true},
		{"a plain shell with the option put on", "setopt posixbuiltins\n", true},
		{"a plain shell", "", false},
		{"and the option taken off again", "setopt posixbuiltins\nunsetopt posixbuiltins\n", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st, _ := runZshSplit(t, t.TempDir(), tc.src+"exec 3< ./no/x\nprint -r -- after\n")
			ended := out == "" && st == 1
			if ended != tc.wantEnded {
				t.Errorf("out %q status %d: ended=%v, want %v", out, st, ended, tc.wantEnded)
			}
		})
	}
}

// And the emulations still land on the right value now that the table no
// longer carries a copy of it — the refactor's own guard.
//
// `emulate ksh` is the row that matters: it is not the sh name, and it turns
// the option on, so a rule that had been rewritten as "sh-ness" rather than
// "the option" would still pass the sh and zsh rows and fail this one.
func TestEveryEmulationStillSetsItThroughTheOption(t *testing.T) {
	for _, tc := range []struct {
		mode      string
		wantEnded bool
	}{
		{"sh", true},
		{"ksh", true},
		{"csh", false},
		{"zsh", false},
	} {
		t.Run("emulate "+tc.mode, func(t *testing.T) {
			out, st, _ := runZshSplit(t, t.TempDir(),
				"emulate "+tc.mode+"\nexec 3< ./no/x\nprint -r -- after\n")
			ended := out == "" && st == 1
			if ended != tc.wantEnded {
				t.Errorf("out %q status %d: ended=%v, want %v", out, st, ended, tc.wantEnded)
			}
		})
	}
}
