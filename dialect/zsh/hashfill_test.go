// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import "testing"

// **The command table is filled by listing PATH** — by `hash -f` and by any
// read of `$commands` — and a bare `hash` lists what is in it (#5266).
// Measured 2026-10-01 against zsh 5.9.2 with this script under `-c`, which
// writes these lines byte for byte: before a fill only the hand-hashed entry;
// after one, every name PATH holds, a directory and a file with no execute bit
// among them, the first directory winning a name both hold and the hand-hashed
// entry kept; `-f` refusing a name; an assignment to PATH emptying the table,
// and a read of `$commands` filling it again; and `hashexecutablesonly` set
// before the fill leaving the two out; and a write to `$commands`, its first
// touch, filling it as a read does.
func TestTheCommandTableIsFilledByListingPath(t *testing.T) {
	out, st := runZsh(t, t.TempDir(), `zmodload zsh/files
zf_mkdir -p p1/zdir p2
for f in p2/zab p2/zbar p2/zbaz p1/zfoo p1/zbar p1/zqux p1/zz; do print '#!/bin/sh' > $f; zf_chmod 755 $f; done
: > p1/znoexec
PATH=$PWD/p1:$PWD/p2
show() { hash | while IFS= read -r l; do print -r -- ${l//$PWD/D}; done; print -- --; }
hash zman=/bin/ls
show
hash -f
show
hash -fv zz; print "fv=$?"
PATH=$PATH; show
print -r -- ${(ko)commands[(I)z*]}
show
hash -r; setopt hashexecutablesonly
print -r -- ${(ko)commands[(I)z*]}
hash -r; unsetopt hashexecutablesonly; commands[zw]=/bin/ls; show`)
	if want := "zman=/bin/ls\n--\nzab=D/p2/zab\nzbar=D/p1/zbar\nzbaz=D/p2/zbaz\nzdir=D/p1/zdir\nzfoo=D/p1/zfoo\nzman=/bin/ls\nznoexec=D/p1/znoexec\nzqux=D/p1/zqux\nzz=D/p1/zz\n--\nzsh:hash:11: too many arguments\nfv=1\n--\nzab zbar zbaz zdir zfoo znoexec zqux zz\nzab=D/p2/zab\nzbar=D/p1/zbar\nzbaz=D/p2/zbaz\nzdir=D/p1/zdir\nzfoo=D/p1/zfoo\nznoexec=D/p1/znoexec\nzqux=D/p1/zqux\nzz=D/p1/zz\n--\nzab zbar zbaz zfoo zqux zz\nzab=D/p2/zab\nzbar=D/p1/zbar\nzbaz=D/p2/zbaz\nzdir=D/p1/zdir\nzfoo=D/p1/zfoo\nznoexec=D/p1/znoexec\nzqux=D/p1/zqux\nzw=/bin/ls\nzz=D/p1/zz\n--\n"; out != want || st != 0 {
		t.Errorf("got %q (status %d), want %q", out, st, want)
	}
}
