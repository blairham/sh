// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash

import (
	"context"
	"fmt"
	"strings"

	"github.com/blairham/sh/interp"
)

// `caller` is bash's question about who called, and it reads the same stack
// the three arrays in callstack.go name — the same frames, one step up.
//
// Measured shapes, from a script two calls deep:
//
//   - bare: the line the current frame was entered from and the file of the
//     frame above it — `2 /s/main.sh` — with `NULL` standing in where there
//     is no frame above, which is what `-c` prints and what the top level of
//     a script prints as `0 NULL`, status 0 either way. With no frame at all
//     — the top level of `-c` — it says nothing and reports 1.
//   - with an expression: line, function and file of the named depth — `2 g
//     /s/main.sh` for 0, `3 main /s/main.sh` for 1 — where the script's own
//     frame answers to bash's name for it, `main`. A depth past the stack is
//     silence and 1.
//   - a word that is not a plain number is `invalid number` and one that
//     leads with `-` is `invalid option`, each with the usage line after it
//     and 2 — `1+1` measured among them, so no arithmetic happens here.
const callerUsage = "caller: usage: caller [expr]"

// registerCaller installs the builtin.
func registerCaller(r *interp.Runner) {
	r.Register("caller", callerBuiltin)
}

func callerBuiltin(r *interp.Runner, _ context.Context, args []string) int {
	frames := r.CallStack()
	if len(args) == 0 {
		if len(frames) == 0 {
			return 1
		}
		file := "NULL"
		if len(frames) > 1 {
			file = frames[1].File
		}
		_, _ = fmt.Fprintf(r.Out(), "%d %s\n", frames[0].Line, file)
		return 0
	}
	depth, code := callerDepth(r, args[0])
	if code != 0 {
		return code
	}
	if depth+1 >= len(frames) {
		// Nothing was calling at that depth. Silence, and 1.
		return 1
	}
	name := frames[depth+1].Name
	if name == "" {
		// The script's own frame, which bash calls main — the same answer
		// FUNCNAME gives for it.
		name = "main"
	}
	_, _ = fmt.Fprintf(r.Out(), "%d %s %s\n", frames[depth].Line, name, frames[depth+1].File)
	return 0
}

// callerDepth reads the expression, which despite the manual's word for it is
// a plain number: `1+1` is refused, measured.
func callerDepth(r *interp.Runner, word string) (int, int) {
	if strings.HasPrefix(word, "-") {
		r.Diagnosef("caller: %s: invalid option\n", word)
		_, _ = fmt.Fprintf(r.Err(), "%s\n", callerUsage)
		return 0, 2
	}
	depth := 0
	for i := 0; i < len(word); i++ {
		if word[i] < '0' || word[i] > '9' {
			r.Diagnosef("caller: %s: invalid number\n", word)
			_, _ = fmt.Fprintf(r.Err(), "%s\n", callerUsage)
			return 0, 2
		}
		depth = depth*10 + int(word[i]-'0')
	}
	if word == "" {
		r.Diagnosef("caller: %s: invalid number\n", word)
		_, _ = fmt.Fprintf(r.Err(), "%s\n", callerUsage)
		return 0, 2
	}
	return depth, 0
}
