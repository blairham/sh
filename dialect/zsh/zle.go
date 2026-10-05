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
//   - **`-a` is a third spelling and it is bare names.** Not a widening of
//     the other two: measured against zsh 5.9, `zle -la` writes `foo` for a
//     widget `zle -N foo myfn` defined, where `zle -l` writes `foo (myfn)`
//     and `zle -l -L` writes `zle -N foo myfn`, and `zle -la -L` with no
//     names writes the bare name too — `-a` takes the listing over from `-L`
//     rather than adding to it. Only with *names* does `-L` still write the
//     definition: `zle -la -L foo` is `zle -N foo myfn`.
//
//     **This is #4426 and it is not cosmetic**, because the listing is read
//     back as data. zsh-autosuggestions binds what it reads —
//     `for widget in ${${(f)"$(builtin zle -la)"}:#...}` — so an annotated
//     line became a widget *named* `autosuggest-clear
//     (_zsh_autosuggest_widget_clear)`, one that zsh has no entry for and
//     that the plugin's own ignore list could not match.
//   - **Every built-in is listed under its dotted spelling as well.**
//     Measured, zsh's 386 are 193 names twice over: `.accept-line` and
//     `accept-line`, with no name having only one of the two. The dotted
//     spelling is not a second widget — it is how a wrapper reaches past a
//     rebinding of the bare name, which is what callWidget already reads —
//     so listing it claims nothing this editor does not do, and the rule
//     above about the 364 is untouched: this shell lists *its own* actions
//     under both spellings, not zsh's.
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
//   - **A widget function is called with no arguments at all**, which is what
//     makes `$WIDGET` the answer to "which binding ran me" rather than a
//     convenience. Measured through a pseudo-terminal with a key bound to each
//     of the three kinds — `zle -N nnn nf`, `zle -N same` backed by a function
//     of its own name, and `zle -C ccc complete-word cf` — `$#` is 0 in every
//     one. The descriptor callback is the exception and is measured the same
//     way: `zle -F 8 h` and `zle -F -w 7 h` both hand the function the
//     descriptor as `$1`. See RunWidget and zlewatch.go. #1649.
//   - **At most one operation letter.** `-f -l -A -C -D -F -I -K -M -N -R -T
//     -U` each choose what this builtin does, and two of them together is
//     `incompatible operation selection options` at status 1 — before the
//     operands are counted, so `zle -ND` with nothing after it says that
//     rather than `not enough arguments`. Repetition is not two: `zle -NN w`
//     defines a widget. `-a -c -g -m -r -w -G -L` are modifiers and stay
//     welcome alongside an operation, which is measured rather than assumed:
//     `zle -aC w complete-word f`, `zle -C -w w complete-word f` and
//     `zle -NL w f` are all status 0. This shell took whichever operation its
//     `switch` reached first, in silence, until #1648 — a wrong answer with
//     nothing said, which is the class this file exists to avoid.
//   - **The wordings.** `bad option: -x`, `incompatible operation selection
//     options`, `not enough arguments for -N`, `too many arguments for -N`,
//     "no such widget `name'" — this builtin's
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
// function can read the line; it cannot yet offer a completion.
//
// **The editor's own completion is untouched either way**, and that took two
// rules rather than one. A key left on its default binding never reaches the
// widget table at all — KeyBindings reports only what somebody rebound. A key
// a startup file *did* rebind to such a widget is answered by the widget's
// completer instead of its function, which is what keeps Tab working when
// `compinit` puts `_main_complete` on it (#2770). Both live in bindkey.go;
// completionBinding carries the argument.
//
// ## What refuses by name, and why that is the point
//
// A `zle` that accepted everything would be worse than the `command not
// found` it replaces, because a plugin would then believe its widget existed.
// So the letters this shell has not got are refused with the wording `whence`
// and `bindkey` use for the same case — `-I is not implemented yet` — which a
// script can tell apart from a typo, and the spellings of *invoking* that need
// a seam repl has not got are refused by name too:
//
//   - **`zle -R` with a display string**. Bare, it is a redraw and repl does
//     that; the string is still refused. `zle -M` and `zle reset-prompt` have
//     left this list: the message row under the line is repl's
//     Actions.Message (#5942) and the prompt drawn again is its reset-prompt
//     (#5940), so the string `-R` would show has somewhere to go now and
//     only wants measuring.
//
// The editor's own actions have all left this list, the two that read a key
// last: a completion that may stop to ask about a listing (#3043), and the
// incremental search (#5895). Both read through the editor's buffer while its
// key loop waits on the call, so nothing is re-entered. repl performs them on
// request, which is repl.Actions and callBuiltinWidget below.
//
// `vared`, `zcompile` and `zregexparse` are not here, and the three took
// three different answers rather than one (#1405): `zcompile` is built and is
// in zcompile.go, `vared` is built as far as a script can see it and is in
// vared.go, and `zregexparse` belongs with the completion system, which this
// shell has not got.

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
	// zleLastWidget is what `$LASTWIDGET` reads for the length of a widget
	// call. See lastWidgetName.
	zleLastWidget = ".zsh.zle.lastwidget"
	// zleNumeric is the numeric argument a widget sees, empty where there is
	// none, and zleKeymap the keymap `$KEYMAP` names. See widgetCall.
	zleNumeric = ".zsh.zle.numeric"
	zleKeymap  = ".zsh.zle.keymap"
	// zleHistNo is the history line's number `$HISTNO` reads. See
	// repl.Line.HistNo.
	zleHistNo = ".zsh.zle.histno"
	// zleKeys is the key sequence `$KEYS` holds: the keystroke the call is
	// for, until `read-command` reads another. See readCommand.
	zleKeys = ".zsh.zle.keys"
	// zlePrebuffer is what `$PREBUFFER` reads: the lines of a command already
	// entered at a continuation prompt, handed in by the editor. See
	// repl.Line.Prebuffer.
	zlePrebuffer = ".zsh.zle.prebuffer"
	// zleAccept is set by `zle accept-line` inside a widget and read once, by
	// the call that ran the widget. A parameter under a name no script can
	// spell, the way the rest of this file keeps its state, so a subshell gets
	// its own and nothing leaks past the keystroke.
	zleAccept = ".zsh.zle.accept"
	// zlePostdisplay is the text a widget asked to be drawn after the line
	// without being part of it, which is what an inline suggestion is made of.
	// A carrier for one keystroke rather than a store: the editor hands in
	// what is on the screen and takes back what the widget left, so nothing
	// here has to know when a line ended. See repl.Line.Postdisplay (#4217).
	zlePostdisplay = ".zsh.zle.postdisplay"
	// zleCutBuffer is the kill `$CUTBUFFER` reads and writes for the length
	// of a call: fetched from the editor when the call opens and handed back
	// when it ends, and around every action the widget asks the editor for.
	// See repl.Actions.CutBuffer (#5916).
	zleCutBuffer = ".zsh.zle.cutbuffer"
	// zleTransform is the transformation table `zle -T` writes: a flat array
	// of pairs, the transformation's name and the widget registered for it.
	// Beside the widget table and in the same shape, so a subshell gets its
	// own copy of this too.
	zleTransform = ".zsh.zle.transform"
	// zleOpened is how the running call opened its parameters, empty where
	// none is open. See widgetOpening.
	zleOpened = ".zsh.zle.opened"
)

// postdisplayName is what a widget reads the text drawn after the line under.
//
// A constant beside regionHighlightName rather than a literal at its three
// sites, for the reason that one is: the name is written where the parameter is
// opened, marked local and closed again, and three spellings of one string is
// how one of them comes to be missed.
const postdisplayName = "POSTDISPLAY"

// cutBufferName is the text of the last kill, as a widget reads and writes it.
// Opened, marked local and closed beside POSTDISPLAY and for the same reason:
// it is line state a widget may change, and nothing outside a widget has it.
const cutBufferName = "CUTBUFFER"

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

// zleQueueParameters are what a widget reads to ask how much input is
// waiting behind the keystroke it was called for, and this editor's answer
// to both is the constant 0.
//
// Not in zleParameters, for the reason region_highlight is not: that list is
// the line and the widget's name, which is what the completion branch marks
// read-only as a set and what the suites walk as "the line parameters".
// These two are neither, and they are read-only under every kind of widget
// rather than only a completion one.
//
// `PENDING` is the count of bytes already typed and immediately readable;
// `KEYS_QUEUED_COUNT` is what `zle -U` has pushed back. This shell reads one
// keystroke at a time and pushes nothing back, so 0 is the honest answer to
// both rather than a placeholder — the same answer real zsh gives on a
// system that cannot ask its terminal how much is buffered.
//
// They were absent, which is not the same as 0 and is what #4211 was:
// zsh-autosuggestions opens `_zsh_autosuggest_modify` with
// `(( $PENDING > 0 || $KEYS_QUEUED_COUNT > 0 ))`, and an *unset* name leaves
// the arithmetic with no left operand at all, so every keystroke at a real
// prompt printed `bad math expression: operand expected at `> 0 || 0 > 0 ”.
var zleQueueParameters = []string{"PENDING", "KEYS_QUEUED_COUNT"}

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
	r.Register("zle", func(r *interp.Runner, ctx context.Context, args []string) int {
		bootLineEditor(r)
		return zleBuiltin(r, ctx, args)
	})
}

// The letters this builtin has, and the ones it has and this shell has not.
// Split so a letter zsh does not have is `bad option` and a letter it has that
// is not built yet says so — the distinction whence.go documents.
const (
	zleLetters            = "acfglmrwACDFGIKLMNRTU"
	zleLettersImplemented = "aACDFKLMNRTUlrw"
	// zleOperationLetters are the letters that choose what this builtin
	// *does*. At most one may be given, and two is a refusal rather than a
	// preference — see zleBuiltin.
	//
	// Measured 2026-09-12 against zsh 5.9.2 by pairing every letter in
	// zleLetters with `-N`: `f l A C D F I K M N R T U` each answered
	// `incompatible operation selection options`, and `a c g m r w G L` each
	// went through as a modifier. So the split is measured across the whole
	// alphabet this builtin has rather than read off the six operations this
	// shell happens to implement, and a letter promoted out of
	// zleLettersImplemented later is already on the right side of it.
	zleOperationLetters = "flACDFIKMNRTU"
)

// zleOpts is what the letters asked for.
type zleOpts struct {
	define    bool // -N
	complete  bool // -C
	delete    bool // -D
	alias     bool // -A
	list      bool // -l
	watch     bool // -F
	draw      bool // -R
	message   bool // -M
	push      bool // -U
	keymap    bool // -K
	all       bool // -a
	source    bool // -L
	widget    bool // -w
	forget    bool // -r, which only -T reads
	transform bool // -T
}

func zleBuiltin(r *interp.Runner, ctx context.Context, args []string) int {
	var opts zleOpts
	rest := args
	// Every letter first, then the questions that are about the whole of what
	// was asked for. The order is measured rather than convenient — see the
	// three checks below, each of which zsh answers at a different point.
	var letters []rune
	for len(rest) > 0 && strings.HasPrefix(rest[0], "-") {
		if rest[0] == "--" || rest[0] == "-" {
			// A lone `-` ends the options as `--` does, and is taken off
			// with them: measured 2026-10-04 against zsh 5.9.2 inside a
			// widget, `zle -U - abc` pushes `abc` at status 0 and `zle -
			// .read-command` calls the widget. This shell left the `-` in
			// place, so the first was `too many arguments for -U` — and
			// that is the spelling zsh's own paste functions push a paste
			// back with (#5880).
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
			// A letter this builtin does not have at all is refused where it
			// is read, and it wins over everything below: measured, both
			// `zle -Nx w` and `zle -xN w` are `bad option: -x` rather than a
			// complaint about the pair.
			if !strings.ContainsRune(zleLetters, letter) {
				r.Diagnosef("bad option: -%c\n", letter)
				return 1
			}
			letters = append(letters, letter)
		}
		rest = rest[1:]
	}
	if operationsAsked(letters) > 1 {
		// Two operations is a refusal and not a preference. Measured: `zle
		// -ND w`, `zle -NA a b`, `zle -Dl` and the same pair split over two
		// words are all this wording at status 1, where this shell used to
		// take whichever the switch below reached first and do it in silence
		// — a widget defined by a line that asked for a deletion. #1648.
		//
		// Before the operands are counted, also measured: `zle -ND` with
		// nothing after it says this rather than `not enough arguments`.
		// After the bad-option check above and before the unimplemented one
		// below, which is the whole reason the letters are collected first.
		r.Diagnosef("incompatible operation selection options\n")
		return 1
	}
	for _, letter := range letters {
		if !strings.ContainsRune(zleLettersImplemented, letter) {
			// A letter this shell has not got says so, rather than being
			// accepted and doing nothing — see the file comment.
			r.Diagnosef("-%c is not implemented yet\n", letter)
			return 1
		}
		setZleLetter(&opts, letter)
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
	case opts.draw:
		return redisplay(r, ctx, rest)
	case opts.message:
		return showMessage(r, ctx, rest)
	case opts.push:
		return pushKeys(r, ctx, rest)
	case opts.keymap:
		return selectWidgetKeymap(r, ctx, rest)
	case opts.transform:
		return transformation(r, opts, rest)
	case len(rest) == 0:
		// `zle` with nothing at all is a question and not a word: whether
		// widgets can be called from here. Measured 2026-10-04 against zsh
		// 5.9.2, status 1 under `zsh -c` and 0 from inside a widget, and the
		// same for `zle --` and `zle -`.
		if editorRunning(r) {
			return 0
		}
		return 1
	}
	call, ok := widgetCallOptions(r, rest[1:])
	if !ok {
		return 1
	}
	args, asItself, nolast := call.args, call.asItself, call.nolast
	if call.keymap != "" && !slices.Contains(keymapsNow(r), call.keymap) {
		// A keymap there is no such thing as: status 1 and not a word,
		// measured.
		return 1
	}
	if insideWidget(r) {
		// The numeric argument and the keymap the called widget sees, for the
		// call alone. See widgetCallOptions.
		if call.numeric != nil || call.clearNumeric {
			was, _ := r.GetVar(zleNumeric)
			switch {
			case call.numeric != nil:
				r.SetVar(zleNumeric, strconv.Itoa(*call.numeric))
			case was != "":
				r.SetVar(zleNumeric, "1")
			}
			defer r.SetVar(zleNumeric, was)
		}
		if call.keymap != "" {
			was, _ := r.GetVar(zleKeymap)
			r.SetVar(zleKeymap, call.keymap)
			defer r.SetVar(zleKeymap, was)
		}
	}
	if !nolast && insideWidget(r) {
		// What ran is the last widget from here on, unless the call said
		// otherwise. See lastWidgetName.
		defer r.SetVar(zleLastWidget, rest[0])
	}
	if asItself {
		// `-w`: the called widget sees its own name as `$WIDGET`, for the
		// call alone. See widgetCallOptions.
		was, _ := r.GetVar(zleWidget)
		if insideWidget(r) {
			r.SetVar(zleWidget, rest[0])
			defer r.SetVar(zleWidget, was)
		}
	}
	return callWidget(r, ctx, rest[0], widgetCallArgs(args))
}

// widgetCallOptions reads the `-N` and `-w` letters that may stand in front
// of a called widget's arguments, alone or bundled, and reports whether `-w`
// was among them. Measured 2026-10-02 through a pseudo-terminal against zsh
// 5.9.2, from inside a widget named `outer`, an `inner` printing `$WIDGET`,
// `$#` and its arguments (#5398):
//
//	zle inner -w            W=inner, 0 arguments — and outer's $WIDGET is
//	                        outer again once it returns
//	zle inner -Nw -- a b    W=inner, `a b`
//	zle inner -N x          W=outer, `x`
//	zle inner -wN           W=inner
//	zle inner               W=outer
//
// And the three that carry a value, measured the same way on 2026-10-02
// (#5495), a `g` printing `$NUMERIC`, `${(t)NUMERIC}` and `$KEYMAP`:
//
//	zle g -n 3 a            NUMERIC=3, integer-local-special; unset again after
//	zle g -n x              NUMERIC=0, status 0
//	zle g -n -2             NUMERIC=-2
//	zle g -n                number expected after -n, status 1
//	zle g -N                NUMERIC unset where there was none, 1 where there was
//	zle g -K vicmd          KEYMAP=vicmd; main again after
//	zle g -K nosuch         status 1, nothing written
//	zle g -K                keymap expected after -K, status 1
//	zle g -f nolast a b     a b, and $LASTWIDGET left as it was
//	zle g -f bogus          'nolast' expected after -f
type widgetCall struct {
	args         []string
	asItself     bool
	nolast       bool
	numeric      *int
	clearNumeric bool
	keymap       string
}

// widgetCallOptions reads the options in front of a called widget's
// arguments. False is a refusal already reported.
func widgetCallOptions(r *interp.Runner, args []string) (widgetCall, bool) {
	var c widgetCall
	for len(args) > 0 {
		a := args[0]
		switch a {
		case "-f":
			if len(args) < 2 || args[1] != "nolast" {
				r.Diagnosef("'nolast' expected after -f\n")
				return c, false
			}
			c.nolast = true
			args = args[2:]
			continue
		case "-n":
			if len(args) < 2 {
				r.Diagnosef("number expected after -n\n")
				return c, false
			}
			n := leadingInteger(args[1])
			c.numeric = &n
			args = args[2:]
			continue
		case "-K":
			if len(args) < 2 {
				r.Diagnosef("keymap expected after -K\n")
				return c, false
			}
			c.keymap = args[1]
			args = args[2:]
			continue
		}
		if len(a) < 2 || a[0] != '-' || strings.Trim(a[1:], "Nw") != "" {
			break
		}
		if strings.ContainsRune(a, 'w') {
			c.asItself = true
		}
		if strings.ContainsRune(a, 'N') {
			c.clearNumeric = true
		}
		args = args[1:]
	}
	c.args = args
	return c, true
}

// leadingInteger is the number at the front of s, with an optional sign, and
// 0 where there is none — `-n x` is 0, measured.
func leadingInteger(s string) int {
	i := 0
	if i < len(s) && (s[i] == '-' || s[i] == '+') {
		i++
	}
	for i < len(s) && s[i] >= '0' && s[i] <= '9' {
		i++
	}
	n, _ := strconv.Atoi(s[:i])
	return n
}

// widgetCallArgs is what a called widget is given, with the `--` that ends
// *its* option list taken off.
//
// `zle name -- args` has two option lists in it and they are parsed in two
// places. The first belongs to the builtin and is read above; this is the
// second, which begins after the widget's name — zsh documents the call form
// as `zle widget [-n num] [-N] [-K keymap] [-w] [--] [arg ...]`.
//
// **Exactly one, and only where it is first.** Measured 2026-09-24 against
// zsh 5.9.2 through a pseudo-terminal, a widget printing `$#` and its
// arguments joined:
//
//	zle inner -- "a b"    1   a b
//	zle inner -- -x       1   -x
//	zle inner --          0
//	zle inner -- -- q     2   --|q
//	zle inner a -- b      3   a|--|b
//	zle inner a b         2   a|b
//
// So the second `--` of a pair is an ordinary operand and a `--` after an
// operand is one too: it is the marker ending an option list, not a word with
// a meaning of its own. This shell passed it straight through, so every one
// of the first four rows arrived with an extra leading `--`.
//
// **That is what made the async suggestion wrong rather than absent.**
// zsh-autosuggestions' response handler draws with
// `zle autosuggest-suggest -- "$suggestion"`, and the widget behind it reads
// `$1` — so once the callback was reaching the line at all (#4413), what it
// drew was `--` instead of the suggestion. The plugin's synchronous path
// calls the same function *directly* rather than through `zle`, which is why
// only the asynchronous one showed it.
//
// The other options of that list are **not** read here and this does not
// pretend to: measured, `zle inner -x` is `unknown option: x` at status 1 with
// the widget never run, where this shell passes `-x` through as an operand.
// That is a separate divergence from the one being fixed, and guessing at
// `-n`, `-N`, `-K` and `-w` from the documentation rather than from a
// measurement is how a preset comes to hold an answer no shell gives.
func widgetCallArgs(args []string) []string {
	if len(args) > 0 && args[0] == "--" {
		return args[1:]
	}
	return args
}

// operationsAsked is how many *distinct* operations the letters chose.
//
// Distinct, because a letter repeated is not two operations: measured, `zle
// -NN w` and `zle -N -N w` both define a widget at status 0, where `zle -DD w`
// gets as far as `no such widget` — so it is the set that has to hold one
// member and not the count of letters given.
func operationsAsked(letters []rune) int {
	var seen []rune
	for _, letter := range letters {
		if strings.ContainsRune(zleOperationLetters, letter) && !slices.Contains(seen, letter) {
			seen = append(seen, letter)
		}
	}
	return len(seen)
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
	case 'R':
		opts.draw = true
	case 'M':
		opts.message = true
	case 'U':
		opts.push = true
	case 'K':
		opts.keymap = true
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
	case 'T':
		opts.transform = true
	case 'r':
		// A modifier and not an operation, and one only `-T` reads:
		// measured 2026-09-26, `zle -N -r w f` defines `w` at status 0 with
		// the letter making no difference, and `zle -r` on its own is the
		// bare `zle` — status 1 and not a word.
		opts.forget = true
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
	if protectedName(r, args[0]) {
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
	// After the completer check and not before it: measured, `zle -C
	// .accept-line notacompleter myfn` complains about the *completer*, so
	// the protected name is the later of the two answers.
	if protectedName(r, args[0]) {
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
		// The name being copied *to* is checked here and not above it:
		// measured, `zle -A nosuchw .accept-line` complains that there is no
		// such widget `nosuchw' rather than that `.accept-line' is
		// protected, so the source is resolved first.
		if protectedName(r, args[1]) {
			return 1
		}
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
		// `-a` is its own spelling and takes the listing over from `-L`:
		// bare names, for a defined widget as much as for one of the
		// editor's own. See the header — an annotated line here is a name
		// to the plugin that reads it back.
		if opts.all {
			_, _ = fmt.Fprintln(r.Out(), name)
			continue
		}
		_, _ = fmt.Fprint(r.Out(), widgetListing(defined, name, opts.source))
	}
	return 0
}

// widgetExists answers `zle -l name`, and `-a` is what widens the question
// from the widgets somebody defined to every widget this shell has.
//
// Under `-a` the dotted spelling answers too, because the listing has it:
// measured, `zle -la .accept-line` is status 0 where `zle -l .accept-line` is
// 1. Asked through builtinWidget rather than through bindkeyWidgets directly,
// so the question `-a` answers is the same set callWidget will perform and the
// same set listedWidgets walks.
func widgetExists(defined map[string]widgetDefinition, name string, all bool) bool {
	if _, ok := defined[name]; ok {
		return true
	}
	if !all {
		return false
	}
	if builtinWidget(name) {
		return true
	}
	builtin, dotted := strings.CutPrefix(name, ".")
	return dotted && builtinWidget(builtin)
}

// listedWidgets is the names a listing walks, sorted by widget name.
//
// Under `-a`, each of the editor's own actions twice: bare and dotted, which
// is the shape zsh's 386 are in. See the header for why the dotted spelling
// costs nothing that the bare one has not already claimed.
func listedWidgets(defined map[string]widgetDefinition, all bool) []string {
	seen := map[string]bool{}
	for name := range defined {
		seen[name] = true
	}
	if all {
		for _, name := range builtinWidgetNames() {
			seen[name] = true
			seen["."+name] = true
		}
	}
	names := make([]string, 0, len(seen))
	for name := range seen {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// builtinWidgetNames is every widget the editor performs under its own steam:
// the names builtinWidget answers `true` for, which is what a listing under
// `-a` must walk and what `-a`'s question must answer for.
//
// Read off the same two tables builtinWidget consults, and in the same order,
// rather than written out again — the predicate and the enumeration had drifted
// apart, which is what left `accept-line` callable, bindable and rebindable
// here while `zle -la` did not have it and `zle -la accept-line` said no. A
// plugin that enumerates before it wraps therefore never saw the one widget
// every plugin wraps.
func builtinWidgetNames() []string {
	seen := map[string]bool{}
	for name := range bindkeyWidgets {
		seen[name] = true
	}
	for name := range widgetFunctionActions {
		seen[name] = true
	}
	for _, name := range editorControlKeys {
		if accepts(name) {
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
// builtinWidget reports whether a name is one of the editor's own widgets,
// which is what a leading `.` may stand for and what `-N`, `-C` and `-A` may
// not take as a name of their own.
//
// The same two tables callWidget consults in its undotted path, asked as one
// question so that the dotted path and the protected-name check cannot drift
// apart from it.
func builtinWidget(name string) bool {
	if accepts(name) {
		return true
	}
	if _, editors := bindkeyWidgets[name]; editors {
		return true
	}
	_, only := widgetFunctionActions[name]
	return only
}

// protectedName refuses a dotted name that a built-in widget already answers
// to, and is what keeps the two name sets disjoint so that callWidget can
// resolve a dotted name by asking one table and then the other.
//
// Measured against zsh 5.9 through a pseudo-terminal, each of `zle -N`,
// `zle -C` and `zle -A` given `.accept-line` or `.clear-screen` writes
// “widget name `.accept-line' is protected“ and is status 1, while the same
// letter given `.plainnew` is status 0 and stores an ordinary user widget.
func protectedName(r *interp.Runner, name string) bool {
	rest, isDotted := strings.CutPrefix(name, ".")
	if !isDotted || !builtinWidget(rest) {
		return false
	}
	r.Diagnosef("widget name `%s' is protected\n", name)
	return true
}

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
// Two kinds, and the split is the editor's rather than this shell's.
//
// The **accept** is the one the editor honors *after* the widget returns
// rather than in the middle of it: zsh's `zle accept-line` does not stop the
// function it is called from — the rest of the body still runs — and the line
// is committed when the widget is finished. So it is recorded and carried back
// by runWidgetFunction, and repl ends the line the way a typed Return ends it.
//
// **Everything else goes to the editor and comes straight back**, through the
// handle repl put on this call's context. The line the editor is given is the
// one the widget is holding *now* and not the one the keystroke started with,
// which is what makes a widget that sets `BUFFER` and then asks for the cursor
// to move mean the line it just wrote; and what comes back is written into the
// same four parameters, so the next line of the widget reads it.
//
// Measured 2026-09-12 against zsh 5.9.2 through a pseudo-terminal, with
// `echo one` and `echo two` in history and a key bound to a widget:
//
//	zle up-line-or-history     rc 0   BUFFER=echo two   CURSOR=8
//	again                      rc 0   BUFFER=echo one   CURSOR=8
//	again, at the top          rc 0   BUFFER=echo one   CURSOR=8
//	zle beginning-of-line      rc 0   BUFFER=echo one   CURSOR=0
//	zle forward-word           rc 0   BUFFER=echo one   CURSOR=5
//	zle kill-word              rc 0   BUFFER=echo       CURSOR=5
//	zle down-line-or-history   rc 0   BUFFER=echo two   CURSOR=8
//	again                      rc 0   BUFFER=           CURSOR=0
//	again, at the bottom       rc 0   BUFFER=           CURSOR=0
//
// Three facts came out of that and all three are in the code. The effect is
// **immediate** — the plugin this was filed for reads `$BUFFER` on the very
// next line. The status is **0 even when the action could not move**, so
// running off either end of history is not something a script can see. And the
// cursor lands where the key would have left it, which is why this runs the
// editor's own action rather than a copy of it.
//
// **The status is the action's own**, and it is 0 for all of them but one. The
// incremental search answers how it ended — 1 for a search that ended failing,
// 3 for one abandoned with `C-g` or `C-c` — measured against zsh 5.9.2 and
// written down beside repl's searchEnd.status. It used to be refused here, as
// a mode with its own read loop, and that cost `C-r` to everyone running
// zsh-autosuggestions, whose wrapper calls it by name (#5895). A completion
// was refused for the same reason until #3043, and was measured wrong the same
// way. An editor that does refuse an action is still reported out loud, in the
// same words a letter this shell has not got gets, because a refusal a script
// can see beats a call that appears to work.
//
// The paste is the one action here that reads an argument. With a name after
// it, `zle .bracketed-paste NAME` puts the paste in that parameter instead of
// in the line — see repl.Actions.Paste for the measurement — and that is the
// spelling a paste plugin uses to look at what was pasted before keeping any
// of it. Every other action ignores what follows it, as it did before.
func callBuiltinWidget(r *interp.Runner, ctx context.Context, name string, args []string) int {
	if accepts(name) {
		r.SetVar(zleAccept, "1")
		return 0
	}
	if action, only := widgetFunctionActions[name]; only {
		actions, inside := repl.ActionsFrom(ctx)
		if !inside {
			return 1
		}
		// None of these kills through the editor, and one of them —
		// copy-region-as-kill — sets the kill itself, so it is handed over
		// when the action is done rather than read back. Each is a widget
		// call that is not a kill, so the next kill starts afresh (#5918).
		defer actions.WidgetCalled()
		defer handBackCutBuffer(r, actions)
		return action(r, actions, args)
	}
	widget, editors := bindkeyWidgets[name]
	if !editors {
		return 1
	}
	actions, inside := repl.ActionsFrom(ctx)
	if !inside {
		// No editor on the other end of this call. Reached where a widget
		// function was called by something that is not a session — an
		// embedder with a Runner and a line and no editor of repl's — rather
		// than by a script, which callWidget has already turned away with its
		// own wording. Silence, because there is no editor to have refused.
		return 1
	}
	// The kill a widget assigned goes to the editor before the action, so a
	// yank inserts it, and the action's own kill comes back after it.
	handBackCutBuffer(r, actions)
	defer fetchCutBuffer(r, actions)
	if widget == repl.WidgetBracketedPaste && len(args) > 0 {
		r.SetVar(args[0], actions.Paste())
		return 0
	}
	if widget == repl.WidgetUndo && len(args) > 0 {
		// `zle undo N`: back to change N, the number `UNDO_CHANGE_NO`
		// gave. Read the way a number is read here, so a word that is not
		// one is 0 — measured, `zle .undo abc` takes the line back to how
		// it began at status 1, which is what 0 does. See repl's undoTo.
		regionsAnchor(r, widgetBuffer(r))
		out, reached := actions.UndoTo(leadingInteger(args[0]), widgetLine(r))
		regionsFollow(r, out.Buffer, out.Cursor)
		setWidgetLine(r, out)
		if !reached {
			return 1
		}
		return 0
	}
	// The line as the widget has it is where the offsets stand — an
	// assignment before this call moved none of them — and what the action
	// does to it moves them. See regionsFollow.
	regionsAnchor(r, widgetBuffer(r))
	var out repl.Line
	var performed bool
	if with, takes := actions.(repl.ArgumentActions); takes && widget == repl.WidgetInsertLastWord && len(args) > 0 {
		// The one action that reads arguments of its own: the history
		// offset, the word, and whether the offset counts from the line
		// being edited (#5987). See repl's insertLastWordWith.
		out, performed = with.PerformWith(widget, widgetLine(r), args)
	} else {
		out, performed = actions.Perform(widget, widgetLine(r))
	}
	if !performed {
		r.Diagnosef("%s: calling a built-in widget is not implemented yet\n", name)
		return 1
	}
	if widget == repl.WidgetPushLineOrEdit && holds(r, zlePrebuffer) {
		// At a continuation prompt the read ends here, and so does the
		// widget: measured 2026-10-04 against zsh 5.9.2, nothing after
		// `zle push-line-or-edit` in the widget runs once the command has
		// been pulled back (#5931). At the main prompt it is push-line and
		// the widget goes on. The stop is the same one send-break takes.
		r.StopTheScript(0)
		return 0
	}
	if widget == repl.WidgetSendBreak {
		// The rest of the widget does not run: measured 2026-10-04 against
		// zsh 5.9.2, `w() { zle send-break; print after }` prints nothing,
		// and the line is given up with `$?` 1 (#5913). The stop is taken
		// back where the widget's call ends, which is what hands the line
		// to the editor to give up.
		r.StopTheScript(1)
		return 1
	}
	regionsFollow(r, out.Buffer, out.Cursor)
	setWidgetLine(r, out)
	if widget.IsIncrementalSearch() || widget == repl.WidgetRecursiveEdit {
		// The one action that reads its own keys and leaves `$KEYS` saying
		// which ended it — `^M`, `^G`, or nothing after `C-c` — measured
		// against zsh 5.9.2; see repl's editorActions.Perform. Every other
		// action leaves `$KEYS` alone, which is what keeps a `read-command`
		// before it meaning what it read.
		r.SetVar(zleKeys, out.Keys)
	}
	return out.Status
}

// holds reports whether one of this file's state names holds anything.
func holds(r *interp.Runner, name string) bool {
	v, _ := r.GetVar(name)
	return v != ""
}

// redisplay is `zle -R`: draw the line as it stands, in the middle of a widget.
//
// The screen catches up when the widget returns whether this is called or not
// — repl redraws unconditionally after every widget — so what this is for is
// the widget that wants the line on the screen *before* it does something
// slow. The plugin this was filed for is exactly that: it draws, then waits up
// to a second for a keystroke, and without the draw the person spends that
// second looking at the line as it was.
//
// Measured 2026-09-12 against zsh 5.9.2. Inside a widget `zle -R` is status 0.
// **Outside one it is status 1 and says nothing at all** — which is not what
// naming an action outside a widget does (`widgets can only be called when ZLE
// is active`) and not what `zle -U` outside one does (`can only be called from
// widget function`). Three spellings, three answers, and this is the silent
// one.
//
// A display string is refused by name. `zle -R "text"` puts the text on a
// status line below the prompt until the next redisplay, and there is nothing
// under repl/ that owns a line below the prompt for something other than a
// search or a listing to write on. Every use of it in the plugins this shell
// is driven under is the bare spelling.
func redisplay(r *interp.Runner, ctx context.Context, args []string) int {
	actions, inside := repl.ActionsFrom(ctx)
	if !inside {
		return 1
	}
	if len(args) > 0 {
		r.Diagnosef("-R with a display string is not implemented yet\n")
		return 1
	}
	actions.Redisplay(widgetLine(r))
	return 0
}

// showMessage is `zle -M STRING`: the string drawn on a row under the line,
// where it stays while the line is edited until another `zle -M` replaces it,
// an empty one takes it away, or the line ends (#5942).
//
// Measured 2026-10-04 through a pseudo-terminal against zsh 5.9.2: status 0
// in a widget, and the message is drawn at once under the line and still
// there after the keys that follow. The arity comes first, inside a widget
// or out: `zle -M` alone is `not enough arguments for -M` and `zle -M a b`
// is `too many arguments for -M`, both at 1, and only then does `zle -M hi`
// outside a widget say `can only be called from widget function`. `zle -M --
// -x` shows `-x`.
func showMessage(r *interp.Runner, ctx context.Context, args []string) int {
	if len(args) == 0 {
		r.Diagnosef("not enough arguments for -M\n")
		return 1
	}
	if len(args) > 1 {
		r.Diagnosef("too many arguments for -M\n")
		return 1
	}
	actions, inside := repl.ActionsFrom(ctx)
	if !inside {
		r.Diagnosef("can only be called from widget function\n")
		return 1
	}
	actions.Message(args[0])
	return 0
}

// pushKeys is `zle -U`: put characters where the editor will read them next.
//
// What a widget that read a keystroke of its own does with the one it did not
// want. The plugin this was filed for waits a second for a key and pushes back
// whatever came, so the key means what it would have meant if the widget had
// never run.
//
// Measured 2026-09-12 against zsh 5.9.2: **exactly one operand**, and the
// arity is settled before the question of whether there is a widget at all.
// `zle -U` alone is `not enough arguments for -U` and `zle -U x y z` is `too
// many arguments for -U`, both at status 1 and both *outside* a widget too,
// where the count is still what it complains about. Worth pinning because the
// idiom is written `zle -U -- "$REPLY"`: a shell that took the operands as a
// list would silently push something else where a `$REPLY` split into words.
//
// Two pushes in one widget come back **newest first**, each push's own
// characters in order — `zle -U ab; zle -U cd` leaves `cdab` on the line. That
// is repl's pushKeys, and the measurement is written down there too.
func pushKeys(r *interp.Runner, ctx context.Context, args []string) int {
	if len(args) == 0 {
		r.Diagnosef("not enough arguments for -U\n")
		return 1
	}
	if len(args) > 1 {
		r.Diagnosef("too many arguments for -U\n")
		return 1
	}
	actions, inside := repl.ActionsFrom(ctx)
	if !inside {
		r.Diagnosef("can only be called from widget function\n")
		return 1
	}
	actions.PushKeys(args[0])
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
	//
	// **Only when the dotted name is one of the editor's own**, which is the
	// half this used to be missing. A dot is not a namespace: it is part of
	// the name, and `zle -A` and `zle -N` will take one so long as it is not
	// a built-in's — measured against zsh 5.9, `zle -N .notabuiltin myfn` is
	// status 0 and `${widgets[.notabuiltin]}` reads `user:myfn`, while
	// `.accept-line` is refused as protected. The two sets are disjoint, so
	// resolving a dotted name is the built-in table and then the ordinary
	// one, under the **whole** name.
	//
	// Routing every dotted name to the built-in table unconditionally is
	// #4424: powerlevel10k saves the widget it wraps as
	// `zle -A clear-screen ._p9k_orig_clear-screen` and reaches the original
	// with `zle ._p9k_orig_clear-screen`. `_p9k_orig_clear-screen` is not a
	// built-in, so the call found nothing and returned status 1 without a
	// word — and `_p9k_widget` reads a failed call as *there was nothing to
	// call* and carries on. `^L` wrote nothing at all.
	if builtin, isDotted := strings.CutPrefix(name, "."); isDotted && builtinWidget(builtin) {
		return callBuiltinWidget(r, ctx, builtin, args)
	}
	def, defined := widgetDefinitionOf(r, name)
	if !defined {
		if builtinWidget(name) {
			return callBuiltinWidget(r, ctx, name, args)
		}
		// Silence, measured: a widget invoking a name nothing answers to is
		// status 1 and not a word, with its own stderr watched to be sure.
		return 1
	}
	if !r.HasFunction(def.function) {
		return 1
	}
	if !insideWidget(r) {
		// **`zle some-widget` from a plain `zle -F` handler is the widget's
		// own context and not a nested call**, because there is no outer
		// widget to nest inside. The handler is an ordinary function — it has
		// no `BUFFER`, no `CURSOR` and no `$WIDGET`, measured — so a widget it
		// invokes has to be *given* the line here, or its assignment to
		// `POSTDISPLAY` lands on a plain variable nothing ever reads back.
		//
		// That is the whole of zsh-autosuggestions' async path: the response
		// handler is a plain handler and every suggestion it has to show is
		// drawn by the `zle autosuggest-suggest` it calls. See zlewatch.go,
		// which holds the line for the length of the handler so this call has
		// something to hand over.
		out, ran := runWidgetFunction(r, ctx, name, widgetLine(r), args...)
		if !ran {
			return 1
		}
		// Back into the held line, so the rest of the handler — and the
		// editor after it — see what the widget left. `zle` twice in one
		// handler is ordinary, and the second call reads the first's line.
		// The offsets were moved by the actions it called and by nothing it
		// assigned, so they stand against that line now.
		setWidgetLine(r, out)
		regionsAnchor(r, out.Buffer)
		if out.Accept {
			r.SetVar(zleAccept, "1")
		}
		// And the editor is still running for the rest of the handler:
		// runWidgetFunction's own cleanup cleared the flag that openEditorActive
		// set, and without this a handler's *second* `zle` would be refused.
		r.SetVar(zleActive, "1")
		return out.Status
	}
	// `$WIDGET` is left alone: measured, a widget invoked from inside another
	// still reports the *outer* one's name.
	status := r.ExitStatus()
	ran, err := r.CallFunction(ctx, def.function, args...)
	// A widget of the shell's is a call that is not a kill, whatever it
	// called inside: measured, `zle k; zle backward-kill-word`, where k
	// kills, leaves two kills (#5918).
	if actions, editing := repl.ActionsFrom(ctx); editing {
		actions.WidgetCalled()
	}
	if err != nil || !ran {
		r.SetExitStatus(status)
		return 1
	}
	// The call answers what the function returned, on this path and the
	// handler's above (#5939). Measured against zsh 5.9.2 through a pty:
	// `w2() { return 3 }` makes `zle w2` 3, `w3() { false }` makes it 1, and
	// `true` before it does not leak through. A wrapper built from other
	// widgets tests exactly this — `zle backward-word || …` — and read 0 for
	// a move that did not happen.
	return r.ExitStatus()
}

// insideWidget reports whether this call is happening inside a widget, as
// against inside a plain `zle -F` descriptor handler.
//
// `$WIDGET` is the question, because it is what the two contexts are measured
// to differ in: a widget has the name of the widget that is running, and a
// plain handler has it unset. editorRunning cannot answer this — it is true in
// both, which is the point of it — so the two predicates are not
// interchangeable however close they read.
func insideWidget(r *interp.Runner) bool {
	name, _ := r.GetVar(zleWidget)
	return name != ""
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
	// A widget function is called with **nothing**, which is the one thing a
	// descriptor callback differs in — there the single argument is the
	// descriptor. See zlewatch.go.
	//
	// Measured 2026-09-12 through a pseudo-terminal against zsh 5.9.2, a key
	// bound to each of the three kinds: `zle -N nnn nf`, `zle -N same` backed
	// by a function of its own name, and `zle -C ccc complete-word cf` all
	// report `$#` of 0 inside the function. The widget's own name is `$WIDGET`
	// — which the same probe read back correctly — and that is how a wrapper
	// shared between several bindings actually asks which one ran. This shell
	// passed the name as `$1` until #1649, so a function that did `shift` or
	// tested `$#`, or one shared with a non-widget caller that dispatches on
	// `$1`, behaved differently here.
	return runWidgetFunction(r, ctx, name, in)
}

// runWidgetFunction is the round trip itself, with what the function is called
// with left to the caller.
//
// One copy for the two callers rather than one each, because everything either
// of them needs is the same: the widget table, the five parameters, the status
// discipline and the deferred close. The second caller arrived with `zle -F -w`
// and would have been written without the defer.
//
// The arguments are variadic because the two callers disagree about whether
// there are any, which is measured on both sides: a widget the editor ran gets
// none and a `-w` descriptor callback gets the descriptor. A signature that
// took one string could only express the second, which is how the widget side
// came to be handed its own name (#1649).
func runWidgetFunction(
	r *interp.Runner, ctx context.Context, name string, in repl.Line, args ...string,
) (repl.Line, bool) {
	out, ran, _ := runWidgetCall(r, ctx, name, in, args...)
	return out, ran
}

// runWidgetCall is runWidgetFunction, and whether the function stopped on a
// fatal error — which a completion widget's caller needs, because such a
// widget keeps its line and so the line cannot say so. See RunCompletion.
func runWidgetCall(
	r *interp.Runner, ctx context.Context, name string, in repl.Line, args ...string,
) (out repl.Line, ran, stopped bool) {
	def, defined := widgetDefinitionOf(r, name)
	if !defined || !r.HasFunction(def.function) {
		return in, false, false
	}
	// `region_highlight` lined up with the line it is handed. From the
	// editor, that line is what the editor did, and the offsets follow it;
	// from another widget's `zle`, it is what that widget assigned, and they
	// stay. See regionsFollow.
	//
	// A nested call puts the caller's line back when it ends, the way it
	// puts back the rest of the caller's state: whoever adopts the line the
	// call hands back lines the offsets up with it, and an editor that runs
	// a widget part-way through an action and drops its line must not leave
	// that line behind as the one the offsets were measured against.
	nested := editorRunning(r)
	if nested {
		callerLine, known := r.GetVar(zleRegionLine)
		defer func() {
			if known {
				regionsAnchor(r, callerLine)
			}
		}()
		regionsAnchor(r, in.Buffer)
	} else {
		regionsFollow(r, in.Buffer, in.Cursor)
	}
	// What was there before this call, put back when it ends — see
	// callerWidgetState for why that is not the same as clearing it.
	caller := saveWidgetState(r)
	setWidgetLine(r, in)
	r.SetVar(zleWidget, name)
	r.SetVar(zleActive, "1")
	last := lastWidgetName(in.Last)
	if held, _ := r.GetVar(zleHeldLast); !in.Last.Known && held != "" {
		// Called from a plain handler, which holds the line without a last
		// widget in it — see runPlainHandler.
		last = held
	}
	r.SetVar(zleLastWidget, last)
	// The keymap the key was read in: `main` while inserting, whichever of
	// emacs and viins that is, and `vicmd` in vi's command mode — measured.
	keymap := "main"
	if in.ViCommand {
		keymap = "vicmd"
	}
	r.SetVar(zleKeymap, keymap)
	r.SetVar(zleKeys, in.Keys)
	r.SetVar(zlePrebuffer, in.Prebuffer)
	// And the count typed before the key, which is `$NUMERIC` (#5498).
	numeric := ""
	if in.Numeric != nil {
		numeric = strconv.Itoa(*in.Numeric)
	}
	r.SetVar(zleNumeric, numeric)
	histNo := ""
	if in.HistNo > 0 {
		histNo = strconv.Itoa(in.HistNo)
	}
	r.SetVar(zleHistNo, histNo)
	// A completion widget looks at the line and does not rewrite it, which is
	// the completer's presence and not a second flag — see openWidgetParameters.
	opened := widgetOpening{completion: def.completer != "", scope: r.ScopeDepth() + 1}
	openWidgetParameters(r, opened)
	actions, editing := repl.ActionsFrom(ctx)
	if editing {
		openUndoChangeNumber(r, actions, opened.scope)
		fetchCutBuffer(r, actions)
	}
	openHistNo(r, actions, editing)
	r.SetVar(zleOpened, opened.String())
	// Deferred rather than called at the end, because a panic in the widget
	// function is caught *outside* this call — repl runs it behind the same
	// guard a typed line runs behind — so a straight-line close would be
	// skipped and the parameters would outlive the call. A script at the next
	// prompt would then find `$BUFFER` set, which is the one thing they must
	// never be, and only after a crash nobody would connect it to.
	defer func() {
		if editing {
			handBackCutBuffer(r, actions)
		}
		closeWidgetParameters(r)
		caller.restore(r)
		if caller.open && editing {
			// The caller's own, which closing this call took away with
			// the rest. The same editor is on both ends of a nested call.
			openUndoChangeNumber(r, actions, caller.opened.scope)
		}
		if caller.open {
			openHistNo(r, actions, editing)
		}
	}()
	// The status goes in and does not come out, measured: the function sees
	// what the last command left, and what the function leaves is not what the
	// next command reads.
	status := r.ExitStatus()
	_, err := r.CallFunction(ctx, def.function, args...)
	returned := r.ExitStatus()
	// A fatal error in the function costs the line, the way one in a typed
	// line does (#5959): measured 2026-10-04 against zsh 5.9.2, a widget
	// whose function assigns `KEYS` or expands `${nosuch?gone}` prints the
	// diagnostic, and the line is given up with `$?` 1 and the editor still
	// reading keys. Here the error was left standing, so every command the
	// session ran afterwards — the next widget's whole body included — was
	// skipped, and the terminal was never taken back for the line.
	//
	// Only at the top. A widget called from another is part of the caller's
	// call, so the error ends the caller too, and it is the caller's own
	// return through here that gives the line up.
	//
	// And not for a completion widget, which keeps the line: measured the
	// same way, `cf() { BUFFER=zz }` behind `zle -C` prints `read-only
	// variable: BUFFER` and a bell and the line stays, with the next
	// widget seeing `$?` 0. The error is still taken back, or it would go on
	// skipping everything after it.
	stopped = !nested && r.GiveUpTheLine()
	broke := stopped && def.completer == ""
	r.SetExitStatus(status)
	if broke {
		if returned == 0 {
			returned = 1
		}
		r.SetExitStatus(returned)
		out := widgetLine(r)
		out.Broken, out.Status = true, returned
		return out, true, true
	}
	// What the function left the line as is the editor's from here, and
	// whatever it assigned moved no offset.
	if !nested {
		regionsAnchor(r, widgetBuffer(r))
	}
	if err != nil {
		return in, false, stopped
	}
	out = widgetLine(r)
	// What the function returned, for a `zle w` from a `zle -F` handler,
	// which answers it (#5939). The editor reads nothing here.
	out.Status = returned
	// Read once: the request belongs to this keystroke, and the deferred
	// restore puts back what was there before the call — nothing, at the
	// top — so a widget that accepted cannot leave the next one accepting too.
	if asked, _ := r.GetVar(zleAccept); asked == "1" {
		out.Accept = true
	}
	return out, true, stopped
}

// openWidgetParameters gives the widget its five parameters, produced rather
// than stored so that each assignment is live in the arithmetic the others
// answer with — see the file comment for the measurement that requires it.
//
// `opened.completion` is whether this is a widget `zle -C` defined, and it makes the
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
func openWidgetParameters(r *interp.Runner, opened widgetOpening) {
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
	// And it carries the attribute, which is what makes interp read what is
	// assigned as arithmetic before this writer sees it (#5929). Measured
	// 2026-10-04 through a pseudo-terminal against zsh 5.9.2, on the line
	// `abcdefghij`: `CURSOR=1; CURSOR+=2` is 3, not `12` clamped to 10;
	// `CURSOR=5; CURSOR=CURSOR-1` is 4; `CURSOR+=x` adds nothing. The listing
	// already said `typeset -i10 CURSOR`, which is a declaration and not the
	// attribute. Lifted again in closeWidgetParameters.
	r.MarkInteger("CURSOR")
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
	// `POSTDISPLAY` is the text drawn after the line, and it is opened here
	// beside the line rather than with `region_highlight`, because it *is*
	// line state: a widget reads it, writes it and clears it, and what it
	// holds is text. Measured 2026-09-22 through a pseudo-terminal against
	// zsh 5.9.2 by pressing a key bound to a widget, `${(t)POSTDISPLAY}` is
	// `scalar-local-special` both before the widget assigns to it and after —
	// the same word BUFFER carries, which is what says it is writable line
	// state and not one of the read-only two.
	//
	// Not in zleLineParameters, and that is measured rather than tidy: a
	// **completion** widget gets the four read-only, and there is nothing in
	// what a completion widget is that would stop it offering a suggestion.
	// Putting it in that list would have made it read-only under `zle -C` on
	// the strength of a guess.
	r.SetDynamic(postdisplayName, func(rr *interp.Runner) string {
		text, _ := rr.GetVar(zlePostdisplay)
		return text
	})
	r.SetDynamicWriter(postdisplayName, func(rr *interp.Runner, value string) {
		rr.SetVar(zlePostdisplay, value)
	})
	// `CUTBUFFER`, the last kill, measured `scalar-local-special` the same
	// way and writable the same way: what a widget assigns is what the next
	// yank inserts. See repl.Actions.CutBuffer (#5916).
	r.SetDynamic(cutBufferName, func(rr *interp.Runner) string {
		text, _ := rr.GetVar(zleCutBuffer)
		return text
	})
	r.SetDynamicWriter(cutBufferName, func(rr *interp.Runner, value string) {
		rr.SetVar(zleCutBuffer, value)
	})
	// And `unset` of one of them empties what it stands for, as well as
	// taking the name away for the rest of the call. Measured 2026-10-05
	// through a pseudo-terminal against zsh 5.9.2, a widget on `abcd` with
	// the cursor at 2: `unset LBUFFER` reads `$BUFFER` back as `cd` at 0,
	// `unset RBUFFER` as `ab` at 2, and `unset BUFFER` as empty at 0, with
	// `${+…}` 0 each time and the next widget seeing the same line; `unset
	// POSTDISPLAY` and `unset CUTBUFFER` leave nothing drawn and nothing to
	// yank. `unset CURSOR` changes nothing, so it has no action here (#6038).
	// Outside a widget the action empties only stores no read reaches
	// there, which each call seeds again, so the names stay ordinary.
	for name, empty := range map[string]func(*interp.Runner){
		"BUFFER": func(rr *interp.Runner) { rr.SetVar(zleBuffer, "") },
		"LBUFFER": func(rr *interp.Runner) {
			runes := []rune(widgetBuffer(rr))
			rr.SetVar(zleBuffer, string(runes[widgetCursor(rr):]))
			rr.SetVar(zleCursor, "0")
		},
		"RBUFFER": func(rr *interp.Runner) {
			runes := []rune(widgetBuffer(rr))
			rr.SetVar(zleBuffer, string(runes[:widgetCursor(rr)]))
		},
		postdisplayName: func(rr *interp.Runner) { rr.SetVar(zlePostdisplay, "") },
		cutBufferName:   func(rr *interp.Runner) { rr.SetVar(zleCutBuffer, "") },
	} {
		r.SetUnsetAction(name, empty)
	}
	// `region_highlight` is opened here and is not one of the five: the other
	// parameters are the line, and this one is what the widget wants *done*
	// with it. It is also the only one backed by a store that outlives the
	// call — see regionhighlight.go.
	openRegionHighlight(r)
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
	r.SetDynamic("LASTWIDGET", func(rr *interp.Runner) string {
		name, _ := rr.GetVar(zleLastWidget)
		return name
	})
	r.MarkReadonly("LASTWIDGET")
	r.SetDynamic("KEYMAP", func(rr *interp.Runner) string {
		name, _ := rr.GetVar(zleKeymap)
		return name
	})
	r.MarkReadonly("KEYMAP")
	// The keys the call is for, which `read-command` replaces with the ones
	// it read. `scalar-local-readonly-special` in zsh 5.9.2, measured
	// 2026-10-04, and `^T` reads `$'\024'` there from a widget on that key.
	r.SetDynamic("KEYS", func(rr *interp.Runner) string {
		keys, _ := rr.GetVar(zleKeys)
		return keys
	})
	r.MarkReadonly("KEYS")
	// The lines already entered at a continuation prompt, and empty at the
	// first. Measured 2026-10-04 against zsh 5.9.2:
	// `scalar-local-readonly-special`, set in every widget, and `if true;
	// then⏎` after that line was entered and while `echo x` is being typed
	// (#5931). Read-only: an assignment answers `read-only variable:
	// PREBUFFER` and stops the widget.
	r.SetDynamic("PREBUFFER", func(rr *interp.Runner) string {
		text, _ := rr.GetVar(zlePrebuffer)
		return text
	})
	r.MarkReadonly("PREBUFFER")
	// The numeric argument, which is there only while there is one.
	r.SetDynamic("NUMERIC", func(rr *interp.Runner) string {
		n, _ := rr.GetVar(zleNumeric)
		return n
	})
	r.SetDynamicPresence("NUMERIC", func(rr *interp.Runner) bool {
		n, _ := rr.GetVar(zleNumeric)
		return n != ""
	})
	// And a widget may set it, which is how a count is handed to the widgets
	// it calls (#5941). Measured 2026-10-04 through a pseudo-terminal against
	// zsh 5.9.2, from a key pressed with no count: `NUMERIC=2` reads back 2
	// as `integer-local-special`, a widget called next sees 2, `zle
	// forward-word` moves two words, `NUMERIC+=3` is 5, `NUMERIC=x` is 0, and
	// `unset NUMERIC` leaves the next call with none. An integer, for the
	// same reason CURSOR is one.
	r.SetDynamicWriter("NUMERIC", func(rr *interp.Runner, value string) {
		if n, err := strconv.Atoi(strings.TrimSpace(value)); err == nil {
			rr.SetVar(zleNumeric, strconv.Itoa(n))
		}
	})
	r.MarkInteger("NUMERIC")
	// The number of the history line being edited. Measured 2026-10-05
	// against zsh 5.9.2: 3 at a fresh prompt after two commands, 2 and 1
	// after Up once and twice, and `integer-local-special` (#5987).
	//
	// Assigning it moves the walk, which is the editor's — see openHistNo.
	r.SetDynamic("HISTNO", func(rr *interp.Runner) string {
		n, _ := rr.GetVar(zleHistNo)
		return n
	})
	r.MarkInteger("HISTNO")
	for _, name := range zleQueueParameters {
		r.SetDynamic(name, func(*interp.Runner) string { return "0" })
		// `integer-local-readonly-special` in real zsh, measured 2026-09-22
		// through a pseudo-terminal on zsh 5.9.2 by pressing a key bound to
		// a widget — the only way to ask, and the same run reported
		// `scalar-local-special` for BUFFER and `scalar-local-readonly-special`
		// for WIDGET, which is what says the harness was reading the right
		// shell. The integer attribute is not decoration: `${(t)…}` is how a
		// plugin asks, and `typeset -p` inside a widget is how the careful
		// ones check before trusting a parameter.
		r.MarkInteger(name)
		r.MarkReadonly(name)
		r.MarkLocal(name)
	}
	if opened.completion {
		for _, name := range zleLineParameters {
			r.MarkReadonly(name)
		}
	}
	declareWidgetParameters(r, opened.scope)
	// Every one of them belongs to this call, and zsh says so in the word a
	// plugin reads: `${(t)BUFFER}` inside a `zle -N` widget is
	// `scalar-local-special` there and was `scalar-special` here, with
	// `region_highlight` missing the same word out of `array-local-special`
	// and `WIDGET` out of `scalar-local-readonly-special`.
	//
	// Measured on zsh 5.9.2 by driving it through a pseudo-terminal and
	// pressing a key bound to a widget, 2026-09-13 — a widget cannot be run
	// any other way, and the `local` half is exactly what a `-c` shell has no
	// route to:
	//
	//	${(t)BUFFER}            scalar-local-special
	//	${(t)LBUFFER}           scalar-local-special
	//	${(t)RBUFFER}           scalar-local-special
	//	${(t)WIDGET}            scalar-local-readonly-special
	//	${(t)region_highlight}  array-local-special
	//
	// It is only what the shell *says*: the parameters were already opened
	// for the length of the call and closed on the way out. But `${(t)…}` is
	// how a plugin asks, and a highlighter that tests for `local` before it
	// trusts the parameter decides it is not running under a real line editor
	// (#2493).
	//
	// The list is zleParameters and region_highlight, which is the same pair
	// closeWidgetParameters takes away — the mark is lifted with the
	// parameter, so nothing outside a widget calls itself local.
	for _, name := range zleParameters {
		r.MarkLocal(name)
	}
	r.MarkLocal(regionHighlightName)
	// And the text drawn after the line, which carries the same word: measured
	// 2026-09-22 the same way, `${(t)POSTDISPLAY}` inside a `zle -N` widget is
	// `scalar-local-special` both before the widget assigns to it and after
	// (#4217). Said here rather than added to zleParameters because that list
	// is what the completion branch above marks *read-only* as a set, and a
	// completion widget offering a suggestion is not something the measurement
	// forbids.
	r.MarkLocal(postdisplayName)
	r.MarkLocal(cutBufferName)
	// And the widget before this one, which is the call's own for the same
	// reason: measured, `${(t)LASTWIDGET}` in a widget is
	// `scalar-local-readonly-special`.
	r.MarkLocal("LASTWIDGET")
	r.MarkLocal("KEYMAP")
	r.MarkLocal("KEYS")
	r.MarkLocal("PREBUFFER")
	r.MarkLocal("NUMERIC")
	r.MarkLocal("HISTNO")
}

// closeWidgetParameters takes them away again, so a script that is not running
// a widget finds them unset.
func closeWidgetParameters(r *interp.Runner) {
	for _, name := range zleParameters {
		r.UnsetDynamic(name)
	}
	for _, name := range zleQueueParameters {
		r.UnsetDynamic(name)
	}
	// And the text drawn after the line, which is opened beside the line and
	// closed beside it for the same reason: what a script finds between two
	// keystrokes is nothing at all.
	r.UnsetDynamic(postdisplayName)
	r.UnsetDynamic(cutBufferName)
	r.UnsetDynamic("LASTWIDGET")
	r.UnsetDynamic("KEYMAP")
	r.UnsetDynamic("KEYS")
	r.UnsetDynamic("PREBUFFER")
	closeUndoParameters(r)
	r.UnsetDynamic("NUMERIC")
	r.UnsetDynamic("HISTNO")
	// The attribute goes with the parameter, so a script between two
	// keystrokes that assigns one of these names assigns text, as it does
	// in zsh, where outside a widget they are not specials at all.
	r.UnmarkInteger("CURSOR")
	r.UnmarkInteger("NUMERIC")
	r.UnmarkInteger("HISTNO")
	for _, name := range zleQueueParameters {
		r.UnmarkInteger(name)
	}
	// Not added to zleParameters, because that list is also what the
	// completion branch above marks read-only and what the tests walk as "the
	// line parameters". This one is neither: a completion widget may colour
	// the line it is refused permission to rewrite.
	closeRegionHighlight(r)
	for name := range widgetParameterDeclarations {
		r.UnsetDynamicDeclaration(name)
	}
	for _, name := range zleQueueParameters {
		r.UnsetDynamicDeclaration(name)
	}
}

// widgetParameterDeclarations is what `typeset -p` writes for each of the
// parameters a widget is given, beside the letters the attribute tables
// already carry. Measured 2026-10-02 on zsh 5.9.2 through a
// pseudo-terminal, inside a widget on the line `ab`:
//
//	typeset BUFFER=ab
//	typeset -i10 CURSOR=2
//	typeset LBUFFER=ab
//	typeset RBUFFER=''
//	typeset POSTDISPLAY=pd
//	typeset -r WIDGET=w
//	typeset -a region_highlight=( '0 1 bold' )
//	typeset -i10 -r KEYS_QUEUED_COUNT=0
//
// Without a declaration a produced parameter is no name to `typeset -p`, and
// every one of these answered `no such variable`.
var widgetParameterDeclarations = map[string]interp.ProducedDeclaration{
	"BUFFER":            {},
	"CURSOR":            {Integer: true, Base: 10},
	"LBUFFER":           {},
	"RBUFFER":           {},
	postdisplayName:     {},
	cutBufferName:       {},
	"WIDGET":            {},
	"LASTWIDGET":        {},
	"KEYMAP":            {},
	"KEYS":              {},
	"PREBUFFER":         {},
	"NUMERIC":           {Integer: true, Base: 10},
	"HISTNO":            {Integer: true, Base: 10},
	regionHighlightName: {Array: true, ListsItsElements: true},
}

// declareWidgetParameters states widgetParameterDeclarations, and the queue
// counters' integer letter, for the length of one widget call.
//
// The parameters are the widget function's own, as a local of it would be:
// measured, `typeset -p BUFFER` in the widget writes `typeset BUFFER=ab` and
// the same line in a function the widget calls writes `typeset -g
// BUFFER=ab`. The widget's call is the scope opened next, which is the one
// they are stated for — own, which the caller works out once, because a call
// reopened for its widget after a nested one closed is no longer at that
// depth. See callerWidgetState.
func declareWidgetParameters(r *interp.Runner, own int) {
	for name, d := range widgetParameterDeclarations {
		d.LocalToScope = own
		r.SetDynamicDeclaration(name, d)
	}
	for _, name := range zleQueueParameters {
		r.SetDynamicDeclaration(name, interp.ProducedDeclaration{Integer: true, Base: 10, LocalToScope: own})
	}
}

// fetchCutBuffer is the editor's kill, put where `$CUTBUFFER` reads it.
func fetchCutBuffer(r *interp.Runner, actions repl.Actions) {
	r.SetVar(zleCutBuffer, actions.CutBuffer())
}

// handBackCutBuffer gives the editor the kill `$CUTBUFFER` holds, where the
// widget changed it — assigned it, or `zle copy-region-as-kill STRING`.
func handBackCutBuffer(r *interp.Runner, actions repl.Actions) {
	if cut, _ := r.GetVar(zleCutBuffer); cut != actions.CutBuffer() {
		actions.SetCutBuffer(cut)
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
	// What is already drawn after the line, so a widget that reads
	// `POSTDISPLAY` before writing it sees what is on the screen — which is
	// what a plugin's wrapper does: it saves the suggestion, clears it while a
	// new one is fetched, and puts one of the two back.
	r.SetVar(zlePostdisplay, in.Postdisplay)
	// And the line's number, which an action that walks the history moves:
	// measured 2026-10-05 against zsh 5.9.2, `zle up-history` in a widget
	// after two commands leaves `$HISTNO` 2 (#6050).
	if in.HistNo > 0 {
		r.SetVar(zleHistNo, strconv.Itoa(in.HistNo))
	}
}

// openHistNo gives `$HISTNO` its writer, which moves the history walk to the
// line numbered: measured 2026-10-05 through a pseudo-terminal against zsh
// 5.9.2, after `: one` and `: two` with `ab` typed, `HISTNO=1` in a widget
// reads `$BUFFER` back as `: one` at once, with the cursor at its end, and a
// number no line has — 0, 9, or text, which is 0 — moves nothing at status
// 0 (#6050). Without an editor that can walk, it is read-only, because a
// writer that moved nothing would be a silent no-op.
func openHistNo(r *interp.Runner, actions repl.Actions, editing bool) {
	walker, walks := actions.(repl.HistoryActions)
	if !editing || !walks {
		r.MarkReadonly("HISTNO")
		return
	}
	r.SetDynamicWriter("HISTNO", func(rr *interp.Runner, value string) {
		n, err := strconv.Atoi(strings.TrimSpace(value))
		if err != nil {
			return
		}
		regionsAnchor(rr, widgetBuffer(rr))
		out := walker.WalkTo(n, widgetLine(rr))
		regionsFollow(rr, out.Buffer, out.Cursor)
		setWidgetLine(rr, out)
	})
}

func widgetLine(r *interp.Runner) repl.Line {
	post, _ := r.GetVar(zlePostdisplay)
	keys, _ := r.GetVar(zleKeys)
	return repl.Line{
		Buffer: widgetBuffer(r), Cursor: widgetCursor(r), Postdisplay: post, Keys: keys,
		Numeric: widgetNumeric(r),
	}
}

// widgetNumeric is `$NUMERIC` as the editor takes a count: none where it is
// unset, which is what an action performed from a widget is then given. A
// count typed before the key and one the widget assigned are the same
// thing by the time an action is asked for (#5941).
//
// Read through the parameter rather than the store behind it, so that `unset
// NUMERIC` — which leaves the store and hides the name — is no count too.
func widgetNumeric(r *interp.Runner) *int {
	raw, _ := r.GetVar("NUMERIC")
	n, err := strconv.Atoi(raw)
	if err != nil {
		return nil
	}
	return &n
}

// editorRunning reports whether the editor is holding a line for something to
// edit, which is what tells a widget apart from a script.
//
// Non-empty rather than merely set, and that is a fix mutation testing found
// rather than a style: a call ending clears these names by storing the empty
// string in them, which GetVar reports as *set*. So after one widget had
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

// widgetOpening is how one widget call opened its parameters: whether it is
// a completion widget, which makes the line read-only, and the scope depth
// the parameters are stated local to. Kept so that the call can be opened
// again, exactly as it was, after a widget it ran has closed them — see
// callerWidgetState.
type widgetOpening struct {
	completion bool
	scope      int
}

// String is the opening as zleOpened holds it, which is "" for no call at
// all: a parameter no script can spell, beside the rest of this file's.
func (o widgetOpening) String() string {
	kind := "n"
	if o.completion {
		kind = "c"
	}
	return kind + strconv.Itoa(o.scope)
}

// parseWidgetOpening reads zleOpened back, and false is no call open.
func parseWidgetOpening(s string) (widgetOpening, bool) {
	if len(s) < 2 {
		return widgetOpening{}, false
	}
	scope, err := strconv.Atoi(s[1:])
	if err != nil {
		return widgetOpening{}, false
	}
	return widgetOpening{completion: s[0] == 'c', scope: scope}, true
}

// widgetCallState is every name this file keeps a call's state under — the
// line, the cursor, what is drawn after it, the widget's name and the one
// before it, the keymap, the count, the accept request, whether the editor
// is running, and how the parameters were opened.
//
// **One list, read by both halves**, which is the reason it is a list: the
// save and the restore must agree name for name, and the bug this exists for
// was a state that could be cleared but not put back.
var widgetCallState = []string{
	zleBuffer, zleCursor, zlePostdisplay, zleWidget, zleLastWidget, zleKeymap,
	zleNumeric, zleAccept, zleActive, zleOpened, zleKeys, zleCutBuffer,
	zlePrebuffer,
}

// callerWidgetState is what was there before a widget call, put back when
// the call ends.
//
// **Put back, not cleared**, and the difference is #5864. A call is not
// always the outermost thing: the editor runs `zle-line-pre-redraw` from a
// redraw, a redraw can happen in the middle of another widget, and a call
// that cleared everything on its way out left the widget it ran inside with
// no `$WIDGET`, no `$BUFFER`, no `PENDING` or `KEYS_QUEUED_COUNT` and no
// `region_highlight` — so zsh-autosuggestions' `(( $PENDING > 0 || … ))`
// failed on every key, and the line the widget handed back was empty, which
// is a Return that never runs anything. Measured 2026-10-04 against zsh
// 5.9.2, from inside a `self-insert` replacement on `e`, after `zle
// .self-insert`: `W=[self-insert] LW=[.self-insert] B=[e] P=[0] K=[0]`, with
// the widget's own `POSTDISPLAY` and `region_highlight` still standing and
// every `${(t)…}` unchanged.
//
// At the top, where nothing was running, the saved state is empty and putting
// it back is the clearing this used to do. A plain `zle -F` handler is the
// middle case: the editor is running and the line is held, but no parameters
// are open, and that is what comes back.
type callerWidgetState struct {
	values []string
	opened widgetOpening
	open   bool
}

func saveWidgetState(r *interp.Runner) callerWidgetState {
	values := make([]string, len(widgetCallState))
	for i, name := range widgetCallState {
		values[i], _ = r.GetVar(name)
	}
	raw, _ := r.GetVar(zleOpened)
	opened, open := parseWidgetOpening(raw)
	return callerWidgetState{values: values, opened: opened, open: open}
}

// restore puts the state back, and the caller's parameters with it: the call
// that is ending closed them on its way out, and a widget that is still
// running must find them as it left them — opened the way it opened them, at
// the depth it opened them at, which is not this one.
func (c callerWidgetState) restore(r *interp.Runner) {
	for i, name := range widgetCallState {
		r.SetVar(name, c.values[i])
	}
	if c.open {
		openWidgetParameters(r, c.opened)
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
// widgetDefinitionOf is one name, looked up without building the table.
//
// A scan of the flat store rather than `readWidgets(r)[name]`, which is what
// it used to be. The table is a map built fresh on every call — four hundred
// entries in a session with a plugin manager — and **this is now asked on
// every printable keystroke**, because a printable key has to find out whether
// something has redefined `self-insert` before it inserts anything (#2485).
// Building a map per character is the shape #1742 is about.
//
// The scan allocates nothing and compares a string per stride. Callers that
// genuinely want the whole table — the two listings, the alias — still use
// readWidgets.
func widgetDefinitionOf(r *interp.Runner, name string) (widgetDefinition, bool) {
	flat, _ := r.GetArray(zleStore)
	for i := 0; i+zleStoreStride <= len(flat); i += zleStoreStride {
		if flat[i] == name {
			return widgetDefinition{function: flat[i+1], completer: flat[i+2]}, true
		}
	}
	return widgetDefinition{}, false
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

// `zle -T <transformation> <widget>` registers a function as a named
// transformation the line editor calls at a defined point, and `zle -Tr
// <transformation>` takes the registration away.
//
// The builtin's half is the table; what reads it is the editor. Measured
// 2026-09-26 on zsh 5.9.2 (aarch64-apple-darwin25.4.0) run `-f` with
// `env -u FPATH`, over a script file with `zmodload zsh/zle` and no terminal —
// so this really is registration and not drawing, and it answers 0 with no
// editor running at all.
//
// **There is exactly one transformation name**, and that is measured rather
// than read: asked for every one-, two- and three-letter name — 18,278 of
// them — this shell takes `tc` and answers `-T: no such transformation` to
// every other one.
//
// The rest of the rows, each of which decides a line here:
//
//	zle -T tc f          0
//	zle -T               too few arguments for option -T
//	zle -T tc            the same — the widget is not optional
//	zle -T nosuchtype    the same: the arity is judged before the name
//	zle -T tc f extra    too many arguments for -T        — no `option`
//	zle -Tr tc f         too many arguments for option -T — with it
//	zle -Tr              too few arguments for option -T
//	zle -T '' f          -T: no such transformation ''
//	zle -T tc nosuchfn   0 — the widget is not looked up here
//	zle -Tr nosuchtype   0 — and a removal checks no name at all
//	zle -- tc f          0 — the marker ends the letters as ever
//
// The two `too many` wordings really do differ by the word `option`, which is
// why they are written out separately instead of one sentence being reused.
func transformation(r *interp.Runner, opts zleOpts, args []string) int {
	if opts.forget {
		switch {
		case len(args) == 0:
			r.Diagnosef("too few arguments for option -T\n")
			return 1
		case len(args) > 1:
			r.Diagnosef("too many arguments for option -T\n")
			return 1
		}
		forgetTransformation(r, args[0])
		return 0
	}
	switch {
	case len(args) < 2:
		r.Diagnosef("too few arguments for option -T\n")
		return 1
	case len(args) > 2:
		r.Diagnosef("too many arguments for -T\n")
		return 1
	}
	if args[0] != zleTransformationName {
		r.Diagnosef("-T: no such transformation '%s'\n", args[0])
		return 1
	}
	writeTransformation(r, args[0], args[1])
	return 0
}

// zleTransformationName is the one name this shell's line editor has a
// transformation point for. A constant rather than a set, because the
// alphabet sweep above found one.
const zleTransformationName = "tc"

// writeTransformation records one, replacing whatever the name held — the
// same shape writeWidget uses, and for the same reason.
func writeTransformation(r *interp.Runner, name, widget string) {
	flat, _ := r.GetArray(zleTransform)
	for i := 0; i+2 <= len(flat); i += 2 {
		if flat[i] == name {
			flat[i+1] = widget
			r.SetArray(zleTransform, flat)
			return
		}
	}
	r.SetArray(zleTransform, append(flat, name, widget))
}

// forgetTransformation takes one away, and says nothing about a name that was
// never there — measured, `zle -Tr nosuchtype` is 0 in silence.
//
// The table these two keep is written and not yet read: the point the editor
// would call a transformation from is not built, and the registration is the
// half a script can see. Said plainly here rather than left for a reader to
// infer from a lookup that does not exist.
func forgetTransformation(r *interp.Runner, name string) {
	flat, _ := r.GetArray(zleTransform)
	for i := 0; i+2 <= len(flat); i += 2 {
		if flat[i] == name {
			r.SetArray(zleTransform, append(flat[:i:i], flat[i+2:]...))
			return
		}
	}
}

// TransformTermcap is what the line editor writes in place of one of its
// terminal operations: what the function `zle -T tc` named leaves in REPLY,
// called with the termcap code and, for a counted one, the count. False where
// no transformation is installed. See repl's termcaptransform.go for the
// measurement.
//
// The status the function leaves is put back, because the editor calling it
// is not a command and must not move `$?`.
func TransformTermcap(r *interp.Runner, ctx context.Context, code, arg string) (string, bool) {
	widget := ""
	flat, _ := r.GetArray(zleTransform)
	for i := 0; i+2 <= len(flat); i += 2 {
		if flat[i] == zleTransformationName {
			widget = flat[i+1]
		}
	}
	if widget == "" {
		return "", false
	}
	args := []string{code}
	if arg != "" {
		args = append(args, arg)
	}
	status := r.ExitStatus()
	defer r.SetExitStatus(status)
	if ok, _ := r.CallFunction(ctx, widget, args...); !ok {
		return "", true
	}
	reply, _ := r.GetVar("REPLY")
	return reply, true
}

// lastWidgetName is what `$LASTWIDGET` says for the widget a keystroke ran:
// the widget's own name, this shell's name for one of the editor's actions,
// or `accept-line` for the key that ended the line before.
//
// Measured 2026-10-02 on zsh 5.9.2 through a pseudo-terminal, a widget `h`
// bound to a key and reading the parameter: `self-insert` after a typed
// character, `backward-char` after the left arrow, `h` after itself, and
// `accept-line` as the first key of a new line. Inside the widget, a widget it
// calls with `zle` is the last one from then on — `zle backward-char` makes
// it `backward-char` — unless the call carried `-f nolast`. It is a readonly
// local special, `scalar-local-readonly-special`, and not a name at all
// outside a widget.
func lastWidgetName(last repl.LastWidget) string {
	switch {
	case !last.Known:
		return ""
	case last.Function != "":
		return last.Function
	case last.Accepted:
		return "accept-line"
	default:
		return widgetNames[last.Widget]
	}
}
