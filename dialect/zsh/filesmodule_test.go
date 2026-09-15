// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The `zsh/files` builtins, measured against zsh 5.9.2 (2026-09-09) with
// `zsh -f`. Whole rendered lines with their locations, because the location is
// half of what these messages say.

// filesTree is a directory with two files in it and nothing else.
func filesTree(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	for _, name := range []string{"a", "b"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(name), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

// The two a real startup calls, doing the work rather than existing.
//
// This is the test a module registered hollow fails. `zmodload -F zsh/files
// b:zf_mv b:zf_rm` answering 0 costs nothing to fake; a `zf_mv` that leaves the
// file at its new name and a `zf_rm -f` that takes it away again are the
// module.
func TestZfMvMovesAndZfRmRemoves(t *testing.T) {
	out, st := runZsh(t, filesTree(t), `zf_mv -f -- a moved
print -r -- "mv=$? a=$([[ -e a ]] && print yes || print no) moved=$([[ -e moved ]] && print yes || print no)"
zf_rm -f -- moved
print -r -- "rm=$? moved=$([[ -e moved ]] && print yes || print no)"`)
	want := "mv=0 a=no moved=yes\nrm=0 moved=no\n"
	if out != want || st != 0 {
		t.Errorf("zf_mv and zf_rm = %q (status %d), want %q", out, st, want)
	}
}

// **`-f` suppresses the complaint about a name that is not there**, which is
// the whole reason a prompt theme writes `zf_rm -f -- $tmp` whether or not it
// got as far as creating the file. Without it the same line is a diagnostic
// and a status.
func TestZfRmDashFIsSilentAboutWhatWasNeverThere(t *testing.T) {
	out, st := runZsh(t, filesTree(t), `zf_rm nosuch 2>&1
print -r -- "plain=$?"
zf_rm -f nosuch 2>&1
print -r -- "forced=$?"
zf_rm 2>&1
print -r -- "none=$?"
zf_rm -f 2>&1
print -r -- "forced-none=$?"`)
	want := "zsh:zf_rm:1: nosuch: no such file or directory\nplain=1\n" +
		"forced=0\n" +
		"zsh:zf_rm:5: not enough arguments\nnone=1\n" +
		"zsh:zf_rm:7: not enough arguments\nforced-none=1\n"
	if out != want || st != 0 {
		t.Errorf("zf_rm -f = %q (status %d), want %q", out, st, want)
	}
}

// A directory is refused until `-r` asks for it, and then everything below it
// goes before it does.
func TestZfRmDescendsOnlyWhenAskedTo(t *testing.T) {
	out, st := runZsh(t, filesTree(t), `zf_mkdir -p tree/under
zf_mv -f -- a tree/under/deep
zf_rm tree 2>&1
print -r -- "plain=$?"
zf_rm -r tree
print -r -- "recursive=$? left=$([[ -e tree ]] && print yes || print no)"`)
	want := "zsh:zf_rm:3: tree: is a directory\nplain=1\n" +
		"recursive=0 left=no\n"
	if out != want || st != 0 {
		t.Errorf("zf_rm -r = %q (status %d), want %q", out, st, want)
	}
}

// `mkdir` and `rmdir`, including the mode that survives the umask and the two
// sentences each of them has for a name it cannot use.
func TestZfMkdirAndZfRmdir(t *testing.T) {
	out, st := runZsh(t, filesTree(t), `zf_mkdir made
print -r -- "made=$?"
zf_mkdir made 2>&1
print -r -- "again=$?"
zf_mkdir -p made
print -r -- "parents=$?"
zf_mkdir -p a 2>&1
print -r -- "over-a-file=$?"
zf_mkdir -m 700 walled
zstat -s +mode walled
zf_mkdir -m zzz bad 2>&1
print -r -- "bad-mode=$?"
zf_rmdir made
print -r -- "removed=$?"
zf_rmdir made 2>&1
print -r -- "gone=$?"
zf_rmdir a 2>&1
print -r -- "not-a-directory=$?"
zf_rmdir 2>&1
print -r -- "none=$?"`)
	want := "made=0\n" +
		"zsh:zf_mkdir:3: cannot make directory `made': file exists\nagain=1\n" +
		"parents=0\n" +
		"zsh:zf_mkdir:7: cannot make directory `a': file exists\nover-a-file=1\n" +
		"drwx------\n" +
		"zsh:zf_mkdir:11: invalid mode `zzz'\nbad-mode=1\n" +
		"removed=0\n" +
		"zsh:zf_rmdir:15: cannot remove directory `made': no such file or directory\ngone=1\n" +
		"zsh:zf_rmdir:17: cannot remove directory `a': not a directory\nnot-a-directory=1\n" +
		"zsh:zf_rmdir:19: not enough arguments\nnone=1\n"
	if out != want || st != 0 {
		t.Errorf("zf_mkdir and zf_rmdir = %q (status %d), want %q", out, st, want)
	}
}

// A link is made, and **never replaces a name that is already there** unless
// `-f` says so. A symbolic link holds the text it was given rather than where
// that text resolved to, which is what makes a relative link relative.
func TestZfLnLinksAndWillNotReplaceByDefault(t *testing.T) {
	out, st := runZsh(t, filesTree(t), `zf_ln -s a link
print -r -- "made=$? points=$(zstat -L +link link)"
zf_ln -s a link 2>&1
print -r -- "again=$?"
zf_ln -sf b link
print -r -- "forced=$? points=$(zstat -L +link link)"
zf_ln -s nosuch dangling
print -r -- "dangling=$? points=$(zstat -L +link dangling)"
zf_ln nosuch hard 2>&1
print -r -- "hard=$?"
zf_ln 2>&1
print -r -- "none=$?"`)
	want := "made=0 points=a\n" +
		"zsh:zf_ln:3: `a': file exists\nagain=1\n" +
		"forced=0 points=b\n" +
		"dangling=0 points=nosuch\n" +
		"zsh:zf_ln:9: nosuch: no such file or directory\nhard=1\n" +
		"zsh:zf_ln:11: not enough arguments\nnone=1\n"
	if out != want || st != 0 {
		t.Errorf("zf_ln = %q (status %d), want %q", out, st, want)
	}
}

// Several sources and a directory to put them in, and the refusal when the
// last operand is not one.
func TestZfMvAndZfLnTakeADirectoryForTheirLastOperand(t *testing.T) {
	out, st := runZsh(t, filesTree(t), `zf_mkdir into
zf_mv a b into
print -r -- "moved=$? a=$([[ -e into/a ]] && print yes || print no) b=$([[ -e into/b ]] && print yes || print no)"
zf_mv into/a into/b into/a 2>&1
print -r -- "not-a-directory=$?"
zf_mv one 2>&1
print -r -- "one=$?"`)
	want := "moved=0 a=yes b=yes\n" +
		"zsh:zf_mv:4: last of many arguments must be a directory\nnot-a-directory=1\n" +
		"zsh:zf_mv:6: not enough arguments\none=1\n"
	if out != want || st != 0 {
		t.Errorf("a directory operand = %q (status %d), want %q", out, st, want)
	}
}

// The mode `chmod` takes is octal and nothing else, and `-R` reaches
// everything under a directory.
func TestZfChmodTakesAnOctalModeAndReachesDownWithDashR(t *testing.T) {
	out, st := runZsh(t, filesTree(t), `zf_chmod 640 a
zstat -s +mode a
zf_chmod u+x a 2>&1
print -r -- "symbolic=$?"
zf_chmod 999 a 2>&1
print -r -- "nine=$?"
zf_chmod 600 nosuch 2>&1
print -r -- "missing=$?"
zf_chmod 600 2>&1
print -r -- "no-files=$?"
zf_mkdir -p tree/under
zf_mv -f -- b tree/under/deep
zf_chmod -R 700 tree
zstat -s +mode tree
zstat -s +mode tree/under/deep`)
	want := "-rw-r-----\n" +
		"zsh:zf_chmod:3: invalid mode `u+x'\nsymbolic=1\n" +
		"zsh:zf_chmod:5: invalid mode `999'\nnine=1\n" +
		"zsh:zf_chmod:7: nosuch: no such file or directory\nmissing=1\n" +
		"zsh:zf_chmod:9: not enough arguments\nno-files=1\n" +
		"drwx------\n-rwx------\n"
	if out != want || st != 0 {
		t.Errorf("zf_chmod = %q (status %d), want %q", out, st, want)
	}
}

// **The paranoid letter refuses a path reached through a symbolic link**, and
// leaves everything else exactly as it was. See filesparanoid.go for the whole
// measured table and for why both of the manual's promises are kept by holding
// each directory open rather than by naming it again.
func TestTheParanoidLetterRefusesASymlinkedComponent(t *testing.T) {
	dir := filesParanoidTree(t)
	for _, tc := range []struct{ name, src, want string }{
		{
			// The manual's own example, and the whole of what the letter is
			// measured to change: the link is `link`, not `passwd`.
			"a link on the way to the name", `zf_rm -s link/passwd 2>&1
print -r -- "st=$? passwd=$([[ -e real/passwd ]] && print yes || print no)"`,
			"zsh:zf_rm:1: link/passwd: not a directory\nst=1 passwd=yes\n",
		},
		{
			// The control, and it is the row that says the letter is doing
			// something: the same line without it removes the file.
			"and without the letter it is removed", `zf_rm link/passwd
print -r -- "st=$? passwd=$([[ -e real/passwd ]] && print yes || print no)"`,
			"st=0 passwd=no\n",
		},
		{
			// `-f` silences it, as it silences everything.
			"the force letter silences it", `zf_rm -s -f link/passwd
print -r -- "st=$? passwd=$([[ -e real/passwd ]] && print yes || print no)"`,
			"st=0 passwd=yes\n",
		},
		{
			// A path with no link in it is unaffected.
			"a real path is removed", `zf_rm -s real/passwd
print -r -- "st=$? passwd=$([[ -e real/passwd ]] && print yes || print no)"`,
			"st=0 passwd=no\n",
		},
		{
			// The **final** component is followed exactly as it is without
			// the letter: a link there is unlinked by `rm` and followed by
			// `chmod`, which is measured and is where `-s` stops.
			"the last component is the link itself", `zf_rm -s link
print -r -- "st=$? link=$([[ -L link ]] && print yes || print no) real=$([[ -d real ]] && print yes || print no)"`,
			"st=0 link=no real=yes\n",
		},
		{
			// The last operand is the discriminating one: it **is** a link,
			// and `chmod` follows it. A walk that went through the held
			// directory for this call would answer `path escapes from
			// parent` — os.Root refuses a link that leaves the root — where
			// the shell it models changes the mode of what the link points
			// at.
			"chmod follows the last component", `zf_chmod -s 700 link/passwd 2>&1
print -r -- "through=$?"
zf_chmod -s 700 real/passwd
print -r -- "direct=$?"
zf_chmod -s 700 link
print -r -- "link=$?"`,
			"zsh:zf_chmod:1: link/passwd: not a directory\nthrough=1\ndirect=0\nlink=0\n",
		},
		{
			"and so do chown and chgrp", `zf_chown -s :0 link/passwd 2>&1
print -r -- "chown=$?"
zf_chgrp -s 0 link/passwd 2>&1
print -r -- "chgrp=$?"`,
			"zsh:zf_chown:1: link/passwd: not a directory\nchown=1\n" +
				"zsh:zf_chgrp:3: link/passwd: not a directory\nchgrp=1\n",
		},
		{
			// A recursive removal is unchanged: the link inside the tree is
			// unlinked rather than descended, which the walk did already,
			// and what it pointed at survives.
			"a recursive removal under the letter", `zf_rm -s -r tree
print -r -- "st=$? tree=$([[ -e tree ]] && print yes || print no) kept=$([[ -e outside/keepme ]] && print yes || print no)"`,
			"st=0 tree=no kept=yes\n",
		},
		{
			// And `ln`'s own `-s` is untouched: the letter means a symbolic
			// link there and the paranoid descent nowhere near it.
			"ln keeps its own -s", `zf_ln -s real/passwd made
print -r -- "st=$? link=$([[ -L made ]] && print yes || print no)"`,
			"st=0 link=yes\n",
		},
		{
			// A typo is still a typo.
			"a letter that is not there", `zf_rm -Q real/passwd 2>&1
print -r -- "typo=$?"`,
			"zsh:zf_rm:1: bad option: -Q\ntypo=1\n",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := runZsh(t, filesParanoidCopy(t, dir), tc.src)
			if out != tc.want || st != 0 {
				t.Errorf("%s = %q (status %d), want %q", tc.src, out, st, tc.want)
			}
		})
	}
}

// **A symbolic link is never asked about**, which is the bug the paranoid
// rows above could not be measured past.
//
// Measured 2026-09-15 on zsh 5.9.2: `zf_rm` over a link to a mode-0444 file is
// a silent 0 and over a *dangling* link the same, where the file it points at
// earns the query. `access` follows the link, so a dangling one answered "not
// writable" and every recursive removal over a tree holding one stopped to ask
// a question no script could answer — and then failed with `directory not
// empty`, because the entry it asked about was still there.
func TestARemovalNeverAsksAboutASymbolicLink(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "target"), []byte("t"), 0o444); err != nil {
		t.Fatal(err)
	}
	for name, at := range map[string]string{"live": "target", "dangling": "no/such/thing"} {
		if err := os.Symlink(at, filepath.Join(dir, name)); err != nil {
			t.Fatal(err)
		}
	}
	out, st := runZsh(t, dir, `zf_rm live dangling
print -r -- "st=$? live=$([[ -L live ]] && print yes || print no) dangling=$([[ -L dangling ]] && print yes || print no)"`)
	if want := "st=0 live=no dangling=no\n"; out != want || st != 0 {
		t.Errorf("removing links = %q (status %d), want %q", out, st, want)
	}
	// The control: the file the live link pointed at is unwritable and *is*
	// asked about, so the row above is about links and not about the query
	// having been taken out.
	out, _ = runZsh(t, dir, "zf_rm target </dev/null 2>&1")
	if !strings.Contains(out, "overriding mode 0444") {
		t.Errorf("removing the file itself = %q, want the query about its mode", out)
	}
}

// realTempDir is t.TempDir with the symbolic links taken out of it, which
// these rows need and nothing else in this file does.
//
// On this platform a temporary directory is under `/var`, and `/var` is a link
// to `/private/var` — so **every** operand under one is a path reached through
// a symbolic link, and `-s` refuses the lot. That is the letter working, and
// the real shell does the same: `zf_rm -s /tmp/x` is `not a directory` on this
// machine because `/tmp` is a link too. It is written down here because the
// first draft of these rows read the refusal as a bug in the walk.
func realTempDir(t *testing.T) string {
	t.Helper()
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return dir
}

// filesParanoidTree is the tree the paranoid rows are measured over: a real
// directory, a symbolic link to it, and a tree holding a link that points out
// of itself.
func filesParanoidTree(t *testing.T) string {
	t.Helper()
	dir := realTempDir(t)
	for _, sub := range []string{"real", "outside", "tree/sub"} {
		if err := os.MkdirAll(filepath.Join(dir, sub), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	for _, f := range []string{"real/passwd", "outside/keepme", "tree/sub/file"} {
		if err := os.WriteFile(filepath.Join(dir, f), []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Symlink(filepath.Join(dir, "real"), filepath.Join(dir, "link")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("../outside", filepath.Join(dir, "tree", "esc")); err != nil {
		t.Fatal(err)
	}
	return dir
}

// filesParanoidCopy gives each row a tree of its own, because these rows
// remove things.
//
// A copy rather than a fresh build, so every row starts from the one tree the
// measurements were taken over — and `cp -R`'s own handling of links is not
// trusted for it: the links are made again by name.
func filesParanoidCopy(t *testing.T, from string) string {
	t.Helper()
	to := realTempDir(t)
	if err := os.CopyFS(to, os.DirFS(from)); err != nil {
		// CopyFS follows links, which is exactly why the links are remade
		// below rather than copied: what it leaves behind for `link` and
		// `esc` is whatever they pointed at.
		t.Fatal(err)
	}
	for _, name := range []string{"link", filepath.Join("tree", "esc")} {
		if err := os.RemoveAll(filepath.Join(to, name)); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Symlink(filepath.Join(to, "real"), filepath.Join(to, "link")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("../outside", filepath.Join(to, "tree", "esc")); err != nil {
		t.Fatal(err)
	}
	return to
}

// `chown` and `chgrp` name what they could not find, and `sync` takes nothing
// at all.
func TestZfChownAndZfChgrpAndZfSync(t *testing.T) {
	out, st := runZsh(t, filesTree(t), `zf_chown nosuchuser a 2>&1
print -r -- "user=$?"
zf_chgrp nosuchgroup a 2>&1
print -r -- "group=$?"
zf_chown 2>&1
print -r -- "none=$?"
zf_sync
print -r -- "sync=$?"
zf_sync extra 2>&1
print -r -- "extra=$?"`)
	want := "zsh:zf_chown:1: nosuchuser: no such user\nuser=1\n" +
		"zsh:zf_chgrp:3: nosuchgroup: no such group\ngroup=1\n" +
		"zsh:zf_chown:5: not enough arguments\nnone=1\n" +
		"sync=0\n" +
		"zsh:zf_sync:9: too many arguments\nextra=1\n"
	if out != want || st != 0 {
		t.Errorf("zf_chown and friends = %q (status %d), want %q", out, st, want)
	}
}

// **A relative name is resolved against the shell's directory and not the
// process's**, which are two different directories the moment a script writes
// `cd`. A builtin that reached for the process's would move and unlink files
// beside whatever program embedded this shell.
func TestTheFileBuiltinsFollowTheShellsOwnDirectory(t *testing.T) {
	out, st := runZsh(t, filesTree(t), `zf_mkdir -p sub
zf_mv -f -- a sub/inner
cd sub
zf_mv -f -- inner renamed
print -r -- "moved=$? here=$([[ -e renamed ]] && print yes || print no)"
zf_mv -f -- ../b .
print -r -- "up=$? b=$([[ -e b ]] && print yes || print no) gone=$([[ -e ../b ]] && print no || print yes)"
zf_rm -f -- renamed b
print -r -- "removed=$? left=$([[ -e renamed ]] && print yes || print no)"`)
	want := "moved=0 here=yes\nup=0 b=yes gone=yes\nremoved=0 left=no\n"
	if out != want || st != 0 {
		t.Errorf("the shell's directory = %q (status %d), want %q", out, st, want)
	}
}

// The query, which is the default for a file the shell cannot write to and
// what `-i` asks for about every file. It goes to standard error with no
// newline after it, and an empty answer is a no.
//
// **Not for a user who can write everything.** The default query is about
// *this* user lacking write permission, and the superuser never does, so for
// root the case under test does not exist — measured 2026-09-10 on Debian 12
// against real zsh and against this shell, and they agree twice: as root both
// remove a mode-0400 file silently, and as an ordinary user both ask
// “remove `a', overriding mode 0400?“. So the skip is a statement about
// which user the behavior belongs to rather than a platform this shell is
// wrong on. It matters because a container runs tests as root by default,
// which is how this was found.
func TestZfRmAsksBeforeRemovingWhatItCannotWrite(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("the superuser can write a mode-0400 file, so there is nothing to ask about")
	}
	dir := filesTree(t)
	if err := os.Chmod(filepath.Join(dir, "a"), 0o400); err != nil {
		t.Fatal(err)
	}
	out, st := runZsh(t, dir, `print n | zf_rm a 2>&1
print -r -- "refused=$? a=$([[ -e a ]] && print yes || print no)"
print y | zf_rm a 2>&1
print -r -- "agreed=$? a=$([[ -e a ]] && print yes || print no)"
print n | zf_rm -i b 2>&1
print -r -- "interactive=$? b=$([[ -e b ]] && print yes || print no)"
zf_rm -f b < /dev/null
print -r -- "forced=$? b=$([[ -e b ]] && print yes || print no)"`)
	want := "zf_rm: remove `a', overriding mode 0400? refused=0 a=yes\n" +
		"zf_rm: remove `a', overriding mode 0400? agreed=0 a=no\n" +
		"zf_rm: remove `b'? interactive=0 b=yes\n" +
		"forced=0 b=no\n"
	if out != want || st != 0 {
		t.Errorf("the query = %q (status %d), want %q", out, st, want)
	}
}
