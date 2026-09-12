// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import "os"

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
	for fd, v := range c.fds {
		f, ok := v.(*os.File)
		if !ok {
			continue
		}
		d, err := dupFile(f)
		if err != nil {
			// Nothing to duplicate with, or nothing left to duplicate. The
			// entry stays as it was, which is the sharing this is here to
			// end — a shell that cannot take its own copy is no worse off
			// than it was before there was one to take.
			continue
		}
		c.fds[fd] = d
		dups = append(dups, d)
	}
	if dups == nil {
		return func() {}
	}
	return func() {
		for _, d := range dups {
			_ = d.Close()
		}
	}
}
