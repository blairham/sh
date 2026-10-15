// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"io"
	"os"
)

// ownDescriptors gives a shell that runs *beside* the one that made it its own
// copy of every descriptor in the table, and answers with what gives them
// back.
//
// # The boundary a fork gives for free
//
// clone copies the descriptor table and shares the streams in it, which is
// right for a subshell the parent waits for: `( exec {a}<&- )` must not take
// the parent's descriptor away, and nothing runs in the parent while the
// parentheses do. It is wrong the moment the two shells run at once. A real
// shell forks, so the concurrent shell holds *its own* descriptors and the
// parent's close ends the parent's name for the file and nothing else; here
// both tables point at one `*os.File`, so a close on either side ends it for
// both.
//
// That is #2116, and what it costs is a command that never runs. A plugin
// manager's scheduler parks a descriptor on every turn and drops it on the
// next:
//
//	exec {a}< <(sleep 0.002; print run)   # the body runs on a goroutine
//	exec {a}<&-                           # …while this closes its table's copy
//
// Both spellings of the damage were measured. When the close lands *before*
// the body starts a command, the descriptor reaches that command closed —
// `exec {a}<f; exec {b}< <(sh -c "cat <&$a")` says `Bad file descriptor`
// where zsh 5.9.2 reads the file, because os/exec turns a closed `*os.File`
// into a number the child closes. When it lands *while* os/exec is starting
// one, the number it already read is gone by the time the child dups it and
// the start itself fails: `@zi-scheduler: sleep: fork/exec /bin/sleep: bad
// file descriptor`, on roughly half of this machine's real startups and
// several times over on the ones it appears in.
//
// The second is a race and the first is not, which is why the fix is tested
// by the first: they are one bug about ownership, and only one of them can be
// asked a question that always answers the same way.
//
// # The input is part of the table
//
// Descriptor 0 is not in `fds` — the named three are fields — and leaving it
// out made the same bug again one number lower. A process substitution's body
// reads the input of the command the word stands in, and inside a pipeline
// that input is the element's pipe, which runPipeline closes the moment the
// element finishes; a body that outlives its element then read a descriptor
// that was already gone. Measured with `printf "PIPE\n" | { exec 3< <(sleep
// 0.4; cat >out); }`, where bash 5.3 writes `PIPE` and this wrote
// `cat: stdin: Bad file descriptor` (#2144).
//
// **The writing side is deliberately not copied**, and that is not an
// oversight to fix later: a second holder of a pipe's *write* end is a reader
// that never sees end-of-file, so duplicating stdout here would change when a
// pipeline ends rather than only who owns what. Nothing holds a read end
// against anybody, which is why this half is safe on its own. The one shell
// that *should* change when its stream's reader ends is a writing
// substitution's body, and it takes its copy by name — see
// ownWritingSubstOutput.
//
// # What it does not copy
//
// A descriptor the shell opened for its *own* plumbing stays shared, which is
// the same exclusion childFiles makes and for the same reason: a coprocess's
// near ends are in the table so `>&${C[1]}` can find them, and a second
// holder of the write end is a coprocess that never reads end-of-file. See
// shellOwnedFd.
//
// Anything that is not a file is left alone too. There is no descriptor to
// duplicate for an embedder's buffer, and nothing closes one, so sharing it
// is what it already was.
//
// # The lifetime
//
// Released when the shell that took them ends, by the same hand-over that
// already releases a substitution's pipe end and finishes a background job —
// so a body that outlives its command, and a `&` job that outlives the body,
// each let go of their own copies and of nobody else's. A clone made by a
// clone duplicates again, which is what keeps the inner one's copies from
// ending when the outer one returns.
func (c *Runner) ownDescriptors() func() {
	var dups []*os.File
	var nums []int
	// Inside the kernel's first table where there is room, and above the
	// region this dialect publishes where there is not — see dupFile,
	// privateCopyFloor and Runner.privateFdFloor. These copies are the
	// shell's own and no script can name them, so the only thing their
	// numbers could do is crowd out a number that *is* published, and
	// registering them is what stops that.
	floor := c.privateFdFloor()
	// take is the copy itself, written once because the input and the table
	// entries are the same question about two places a descriptor is kept.
	//
	// A failure leaves the entry as it was — nothing to duplicate with, or
	// nothing left to duplicate. That is the sharing this is here to end, and
	// a shell that cannot take its own copy is no worse off than it was
	// before there was one to take.
	take := func(v any) (*os.File, bool) {
		f, ok := v.(*os.File)
		if !ok {
			return nil, false
		}
		d, n, err := dupFile(f, floor)
		if err != nil {
			return nil, false
		}
		holdPrivateCopy(n)
		dups = append(dups, d)
		nums = append(nums, n)
		return d, true
	}
	if d, ok := take(c.Stdin); ok {
		c.Stdin = d
	}
	for fd, v := range c.fds {
		if d, ok := take(v); ok {
			c.fds[fd] = d
		}
	}
	if dups == nil {
		return func() {}
	}
	return func() {
		for i, d := range dups {
			releasePrivateCopy(nums[i])
			_ = d.Close()
		}
	}
}

// ownWritingSubstOutput gives a `>(cmd)`'s body its own copy of the output it
// writes into, where that output is a descriptor, and answers with what gives
// the copy back. ok is false where there is no descriptor to copy — an
// embedder's io.Writer — or the copy could not be made, and then out is w
// unchanged.
//
// # The second holder ownDescriptors refuses is exactly right here
//
// Every shell that has the construct forks the body, so the body is a process
// holding its standard output open for as long as it runs, and whoever reads
// that stream reads until the body is done as well. That is a measured
// ordering, not a lifetime detail. In bash 5.3.20, 20 runs of 20 each,
// 2026-10-15:
//
//	{ echo hi; echo B >&4; } > >(sed s/^/A/) 4> >(sed s/^/b/)    Ahi AbB
//	{ echo A >&3; echo B >&4; } 3> >(sed s/^/x/) 4> >(sed s/^/y/) yB xA
//
// The second body is started after `> >(sed s/^/A/)` was applied, so its
// standard output *is* the first pipe's writing end, and it writes `bB`
// through the first sed. Here that stream was the shell's own `*os.File`,
// shared: the command's redirections were undone, the shell closed it, and
// the body's bytes — held for the join — were delivered into a closed file
// and lost without a word (#6526). The same sharing is the zsh dialect's
// `sed: stdout: Bad file descriptor` on that row. And `{ : >(sleep 1); } |
// cat` takes a second in bash and zsh, because the body holds the pipeline's
// writing end; here it took none.
//
// # And a descriptor is written straight through
//
// A body that does not hold its own copy has its output captured and
// delivered at the join — procSubPipe.captured — because two goroutines on
// one io.Writer are a data race. Two writers on one descriptor are the
// kernel's problem, as they are for every shell that forks, so a body given a
// copy here writes as it goes, and the order its bytes arrive in is the order
// it wrote them: `echo a > >(cat); sleep .3; echo b` is `a b` in bash, and
// was `b a` in this shell's bash dialect while the bytes were held to the end
// of the script.
func (c *Runner) ownWritingSubstOutput(w io.Writer) (out io.Writer, release func(), ok bool) {
	f, isFile := w.(*os.File)
	if !isFile {
		return w, func() {}, false
	}
	d, n, err := dupFile(f, c.privateFdFloor())
	if err != nil {
		return w, func() {}, false
	}
	holdPrivateCopy(n)
	return d, func() {
		releasePrivateCopy(n)
		_ = d.Close()
	}, true
}
