// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

// Handing a caller's streams back when the shell is done with them.
//
// # The hole
//
// A `<( … )` body runs on a goroutine of its own, and at the **top level**
// nothing joins it. endHeldProcSubs joins r.bodies only when r.ownsBodies,
// and that flag is set by collectBodies, which is set up around a *command
// substitution*; heldProcSubs is the `exec 3> >(cat)` shape and does not
// cover it either. So a `<(cmd)` written on the script's own line is never
// waited for, and anything it writes to the shell's standard error reached
// the embedder's io.Writer with **no happens-before edge to Run returning**.
// A background job left running at the end is the same hole by a different
// road.
//
// lockedWriter serializes the shell's own writers against each other and says
// nothing about a reader outside the runner. In a shell binary both ends are
// fd 2 and it is a wash; in a library it is a data race, and it is the one CI
// reported over `while read -r l; do :; done < <(v=$(echo hi; for))` on
// #3963 — between strings.(*Builder).String() in the test and
// strings.(*Builder).Write() under lockedWriter on the body's goroutine, with
// the body goroutine reported as *finished*. A missing edge, not an overlap.
//
// # Why the answer is not to join the body
//
// Measured 2026-09-21, timing the **shell's own exit** rather than a `$( … )`
// around it — a capture waits for the pipe, which the abandoned body is still
// holding open, and reports 1.0s for every column whatever the shell did:
//
//	sh -c 'echo <(sleep 1; echo late >&2) >/dev/null' >out 2>err
//
//	bash 5.3     exits in 0.01s     `late` lands in err ~1s later
//	bash 3.2     exits in 0.00s     `late` lands in err ~1s later
//	ksh93        exits in 0.00s     `late` lands in err ~1s later
//	zsh 5.9.2    exits in 0.01s     `late` lands in err ~1s later
//
// Unanimous, and the same for `exec 3< <(sleep 1; echo late)` and for a body
// nobody ever reads. dash and ash have no process substitution, so there is
// no axis here and no dialect is asked. Joining would hang the exit on
// `<(sleep 60)`, which no shell in the panel does.
//
// So the shell must exit without waiting, and the body may still be writing
// when it does. What a real shell has that this does not is a **file
// descriptor**: the body is a process, fd 2 outlives the shell, and the
// kernel makes the late write safe for everyone. An embedder's io.Writer has
// no such property, and the shell is the party that created the concurrency.
//
// # The seal
//
// So the edge is put where the descriptor would be. When the shell ends, the
// runner takes the stream lock and marks it sealed:
//
//   - every write that already happened is ordered before that Lock, which is
//     ordered before Finish returning, which is ordered before whatever the
//     caller does with its writer next. The finished body's bytes are visible
//     and the race is gone.
//   - every write after it takes the same lock, sees the seal, and never
//     touches the caller's writer at all. An abandoned body cannot reach it.
//
// A write still in flight at the seal is covered by the first of those: the
// seal waits on the lock the write is holding.
//
// **A stream that is an *os.File is not sealed, because it is not wrapped.**
// lockWriter leaves a file alone — the kernel is already the guard — so a
// shell binary, whose streams are os.Stdout and os.Stderr, behaves exactly as
// the panel does above: it exits at once and the late bytes still land. The
// seal is only over the streams interp took ownership of by wrapping, which
// is precisely the set an embedder cannot read safely on its own.
//
// # Why the sealed write reports success
//
// There is nobody left to tell. The write is on an abandoned goroutine of a
// shell that has returned; an error would be reported through the stream that
// has just been taken away, or dropped, and a shell that answered EBADF to
// its own body would be inventing a descriptor state no script can observe.
// The bytes are lost the way ksh93 loses a `>(cmd)` body's output — see
// Runner.bodies, which records that measurement — and losing them is the
// price of a caller's io.Writer not being a descriptor. A caller that wants
// the late output has the same answer a real shell gives: hand the shell an
// *os.File.
//
// # Why it is disarmed again
//
// A Runner is not always used once. RunPart is the chunk-at-a-time API a
// front end reading a line at a time drives, and a caller may reach Finish
// and then run more; a seal that never came off would leave such a shell
// writing into nothing for the rest of the session. Asking for more work is
// the caller handing the streams *back*, which is the same boundary in the
// other direction, so both entry points clear it.

// sealStreams hands the caller's streams back: nothing this shell started may
// write to them again, and everything it already wrote is visible.
//
// Only on the shell at the top. A clone reaches Finish too — a subshell, a
// command substitution's body, a `<(cmd)` body — and it shares these locks
// with the shell above it, so one of them sealing would silence the shell
// that is still running. The same boundary cleanUpAtEnd and runExitTrap are
// held behind, for the same reason.
//
// A runner that never made stream locks never wrapped a stream, so there is
// no lockedWriter anywhere that could read a seal and nothing to set.
func (r *Runner) sealStreams() {
	r.setStreamSeal(true)
}

// unsealStreams takes it off again, for a caller that drives the shell on
// past an end. See the last section above.
func (r *Runner) unsealStreams() {
	r.setStreamSeal(false)
}

func (r *Runner) setStreamSeal(sealed bool) {
	if r.inSubshell || r.streams == nil {
		return
	}
	r.streams.write.Lock()
	defer r.streams.write.Unlock()
	r.streams.sealed = sealed
}
