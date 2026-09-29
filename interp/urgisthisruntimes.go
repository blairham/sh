// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

// Why a trap for `URG` runs on signals nothing sent, and why this package
// cannot stop it.
//
// `trap 'print CAUGHT' URG; kill -URG $$` writes `CAUGHT` **twice** here and
// once in the reference: the real signal, and one the Go runtime sent itself.
// The runtime uses `SIGURG` to preempt a goroutine that has run too long, and
// `os/signal` hands those to whatever registered for the signal — which is
// this shell, as soon as a script traps it (#5109).
//
// Measured 2026-09-29 on darwin/arm64 with Go 1.26.6 and on linux/arm64 in
// `golang:1.26`, script files under `env -i PATH=/usr/bin:/bin` with a scratch
// HOME and standard input on the null device. Reference
// `/opt/homebrew/bin/zsh`, zsh 5.9.2.
//
// **It is URG alone.** All 29 signals a script may trap were swept with the
// same busy loop; only `URG` catches anything, and the reference catches
// nothing on any of them.
//
//	catches, nothing sent    	GOMAXPROCS=1  GOMAXPROCS=2  GOMAXPROCS=4
//	this shell               	           7            33            53
//	the reference            	           0             0             0
//
// The rate rising with `GOMAXPROCS` is the tell: more parallelism, more
// preemption. It is not confined to a busy loop — `trap … URG; sleep 1` catches
// one — and it is not confined to a platform.
//
// **The mechanism is confirmed by switching it off.** With
// `GODEBUG=asyncpreemptoff=1` in the environment the count is 0 at every
// `GOMAXPROCS`, on both platforms. So it is asynchronous preemption and nothing
// else.
//
// **And every route to that setting from inside this program is closed**, which
// is the part worth writing down because each one looks like it should work:
//
//	os.Setenv("GODEBUG", "asyncpreemptoff=1")	no effect: 20 spurious before
//	                                         	and 20 after, in one process
//	//go:debug asyncpreemptoff=1             	build error, `unknown
//	                                         	//go:debug setting`
//	godebug asyncpreemptoff=1 in go.mod      	load error, `unknown godebug`
//
// The setting is read at runtime start and is not one the toolchain will let a
// module pin, so only the environment of the exec that started the shell can
// carry it — which a shell cannot arrange for itself.
//
// **Nor can the arrival be filtered.** Telling the runtime's signal from a
// script's needs the `si_code` — `SI_TKILL` against `SI_USER` — and
// `os/signal` delivers a bare `os.Signal` with no sender and no code. The
// runtime does filter on that code for signals nobody registered, and stops
// filtering the moment a program asks to be notified, which is exactly when a
// script traps it.
//
// So the arrangement here is deliberate: subscribe as for any other signal,
// and record the defect. Two things follow for whoever fixes it.
//
// **The discriminating row is `trap 'print CAUGHT' URG; kill -URG $$`**, which
// the reference answers with one `CAUGHT` and this shell with two. A fix that
// simply stops delivering `URG` answers it with none, and a row that only
// asserts "no spurious catches" would call that a pass — the wrong fix and the
// right one are told apart only by keeping the real signal in the same
// assertion. TestARealUrgStillRunsItsTrap is that half.
//
// **And the fix is not in this file.** It is either the environment of the
// process that starts the shell, a change in what `os/signal` forwards, or a
// deliberate refusal to trap the signal at all — and the last would break a
// row the whole panel agrees on, since every reference shell takes
// `trap 'x' URG` quietly.
