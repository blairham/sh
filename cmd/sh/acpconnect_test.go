// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bufio"
	"context"
	"errors"
	"net"
	"os"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/blairham/sh/driver"
	"github.com/blairham/sh/internal/acp"
	"github.com/blairham/sh/internal/pty"
)

// Terminal authentication is claimed only where it can be honored, and here
// that means a terminal on both streams. A shell whose prompts arrive on a
// pipe cannot show anybody a login, and an agent told otherwise offers one
// that draws nothing.
func TestTerminalAuthNeedsATerminalOnBothStreams(t *testing.T) {
	control, term, err := pty.Open()
	if err != nil {
		if errors.Is(err, pty.ErrUnsupported) {
			t.Skip("no pseudo-terminal on this platform")
		}
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = control.Close(); _ = term.Close() })
	// The negative half needs something that is definitely not a terminal, and
	// the null device is the one every platform here has.
	pipe, err := os.Open(os.DevNull)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = pipe.Close() })

	for _, tc := range []struct {
		name     string
		in, out  *os.File
		wantHook bool
	}{
		{"both a terminal", term, term, true},
		{"input is not", pipe, term, false},
		{"output is not", term, pipe, false},
		{"neither", pipe, pipe, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			hook := terminalAuth([]string{"npx", "pkg"}, tc.in, tc.out)
			if (hook != nil) != tc.wantHook {
				t.Errorf("relaunch hook present = %v, want %v", hook != nil, tc.wantHook)
			}
		})
	}
}

// The login is the agent's own invocation with the method's arguments after
// it, not instead of it: the method describes what to *add*, and an agent
// relaunched without its own flags is a different program.
func TestALoginRepeatsTheAgentsOwnInvocation(t *testing.T) {
	t.Setenv("SH_ACP_TEST_VAR", "inherited")
	login := loginCommand(t.Context(),
		[]string{"npx", "pkg", "--acp"},
		[]string{"/login", "--force"},
		map[string]string{"SH_ACP_TEST_VAR": "overridden"})

	want := []string{"npx", "pkg", "--acp", "/login", "--force"}
	if !slices.Equal(login.Args, want) {
		t.Errorf("argv = %v, want %v", login.Args, want)
	}
	// The environment is inherited and then written over, and os/exec keeps
	// the last of a repeated name — which is what "these values override
	// same-named variables" asks for. Read from the end for that reason.
	got := ""
	for _, kv := range login.Env {
		if v, ok := strings.CutPrefix(kv, "SH_ACP_TEST_VAR="); ok {
			got = v
		}
	}
	if got != "overridden" {
		t.Errorf("SH_ACP_TEST_VAR = %q, want the method's value to win", got)
	}
}

// The method a person named has to reach the client, and a name the agent
// never advertised has to end the run rather than be dropped on the way. This
// shell's own agent side advertises none, which makes it exactly the agent
// that must refuse.
func TestTalkCarriesTheNamedAuthMethod(t *testing.T) {
	a, b := net.Pipe()
	agent := acp.NewAgent(driver.Shell{Name: "sh"}, acp.Implementation{Name: "sh", Version: "test"})
	ctx, cancel := context.WithCancel(t.Context())
	var wg sync.WaitGroup
	wg.Add(2)
	go func() { defer wg.Done(); _ = agent.Serve(ctx, a, a) }()
	client := &acp.Client{Info: acp.Implementation{Name: "sh", Version: "test"}}
	client.Connect(b, b)
	go func() { defer wg.Done(); _ = client.Serve(ctx) }()
	t.Cleanup(func() {
		agent.Close(context.Background())
		cancel()
		_ = a.Close()
		_ = b.Close()
		wg.Wait()
	})

	if code := talk(ctx, client, "no-such-method", bufio.NewScanner(strings.NewReader(""))); code == 0 {
		t.Error("a method the agent never advertised was reported as a success")
	}
}

// The three arrangements for who settles a permission request, and the one
// that has to be the default. Everything that is not an explicit allow is a
// denial, so a run with nobody to ask refuses rather than proceeding.
func TestWhoAnswersAPermissionRequest(t *testing.T) {
	ask := acp.RequestPermissionRequest{
		ToolCall: acp.ToolCall{ToolCallID: "1", Title: "rm -rf /"},
		Options:  acp.PermissionOptions(),
	}
	for _, tc := range []struct {
		name    string
		allow   bool
		tty     bool
		typed   string
		wantOpt string
	}{
		{"-acp-allow answers without asking", true, false, "", acp.OptionAllowOnce},
		{"-acp-allow still does not ask at a terminal", true, true, "R", acp.OptionAllowOnce},
		{"no terminal and no flag refuses", false, false, "a", acp.OptionRejectOnce},
		{"a person allows once", false, true, "a", acp.OptionAllowOnce},
		{"a person allows always", false, true, "A", acp.OptionAllowAlways},
		{"a person rejects", false, true, "r", acp.OptionRejectOnce},
		{"a person rejects always", false, true, "R", acp.OptionRejectAlways},
		{"a person may type the option id", false, true, "allow-always", acp.OptionAllowAlways},
		{"an answer that is not an option refuses", false, true, "maybe", acp.OptionRejectOnce},
		{"an empty answer refuses", false, true, "", acp.OptionRejectOnce},
		{"whitespace is not an answer either", false, true, "   ", acp.OptionRejectOnce},
		{"the input ending refuses", false, true, "\x00none", acp.OptionRejectOnce},
	} {
		t.Run(tc.name, func(t *testing.T) {
			input := tc.typed + "\n"
			if tc.typed == "\x00none" {
				input = ""
			}
			answer := answerer(tc.allow, tc.tty, bufio.NewScanner(strings.NewReader(input)))
			got, err := answer(t.Context(), ask)
			if err != nil {
				t.Fatalf("answering: %v", err)
			}
			if got.OptionID != tc.wantOpt {
				t.Errorf("chose %q, want %q", got.OptionID, tc.wantOpt)
			}
			if got.Outcome != acp.OutcomeSelected {
				t.Errorf("outcome = %q, want a selection", got.Outcome)
			}
		})
	}
}

// The keystroke is derived from the option's *kind* and not from its id: the
// ids are the agent's to spell, and another agent spells them differently, so
// keying on them would make one letter mean different things per agent.
func TestAnAnswerIsKeyedToTheOptionKindNotItsId(t *testing.T) {
	// An agent whose ids are nothing like ours.
	offered := []acp.PermissionOption{
		{OptionID: "yes-just-this-once", Name: "Yes", Kind: acp.KindAllowOnce},
		{OptionID: "no-thanks", Name: "No", Kind: acp.KindRejectOnce},
	}
	answer := answerer(false, true, bufio.NewScanner(strings.NewReader("a\n")))
	got, err := answer(t.Context(), acp.RequestPermissionRequest{Options: offered})
	if err != nil {
		t.Fatalf("answering: %v", err)
	}
	if got.OptionID != "yes-just-this-once" {
		t.Errorf("chose %q, want the agent's own allow-once id", got.OptionID)
	}
}

// An option carrying no id cannot be chosen by somebody who pressed return.
//
// It is the "an option id we never offered is a denial" rule from the other
// end: an agent that sent a malformed option would otherwise have it picked by
// the answer that means "I did not answer".
func TestAnEmptyAnswerCannotSelectAnEmptyOptionId(t *testing.T) {
	answer := answerer(false, true, bufio.NewScanner(strings.NewReader("\n")))
	got, err := answer(t.Context(), acp.RequestPermissionRequest{
		Options: []acp.PermissionOption{
			{OptionID: "", Name: "Allow", Kind: acp.KindAllowOnce},
			{OptionID: "reject-once", Name: "Reject", Kind: acp.KindRejectOnce},
		},
	})
	if err != nil {
		t.Fatalf("answering: %v", err)
	}
	if got.OptionID != acp.OptionRejectOnce {
		t.Errorf("chose %q, want a refusal", got.OptionID)
	}
}

// An option the agent never offered cannot be chosen, however it is typed.
func TestAnAnswerCannotInventAnOption(t *testing.T) {
	answer := answerer(false, true, bufio.NewScanner(strings.NewReader("A\n")))
	got, err := answer(t.Context(), acp.RequestPermissionRequest{
		Options: []acp.PermissionOption{
			{OptionID: "allow-once", Name: "Allow", Kind: acp.KindAllowOnce},
		},
	})
	if err != nil {
		t.Fatalf("answering: %v", err)
	}
	if got.OptionID != acp.OptionRejectOnce {
		t.Errorf("chose %q for a kind that was never offered, want a refusal", got.OptionID)
	}
}

// A form the agent described, filled in one field at a time and read back as
// the type it declared — not as the text that was typed. An agent that asked
// for an integer and was handed "42" would have to parse the form it had just
// described.
func TestAFormFieldIsReadAsItsDeclaredType(t *testing.T) {
	for _, tc := range []struct {
		name  string
		prop  acp.ElicitationProperty
		typed string
		want  any
	}{
		{"a string", acp.ElicitationProperty{Type: acp.PropertyString}, "Blair", "Blair"},
		{"an integer", acp.ElicitationProperty{Type: acp.PropertyInteger}, "42", 42},
		{"a number", acp.ElicitationProperty{Type: acp.PropertyNumber}, "2.5", 2.5},
		{"a boolean, spelled long", acp.ElicitationProperty{Type: acp.PropertyBoolean}, "yes", true},
		{"a boolean, spelled short", acp.ElicitationProperty{Type: acp.PropertyBoolean}, "n", false},
		{
			"a single-select enum",
			acp.ElicitationProperty{Type: acp.PropertyString, Enum: []string{"blue", "red"}},
			"blue", "blue",
		},
		{
			"a type this client has not seen, taken as written",
			acp.ElicitationProperty{Type: "color-of-the-sky"},
			"blue", "blue",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fill := form(true, bufio.NewScanner(strings.NewReader(tc.typed+"\n")))
			if fill == nil {
				t.Fatal("no form hook at a terminal")
			}
			got, err := fill(t.Context(), acp.CreateElicitationRequest{
				Message: "one field",
				Mode:    acp.ElicitForm,
				RequestedSchema: &acp.ElicitationSchema{
					Type:       "object",
					Properties: map[string]acp.ElicitationProperty{"it": tc.prop},
				},
			})
			if err != nil {
				t.Fatalf("filling in: %v", err)
			}
			if got.Action != acp.ElicitAccept {
				t.Fatalf("action = %q, want an acceptance", got.Action)
			}
			if got.Content["it"] != tc.want {
				t.Errorf("value = %#v, want %#v", got.Content["it"], tc.want)
			}
		})
	}
}

// Every field of a form comes back, not only the first.
//
// All of them are strings on purpose: a schema's properties are a map, so the
// order they are asked in is Go's to choose, and a test that fed one answer
// per type would depend on that order.
func TestEveryFieldOfAFormComesBack(t *testing.T) {
	fill := form(true, bufio.NewScanner(strings.NewReader("one\ntwo\nthree\n")))
	got, err := fill(t.Context(), acp.CreateElicitationRequest{
		Mode: acp.ElicitForm,
		RequestedSchema: &acp.ElicitationSchema{
			Type: "object",
			Properties: map[string]acp.ElicitationProperty{
				"a": {Type: acp.PropertyString},
				"b": {Type: acp.PropertyString},
				"c": {Type: acp.PropertyString},
			},
		},
	})
	if err != nil {
		t.Fatalf("filling in: %v", err)
	}
	if len(got.Content) != 3 {
		t.Errorf("content = %v, want all three fields", got.Content)
	}
	// Whichever order they were asked in, the three answers are what came back.
	seen := map[string]bool{}
	for _, v := range got.Content {
		seen[v.(string)] = true
	}
	for _, want := range []string{"one", "two", "three"} {
		if !seen[want] {
			t.Errorf("content = %v, want it to carry %q", got.Content, want)
		}
	}
}

// An enum takes one of its own values and nothing else, and a value that is
// not one of them is asked for again rather than accepted.
func TestAnEnumTakesOnlyItsOwnValues(t *testing.T) {
	fill := form(true, bufio.NewScanner(strings.NewReader("purple\nblue\n")))
	got, err := fill(t.Context(), acp.CreateElicitationRequest{
		Mode: acp.ElicitForm,
		RequestedSchema: &acp.ElicitationSchema{
			Type: "object",
			Properties: map[string]acp.ElicitationProperty{
				"color": {Type: acp.PropertyString, Enum: []string{"blue", "red"}},
			},
		},
	})
	if err != nil {
		t.Fatalf("filling in: %v", err)
	}
	if got.Content["color"] != "blue" {
		t.Errorf("color = %#v, want the value that was in the enum", got.Content["color"])
	}
}

// A required field left empty declines the whole form: a form returned without
// what it required is not an answer to it.
func TestAFormMissingARequiredFieldIsDeclined(t *testing.T) {
	fill := form(true, bufio.NewScanner(strings.NewReader("\n")))
	got, err := fill(t.Context(), acp.CreateElicitationRequest{
		Mode: acp.ElicitForm,
		RequestedSchema: &acp.ElicitationSchema{
			Type:       "object",
			Properties: map[string]acp.ElicitationProperty{"name": {Type: acp.PropertyString}},
			Required:   []string{"name"},
		},
	})
	if err != nil {
		t.Fatalf("filling in: %v", err)
	}
	if got.Action != acp.ElicitDecline {
		t.Errorf("action = %q, want a decline", got.Action)
	}
}

// A value that will not read as its type is asked for again rather than
// refused: a mistyped number is a slip and not a decision.
func TestAMistypedValueIsAskedForAgain(t *testing.T) {
	fill := form(true, bufio.NewScanner(strings.NewReader("twelve\n12\n")))
	got, err := fill(t.Context(), acp.CreateElicitationRequest{
		Mode: acp.ElicitForm,
		RequestedSchema: &acp.ElicitationSchema{
			Type:       "object",
			Properties: map[string]acp.ElicitationProperty{"count": {Type: acp.PropertyInteger}},
		},
	})
	if err != nil {
		t.Fatalf("filling in: %v", err)
	}
	if got.Content["count"] != 12 {
		t.Errorf("count = %#v, want 12 after the second try", got.Content["count"])
	}
}

// No terminal, no form hook — and it is the nil hook that withholds the
// capability, so an agent is never told to ask a question that goes nowhere.
func TestNoTerminalMeansNoForm(t *testing.T) {
	if form(false, bufio.NewScanner(strings.NewReader(""))) != nil {
		t.Error("a form hook was built with no terminal to draw it on")
	}
}

// What a person is told about the half of the policy they did not get.
//
// The claim `-acp-connect` makes is that a shell's policy reaches a coding
// agent, and #786 measured what that leaves out: an agent that runs commands in
// its own process is one no gate can see, and two of the three published
// adapters do exactly that. The notice exists so a person does not have to read
// a trace to find out which half they have.
func TestTheCoverageNoticeSaysWhichHalfOfThePolicyApplied(t *testing.T) {
	cases := []struct {
		name             string
		asked, announced int
		want             string
	}{
		{"a turn with no commands in it says nothing", 0, 0, ""},
		{"everything it ran, it asked us to run", 3, 3, ""},
		{"more asked than reported is still nothing to warn about", 3, 1, ""},
		{"it ran everything itself", 0, 2, "asked this shell to run none"},
		{"it asked for some of them", 1, 4, "asked this shell to run 1"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := commandCoverage(tc.asked, tc.announced)
			if tc.want == "" {
				if got != "" {
					t.Errorf("said %q, want silence", got)
				}
				return
			}
			if !strings.Contains(got, tc.want) {
				t.Errorf("said %q, want it to contain %q", got, tc.want)
			}
			if !strings.Contains(got, "passes no gate") {
				t.Errorf("said %q, and it has to say what the gap is", got)
			}
			// And the sharp half of it, which the live measurement forced: a
			// command an agent runs itself is also how it reads and writes
			// files, so a notice naming only "commands" would leave a person
			// believing -deny still covered their files. It did not: measured,
			// `-deny write /**` did not stop a file being created, because the
			// agent ran `echo … > path` rather than asking us to write.
			if !strings.Contains(got, "reads and writes") {
				t.Errorf("said %q, and it has to say that files go the same way", got)
			}
		})
	}
}
