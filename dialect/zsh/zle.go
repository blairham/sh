// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/blairham/sh/interp"
	"github.com/blairham/sh/repl"
)

// The `zle` builtin: defining an editing action in shell, and running one.
//
// Measured 2026-09-07 against zsh 5.9.2 in the oracle environment, with a
// scratch HOME and no startup files — under `-c` for everything a script can
// see, and through a pseudo-terminal, one keystroke at a time, for everything
// only a widget that actually ran can see.
//
// This is the other half of what bindkey.go started, and the asymmetry is what
// the issue behind it was filed for: the binding *table* was here and the
// ability to define what a key is bound *to* was not, so an rc file that binds
// a plugin's own widget bound a name nothing answered to. `bindkey` records
// that an unknown widget name is stored in silence, which is right, and left
// the name permanently unknown.
//
// `-F`, the callback on a descriptor, was refused by name here for exactly as
// long as the read loop could only wait on the terminal. It no longer is: see
// zlewatch.go, which is that operation and the measurements behind it.
//
// **The capability is repl's and only the naming is here.** repl gained one
// seam for this — repl.Shell.RunWidget, a round trip that hands out the line
// and takes it back — and everything a shell says about it is in this file:
// that the action is called a widget, that `zle -N` defines one, that the line
// arrives in `$BUFFER` with the cursor in `$CURSOR`, and that the two halves
// either side of the cursor are `$LBUFFER` and `$RBUFFER`. Nothing under
// `repl/` knows any of those words, the same way nothing there knows that
// `up-line-or-history` is what this shell calls walking back through history.
//
// ## What was measured, and what each fact settled
//
//   - **`zle -N name` defines a widget backed by a function of the same name,
//     and `zle -N name fn` by `fn`.** Neither has to exist yet: `zle -N foo`
//     with no `foo` anywhere is status 0 and silence, and the listing shows it.
//     That is the fact that makes the order in a real startup file work, since
//     a plugin defines its widgets and its functions in whichever order suits
//     it. A definition naming a function that never arrives is a key that does
//     nothing, which is what pressing it does in zsh too — measured through a
//     pseudo-terminal, silence and a live shell.
//   - **The listing has two spellings and they are both exact.** `zle -l`
//     writes `name` where the function's name is the widget's and `name (fn)`
//     where it is not; `zle -l -L` writes the command that would define it
//     back, `zle -N name` or `zle -N name fn`. Both are sorted by widget name
//     whatever order they were defined in.
//   - **`zle -l name` is a question, not a listing.** No output, status 0 if
//     every name given is a widget and 1 if any is not — which is what
//     `zle -l foo || zle -N foo` in a plugin is asking, and the reason this
//     had to be got right rather than approximated.
//   - **`zle -la` is every widget and `zle -l` is only the ones somebody
//     defined.** Under `-f` with nothing defined, `zle -l` prints nothing at
//     all and `zle -la` prints 386 lines. This shell's `-la` is its own list
//     and not that one, for the reason bindkey.go gives for the keymaps: a
//     name in the answer is a claim that pressing a key bound to it does
//     something, and 386 names where 22 actions exist would be 364 lies.
//   - **The parameters are `local` to the widget's call**, which
//     `${(t)BUFFER}` says outright — `scalar-local-special` — and which is
//     visible from outside as well: `${BUFFER-UNSET}` is `UNSET` in a shell
//     that is not running a widget, and assigning to `BUFFER` there is an
//     ordinary variable assignment. So they are produced parameters that
//     exist for the length of the call and are taken away again, which gives
//     a function the widget calls the same view — zsh's locals are
//     dynamically scoped — and gives a script outside one nothing.
//   - **Their arithmetic is exact and each assignment is live.** With
//     `BUFFER=abcdef` and `CURSOR=2`, `$LBUFFER` is `ab` and `$RBUFFER` is
//     `cdef`. Then `LBUFFER=XY` makes `$BUFFER` `XYcdef` and `$CURSOR` 2 —
//     the cursor follows the length of what was written — and `RBUFFER=ZZ`
//     makes `$BUFFER` `XYZZ` and leaves `$CURSOR` where it was. Read back
//     *inside* the same widget, which is why these cannot be four plain
//     variables reconciled when the call returns, and why interp grew
//     SetDynamicWriter: a produced parameter a script assigns to needs
//     somewhere for the assignment to go.
//   - **The cursor is clamped rather than refused.** `CURSOR=999` on a
//     four-character line reads back 4, and `CURSOR=-5` reads back 0.
//   - **`$?` goes in and does not come out.** With `false` before the
//     keystroke, the widget function sees `$?` of 1; the widget returning 7
//     leaves the next command reading 1. The same discipline hooks already
//     run under, measured the same way.
//   - **A widget is only callable while the editor is running one.** `zle
//     some-widget` from a script is `widgets can only be called when ZLE is
//     active` at status 1, and so is `zle reset-prompt`. From inside a widget,
//     `zle other-widget` runs it against the same line — a change `other`
//     makes to `$BUFFER` is there when it returns — and a name nothing answers
//     to is status 1 in *silence*, with no diagnostic at all. Measured with
//     the widget's own stderr redirected to a file, so the redraw could not
//     have hidden one.
//   - **`$WIDGET` is the widget the editor ran, and it does not change for a
//     widget invoked from inside it**: `zle b` from `a` leaves `$WIDGET` as
//     `a` inside `b`.
//   - **The wordings.** `bad option: -x`, `not enough arguments for -N`, `too
//     many arguments for -N`, `no such widget `+"`"+`name'` — this builtin's
//     `bad option` is bindkey's and zmodload's and not zstyle's `invalid
//     option`, and its usage complaints name the letter that was short, which
//     zstyle's do not. `zle` with no arguments at all is status 1 and no
//     output whatsoever.
//
// ## What refuses by name, and why that is the point
//
// A `zle` that accepted everything would be worse than the `command not
// found` it replaces, because a plugin would then believe its widget existed.
// So the letters this shell has not got are refused with the wording `whence`
// and `bindkey` use for the same case — `-M is not implemented yet` — which a
// script can tell apart from a typo, and the spellings of *invoking* that need
// a seam repl has not got are refused by name too:
//
//   - **`zle -R`, `zle -M` and `zle reset-prompt`**, redisplay. These write to
//     the screen in the middle of a widget rather than changing the line, so
//     they belong with the question of who owns the prompt while a widget is
//     running.
//   - **`zle <one of the editor's own actions>`** — `zle end-of-line` from
//     inside a widget. The name resolves perfectly well; what it would take is
//     for a shell function to reach back into the editor mid-keystroke, which
//     is re-entering the read loop rather than transforming the line.
//
// `vared` is left out entirely, and so are `zcompile` and `zregexparse`: the
// first two are separate features and the third belongs with the completion
// system, which this shell has not got.

// zleStore is the widget table: a flat array of pairs, widget name then the
// function behind it.
//
// zleBuffer, zleCursor, zleWidget and zleActive are the state of the call a
// widget is running inside, and zleActive is what tells a widget apart from a
// script — `zle other-widget` is refused outside one.
//
// All five in the Runner's own tables under names no script can spell, the way
// bindkey.go keeps its bindings, which is also what gives a subshell its own
// copy.
const (
	zleStore  = ".zsh.zle"
	zleBuffer = ".zsh.zle.buffer"
	zleCursor = ".zsh.zle.cursor"
	zleWidget = ".zsh.zle.widget"
	zleActive = ".zsh.zle.active"
)

// zleParameters are what a widget function reads the line as, and they exist
// only while one is running. See the file comment.
var zleParameters = []string{"BUFFER", "CURSOR", "LBUFFER", "RBUFFER", "WIDGET"}

// registerZle installs the builtin.
func registerZle(r *interp.Runner) {
	r.Register("zle", zleBuiltin)
}

// The letters this builtin has, and the ones it has and this shell has not.
// Split so a letter zsh does not have is `bad option` and a letter it has that
// is not built yet says so — the distinction whence.go documents.
const (
	zleLetters            = "acfglmrwACDFGIKLMNRTU"
	zleLettersImplemented = "aADFLNlw"
)

// zleOpts is what the letters asked for.
type zleOpts struct {
	define bool // -N
	delete bool // -D
	alias  bool // -A
	list   bool // -l
	watch  bool // -F
	all    bool // -a
	source bool // -L
	widget bool // -w
}

func zleBuiltin(r *interp.Runner, ctx context.Context, args []string) int {
	var opts zleOpts
	rest := args
	for len(rest) > 0 && strings.HasPrefix(rest[0], "-") && rest[0] != "-" {
		if rest[0] == "--" {
			rest = rest[1:]
			break
		}
		if rest[0][1] >= '0' && rest[0][1] <= '9' {
			// A word beginning `-` and then a digit is an operand and not
			// options, measured: `zle -0` is an attempt to *call* a widget
			// called `-0` and says `widgets can only be called when ZLE is
			// active`, and `zle -F -3` gets as far as complaining about the
			// descriptor number rather than about an option `-3`. Which is
			// also the whole reason `zle -F -0` appears to remove a watcher:
			// it is the number nought reaching the descriptor parser.
			break
		}
		for _, letter := range rest[0][1:] {
			if !strings.ContainsRune(zleLetters, letter) {
				r.Diagnosef("bad option: -%c\n", letter)
				return 1
			}
			if !strings.ContainsRune(zleLettersImplemented, letter) {
				// A letter this shell has not got says so, rather than being
				// accepted and doing nothing — see the file comment.
				r.Diagnosef("-%c is not implemented yet\n", letter)
				return 1
			}
			setZleLetter(&opts, letter)
		}
		rest = rest[1:]
	}
	switch {
	case opts.define:
		return defineWidget(r, rest)
	case opts.delete:
		return deleteWidgets(r, rest)
	case opts.alias:
		return aliasWidget(r, rest)
	case opts.list:
		return listWidgets(r, opts, rest)
	case opts.watch:
		return watchDescriptor(r, opts, rest)
	case len(rest) == 0:
		// `zle` with nothing at all: status 1 and not a word, measured.
		return 1
	}
	return callWidget(r, ctx, rest[0], rest[1:])
}

func setZleLetter(opts *zleOpts, letter rune) {
	switch letter {
	case 'N':
		opts.define = true
	case 'D':
		opts.delete = true
	case 'A':
		opts.alias = true
	case 'l':
		opts.list = true
	case 'F':
		opts.watch = true
	case 'w':
		// A modifier and not an operation: measured, `zle -w` alone is the
		// bare `zle` — status 1 and not a word — and `zle -N -w a f` defines
		// `a` at status 0 with the letter making no difference. It changes
		// only what `-F` arms.
		opts.widget = true
	case 'a':
		opts.all = true
	case 'L':
		opts.source = true
	}
}

// defineWidget is `zle -N name [function]`.
//
// The function defaults to the widget's own name, and neither it nor the name
// has to exist yet — see the file comment for why that is load-bearing rather
// than lenient.
func defineWidget(r *interp.Runner, args []string) int {
	switch {
	case len(args) == 0:
		r.Diagnosef("not enough arguments for -N\n")
		return 1
	case len(args) > 2:
		r.Diagnosef("too many arguments for -N\n")
		return 1
	}
	fn := args[0]
	if len(args) == 2 {
		fn = args[1]
	}
	writeWidget(r, args[0], fn)
	return 0
}

// deleteWidgets is `zle -D name...`, and a name nothing answers to costs the
// status rather than the rest of the list.
func deleteWidgets(r *interp.Runner, args []string) int {
	if len(args) == 0 {
		r.Diagnosef("not enough arguments for -D\n")
		return 1
	}
	status := 0
	for _, name := range args {
		if !removeWidget(r, name) {
			r.Diagnosef("no such widget `%s'\n", name)
			status = 1
		}
	}
	return status
}

// aliasWidget is `zle -A old new`: a second widget running the same function.
//
// Measured, the copy is indistinguishable from a definition — `zle -A w x`
// then `zle -l -L` writes `zle -N x f` — so this stores one rather than
// keeping a notion of an alias. What it will not do is copy one of the
// editor's own actions, which would need a widget name that stands for a
// Widget rather than for a function; that refuses by name.
func aliasWidget(r *interp.Runner, args []string) int {
	switch {
	case len(args) < 2:
		r.Diagnosef("not enough arguments for -A\n")
		return 1
	case len(args) > 2:
		r.Diagnosef("too many arguments for -A\n")
		return 1
	}
	if fn, defined := widgetFunction(r, args[0]); defined {
		writeWidget(r, args[1], fn)
		return 0
	}
	if _, editors := bindkeyWidgets[args[0]]; editors {
		r.Diagnosef("-A of a built-in widget is not implemented yet\n")
		return 1
	}
	r.Diagnosef("no such widget `%s'\n", args[0])
	return 1
}

// listWidgets is `zle -l`, with `-a` for every widget rather than the defined
// ones and `-L` for the command that would define each back.
//
// With names, it is a question and not a listing: nothing is printed unless
// `-L` asked for it, and the status says whether every name given is a widget.
func listWidgets(r *interp.Runner, opts zleOpts, names []string) int {
	defined := readWidgets(r)
	if len(names) > 0 {
		status := 0
		for _, name := range names {
			if !widgetExists(defined, name, opts.all) {
				status = 1
				continue
			}
			if opts.source {
				_, _ = fmt.Fprint(r.Out(), widgetListing(defined, name, true))
			}
		}
		return status
	}
	for _, name := range listedWidgets(defined, opts.all) {
		_, _ = fmt.Fprint(r.Out(), widgetListing(defined, name, opts.source))
	}
	return 0
}

// widgetExists answers `zle -l name`, and `-a` is what widens the question
// from the widgets somebody defined to every widget this shell has.
func widgetExists(defined map[string]string, name string, all bool) bool {
	if _, ok := defined[name]; ok {
		return true
	}
	if !all {
		return false
	}
	_, editors := bindkeyWidgets[name]
	return editors
}

// listedWidgets is the names a listing walks, sorted by widget name.
func listedWidgets(defined map[string]string, all bool) []string {
	seen := map[string]bool{}
	for name := range defined {
		seen[name] = true
	}
	if all {
		for name := range bindkeyWidgets {
			seen[name] = true
		}
	}
	names := make([]string, 0, len(seen))
	for name := range seen {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// widgetListing is one line of a listing, in whichever of the two spellings
// was asked for.
//
// One of the editor's own actions has neither spelling's second half: there is
// no function behind it and no `zle -N` that would define it, so it is its own
// name under either flag — which is what zsh writes for the ones it built in.
func widgetListing(defined map[string]string, name string, source bool) string {
	fn, user := defined[name]
	switch {
	case !user:
		return name + "\n"
	case source && fn == name:
		return "zle -N " + name + "\n"
	case source:
		return "zle -N " + name + " " + fn + "\n"
	case fn == name:
		return name + "\n"
	}
	return name + " (" + fn + ")\n"
}

// callWidget is `zle widget-name [args]`: running one widget from inside
// another.
//
// Only from inside another, which is measured and is not a restriction this
// shell invented: the line the widget would edit exists only while the editor
// is holding one.
func callWidget(r *interp.Runner, ctx context.Context, name string, args []string) int {
	if !editorRunning(r) {
		r.Diagnosef("widgets can only be called when ZLE is active\n")
		return 1
	}
	fn, defined := widgetFunction(r, name)
	if !defined {
		if _, editors := bindkeyWidgets[name]; editors {
			r.Diagnosef("%s: calling a built-in widget is not implemented yet\n", name)
			return 1
		}
		// Silence, measured: a widget invoking a name nothing answers to is
		// status 1 and not a word, with its own stderr watched to be sure.
		return 1
	}
	if !r.HasFunction(fn) {
		return 1
	}
	// `$WIDGET` is left alone: measured, a widget invoked from inside another
	// still reports the *outer* one's name.
	status := r.ExitStatus()
	ran, err := r.CallFunction(ctx, fn, args...)
	if err != nil || !ran {
		r.SetExitStatus(status)
		return 1
	}
	return 0
}

// RunWidget runs one of this shell's widget functions over the line.
//
// The dialect's answer to driver.Shell.RunWidget: repl hands out the line, this
// publishes it under the names this shell's widget functions read, calls the
// function, and hands back whatever the function left. false is a name that is
// not a widget here, or one whose function never arrived — a key bound to it
// does nothing, which is what pressing it in zsh does.
//
// The parameters are opened and closed around the call rather than living for
// the session, which is what `${(t)BUFFER}` reporting them `local` means from
// the outside: a script that is not running a widget must find them unset.
func RunWidget(r *interp.Runner, ctx context.Context, name string, in repl.Line) (repl.Line, bool) {
	// The name the key was bound to is what the function is called with,
	// which is the one thing a descriptor callback differs in — there the
	// argument is the descriptor. See zlewatch.go.
	return runWidgetFunction(r, ctx, name, in, name)
}

// runWidgetFunction is the round trip itself, with what the function is called
// with left to the caller.
//
// One copy for the two callers rather than one each, because everything either
// of them needs is the same: the widget table, the five parameters, the status
// discipline and the deferred close. The second caller arrived with `zle -F -w`
// and would have been written without the defer.
func runWidgetFunction(
	r *interp.Runner, ctx context.Context, name string, in repl.Line, arg string,
) (repl.Line, bool) {
	fn, defined := widgetFunction(r, name)
	if !defined || !r.HasFunction(fn) {
		return in, false
	}
	setWidgetLine(r, in)
	r.SetVar(zleWidget, name)
	r.SetVar(zleActive, "1")
	openWidgetParameters(r)
	// Deferred rather than called at the end, because a panic in the widget
	// function is caught *outside* this call — repl runs it behind the same
	// guard a typed line runs behind — so a straight-line close would be
	// skipped and the parameters would outlive the call. A script at the next
	// prompt would then find `$BUFFER` set, which is the one thing they must
	// never be, and only after a crash nobody would connect it to.
	defer func() {
		closeWidgetParameters(r)
		unsetWidgetState(r)
	}()
	// The status goes in and does not come out, measured: the function sees
	// what the last command left, and what the function leaves is not what the
	// next command reads.
	status := r.ExitStatus()
	_, err := r.CallFunction(ctx, fn, arg)
	r.SetExitStatus(status)
	if err != nil {
		return in, false
	}
	return widgetLine(r), true
}

// openWidgetParameters gives the widget its five parameters, produced rather
// than stored so that each assignment is live in the arithmetic the others
// answer with — see the file comment for the measurement that requires it.
func openWidgetParameters(r *interp.Runner) {
	r.SetDynamic("BUFFER", func(rr *interp.Runner) string { return widgetBuffer(rr) })
	r.SetDynamicWriter("BUFFER", func(rr *interp.Runner, value string) {
		// The stored cursor is left alone and every *read* of it clamps — see
		// widgetCursor. Clamping here as well was a second copy of the same
		// rule, and mutation testing found it: either one could be deleted
		// with nothing failing, which is two places to fix one behavior and
		// the shape bindings.go warns about. A line that just got shorter
		// still reports the cursor at its end, because the reader is where
		// the length is known.
		rr.SetVar(zleBuffer, value)
	})
	r.SetDynamic("CURSOR", func(rr *interp.Runner) string {
		return strconv.Itoa(widgetCursor(rr))
	})
	r.SetDynamicWriter("CURSOR", func(rr *interp.Runner, value string) {
		// A value that is not a number leaves the cursor where it is: this
		// parameter is an integer in zsh — `integer-local-special` — and what
		// a non-number assigned to one means is interp's question and not
		// this file's.
		if n, err := strconv.Atoi(strings.TrimSpace(value)); err == nil {
			setWidgetCursor(rr, n)
		}
	})
	r.SetDynamic("LBUFFER", func(rr *interp.Runner) string {
		runes := []rune(widgetBuffer(rr))
		return string(runes[:widgetCursor(rr)])
	})
	r.SetDynamicWriter("LBUFFER", func(rr *interp.Runner, value string) {
		// Measured: the text before the cursor is replaced and the cursor
		// follows the length of what was written.
		runes := []rune(widgetBuffer(rr))
		right := string(runes[widgetCursor(rr):])
		rr.SetVar(zleBuffer, value+right)
		rr.SetVar(zleCursor, strconv.Itoa(len([]rune(value))))
	})
	r.SetDynamic("RBUFFER", func(rr *interp.Runner) string {
		runes := []rune(widgetBuffer(rr))
		return string(runes[widgetCursor(rr):])
	})
	r.SetDynamicWriter("RBUFFER", func(rr *interp.Runner, value string) {
		// And here the cursor does not move, which is the half of the pair
		// that had to be measured rather than inferred.
		runes := []rune(widgetBuffer(rr))
		left := string(runes[:widgetCursor(rr)])
		rr.SetVar(zleBuffer, left+value)
	})
	r.SetDynamic("WIDGET", func(rr *interp.Runner) string {
		name, _ := rr.GetVar(zleWidget)
		return name
	})
	// Read-only, measured: assigning to it inside a widget answers
	// `w: read-only variable: WIDGET` and the widget stops there. A writer
	// that quietly stored the new name would be the silent no-op this file
	// exists to avoid, and there is nothing a widget could want from writing
	// to it — the name of the widget that is running is not a widget's to
	// change. Lifted again by UnsetDynamic when the call ends, so a script
	// outside one finds an ordinary variable.
	r.MarkReadonly("WIDGET")
}

// closeWidgetParameters takes them away again, so a script that is not running
// a widget finds them unset.
func closeWidgetParameters(r *interp.Runner) {
	for _, name := range zleParameters {
		r.UnsetDynamic(name)
	}
}

// The line a widget is editing, as this file keeps it.
func widgetBuffer(r *interp.Runner) string {
	buf, _ := r.GetVar(zleBuffer)
	return buf
}

// widgetCursor is the cursor, clamped into the line it points at.
//
// **The clamp is here and nowhere else**, and the reason is that this is the
// only place that can be right: the line can get *shorter* after the cursor was
// stored — `BUFFER=ab` on an eight-character line — so a cursor checked when it
// was written is not a cursor still in range when it is read. Clamping on the
// way in as well was a second copy of one rule, which mutation testing found by
// deleting either half with nothing failing.
//
// Measured: `CURSOR=999` on a four-character line reads back 4 and `CURSOR=-5`
// reads back 0, so out of range is brought back rather than refused.
func widgetCursor(r *interp.Runner) int {
	raw, _ := r.GetVar(zleCursor)
	n, err := strconv.Atoi(raw)
	if err != nil {
		return 0
	}
	return min(max(n, 0), len([]rune(widgetBuffer(r))))
}

// setWidgetCursor stores what it was given and clamps nothing, because
// widgetCursor clamps every read — see there for why that is the one place it
// can be done and not merely the one place it is done.
func setWidgetCursor(r *interp.Runner, n int) {
	r.SetVar(zleCursor, strconv.Itoa(n))
}

func setWidgetLine(r *interp.Runner, in repl.Line) {
	r.SetVar(zleBuffer, in.Buffer)
	setWidgetCursor(r, in.Cursor)
}

func widgetLine(r *interp.Runner) repl.Line {
	return repl.Line{Buffer: widgetBuffer(r), Cursor: widgetCursor(r)}
}

// editorRunning reports whether the editor is holding a line for something to
// edit, which is what tells a widget apart from a script.
//
// Non-empty rather than merely set, and that is a fix mutation testing found
// rather than a style: unsetWidgetState clears these names by storing the
// empty string in them, which GetVar reports as *set*. So after one widget had
// run, a plain script could invoke widgets for the rest of the session —
// status 0 and the function actually ran — where the shell being modeled
// refuses every time. Nothing in the suite noticed, because every other test
// asked a *fresh* runner.
//
// One predicate for the two callers, because the descriptor callbacks in
// zlewatch.go ask the same question, and a second copy of it written from the
// same understanding is how this bug would have come back.
func editorRunning(r *interp.Runner) bool {
	active, _ := r.GetVar(zleActive)
	return active != ""
}

// unsetWidgetState clears what the call left behind, so nothing about one
// keystroke's widget is visible to the next one's.
func unsetWidgetState(r *interp.Runner) {
	for _, name := range []string{zleBuffer, zleCursor, zleWidget, zleActive} {
		r.SetVar(name, "")
	}
}

// The widget table, encoded the way bindkey.go encodes its bindings: a flat
// indexed array of pairs, so nothing in either half needs escaping.
func readWidgets(r *interp.Runner) map[string]string {
	flat, _ := r.GetArray(zleStore)
	out := make(map[string]string, len(flat)/2)
	for i := 0; i+2 <= len(flat); i += 2 {
		out[flat[i]] = flat[i+1]
	}
	return out
}

// widgetFunction is the function behind a widget name, and whether the name is
// a widget at all.
func widgetFunction(r *interp.Runner, name string) (string, bool) {
	fn, ok := readWidgets(r)[name]
	return fn, ok
}

func writeWidget(r *interp.Runner, name, fn string) {
	flat, _ := r.GetArray(zleStore)
	for i := 0; i+2 <= len(flat); i += 2 {
		if flat[i] == name {
			flat[i+1] = fn
			r.SetArray(zleStore, flat)
			return
		}
	}
	r.SetArray(zleStore, append(flat, name, fn))
}

func removeWidget(r *interp.Runner, name string) bool {
	flat, _ := r.GetArray(zleStore)
	for i := 0; i+2 <= len(flat); i += 2 {
		if flat[i] == name {
			r.SetArray(zleStore, append(flat[:i:i], flat[i+2:]...))
			return true
		}
	}
	return false
}
