// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package acp_test

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/blairham/sh/internal/acp"
	"github.com/blairham/sh/interp"
)

func exec(path string) interp.Action {
	return interp.Action{Kind: interp.ActionExec, Path: path, Args: []string{path}}
}

// Everything that is not an explicit allow is a denial, and the list of ways
// that happens is the point of the function.
func TestDecideDeniesEverythingThatIsNotAnAllow(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name     string
		out      acp.PermissionOutcome
		allow    interp.Decision
		remember bool
	}{
		{"allow once", sel(acp.OptionAllowOnce), interp.Allow, false},
		{"allow always", sel(acp.OptionAllowAlways), interp.Allow, true},
		{"reject once", sel(acp.OptionRejectOnce), interp.Deny, false},
		{"reject always", sel(acp.OptionRejectAlways), interp.Deny, true},
		{"the turn ended first", acp.PermissionOutcome{Outcome: acp.OutcomeCancelled}, interp.Deny, false},
		{"an option we never offered", sel("something-else"), interp.Deny, false},
		{"an outcome we do not know", acp.PermissionOutcome{Outcome: "invented"}, interp.Deny, false},
		{"nothing at all", acp.PermissionOutcome{}, interp.Deny, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := acp.Decide(tc.out)
			if got.Allow != tc.allow || got.Remember != tc.remember {
				t.Errorf("Decide(%+v) = %+v, want allow %v remember %v",
					tc.out, got, tc.allow, tc.remember)
			}
		})
	}
}

func sel(id string) acp.PermissionOutcome {
	return acp.PermissionOutcome{Outcome: acp.OutcomeSelected, OptionID: id}
}

// Which actions are worth stopping a person for. A glob stats in bulk, so a
// read that asked would be a prompt nobody reads.
func TestEscalatesOnlyForWhatChangesTheWorld(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name string
		a    interp.Action
		want bool
	}{
		{"an exec", exec("/bin/rm"), true},
		{"a write", interp.Action{Kind: interp.ActionOpen, Path: "/tmp/f", Write: true}, true},
		{"a signal", interp.Action{Kind: interp.ActionSignal, PID: 42, Signal: 9}, true},
		{"a read", interp.Action{Kind: interp.ActionOpen, Path: "/tmp/f"}, false},
		{"a stat", interp.Action{Kind: interp.ActionStat, Path: "/tmp/f"}, false},
		{"a directory read", interp.Action{Kind: interp.ActionReadDir, Path: "/tmp"}, false},
		{"an inherited descriptor", interp.Action{Kind: interp.ActionInherit, Path: "/dev/fd/3"}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := acp.Escalates(tc.a); got != tc.want {
				t.Errorf("Escalates(%+v) = %v, want %v", tc.a, got, tc.want)
			}
		})
	}
}

// asked counts the questions a gate put, so a test can say "and nobody was
// asked" rather than only "and it was refused".
type asked struct {
	mu      sync.Mutex
	actions []interp.Action
	answer  acp.Decision
	err     error
}

func (a *asked) ask(_ context.Context, act interp.Action) (acp.Decision, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.actions = append(a.actions, act)
	return a.answer, a.err
}

func (a *asked) count() int {
	a.mu.Lock()
	defer a.mu.Unlock()
	return len(a.actions)
}

// A policy refusal is final and is not put to a person. A button that
// overrides the sandbox is a sandbox that is advisory.
func TestAPolicyRefusalIsNotNegotiable(t *testing.T) {
	t.Parallel()
	a := &asked{answer: acp.Decision{Allow: interp.Allow}}
	g := &acp.Gate{
		Inner: interp.GateFunc(func(context.Context, interp.Action) interp.Decision {
			return interp.Deny
		}),
		Ask: a.ask,
	}
	if got := g.Allow(t.Context(), exec("/bin/rm")); got != interp.Deny {
		t.Errorf("decision = %v, want Deny", got)
	}
	if a.count() != 0 {
		t.Errorf("%d questions were asked, want none — the policy had already refused", a.count())
	}
}

// What the policy allows and the escalation set does not name proceeds
// without a question. This is the case that decides whether the front end is
// usable at all.
func TestAReadIsAllowedWithoutAsking(t *testing.T) {
	t.Parallel()
	a := &asked{answer: acp.Decision{Allow: interp.Deny}}
	g := &acp.Gate{Ask: a.ask}

	if got := g.Allow(t.Context(), interp.Action{Kind: interp.ActionStat, Path: "/etc/hosts"}); got != interp.Allow {
		t.Errorf("decision = %v, want Allow", got)
	}
	if a.count() != 0 {
		t.Errorf("%d questions were asked about a stat, want none", a.count())
	}
}

func TestAnEscalatedActionIsAskedAbout(t *testing.T) {
	t.Parallel()
	a := &asked{answer: acp.Decision{Allow: interp.Allow}}
	g := &acp.Gate{Ask: a.ask}

	if got := g.Allow(t.Context(), exec("/bin/ls")); got != interp.Allow {
		t.Errorf("decision = %v, want Allow", got)
	}
	if a.count() != 1 {
		t.Fatalf("%d questions were asked, want 1", a.count())
	}
	if a.actions[0].Path != "/bin/ls" {
		t.Errorf("asked about %q", a.actions[0].Path)
	}
}

// Nobody to ask is a denial, not a quiet allow. A shell with an escalating
// gate and no way to escalate must not proceed.
func TestNoAskerDenies(t *testing.T) {
	t.Parallel()
	g := &acp.Gate{}
	if got := g.Allow(t.Context(), exec("/bin/rm")); got != interp.Deny {
		t.Errorf("decision = %v, want Deny", got)
	}
}

// A question that could not be asked has not been answered.
func TestAFailedQuestionDenies(t *testing.T) {
	t.Parallel()
	a := &asked{answer: acp.Decision{Allow: interp.Allow}, err: errors.New("the connection closed")}
	g := &acp.Gate{Ask: a.ask}
	if got := g.Allow(t.Context(), exec("/bin/rm")); got != interp.Deny {
		t.Errorf("decision = %v, want Deny — the answer never arrived", got)
	}
}

// "Always" is kept, and is kept for the action rather than for the question.
func TestAlwaysIsRememberedAndAskedOnlyOnce(t *testing.T) {
	t.Parallel()
	a := &asked{answer: acp.Decision{Allow: interp.Allow, Remember: true}}
	g := &acp.Gate{Ask: a.ask, Remembered: &acp.Memory{}}

	for range 3 {
		if got := g.Allow(t.Context(), exec("/bin/ls")); got != interp.Allow {
			t.Fatalf("decision = %v, want Allow", got)
		}
	}
	if a.count() != 1 {
		t.Errorf("%d questions were asked, want 1 — the answer was not kept", a.count())
	}
}

func TestRejectAlwaysIsRememberedToo(t *testing.T) {
	t.Parallel()
	a := &asked{answer: acp.Decision{Allow: interp.Deny, Remember: true}}
	g := &acp.Gate{Ask: a.ask, Remembered: &acp.Memory{}}

	for range 3 {
		if got := g.Allow(t.Context(), exec("/bin/rm")); got != interp.Deny {
			t.Fatalf("decision = %v, want Deny", got)
		}
	}
	if a.count() != 1 {
		t.Errorf("%d questions were asked, want 1", a.count())
	}
}

// A once answer is not kept, which is the whole difference between the two
// allow kinds.
func TestOnceIsAskedEveryTime(t *testing.T) {
	t.Parallel()
	a := &asked{answer: acp.Decision{Allow: interp.Allow}}
	g := &acp.Gate{Ask: a.ask, Remembered: &acp.Memory{}}

	for range 3 {
		g.Allow(t.Context(), exec("/bin/ls"))
	}
	if a.count() != 3 {
		t.Errorf("%d questions were asked, want 3", a.count())
	}
}

// The memory is keyed on what was asked about. Allowing a program is not
// allowing a different one, and allowing a file to be *read* is not allowing
// it to be written — which is the pair that would be quietly wrong if the
// write flag were left out of the key.
func TestTheMemoryIsKeyedOnTheAction(t *testing.T) {
	t.Parallel()
	a := &asked{answer: acp.Decision{Allow: interp.Allow, Remember: true}}
	g := &acp.Gate{
		Ask:        a.ask,
		Remembered: &acp.Memory{},
		// Everything escalates here, so that the read and the write of one
		// path are both questions and the key is what tells them apart.
		Escalate: func(interp.Action) bool { return true },
	}

	read := interp.Action{Kind: interp.ActionOpen, Path: "/tmp/f"}
	write := interp.Action{Kind: interp.ActionOpen, Path: "/tmp/f", Write: true}
	g.Allow(t.Context(), read)
	g.Allow(t.Context(), read)
	if a.count() != 1 {
		t.Fatalf("%d questions for the same read, want 1", a.count())
	}
	g.Allow(t.Context(), write)
	if a.count() != 2 {
		t.Errorf("%d questions, want 2 — allowing a read allowed a write", a.count())
	}
	g.Allow(t.Context(), exec("/bin/ls"))
	if a.count() != 3 {
		t.Errorf("%d questions, want 3 — allowing one action allowed another", a.count())
	}
}

// A gate keeps state and is consulted from every goroutine a shell has: a
// background job and each half of a pipeline ask on their own. The first gate
// ever written against this seam raced the moment a script said `&`.
func TestAGateIsSafeUnderConcurrentQuestions(t *testing.T) {
	t.Parallel()
	a := &asked{answer: acp.Decision{Allow: interp.Allow, Remember: true}}
	g := &acp.Gate{Ask: a.ask, Remembered: &acp.Memory{}}

	var wg sync.WaitGroup
	for i := range 32 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			g.Allow(context.Background(), exec(string(rune('a'+i%4))))
		}()
	}
	wg.Wait()
	if a.count() == 0 {
		t.Error("nothing was asked at all")
	}
}
