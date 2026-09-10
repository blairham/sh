// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"os"
	"testing"
)

// `zsocket`, measured against zsh 5.9.2 (2026-09-09) with `zsh -f`.
//
// Every accept here is written `-a -t`, which returns rather than waiting: a
// test that blocked on a connection that never came would hang the suite
// rather than fail it.

// socketDir is a directory short enough to hold a socket.
//
// t.TempDir is not, and that is a fact about the kernel rather than about this
// package: the address of a Unix-domain socket lives in a fixed-width field —
// 104 bytes on this machine — and t.TempDir spells the *test's own name* into
// the path, so a descriptive name is what pushes the address over. The failure
// is `invalid argument` from the bind, which reads like a bad address and is a
// long one.
//
// Removed at the end of the test the way t.TempDir's is, so nothing is left
// behind either way.
func socketDir(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("", "zsk")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	return dir
}

// **A socket, and a message across it.** The three forms in one script,
// because each on its own proves nothing: a listener nobody connects to, a
// connection nobody accepts and an accept with no listener are all things a
// stub could report success for. A byte arriving at the other end is not.
//
// This is the test a hollow module fails. `zmodload -F zsh/net/socket
// b:zsocket` answering 0 costs nothing to fake.
func TestZsocketListensConnectsAcceptsAndCarriesAMessage(t *testing.T) {
	out, st := runZsh(t, socketDir(t), `zsocket -l sock
print -r -- "listen=$? usable=$(( REPLY > 2 ))"
listener=$REPLY
zsocket sock
print -r -- "connect=$?"
client=$REPLY
zsocket -a -t $listener
print -r -- "accept=$?"
server=$REPLY
print -u $client -- ping
read -u $server line
print -r -- "got=[$line]"`)
	want := "listen=0 usable=1\nconnect=0\naccept=0\ngot=[ping]\n"
	if out != want || st != 0 {
		t.Errorf("zsocket = %q (status %d), want %q", out, st, want)
	}
}

// **`-t` is the difference between asking and waiting, and it is silent.** A
// listener with nothing pending is status 1 and not a word, because a
// diagnostic there is a diagnostic a polling loop writes once a turn. The
// second half is the control: the same listener still has an answer when a
// connection *is* waiting, so the 1 is about the queue rather than about the
// descriptor.
func TestZsocketDashTAnswersWithoutWaiting(t *testing.T) {
	out, st := runZsh(t, socketDir(t), `zsocket -l sock
listener=$REPLY
zsocket -a -t $listener 2>&1
print -r -- "empty=$?"
zsocket sock
zsocket -a -t $listener 2>&1
print -r -- "pending=$?"
zsocket -a -t 33 2>&1
print -r -- "closed=$?"`)
	want := "empty=1\npending=0\nclosed=1\n"
	if out != want || st != 0 {
		t.Errorf("zsocket -a -t = %q (status %d), want %q", out, st, want)
	}
}

// `-d` puts the descriptor at a number the script chose, and `-v` says where
// it went — on standard output, because it is what the caller asked for rather
// than a complaint.
//
// The accept between the two connections is not tidiness. A listener here
// holds **one** unaccepted connection and refuses the next, which is measured
// and is what zsocketBacklog exists to reproduce; a second connection with the
// first still queued is `connection refused` rather than a second descriptor.
func TestZsocketPutsTheDescriptorWhereItIsToldAndSaysSoWithDashV(t *testing.T) {
	out, st := runZsh(t, socketDir(t), `zsocket -l sock
listener=$REPLY
zsocket -d 8 sock
print -r -- "targeted=$? REPLY=$REPLY"
zsocket -a -t -d 7 $listener
print -r -- "accepted=$? REPLY=$REPLY"
zsocket -v -d 9 sock`)
	want := "targeted=0 REPLY=8\naccepted=0 REPLY=7\nsock is now on fd 9\n"
	if out != want || st != 0 {
		t.Errorf("zsocket -d = %q (status %d), want %q", out, st, want)
	}
}

// **A relative name is resolved against the shell's directory**, the way every
// other relative name in the language is — see the same test for the file
// builtins, and shellpath.go for why the process's directory is not the
// shell's.
func TestZsocketFollowsTheShellsOwnDirectory(t *testing.T) {
	out, st := runZsh(t, socketDir(t), `zf_mkdir -p sub
cd sub
zsocket -l sock
print -r -- "bound=$? here=$([[ -e sock ]] && print yes || print no) up=$([[ -e ../sock ]] && print yes || print no)"`)
	want := "bound=0 here=yes up=no\n"
	if out != want || st != 0 {
		t.Errorf("the shell's directory = %q (status %d), want %q", out, st, want)
	}
}
