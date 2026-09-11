// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh

import (
	"context"
	"fmt"
	"slices"
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
//     many arguments for -N`, "no such widget `name'" — this builtin's
//     `bad option` is bindkey's and zmodload's and not zstyle's `invalid
//     option`, and its usage complaints name the letter that was short, which
//     zstyle's do not. `zle` with no arguments at all is status 1 and no
//     output whatsoever.
//
// ## The completion widget, and what it is here
//
// `zle -C name completer function` is the other kind of widget, and it is the
// one a real startup needs most: zsh's completion system installs every widget
// it owns through this letter, so refusing it cost fifteen lines of a startup
// and left the shell with none of them (#1615). Measured 2026-09-09 the same
// two ways, under `-c` and through a pseudo-terminal.
//
//   - **All three words are required**, which is the first thing that differs
//     from `-N`: one or two is `not enough arguments for -C` and four is `too
//     many`, so the function may not be left to default to the widget's name.
//     The function still need not exist yet, for the reason `-N`'s does not.
//   - **The completer is a closed set and not "any widget".** Measured by
//     asking for every one of the 386 names `zle -la` reports: exactly the
//     eight builtin completion widgets and their `.`-prefixed spellings are
//     accepted, and everything else — `end-of-line`, which is unarguably a
//     widget — is "invalid widget `name'", a different complaint from the
//     `no such widget` that `-D` and `-A` make. `menu-select` belongs to
//     zsh/complist and is refused until that is loaded, which this shell will
//     not do; the loader guards that one line with `zle -la menu-select`, so
//     it never asks. See zleCompleters.
//   - **It has its own pair of listing spellings and abbreviates neither.**
//     `name -C completer function` plainly and `zle -C name completer
//     function` under `-L`, all three words even when the function is named
//     identically to the widget — the case `-N` writes as a bare name. The
//     completer in the middle is not recoverable from a default.
//   - **It is a widget everywhere else.** `zle -l name` answers for it, `-la`
//     has it, `-D` removes it, and `-A` copies it *completer and all* rather
//     than quietly returning an ordinary widget. Redefining across the two
//     kinds replaces the whole definition and never merges it. That is why
//     there is one table with a wider row rather than a second table: every
//     one of those operations would otherwise need to remember to ask both.
//   - **The line is read-only for the length of the call.** This is the only
//     part visible from inside a widget rather than in a listing, and it is
//     measured rather than inferred: through a pseudo-terminal with a key
//     bound to one, `${(t)BUFFER}` is `scalar-local-readonly-special` where an
//     ordinary widget reports `scalar-local-special`, and each of `BUFFER=`,
//     `CURSOR=`, `LBUFFER=` and `RBUFFER=` answers `read-only variable:` and
//     stops the function. A completion widget looks at the line and offers
//     candidates; it does not rewrite it.
//
// **What is not here is the completion context**, and it is a long way beyond
// this letter. zsh gives such a widget `compstate`, `words`, `CURRENT`,
// `PREFIX` and the `compadd` builtin — the whole of how candidates are
// produced, filtered and displayed — and this shell has no completion system
// for any of it to describe; `zsh/complete` and `zsh/computil` are both in
// zmodload.go's roster of modules it declines. So a completion widget defined
// here registers, lists, aliases, deletes and runs its function, and the
// function can read the line; it cannot yet offer a completion. The editor's
// own completion is untouched, because a key left on its default binding never
// reaches the widget table at all — see bindkey.go's KeyBindings, which
// reports only what somebody rebound.
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

// zleStore is the widget table: a flat array of triples, widget name, then
// the function behind it, then the builtin completion widget a `-C`
// definition named — empty for the `-N` ones, which have no completer.
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
	// zleAccept is set by `zle accept-line` inside a widget and read once, by
	// the call that ran the widget. A parameter under a name no script can
	// spell, the way the rest of this file keeps its state, so a subshell gets
	// its own and nothing leaks past the keystroke.
	zleAccept = ".zsh.zle.accept"
)

// zleLineParameters are the four a widget reads and writes the line through.
//
// Separate from the fifth because a *completion* widget gets these four
// read-only and an ordinary one gets them writable — the one thing about
// `zle -C` that is visible from inside the call rather than only in a
// listing. See the file comment.
var zleLineParameters = []string{"BUFFER", "CURSOR", "LBUFFER", "RBUFFER"}

// zleParameters is those four and the name of the widget that is running:
// what a call opens and closes. They exist only while one is running — see
// the file comment.
var zleParameters = append(slices.Clone(zleLineParameters), "WIDGET")

// zleCompleters is what may be named as the second word of `zle -C`: the
// builtin completion widgets, each under its own name and under the `.`
// spelling that reaches the builtin even when something has redefined the
// plain one — which is the spelling this shell's own completion loader uses.
//
// Measured by asking zsh 5.9.2 for `zle -C w $each f` over the whole of
// `zle -la`, all 386 of them: exactly these eight and their dotted forms are
// accepted and every other widget answers `invalid widget`. So the argument
// is a closed set and not "any widget", and getting the set right is what
// makes the loader's eight-name rebinding loop run to the end.
//
// `menu-select` is deliberately absent, and that is measured too rather than
// an omission: it is a widget only once `zsh/complist` is loaded, which this
// shell will not do, and zsh without that module refuses it here exactly as
// this does. The loader guards that one line with `zle -la menu-select`, so
// it never asks.
var zleCompleters = completerNames()

func completerNames() map[string]bool {
	out := map[string]bool{}
	for _, name := range []string{
		"complete-word", "delete-char-or-list", "expand-or-complete",
		"expand-or-complete-prefix", "list-choices", "menu-complete",
		"menu-expand-or-complete", "reverse-menu-complete",
	} {
		out[name] = true
		out["."+name] = true
	}
	return out
}

// widgetDefinition is what a widget name resolves to.
//
// One record for the two kinds rather than a second table for the completion
// ones: everything that walks the widgets — the two listings, the alias, the
// removal, the round trip that runs one — has to see both kinds or it has a
// hole, and a parallel store is how the second half of a pair gets forgotten.
type widgetDefinition struct {
	// function is the shell function the widget runs. It need not exist yet.
	function string
	// completer is the builtin completion widget named by `zle -C`, and
	// empty for a widget defined by `zle -N`. Non-empty is what *makes* this
	// a completion widget: it changes both listings and it makes the line
	// read-only for the length of the call.
	completer string
}

// registerZle installs the builtin.
func registerZle(r *interp.Runner) {
	r.Register("zle", zleBuiltin)
}

// The letters this builtin has, and the ones it has and this shell has not.
// Split so a letter zsh does not have is `bad option` and a letter it has that
// is not built yet says so — the distinction whence.go documents.
const (
	zleLetters            = "acfglmrwACDFGIKLMNRTU"
	zleLettersImplemented = "aACDFLNlw"
)

// zleOpts is what the letters asked for.
type zleOpts struct {
	define   bool // -N
	complete bool // -C
	delete   bool // -D
	alias    bool // -A
	list     bool // -l
	watch    bool // -F
	all      bool // -a
	source   bool // -L
	widget   bool // -w
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
	case opts.complete:
		return defineCompletionWidget(r, rest)
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
	case 'C':
		opts.complete = true
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
	writeWidget(r, args[0], widgetDefinition{function: fn})
	return 0
}

// defineCompletionWidget is `zle -C name completer function`.
//
// All three words are required — measured, one or two of them is `not enough
// arguments for -C` and four is `too many`, so unlike `-N` the function may
// not be left to default to the name. The completer must be one of the
// builtin completion widgets and nothing else; see zleCompleters.
//
// What this registers is a widget: it is in both listings in the `-C`
// spelling, `zle -l name` answers for it, `-D` removes it and `-A` copies it
// completer and all, and a key bound to it runs the function. What it does
// not carry is the completion *context* the completer names — `compstate`,
// `compadd` and the rest of the parameters a completion widget reads the
// candidate list through — because this shell has no completion system for
// them to describe. So the function runs and can look at the line; it cannot
// yet offer a completion. The one part of that context this does model is the
// part that costs nothing to get right and is wrong if guessed: the line is
// read-only for the length of the call. See openWidgetParameters.
//
// The order the letters are checked in is why this sits beside defineWidget
// rather than inside it: they are two operations that happen to write to one
// table, and folding them would make the argument counts conditional on a
// letter, which is the shape the `-N` rules are stated in.
func defineCompletionWidget(r *interp.Runner, args []string) int {
	switch {
	case len(args) < 3:
		r.Diagnosef("not enough arguments for -C\n")
		return 1
	case len(args) > 3:
		r.Diagnosef("too many arguments for -C\n")
		return 1
	}
	if !zleCompleters[args[1]] {
		// A different wording from `-D`'s and `-A`'s `no such widget`, and
		// measured: the complaint is that the name is not a *completion*
		// widget, which `end-of-line` is not even though it is a widget.
		r.Diagnosef("invalid widget `%s'\n", args[1])
		return 1
	}
	// The function does not have to exist yet, the same as `-N` and for the
	// same reason: measured, `zle -C w complete-word nosuchfn` is status 0
	// and the definition is stored. The completion loader defines every one
	// of its widgets against `_main_complete` before autoloading it.
	writeWidget(r, args[0], widgetDefinition{function: args[2], completer: args[1]})
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
	if def, defined := widgetDefinitionOf(r, args[0]); defined {
		// The whole definition and not only the function: measured, `zle -A`
		// of a completion widget gives a copy that is itself a completion
		// widget, `y -C complete-word f`, rather than a plain one.
		writeWidget(r, args[1], def)
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
func widgetExists(defined map[string]widgetDefinition, name string, all bool) bool {
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
func listedWidgets(defined map[string]widgetDefinition, all bool) []string {
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
//
// A completion widget is its own pair of spellings and it abbreviates
// neither: measured, `zle -C w complete-word w` — the function named
// identically to the widget, which is the case `-N` writes as a bare `w` —
// still reads back as `w -C complete-word w` and `zle -C w complete-word w`.
// All three words, always, because the completer in the middle is not
// recoverable from a default the way the function is.
func widgetListing(defined map[string]widgetDefinition, name string, source bool) string {
	def, user := defined[name]
	switch {
	case !user:
		return name + "\n"
	case def.completer != "" && source:
		return "zle -C " + name + " " + def.completer + " " + def.function + "\n"
	case def.completer != "":
		return name + " -C " + def.completer + " " + def.function + "\n"
	case source && def.function == name:
		return "zle -N " + name + "\n"
	case source:
		return "zle -N " + name + " " + def.function + "\n"
	case def.function == name:
		return name + "\n"
	}
	return name + " (" + def.function + ")\n"
}

// callWidget is `zle widget-name [args]`: running one widget from inside
// another.
//
// Only from inside another, which is measured and is not a restriction this
// shell invented: the line the widget would edit exists only while the editor
// is holding one.
// accepts reports whether a widget name is the editor's "commit this line".
//
// Read off editorControlKeys rather than written out again, so the two cannot
// disagree about what Return is called: bindkey.go is where this shell names
// the keys the editor reads, and `accept-line` is one of them.
func accepts(name string) bool {
	for _, widget := range editorControlKeys {
		if widget == name && widget == "accept-line" {
			return true
		}
	}
	return false
}

// callBuiltinWidget performs one of the editor's own actions, asked for from
// inside a widget.
//
// Only the accept, which is the one the editor can honor *after* the widget
// returns rather than in the middle of it: zsh's `zle accept-line` does not
// stop the function it is called from — the rest of the body still runs — and
// the line is committed when the widget is finished. So it is recorded and
// carried back by runWidgetFunction, and repl ends the line the way a typed
// Return ends it.
//
// The others are still refused out loud. `zle end-of-line` from inside a
// widget really does mean re-entering the read loop mid-keystroke, which this
// shell cannot do and should not pretend to — see the file comment.
func callBuiltinWidget(r *interp.Runner, name string) int {
	if !accepts(name) {
		if _, editors := bindkeyWidgets[name]; editors {
			r.Diagnosef("%s: calling a built-in widget is not implemented yet\n", name)
			return 1
		}
		return 1
	}
	r.SetVar(zleAccept, "1")
	return 0
}

func callWidget(r *interp.Runner, ctx context.Context, name string, args []string) int {
	if !editorRunning(r) {
		r.Diagnosef("widgets can only be called when ZLE is active\n")
		return 1
	}
	// A leading `.` names the *built-in* widget explicitly, past whatever a
	// plugin has rebound the bare name to. That spelling is how a wrapper
	// reaches the thing it wrapped — zsh-autosuggestions writes
	// `_zsh_autosuggest_orig_accept-line() { zle .accept-line }` — so it is
	// read here rather than treated as a name nothing answers to.
	if builtin, isDotted := strings.CutPrefix(name, "."); isDotted {
		return callBuiltinWidget(r, builtin)
	}
	def, defined := widgetDefinitionOf(r, name)
	if !defined {
		if accepts(name) {
			return callBuiltinWidget(r, name)
		}
		if _, editors := bindkeyWidgets[name]; editors {
			r.Diagnosef("%s: calling a built-in widget is not implemented yet\n", name)
			return 1
		}
		// Silence, measured: a widget invoking a name nothing answers to is
		// status 1 and not a word, with its own stderr watched to be sure.
		return 1
	}
	if !r.HasFunction(def.function) {
		return 1
	}
	// `$WIDGET` is left alone: measured, a widget invoked from inside another
	// still reports the *outer* one's name.
	status := r.ExitStatus()
	ran, err := r.CallFunction(ctx, def.function, args...)
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
	def, defined := widgetDefinitionOf(r, name)
	if !defined || !r.HasFunction(def.function) {
		return in, false
	}
	setWidgetLine(r, in)
	r.SetVar(zleWidget, name)
	r.SetVar(zleActive, "1")
	// A completion widget looks at the line and does not rewrite it, which is
	// the completer's presence and not a second flag — see openWidgetParameters.
	openWidgetParameters(r, def.completer != "")
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
	_, err := r.CallFunction(ctx, def.function, arg)
	r.SetExitStatus(status)
	if err != nil {
		return in, false
	}
	out := widgetLine(r)
	// Read once: the request belongs to this keystroke, and unsetWidgetState
	// clears it on the way out with the rest of the call's state, so a widget
	// that accepted cannot leave the next one accepting too.
	if asked, _ := r.GetVar(zleAccept); asked == "1" {
		out.Accept = true
	}
	return out, true
}

// openWidgetParameters gives the widget its five parameters, produced rather
// than stored so that each assignment is live in the arithmetic the others
// answer with — see the file comment for the measurement that requires it.
//
// `completion` is whether this is a widget `zle -C` defined, and it makes the
// four line parameters read-only for the length of the call. That is measured
// and it is not an inference from what completion is for: driving zsh 5.9.2
// through a pseudo-terminal and pressing a key bound to a `zle -C` widget,
// `${(t)BUFFER}` inside the function is `scalar-local-readonly-special` where
// the same probe in a `zle -N` widget reports `scalar-local-special`, and all
// four of `BUFFER=`, `CURSOR=`, `LBUFFER=` and `RBUFFER=` answer `read-only
// variable:` and stop the function. It is also the whole of what this shell
// can honestly model about a completion widget's context, and it is worth
// modeling precisely because it is the half that *refuses* — a completion
// widget written against a shell that let it rewrite the line is a widget
// that does not work in zsh.
func openWidgetParameters(r *interp.Runner, completion bool) {
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
	if completion {
		for _, name := range zleLineParameters {
			r.MarkReadonly(name)
		}
	}
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
	for _, name := range []string{zleBuffer, zleCursor, zleWidget, zleActive, zleAccept} {
		r.SetVar(name, "")
	}
}

// The widget table, encoded the way bindkey.go encodes its bindings: a flat
// indexed array of pairs, so nothing in either half needs escaping.
func readWidgets(r *interp.Runner) map[string]widgetDefinition {
	flat, _ := r.GetArray(zleStore)
	out := make(map[string]widgetDefinition, len(flat)/zleStoreStride)
	for i := 0; i+zleStoreStride <= len(flat); i += zleStoreStride {
		out[flat[i]] = widgetDefinition{function: flat[i+1], completer: flat[i+2]}
	}
	return out
}

// zleStoreStride is how many array elements one widget occupies: the name, the
// function, and the completer that is empty unless `-C` named one.
const zleStoreStride = 3

// widgetDefinitionOf is what a widget name resolves to, and whether the name
// is a widget at all.
func widgetDefinitionOf(r *interp.Runner, name string) (widgetDefinition, bool) {
	def, ok := readWidgets(r)[name]
	return def, ok
}

// writeWidget stores a definition, replacing any earlier one for the same
// name — measured, `-N` over a `-C` and `-C` over an `-N` both leave one
// widget of the later kind rather than two entries or a hybrid, which is why
// the whole record is replaced and never merged field by field.
func writeWidget(r *interp.Runner, name string, def widgetDefinition) {
	flat, _ := r.GetArray(zleStore)
	for i := 0; i+zleStoreStride <= len(flat); i += zleStoreStride {
		if flat[i] == name {
			flat[i+1], flat[i+2] = def.function, def.completer
			r.SetArray(zleStore, flat)
			return
		}
	}
	r.SetArray(zleStore, append(flat, name, def.function, def.completer))
}

func removeWidget(r *interp.Runner, name string) bool {
	flat, _ := r.GetArray(zleStore)
	for i := 0; i+zleStoreStride <= len(flat); i += zleStoreStride {
		if flat[i] == name {
			r.SetArray(zleStore, append(flat[:i:i], flat[i+zleStoreStride:]...))
			return true
		}
	}
	return false
}
