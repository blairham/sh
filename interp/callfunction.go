// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import "context"

// CallFunction runs a shell function by name with the given words as its
// positional parameters, and reports whether there was one to run.
//
// It is the call a *front end* needs and a script never does: a script names a
// function in its own text and the interpreter finds it on the way past, but a
// prompt loop has to run a function whose name it read out of a variable, with
// arguments it computed, at a moment no shell text mentions. That is what a
// hook is — see repl's hooks.go — and without it the only way to reach a
// function from outside was to build the text of a call and parse it, which
// would quote the arguments back through the grammar and get `preexec` an
// argument that was never typed.
//
// A *builtin* needs it for the same reason, which is the second caller: zsh's
// `autoload -X` resolves the function it is running inside and then runs what
// it resolved, from inside the builtin, with the positional parameters of the
// call it replaced. See dialect/zsh's autoloadRunResolved. The two callers
// want the same three things — a name that must be a function, arguments that
// are not shell text, and the function's own status left in `$?` — which is
// why this is one method rather than a hook-shaped one.
//
// **The name has to be a function.** A builtin, an alias or a command on the
// path by that name is not one, and this answers `false` for all three rather
// than running them. Measured on 2026-09-07, zsh 5.9.2 through a pseudo
// terminal, with `precmd_functions=(precmd builtin_echo_zz print /bin/echo pcX
// pcX)`: `print` is a builtin and did not run, `/bin/echo` is on the path and
// did not run, the undefined name ran nothing and said nothing, and the two
// names that *are* functions ran — `precmd` a second time on top of its own
// call, and `pcX` twice. So the rule is the name resolving to a function, the
// list is not deduplicated against anything, and a name that resolves to
// nothing is passed over in silence.
//
// The status is the function's, left in `$?` exactly as a call in a script
// leaves it. A caller that must not disturb the shell's status saves it and
// puts it back; every hook site does, because no shell in the panel lets a
// hook's status reach the next command — see repl.Shell.fireHook.
// **The body is the script's, not the caller's.** A builtin that calls a
// function is a route into shell code and not a frame the code belongs to, so
// nothing the body reports is located as the builtin's: the dialect that names
// a builtin in a diagnostic's location said `errf:builtin:2: command not
// found: …` for a missing command inside a function `autoload` had just
// loaded, where zsh says `errf:2:` — the same words a function defined in the
// script gets. The word `builtin` there was the autoload stub's own
// implementation showing through. It is the rule `command` and `.` already
// follow for the text they run; see biCommand.
func (r *Runner) CallFunction(ctx context.Context, name string, args ...string) (bool, error) {
	fn, ok := r.funcs[name]
	if !ok {
		return false, nil
	}
	// The context the call runs under, for the length of the call. A hook
	// fires between two chunks rather than inside one, so there is no
	// RunPart in progress to have set this, and a function that starts a
	// command would otherwise run it under whatever the last chunk carried.
	saved := r.ctx
	r.ctx = ctx
	defer func() { r.ctx = saved }()
	outerBuiltin := r.inBuiltin
	r.inBuiltin = ""
	defer func() { r.inBuiltin = outerBuiltin }()
	return true, r.callFunc(ctx, fn, args)
}

// HasFunction reports whether a name is a function this shell has defined.
//
// The question [Runner.CallFunction] answers by calling, asked without
// calling: a front end that wants to *say* a hook exists and will not be run —
// see repl's hooks.go and the honest-refusal rule in AGENTS.md — has to be
// able to see one without firing it.
func (r *Runner) HasFunction(name string) bool {
	_, ok := r.funcs[name]
	return ok
}
