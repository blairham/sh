// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package prompttheme_test

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/blairham/sh/internal/prompttheme"
)

func TestTheFileIsTheNamespaceWrittenDown(t *testing.T) {
	const text = `# the look I use everywhere

LEFT_ELEMENTS = dir vcs newline prompt_char
SH_PROMPT_DIR_FOREGROUND = 31
TRANSIENT = always
SEPARATOR = " "
DIR_PREFIX =
`
	store, err := prompttheme.ParseFile("file", strings.NewReader(text))
	if err != nil {
		t.Fatalf("ParseFile: %v", err)
	}

	want := []string{"DIR_FOREGROUND", "DIR_PREFIX", "LEFT_ELEMENTS", "SEPARATOR", "TRANSIENT"}
	if got := store.Keys(); !slices.Equal(got, want) {
		t.Errorf("Keys = %#v, want %#v", got, want)
	}
	if got := store.Problems(); len(got) != 0 {
		t.Errorf("a file with nothing wrong in it reported %#v", got)
	}

	settings := prompttheme.NewSettings(store)
	if got := settings.List("LEFT_ELEMENTS"); !slices.Equal(got, []string{"dir", "vcs", "newline", "prompt_char"}) {
		t.Errorf("LEFT_ELEMENTS = %#v", got)
	}
	if got := settings.Str("DIR_FOREGROUND", ""); got != "31" {
		t.Errorf("the SH_PROMPT_ prefix was not stripped: %q", got)
	}
	if got := settings.Str("SEPARATOR", ""); got != " " {
		t.Errorf("a quoted space read as %q", got)
	}
	if !settings.Has("DIR_PREFIX") || settings.Str("DIR_PREFIX", "x") != "" {
		t.Error("a setting emptied on purpose did not survive the file")
	}
}

func TestALineThatIsNotASettingIsNamedRatherThanDropped(t *testing.T) {
	const text = `DIR_FOREGROUND = 31
this line is prose
a key with spaces = 1
`
	store, err := prompttheme.ParseFile("file:/tmp/prompt.conf", strings.NewReader(text))
	if err != nil {
		t.Fatalf("ParseFile: %v", err)
	}

	if got := store.Keys(); !slices.Equal(got, []string{"DIR_FOREGROUND"}) {
		t.Errorf("Keys = %#v", got)
	}
	problems := store.Problems()
	if len(problems) != 2 {
		t.Fatalf("Problems = %#v, want two", problems)
	}
	if !strings.Contains(problems[0], "file:/tmp/prompt.conf:2") {
		t.Errorf("a problem did not say where it was: %q", problems[0])
	}
	if !strings.Contains(problems[1], "not a setting name") {
		t.Errorf("a problem did not say what was wrong: %q", problems[1])
	}
}

func TestAnEditTakesEffectWithoutACommand(t *testing.T) {
	path := filepath.Join(t.TempDir(), "prompt.conf")
	write(t, path, "DIR_FOREGROUND = 31\n", time.Now().Add(-time.Minute))

	file := prompttheme.OpenFile(path)
	if !file.Refresh() {
		t.Fatal("the first Refresh read nothing")
	}
	settings := prompttheme.NewSettings(file)
	if got := settings.Str("DIR_FOREGROUND", ""); got != "31" {
		t.Fatalf("DIR_FOREGROUND = %q", got)
	}
	if file.Refresh() {
		t.Error("a file nobody touched was read again")
	}

	write(t, path, "DIR_FOREGROUND = blue\n", time.Now())
	if !file.Refresh() {
		t.Fatal("an edited file was not re-read")
	}
	if got := settings.Str("DIR_FOREGROUND", ""); got != "blue" {
		t.Errorf("DIR_FOREGROUND = %q, want blue", got)
	}
	if layer, _ := settings.Origin("DIR_FOREGROUND"); layer != "file:"+path {
		t.Errorf("Origin = %q", layer)
	}
}

func TestAFileSomebodyNamedAndIsNotThereIsAProblem(t *testing.T) {
	// Named or nowhere: an unset SH_PROMPT_CONFIG is a person who has not
	// asked for a file, and never reaches this. A name that does not resolve
	// is somebody's typo, and saying so is the whole difference.
	path := filepath.Join(t.TempDir(), "absent.conf")
	file := prompttheme.OpenFile(path)
	file.Refresh()

	if got := file.Problems(); len(got) != 1 || !strings.Contains(got[0], path) {
		t.Errorf("Problems = %#v", got)
	}
	if got := file.Keys(); len(got) != 0 {
		t.Errorf("an absent file held %#v", got)
	}
}

func TestAFileThatGoesAwayTakesItsSettingsWithIt(t *testing.T) {
	path := filepath.Join(t.TempDir(), "prompt.conf")
	write(t, path, "DIR_FOREGROUND = 31\n", time.Now().Add(-time.Minute))

	file := prompttheme.OpenFile(path)
	file.Refresh()
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if !file.Refresh() {
		t.Error("a file that was deleted reported no change")
	}
	if prompttheme.NewSettings(file).Has("DIR_FOREGROUND") {
		t.Error("a deleted file kept answering")
	}
}

func write(t *testing.T, path, text string, mtime time.Time) {
	t.Helper()
	if err := os.WriteFile(path, []byte(text), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(path, mtime, mtime); err != nil {
		t.Fatal(err)
	}
}
