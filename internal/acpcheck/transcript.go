// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package acpcheck

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"sync"
)

// A transcript of one real session, for showing somebody.
//
// Every other output here is a verdict, and a verdict is something you have to
// take on trust. This is the bytes: the same connection the graded rows use,
// printed in the order the two processes said them, with a line of English
// beside each one. Somebody who does not know this codebase can read it and
// see the shell announce a write, ask, be told no, and not write the file.
//
// The annotations are the only editorializing, and they are attached by method
// name rather than by position, so a transcript that changes shape does not
// quietly keep its old commentary.

// Transcript drives one session and returns it, annotated.
//
// dialect is which shell the binary should be; empty is its default.
func Transcript(ctx context.Context, bin, dir, dialect string) (string, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	var (
		mu    sync.Mutex
		lines []string
	)
	trace := func(direction, line string) {
		mu.Lock()
		defer mu.Unlock()
		lines = append(lines, direction+" "+line)
	}
	// Refuse the first thing asked and allow the second, so that one
	// transcript shows both answers and the difference they make.
	n := 0
	answer := func(Ask) string {
		n++
		if n == 1 {
			return RejectOnce
		}
		return AllowOnce
	}
	args := []string{"-acp"}
	if dialect != "" {
		args = append([]string{"-dialect", dialect}, args...)
	}
	c, err := Dial(bin, Options{Args: args, Dir: dir, Answer: answer, Trace: trace})
	if err != nil {
		return "", err
	}
	if _, err := c.Initialize(ctx); err != nil {
		_ = c.Close()
		return "", err
	}
	session, err := c.NewSession(ctx, dir)
	if err != nil {
		_ = c.Close()
		return "", err
	}
	if _, err := c.Prompt(ctx, session, `eval "echo payload > refused.txt"; echo kept-going`); err != nil {
		_ = c.Close()
		return "", err
	}
	if _, err := c.Prompt(ctx, session, `eval "echo payload > allowed.txt"; cat allowed.txt`); err != nil {
		_ = c.Close()
		return "", err
	}
	_ = c.Close()

	_, refusedErr := os.Stat(dir + "/refused.txt")
	_, allowedErr := os.Stat(dir + "/allowed.txt")

	var b strings.Builder
	fmt.Fprintf(&b, "A session, as it went over the pipe\n\n")
	fmt.Fprintf(&b, "  --> is the client writing to the agent's standard input\n")
	fmt.Fprintf(&b, "  <-- is the agent writing to its standard output\n\n")
	fmt.Fprintf(&b, "  The client refuses the first thing it is asked and allows the second.\n")
	fmt.Fprintf(&b, "  Both writes are inside an eval, so neither one is visible in the text\n")
	fmt.Fprintf(&b, "  of the command that was sent.\n\n")
	mu.Lock()
	for _, l := range lines {
		direction, body, _ := strings.Cut(l, " ")
		fmt.Fprintf(&b, "  %s %s\n", direction, body)
		if note := annotate(body); note != "" {
			fmt.Fprintf(&b, "      %s\n", note)
		}
	}
	mu.Unlock()
	fmt.Fprintf(&b, "\n  Afterwards, on disk:\n")
	fmt.Fprintf(&b, "    refused.txt  %s\n", exists(refusedErr))
	fmt.Fprintf(&b, "    allowed.txt  %s\n", exists(allowedErr))
	fmt.Fprintf(&b, "\n  The refusal is the file that is not there. The shell did not report a\n")
	fmt.Fprintf(&b, "  refusal and write it anyway, and it carried on with the rest of the\n")
	fmt.Fprintf(&b, "  line rather than dying, which is what a refused open does in a shell.\n")
	return b.String(), nil
}

func exists(err error) string {
	if err == nil {
		return "written"
	}
	return "does not exist"
}

// annotate explains one message, by what it is.
func annotate(line string) string {
	var m struct {
		Method string `json:"method"`
		Result struct {
			StopReason string `json:"stopReason"`
			SessionID  string `json:"sessionId"`
			Protocol   *int   `json:"protocolVersion"`
		} `json:"result"`
		Params struct {
			Update struct {
				Kind   string `json:"sessionUpdate"`
				Title  string `json:"title"`
				Status string `json:"status"`
			} `json:"update"`
			Outcome struct {
				OptionID string `json:"optionId"`
			} `json:"outcome"`
		} `json:"params"`
		Error *RPCError `json:"error"`
	}
	if json.Unmarshal([]byte(line), &m) != nil {
		return ""
	}
	switch {
	case m.Method == "initialize":
		return "the client opens: which protocol version, and what it can do"
	case m.Method == "session/new":
		return "open a session — a shell of its own, rooted at a directory"
	case m.Method == "session/prompt":
		return "a turn: the shell to run"
	case m.Method == "session/request_permission":
		return "the agent stops and asks. Nothing has happened yet"
	case m.Method == "session/update":
		switch m.Params.Update.Kind {
		case "tool_call":
			return "announced before it happens: " + m.Params.Update.Title
		case "tool_call_update":
			return "how it went: " + m.Params.Update.Status
		case "agent_message_chunk":
			return "output from the shell, as it was printed"
		}
		return ""
	case m.Error != nil:
		return "refused: " + m.Error.Message
	case m.Result.Protocol != nil:
		return "the agent answers with the version it speaks and what it is"
	case m.Result.SessionID != "":
		return "the session id, which every later message carries"
	case m.Result.StopReason != "":
		return "the turn is over: " + m.Result.StopReason
	}
	if strings.Contains(line, `"outcome"`) {
		return "the client's answer"
	}
	return ""
}
