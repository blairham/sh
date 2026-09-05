// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package policy

import "strings"

// Platform aliases: two names for one place, resolved once when a rule is
// parsed rather than once per decision.
//
// The problem is narrow and worth stating exactly, because a neighboring
// problem has the opposite answer. On macOS `/tmp` is a symbolic link to
// `/private/tmp` — always, on every machine, installed by the operating system
// — so `deny path /tmp/**` written by somebody who typed the name they use is a
// rule that may protect nothing. "The policy matched nothing" and "the policy
// allowed it" are indistinguishable from outside, which makes this a
// correctness failure rather than an inconvenience.
//
// # Why this is not "resolve links in the gate"
//
// Resolving a path when a decision is made is a different thing and is
// rejected, for two reasons that both stand. It would mean the policy
// performing filesystem reads of its own, outside the boundary it is
// enforcing. And it would still be a time-of-check-to-time-of-use race,
// because an attacker-controlled link can be replaced between the check and
// the open. That limit is real, is recorded in docs/design/sandboxing.md, and
// is closed only by an operating-system backend.
//
// This is answerable earlier. An alias here is a *stable platform* alias with
// no attacker in it: it is the same on every machine, it is installed by the
// operating system, and it cannot be swapped by a script. So it is expanded
// **once, when the rule is parsed**, which costs no filesystem access at all —
// the table is a compile-time constant, and nothing here calls the operating
// system — and introduces no race, because nothing is read.
//
// # What the enforcement backends do, measured
//
// The two backends a real sandbox would delegate to both resolve at
// rule-creation time, which is the same answer arrived at from the kernel side.
//
// **Landlock** (Linux) does not take a pathname at all. A rule is an `O_PATH`
// descriptor: the name is resolved once, while the ruleset is being built, and
// the rule thereafter follows the object. There is no name left to resolve when
// a decision is made.
//
// **Seatbelt** (macOS) takes a path string, and measured with `sandbox-exec` on
// Darwin 25.5.0 it enforces on the physical one:
//
//   - `(deny file-read* (subpath "/tmp/x"))` refuses **nothing** — neither
//     `/tmp/x` nor `/private/tmp/x`.
//   - `(deny file-read* (subpath "/private/tmp/x"))` refuses **both**.
//
// So the alias failure this closes is not ours alone: the platform's own
// sandbox has it, and a profile written with `/tmp` silently protects nothing.
// A policy that disagreed with the kernel it delegates to would be worse than
// either, and after this it agrees on the case that has an answer.
//
// # Why both spellings are kept, rather than rewriting to the physical one
//
// Rewriting `/tmp/**` to `/private/tmp/**` is what a backend does, and it would
// be wrong here, because the two match different things. A backend matches
// objects: by the time the kernel decides, the name is gone. This matches
// *names as the interpreter presents them*, and the interpreter does not
// resolve links — `cat /tmp/x` reaches the gate as `/tmp/x` and `cd -P /tmp;
// cat x` reaches it as `/private/tmp/x`. Rewriting would close the second hole
// by opening the first.
//
// So a rule keeps what was written and gains the other spelling beside it, and
// matches either. That is strictly a widening: no rule stops covering anything
// it covered before, which is the property that makes this safe to apply to
// policies people have already written.
//
// # What is deliberately not here
//
// An ordinary symbolic link somebody made. `/data -> /mnt/data` is not a
// platform alias: it is not on every machine, it is not the operating system's,
// and it can change while the shell runs. Expanding it would need a filesystem
// read, which is the thing above that is rejected, and it would be a promise
// this layer cannot keep. It remains the recorded limit, and the posture is the
// mitigation: allow a subtree you control rather than deny one you do not.

// alias is one stable two-name place: whatever is written, and the other name
// for it. Both directions are expanded, because a policy naming the physical
// path has the same hole in the other direction — `deny path /private/tmp/**`
// would miss `cat /tmp/x`, which is what a shell that has not resolved anything
// presents.
type alias struct{ from, to string }

// expandAlias gives the other spelling of a pattern, or the empty string when
// the pattern is not under an alias.
//
// Whole components only. `/tmpfoo` is not under `/tmp` and must not become
// `/private/tmpfoo`, which is the same rule the gate's own prefix matching
// follows and for the same reason: a neighbor whose name starts the same way is
// a different directory.
//
// One hop. `/tmp` expands to `/private/tmp` and stops; following the result
// back through the table would loop, and there is nothing to gain — a table of
// two-name places has no chains in it.
func expandAlias(pattern string, table []alias) string {
	for _, a := range table {
		if rest, ok := underComponent(pattern, a.from); ok {
			return a.to + rest
		}
	}
	return ""
}

// underComponent reports whether pattern is prefix itself or lies beneath it,
// and returns what follows — "" for the directory itself, "/x/**" for what is
// under it.
func underComponent(pattern, prefix string) (rest string, ok bool) {
	if pattern == prefix {
		return "", true
	}
	if strings.HasPrefix(pattern, prefix+"/") {
		return pattern[len(prefix):], true
	}
	return "", false
}
