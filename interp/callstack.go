// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

// Where a shell is, in the sense a script means when it asks.
//
// `${BASH_SOURCE[0]}` is how a script finds its own directory, and it is one
// of the first things a great many of them do — which is why the answer has
// to be produced rather than stored. It changes with every function call and
// every sourced file, and a plain array set once would be right until the
// first `source` and silently wrong afterwards, which is the worst shape a
// bug can have here.
//
// The core keeps the stack and names none of it. Three shells expose it under
// three different sets of names, and a fourth exposes nothing at all, so
// naming is the dialect's the way every other wording is.

// sourceFrameName is what a sourced file's frame is called, which is what
// tells it apart from a function's. It is the builtin's own name because that
// is what the shells put in their stacks, and it is spelled once here because
// four places now ask the question — including `$0`, where reading a
// function's frame as a file's would answer with the wrong kind of thing.
const sourceFrameName = "source"

// Frame is one entry of the call stack.
type Frame struct {
	// File is the file being read in this frame: the one a function was
	// *defined* in, or the one that was sourced.
	File string

	// Name is the function's name, or sourceFrameName for a sourced file.
	// Empty for the script itself.
	Name string

	// Operand is the word `.` was given, exactly as the shell constructed
	// it and before any PATH search — which is a different string from File
	// whenever the search found the file somewhere else. Empty for a
	// function's frame, where Name is the answer instead.
	//
	// It exists because `$0` and a diagnostic disagree about a sourced file
	// found on PATH, measured: `PATH=dir; . inc.sh` reports `$0` as the bare
	// `inc.sh` and locates a failure inside the file at `dir/inc.sh`. So
	// File cannot serve both, and using it for `$0` would have answered with
	// a path the script never wrote.
	Operand string

	// Line is the line this frame was entered from, in the frame below it.
	Line int

	// OuterFunc and OuterFuncLine are the function the frame was entered
	// from and the line that function was written on — the pair
	// locationPrefixNamed counts a message's line against, as it stood one frame
	// down. They are Line's companions and are filled in beside it: Line
	// alone says where the call was made and not what it was made *inside*,
	// and a diagnostic located at the call needs both. See
	// [Runner.LocatedAtTheCall].
	//
	// Exported for the same reason Line is: one dialect reports a call site
	// **counted from the calling function** rather than from the file — the
	// offset into the caller's body, which is the pair of numbers Line alone
	// cannot produce. OuterFunc is empty where the call was made from a file
	// rather than from inside a function body, and the file is then the File
	// of the frame below this one.
	OuterFunc     string
	OuterFuncLine int

	// Keyword marks a function defined with the `function` keyword rather
	// than with `name()`, for the dialect whose `$0` answers only for those
	// — see [DollarZeroIsTheInnermostKeywordFunction]. False for a sourced
	// file and for a startup file, neither of which is a function at all.
	Keyword bool

	// Startup marks a frame the *shell* entered rather than a script: a
	// run-commands file, the login profile, `$ENV`, `$BASH_ENV`.
	//
	// It exists because such a file is a file the shell is *in* — a
	// diagnostic raised at its top level names it, which is what the frame
	// is for — and is at the same time outside the rule that lets `$0` name
	// the innermost call. Measured: a `~/.zshrc` printing `$0` under `zsh
	// -i` prints the path of the zsh binary, not the rc's, in the one shell
	// whose `$0` follows the stack at all. Without the mark, giving the file
	// a frame would have moved `$0` with it (#1123).
	Startup bool

	// outerParams is the positional list that was in effect when this call
	// was made — the list of the frame *below* this one, saved on the way in
	// and put back on the way out.
	//
	// The same shape scopeBase has and for the same reader: a frame
	// selection answers `$1`, `$@`, `$*` and `$#` from the frame a script
	// selected, and the positional list has no save-and-restore structure of
	// its own for that walk to read. `Runner.Params` is one slice swapped
	// around a call, so before this nothing on the stack knew what any other
	// frame's arguments were.
	//
	// Kept on the *callee* rather than the caller because that is where it
	// can be written: the caller's frame is already on the stack and the
	// list is saved by the call. So the frame standing at level `n+1` is
	// what answers for level `n`, which is also how level 0 gets an answer
	// with no frame of its own. See interp/frameparams.go.
	//
	// The same slice the call's own restore holds, aliasing included: a
	// write that reaches into the backing array — `${3}=x` over a standing
	// list — was always visible to the restore and is visible here in
	// exactly the same way.
	outerParams []string

	// scopeBase is the first index of r.scopes that is *inner* to this
	// frame: the scope the call itself opened is one below it, and every
	// scope from here up was opened by something this frame went on to run.
	//
	// It is the window a frame selection reads and writes through. The
	// scopes here are save-and-restore records rather than a chain of
	// tables, so "what does this name hold in that frame" is answered by
	// the innermost thing that displaced it — see selectedScopeBase.
	//
	// Stamped where the call's scope is pushed rather than computed from the
	// frame's position, because a scope is not only a call: one dialect's
	// `${ … ;}` body opens one too, and counting frames would have put the
	// window in the wrong place for any stack with one in it.
	scopeBase int

	// serial numbers this frame among every frame ever pushed, so a
	// sibling entered later at the same depth is still a different frame —
	// which is the distinction the RETURN trap turns on.
	serial int
	// ran says this frame has dispatched at least one simple command.
	//
	// One diagnostic reads it, and it is the only thing that tells the two
	// halves of a measured rule apart: a refusal raised by the *first*
	// command a call runs is spoken for by the call, and the same refusal
	// anywhere later in the same call is spoken for by nobody. See
	// Runner.operandLocatedUnderTheCall for the nine shapes that pin it, and
	// Runner.simple for where this is set — after the command rather than
	// before it, because the command that raises the refusal is itself the
	// first one.
	ran bool

	// zeroName is a `$0` this frame was *given*, and zeroNameHeld says it
	// was. One dialect lets a builtin's output operand name position 0, and
	// what that writes is a value the frame carries rather than a parameter:
	// it answers inside the call that wrote it, it is gone when the call
	// returns, and a call made afterwards still reports its own name. So it
	// sits beside the frame for the same reason the positional list is saved
	// and restored around a call, and the frame the call pushes starts
	// without one — see [Runner.dollarZero] and #3672.
	zeroName     string
	zeroNameHeld bool
}

// IsFunction reports whether this frame is a function call, as against a
// sourced file, a startup file the shell read of its own accord, or the
// script itself.
//
// Structural rather than a naming: which frames are calls of a *function* is
// the stack's own shape, and one dialect counts exactly those — `${.sh.level}`
// is 2 two functions deep and 0 at the top, where `${BASH_SOURCE[@]}` next to
// it counts every frame there is. A function really named `source` is the one
// thing this cannot tell apart, which is the same seam innermostCall works
// on.
func (f Frame) IsFunction() bool {
	return !f.Startup && f.Name != "" && f.Name != sourceFrameName
}

// CallStack is the frames a shell is currently inside, innermost first, with
// the script itself last.
//
// The script's own frame is there whenever there is a script, even at the top
// level with nothing called: a script that asks where it is expects an answer
// before it has done anything. A shell given `-c` or standard input has no
// such frame, because there is no file — and that is measured rather than
// assumed: bash inside a function called from `-c` reports one frame and not
// two, and reports nothing at all outside one.
func (r *Runner) CallStack() []Frame {
	out := make([]Frame, 0, len(r.frames)+1)
	for i := len(r.frames) - 1; i >= 0; i-- {
		out = append(out, r.frames[i])
	}
	if r.scriptFile != "" {
		out = append(out, Frame{File: r.scriptFile})
	}
	return out
}

// HasScriptFrame reports whether the bottom of the stack is a script.
//
// The dialect that names the bottom frame `main` names it only when it is
// there, so this is the question that decides rather than the file's text.
func (r *Runner) HasScriptFrame() bool { return r.scriptFile != "" }

// InCall reports whether anything has been called: a function entered or a
// file sourced.
//
// One dialect's name for the innermost function is absent at the top level
// rather than empty, which is a different thing to a script testing it.
func (r *Runner) InCall() bool { return len(r.frames) > 0 }

// SetScriptFile says which file the shell was given, for the bottom of the
// stack. The front end knows and the interpreter does not.
func (r *Runner) SetScriptFile(path string) { r.scriptFile = path }

// pushFrame enters a function or a sourced file.
//
// It reads where the shell is *now* for the frame's Line and for the
// enclosing function it records, so every route in gets the same answer from
// one place. That is why a function call pushes its frame before it moves the
// location into the body: the two facts are about the caller, and a push made
// after the move would record the callee's.
func (r *Runner) pushFrame(f Frame) {
	f.Line = r.line
	f.OuterFunc, f.OuterFuncLine = r.inFunc, r.funcLine
	r.frameSerial++
	f.serial = r.frameSerial
	if f.File == "" {
		// A function defined where nothing was read from a file — `-c`, or
		// standard input — belongs to whatever the shell calls itself, which
		// is what bash puts there: `bash -c 'f(){ …; }; f'` reports the
		// frame's file as `bash`.
		f.File = r.Name
	}
	r.frames = append(r.frames, f)
}

// heldDollarZero is the `$0` the innermost frame was given, if it was given
// one.
//
// The script's own level is not in r.frames — it is the shell rather than a
// call — so the runner carries that one itself and this is the single place
// that knows which of the two answers.
//
// It takes the stack as a parameter rather than reading r.frames, because
// the one reader has a *view* of it to ask: a frame selection cuts the stack
// at the selected frame, and the answer is then that frame's stored `$0`
// rather than the running frame's. See interp/frameparams.go.
func (r *Runner) heldDollarZero(frames []Frame) (string, bool) {
	if n := len(frames); n > 0 {
		f := frames[n-1]
		return f.zeroName, f.zeroNameHeld
	}
	return r.zeroName, r.zeroNameHeld
}

// storeDollarZero puts a value where heldDollarZero will find it.
//
// Nothing is pushed or popped: a frame already ends when its call does, so a
// value written onto one goes away with it and a value written at the top
// level lasts as long as the shell does. That is the whole of the measured
// behavior and it needs no unwinding of its own.
func (r *Runner) storeDollarZero(value string) {
	if n := len(r.frames); n > 0 {
		r.frames[n-1].zeroName = value
		r.frames[n-1].zeroNameHeld = true
		return
	}
	r.zeroName, r.zeroNameHeld = value, true
}

// popFrame leaves one.
func (r *Runner) popFrame() {
	if len(r.frames) > 0 {
		r.frames = r.frames[:len(r.frames)-1]
	}
}

// innermostCall is what the shell is inside, for the dialect that lets `$0`
// say: the name of the function being run, or the path of the file being
// sourced, whichever was entered last.
//
// It reports false at the top level, where there is nothing to name and `$0`
// is the shell's own. The script's own frame is deliberately not consulted —
// it is not in r.frames, and a script naming itself is what `$0` already
// answers without any of this.
//
// A sourced file answers with the operand rather than the file, which is not
// the same string for a file found on PATH. See Frame.Operand.
//
// The stack is a parameter because `$0` has a *view* of it to ask — a frame
// selection cuts it at the selected frame — and a diagnostic's location does
// not: that has a selection mechanism of its own, and passes r.frames. See
// interp/frameparams.go.
func (r *Runner) innermostCall(frames []Frame) (string, bool) {
	if len(frames) == 0 {
		return "", false
	}
	f := frames[len(frames)-1]
	if f.Startup {
		// A startup file is read by the shell rather than called by a
		// script, so there is nothing here for `$0` to name — see
		// Frame.Startup. A file *it* sources is an ordinary frame above
		// this one and answers for itself.
		return "", false
	}
	if f.Name == sourceFrameName {
		return f.Operand, true
	}
	return f.Name, true
}

// innermostKeywordFunction is the nearest function on the stack that was
// defined with the `function` keyword, for the dialect whose `$0` names one.
//
// It walks down rather than looking at the top frame, and that is the
// measurement rather than a convenience: in the shell that has this, a
// `name()` function called from inside a `function` one still reports the
// outer function's name, and so does a file sourced from inside one. So the
// frames that do not answer are transparent instead of being an answer of
// their own.
//
// A startup file stops the walk. It is read by the shell rather than called
// by a script — see [Frame.Startup] — so there is nothing below it a script
// named, and a function it happens to be running is the shell's own.
//
// The stack is a parameter for the reason innermostCall's is.
func (r *Runner) innermostKeywordFunction(frames []Frame) (string, bool) {
	for i := len(frames) - 1; i >= 0; i-- {
		f := frames[i]
		if f.Startup {
			return "", false
		}
		if f.Keyword {
			return f.Name, true
		}
	}
	return "", false
}

// currentFile is the file being read now, which is what a function defined
// here will remember.
func (r *Runner) currentFile() string {
	if len(r.frames) > 0 {
		return r.frames[len(r.frames)-1].File
	}
	return r.scriptFile
}

// LocatedAtTheCall reports something the shell did *for* a call rather than
// something the call did, and locates it where the call was made.
//
// A shell writes such a message when it fails to produce the thing a name
// stands for — a function body it had to fetch before the call could run.
// The work happens inside a frame the script never wrote, so the ordinary
// location names a place nobody can open: a generated stub is one line long
// and named after the function, which is how eight lines of a real startup
// came out as `is-at-least:1:` and `add-zsh-hook:1:` (#1994).
//
// One mechanism rather than a second diagnostic helper: it wraps whichever
// of [Runner.Diagnosef] and [Runner.DiagnoseAsTheShellf] the message already
// used, so the voice and the location stay separate questions and a dialect
// cannot pick one and forget the other.
//
// What steps out is the *location* and not the shell. The line, and the
// function a line is counted within, come from the frame below; `$0` does
// not move, because the shell is still inside the call — see locationFile
// for the one row of the measurement where that shows.
//
// At the top level, where there is no call to step out of, it is the report
// on its own.
func (r *Runner) LocatedAtTheCall(report func()) {
	n := len(r.frames) - r.outsideCall
	if n <= 0 {
		report()
		return
	}
	f := r.frames[n-1]
	savedFunc, savedFuncLine, savedLine := r.inFunc, r.funcLine, r.line
	r.inFunc, r.funcLine, r.line = f.OuterFunc, f.OuterFuncLine, f.Line
	r.outsideCall++
	defer func() {
		r.outsideCall--
		r.inFunc, r.funcLine, r.line = savedFunc, savedFuncLine, savedLine
	}()
	report()
}

// locationFile is the file a diagnostic names: currentFile, except that it
// steps out of whatever [Runner.LocatedAtTheCall] stepped out of.
//
// The last clause is the whole of the difference and it is measured. Stepping
// out of the only call there was leaves the top level of a script, where the
// name a diagnostic carries is a *file* — and in the dialect that names one
// here at all, the file it names is `$0`, which is still the call's: the
// shell has not left it, only the location has. So a function file missing at
// the top level of a script names the *function* and the caller's line
// together, where the same failure one frame deeper names the caller, and
// where turning that dialect's `$0` rule off names the script. All three were
// measured on zsh 5.9.2, and the third is what ties the row to `$0` rather
// than to the stub.
//
// Only where there is a script. Under `-c` and on standard input the top
// level has no file, the dialect answers with its own fixed name instead, and
// the same failure there is `zsh:3:` and `zsh:` — measured, and the reason
// the scriptFile test is part of the condition rather than of the fallback.
//
// The answer is compared rather than asked: [Runner.ask] writes a diagnostic
// for an axis nobody answered, and this runs while one is being written.
func (r *Runner) locationFile() string {
	if n := len(r.frames) - r.outsideCall; n > 0 {
		return r.frames[n-1].File
	}
	if r.outsideCall > 0 && r.scriptFile != "" && r.sem().DollarZeroNames == DollarZeroIsTheInnermostCall {
		if in, ok := r.innermostCall(r.frames); ok {
			return in
		}
	}
	return r.scriptFile
}

// locationIsInsideAFunctionBody reports whether the line a diagnostic is
// about was read from a function's body rather than from a file the shell is
// reading, which is what decides between the two ways a location can be
// written: the dialect that names a function names it for a line the function
// contains, and names a file for every other line.
//
// The innermost frame is what answers, and that is measured rather than
// reasoned: a function that sources a file is still the innermost *function*
// while the file runs, so `r.inFunc` alone named the function for a line it
// never contained — `q:1:` where zsh 5.9.2 writes `/tmp/inc:2:` (#2037). The
// stack already tells the two kinds apart, so the question is which frame to
// ask rather than what to remember.
//
// A frame the location has stepped out of does not answer — see
// [Runner.LocatedAtTheCall] — so this counts from the same depth
// [Runner.locationFile] names the file at, and the two cannot disagree.
//
// Nesting either way is the same question asked once: a file sourced by a
// sourced file inside a function is a file, and a function defined *in* a
// sourced file is a function wherever it was read from. Both measured.
func (r *Runner) locationIsInsideAFunctionBody() bool {
	if r.inFunc == "" {
		return false
	}
	n := len(r.frames) - r.outsideCall
	if n <= 0 {
		// Nothing left to ask, which is the top level: r.inFunc is empty
		// there, so this is reached only if a frame recorded an enclosing
		// function the stack no longer holds.
		return true
	}
	return !r.frames[n-1].readsAFile()
}

// readsAFile reports whether this frame is a file the shell is reading — a
// sourced file, or a startup file it read for itself — rather than a function
// body it is running.
func (f Frame) readsAFile() bool { return f.Name == sourceFrameName || f.Startup }

// The frame a script has *pointed* the shell's own location parameters at,
// which is not always the one it is standing in.
//
// One dialect has a pair of parameters naming the function running now and
// how deep it is, and writing to the depth one **selects a frame**: the name
// then answers for that frame instead. It is how a debug trap looks at its
// caller without the caller having handed anything down. The core keeps the
// mechanism and names neither parameter — see dialect/ksh, where the two are
// registered as produced parameters with writers.
//
// Measured on AT&T ksh93u+ 2012-08-01, 2026-09-16, and the shape is not the
// obvious one:
//
//   - Levels count *function* frames, innermost last: `1` is the outermost
//     function on the stack and the current depth is the innermost. `0` is
//     the top level, where the name is empty.
//   - A level outside `0…depth` **does nothing at all** — it does not clamp
//     and it does not reset. Inside a function two deep, `.sh.level=1`
//     followed by `.sh.level=9` still reads `1`.
//   - Except at the top level, where there is no frame to be out of range of
//     and the number is simply kept: `.sh.level=7` there reads back `7` and
//     `.sh.level=-1` reads back `-1`, both with an empty name.
//   - The selection lasts as long as the frame that made it. A function that
//     selects its caller and then calls something sees the callee's own
//     answers while it runs, and the top level is back to `0` when
//     everything returns.
//   - The name is separately assignable, and an assignment to it is a plain
//     string: `.sh.fun=zzz` reads back `zzz` until the frame ends. Writing
//     the level afterwards replaces it, because the level's answer is
//     computed from the stack.
//
// The *frame* the selection was made in is recorded with it, which is the
// whole of "lasts as long as the frame": a read from anywhere else finds the
// stamp stale and takes the stack's own answer. The frame's serial and not
// its depth, because two functions called one after the other stand at the
// same depth and are not the same frame — with a depth stamp, a name written
// in the first was still there in the second. A scalar rather than a stack,
// so a subshell gets its own copy of it the way every other scalar here does.
//
// Two further things move with the selection, and each is a mechanism of its
// own rather than a second answer from this one: the **variable scope** goes
// there, so a function that selects its caller reads the caller's locals (see
// interp/framescope.go, #3115), and so do the **positional parameters and
// `$0`** (see interp/frameparams.go, #3952).

// FunctionDepth is how many function calls the shell is inside, not counting
// sourced files or the startup files it read of its own accord.
func (r *Runner) FunctionDepth() int {
	depth := 0
	for _, f := range r.CallStack() {
		if f.IsFunction() {
			depth++
		}
	}
	return depth
}

// SelectedCallFrame is the frame the location parameters answer about — the
// one a script selected, or the innermost — and the function running there.
//
// The name is empty for level 0 and for a level with no frame at it, which is
// the same answer the top level gives when nothing has been called.
func (r *Runner) SelectedCallFrame() (level int, name string) {
	depth := r.FunctionDepth()
	if !r.frameSelected || r.frameSelectedAt != r.innermostFrameSerial() {
		return depth, r.functionAtLevel(depth, depth)
	}
	if r.frameNamed {
		return r.selectedFrame, r.selectedFrameName
	}
	return r.selectedFrame, r.functionAtLevel(r.selectedFrame, depth)
}

// functionAtLevel is the function running at one level, counting the
// outermost as 1 — empty at level 0 and at any level the stack has no frame
// for.
func (r *Runner) functionAtLevel(level, depth int) string {
	if level <= 0 || level > depth {
		return ""
	}
	// CallStack is innermost first, so the innermost function is the current
	// depth and each one out is a level lower.
	at := depth
	for _, f := range r.CallStack() {
		if !f.IsFunction() {
			continue
		}
		if at == level {
			return f.Name
		}
		at--
	}
	return ""
}

// SelectCallFrame points the location parameters at a frame, reading the text
// a script assigned as arithmetic — which is what an assignment to an integer
// parameter is, and is why `n=1; .sh.level=n` selects frame 1 rather than
// nothing.
//
// A level outside the stack is heard and does nothing, leaving whatever was
// selected before. The top level is the exception the comment above records.
func (r *Runner) SelectCallFrame(text string) {
	level, ok := r.arithLevel(text)
	if !ok {
		return
	}
	depth := r.FunctionDepth()
	if depth > 0 && (level < 0 || level > depth) {
		return
	}
	r.selectedFrame, r.frameSelected = level, true
	r.frameSelectedAt = r.innermostFrameSerial()
	// A level names a frame, so it replaces whatever name was written into
	// the pair: measured, `.sh.fun=zzz` then `.sh.level=0` reads the name
	// back empty, where the two in the other order keep `zzz`.
	r.frameNamed, r.selectedFrameName = false, ""
}

// NameSelectedCallFrame writes the name the location parameters answer with,
// which the dialect's own naming parameter is assignable to. It lasts as long
// as the frame that wrote it, exactly as a selected level does.
func (r *Runner) NameSelectedCallFrame(name string) {
	if serial := r.innermostFrameSerial(); !r.frameSelected || r.frameSelectedAt != serial {
		r.selectedFrame, r.frameSelected, r.frameSelectedAt = r.FunctionDepth(), true, serial
	}
	r.frameNamed, r.selectedFrameName = true, name
}

// innermostFrameSerial identifies the frame a selection belongs to. Zero at
// the top level, where there is no frame and the selection lasts until one is
// entered and left again.
func (r *Runner) innermostFrameSerial() int {
	for i := len(r.frames) - 1; i >= 0; i-- {
		if r.frames[i].IsFunction() {
			return r.frames[i].serial
		}
	}
	return 0
}

// arithLevel reads an assigned level. A text that is not an expression at all
// is `0` rather than a refusal — measured, `.sh.level=abc` selects the top
// level and reports success, which is what an unset name comes to in
// arithmetic.
func (r *Runner) arithLevel(text string) (int, bool) {
	if text == "" {
		return 0, true
	}
	tree, err := r.arithTreeRead(text)
	if err != nil || tree == nil {
		return 0, true
	}
	n, err := r.evalArith(tree)
	if err != nil {
		return 0, true
	}
	return n, true
}
