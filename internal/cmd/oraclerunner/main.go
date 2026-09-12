// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

// Command oraclerunner is the oracle harness, inside a container.
//
// It exists for one panel member. There is no BusyBox ash on a macOS machine,
// so the ash column is reached through a container image instead of through a
// path (see oracle.Reach), and something on the far side has to run the case.
//
// The thing it deliberately is *not* is a second harness. It runs
// oracle.Exec — the same function, cross-compiled from the same tree — so the
// environment scrub, the signal-disposition scrub, the ten-second timeout,
// the wait-status reading and every normalization rule are shared with the
// columns that are ordinary binaries. A container command line built to
// resemble what Exec does would be the second mechanism this repository keeps
// being bitten by, and it would drift the first time a rule was added on one
// side only.
//
// It speaks one JSON object per line in each direction: an oracle.Request in,
// an oracle.Reply out, in order, forever. One long-lived process rather than
// one container invocation per case is what makes the column cost seconds
// instead of half an hour — inside, a case is an ordinary fork and exec.
//
//	oraclerunner -lookup /bin/ash,/bin/busybox
package main

import (
	"bufio"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/blairham/sh/internal/oracle"
)

func main() {
	lookup := flag.String("lookup", "/bin/sh", "comma-separated candidate paths for the shell in this image, tried in order")
	idle := flag.Duration("idle", 5*time.Minute, "exit if no case arrives for this long")
	flag.Parse()

	out := bufio.NewWriter(os.Stdout)
	say := func(r oracle.Reply) {
		b, err := json.Marshal(r)
		if err != nil {
			// Nothing useful is left to say on the wire; the caller's read
			// will fail and it will report the container as having died,
			// which is what this is.
			fmt.Fprintln(os.Stderr, "oraclerunner:", err)
			return
		}
		out.Write(b)
		out.WriteByte('\n')
		// Flushed per line and not at the end. A case that kills its own
		// process group can take this process with it, and every
		// measurement made before that one has to have already arrived.
		out.Flush()
	}

	ctx := context.Background()
	path, ok := oracle.Locate(strings.Split(*lookup, ","))
	if !ok {
		say(oracle.Reply{Err: fmt.Sprintf("no shell in this image at any of %s", *lookup)})
		os.Exit(1)
	}
	say(oracle.Reply{Path: path, Version: oracle.Version(ctx, path)})

	// The idle timeout is what stops a container outliving the harness that
	// started it. The caller removes it when it is done, but a caller that
	// is killed cannot, and a container left holding a shell nobody is
	// measuring is exactly the kind of stray this repository does not want on
	// a machine running eleven sessions at once.
	in := requests()
	for {
		var req incoming
		select {
		case got, ok := <-in:
			if !ok {
				return
			}
			req = got
		case <-time.After(idleAfter(*idle)):
			fmt.Fprintln(os.Stderr, "oraclerunner: no case in", *idle, "- nobody is listening")
			return
		}
		if req.err != "" {
			say(oracle.Reply{Err: req.err})
			continue
		}
		res := oracle.Exec(ctx, req.req.Shell, req.req.Case)
		say(oracle.Reply{Result: &res})
	}
}

// idleAfter keeps the wait longer than a single case can take, so a shell the
// harness is patiently timing out is never mistaken for a caller that left.
func idleAfter(d time.Duration) time.Duration {
	if floor := oracle.RunTimeout + time.Minute; d < floor {
		return floor
	}
	return d
}

type incoming struct {
	req oracle.Request
	err string
}

// requests reads one case per line until end of file.
//
// A scanner rather than a decoder because the framing is the protocol: one
// object per line in each direction, so a line that will not parse costs that
// one case rather than desynchronising the stream.
func requests() <-chan incoming {
	ch := make(chan incoming)
	go func() {
		defer close(ch)
		in := bufio.NewScanner(os.Stdin)
		// A case snippet can be long, and the default 64KiB limit would end
		// the run silently at the first one that is.
		in.Buffer(make([]byte, 0, 1<<16), 1<<24)
		for in.Scan() {
			line := strings.TrimSpace(in.Text())
			if line == "" {
				continue
			}
			var req oracle.Request
			if err := json.Unmarshal([]byte(line), &req); err != nil {
				ch <- incoming{err: "unreadable request: " + err.Error()}
				continue
			}
			ch <- incoming{req: req}
		}
	}()
	return ch
}
