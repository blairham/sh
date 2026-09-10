// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bufio"
	"bytes"
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
//
// The refusal it falls back to is subject to the same rule, and this agent
// offered nothing to refuse with either: there is no id to send, so the answer
// is a cancellation. Sending our own reject-once here would be the same defect
// wearing a safer-looking outcome — an agent that never offered that id has no
// reason to read it as anything, and one of them reads an unknown id as no
// answer at all.
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
	if got.Outcome != acp.OutcomeCancelled || got.OptionID != "" {
		t.Errorf("answered %q/%q, want a bare cancellation", got.Outcome, got.OptionID)
	}
}

// The fixed answers -acp-allow and its absence are subject to the same rule as
// a typed one, and this is the test the old ones could not fail: every case in
// TestWhoAnswersAPermissionRequest offers acp.PermissionOptions(), which is
// *our* spelling, so an implementation that ignored the request entirely and
// returned the constant looked right. It was not right. Measured against
// @zed-industries/claude-code-acp 0.16.2, whose ids are the ones below, the
// constant allow-once was an id that agent had never heard of and it ran the
// turn as though the tool had been rejected — #1777. The ids here are that
// agent's, copied off the wire.
func TestAFixedAnswerUsesTheAgentsOwnOptionIds(t *testing.T) {
	offered := []acp.PermissionOption{
		{OptionID: "allow_always", Name: "Always Allow", Kind: acp.KindAllowAlways},
		{OptionID: "allow", Name: "Allow", Kind: acp.KindAllowOnce},
		{OptionID: "reject", Name: "Reject", Kind: acp.KindRejectOnce},
	}
	for _, tc := range []struct {
		name  string
		allow bool
		want  string
	}{
		{"-acp-allow", true, "allow"},
		{"no flag", false, "reject"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			answer := answerer(tc.allow, false, bufio.NewScanner(strings.NewReader("")))
			got, err := answer(t.Context(), acp.RequestPermissionRequest{Options: offered})
			if err != nil {
				t.Fatalf("answering: %v", err)
			}
			if got.Outcome != acp.OutcomeSelected || got.OptionID != tc.want {
				t.Errorf("answered %q/%q, want a selection of %q", got.Outcome, got.OptionID, tc.want)
			}
		})
	}
}

// An agent that offers no option of the kind a fixed answer wants gets a
// cancellation rather than an invention — and -acp-allow saying "allow" while
// the agent was sent nothing of the sort is how a session goes quiet, so the
// line on standard error says which happened.
func TestAFixedAnswerCancelsWhatTheAgentDidNotOffer(t *testing.T) {
	only := []acp.PermissionOption{{OptionID: "reject", Name: "Reject", Kind: acp.KindRejectOnce}}
	answer := answerer(true, false, bufio.NewScanner(strings.NewReader("")))
	got, err := answer(t.Context(), acp.RequestPermissionRequest{Options: only})
	if err != nil {
		t.Fatalf("answering: %v", err)
	}
	if got.Outcome != acp.OutcomeCancelled || got.OptionID != "" {
		t.Errorf("answered %q/%q, want a bare cancellation", got.Outcome, got.OptionID)
	}
	if said := answered(got, acp.KindAllowOnce); !strings.Contains(said, acp.OutcomeCancelled) {
		t.Errorf("reported %q, which does not say the allow never reached the agent", said)
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

// What a person sees when an agent is driven and will not open a session.
//
// This is the closing criterion for #493, as amended: all three agents are
// driven, and any that cannot complete a session **reports exactly why, with
// its advertised methods**. Gemini CLI is the agent that criterion exists for,
// and it reaches this path twice over — once with no method named, and once
// having accepted `gemini-api-key` and refused a session anyway.
//
// The message is a test rather than a shape somebody eyeballed once, because
// it is the whole of what the release ships for that agent.
func TestASessionRefusedForAuthenticationSaysExactlyWhy(t *testing.T) {
	// What Gemini CLI 0.58.0 actually advertises, in the order it sends it.
	gemini := []acp.AuthMethod{
		{ID: "oauth-personal", Name: "Log in with Google", Description: "Log in with your Google account"},
		{ID: "gemini-api-key", Name: "Gemini API key", Description: "Use an API key with Gemini Developer API"},
		{ID: "vertex-ai", Name: "Vertex AI", Description: "Use an API key with Vertex AI GenAI API"},
		{ID: "gateway", Name: "AI API Gateway", Description: "Use a custom AI API Gateway"},
	}
	// And the error it answers session/new with, which is the specific half:
	// a credential is missing, not a protocol we do not speak.
	refusal := errors.New("jsonrpc -32000: Gemini API key is missing or not configured.")

	t.Run("with no method named, the list is a menu", func(t *testing.T) {
		var out strings.Builder
		refusedASession(&out, "gemini-cli 0.58.0", "", gemini, refusal)
		got := out.String()
		for _, want := range []string{
			"gemini-cli 0.58.0",
			"needs authenticating first",
			"API key is missing or not configured",
			"oauth-personal", "gemini-api-key", "vertex-ai", "gateway",
			"choose one with -acp-auth oauth-personal",
		} {
			if !strings.Contains(got, want) {
				t.Errorf("the diagnostic does not contain %q:\n%s", want, got)
			}
		}
	})

	t.Run("with a method accepted, it is the credential and not the choice", func(t *testing.T) {
		var out strings.Builder
		refusedASession(&out, "gemini-cli 0.58.0", "gemini-api-key", gemini, refusal)
		got := out.String()
		// The false thing it used to say. A person who named a method and had
		// it accepted did authenticate, and being told to do it first is what
		// sends them round the same flag again.
		if strings.Contains(got, "needs authenticating first") {
			t.Errorf("it still says authentication was skipped after it was accepted:\n%s", got)
		}
		for _, want := range []string{
			"accepted -acp-auth gemini-api-key",
			"still refuses a session",
			"API key is missing or not configured",
			"the credential behind it",
			"<- the one -acp-auth named",
		} {
			if !strings.Contains(got, want) {
				t.Errorf("the diagnostic does not contain %q:\n%s", want, got)
			}
		}
		// And it must not hand back the method that was just used.
		if strings.Contains(got, "choose one with -acp-auth gemini-api-key") {
			t.Errorf("it suggests the method that was already settled:\n%s", got)
		}
		if !strings.Contains(got, "choose one with -acp-auth oauth-personal") {
			t.Errorf("it suggests nothing else to try:\n%s", got)
		}
	})

	t.Run("one method, already tried, leaves nothing to suggest", func(t *testing.T) {
		var out strings.Builder
		only := []acp.AuthMethod{{ID: "terminal-login", Type: acp.AuthTerminal, Name: "Log in"}}
		refusedASession(&out, "an-agent 1.0", "terminal-login", only, refusal)
		if got := out.String(); strings.Contains(got, "choose one with") {
			t.Errorf("it offers a choice where there is none:\n%s", got)
		}
	})

	t.Run("an agent that advertises none says so", func(t *testing.T) {
		var out strings.Builder
		refusedASession(&out, "an-agent 1.0", "", nil, refusal)
		if got := out.String(); !strings.Contains(got, "advertised no authentication methods") {
			t.Errorf("an empty list is reported as %q", got)
		}
	})
}

// The claim the whole client side rests on, and the reason an agent's command
// line is interpreted rather than exec'd: the gate sees the commands *inside*
// the line.
//
// It has to be tested at full depth rather than at the seam, because every
// piece of it can be right while the composition is wrong — a Shell copied
// without its Gate would run the line perfectly and see nothing. The probe is
// `/bin/echo` rather than `echo`, which is exactly the distinction that makes
// this a test: `echo` is a builtin, so a line containing one execs nothing and
// a gate that was never wired would look identical to one that was.
func TestTheInterpreterRunsUnderTheSessionsGate(t *testing.T) {
	var trace bytes.Buffer
	sh, closer, err := installSeams(driver.Shell{}, ownFlags{traceEvents: true}, &trace)
	if err != nil {
		t.Fatalf("installing the seams: %v", err)
	}
	if closer != nil {
		defer func() { _ = closer.Close() }()
	}

	var out bytes.Buffer
	code := interpreter(sh)(t.Context(), acp.TerminalCommand{
		Line: "/bin/echo inside-the-line", Env: os.Environ(),
	}, &out)

	if code != 0 {
		t.Errorf("status = %d, want the line to have run", code)
	}
	if got := out.String(); !strings.Contains(got, "inside-the-line") {
		t.Errorf("output = %q, want what the command wrote", got)
	}
	if got := trace.String(); !strings.Contains(got, "/bin/echo") {
		t.Errorf("the gate never saw the exec inside the line; trace = %q", got)
	}
}

// And a policy reaches it. This is the half that does not negotiate: the
// person's answer to the agent's permission request has already been given by
// the time a line is interpreted, and the policy still refuses.
//
// It is also the sharpest argument for interpreting rather than exec'ing. On
// the exec route a gate would be asked about one filename; here it is asked
// about every command the line runs, so a rule naming `/bin/echo` reaches an
// `/bin/echo` the agent buried in a pipeline.
func TestAPolicyRefusesACommandInsideAnAgentsLine(t *testing.T) {
	var trace bytes.Buffer
	sh, closer, err := installSeams(driver.Shell{},
		ownFlags{deny: []string{"exec:/bin/echo"}}, &trace)
	if err != nil {
		t.Fatalf("installing the seams: %v", err)
	}
	if closer != nil {
		defer func() { _ = closer.Close() }()
	}

	var out bytes.Buffer
	interpreter(sh)(t.Context(), acp.TerminalCommand{
		Line: "true | /bin/echo refused-inside-a-pipeline", Env: os.Environ(),
	}, &out)

	if got := out.String(); strings.Contains(got, "refused-inside-a-pipeline") {
		t.Errorf("the denied command ran anyway: %q", got)
	}
	if got := out.String(); !strings.Contains(got, "refused") {
		t.Errorf("output = %q, want the refusal said out loud", got)
	}
}
