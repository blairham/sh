// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package repl

import (
	"os"
	"time"

	"github.com/blairham/sh/internal/prompttheme"
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

	settings *prompttheme.Settings
	engine   *prompttheme.Engine
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
	// Through a closure rather than PromptChar(t.Char) directly, so that a
	// front end setting Char after this returns gets the character it set
	// rather than the one that was there when the roster was built.
	t.roster.Compile("prompt_char", prompttheme.SegmentFunc(
		func(settings *prompttheme.Settings, ctx *prompttheme.Context) (prompttheme.Rendered, bool) {
			return prompttheme.PromptChar(t.Char).Render(settings, ctx)
		}))
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
	if len(settings.List("LEFT_ELEMENTS")) == 0 && len(settings.List("RIGHT_ELEMENTS")) == 0 {
		// Nothing asked for a theme. Answering "not drawing" rather than
		// drawing a bare character is what keeps a wired theme from
		// overriding a prompt somebody set: the engine's own bare prompt is
		// for a configuration that names no elements, not for a session that
		// named no configuration.
		return ThemedPrompt{}, false
	}

	t.engine.Settings = settings
	t.engine.Bare = t.Char + " "
	t.engine.Continued = t.Continued
	drawn := t.engine.Render(t.context(info))
	return ThemedPrompt{Text: drawn.Text, Cont: drawn.Cont}, true
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
	return problems
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
