// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package repl

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/blairham/sh/internal/boundary"
	"github.com/blairham/sh/internal/policy"
	"github.com/blairham/sh/internal/pty"
	"github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// Tab reads directories, and the path it reads is one a person typed.
//
// That is the whole of #951. The interpreter's own listing has been gated
// since ActionReadDir existed — `echo srv/*` under `deny read srv/**` finds
// nothing, because a denied directory reads as empty — and completion went to
// the os package, so the same policy answered the same question two different
// ways depending on which half of the shell was asked. A policy that hid a
// directory from the script did not hide it from the prompt.
//
// The fixture is built under t.TempDir() and the policies are parsed by
// internal/policy rather than written as Go functions, which is #944's finding
// carried forward: a selector that does not mean what its author thought
// matches nothing, and matching nothing reads exactly like a rule being obeyed.

// gatedFixture builds a directory with one subdirectory a policy hides and one
// it does not, each holding a file whose name says which it came from.
//
// The visible half is not decoration. A test that only asserts an empty answer
// passes just as well when completion is broken outright, and that is the
// mistake worth guarding against here more than anywhere: the whole finding is
// that a listing came back when it should not have.
func gatedFixture(t *testing.T) (dir string) {
	t.Helper()
	dir = t.TempDir()
	for _, d := range []string{"hidden", "shown"} {
		if err := os.Mkdir(filepath.Join(dir, d), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	for _, f := range []string{"hidden/secret.txt", "shown/visible.txt"} {
		if err := os.WriteFile(filepath.Join(dir, f), nil, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

// hidesReads parses a policy that refuses every read beneath the named
// directories and allows everything else.
func hidesReads(t *testing.T, dirs ...string) *policy.Policy {
	t.Helper()
	var b strings.Builder
	b.WriteString("version 1\ndefault allow\n")
	for _, d := range dirs {
		fmt.Fprintf(&b, "deny read %s/**\n", d)
	}
	p, err := policy.Parse(strings.NewReader(b.String()))
	if err != nil {
		t.Fatalf("policy: %v", err)
	}
	return p
}

// gated is a completer in a directory, under a policy.
func gated(dir string, g interp.Gate, events interp.Sink) shellCompleter {
	return shellCompleter{
		dir:   dir,
		bound: boundary.Boundary{Gate: g, Events: events, Session: "SESSIONUNDERTEST"},
		ctx:   context.Background(),
	}
}

// TestTabDoesNotListADirectoryThePolicyHides is the bug, as a test.
//
// Both halves are asserted from one policy and one completer, so a change that
// makes the first pass by breaking completion fails the second.
func TestTabDoesNotListADirectoryThePolicyHides(t *testing.T) {
	t.Parallel()
	dir := gatedFixture(t)
	s := gated(dir, hidesReads(t, filepath.Join(dir, "hidden")), nil)

	if got := s.paths("hidden/", nil); len(got) != 0 {
		t.Errorf("Tab offered %q from a directory the policy hides", got)
	}
	if got := s.paths("shown/", nil); len(got) != 1 || got[0] != "shown/visible.txt" {
		t.Errorf("Tab offered %q from an allowed directory, want [shown/visible.txt]", got)
	}
}

// TestTabAndAGlobAgreeAboutADeniedDirectory is the inconsistency the issue is
// named for, asserted as an equality rather than as two separate facts.
//
// The interpreter's listing and the completer's are now the same call to the
// same gate about the same action, so a policy that hides a directory hides it
// from both. Written against the two code paths rather than against two
// expected values, because what matters is that they cannot drift apart.
func TestTabAndAGlobAgreeAboutADeniedDirectory(t *testing.T) {
	t.Parallel()
	dir := gatedFixture(t)
	p := hidesReads(t, filepath.Join(dir, "hidden"))
	r := newTestRunner(nil)
	r.Dir, r.Gate = dir, p
	var out strings.Builder
	r.Stdout = &out

	// What the script sees.
	if err := runScript(t, r, "echo hidden/*; echo shown/*"); err != nil {
		t.Fatal(err)
	}
	glob := strings.Fields(out.String())
	// What the person at the prompt sees.
	s := gated(dir, p, nil)
	tab := append(s.paths("hidden/", nil), s.paths("shown/", nil)...)

	// The glob leaves an unmatched pattern standing, so the denied half is the
	// pattern itself and the allowed half is the file. What is being compared
	// is which of the two directories each route could enumerate.
	if len(glob) != 2 || glob[0] != "hidden/*" || glob[1] != "shown/visible.txt" {
		t.Fatalf("the glob produced %q, want the denied pattern unmatched and the allowed file", glob)
	}
	if len(tab) != 1 || tab[0] != "shown/visible.txt" {
		t.Fatalf("Tab produced %q, want only the allowed file", tab)
	}
}

// TestTabInTheCommandPositionSkipsADeniedDirectoryOnPATH.
//
// The second of the two listings, and it is not the same code: `commands`
// walks PATH and the entries it keeps are the executable ones. A PATH entry
// the policy hides contributes no names, exactly as one that is not on the
// disk contributes none.
func TestTabInTheCommandPositionSkipsADeniedDirectoryOnPATH(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	shown, hidden := filepath.Join(dir, "binshown"), filepath.Join(dir, "binhidden")
	for _, d := range []string{shown, hidden} {
		if err := os.Mkdir(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(shown, "zz-allowed"), nil, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(hidden, "zz-denied"), nil, 0o755); err != nil {
		t.Fatal(err)
	}
	s := gated(dir, hidesReads(t, hidden), nil)
	s.path = shown + string(os.PathListSeparator) + hidden

	got := s.commands("zz-")
	if len(got) != 1 || got[0] != "zz-allowed" {
		t.Errorf("the command position offered %q, want only [zz-allowed]", got)
	}
}

// TestTabThroughALinkIsCheckedOnWhatItReached.
//
// The name a person types is not the object the kernel gives back, and a rule
// matching names alone is defeated by one symbolic link. A person can make
// that link — nothing about a prompt stops them — so the listing is verified
// against what the open reached, exactly as every other open in this shell is.
func TestTabThroughALinkIsCheckedOnWhatItReached(t *testing.T) {
	t.Parallel()
	if runtime.GOOS != "darwin" && runtime.GOOS != "linux" {
		t.Skip("no way to ask the kernel what an open reached here")
	}
	dir := gatedFixture(t)
	if err := os.Symlink(filepath.Join(dir, "hidden"), filepath.Join(dir, "innocent")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(dir, "shown"), filepath.Join(dir, "ordinary")); err != nil {
		t.Fatal(err)
	}
	s := gated(dir, hidesReads(t, filepath.Join(dir, "hidden")), nil)

	if got := s.paths("innocent/", nil); len(got) != 0 {
		t.Errorf("Tab offered %q through a link into a directory the policy hides", got)
	}
	// And it is a refusal of what the policy refuses rather than of links.
	if got := s.paths("ordinary/", nil); len(got) != 1 || got[0] != "ordinary/visible.txt" {
		t.Errorf("Tab offered %q through an allowed link, want [ordinary/visible.txt]", got)
	}
}

// TestARefusedTabIsIndistinguishableFromADirectoryThatIsNotThere.
//
// #951 asked what a refusal should *show*, and this is the answer: nothing at
// all, which is what a person already sees for a path that does not exist. Tab
// offering no completions is the whole of it — no diagnostic, no beep of its
// own, nothing that would let somebody map a hidden tree by watching which
// prefixes refuse differently from which are absent.
//
// It is the shape ActionReadDir already documents and the shape a denied stat
// already has, and the reason it is quiet here is stronger than there: a
// completion list is read by a person rather than by a script, so a refusal
// that identified itself would be an oracle with a human running it.
func TestARefusedTabIsIndistinguishableFromADirectoryThatIsNotThere(t *testing.T) {
	t.Parallel()
	dir := gatedFixture(t)
	s := gated(dir, hidesReads(t, filepath.Join(dir, "hidden")), nil)

	// The comparison is what each prefix *did to the line*, not the two lines,
	// which differ because the words differ. A prefix that completes nothing
	// leaves the line exactly as it was typed.
	for _, typed := range []string{": hidden/", ": nowhere/"} {
		line, listed := typeAndTab(t, s, typed)
		if line != typed {
			t.Errorf("Tab on %q left the line %q; a refusal and an absence both leave it alone",
				typed, line)
		}
		if len(listed) != 0 {
			t.Errorf("Tab on %q listed %q", typed, listed)
		}
	}
	// And the same completer with no policy at all does complete the denied
	// name, so the equality above is the policy's doing and not a fixture that
	// never had anything in it.
	open := gated(dir, nil, nil)
	if line, _ := typeAndTab(t, open, ": hidden/"); line != ": hidden/secret.txt " {
		t.Errorf("with no policy the line became %q, want the file completed", line)
	}
}

// TestWhatOneTabPutsInTheEventStream is the per-keystroke question the issue
// raised, settled by counting.
//
// The listing is recorded and is neither suppressed nor coarsened, and the
// cost of that is a number rather than a worry: a Tab in the file position
// makes one record, and a Tab in the command position makes one per PATH
// entry — which is the same shape the shell already pays per *command*, since
// a PATH search stats a candidate in every directory PATH names.
//
// Completion also runs on Tab and on nothing else. The editor's completion
// arrives on a key, not on every character typed, so a long prefix costs
// nothing until it is asked to complete one.
func TestWhatOneTabPutsInTheEventStream(t *testing.T) {
	t.Parallel()
	dir := gatedFixture(t)
	var got []interp.Event
	sink := interp.SinkFunc(func(_ context.Context, e interp.Event) { got = append(got, e) })
	s := gated(dir, hidesReads(t, filepath.Join(dir, "hidden")), sink)

	s.paths("shown/", nil)
	if len(got) != 1 {
		t.Fatalf("one Tab in the file position recorded %d events, want 1: %+v", len(got), got)
	}
	if got[0].Kind != interp.EventAccess || got[0].Action.Kind != interp.ActionReadDir ||
		got[0].Action.Path != filepath.Join(dir, "shown") {
		t.Errorf("recorded %+v, want a read-dir access naming the directory listed", got[0])
	}
	if got[0].Session != "SESSIONUNDERTEST" {
		t.Errorf("the record says session %q, want the session's", got[0].Session)
	}

	// A refusal is recorded too, and as a denial. The stream is the operator's
	// half: a person exploring a directory the policy hides is precisely what
	// somebody reviewing a session wants to be able to see.
	got = nil
	s.paths("hidden/", nil)
	if len(got) != 1 || got[0].Kind != interp.EventDenied ||
		got[0].Action.Path != filepath.Join(dir, "hidden") {
		t.Fatalf("a refused Tab recorded %+v, want one denial naming the directory", got)
	}

	// And the command position is one per PATH entry, which is the count the
	// decision not to suppress was made against.
	got = nil
	s.path = strings.Join([]string{dir, filepath.Join(dir, "shown"), filepath.Join(dir, "hidden")},
		string(os.PathListSeparator))
	s.commands("zz-")
	if len(got) != 3 {
		t.Errorf("one Tab over a three-entry PATH recorded %d events, want 3: %+v", len(got), got)
	}
}

// TestAnUnpoliciedTabRecordsNothingAndCompletes, so a session nobody is
// watching pays two nil checks and makes the call it always made.
func TestAnUnpoliciedTabRecordsNothingAndCompletes(t *testing.T) {
	t.Parallel()
	dir := gatedFixture(t)
	s := gated(dir, nil, nil)
	if got := s.paths("hidden/", nil); len(got) != 1 || got[0] != "hidden/secret.txt" {
		t.Errorf("without a policy Tab offered %q, want the file", got)
	}
}

// runScript runs a snippet on a Runner, so a test can put the interpreter's
// answer beside the completer's.
func runScript(t *testing.T, r *interp.Runner, src string) error {
	t.Helper()
	return r.RunPart(t.Context(), syntax.NewParser(src+"\n", syntax.Core()).Parse())
}

// TestTabAtATerminalOffersNothingFromADirectoryThePolicyHides is the same
// claim made where a person would see it.
//
// Everything above asserts on what `paths` returns, and a fix that never
// reaches the session would satisfy all of it: the completer is built from the
// Shell, per session, and a boundary dropped on that path is invisible to a
// unit test that constructs the completer itself. So this drives a whole shell
// on a real terminal under a real policy and reads the completed word back
// through `printf`, which is the only way to see what the line *became*.
//
// The allowed directory in the same session is what stops it passing for the
// wrong reason: a session where Tab does nothing at all would satisfy the
// first line and fail the second.
func TestTabAtATerminalOffersNothingFromADirectoryThePolicyHides(t *testing.T) {
	control, tty, err := pty.Open()
	if err != nil {
		t.Skipf("no pseudo-terminal: %v", err)
	}
	defer func() {
		_ = tty.Close()
		_ = control.Close()
	}()

	dir := gatedFixture(t)
	p := hidesReads(t, filepath.Join(dir, "hidden"))
	out, errs := &syncBuffer{}, &syncBuffer{}
	r := newTestRunner(map[string]string{"PS1": "$ "})
	r.Stdout, r.Stderr, r.Gate = out, errs, p
	s := Shell{Runner: r, In: tty, Out: out, Err: errs, Name: "sh", Gate: p}

	done := make(chan error, 1)
	go func() {
		_, err := s.Run(t.Context())
		done <- err
	}()

	type step struct{ typed, want string }
	for _, st := range []step{
		{"cd '" + dir + "'\n", ""},
		// The policy hides this directory, so Tab has nothing to offer and the
		// word reaches printf exactly as it was typed.
		{"printf '<%s>' hidden/\t\n", "<hidden/>"},
		// And this one it does not, so the same keystroke completes the file.
		{"printf '<%s>' shown/\t\n", "<shown/visible.txt>"},
	} {
		waitFor(t, out, "$ ", "the prompt")
		if _, err := control.WriteString(st.typed); err != nil {
			t.Fatal(err)
		}
		if st.want != "" {
			waitFor(t, out, st.want, "the completed word run back through printf")
		}
	}
	// The name behind the policy must not be anywhere on the screen, which is
	// the assertion the two above cannot make: a listing printed by a second
	// Tab would not have changed the line either.
	if strings.Contains(out.String(), "secret.txt") {
		t.Errorf("the session drew the hidden name:\n%s", out.String())
	}

	waitFor(t, out, "$ ", "the last prompt")
	if _, err := control.WriteString("\x04"); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(20 * time.Second):
		t.Fatal("the shell did not exit on ^D")
	}
}

// TestTheSessionsGateAndSinkBothReachTheCompleter.
//
// The two are wired one line apart, and only one of them fails loudly. A
// dropped gate makes Tab list a directory the policy hides, which every test
// above catches. A dropped *sink* changes nothing a person or a script can
// see — completion still refuses exactly what it should refuse — and silently
// removes every completion from the audit stream, which is the half of this
// change somebody reviewing a session depends on. A mutation that dropped it
// survived the whole suite.
//
// So the claim is made from the Shell rather than from a hand-built completer,
// because the wiring is what was missing: this is the same path a session
// takes, minus the terminal.
func TestTheSessionsGateAndSinkBothReachTheCompleter(t *testing.T) {
	t.Parallel()
	dir := gatedFixture(t)
	var got []interp.Event
	r := newTestRunner(nil)
	r.Dir = dir
	s := Shell{
		Runner:  r,
		Gate:    hidesReads(t, filepath.Join(dir, "hidden")),
		Events:  interp.SinkFunc(func(_ context.Context, e interp.Event) { got = append(got, e) }),
		Session: "SESSIONUNDERTEST",
	}
	c := s.completer(t.Context())

	if line, _ := typeAndTab(t, c, ": shown/"); line != ": shown/visible.txt " {
		t.Fatalf("the session's completer made the line %q, want the file completed", line)
	}
	if len(got) != 1 || got[0].Kind != interp.EventAccess ||
		got[0].Action.Kind != interp.ActionReadDir ||
		got[0].Action.Path != filepath.Join(dir, "shown") ||
		got[0].Session != "SESSIONUNDERTEST" {
		t.Fatalf("the session recorded %+v, want one read-dir access naming the directory", got)
	}

	got = nil
	if line, _ := typeAndTab(t, c, ": hidden/"); line != ": hidden/" {
		t.Fatalf("the session's completer made the line %q from a hidden directory", line)
	}
	if len(got) != 1 || got[0].Kind != interp.EventDenied ||
		got[0].Action.Path != filepath.Join(dir, "hidden") ||
		got[0].Session != "SESSIONUNDERTEST" {
		t.Fatalf("the session recorded %+v, want one denial naming the directory", got)
	}
}
