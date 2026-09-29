// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import "io"

// A process substitution written as a **redirection's target** does not have
// its body traced.
//
// This engine traced every substitution's body, so `print x > >(read v; print
// B)` wrote `@ read v` and `@ print B` where the reference writes neither
// (#5089).
//
// **The rule is the substitution's *place*, not its direction**, which the
// issue's framing had the other way round and which one line settles: in
//
//	cat <(print A) < <(print B)
//
// the reference traces `print A` and not `print B` — one command, two reading
// substitutions, and the only difference between them is that the first is a
// word and the second is a redirection's target. The direction reading is
// refuted from the other side too: `: >(read v)` is a *writing* substitution as
// a word, and its body **is** traced there.
//
// Measured 2026-09-29 on zsh 5.9.2 (`/opt/homebrew/bin/zsh`; `go version -m`
// reports *not a Go executable*), script files under `env -i
// PATH=/usr/bin:/bin` with a scratch HOME and standard input on the null
// device, three runs of each row:
//
//	traced, a word            	: <(print A)  ·  cat <(print A)  ·  : >(read v)
//	                          	: >(read v; print B)  ·  : =(print A)
//	not traced, a redirection 	read v < <(print A)  ·  cat < <(print A)
//	                          	print x > >(read v)  ·  true > >(read v)
//	                          	print x >> >(read v)  ·  exec 3> >(read v)
//
// **The option is still on inside the body**, which is what makes this a
// question about the trace's destination rather than about the option: `print x
// > >(read v; [[ -o xtrace ]] && print ON)` answers `ON` in the reference. So
// the body's lines are rendered and go nowhere, and clearing `xtrace` in the
// body would have been a different change that a script can see.
func discardedTrace(*Runner) io.Writer { return io.Discard }

// bodyTraceIsDiscarded says this substitution's body writes its trace nowhere,
// which is a redirection target's and nobody else's.
//
// Read in substRunner, where the body's runner is built. A body whose trace is
// discarded hands that on to every runner it clones, so a substitution nested
// inside a redirection's body is silent too — unmeasured on the reference, and
// the reading that follows from the sink being the body's rather than the
// line's.
func (r *Runner) bodyTraceIsDiscarded() bool { return r.redirectTargetWord }
