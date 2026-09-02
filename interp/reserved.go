// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

// Names a shell must answer itself.
//
// A builtin exists because running it in a child process cannot work: `umask`
// and `ulimit` change the state a later command inherits, `alias` and `hash`
// change how the shell resolves the next word, and `type` reports what this
// shell would run rather than what some other shell would. A child changes its
// own copy and exits, and the caller sees nothing.
//
// That is not hypothetical. macOS ships `/usr/bin/umask`, `/usr/bin/type` and
// eight more, each a four-line script whose body is `builtin "$name" "$@"`.
// Lacking the builtin, this shell found one on PATH and ran it — so
//
//	umask 077; touch t
//
// exited 0, printed a plausible 0022 from the next `umask`, and left the file
// world-readable. A script tightening its umask before writing a credential got
// no error and no protection.
//
// So a name on this list is never resolved from PATH. Where the builtin is
// missing the command is refused, which is the whole of the fix: an honest
// failure in place of a silent one. Implementing them is separate work, and
// `umask` and `ulimit` need a decision first — they change *process* state,
// which this package does not touch except behind a hook.
//
// The list is measured rather than assumed. `command -v` in each panel shell
// says these nine are builtins in all four — allowing that ksh93 reaches two
// of them through an alias (`type` is `whence -v`, `hash` is `alias -t --`),
// which is still not an external.
//
// `jobs`, `fg` and `bg` have left the list, for the reason `umask` and
// `ulimit` did: they are builtins now, and a builtin is found before PATH is
// searched, so the wrappers are unreachable rather than merely refused.
//
// `fc` is deliberately absent, and it is why the list was nine rather than ten:
// dash answers `command -v fc` with `/usr/bin/fc`. It really is an external
// there, so reserving it would be this shell inventing a rule the panel does
// not have.
var reservedBuiltins = map[string]bool{
	"alias":   true,
	"hash":    true,
	"type":    true,
	"ulimit":  true,
	"umask":   true,
	"unalias": true,
}

// reservedBuiltin reports whether a name must be answered by this shell or not
// at all, rather than being looked for on PATH.
//
// It answers about the *name* and not about whether the builtin exists. Both
// callers look the builtin up first and return before reaching this, so a
// dialect that registers one of these gets it — the point is never to run a
// *child* for one, not to keep the name unimplemented. Asking here as well
// was a line no test could distinguish, which mutation found; the rule it was
// protecting lives in the two callers, and in the tests that register a
// builtin and expect it to run.
func (r *Runner) reservedBuiltin(name string) bool { return reservedBuiltins[name] }
