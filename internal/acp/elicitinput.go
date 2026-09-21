// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package acp

import (
	"context"
	"io"
	"strings"
	"sync"
)

// A script's `read`, put to a person.
//
// The agent side of this shell has no terminal and cannot have one: the
// protocol is on the descriptors a prompt would need, so anything drawn into
// them is a message the client cannot read. What a session's standard input
// was, therefore, is the null device — a `read` gets end of file and nobody
// is asked anything.
//
// elicitation/create is the protocol's own answer to that, and the obstacle
// to using it was never the protocol. It was that **a shell's standard input
// is inherited by every command it runs, and os/exec reads a non-file one on
// the child's behalf whether or not the child ever reads it** — so a reader
// that asks a person would be asked once per external command, and almost
// every prompt a session is given runs one. A form for `echo` is worse than
// no form at all.
//
// [interp.Runner.ChildStdin] is what settles it: the shell's own input is the
// reader below and a child keeps the empty stream it already had. That field
// is #934's decision, and driver.Shell carries it here. The cost it names is
// that "the shell's input" now means two things; what keeps it from meaning
// three is that the substitution reaches the shell's own input and nothing a
// script asked for by name, so `cat < f` and `echo x | cat` are what they
// were.

// askingInput is the reader a session's shell reads from, or nil where the
// client cannot reach a person.
//
// A client that does not claim form elicitation is not asked one, which is
// the rule every other capability here is held to: the file and terminal
// capabilities are read before they are used, and a claim is what makes a
// method safe to call. A session on such a client keeps the empty input, so
// the behavior this replaces is still exactly the behavior for every client
// that could not answer anyway.
func (s *session) askingInput() io.Reader {
	s.agent.mu.Lock()
	e := s.agent.client.Elicitation
	s.agent.mu.Unlock()
	if e == nil || e.Form == nil {
		return nil
	}
	return &askingReader{s: s}
}

// askingReader turns a read of the shell's input into one question.
//
// The whole answer is buffered, because a `read` builtin takes its line a
// byte at a time and one question per byte is not a question. So the first
// Read that finds the buffer empty asks, and every Read after it drains what
// the person gave until the next one is needed.
type askingReader struct {
	s *session

	mu   sync.Mutex
	rest string
	// done is set by a *failure* and not by an answer: a client that cannot
	// serve the method, or a connection that has gone, must not be asked
	// again by every later read in the same loop.
	done bool
}

// The field a line arrives in, and what the person is told they are
// answering. One string property, because what a shell's `read` wants is a
// line and a form with one field is the smallest true description of that.
const (
	askLineField   = "line"
	askLineTitle   = "Standard input"
	askLineMessage = "The shell is reading a line from standard input."
)

// Read serves what is left of the last answer, and asks for another when
// there is nothing left.
//
// End of file is what a decline, a cancel and an unanswerable connection all
// come back as, because end of file is what a shell's input says when there
// is no more of it — `read` fails, the `while read` around it stops, and the
// script goes on to whatever follows. A decline does *not* make the stream
// dead: the person said no to this question, and a later `read` is a later
// question, which is how a terminal behaves after somebody types the end-of-
// file key. Only a failure is final, and that is the `done` field.
func (a *askingReader) Read(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.rest == "" {
		if a.done {
			return 0, io.EOF
		}
		line, ok, fatal := a.ask()
		if fatal {
			a.done = true
		}
		if !ok {
			return 0, io.EOF
		}
		a.rest = line
	}
	n := copy(p, a.rest)
	a.rest = a.rest[n:]
	return n, nil
}

// ask puts one question and reports the line, whether there was one, and
// whether the failure was the kind that must not be retried.
func (a *askingReader) ask() (line string, ok, fatal bool) {
	ctx := a.s.currentTurn()
	if ctx == nil {
		// No turn is running, so there is nothing for a person to be
		// answering: a read from outside a prompt is the shell reading at a
		// moment the client is not watching. Not fatal — the next turn is a
		// moment it is.
		return "", false, false
	}
	var resp CreateElicitationResponse
	if err := a.s.agent.conn.Call(ctx, MethodCreateElicitation, CreateElicitationRequest{
		SessionID: a.s.id,
		Mode:      ElicitForm,
		Message:   askLineMessage,
		RequestedSchema: &ElicitationSchema{
			Type:  "object",
			Title: askLineTitle,
			Properties: map[string]ElicitationProperty{
				askLineField: {
					Type:        PropertyString,
					Title:       askLineTitle,
					Description: askLineMessage,
				},
			},
			Required: []string{askLineField},
		},
	}, &resp); err != nil {
		// A client that claimed the capability and then refused the call, or
		// a connection that has gone. Either way asking again would produce
		// the same answer for every read in the loop.
		//
		// A canceled turn is the exception and is not the reader's to
		// remember: the person stopped this turn, not the session.
		return "", false, ctx.Err() == nil
	}
	if resp.Action != ElicitAccept {
		return "", false, false
	}
	text, _ := resp.Content[askLineField].(string)
	// The newline the person did not type. A form field is a line without
	// one, and a shell's `read` is looking for the terminator — without it
	// the builtin reads to end of file and a `while read` loop gets one
	// iteration for the whole conversation.
	//
	// Whatever the field already ends with is left alone, so a client that
	// sends the terminator does not get two.
	if !strings.HasSuffix(text, "\n") {
		text += "\n"
	}
	return text, true, false
}

// currentTurn is the context of the prompt turn that is running, and nil
// where none is.
//
// A question belongs to a turn: it is the turn the client is waiting on, and
// canceling the turn has to end the wait. Read off the session rather than
// captured when the reader was built, because a session outlives every turn
// in it and a captured context would be one that had already been canceled.
func (s *session) currentTurn() context.Context {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.turn
}
