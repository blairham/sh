// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

// ScriptFile is the path of the script this shell was given, **as the caller
// wrote it**, and empty where the program did not arrive as a named file.
//
// The path and not a resolved one, which is the fact a dialect with a
// parameter for it needs: measured 2026-09-27, the reference writes
// `zs.zsh` for a relative operand, `./zs.zsh` for one written that way and
// the absolute path for an absolute one — the same three answers `$0` gives,
// from the same word.
//
// The read half of [Runner.SetScriptFile], which the front end fills in
// because which operand is the script is the invocation's question. Empty
// says the shell has no script frame at all — a command string, standard
// input, a prompt — which is a state a dialect's parameter can be *absent*
// in rather than empty in; see [Runner.SetDynamicPresence].
//
// It is not `$0`. `$0` follows the call stack where a dialect says so, so
// inside a function or a sourced file it names that instead; this is the
// outermost fact and does not move. See Runner.dollarZero.
func (r *Runner) ScriptFile() string { return r.scriptFile }
