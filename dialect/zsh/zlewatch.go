// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh

import (
	"context"
	"fmt"
	"strconv"

	"github.com/blairham/sh/interp"
	"github.com/blairham/sh/repl"
)

// `zle -F`: a callback on a descriptor, which is how a plugin in this shell
// does asynchrony.
//
// Measured 2026-09-07 against zsh 5.9.2 in the oracle environment — under `-c`
// for everything a script can see, and through a pseudo-terminal in a real
// session, with the descriptor poked from *outside* the shell, for everything
// only a handler that actually ran can see.
//
// zle.go named this as the reason the issue behind it was not cosmetic and
// left it refused. What it needed was for the editor's read loop to wait on
// more than the terminal; repl has that now — repl.Shell.WatchedDescriptors
// and repl.Shell.DescriptorReady, one seam in two halves because the list is
// asked before every wait and the answer only when something woke. As with
// the widget table, **the capability is repl's and only the naming is here**:
// nothing under `repl/` knows the word `zle`, that a callback is spelled with
// `-F`, that omitting the handler removes it, or that `-w` means the callback
// wants the line.
//
// ## What was measured, and what each fact settled
//
//   - **Arming one is silent and almost unconditional.** `zle -F 0 handler` is
//     status 0 and not a word — with the handler undefined, with a descriptor
//     that was never open, with a descriptor number of 99999, and from a
//     script as much as from a session. It is *not* one of the spellings that
//     needs ZLE active, which is the fact that makes it callable from a
//     `precmd`-time scheduler at all, and so from the one this had to work
//     for.
//   - **Three arities, and the middle one is a removal.** No operand lists;
//     one operand removes; two arm. `zle -F 0` on a descriptor nothing is
//     watching is `No handler installed for fd 0` at 1, which is the *only*
//     thing in this operation that complains about state rather than about
//     the words used.
//   - **`zle -F -<fd>` is not a removal spelling**, though it reads like one.
//     `-3` is `Bad file descriptor number for -F: -3`, and so are `-1`, `-2`,
//     `-9`, `-10` and `-- -1`. `-0` is accepted only because it *is* zero.
//     Worth writing down because getting it wrong the other way would make
//     `zle -F -$fd` in a plugin appear to work.
//   - **The descriptor is a number and the whole word must be one.** `007`
//     arms 7 and lists back as 7; `3x` and `" 3 "` are both
//     `Bad file descriptor number for -F:` with the word repeated as it was
//     given. Which is a signed parse followed by a refusal of the negative
//     ones, and `-0` falling through is what proves it.
//   - **The listing is the command that would arm it back, and it is in
//     arming order.** `zle -F 5 h; zle -F 2 g; zle -F 9 h; zle -F` writes
//     those three lines in that order — *not* sorted, which is the opposite
//     of `zle -l` and had to be measured rather than assumed. Re-arming a
//     descriptor replaces it where it stands and keeps its place. There is
//     one spelling: no bare form and no `-L` to ask for this one.
//   - **The handler is called with one argument, the descriptor**, and `$#`
//     is 1. Nothing else — no second argument saying why, under any condition
//     this could produce.
//   - **`-w` is a different thing, and it needs a widget.** Plainly, the
//     handler is an ordinary function call: `BUFFER`, `CURSOR` and `$WIDGET`
//     are all *unset* inside it, and nothing is redrawn afterwards — a line
//     half-typed is left sitting behind whatever the handler printed. With
//     `-w` and a handler that `zle -N` defined, the same call arrives with
//     the full five parameters, `$WIDGET` naming the handler, and an
//     assignment to `BUFFER` lands on the line and is drawn. With `-w` and a
//     handler that is *not* a widget, it never fires at all, in silence.
//   - **`$?` goes in and does not come out.** `(exit 4)`, then the poke, and
//     the handler saw 4; the handler returning 7 left the next command
//     reading 4. The same discipline widgets, hooks and `sched` run under.
//   - **ZLE is active inside a handler even though the line is not there.**
//     `zle some-widget` from inside one is status 0, so this is not the
//     script case — which is why the plain path marks the editor active
//     without publishing the parameters.
//   - **A descriptor at the end of its input is readable for ever, and this
//     shell spins on one.** Watching a pipe whose writer had exited — and
//     `/dev/null`, which answers the same — zsh calls the handler and never
//     removes it: removing itself is the handler's job, and the plugins that
//     use this do exactly that. Re-measured 2026-09-08 with a handler that
//     only counts, that is **250,000 to 270,000 calls a second** and a whole
//     core held for as long as the descriptor stays armed. "Hundreds of times
//     in two seconds" was written here first and is wrong by three orders of
//     magnitude: it was taken with a handler that printed into a
//     pseudo-terminal nobody was reading, where the shell blocks on the write
//     and spends 0.00s of CPU in two seconds. Nothing here tries to be
//     cleverer, but repl lets a keystroke past such a descriptor so a prompt
//     stays usable — see watchfd.go, which carries what that costs *us*
//     rather than this shell.
//
//     Re-measured side by side 2026-09-10, because #1759 reported the spin as
//     a divergence — "zsh prints the handler once per readable turn" — and it
//     is not one. The same startup file (`exec {wfd}< /etc/hosts`, a handler
//     that prints, `zle -F $wfd handler`) driven through the same
//     pseudo-terminal for the same window: **zsh 5.9.2 fired it 458,550 times
//     and this shell 638,001**, and *both* answered every line typed while
//     doing it. Same order, same usable prompt. What that issue was really
//     seeing is in procsubst.go — a session that ended before it could type,
//     which looked like the watcher because the watcher was what armed the
//     descriptor.
//
// What is *not* here is a way to make one fire outside the read loop, and that
// is measured too rather than left as a limit of this shell: with a six-second
// command running and the descriptor becoming readable three seconds in, real
// zsh ran the handler after the command finished. So "only while waiting for a
// key" is the behavior and not the shortfall.

// zleWatch is the table of armed descriptors: a flat indexed array of triples,
// descriptor then handler then whether `-w` was asked for.
//
// A flat array rather than a map, because the order is load-bearing — measured,
// the listing is in arming order — and in the Runner's own tables under a name
// no script can spell, the way zle.go keeps the widgets, which is also what
// gives a subshell its own copy.
const zleWatch = ".zsh.zle.watch"

// zleWatchWidget is what the third of each triple holds when `-w` was asked
// for. Any other value, empty included, is the plain spelling.
const zleWatchWidget = "w"

// watcher is one armed descriptor, as this file holds it.
type watcher struct {
	// fd is the descriptor in the spelling the listing uses, which is the
	// number and not the word that was typed: `007` lists back as `7`.
	fd string

	// handler is the name given for it, which need not be a function that
	// exists — measured, arming one that is nowhere is silent and stored.
	handler string

	// widget says `-w` was asked for: the handler wants the line, and only a
	// widget can have it.
	widget bool
}

// watchDescriptor is the whole of `zle -F`, and which of the three things it
// does is decided by how many operands there were.
func watchDescriptor(r *interp.Runner, opts zleOpts, args []string) int {
	switch len(args) {
	case 0:
		for _, w := range readWatchers(r) {
			_, _ = fmt.Fprint(r.Out(), w.listing())
		}
		return 0
	case 1:
		fd, ok := watchedNumber(r, args[0])
		if !ok {
			return 1
		}
		if !removeWatcher(r, fd) {
			// The one complaint in this operation that is about state rather
			// than about the words, measured with zsh's own capitalisation.
			r.Diagnosef("No handler installed for fd %s\n", fd)
			return 1
		}
		return 0
	case 2:
		fd, ok := watchedNumber(r, args[0])
		if !ok {
			return 1
		}
		writeWatcher(r, watcher{fd: fd, handler: args[1], widget: opts.widget})
		return 0
	}
	r.Diagnosef("too many arguments for -F\n")
	return 1
}

// listing is the one spelling a watcher is written back in: the command that
// would arm it again. Measured — there is no bare form of this listing.
func (w watcher) listing() string {
	if w.widget {
		return "zle -F -w " + w.fd + " " + w.handler + "\n"
	}
	return "zle -F " + w.fd + " " + w.handler + "\n"
}

// watchedNumber is the descriptor a word names, in the spelling the table and
// the listing use.
//
// A signed parse of the whole word followed by a refusal of the negative ones,
// which is what the measurements require between them: `007` is 7, `-0` is 0
// and accepted, `-3` and `3x` and `" 3 "` are all refused, and the refusal
// repeats the word exactly as it was given rather than as it was understood.
// A number far beyond any descriptor this process has is *accepted* — zsh
// stores it and it never fires.
func watchedNumber(r *interp.Runner, word string) (string, bool) {
	fd, err := strconv.Atoi(word)
	if err != nil || fd < 0 {
		r.Diagnosef("Bad file descriptor number for -F: %s\n", word)
		return "", false
	}
	return strconv.Itoa(fd), true
}

// The table, encoded the way zle.go encodes the widgets, with one more field
// per entry so that nothing in any of the three needs escaping.
func readWatchers(r *interp.Runner) []watcher {
	flat, _ := r.GetArray(zleWatch)
	out := make([]watcher, 0, len(flat)/3)
	for i := 0; i+3 <= len(flat); i += 3 {
		out = append(out, watcher{
			fd:      flat[i],
			handler: flat[i+1],
			widget:  flat[i+2] == zleWatchWidget,
		})
	}
	return out
}

func writeWatchers(r *interp.Runner, watchers []watcher) {
	flat := make([]string, 0, len(watchers)*3)
	for _, w := range watchers {
		flag := ""
		if w.widget {
			flag = zleWatchWidget
		}
		flat = append(flat, w.fd, w.handler, flag)
	}
	r.SetArray(zleWatch, flat)
}

// writeWatcher arms one, replacing an entry for the same descriptor **where it
// stands**: measured, re-arming keeps its place in the listing, and a plain
// re-arm of one that had `-w` drops the `-w`.
func writeWatcher(r *interp.Runner, add watcher) {
	watchers := readWatchers(r)
	for i, w := range watchers {
		if w.fd == add.fd {
			watchers[i] = add
			writeWatchers(r, watchers)
			return
		}
	}
	writeWatchers(r, append(watchers, add))
}

func removeWatcher(r *interp.Runner, fd string) bool {
	watchers := readWatchers(r)
	for i, w := range watchers {
		if w.fd == fd {
			writeWatchers(r, append(watchers[:i:i], watchers[i+1:]...))
			return true
		}
	}
	return false
}

// **The number a script says is not the number the kernel waits on.** A shell
// models its own descriptor table: `exec {AFD}< …` — which is how every real
// use of this arms itself — puts an open file at a number the *shell* chose,
// and the descriptor the kernel gave that file is a different number
// altogether. So `zle -F $AFD handler` records the shell's number, because
// that is what the handler's `read -u $1` has to be able to use, and what goes
// to repl to be waited on is what interp's SystemDescriptor translates it to.
//
// Getting that backwards is a bug with no symptoms in a unit test — a test
// that hands the dialect a bare number cannot tell the two apart — and no
// behavior at all in a real session, because the wait would be on a
// descriptor belonging to something else entirely, or on nothing. It is the
// reason this file's end-to-end test opens a pipe *from the shell* rather than
// from Go.

// WatchedDescriptors is which descriptors this shell has armed, as the kernel
// numbers them.
//
// The dialect's answer to driver.Shell.WatchedDescriptors, asked before every
// wait for a key. A watcher whose number nothing is open at is left out: it
// cannot be waited on, and leaving it out is what makes it *never fire*, which
// is exactly what zsh does with a watcher on a descriptor that was never open.
func WatchedDescriptors(r *interp.Runner) []int {
	watchers := readWatchers(r)
	fds := make([]int, 0, len(watchers))
	for _, w := range watchers {
		if system, open := systemDescriptor(r, w); open {
			fds = append(fds, system)
		}
	}
	return fds
}

// systemDescriptor translates one watcher's number into the kernel's.
func systemDescriptor(r *interp.Runner, w watcher) (int, bool) {
	fd, err := strconv.Atoi(w.fd)
	if err != nil {
		return 0, false
	}
	return r.SystemDescriptor(fd)
}

// DescriptorReady runs what was armed for a descriptor that has become
// readable, and answers with the line as the callback left it.
//
// The dialect's answer to driver.Shell.DescriptorReady. false is the ordinary
// case and does not mean refusal: it means the line is untouched and nothing
// is to be drawn, which is what the plain spelling of `zle -F` measures as.
// Only `-w` on a handler that is really a widget answers true.
//
// The status goes in and does not come out, measured, and that is done here
// rather than in repl for the reason zle.go gives: what a shell's own code
// leaves behind is the shell's business.
func DescriptorReady(r *interp.Runner, ctx context.Context, fd int, in repl.Line) (repl.Line, bool) {
	armed, found := watcherFor(r, fd)
	if !found {
		return in, false
	}
	// The handler is told the number *the script* armed, not the one the
	// kernel woke on, because `read -u $1` inside it is a question about the
	// shell's table. This is the same translation WatchedDescriptors did, read
	// the other way.
	if armed.widget {
		// `-w` wants the line, and only a widget can be given it. A handler
		// that is not one never fires at all — measured, in silence — so the
		// widget table is the whole of the question here.
		return runWidgetFunction(r, ctx, armed.handler, in, armed.fd)
	}
	runPlainHandler(r, ctx, armed.handler, armed.fd)
	return in, false
}

// runPlainHandler calls a handler that is an ordinary function, with the
// descriptor as its one argument.
//
// The line is not published: measured, `BUFFER`, `CURSOR` and `$WIDGET` are
// unset inside a plain handler. What *is* published is that the editor is
// running — measured, `zle some-widget` from inside one is status 0 — so this
// is not the script case even though the parameters are not there.
//
// Nothing comes back, because nothing can: whatever the handler printed went
// where the cursor was and the line was left alone, which is what zsh does and
// which is why plugins call the redisplay commands themselves.
func runPlainHandler(r *interp.Runner, ctx context.Context, handler, arg string) {
	if !r.HasFunction(handler) {
		// A name nothing answers to. Silent and harmless, the way arming one
		// was: the table is allowed to hold a handler whose function has not
		// arrived, or has gone.
		return
	}
	restore := openEditorActive(r)
	defer restore()
	status := r.ExitStatus()
	_, _ = r.CallFunction(ctx, handler, arg)
	r.SetExitStatus(status)
}

// openEditorActive marks the editor as running for the length of a call, and
// hands back what puts it as it was.
//
// Around the call rather than for the session, so that a script at the next
// prompt still finds `zle some-widget` refused — the same discipline
// openWidgetParameters keeps, and deferred for the same reason: a panic in the
// handler is caught outside this call.
func openEditorActive(r *interp.Runner) func() {
	was, _ := r.GetVar(zleActive)
	r.SetVar(zleActive, "1")
	// Back to whatever it was, which is the empty string outside a widget —
	// and the empty string is what editorRunning reads as "not running", for
	// the reason that predicate exists at all.
	return func() { r.SetVar(zleActive, was) }
}

// watcherFor is the entry armed for a descriptor the kernel woke on, if there
// is one.
//
// The search is by the *system* number and the entries are stored by the
// shell's, so each one is translated as it is considered — the reverse of what
// WatchedDescriptors did on the way out. Where two of the shell's numbers are
// duplicates of one descriptor the first armed wins, which is the same order
// the listing is in and is the only answer that does not depend on a map.
//
// The shell's own number is accepted as well, and it costs nothing to accept:
// where the two coincide — the named three in a session, most plainly — it is
// the same entry, and where they do not, a number no descriptor translates to
// can only have come from the table itself.
func watcherFor(r *interp.Runner, fd int) (watcher, bool) {
	watchers := readWatchers(r)
	for _, w := range watchers {
		if system, open := systemDescriptor(r, w); open && system == fd {
			return w, true
		}
	}
	want := strconv.Itoa(fd)
	for _, w := range watchers {
		if w.fd == want {
			return w, true
		}
	}
	return watcher{fd: "", handler: "", widget: false}, false
}
