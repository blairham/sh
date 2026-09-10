// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package repl

import (
	"context"
	"strings"

	"github.com/blairham/sh/syntax"
)

// Hooks are the functions a session runs between commands: one before every
// prompt, one after a line has been read and before it runs.
//
// A dialect that has them names them here and this package fires them. The
// zero value is a shell with none, which is what three of the four are — see
// dialect/zsh's HookStyle for the one that has them, and PROMPT_COMMAND below
// for the neighboring mechanism that is *not* this one.
//
// # What was measured
//
// zsh 5.9.2, through a paced pseudo-terminal on 2026-09-07, with all four
// names defined in a startup file — `precmd`, `precmd_functions`, `preexec`
// and `preexec_functions` — and a marker printed by each:
//
//	[PRECMD-NAMED st=0]        ← at startup, before the first prompt
//	[PCF1 st=0] [PCF2 st=0] [PCF3 st=0]
//	[PEXP]RDY> true            ← the prompt's own $(…) ran after all four
//	[PXNAMED st=0 n=3 …]       ← after the line was read, before it ran
//	[PXF1 …] [PXF2]
//	[PRECMD-NAMED st=0] …      ← and again before the next prompt
//
// which answers the ordering questions together:
//
//   - The named function runs **first**, then the members of the array in the
//     order the array holds them. Neither list is deduplicated: with `precmd`
//     itself named in `precmd_functions` it ran twice, and a name appearing
//     twice in the array ran twice.
//   - Both fire **before the prompt is expanded**. The prompt above draws a
//     command substitution and it ran after every hook, so a hook's output
//     cannot land mid-prompt — it is above the prompt, always.
//   - `precmd` fires once at startup before the first prompt, once after every
//     accepted line including an empty one, and once after a line that would
//     not parse. It does **not** fire at a continuation prompt: typing a `for`
//     loop over four lines drew `PS2` three times and no hook ran.
//   - `preexec` fires once per accepted line and only where something will
//     run: nothing for an empty line, nothing for a line the parser refused.
//   - The job notices come first. `[1] + done sleep 0.3` was drawn, then the
//     hooks, then the prompt.
//
// # `$?`
//
// A hook is told the status of the command before it and cannot change it.
// After `(exit 7)` every hook of the chain saw `7` — including the ones after
// a hook that had returned 3 — and the next command typed still read `$?` as
// 7. So the status is put back **before each hook** as well as after the last
// one, which is two facts and not one: a chain that only restored at the end
// would show the second hook the first one's status, and zsh does not.
//
// The same holds for `preexec`, measured the same way: with the named hook
// returning 4 and the first array member returning 9, all three saw the status
// of the line before, and the line about to run read it unchanged.
//
// # Failure
//
// Nothing stops the chain. A hook returning non-zero, a name in the array with
// no function behind it, and a name whose function was removed mid-session
// were all measured, and in every case the rest of the array still ran and
// nothing was reported. A name that resolves to a *builtin* or to a file on
// PATH is passed over in silence too — only a function runs. See
// interp.CallFunction, which is where that rule lives.
//
// # The neighboring mechanism, which is a second field and not this one
//
// bash's `PROMPT_COMMAND` was measured beside these, 5.3.15 and 3.2.57, same
// harness, on 2026-09-07. It fires at the same site — before the prompt is
// expanded, at startup, after an empty line, after a parse error, never at a
// continuation prompt, always after the job notices — and it preserves `$?`
// the same way, element by element. There the resemblance stops:
// `PROMPT_COMMAND` holds **command text**, evaluated, and as an array holds
// one command string per element; zsh's hooks hold **function names**, called.
// One is `eval`, the other is a call, and a shell with one has neither the
// array-of-names nor the `preexec` half of the other.
//
// So the *site* is shared and the mechanism is not, which is why BeforePrompt
// names a function and BeforePromptVariable names a variable, rather than one
// field pretending to hold both. What they do share is the *chain* — save the
// status, run each item, put the status back before each and after the last,
// stop at an item that exited — and that is interp.Runner.FireChain, written
// once and called by both. A second loop beside it is how a fix comes to be
// carried by one caller and not the other.
//
// The chain lives in interp rather than here because a third hook fires from
// there: `chpwd` runs inside `cd`, which is a builtin, and a builtin cannot
// reach up into a front end. What this package still owns is the panic guard,
// which interp deliberately does not have.
type HookStyle struct {
	// BeforePrompt is the function run before each prompt — zsh's `precmd`.
	// Empty is a dialect without one.
	BeforePrompt string

	// BeforePromptVariable is the *variable* whose command text is evaluated
	// before each prompt — bash's `PROMPT_COMMAND`. Empty is a dialect
	// without one, which is three of the four.
	//
	// A variable's name rather than its text, because the value is read at
	// every prompt and not once at the start: measured, a `PROMPT_COMMAND`
	// that assigns to `PROMPT_COMMAND` runs the *old* text out to its end and
	// the new text from the next prompt on, which is what reading the name
	// each time gives and what holding the text could not.
	//
	// **Every element, in order, when it holds an array.** bash 5.3.15 with
	// `PROMPT_COMMAND=('echo A' 'echo B; false' 'echo C=$?')` printed `A`,
	// `B` and `C=` the status of the line *before* the prompt — so a failing
	// element stops nothing and no element sees another's status. The list is
	// the value as it stood when the prompt began: an element that replaced
	// the array mid-chain did not change what the rest of that chain ran.
	//
	// bash 3.2.57 is the one panel member that reads the same variable as a
	// scalar — the array above printed `A` alone there, which is element 0
	// and nothing after it. It is recorded here rather than made an axis
	// because this tree carries one bash, and the array form is 5.3's.
	//
	// Empty and whitespace-only text run nothing and report nothing, measured
	// in both builds, so there is no case for the chain to skip.
	BeforePromptVariable string

	// BeforeCommand is the function run after a line is read and before it
	// runs — zsh's `preexec`. Empty is a dialect without one.
	//
	// It is called with three arguments, measured rather than guessed. Given
	// `alias gg='echo aliased'` and the lines below typed at a zsh prompt:
	//
	//	typed              $1                  $2                   $3
	//	echo    a     b    echo    a     b     echo a b             echo a b
	//	gg                 gg                  echo aliased         echo aliased
	//	true;false         true;false          true; false          true⏎false
	//
	// so `$1` is the text as it was typed, and `$2` and `$3` are the command
	// that will actually run, written back out — one line and many. This
	// session has all three: `$1` is what the editor accepted, and the other
	// two are the parsed line printed, which is alias-expanded because this
	// package expands aliases while parsing a typed line (see accept).
	//
	// Two spellings of zsh's own printer differ from ours and are recorded
	// rather than chased: it writes a stray `;` after the `do` of a one-line
	// loop (`for i in 1 2; do; echo $i; done`), and it breaks a subshell over
	// three lines in `$3` where this printer keeps `( exit 7 )` on one. Both
	// are how a shell writes a tree back rather than what it will run.
	BeforeCommand string

	// CommandLayout is how the third argument to BeforeCommand is arranged —
	// the many-line form of the line about to run. The zero value keeps the
	// line structure the input had, which is also what the second argument
	// always gets.
	CommandLayout syntax.Layout

	// Unfired are hooks this dialect has and this session does not run.
	//
	// They are named rather than ignored. A hook that is registered and never
	// called is the exact failure #1281 was filed for — a startup file adds a
	// function to a list, the list is populated, and nothing reads it — so a
	// session that will not call one says so, once per name, the first time
	// it sees the name defined. That is the same three things an absent
	// parameter does (interp's absentparam.go): name it, refuse it, and let
	// nothing quietly depend on it.
	//
	// zsh's siblings on the same *calling convention* are here because they
	// do not share this *site*: `periodic` fires on a timer read from
	// `$PERIOD`; `zshaddhistory` where a line is saved, with the power to
	// reject it; `zshexit` on the way out. Measured, each takes the named
	// function and the `_functions` array exactly as `precmd` does, so one
	// implementation of the *chain* serves all of them — interp's FireHook is
	// written for any name — and each still needs its own firing site.
	//
	// `chpwd` was the fourth and is no longer here. Its site is inside `cd`,
	// which is a builtin, so it is fired from interp against
	// Semantics.DirectoryChangeHook — a name this loop never sees and could
	// not have fired without missing every `cd` inside a function. #1775.
	Unfired []string
}

// hookState is what this session has already said about a hook it will not
// fire. A pointer, because Shell is copied by value and a notice given once
// has to stay given.
type hookState struct {
	reported map[string]bool
}

// fireBeforePrompt runs the prompt hooks this dialect has: the function it
// names, and the variable whose text it evaluates. A dialect has one of them
// or neither — no shell in the panel has both.
//
// Not at a continuation prompt, which is measured: an unfinished construct
// draws PS2 and fires nothing. It reads as an ordering rule and is really the
// same rule as everywhere else in this loop — a continuation prompt is the
// middle of one line rather than the boundary between two.
func (s Shell) fireBeforePrompt(ctx context.Context, continuing bool) {
	if continuing {
		return
	}
	s.fireHook(ctx, s.Hooks.BeforePrompt)
	s.fireEvaluated(ctx, s.Hooks.BeforePromptVariable)
}

// fireEvaluated runs the chain a *variable* holds: bash's `PROMPT_COMMAND`.
//
// The value is read once, here, and the chain runs over what it read — which
// is measured rather than convenient. bash 5.3.15 with a first element that
// assigned a whole new array to the name still ran the *old* second element,
// and took the new array from the next prompt on.
//
// One value read as a list, because that is what the shell does with it: a
// scalar is one command text and an array is one per element, and GetArray
// answers both — a plain variable is a one-element array to the reader that
// underlies it, which is the same reading `${x[0]}` gets.
//
// No ordering question against the function hook above, and no measurement to
// settle one: no shell in the panel has both. They are written in the order a
// reader would guess and nothing depends on it.
func (s Shell) fireEvaluated(ctx context.Context, name string) {
	if name == "" || s.Runner == nil {
		return
	}
	// The absent case needs no branch of its own and deliberately does not
	// have one: GetArray answers a name nothing has been put under with an
	// empty list, and a chain of nothing runs nothing. A guard here would be
	// a condition no test could reach and no behavior could distinguish.
	text, _ := s.Runner.GetArray(name)
	guard := s.guard()
	s.Runner.FireChain(text, func(cmd string) {
		guard.Do(func() { _ = s.Runner.EvalVariable(ctx, name, cmd) })
	})
}

// fireBeforeCommand runs the command hook with the three arguments zsh passes,
// which are documented on HookStyle.BeforeCommand.
//
// typed is the line as accepted, without the newline the editor added: `$1` in
// zsh carries the whole of a multi-line construct with its newlines inside it
// and no newline after it, which is what take already returns minus that last
// separator.
func (s Shell) fireBeforeCommand(ctx context.Context, typed string, stmts []*syntax.File) {
	if s.Hooks.BeforeCommand == "" {
		return
	}
	line := strings.TrimSuffix(typed, "\n")
	s.fireHook(ctx, s.Hooks.BeforeCommand, line,
		s.printed(stmts, syntax.Layout{}, "; "),
		s.printed(stmts, s.Hooks.CommandLayout, "\n"))
}

// printed writes the accepted line back out in one arrangement.
//
// A line can be several files — the parser hands back one per line of input —
// so they are joined by whatever separates statements in that arrangement.
func (s Shell) printed(stmts []*syntax.File, layout syntax.Layout, sep string) string {
	parts := make([]string, 0, len(stmts))
	for _, f := range stmts {
		if text := strings.TrimSuffix(syntax.PrintFileWith(f, layout), "\n"); text != "" {
			parts = append(parts, text)
		}
	}
	return strings.Join(parts, sep)
}

// fireHook runs one *function* hook's whole chain: the function of that name,
// then every function named in its list, in order.
//
// The chain itself is interp's — see interp.Runner.FireHook, which holds every
// rule about what a hook chain is and how it runs, because the *sites* are on
// both sides of this package's boundary: `precmd` fires in this loop and
// `chpwd` fires inside `cd`, which is a builtin and cannot reach up here. What
// this package adds is the one thing interp may not have, which is the panic
// guard: nothing under interp/ recovers, and this session already runs a typed
// line behind one.
//
// Guarded per call rather than around the whole chain, for the reason a prompt
// provider is: one hook with a bug costs its own call and not the rest of the
// chain — which is also what a hook that merely *fails* does, measured.
func (s Shell) fireHook(ctx context.Context, name string, args ...string) {
	if name == "" || s.Runner == nil {
		return
	}
	guard := s.guard()
	s.Runner.FireHook(ctx, func(call func()) { guard.Do(call) }, name, args...)
}

// reportUnfiredHooks names the hooks this dialect has, this session has been
// given, and will not run — once each, the first time one is seen defined.
//
// At the prompt rather than where the hook was written, because a startup file
// is read before there is anywhere to say it and the person is not looking
// yet. The prompt is the first moment there is a session to complain to, and
// it is also the moment the hook would have fired if it were implemented.
func (s Shell) reportUnfiredHooks() {
	if s.Runner == nil || s.hooks == nil {
		return
	}
	for _, name := range s.Hooks.Unfired {
		if s.hooks.reported[name] || !s.hookIsDefined(name) {
			continue
		}
		if s.hooks.reported == nil {
			s.hooks.reported = map[string]bool{}
		}
		s.hooks.reported[name] = true
		s.errf("%s: %s: hook not implemented yet\n", or(s.Name, "sh"), name)
	}
}

// hookIsDefined reports whether anything has been put in a hook's way: a
// function of that name, or a name in its list.
//
// The list counts on its own, and that is the whole point of asking this way.
// `add-zsh-hook chpwd f` leaves no function called `chpwd` behind — it appends
// to `chpwd_functions` — so a check for the named function alone would find
// nothing and say nothing, which is the silence #1281 is about.
func (s Shell) hookIsDefined(name string) bool {
	for _, fn := range s.Runner.HookChain(name) {
		if s.Runner.HasFunction(fn) {
			return true
		}
	}
	return false
}
