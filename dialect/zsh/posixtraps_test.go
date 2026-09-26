// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"strings"
	"testing"
)

// `POSIX_TRAPS` decides whether an EXIT trap set inside a function is the
// function's — firing at its return, which is this shell's own answer and
// nobody else's — or the shell's, firing when the shell exits, which is what
// POSIX says and what the other four dialects do without being asked.
//
// It was accepted and then ignored until #4547: the name went into the
// recorded store, `[[ -o posixtraps ]]` reported it back faithfully, and the
// trap fired at the function's return in both states. Every case below is run
// in **both** states for that reason — a shell that ignores the option fails
// one half of every pair rather than passing a row that only ever asked it
// one question.
//
// Measured against `/opt/homebrew/bin/zsh` — zsh 5.9.2
// (aarch64-apple-darwin25.4.0) — with `-f` and each snippet in a file of its
// own, 2026-09-25.
func TestPosixTrapsDefersAFunctionsExitTrap(t *testing.T) {
	for _, c := range []struct {
		name, src, on, off string
		status             int
	}{
		{
			// The issue's own reduction.
			name: "the reduction",
			src: "f() { trap 'print EXITTRAP' EXIT; print in-f }\n" +
				"f\n" +
				"print after-f\n",
			on:  "in-f\nafter-f\nEXITTRAP\n",
			off: "in-f\nEXITTRAP\nafter-f\n",
		},
		{
			// Deferring is not the same as *holding*: with the option on
			// the trap is simply the shell's, so the listing after the
			// call shows it and the caller's has been replaced rather than
			// put back. With the option off the caller's comes back.
			name: "the caller's trap is replaced rather than held",
			src: "trap 'print TOP-EXIT' EXIT\n" +
				"f() { trap 'print F-EXIT' EXIT }\n" +
				"f\n" +
				"print listing:\n" +
				"trap\n",
			on:  "listing:\ntrap -- 'print F-EXIT' EXIT\nF-EXIT\n",
			off: "F-EXIT\nlisting:\ntrap -- 'print TOP-EXIT' EXIT\nTOP-EXIT\n",
		},
		{
			// Nesting follows from that and is not a second rule: with the
			// option on there is one EXIT trap and the last writer holds
			// it, so the outer function's is gone by the time the shell
			// exits and only one line is written.
			name: "nested calls",
			src: "g() { trap 'print G-EXIT' EXIT; print in-g }\n" +
				"f() { trap 'print F-EXIT' EXIT; g; print in-f }\n" +
				"f\n" +
				"print after-f\n",
			on:  "in-g\nin-f\nafter-f\nG-EXIT\n",
			off: "in-g\nG-EXIT\nin-f\nF-EXIT\nafter-f\n",
		},
		{
			// An explicit `return` does not change which end the trap
			// fires at, and the status the caller reads is the call's in
			// both states.
			name: "an explicit return",
			src: "f() { trap 'print F-EXIT' EXIT; print in-f; return 3 }\n" +
				"f\n" +
				"print \"rc=$?\"\n",
			on:  "in-f\nrc=3\nF-EXIT\n",
			off: "in-f\nF-EXIT\nrc=3\n",
		},
		{
			// An `exit` from inside the body ends the shell, and the trap
			// runs on the way out in both states — the one shape where
			// deferring to the shell's exit and firing at the return put
			// the firing in the same place. It is here as the row that
			// must *not* move.
			name: "an exit from the body",
			src: "f() { trap 'print F-EXIT' EXIT; print in-f; exit 4 }\n" +
				"f\n" +
				"print unreached\n",
			on:     "in-f\nF-EXIT\n",
			off:    "in-f\nF-EXIT\n",
			status: 4,
		},
		{
			// Only EXIT. A trap on a real signal set inside a function is
			// the shell's to keep in both states, so the inner handler is
			// what a signal sent after the return finds.
			name: "a signal trap is not this question",
			src: "trap 'print OUTER-USR1' USR1\n" +
				"f() { trap 'print INNER-USR1' USR1; print in-f }\n" +
				"f\n" +
				"kill -USR1 $$\n" +
				"print after\n",
			on:  "in-f\nINNER-USR1\nafter\n",
			off: "in-f\nINNER-USR1\nafter\n",
		},
		{
			// And neither is ZERR, which is the condition closest to EXIT
			// in spelling and furthest from it here: set inside a
			// function, it still judges a failure after the return in both
			// states.
			name: "a ZERR trap is not this question either",
			src: "f() { trap 'print INNER-ZERR' ZERR; false; print in-f }\n" +
				"f\n" +
				"false\n" +
				"print after\n",
			on:  "INNER-ZERR\nin-f\nINNER-ZERR\nafter\n",
			off: "INNER-ZERR\nin-f\nINNER-ZERR\nafter\n",
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			for _, state := range []struct{ name, setopt, want string }{
				{"on", "setopt posixtraps\n", c.on},
				{"off", "unsetopt posixtraps\n", c.off},
			} {
				t.Run(state.name, func(t *testing.T) {
					out, st := runZsh(t, t.TempDir(), state.setopt+c.src)
					if out != state.want || st != c.status {
						t.Errorf("got %q status %d, want %q at %d", out, st, state.want, c.status)
					}
				})
			}
		})
	}
}

// The moment the option is read is the *`trap` command's*, not the function's
// return and not its entry. That is the whole of the rule and it is the thing
// a reading taken at the return would have got backwards, so it is pinned
// with a pair that holds the state at the return fixed at the opposite value
// in each direction.
//
// Measured on zsh 5.9.2, `-f`, 2026-09-25.
func TestPosixTrapsIsReadWhenTheTrapIsSet(t *testing.T) {
	for _, c := range []struct{ name, src, want string }{
		{
			// On when the `trap` ran and **off** by the return: deferred
			// all the same.
			"on at the set, off at the return",
			"setopt posixtraps\n" +
				"f() { trap 'print EXITTRAP' EXIT; print in-f; unsetopt posixtraps }\n" +
				"f\nprint after-f\n",
			"in-f\nafter-f\nEXITTRAP\n",
		},
		{
			// Off when the `trap` ran and **on** by the return: fires at
			// the return all the same. Together with the row above, the
			// state at the return takes both values on each side of the
			// answer, so it cannot be what decides.
			"off at the set, on at the return",
			"unsetopt posixtraps\n" +
				"f() { trap 'print EXITTRAP' EXIT; print in-f; setopt posixtraps }\n" +
				"f\nprint after-f\n",
			"in-f\nEXITTRAP\nafter-f\n",
		},
		{
			// And the same pair against the function's *entry*: off there
			// and on by the `trap`, which defers.
			"off at the entry, on at the set",
			"unsetopt posixtraps\n" +
				"f() { setopt posixtraps; trap 'print EXITTRAP' EXIT; print in-f }\n" +
				"f\nprint after-f\n",
			"in-f\nafter-f\nEXITTRAP\n",
		},
		{
			// On at the entry and off by the `trap`, which fires at the
			// return.
			"on at the entry, off at the set",
			"setopt posixtraps\n" +
				"f() { unsetopt posixtraps; trap 'print EXITTRAP' EXIT; print in-f }\n" +
				"f\nprint after-f\n",
			"in-f\nEXITTRAP\nafter-f\n",
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

// Whether the call *holds* the trap it inherited is a second question with a
// second moment: it is decided at the function's **entry**, where the firing
// is decided at the `trap`. The two come apart, and this is the pair that
// separates them — the bodies are identical and only the side of the call the
// `setopt` stands on differs.
//
// Measured on zsh 5.9.2, `-f`, 2026-09-25.
func TestPosixTrapsAtTheEntryDecidesWhetherTheCallersTrapComesBack(t *testing.T) {
	for _, c := range []struct{ name, src, want string }{
		{
			// Entered with the option off, so the call held TOP-EXIT and
			// puts it back over its own — which never fires, because the
			// option was on when the `trap` ran.
			"turned on inside the body",
			"unsetopt posixtraps\n" +
				"trap 'print TOP-EXIT' EXIT\n" +
				"g() { setopt posixtraps; trap 'print G-EXIT' EXIT; print in-g }\n" +
				"g\nprint after\nprint listing:\ntrap\n",
			"in-g\nafter\nlisting:\ntrap -- 'print TOP-EXIT' EXIT\nTOP-EXIT\n",
		},
		{
			// The same body with the `setopt` moved above the call.
			// Nothing was held, so G-EXIT stands.
			"turned on before the call",
			"unsetopt posixtraps\nsetopt posixtraps\n" +
				"trap 'print TOP-EXIT' EXIT\n" +
				"g() { trap 'print G-EXIT' EXIT; print in-g }\n" +
				"g\nprint after\nprint listing:\ntrap\n",
			"in-g\nafter\nlisting:\ntrap -- 'print G-EXIT' EXIT\nG-EXIT\n",
		},
		{
			// The inherited trap has to exist to come back. With no
			// top-level trap the first shape leaves the body's own
			// installed and fires it at the shell's exit, so this is not a
			// plain restore of whatever was there.
			"turned on inside the body, with nothing to come back",
			"unsetopt posixtraps\n" +
				"g() { setopt posixtraps; trap 'print G-EXIT' EXIT; print in-g }\n" +
				"g\nprint after\nprint listing:\ntrap\n",
			"in-g\nafter\nlisting:\ntrap -- 'print G-EXIT' EXIT\nG-EXIT\n",
		},
		{
			// And it is the state at the entry rather than "the option was
			// off at some point in the body": off and on again before the
			// `trap` holds nothing.
			"off and on again inside the body",
			"setopt posixtraps\n" +
				"trap 'print TOP-EXIT' EXIT\n" +
				"g() { unsetopt posixtraps; setopt posixtraps; trap 'print G-EXIT' EXIT; print in-g }\n" +
				"g\nprint after\nprint listing:\ntrap\n",
			"in-g\nafter\nlisting:\ntrap -- 'print G-EXIT' EXIT\nG-EXIT\n",
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

// `emulate sh` and `emulate ksh` turn the option on, which is what lifts this
// above an opt-in curiosity: an `emulate sh` at the head of a zsh function
// library asks for POSIX trap timing without anyone typing `setopt`.
//
// The reporting half already worked (#2549) and must not be traded away for
// the behavior, so both are asked here: what the emulation does to the option
// *and* what the shell then does with a trap.
//
// Measured on zsh 5.9.2, `-f`, 2026-09-25.
func TestEmulationTurnsPosixTrapsOn(t *testing.T) {
	out, st := runZsh(t, t.TempDir(),
		"emulate sh  ; [[ -o posixtraps ]] && print 'sh: on'\n"+
			"emulate ksh ; [[ -o posixtraps ]] && print 'ksh: on'\n"+
			"emulate zsh ; [[ -o posixtraps ]] || print 'zsh: off'\n")
	if want := "sh: on\nksh: on\nzsh: off\n"; out != want || st != 0 {
		t.Errorf("reporting: got %q status %d, want %q at 0", out, st, want)
	}
	for _, c := range []struct{ name, head, want string }{
		{"emulate sh", "emulate sh\n", "in-f\nafter-f\nF-EXIT\n"},
		{"emulate ksh", "emulate ksh\n", "in-f\nafter-f\nF-EXIT\n"},
		{"emulate -R sh", "emulate -R sh\n", "in-f\nafter-f\nF-EXIT\n"},
		// The control: the mode that leaves the option alone leaves the
		// timing alone, so a row that passed by turning the trap around
		// for every emulation would fail here.
		{"emulate zsh", "emulate zsh\n", "in-f\nF-EXIT\nafter-f\n"},
	} {
		t.Run(c.name, func(t *testing.T) {
			src := c.head +
				"f() { trap 'print F-EXIT' EXIT; print in-f }\n" +
				"f\nprint after-f\n"
			out, st := runZsh(t, t.TempDir(), src)
			if out != c.want || st != 0 {
				t.Errorf("got %q status %d, want %q at 0", out, st, c.want)
			}
		})
	}
}

// And the state is the semantics vector's rather than a bit in the recorded
// store, which is what makes a subshell's change stay in the subshell and
// what lets `localoptions` scope it to a function.
//
// Measured on zsh 5.9.2, `-f`, 2026-09-25.
func TestPosixTrapsIsScopedLikeAnOption(t *testing.T) {
	for _, c := range []struct{ name, src, want string }{
		{
			"a subshell keeps its change to itself",
			"unsetopt posixtraps\n" +
				"( setopt posixtraps; [[ -o posixtraps ]] && print IN-ON )\n" +
				"[[ -o posixtraps ]] && print OUT-ON || print OUT-OFF\n",
			"IN-ON\nOUT-OFF\n",
		},
		{
			// `localoptions` puts it back at the return, so the call after
			// the one that set it is timed the old way again.
			"localoptions puts it back at the return",
			"unsetopt posixtraps\n" +
				"h() { setopt localoptions posixtraps; print in-h }\n" +
				"h\n" +
				"f() { trap 'print F-EXIT' EXIT; print in-f }\n" +
				"f\nprint after-f\n",
			"in-h\nin-f\nF-EXIT\nafter-f\n",
		},
		{
			// The control for the row above: the same body without
			// `localoptions` leaves the option on, and the later call is
			// timed the new way.
			"and without it the change stands",
			"unsetopt posixtraps\n" +
				"h() { setopt posixtraps; print in-h }\n" +
				"h\n" +
				"f() { trap 'print F-EXIT' EXIT; print in-f }\n" +
				"f\nprint after-f\n",
			"in-h\nin-f\nafter-f\nF-EXIT\n",
		},
		{
			"the setopt listing names it",
			"setopt posixtraps\nsetopt\n",
			"",
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, st := runZsh(t, t.TempDir(), c.src)
			if st != 0 {
				t.Fatalf("status %d: %q", st, out)
			}
			if c.want == "" {
				if !strings.Contains(out, "posixtraps\n") {
					t.Errorf("the `setopt` listing is %q; an option turned on is named there", out)
				}
				return
			}
			if out != c.want {
				t.Errorf("got %q, want %q", out, c.want)
			}
		})
	}
}
