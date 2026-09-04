// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"context"
	"os"

	"github.com/blairham/sh/syntax"
)

// coprocClause runs `coproc [NAME] command`: the command goes to the
// background with a pipe on each of its named streams, and the shell keeps
// the near ends in the array the name names — [0] to read what the command
// writes, [1] to write what it reads — with the process in NAME_PID.
//
// Real pipes rather than in-process ones, because the command is usually an
// external process and a pipe an os/exec child inherits is a descriptor, not
// a Go value. Closing the write end through `{v}>&-` is what lets the
// command see its input finish, which is why that close is a real one.
func (r *Runner) coprocClause(ctx context.Context, c *syntax.CoprocClause) error {
	name := c.Name
	if name == "" {
		name = "COPROC"
	}
	// What the command reads: the shell writes shellW, the command reads
	// childIn. And the reverse for what it writes.
	childIn, shellW, err := os.Pipe()
	if err != nil {
		r.diagf("%v\n", err)
		r.status = 1
		return nil
	}
	shellR, childOut, err := os.Pipe()
	if err != nil {
		_ = childIn.Close()
		_ = shellW.Close()
		r.diagf("%v\n", err)
		r.status = 1
		return nil
	}

	job := &Job{
		done:    make(chan struct{}),
		ready:   make(chan struct{}),
		Command: name,
	}
	sub := r.clone()
	sub.retagTrapBoundary(trapContextBackground)
	sub.bg = job
	sub.Stdin = childIn
	sub.Stdout = childOut
	// Only the two named streams go through the pipes; complaints still
	// reach whoever is watching the shell.
	sub.Stderr = r.lockedStderr()
	go func() {
		if err := sub.command(ctx, c.Cmd); err != nil {
			sub.diagf("%v\n", err)
		}
		// The command is done with its ends, and closing them here is what
		// turns its exit into end-of-file for whoever reads NAME[0].
		_ = childIn.Close()
		_ = childOut.Close()
		job.markReady()
		job.finish(sub.status)
	}()
	<-job.ready

	r.jobs = append(r.jobs, job)
	r.lastJob = job

	// The near ends go into the descriptor table the way `exec {fd}>f`
	// would put them there: numbered from ten up, for keeps.
	rfd := r.nextFreeFd()
	r.setFd(rfd, shellR)
	wfd := r.nextFreeFd()
	r.setFd(wfd, shellW)
	r.setArrayElem(name, 0, itoa(rfd))
	r.setArrayElem(name, 1, itoa(wfd))
	r.setVar(name+"_PID", itoa(job.PID))
	r.status = 0
	return nil
}
