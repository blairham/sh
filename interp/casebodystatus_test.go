// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import "testing"

// What `$?` reads inside a `case` arm's body.
//
// A `case` that runs nothing reports 0, and that zero was written before the
// arms were tested — so it reached the **body**, and every arm opened with 0
// however the command above the `case` had gone. Measured 2026-09-24 over a
// script file under `env -i PATH=/usr/bin:/bin`, with a function returning 5
// on the line above:
//
//	case x in x) echo "$?";; esac
//
//	bash 5.3.20, zsh 5.9, ksh93u+, dash 0.5.12, BusyBox ash 1.37   5
//	here, before                                                   0
//
// Unanimous, so this is the shell's own reading and not an axis. It was the
// last unclassified line of `suite: assoc.tests` (#4179), where a `wait -p …
// -n` reports a job's status and the arm that matches prints it.
//
// The zero still belongs to the construct: a `case` that matches nothing, and
// one whose chosen arm has an **empty** body, both report 0 over a preceding
// failure — which is why the status is handed over only where the arm has
// commands in it, rather than restored around the whole clause.
func TestACaseArmsBodyReadsTheStatusFromBeforeTheCase(t *testing.T) {
	const pre = "five() { return 5; }\nfive\n"
	for _, c := range []struct{ name, src, want string }{
		{"a matched arm", "case x in x) printf %s \"$?\";; esac", "5"},
		{"the default arm", "case x in y) ;; *) printf %s \"$?\";; esac", "5"},
		{"an empty subject", "case \"\" in \"\") printf %s \"$?\";; esac", "5"},
		// And the three the zero still owns.
		{"no arm matches", "case x in y) ;; esac; printf %s \"$?\"", "0"},
		{"an empty body", "case x in x) ;; esac; printf %s \"$?\"", "0"},
		{"the body's own last command", "case x in x) false;; esac; printf %s \"$?\"", "1"},
		// And the subject's own commands are what the body reads, which is
		// the half a "the status from before the case" reading gets wrong:
		// the substitution ran and left 0.
		{"a subject that ran a command", "case $(printf x) in x) printf %s \"$?\";; esac", "0"},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, _ := run(t, pre+c.src+"\n", nil)
			if out != c.want {
				t.Errorf("wrote %q, want %q", out, c.want)
			}
		})
	}
}
