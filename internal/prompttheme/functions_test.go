// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package prompttheme_test

import (
	"strings"
	"testing"

	"github.com/blairham/sh/internal/prompttheme"
)

// session is a stand-in for the shell functions a person defined.
type session struct{ text map[string]string }

func (s session) has(name string) bool { _, ok := s.text[name]; return ok }

func (s session) call(name string) (string, bool) {
	text, ok := s.text[name]
	return text, ok
}

func rosterWith(t *testing.T, defined map[string]string) *prompttheme.Roster {
	t.Helper()
	r := prompttheme.NewRoster()
	r.Compile("dir", prompttheme.SegmentFunc(
		func(*prompttheme.Settings, *prompttheme.Context) (prompttheme.Rendered, bool) {
			return prompttheme.Rendered{Content: "BUILT-IN"}, true
		}))
	s := session{text: defined}
	r.Consult(prompttheme.Functions("a session function", s.has, s.call))
	return r
}

func draw(t *testing.T, r *prompttheme.Roster, element string) (prompttheme.Rendered, bool, bool) {
	t.Helper()
	segment, known := r.Resolve(element)
	if !known {
		return prompttheme.Rendered{}, false, false
	}
	out, shown := segment.Render(prompttheme.NewSettings(), &prompttheme.Context{})
	return out, shown, true
}

func TestAShellFunctionIsASegment(t *testing.T) {
	t.Parallel()
	// The everyday answer to "expandable without ever building the shell":
	// the common case is a few lines of shell over a file or a variable, and
	// that should cost a person nothing but their own rc.
	r := rosterWith(t, map[string]string{"my_thing": "hello\n"})
	out, shown, known := draw(t, r, "my_thing")
	if !known || !shown || out.Content != "hello" {
		t.Errorf("a session function drew %q (shown %v, known %v)", out.Content, shown, known)
	}
}

func TestAFunctionWithNothingToSayCostsNoSpace(t *testing.T) {
	t.Parallel()
	// What makes a conditional segment written in three lines of shell
	// affordable: it says nothing on the lines it has nothing for, and an
	// absent thing costs no space rather than an empty box.
	r := rosterWith(t, map[string]string{"quiet": "\n"})
	if _, shown, _ := draw(t, r, "quiet"); shown {
		t.Error("a function that printed a bare newline drew a segment")
	}
}

func TestANameThatIsNotAFunctionIsNotASegment(t *testing.T) {
	t.Parallel()
	// The resolver must decline, not claim the name: claiming it would put
	// every compiled-in segment behind a resolver that answers everything.
	r := rosterWith(t, map[string]string{"mine": "x"})
	out, _, _ := draw(t, r, "dir")
	if out.Content != "BUILT-IN" {
		t.Errorf("the compiled-in segment drew %q", out.Content)
	}
	if _, _, known := draw(t, r, "nothing_at_all"); known {
		t.Error("a name nobody defined resolved to a segment")
	}
}

func TestAFunctionWinsOverTheBuiltInAndTheCollisionIsNamed(t *testing.T) {
	t.Parallel()
	// The session wins because it is the most local thing and the person
	// writing it is present. But somebody whose segment stopped drawing
	// because a release added a built-in of the same name has been silently
	// overruled by their own shell, so the collision is said out loud.
	r := rosterWith(t, map[string]string{"dir": "MINE"})
	out, _, _ := draw(t, r, "dir")
	if out.Content != "MINE" {
		t.Errorf("the session's own function drew %q", out.Content)
	}
	shadowed := r.Shadowed()
	if len(shadowed) != 1 || !strings.Contains(shadowed[0], "dir") {
		t.Errorf("the collision was reported as %v", shadowed)
	}
	// And a name only the session answers is not a collision.
	rosterWith(t, map[string]string{"mine": "x"})
	if r2 := rosterWith(t, map[string]string{"mine": "x"}); func() bool {
		draw(t, r2, "mine")
		return len(r2.Shadowed()) != 0
	}() {
		t.Error("a name nothing else answers was reported as a collision")
	}
}
