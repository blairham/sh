// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package repl

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/blairham/sh/internal/prompttheme"
)

// A theme is the whole prompt. Neither the parameter nor a contribution
// appears beside it — a contribution in front of a frame is outside the
// frame, which is the shape the seam exists to avoid.
func TestAThemeIsTheWholePromptAndNotAPrefix(t *testing.T) {
	called := false
	s := Shell{
		Runner: newTestRunner(map[string]string{"PS1": "$ "}),
		PromptProviders: []PromptProvider{PromptProviderFunc(func(PromptInfo) string {
			called = true
			return "contributed"
		})},
		Theme: PromptThemeFunc(func(PromptInfo) (ThemedPrompt, bool) {
			return ThemedPrompt{Text: "theme> ", Cont: "…> "}, true
		}),
	}
	var pending strings.Builder
	got := s.beforeReading(t.Context(), nil, &pending)

	if got.text != "theme> " {
		t.Errorf("the prompt is %q, want %q", got.text, "theme> ")
	}
	if got.cells != len("theme> ") {
		t.Errorf("the prompt is %d cells, want %d", got.cells, len("theme> "))
	}
	if called {
		t.Error("a provider contributed to a prompt a theme was drawing")
	}
}

// And the parameter is not touched, so turning the theme off restores
// whatever the person had.
func TestAThemeNeitherReadsNorWritesTheParameter(t *testing.T) {
	runner := newTestRunner(map[string]string{"PS1": "mine> "})
	s := Shell{
		Runner: runner,
		Theme: PromptThemeFunc(func(PromptInfo) (ThemedPrompt, bool) {
			return ThemedPrompt{Text: "theme> "}, true
		}),
	}
	var pending strings.Builder
	s.beforeReading(t.Context(), nil, &pending)

	if v, _ := runner.GetVar("PS1"); v != "mine> " {
		t.Errorf("PS1 is now %q", v)
	}

	s.Theme = nil
	if got := s.beforeReading(t.Context(), nil, &pending); got.text != "mine> " {
		t.Errorf("with the theme off the prompt is %q, want the parameter back", got.text)
	}
}

// A theme that is not drawing is exactly a session with no theme wired.
func TestAThemeThatIsNotDrawingLeavesEverythingAlone(t *testing.T) {
	s := Shell{
		Runner:          newTestRunner(map[string]string{"PS1": "$ "}),
		PromptProviders: []PromptProvider{PromptProviderFunc(func(PromptInfo) string { return "seg" })},
		Theme:           PromptThemeFunc(func(PromptInfo) (ThemedPrompt, bool) { return ThemedPrompt{}, false }),
	}
	var pending strings.Builder
	if got := s.beforeReading(t.Context(), nil, &pending); got.text != "seg$ " {
		t.Errorf("the prompt is %q, want %q", got.text, "seg$ ")
	}
}

// The continuation prompt is the theme's too, and it is told which it is.
func TestTheContinuationPromptIsTheThemes(t *testing.T) {
	var told []PromptInfo
	s := Shell{
		Runner: newTestRunner(map[string]string{"PS2": "> "}),
		Theme: PromptThemeFunc(func(info PromptInfo) (ThemedPrompt, bool) {
			told = append(told, info)
			return ThemedPrompt{Text: "theme> ", Cont: "…> "}, true
		}),
	}
	var pending strings.Builder
	pending.WriteString("for i in a b\n")
	if got := s.beforeReading(t.Context(), nil, &pending); got.text != "…> " {
		t.Errorf("the continuation prompt is %q, want %q", got.text, "…> ")
	}
	if len(told) != 1 || !told[0].Continued {
		t.Errorf("the theme was told %+v, want one call with Continued set", told)
	}
}

// A theme that panics costs its prompt and not the session: the parameter is
// what appears, and the shell keeps reading.
func TestAThemeThatPanicsLeavesTheSessionItsPrompt(t *testing.T) {
	var reported strings.Builder
	s := Shell{
		Name:   "sh",
		Err:    &reported,
		Runner: newTestRunner(map[string]string{"PS1": "$ "}),
		Theme: PromptThemeFunc(func(PromptInfo) (ThemedPrompt, bool) {
			panic("a segment went wrong")
		}),
	}
	var pending strings.Builder
	got := s.beforeReading(t.Context(), nil, &pending)

	if got.text != "$ " {
		t.Errorf("the prompt is %q, want the parameter", got.text)
	}
	if !strings.Contains(reported.String(), "a segment went wrong") {
		t.Errorf("nothing was reported; saw %q", reported.String())
	}
}

// The engine reads the session's own variables, so a person can configure a
// prompt at the prompt.
func TestTheThemeReadsTheSessionsOwnVariables(t *testing.T) {
	runner := newTestRunner(map[string]string{
		"SH_PROMPT_LEFT_ELEMENTS": "prompt_char",
	})
	theme := NewTheme(runner.GetVar)

	drawn, drawing := theme.DrawPrompt(PromptInfo{})
	if !drawing {
		t.Fatal("a configuration naming an element did not draw")
	}
	if drawn.Text != "$ " {
		t.Errorf("the prompt is %q, want %q", drawn.Text, "$ ")
	}

	failed, _ := theme.DrawPrompt(PromptInfo{Status: 1})
	if failed.Text != "$ " {
		t.Errorf("after a failure the prompt is %q", failed.Text)
	}
	runner.SetVar("SH_PROMPT_PROMPT_CHAR_ERROR_SYMBOL", "!")
	failed, _ = theme.DrawPrompt(PromptInfo{Status: 1})
	if failed.Text != "! " {
		t.Errorf("a setting made at the prompt did not take: %q", failed.Text)
	}
}

// With nothing configured the theme is not drawing, so a wired theme cannot
// override a prompt somebody set.
func TestAThemeWithNoElementsConfiguredIsNotDrawing(t *testing.T) {
	theme := NewTheme(newTestRunner(map[string]string{"PS1": "mine> "}).GetVar)
	if _, drawing := theme.DrawPrompt(PromptInfo{}); drawing {
		t.Error("a theme nobody configured drew a prompt over one somebody set")
	}
}

// The file is asked for by name — named or nowhere — and re-read when it
// changes, so editing it takes effect on the next prompt.
func TestTheConfigurationFileIsNamedAndReRead(t *testing.T) {
	path := filepath.Join(t.TempDir(), "prompt.conf")
	writeThemeFile(t, path, "LEFT_ELEMENTS = prompt_char\nPROMPT_CHAR_SYMBOL = ›\n", time.Now().Add(-time.Minute))

	runner := newTestRunner(map[string]string{"SH_PROMPT_CONFIG": path})
	theme := NewTheme(runner.GetVar)

	drawn, drawing := theme.DrawPrompt(PromptInfo{})
	if !drawing || drawn.Text != "› " {
		t.Fatalf("the file's prompt is %q (drawing=%v)", drawn.Text, drawing)
	}

	writeThemeFile(t, path, "LEFT_ELEMENTS = prompt_char\nPROMPT_CHAR_SYMBOL = »\n", time.Now())
	drawn, _ = theme.DrawPrompt(PromptInfo{})
	if drawn.Text != "» " {
		t.Errorf("an edited file did not take effect: %q", drawn.Text)
	}

	// The session wins over the file, which is the layer order.
	runner.SetVar("SH_PROMPT_PROMPT_CHAR_SYMBOL", "$")
	drawn, _ = theme.DrawPrompt(PromptInfo{})
	if drawn.Text != "$ " {
		t.Errorf("the session did not win over the file: %q", drawn.Text)
	}
}

// A file that was named and is not there is a problem, because somebody
// named it — and an unnamed one is not, because nobody did.
func TestANamedFileThatIsNotThereIsAProblem(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "absent.conf")
	runner := newTestRunner(map[string]string{
		"SH_PROMPT_CONFIG":        missing,
		"SH_PROMPT_LEFT_ELEMENTS": "prompt_char",
	})
	theme := NewTheme(runner.GetVar)
	theme.DrawPrompt(PromptInfo{})

	problems := theme.Problems()
	if len(problems) != 1 || !strings.Contains(problems[0], missing) {
		t.Errorf("Problems = %#v", problems)
	}

	quiet := NewTheme(newTestRunner(map[string]string{"SH_PROMPT_LEFT_ELEMENTS": "prompt_char"}).GetVar)
	quiet.DrawPrompt(PromptInfo{})
	if got := quiet.Problems(); len(got) != 0 {
		t.Errorf("a session that named no file reported %#v", got)
	}
}

// An element nothing draws is named rather than silently absent.
func TestAnElementNothingDrawsIsNamed(t *testing.T) {
	theme := NewTheme(newTestRunner(map[string]string{
		"SH_PROMPT_LEFT_ELEMENTS": "kubernetes prompt_char",
	}).GetVar)
	theme.DrawPrompt(PromptInfo{})

	problems := theme.Problems()
	if len(problems) != 1 || !strings.Contains(problems[0], "kubernetes") {
		t.Errorf("Problems = %#v", problems)
	}
}

// The context carries the facts a segment is allowed to know, including the
// four the spec named as gaps.
func TestTheContextCarriesTheFactsASegmentIsAllowedToKnow(t *testing.T) {
	runner := newTestRunner(map[string]string{
		"SH_PROMPT_LEFT_ELEMENTS": "spy",
		"HOME":                    "/home/someone",
		"USER":                    "someone",
	})
	theme := NewTheme(runner.GetVar)
	var seen *prompttheme.Context
	theme.roster.Compile("spy", prompttheme.SegmentFunc(
		func(_ *prompttheme.Settings, ctx *prompttheme.Context) (prompttheme.Rendered, bool) {
			seen = ctx
			return prompttheme.Rendered{Content: "x"}, true
		}))

	theme.DrawPrompt(PromptInfo{
		Dir: "/src/sh", PrevDir: "/src", Status: 3, Jobs: 2,
		Duration: 5 * time.Second, Columns: 80, Root: true, Remote: true,
	})

	if seen == nil {
		t.Fatal("the segment was never called")
	}
	for _, c := range []struct {
		name      string
		got, want any
	}{
		{"Dir", seen.Dir, "/src/sh"},
		{"PrevDir", seen.PrevDir, "/src"},
		{"Home", seen.Home, "/home/someone"},
		{"User", seen.User, "someone"},
		{"Status", seen.Status, 3},
		{"Jobs", seen.Jobs, 2},
		{"Duration", seen.Duration, 5 * time.Second},
		{"Columns", seen.Columns, 80},
		{"Root", seen.Root, true},
		{"Remote", seen.Remote, true},
	} {
		if c.got != c.want {
			t.Errorf("%s = %v, want %v", c.name, c.got, c.want)
		}
	}
	if seen.Variable("USER") != "someone" {
		t.Errorf("the variable lookup answered %q", seen.Variable("USER"))
	}
}

// The previous prompt's directory is the previous one's, and there is not one
// at the first prompt of a session.
func TestThePreviousDirectoryIsThePreviousPrompts(t *testing.T) {
	// The Runner's directory and not the process's, and not $PWD: `cd` moves
	// the Runner's, and an earlier draft of this test set $PWD instead — so
	// both prompts reported the empty string, the assertion compared it with
	// itself, and removing the line that records the directory changed
	// nothing. The values below are asserted outright for that reason.
	runner := newTestRunner(map[string]string{"PS1": "$ "})
	runner.Dir = "/one"
	var told []PromptInfo
	s := Shell{
		Runner: runner,
		PromptProviders: []PromptProvider{PromptProviderFunc(func(info PromptInfo) string {
			told = append(told, info)
			return ""
		})},
		counts: &counts{},
	}
	var pending strings.Builder
	s.beforeReading(t.Context(), nil, &pending)
	runner.Dir = "/two"
	s.beforeReading(t.Context(), nil, &pending)

	if len(told) != 2 {
		t.Fatalf("the provider was called %d times", len(told))
	}
	for _, c := range []struct {
		what string
		got  string
		want string
	}{
		{"the first prompt's directory", told[0].Dir, "/one"},
		{"the first prompt's previous directory", told[0].PrevDir, ""},
		{"the second prompt's directory", told[1].Dir, "/two"},
		{"the second prompt's previous directory", told[1].PrevDir, "/one"},
	} {
		if c.got != c.want {
			t.Errorf("%s is %q, want %q", c.what, c.got, c.want)
		}
	}
}

// A session that is not remote says so, rather than guessing from the
// terminal's name.
func TestRemoteIsReadFromTheSessionsOwnVariables(t *testing.T) {
	local := Shell{Runner: newTestRunner(map[string]string{})}
	if local.remote() {
		t.Error("a local session reported itself remote")
	}
	over := Shell{Runner: newTestRunner(map[string]string{"SSH_CONNECTION": "10.0.0.1 51000 10.0.0.2 22"})}
	if !over.remote() {
		t.Error("a session over ssh reported itself local")
	}
	empty := Shell{Runner: newTestRunner(map[string]string{"SSH_CONNECTION": ""})}
	if empty.remote() {
		t.Error("an empty SSH_CONNECTION was read as a connection")
	}
}

func writeThemeFile(t *testing.T, path, text string, mtime time.Time) {
	t.Helper()
	if err := os.WriteFile(path, []byte(text), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(path, mtime, mtime); err != nil {
		t.Fatal(err)
	}
}

// Emptying the elements and never naming them are different answers, which is
// the rule the whole namespace is built on. Emptied is a themed prompt with
// no segments; unset is no theme, and the person's own parameter.
func TestAnEmptiedElementsListIsAThemeAndAnAbsentOneIsNot(t *testing.T) {
	emptied := NewTheme(newTestRunner(map[string]string{"SH_PROMPT_LEFT_ELEMENTS": ""}).GetVar)
	drawn, drawing := emptied.DrawPrompt(PromptInfo{})
	if !drawing {
		t.Error("emptying the elements turned the theme off, where it should draw a bare prompt")
	}
	if drawn.Text != "$ " {
		t.Errorf("the emptied theme drew %q, want the bare prompt", drawn.Text)
	}

	absent := NewTheme(newTestRunner(map[string]string{}).GetVar)
	if _, drawing := absent.DrawPrompt(PromptInfo{}); drawing {
		t.Error("a session that named no elements at all drew a theme")
	}
}
