// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package repl

import (
	"context"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/blairham/sh/internal/prompttheme"
	"github.com/blairham/sh/internal/repostatus"
	"github.com/blairham/sh/interp"
)

// The prompt theme engine, wired to a session.
//
// docs/spec/prompt-theme.md is the design. internal/prompttheme is the engine
// and imports no dialect, no interpreter and no editor — it takes what it
// needs as functions — so this file is where those functions come from, and
// it is the whole of the coupling between the two.
//
// A theme is a capability of the substrate rather than of a language, which
// is the point of the exercise: a script run under `dash` gets the same
// prompt as one run under `zsh`, because the front end draws it and the
// language never sees it.

// Theme draws a prompt from a configuration.
//
// It satisfies PromptTheme, so a front end wires one by setting Shell.Theme.
// Whether it *draws* is decided by the configuration and not by the wiring:
// with no elements configured it answers that it is not drawing, and the
// prompt is the person's own parameter exactly as it would be with no theme
// at all. That is what makes turning it off cost nothing to restore.
type Theme struct {
	// Char is the prompt character a bare prompt draws, and Continued is the
	// prompt for the rest of an unfinished construct. A front end sets them
	// where its shell prompts with something else; this package names no
	// shell, so the defaults are the neutral ones.
	Char      string
	Continued string

	get    func(name string) (string, bool)
	roster *prompttheme.Roster

	// file is the configuration file layer, rebuilt when SH_PROMPT_CONFIG
	// names a different one, and nil while it names nothing. Named or
	// nowhere: an unset variable is a person who has not asked for a file.
	file     *prompttheme.File
	filePath string

	// preset is the look SH_PROMPT_PRESET names — one this binary carries or
	// a file somewhere — and presetTrouble is what to say where it named
	// neither. Under the file in the stack, because a preset is what somebody
	// started from and a file is what they changed about it.
	preset        prompttheme.Layer
	presetName    string
	presetTrouble string

	settings *prompttheme.Settings
	engine   *prompttheme.Engine

	// iconSet is the table SH_PROMPT_ICONS last named, kept so that a render
	// costs a lookup rather than a parse. The name it was asked for is kept
	// beside it because a table that was named and is not carried is served
	// by the default and has to say so.
	iconSet  *prompttheme.IconSet
	iconName string

	// published is how a segment that computes its answer somewhere else
	// tells the session to draw the prompt again, or nil between sessions.
	// Written by the session that owns the theme and read by whatever
	// goroutine did the computing, which is what the lock is for.
	mu          sync.Mutex
	published   func()
	sessionHas  func(string) bool
	sessionCall func(string) (string, bool)

	// repos is the repository-status capability this session's prompt draws
	// from — a resident cache kept honest by a filesystem watch, which opens
	// nothing until something asks it a question. See internal/repostatus.
	repos *repostatus.Cache
}

// Close stops anything this theme started, which today is the repository
// watch. A session does it through PublishTo(nil); this is the same thing by
// name, for an embedder that built a theme and is finished with it.
func (t *Theme) Close() error { return t.repos.Close() }

// PublishTo satisfies PromptPublisher: a session hands the theme the way to
// say that a redraw would differ, and takes it back when the session ends.
//
// Nothing about an asynchronous segment reaches this package's own segments
// through here. They are given Publish below and know nothing of sessions,
// which is what keeps a segment the same thing whether it computes its
// answer here or somewhere else.
func (t *Theme) PublishTo(publish func()) {
	t.mu.Lock()
	t.published = publish
	t.mu.Unlock()
	if publish == nil {
		// The session has ended. Everything this theme started stops with
		// it: a watch left running would be a goroutine holding descriptors
		// on a repository nobody is looking at any more.
		_ = t.repos.Close()
	}
}

// Publish says that what this theme would draw has changed.
//
// Safe from any goroutine, and nothing at all when no session is listening —
// a scanner that finishes after the shell has gone is an ordinary outcome and
// not an error. It is a statement rather than a request: the session decides
// whether anything is actually redrawn, and a prompt that renders the same is
// not written to the screen.
func (t *Theme) Publish() {
	t.mu.Lock()
	publish := t.published
	t.mu.Unlock()
	if publish != nil {
		publish()
	}
}

// NewTheme returns a theme reading its settings through get, which is a
// session's own variable lookup.
//
// Through the shell's variables rather than the process environment — the
// rule the block store and HISTFILE already follow, and for the same reason:
// a session can set one at the prompt and mean it. It is also what makes
// configuration work identically in all five dialects, since every dialect
// has variables even where it has no hooks and no arrays.
func NewTheme(get func(name string) (string, bool)) *Theme {
	t := &Theme{Char: "$", Continued: "> ", get: get}
	t.roster = prompttheme.NewRoster()
	for element, segment := range prompttheme.CoreSegments() {
		t.roster.Compile(element, segment)
	}
	// Through a closure rather than PromptChar(t.Char) directly, so that a
	// front end setting Char after this returns gets the character it set
	// rather than the one that was there when the roster was built.
	t.roster.Compile("prompt_char", prompttheme.SegmentFunc(
		func(settings *prompttheme.Settings, ctx *prompttheme.Context) (prompttheme.Rendered, bool) {
			return prompttheme.PromptChar(t.Char).Render(settings, ctx)
		}))
	// The interpreter's strftime, which is the same one `printf '%(fmt)T'`
	// and zsh's `strftime` builtin write through. One reader of a format
	// language, for the reason that one is exported: two would drift the
	// first time a conversion was fixed in either.
	t.roster.Compile("time", prompttheme.Clock(interp.Strftime))
	// And the repository, which is the first segment whose answer can change
	// while nobody is typing. The cache publishes through the theme, so a
	// branch moved in another terminal redraws the prompt somebody is
	// sitting in front of — and it opens no descriptor and starts no
	// goroutine until a configuration names this element in a directory that
	// is actually in a repository.
	t.repos = repostatus.New(t.Publish)
	t.roster.Compile("vcs", prompttheme.Repository(t.repository))
	// And the session's own functions, ahead of everything compiled in: the
	// person who defined a segment in their own startup file is the most
	// present author of it. Nothing is consulted for an element no
	// configuration named, and a name that is not a function is not a
	// segment, so a session that defines none pays one map lookup per named
	// element and nothing else.
	t.roster.Consult(prompttheme.Functions("a session function", t.hasFunction, t.callFunction))
	t.engine = &prompttheme.Engine{
		Roster: t.roster,
		Screen: prompttheme.Screen{
			// The editor's own cell measurement, against the generated East
			// Asian tables, and the editor's own non-printing markers. Both
			// borrowed rather than reimplemented: a second reader of either
			// question is how a fix lands in one of them and not the other.
			Width: displayWidth,
			Mark:  NonPrinting,
		},
	}
	return t
}

// DrawPrompt draws one prompt, or reports that no configuration asked for one.
func (t *Theme) DrawPrompt(info PromptInfo) (ThemedPrompt, bool) {
	settings := t.resolve()
	if !settings.Has("LEFT_ELEMENTS") && !settings.Has("RIGHT_ELEMENTS") {
		// Nothing asked for a theme. Answering "not drawing" rather than
		// drawing a bare character is what keeps a wired theme from
		// overriding a prompt somebody set: the engine's own bare prompt is
		// for a configuration that names no elements, not for a session that
		// named no configuration.
		//
		// Whether the setting is *there*, not whether it has anything in it.
		// Set-to-empty is an answer everywhere else in this namespace and it
		// is one here: emptying the elements is a themed prompt with no
		// segments in it, and unsetting them is no theme at all. Reading the
		// two the same way would leave no way to say the first, and would
		// make a preset that empties a side turn the whole theme off.
		return ThemedPrompt{}, false
	}

	t.engine.Settings = settings
	t.engine.Icons = t.icons(settings).Glyph
	t.engine.Bare = t.Char + " "
	t.engine.Continued = t.Continued
	drawn := t.engine.Render(t.context(info))
	return ThemedPrompt{Text: drawn.Text, Cont: drawn.Cont, Right: drawn.Right}, true
}

// Settings is what the theme currently resolves, for something that reports a
// configuration rather than drawing one.
func (t *Theme) Settings() *prompttheme.Settings { return t.resolve() }

// Problems is what was read and not honored — a line in the file that is not
// an assignment, a file that was named and is not there, an element nothing
// answers.
func (t *Theme) Problems() []string {
	var problems []string
	if t.file != nil {
		problems = append(problems, t.file.Problems()...)
	}
	for _, element := range t.roster.NotYet() {
		problems = append(problems, "no segment draws "+element)
	}
	if t.presetTrouble != "" {
		problems = append(problems, t.presetTrouble)
	}
	for _, shadow := range t.roster.Shadowed() {
		problems = append(problems, shadow+" is drawn by a session function and not by the built-in segment")
	}
	if trouble := t.repos.Trouble(); trouble != "" {
		// Behavior where watches are unavailable must degrade to something
		// honest and never to a silently stale answer. This is the honest
		// half: the prompt is one prompt late rather than wrong, and it says
		// which.
		problems = append(problems, trouble)
	}
	if set := t.icons(t.resolve()); !set.Carried() {
		// Silently substituting a different glyph set is how a prompt ends up
		// full of boxes with no explanation. The table still draws — a person
		// who named one wanted icons — and the substitution is named.
		problems = append(problems, "icon table "+set.Name+" is not carried; served by "+set.Served)
	}
	return problems
}

// useSession points the theme at the shell functions this session has, and at
// nil when the session ends.
//
// Unexported and reached from Shell.Run rather than through an interface,
// because calling a function needs the session's context and only this
// package has one — a front end wiring a theme has no moment with one in it.
// The resolver holds nothing but these two functions, so the end of a session
// takes the session with it and leaves the theme able to draw.
func (t *Theme) useSession(has func(string) bool, call func(string) (string, bool)) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.sessionHas, t.sessionCall = has, call
}

// hasFunction and callFunction are what the resolver is built on, read under
// the lock because the session that set them is not always the goroutine
// asking.
func (t *Theme) hasFunction(name string) bool {
	t.mu.Lock()
	has := t.sessionHas
	t.mu.Unlock()
	return has != nil && has(name)
}

func (t *Theme) callFunction(name string) (string, bool) {
	t.mu.Lock()
	call := t.sessionCall
	t.mu.Unlock()
	if call == nil {
		return "", false
	}
	return call(name)
}

// segmentFunctions is how this session's own shell functions are reached as
// prompt segments: whether a name is one, and what one writes when it is run.
//
// **In-process, with no fork**, which is the whole of why this is worth
// having: the interpreter is right here, so a segment nobody here has thought
// of costs a person a few lines in their own startup file and no process. It
// works in whatever dialect the session is running, because the function is
// that session's.
//
// Three things it does that a caller would otherwise get wrong:
//
//   - **The status is put back.** A segment is drawn between two commands and
//     must not be able to change what `$?` says about the one that ran —
//     which is what the hook chain already does, and for the same reason.
//   - **Both streams are captured, into one buffer in the order they were
//     written.** A prompt is drawn with the terminal in raw mode, where a
//     newline moves down without returning the carriage, so a function that
//     printed straight through would smear the screen. Capturing standard
//     error *with* standard output rather than discarding it is deliberate:
//     a function that complains draws its complaint, which is how the person
//     finds out, and a diagnostic dropped on the floor is the silent half of
//     the failure this repository treats as its worst.
//   - **The panic guard**, the same one a provider and a theme run behind. A
//     prompt that took the session down over a decorative segment would be
//     worse than the line that did.
//
// The streams are wrapped for the length of one call and put back
// immediately, which is the bargain lazyDiscipline already documents: an
// external command a segment function starts is on a pipe for that call. For
// a segment that is the right answer anyway, since its output is being read.
func (s Shell) segmentFunctions(ctx context.Context) (func(string) bool, func(string) (string, bool)) {
	if s.Runner == nil {
		return nil, nil
	}
	r, guard := s.Runner, s.guard()
	call := func(name string) (string, bool) {
		var out strings.Builder
		stdout, stderr := r.Stdout, r.Stderr
		status := r.ExitStatus()
		r.Stdout, r.Stderr = &out, &out
		defer func() {
			r.Stdout, r.Stderr = stdout, stderr
			r.SetExitStatus(status)
		}()

		ran := false
		if guard.Do(func() { ran, _ = r.CallFunction(ctx, name) }) {
			// It panicked. The report is already written and the segment
			// draws nothing, which is what an element with nothing to say
			// looks like everywhere else.
			return "", false
		}
		if !ran || r.ExitStatus() != 0 {
			// A name that is not a function and a function that failed are
			// ordinary outcomes rather than errors to report on a prompt.
			return "", false
		}
		return out.String(), true
	}
	return r.HasFunction, call
}

// repository is the capability's answer in the shape the segment takes.
//
// Two shapes rather than one because the engine names no capability: a
// segment is handed facts, and what computed them is this file's business.
// It is also what lets the segment be driven from a table in a test.
func (t *Theme) repository(dir string) (prompttheme.Repo, bool) {
	status, ok := t.repos.Status(dir)
	if !ok {
		return prompttheme.Repo{}, false
	}
	return prompttheme.Repo{
		Branch:    status.Branch,
		Commit:    status.Commit,
		Operation: status.Operation,
	}, true
}

// icons is the table this configuration asked for, parsed once per name.
func (t *Theme) icons(settings *prompttheme.Settings) *prompttheme.IconSet {
	name := settings.Str("ICONS", "")
	if t.iconSet == nil || name != t.iconName {
		t.iconName, t.iconSet = name, prompttheme.LoadIcons(name)
	}
	return t.iconSet
}

// resolve builds the layer stack for this prompt, earliest layer first.
//
// The file is re-read when its mtime has changed, which is how editing it
// takes effect on the next prompt with no reload command. The session's own
// variables are a lookup rather than a map, because no shell offers to
// enumerate a namespace and asking by name is what a three-step chain does
// anyway.
func (t *Theme) resolve() *prompttheme.Settings {
	path := ""
	if t.get != nil {
		path, _ = t.get(prompttheme.Prefix + "CONFIG")
	}
	preset := ""
	if t.get != nil {
		preset, _ = t.get(prompttheme.Prefix + "PRESET")
	}
	if preset != t.presetName {
		t.presetName = preset
		t.preset, t.presetTrouble = prompttheme.LoadPreset(preset)
		t.settings = nil
	}
	if file, ok := t.preset.(*prompttheme.File); ok {
		// A preset kept in a file is a file somebody is editing, and it is
		// re-read on the same terms the configuration file is.
		file.Refresh()
	}
	if path != t.filePath {
		t.filePath, t.file = path, nil
		if path != "" {
			t.file = prompttheme.OpenFile(path)
		}
		t.settings = nil
	}
	if t.file != nil {
		t.file.Refresh()
	}
	if t.settings == nil {
		var layers []prompttheme.Layer
		if t.preset != nil {
			layers = append(layers, t.preset)
		}
		if t.file != nil {
			layers = append(layers, t.file)
		}
		layers = append(layers, prompttheme.NewVars("session", t.get))
		t.settings = prompttheme.NewSettings(layers...)
	}
	return t.settings
}

// context is what a segment is allowed to know about this prompt.
//
// Everything in it comes from the session or from one process fact, and
// nothing from a dialect: the fidelity harness pins every field of it on both
// sides of a comparison, and a fact a segment could reach around this is a
// fact the harness could not pin.
func (t *Theme) context(info PromptInfo) *prompttheme.Context {
	host, _ := os.Hostname()
	return &prompttheme.Context{
		Continued: info.Continued,
		Dir:       info.Dir,
		PrevDir:   info.PrevDir,
		Home:      t.variable("HOME"),
		User:      t.variable("USER"),
		Host:      host,
		Status:    info.Status,
		Duration:  info.Duration,
		Jobs:      info.Jobs,
		Columns:   info.Columns,
		Root:      info.Root,
		Remote:    info.Remote,
		Now:       time.Now(),
		Var:       t.get,
	}
}

func (t *Theme) variable(name string) string {
	if t.get == nil {
		return ""
	}
	v, _ := t.get(name)
	return v
}
