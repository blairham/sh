// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package driver

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/blairham/sh/internal/event"
	"github.com/blairham/sh/internal/policy"
	"github.com/blairham/sh/interp"
	"github.com/blairham/sh/repl"
)

// `--policy` and `--audit`: the flags that make the gate a sandbox, on every
// binary this front end makes.
//
// # Why they are here and not in cmd/sh alone (#1826, #1334)
//
// They were `cmd/sh`'s, and the argument for that was good while it stood:
// driver is the shared front end for binaries that claim to *be* bash or zsh,
// so a flag here is a flag `./bash` accepts, and a `./bash` that accepts what
// bash refuses is not evidence of anything. What that argument could not
// survive is the shape of the thing it excluded. `-policy` on `cmd/sh` only
// means the four binaries `make install` puts on disk — the names a shebang,
// `chsh` and `login` use — cannot be sandboxed by name at all, and the one
// scenario with the strongest external pull is exactly the one with nowhere
// to put a flag: a coding agent runs `$SHELL -c '<command>'`, and there is no
// argument vector a person controls. The workaround was a wrapper script the
// user had to know to write.
//
// So the decision is the front-end rule the rest of this package already
// keeps: a feature added to one binary reaches all five. `cmd/sh` learning to
// run a script file while the dialect binaries did not is the failure this
// package exists to prevent, and `-policy` was in that position.
//
// # Why the long form shadows nothing, measured
//
// The rule above is not a licence to add any flag. What makes these two safe
// is a measurement rather than a principle — every real shell in the panel
// refuses both spellings outright, so there is no behavior to shadow:
//
//	bash 5.3.15   --policy → "--policy: invalid option"      status 2
//	bash 3.2.57   --policy → "--policy: invalid option"      status 2
//	zsh 5.9.2     --policy → "no such option: policy"        status 1
//	ksh93u+       --policy → "policy: bad option(s)"         status 2
//	dash 0.5.x    --policy → "Illegal option --"             status 2
//
// and identically for `--audit`. Measured 2026-09-12 on macOS 15, with and
// without a value, against `-c 'echo RAN'`; no shell ran the command.
//
// The **long form only**, and that is the same measurement speaking. Real
// bash *accepts* `-acp` as `-a -c -p` and sets allexport, so a single-dash
// spelling of a word here would shadow working behavior rather than add a
// flag — which is why `cmd/sh` keeps its own single-dash `-policy` and the
// dialect binaries do not get one.
//
// # What the flags mean is unchanged
//
// The boundary is drawn around the interpreter and not around the process
// tree: a policy refuses what the *shell* opens, stats and runs, and a
// command the shell was allowed to start makes its own accesses that nothing
// here sees. `allow exec /bin/cat` is `allow read /**` spelled less
// obviously. See docs/design/sandboxing.md.
//
// A policy is never discovered: no environment variable, no dotfile. One that
// could be named by the environment could be replaced by anything that can
// set it, including the sandboxed script on its way to invoking a nested
// shell. It comes from this flag or from an embedder assigning Shell.Gate,
// and from nowhere else.
//
// Neither the policy file nor the audit stream passes the gate. That is an
// exemption against the rule docs/design.md states — an access is inside the
// boundary when the path was chosen by whoever the policy is about — and the
// reason is subject versus apparatus: the script is what the policy is about,
// while these two are the policy's own machinery, and a boundary that could
// be told to stop reading its rules or stop recording what it did is not one.
// Both are also opened before there is a gate to ask.

// Gates is every gate an invocation asked for, consulted as one.
//
// Any refusal refuses, which makes composition an intersection: two policies
// together allow only what both allow. That is the same rule the policy file
// uses between its own lines, and it is the only composition that lets
// someone add a second rule set to an existing policy and be sure they
// narrowed it.
type Gates []interp.Gate

func (g Gates) Allow(ctx context.Context, a interp.Action) interp.Decision {
	for _, one := range g {
		if one.Allow(ctx, a) == interp.Deny {
			return interp.Deny
		}
	}
	return interp.Allow
}

// Sinks is every sink an invocation asked for, fed as one. A trace to watch by
// eye and an audit file to keep are different jobs and a person may want both.
type Sinks []interp.Sink

func (s Sinks) Emit(ctx context.Context, e interp.Event) {
	for _, one := range s {
		one.Emit(ctx, e)
	}
}

// AddGate returns sh with g asked about every action *in addition to*
// whatever gate it already had, and with the session marked as gated.
//
// Composed rather than replaced, because replacing is how a second `-deny`
// would silently widen a policy somebody wrote. One of a kind is installed as
// itself rather than as a list of one, so the common case pays nothing for
// the composition and a stack trace names what is actually deciding.
//
// The prompt mark travels with the gate rather than with the flag that
// produced it, so a route that installs a gate some other way is marked too
// and a route that installs none is not — see sandboxMarker. It is added at
// most once however many gates are composed: a session does not become
// sandboxed twice.
func AddGate(sh Shell, g interp.Gate) Shell {
	if g == nil {
		return sh
	}
	switch had := sh.Gate.(type) {
	case nil:
		sh.Gate = g
	case Gates:
		sh.Gate = append(had, g)
	default:
		sh.Gate = Gates{had, g}
	}
	return markGated(sh)
}

// AddSink returns sh with s fed every event in addition to whatever sink it
// already had. Fanned out rather than replaced for the reason gates are
// intersected: a person who added a plugin to a shell that was already
// keeping a record did not ask for the record to stop.
//
// An event sink alone is not a sandbox and is not marked. Watching a shell
// does not change what it may do.
func AddSink(sh Shell, s interp.Sink) Shell {
	if s == nil {
		return sh
	}
	switch had := sh.Events.(type) {
	case nil:
		sh.Events = s
	case Sinks:
		sh.Events = append(had, s)
	default:
		sh.Events = Sinks{had, s}
	}
	return sh
}

// LoadPolicy reads a policy file and returns it as a gate, along with the
// rules that gained a second name when they were read.
//
// The normalized rules come back because a caller has one more question than
// a Gate can answer: a boundary that normalizes silently is a boundary whose
// meaning is not in the file it came from, and on a platform where `/tmp` is
// another name for `/private/tmp` that is every rule about either. Nothing
// here prints them — a shell that chattered about its policy on every run
// would be worse than one that did not — so a front end with a flag whose job
// is showing what the boundary is doing reports them, and every other front
// end drops them.
func LoadPolicy(path string) (interp.Gate, []string, error) {
	p, err := policy.ParseFile(path)
	if err != nil {
		return nil, nil, fmt.Errorf("policy: %w", err)
	}
	return p, p.NormalizedText(), nil
}

// OpenAudit opens the destination for the event stream and returns it as a
// sink. A lone `-` is stderr, which is the stream a trace already uses.
//
// Appended rather than truncated, because an audit trail that erases the
// previous run on the next one is not an audit trail, and 0600 because a
// record of what a script reached for names paths that are nobody else's
// business.
//
// The closer is the file, when there is one, and it is returned rather than
// deferred because a binary's main ends with os.Exit and a defer would never
// run. Nothing is lost when it is skipped — json.Encoder writes each record
// straight through, on purpose, so a shell that dies mid-script has still
// recorded everything up to the action that killed it, and a buffer would
// lose exactly the records an investigation wants.
func OpenAudit(path string, stderr io.Writer) (interp.Sink, io.Closer, error) {
	if path == "-" {
		return event.NewEncoder(stderr), nil, nil
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return nil, nil, err
	}
	return event.NewEncoder(f), f, nil
}

// A prompt that says the session is gated.
//
// A shell running under a policy refuses things an ungated shell would do,
// and the refusal is worded as the failure it produces: a stat that is denied
// reads as a path that is not there, and an exec that is denied reads as a
// command that failed. That is deliberate — docs/design/sandboxing.md argues
// a policy must be indistinguishable from the kernel to the *script* — and it
// leaves the person at the keyboard with no way to tell a sandboxed session
// from an ordinary one until something surprising happens.
//
// So the prompt says. It is the same reason every shell in the panel draws
// `#` for root and `$` for everybody else: a session with unusual powers says
// so before you use them, and this is that in the other direction. Prior art
// outside shells does it the same way — an environment that has changed what
// a shell can reach prefixes the prompt with its name in parentheses.
//
// Here rather than in a dialect, because a provider is a code path and a
// dialect is a table of values; and here rather than in one binary, because
// the flag that installs a gate is now every binary's. It is also the
// reachable consumer of repl's prompt seam, which is why it is not a test: a
// seam nothing reaches is a seam nothing grades, and the wiring from a flag
// to a drawn prompt crosses three packages.
type sandboxMarker struct{}

// sandboxPrompt is the mark itself, with the trailing space that separates it
// from whatever PS1 is. The seam adds no separator of its own, because a
// provider that wanted none would have no way to take one back.
const sandboxPrompt = "(sandboxed) "

// Prompt draws the mark before a new command and nothing before the rest of an
// unfinished one.
//
// Not at the continuation prompt, because the mark is about the session rather
// than about the line: PS2 is the middle of a command that has already been
// marked, and repeating it on every continuation line would say the session
// became sandboxed four times.
func (sandboxMarker) Prompt(info repl.PromptInfo) string {
	if info.Continued {
		return ""
	}
	return sandboxPrompt
}

// markGated adds the mark once, however it is reached.
func markGated(sh Shell) Shell {
	for _, p := range sh.PromptProviders {
		if _, ok := p.(sandboxMarker); ok {
			return sh
		}
	}
	sh.PromptProviders = append(sh.PromptProviders, sandboxMarker{})
	return sh
}

// sandboxOption reads `--policy` and `--audit`, in either of the two forms the
// rest of this front end accepts a value in: attached with `=`, or as the next
// word.
//
// Not repeatable, unlike a front end's `-deny`, and the difference is what a
// second one would mean. Two deny paths are two rules; two policies would be
// two rule sets, and while composing them is well defined — deny wins, so the
// result is the intersection — silently reading only one of a pair somebody
// wrote is the dangerous half of that. One policy, named once, and a later
// one replaces an earlier rather than being quietly ignored.
func sandboxOption(word string, args []string, inv *invocation) (rest []string, matched bool, err error) {
	name, val, hasVal := strings.Cut(word, "=")
	var dst *string
	switch name {
	case "--policy":
		dst = &inv.policy
	case "--audit":
		dst = &inv.audit
	default:
		return args, false, nil
	}
	if !hasVal {
		// Refused rather than ignored when there is none, which is how every
		// other option here that takes an argument is refused: an invocation
		// that named a policy and did not say which is not one a shell can
		// guess at, and guessing would be a shell that ran unsandboxed.
		if len(args) < 1 {
			return nil, true, fmt.Errorf("%s requires a path", name)
		}
		val, args = args[0], args[1:]
	}
	*dst = val
	return args, true, nil
}

// installSandbox turns what the option loop read into a Gate and a Sink on the
// shell, and hands back the audit file to close.
//
// Called before the operands are read, which is not an ordering detail: the
// program a shell was pointed at is an access chosen by whoever invoked it, so
// `bash --policy p script.sh` opens the script through the gate, and an audit
// trail that recorded every file a script opened and not the script itself
// would be missing the one that chose all the others.
//
// A policy that will not load ends the invocation. A shell that quietly ran
// unsandboxed because its rules would not parse is the failure this whole
// surface exists to prevent.
func (sh Shell) installSandbox(inv invocation) (Shell, io.Closer, error) {
	if inv.policy != "" {
		g, _, err := LoadPolicy(inv.policy)
		if err != nil {
			return sh, nil, err
		}
		sh = AddGate(sh, g)
	}
	var closer io.Closer
	if inv.audit != "" {
		s, c, err := OpenAudit(inv.audit, sh.Stderr)
		if err != nil {
			return sh, nil, err
		}
		closer = c
		sh = AddSink(sh, s)
	}
	if sh.Gate != nil {
		// An embedder that assigned a Gate of its own and passed no flag gets
		// the mark too. Keyed on the gate rather than on the invocation, for
		// the reason AddGate is.
		sh = markGated(sh)
	}
	return sh, closer, nil
}
