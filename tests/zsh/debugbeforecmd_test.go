// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"strings"
	"testing"
)

// `DEBUG_BEFORE_CMD` decides *where* the DEBUG trap fires: ahead of each
// command, which is zsh's default, or behind it.
//
// It was accepted and then ignored until #4473 — the option went into the
// recorded store, the shell went on firing ahead of the command, and the two
// states of it produced byte-identical output. That is what these rows are
// written to catch: every case below is run in **both** states, so a shell
// that ignores the option fails one half of every pair rather than passing a
// row that only ever asked it one question.
//
// Measured 2026-09-25 against `/opt/homebrew/bin/zsh` — zsh 5.9.2
// (aarch64-apple-darwin25.4.0) — with `-f` and each snippet in a file of its
// own, so the `$LINENO` in the expectations is the file's own numbering and
// the `setopt` line the two columns differ by is line 1 of both.
func TestDebugBeforeCmdDecidesWhereTheTrapFires(t *testing.T) {
	for _, c := range []struct {
		name, src, behind, ahead string
	}{
		{
			// The issue's own reduction. The first line of the `behind`
			// column is the finding: the `trap` command that *sets* the
			// trap fires for itself, because the placement is decided
			// once the command has run and by then there is a trap. The
			// `ahead` column takes the decision a command too early and
			// writes nothing for it.
			name: "two simple commands",
			src: "trap 'print \"T@$LINENO\"' DEBUG\n" +
				"print A\n" +
				"print B\n",
			behind: "T@2\nA\nT@3\nB\nT@4\n",
			ahead:  "T@3\nA\nT@4\nB\n",
		},
		{
			// A compound's head goes behind the *whole* construct, at the
			// head's own line and after everything the body fired. The
			// same rule and not a second one, which is why the two
			// columns hold the same four events in the opposite order.
			name: "a compound head",
			src: "trap 'print \"T@$LINENO\"' DEBUG\n" +
				"if true\n" +
				"then\n" +
				"  print C\n" +
				"fi\n",
			behind: "T@2\nT@3\nC\nT@5\nT@3\n",
			ahead:  "T@3\nT@3\nT@5\nC\n",
		},
		{
			// The status the action reads moves with the firing: ahead of
			// the command it is the previous command's, behind it the
			// command's own. So the `false` is read as a 1 by the firing
			// *for* it in one column and by the firing for the command
			// after it in the other.
			name: "the status the action sees",
			src: "trap 'print \"T@$LINENO st=$?\"' DEBUG\n" +
				"false\n" +
				"print end\n",
			behind: "T@2 st=0\nT@3 st=1\nend\nT@4 st=0\n",
			ahead:  "T@3 st=0\nT@4 st=1\nend\n",
		},
		{
			// A call, where `$LINENO` counts from the line the definition
			// was written on: the body's own firing is `1` in both
			// columns and only which side of `body` it stands on moves.
			name: "inside a call",
			src: "f() {\n" +
				"  print body\n" +
				"}\n" +
				"trap 'print \"T@$LINENO\"' DEBUG\n" +
				"f\n",
			behind: "T@5\nbody\nT@1\nT@6\n",
			ahead:  "T@6\nT@1\nbody\n",
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			for _, state := range []struct {
				name, setopt, want string
			}{
				{"ahead", "setopt DEBUG_BEFORE_CMD\n", c.ahead},
				{"behind", "unsetopt DEBUG_BEFORE_CMD\n", c.behind},
			} {
				t.Run(state.name, func(t *testing.T) {
					// Prepended rather than written into each snippet so
					// that both states run the same numbered source, which
					// is what lets the two columns be read against each
					// other line for line. It runs before the trap exists,
					// so it fires nothing in either state.
					out, st := runZsh(t, t.TempDir(), state.setopt+c.src)
					if out != state.want || st != 0 {
						t.Errorf("got %q status %d, want %q at 0", out, st, state.want)
					}
				})
			}
		})
	}
}

// And the state is the semantics vector's rather than a bit in the recorded
// store, which is what makes a subshell's change stay in the subshell.
//
// Measured on zsh 5.9.2, 2026-09-25: the firing for `print in` stands behind
// it, and the one for `print out` — after the subshell has closed — is ahead
// of it again.
func TestDebugBeforeCmdIsSubshellLocal(t *testing.T) {
	src := "trap 'print \"T@$LINENO\"' DEBUG\n" +
		"(unsetopt debugbeforecmd; print in)\n" +
		"print out\n"
	const want = "T@2\nT@2\nin\nT@2\nT@3\nout\n"
	out, st := runZsh(t, t.TempDir(), src)
	if out != want || st != 0 {
		t.Errorf("got %q status %d, want %q at 0", out, st, want)
	}
}

// The option still reports itself on every surface it shows on, which is the
// half that already worked and must not be traded away: moving the state onto
// the axis would be no gain if `[[ -o … ]]` and the listing then described a
// shell that no longer exists.
//
// Measured on zsh 5.9.2, 2026-09-25, `-f` throughout.
func TestDebugBeforeCmdReportsItsState(t *testing.T) {
	for _, c := range []struct{ name, src, want string }{
		{"on by default", "[[ -o debugbeforecmd ]] && print on || print off\n", "on\n"},
		{
			// The namespace's own spellings reach the axis too — a single
			// `no` prefix, and underscores ignored.
			"the underscored spelling",
			"unsetopt DEBUG_BEFORE_CMD\n[[ -o debug_before_cmd ]] && print on || print off\n",
			"off\n",
		},
		{
			"the options parameter",
			"unsetopt debugbeforecmd\nzmodload zsh/parameter\nprint -r -- \"${options[debugbeforecmd]}\"\n",
			"off\n",
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, st := runZsh(t, t.TempDir(), c.src)
			if out != c.want || st != 0 {
				t.Errorf("got %q status %d, want %q at 0", out, st, c.want)
			}
		})
	}
}

// An option that defaults on is named in the `setopt` listing by its
// negation once it has been moved, and the listing reads the axis now that
// the axis is where the state lives. Measured on zsh 5.9.2, 2026-09-25:
// `unsetopt debugbeforecmd; setopt` writes `nodebugbeforecmd` among the
// deviations.
func TestDebugBeforeCmdIsListedWhenItIsOff(t *testing.T) {
	out, st := runZsh(t, t.TempDir(), "unsetopt debugbeforecmd\nsetopt\n")
	if st != 0 {
		t.Fatalf("status %d: %q", st, out)
	}
	if !strings.Contains(out, "nodebugbeforecmd\n") {
		t.Errorf("the `setopt` listing is %q; an option turned off is named there by its negation", out)
	}
}

// `exit` is the one command the mirror does not cover, and it is measured
// rather than reasoned.
//
// Behind the command, an `exit` written at the script's own level fires
// nothing — not for itself, and not for the `{ }` or the `if` it stands
// inside, whose held firings are behind it in the same unwinding. Inside a
// **function** it does fire, at its own offset, and the unwinding fires once
// more for each caller's own line as it passes through. A sourced file goes
// with the script level and not with the call.
//
// Measured on zsh 5.9.2, 2026-09-25, `-f`, each shape in a file of its own.
func TestDebugBeforeCmdDropsTheFiringAnExitWouldHaveHeld(t *testing.T) {
	for _, c := range []struct{ name, src, want string }{
		{
			"at the script's own level",
			"unsetopt DEBUG_BEFORE_CMD\ntrap 'print \"T@$LINENO\"' DEBUG\n:\n:\nexit 0\n",
			"T@2\nT@3\nT@4\n",
		},
		{
			// The group's own held firing is behind the `exit`'s, so a
			// rule that stopped at the command rather than at the
			// unwinding would still write the head's line here.
			"inside a group",
			"unsetopt DEBUG_BEFORE_CMD\ntrap 'print \"T@$LINENO\"' DEBUG\n:\n{\n  :\n  exit 0\n}\n",
			"T@2\nT@3\nT@5\n",
		},
		{
			"inside a call",
			"unsetopt DEBUG_BEFORE_CMD\nf() {\n  :\n  :\n  :\n  exit 0\n}\n" +
				"trap 'print \"T@$LINENO\"' DEBUG\nf\n",
			"T@8\nT@1\nT@2\nT@3\nT@4\n",
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, st := runZsh(t, t.TempDir(), c.src)
			if out != c.want || st != 0 {
				t.Errorf("got %q status %d, want %q at 0", out, st, c.want)
			}
		})
	}
}

// The command that moves the option **back** is the one command with no
// firing of its own, and it is the other end of the reduction this file opens
// with. There, the `trap` that sets the trap fires for itself because the
// placement is decided once the command has run and by then there is a trap.
// Here the same lateness takes a firing away: `setopt DEBUG_BEFORE_CMD` is
// dispatched with the option off, so nothing fires ahead of it, and by the
// time its held firing would happen the shell fires ahead of commands again,
// so it does not happen either.
//
// The mirror image is the control in the same file and needs no rule:
// `unsetopt debugbeforecmd` fires **once**, ahead, and is not then fired for
// a second time behind — which is what says the placement is decided once and
// not re-decided at both ends.
//
// Measured on zsh 5.9.2 under `-f`, 2026-09-25, with the action counting its
// own firings so that two firings at one line can be told from one.
func TestTurningDebugBeforeCmdBackOnDropsThatCommandsHeldFiring(t *testing.T) {
	src := "trap 'n=$((n+1)); print \"T@$LINENO n=$n\"' DEBUG\n" +
		"unsetopt debugbeforecmd\n" +
		"print a\n" +
		"setopt debugbeforecmd\n" +
		"print c\n" +
		"trap - DEBUG\n" +
		"print \"total=$n\"\n"
	const want = "T@2 n=1\na\nT@3 n=2\nT@5 n=3\nc\nT@6 n=4\ntotal=4\n"
	out, st := runZsh(t, t.TempDir(), src)
	if out != want || st != 0 {
		t.Errorf("got %q status %d, want %q at 0", out, st, want)
	}
}
