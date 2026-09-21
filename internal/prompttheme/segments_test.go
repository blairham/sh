// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package prompttheme_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/blairham/sh/internal/prompttheme"
)

// render drives one compiled-in segment the way the layout does, through the
// same Segment interface a segment from outside the binary is reached
// through. That is the constraint the roster is built on — a compiled-in
// segment may use no capability an outside one cannot — and driving them
// differently in a test is how it would stop being true without anything
// failing.
func render(t *testing.T, element string, settings *prompttheme.Store, ctx *prompttheme.Context) (prompttheme.Rendered, bool) {
	t.Helper()
	segment, ok := prompttheme.CoreSegments()[element]
	if !ok {
		t.Fatalf("no compiled-in segment named %q", element)
	}
	layers := []prompttheme.Layer{}
	if settings != nil {
		layers = append(layers, settings)
	}
	return segment.Render(prompttheme.NewSettings(layers...), ctx)
}

func TestTheDirectorySegmentCollapsesTheHomeDirectory(t *testing.T) {
	t.Parallel()
	out, drawn := render(t, "dir", nil, &prompttheme.Context{Dir: "/home/p/work", Home: "/home/p"})
	if !drawn || out.Content != "~/work" {
		t.Errorf("dir drew %q (drawn %v), want ~/work", out.Content, drawn)
	}

	at, _ := render(t, "dir", nil, &prompttheme.Context{Dir: "/home/p", Home: "/home/p"})
	if at.Content != "~" {
		t.Errorf("the home directory itself drew %q, want ~", at.Content)
	}

	// A directory that merely starts with the same letters is not inside it.
	// `/home/pat` under a home of `/home/p` is the case, and it is why the
	// prefix test carries the separator.
	beside, _ := render(t, "dir", nil, &prompttheme.Context{Dir: "/home/pat", Home: "/home/p"})
	if beside.Content != "/home/pat" {
		t.Errorf("a sibling of home drew %q, want it whole", beside.Content)
	}
}

func TestTheDirectorySegmentOffersItsPiecesToTheTemplate(t *testing.T) {
	t.Parallel()
	// The content template is the configuration's, so a segment that computed
	// more than one thing has to offer all of them or the template can only
	// draw the arrangement the segment chose.
	out, _ := render(t, "dir", nil, &prompttheme.Context{Dir: "/home/p/work", Home: "/home/p"})
	if out.Fields["FULL"] != "/home/p/work" || out.Fields["LAST"] != "work" {
		t.Errorf("dir offered %v", out.Fields)
	}
}

func TestTruncationIsOffUntilAConfigurationAsksForIt(t *testing.T) {
	t.Parallel()
	deep := &prompttheme.Context{Dir: "/a/b/c/d/e"}
	if out, _ := render(t, "dir", nil, deep); out.Content != "/a/b/c/d/e" {
		t.Errorf("an unconfigured prompt shortened a path to %q", out.Content)
	}

	short, _ := render(t, "dir", assignments(t, "preset", "DIR_MAX_DEPTH", "2"), deep)
	if short.Content != "…/d/e" {
		t.Errorf("DIR_MAX_DEPTH=2 drew %q", short.Content)
	}

	marked, _ := render(t, "dir", assignments(t, "preset",
		"DIR_MAX_DEPTH", "2", "DIR_TRUNCATION", "..."), deep)
	if marked.Content != ".../d/e" {
		t.Errorf("a configured truncation marker drew %q", marked.Content)
	}
}

func TestADirectoryThatCannotBeWrittenSaysSoAsAState(t *testing.T) {
	t.Parallel()
	// The state is the middle step of the lookup chain, which is what lets
	// DIR_NOT_WRITABLE_FOREGROUND exist without the segment knowing about
	// colors at all.
	dir := t.TempDir()
	locked := filepath.Join(dir, "locked")
	if err := os.Mkdir(locked, 0o500); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(locked, 0o700) })
	if os.Geteuid() == 0 {
		t.Skip("the superuser writes into every directory, so there is no state to read")
	}

	out, _ := render(t, "dir", nil, &prompttheme.Context{Dir: locked})
	if out.State != "not_writable" {
		t.Errorf("an unwritable directory drew state %q", out.State)
	}
	open, _ := render(t, "dir", nil, &prompttheme.Context{Dir: dir})
	if open.State != "" {
		t.Errorf("a writable directory drew state %q", open.State)
	}
}

func TestTheStatusSegmentIsQuietAfterASuccessAndNeverAfterAFailure(t *testing.T) {
	t.Parallel()
	if _, drawn := render(t, "status", nil, &prompttheme.Context{}); drawn {
		t.Error("the status segment drew on an ordinary success")
	}
	out, drawn := render(t, "status", nil, &prompttheme.Context{Status: 127})
	if !drawn || out.Content != "127" || out.State != "error" {
		t.Errorf("a failure drew %q in state %q (drawn %v)", out.Content, out.State, drawn)
	}
	asked, drawn := render(t, "status", assignments(t, "preset", "STATUS_OK", "true"), &prompttheme.Context{})
	if !drawn || asked.State != "ok" {
		t.Errorf("STATUS_OK=true drew state %q (drawn %v)", asked.State, drawn)
	}
}

func TestADurationIsDrawnOnlyOnceItIsWorthSaying(t *testing.T) {
	t.Parallel()
	quick := &prompttheme.Context{Duration: 900 * time.Millisecond}
	if _, drawn := render(t, "command_execution_time", nil, quick); drawn {
		t.Error("a command under the threshold drew a duration")
	}
	slow := &prompttheme.Context{Duration: 4 * time.Second}
	out, drawn := render(t, "command_execution_time", nil, slow)
	if !drawn || out.Content != "4s" {
		t.Errorf("four seconds drew %q (drawn %v)", out.Content, drawn)
	}
	lowered, drawn := render(t, "command_execution_time",
		assignments(t, "preset",
			"COMMAND_EXECUTION_TIME_THRESHOLD", "0",
			"COMMAND_EXECUTION_TIME_PRECISION", "1"), quick)
	if !drawn || lowered.Content != "0.9s" {
		t.Errorf("a threshold of zero drew %q (drawn %v)", lowered.Content, drawn)
	}
}

func TestADurationDropsTheUnitsThatAreZeroAndKeepsTheOnesBelow(t *testing.T) {
	t.Parallel()
	for _, row := range []struct {
		d    time.Duration
		want string
	}{
		{5 * time.Second, "5s"},
		{65 * time.Second, "1m 5s"},
		{3665 * time.Second, "1h 1m 5s"},
		{90065 * time.Second, "1d 1h 1m 5s"},
	} {
		out, _ := render(t, "command_execution_time", nil, &prompttheme.Context{Duration: row.d})
		if out.Content != row.want {
			t.Errorf("%v drew %q, want %q", row.d, out.Content, row.want)
		}
	}
}

func TestTheJobsSegmentDeclinesWhenThereAreNone(t *testing.T) {
	t.Parallel()
	if _, drawn := render(t, "background_jobs", nil, &prompttheme.Context{}); drawn {
		t.Error("a shell with no jobs drew a job count")
	}
	out, drawn := render(t, "background_jobs", nil, &prompttheme.Context{Jobs: 3})
	if !drawn || out.Content != "3" {
		t.Errorf("three jobs drew %q (drawn %v)", out.Content, drawn)
	}
	always, drawn := render(t, "background_jobs",
		assignments(t, "preset", "BACKGROUND_JOBS_ALWAYS", "yes"), &prompttheme.Context{})
	if !drawn || always.Content != "0" {
		t.Errorf("BACKGROUND_JOBS_ALWAYS drew %q (drawn %v)", always.Content, drawn)
	}
}

func TestTheContextSegmentDrawsWhereNotKnowingIsExpensive(t *testing.T) {
	t.Parallel()
	local := &prompttheme.Context{User: "p", Host: "here"}
	if _, drawn := render(t, "context", nil, local); drawn {
		t.Error("an ordinary local session drew a context segment")
	}
	for _, row := range []struct {
		ctx   *prompttheme.Context
		state string
	}{
		{&prompttheme.Context{User: "p", Host: "here", Root: true}, "root"},
		{&prompttheme.Context{User: "p", Host: "here", Remote: true}, "remote"},
	} {
		out, drawn := render(t, "context", nil, row.ctx)
		if !drawn || out.State != row.state || out.Content != "p@here" {
			t.Errorf("drew %q in state %q (drawn %v), want p@here in %q",
				out.Content, out.State, drawn, row.state)
		}
	}
	asked, drawn := render(t, "context", assignments(t, "preset", "CONTEXT_ALWAYS", "1"), local)
	if !drawn || asked.Content != "p@here" {
		t.Errorf("CONTEXT_ALWAYS drew %q (drawn %v)", asked.Content, drawn)
	}
}

func TestTheClockTakesItsFormatLanguageFromTheCaller(t *testing.T) {
	t.Parallel()
	// One reader of a format language. The engine takes the formatter rather
	// than owning a second copy, so what this checks is that the setting
	// reaches it and that the render clock is what is formatted.
	var got string
	clock := prompttheme.Clock(func(layout string, at time.Time) string {
		got = layout
		return at.UTC().Format("15:04")
	})
	now := time.Date(2026, 9, 20, 13, 45, 0, 0, time.UTC)
	settings := prompttheme.NewSettings(assignments(t, "preset", "TIME_FORMAT", "%H:%M"))
	out, drawn := clock.Render(settings, &prompttheme.Context{Now: now})
	if !drawn || got != "%H:%M" || out.Content != "13:45" {
		t.Errorf("the clock was asked for %q and drew %q (drawn %v)", got, out.Content, drawn)
	}
}

func TestEverySegmentNamesAnIconRatherThanDrawingOne(t *testing.T) {
	t.Parallel()
	// A segment that hardcoded its glyph would make an icon table a lie and
	// would leave a global icon override with nothing to default to. So no
	// segment's content may carry a glyph from a table.
	ctx := &prompttheme.Context{Dir: "/tmp", Status: 1, Duration: time.Minute, Jobs: 2, Root: true}
	set := prompttheme.LoadIcons("nerdfont")
	for element := range prompttheme.CoreSegments() {
		out, drawn := render(t, element, assignments(t, "preset", "STATUS_OK", "true"), ctx)
		if !drawn {
			continue
		}
		if out.Icon == "" {
			t.Errorf("%s named no icon", element)
			continue
		}
		if glyph, ok := set.Glyph(out.Icon); ok && glyph != "" && strings.Contains(out.Content, glyph) {
			t.Errorf("%s drew its own glyph instead of naming one", element)
		}
	}
}
