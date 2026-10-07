// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

// Commands a dialect gives its own prelude and nobody else.
//
// [diagnoseCommand], [abbreviateDirCommand] and [promptEngineCommand] are the
// interpreter's members of this family: a word the lookup answers only while
// a function the prelude defined is running, so a script that types it gets
// what the shell being imitated would give it — a command that was not found.
// This is the same seam opened to a dialect, for a capability the dialect's
// own text needs and no real shell has a builtin for.
//
// The completion dump is the first (#6307). zsh's `compinit` keeps its tables
// in a file that the next start reads back; this shell keeps one of its own,
// and the part that decides whether the file still describes `$fpath` — a
// stat of every completion file and a checksum over the dump — is Go. A
// builtin for it would be a name in `$builtins`, `whence -w` and `type` that
// real zsh does not have; a prelude-private function in front of a command
// registered here is reachable from the shipped `compinit` and invisible to
// every listing, which is the line #1117 and interp/promptengine.go draw.

// RegisterPreludeCommand adds a command that only the prelude's own functions
// can run.
//
// Called while a dialect is applied, before the first command; the table is
// read-only after that and a subshell shares it rather than copying it, the
// way it shares the prompt engine.
func (r *Runner) RegisterPreludeCommand(name string, fn Builtin) {
	if r.preludeCommands == nil {
		r.preludeCommands = map[string]Builtin{}
	}
	r.preludeCommands[name] = fn
}

// preludeCommand is the lookup's answer for a name registered above, or nil
// where the word is a script's: r.speaker is non-empty only while a function
// the prelude defined is on the stack.
func (r *Runner) preludeCommand(name string) (Builtin, bool) {
	if r.speaker == "" || r.preludeCommands == nil {
		return nil, false
	}
	fn, ok := r.preludeCommands[name]
	return fn, ok && fn != nil
}
