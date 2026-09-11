// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"fmt"
	"os"
	"os/user"
	"path/filepath"
	"strconv"
	"syscall"
	"testing"
)

// modeDir holds one regular file per mode the access-rights qualifier has to
// tell apart, named after the mode so a failure reads as a list of modes.
//
// Regular files only, and no directory among them: `f` is about the twelve
// mode bits and a fixture that also varied the type would make every row
// answer two questions at once. The sticky bit lives in permissionDir, on the
// directory that can carry it on every platform this suite runs on.
func modeDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	for _, mode := range []os.FileMode{
		0o000, 0o600, 0o644, 0o664, 0o666, 0o700, 0o755,
	} {
		writeMode(t, dir, fmt.Sprintf("m%04o", mode), mode)
	}
	writeMode(t, dir, "m4755", 0o755|os.ModeSetuid)
	return dir
}

// writeMode creates one file and puts it at a mode, with the chmod separate
// from the create because the create is subject to a umask and the answer
// this fixture needs is the exact bits.
func writeMode(t *testing.T, dir, name string, mode os.FileMode) {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(p, mode); err != nil {
		t.Fatal(err)
	}
	info, err := os.Lstat(p)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode() != mode {
		// A filesystem that will not hold the bits asked for would make every
		// row below read as a bug in the shell.
		t.Fatalf("%s is %v, want %v: this filesystem will not hold the fixture", name, info.Mode(), mode)
	}
}

// `f` is access rights, and its argument comes in two spellings.
//
// Measured on zsh 5.9.2, 2026-09-10, against exactly the directory modeDir
// builds. The rows are grouped by what they establish rather than by
// spelling, because the spellings share an evaluator and the questions do
// not.
func TestTheAccessRightsQualifier(t *testing.T) {
	dir := modeDir(t)
	for _, tc := range []struct{ name, list, want string }{
		// A number on its own is an exact comparison, and it compares as
		// many digits as were written: `755` leaves the set-user-ID file in
		// and `0755` takes it out. That pair is the whole of the rule.
		{"a number is exact", "f0666", "[m0666]"},
		{"three digits compare three", "f755", "[m0755][m4755]"},
		{"four digits compare four", "f0755", "[m0755]"},
		{"an equals sign is the same thing", "f=644", "[m0644]"},
		{"and a question mark asks nothing of a digit", "f70?", "[m0700]"},
		{"wherever it stands", "f?55", "[m0755][m4755]"},
		// `+` is every bit and `-` is no bit, which needs a number the
		// fixture holds partially: 0666 has two bits of 0700 and is left out
		// by both.
		{"a plus is every bit of the number", "f+022", "[m0666]"},
		{"and a partial overlap is not enough", "f+0700", "[m0700][m0755][m4755]"},
		{"a minus is no bit of it", "f-022", "[m0000][m0600][m0644][m0700][m0755][m4755]"},
		{"and a partial overlap is still too many", "f-0700", "[m0000]"},
		{"a number with no digits asks nothing", "f=", "[m0000][m0600][m0644][m0664][m0666][m0700][m0755][m4755]"},
		// The delimited form, which is what the completion system writes.
		{"a chmod-style clause", "f:g+w:", "[m0664][m0666]"},
		{"and the other half of the pair", "f:o+w:", "[m0666]"},
		{"two clauses are an and", "f:g+w,o+w:", "[m0666]"},
		{"a class may be a list", "f:ug+w:", "[m0664][m0666]"},
		{"and `a` is all three", "f:a-w:", "[m0000]"},
		{"an equals compares the class's whole triple", "f:u=rw:", "[m0600][m0644][m0664][m0666]"},
		{"set-user-ID inside it", "f:u=rwx:", "[m0700][m0755]"},
		{"and named", "f:u=rwxs:", "[m4755]"},
		{"`s` is the class's own bit", "f:u+s:", "[m4755]"},
		{"and a class without one asks nothing", "f:g+t:", "[m0000][m0600][m0644][m0664][m0666][m0700][m0755][m4755]"},
		{"an octal digit stands for a triple", "f:u+7:", "[m0700][m0755][m4755]"},
		{"an empty permission list asks nothing", "f:u+:", "[m0000][m0600][m0644][m0664][m0666][m0700][m0755][m4755]"},
		{"and an empty one under `=` asks for none", "f:u=:", "[m0000]"},
		// The operator may be left out, and leaving it out is `=`: `f:u:`
		// is the same question as `f:u=:`.
		{"a class with no operator is that `=`", "f:u:", "[m0000]"},
		{"and it is about that class alone", "f:g:", "[m0000][m0600][m0700]"},
		{"`a` included", "f:a:", "[m0000]"},
		{"and it composes with a clause beside it", "f:u:,f:g+w:", "[m0000][m0664][m0666]"},
		{"a number may be the last sub-spec", "f:u+w,+022:", "[m0666]"},
		{"the delimiter is whatever follows the letter", "f{g+w}", "[m0664][m0666]"},
		{"and a bracket closes with its partner", "f[g+w]", "[m0664][m0666]"},
		// The qualifier list carries on where the argument ends, which is
		// what says the number is read rather than the rest of the list.
		{"the list resumes behind the number", "f644.", "[m0644]"},
		{"and behind the delimiters", "f:g+w:.", "[m0664][m0666]"},
		{"a caret still turns it", "^f644", "[m0000][m0600][m0664][m0666][m0700][m0755][m4755]"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := runQualified(t, dir, `printf "[%s]" *(`+tc.list+`)`)
			if out != tc.want || st != 0 {
				t.Errorf("*(%s) = %q (status %d), want %q at 0", tc.list, out, st, tc.want)
			}
		})
	}
}

// Every way `f`'s argument can fail to read, and they are one sentence.
//
// The wording is the shell's and it names nothing, which is the point of
// asserting it: `invalid mode specification` is what a reader gets, so a
// refusal that named a character instead would be a different shell's
// message.
func TestAnUnreadableModeSpecIsRefused(t *testing.T) {
	dir := modeDir(t)
	for _, list := range []string{
		"f",           // nothing to delimit with
		"f:",          // and nothing between the delimiters
		"f:g+w",       // a delimiter with no partner
		"f:x+w:",      // `x` is not a class
		"f:+w:",       // and a clause with no class at all is not `a`
		"f:u+X:",      // `X` is not a permission
		"f:u+8:",      // nor is a digit outside octal
		"f:u+w-x:",    // one operator to a clause
		"f:g=u:",      // and no copying one class's bits to another
		"f:,g+w:",     // an empty sub-spec
		"f:g+w,:",     // at either end
		"f:+022,u+w:", // a number that is not the last sub-spec
		"f:755,644:",  // even when what follows it is another number
	} {
		out, st := runQualified(t, dir, `printf "[%s]" *(`+list+`); echo after`)
		if !containsSub(out, "invalid mode specification") || st == 0 {
			t.Errorf("*(%s) = %q (status %d), want the mode spec refused", list, out, st)
		}
		if containsSub(out, "after") {
			t.Errorf("*(%s) = %q, want the failure fatal", list, out)
		}
	}
}

// ownerOf is the uid and gid a file in dir really has.
//
// Asked of the fixture rather than of this process, because the two are not
// the same question: on the BSDs a new file takes the *directory's* group, so
// a test that compared against getegid would be asserting something false of
// half the platforms it runs on — measured, a file made in /tmp on macOS is
// group `wheel` where the process is group `staff`.
func ownerOf(t *testing.T, path string) (uid, gid uint32) {
	t.Helper()
	info, err := os.Lstat(path)
	if err != nil {
		t.Fatal(err)
	}
	st, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		t.Fatalf("%s: no stat fields on this platform", path)
	}
	return st.Uid, st.Gid
}

// `u` and `g` are ownership, and `U` and `G` are the same two questions with
// this process's own ids filled in.
//
// Every row is written against the ids the fixture really has, so the test
// says the same thing for a developer, for a CI runner and for a container
// running as root — none of which have the same uid, and the last of which
// would make a hard-coded `u0` mean the opposite of what it means elsewhere.
func TestTheOwnershipQualifiers(t *testing.T) {
	dir := modeDir(t)
	uid, gid := ownerOf(t, filepath.Join(dir, "m0644"))
	all := "[m0000][m0600][m0644][m0664][m0666][m0700][m0755][m4755]"

	for _, tc := range []struct{ name, list, want string }{
		{"a uid the files have", fmt.Sprintf("u%d", uid), all},
		{"one they do not", fmt.Sprintf("u%d", uid+1), ""},
		{"a caret turns it", fmt.Sprintf("^u%d", uid), ""},
		{"and the other one round", fmt.Sprintf("^u%d", uid+1), all},
		// The shape the completion system writes: `^u0u${EUID}` is *neither*
		// root nor me, because the caret turns everything after it in the
		// section rather than only the qualifier it stands in front of.
		{"a caret turns both of two", fmt.Sprintf("^u%du%d", uid+1, uid), ""},
		{"and a second one turns them back", fmt.Sprintf("^u%d^u%d", uid+1, uid), all},
		{"a gid the files have", fmt.Sprintf("g%d", gid), all},
		{"one they do not", fmt.Sprintf("g%d", gid+1), ""},
		{"the effective user", "U", all},
		// A number so long it is no id at all is read and matches nothing,
		// rather than being a refusal: the qualifier was written correctly.
		{"a number no file can carry", "u99999999999999999999999", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := runQualified(t, dir, `printf "[%s]" *(`+tc.list+`)`)
			if tc.want == "" {
				if !containsSub(out, "no matches found: *("+tc.list+")") || st == 0 {
					t.Errorf("*(%s) = %q (status %d), want a fatal miss naming the word", tc.list, out, st)
				}
				if containsSub(out, "unknown file attribute") {
					t.Errorf("*(%s) = %q, want the letter claimed rather than refused", tc.list, out)
				}
				return
			}
			if out != tc.want || st != 0 {
				t.Errorf("*(%s) = %q (status %d), want %q at 0", tc.list, out, st, tc.want)
			}
		})
	}

	// `G` is the same assertion where the two agree, and the honest one
	// where they do not: a file's group is the directory's on the BSDs.
	t.Run("the effective group", func(t *testing.T) {
		out, st := runQualified(t, dir, `printf "[%s]" *(G)`)
		if gid == uint32(os.Getegid()) {
			if out != all || st != 0 {
				t.Errorf("*(G) = %q (status %d), want %q at 0", out, st, all)
			}
			return
		}
		if !containsSub(out, "no matches found: *(G)") || st == 0 {
			t.Errorf("*(G) = %q (status %d), want a fatal miss", out, st)
		}
	})
}

// The delimited spelling of `u` and `g` is a *name*, always — and the two
// ways it can fail are two different sentences.
//
// On a goroutine with a deadline because a name lookup is the one thing here
// that leaves the process: it reads a directory service, and a container
// whose resolver is unreachable would hang rather than fail. Twenty seconds
// is far past any real answer and far inside the package's own timeout, so a
// blocked lookup reports as this test rather than as a suite that never
// finished.
func TestAnOwnerMayBeNamed(t *testing.T) {
	dir := modeDir(t)
	uid, _ := ownerOf(t, filepath.Join(dir, "m0644"))
	all := "[m0000][m0600][m0644][m0664][m0666][m0700][m0755][m4755]"

	deadline(t, "looking a user up by name", func() {
		me, err := userNameOf(uid)
		if err != nil {
			t.Skipf("this uid has no name here: %v", err)
		}
		out, st := runQualified(t, dir, `printf "[%s]" *(u:`+me+`:)`)
		if out != all || st != 0 {
			t.Errorf("*(u:%s:) = %q (status %d), want %q at 0", me, out, st, all)
		}
		// The brackets are a delimiter like any other, and what is inside
		// them is a name even when it reads as a number.
		out, st = runQualified(t, dir, `printf "[%s]" *(u[`+me+`])`)
		if out != all || st != 0 {
			t.Errorf("*(u[%s]) = %q (status %d), want %q at 0", me, out, st, all)
		}
		// An operator delimits like anything else, which is not what the
		// three refusals below look like: they are missing *partners*.
		out, st = runQualified(t, dir, `printf "[%s]" *(u+`+me+`+)`)
		if out != all || st != 0 {
			t.Errorf("*(u+%s+) = %q (status %d), want %q at 0", me, out, st, all)
		}
		out, st = runQualified(t, dir, `printf "[%s]" *(u+0+); echo after`)
		if !containsSub(out, "unknown username '0'") || st == 0 || containsSub(out, "after") {
			t.Errorf("*(u+0+) = %q (status %d), want the number refused as a name, fatally", out, st)
		}
		out, st = runQualified(t, dir, `printf "[%s]" *(u[0]); echo after`)
		if !containsSub(out, "unknown username '0'") || st == 0 || containsSub(out, "after") {
			t.Errorf("*(u[0]) = %q (status %d), want the number refused as a name, fatally", out, st)
		}
	})

	// The two complaints that do not need a lookup at all.
	for _, tc := range []struct{ list, want string }{
		{"u", "missing delimiter for 'u' glob qualifier"},
		{"uu", "missing delimiter for 'u' glob qualifier"},
		{"u+5", "missing delimiter for 'u' glob qualifier"},
		{"u:x", "missing delimiter for 'u' glob qualifier"},
		{"g", "missing delimiter for 'g' glob qualifier"},
		{"g=5", "missing delimiter for 'g' glob qualifier"},
	} {
		out, st := runQualified(t, dir, `printf "[%s]" *(`+tc.list+`); echo after`)
		if !containsSub(out, tc.want) || st == 0 || containsSub(out, "after") {
			t.Errorf("*(%s) = %q (status %d), want %q, fatally", tc.list, out, st, tc.want)
		}
	}
}

// userNameOf is the login name for a uid, which is looked up here rather than
// asked of the environment: $USER is what a session was started as and the
// files below are owned by whoever the tests run as.
func userNameOf(uid uint32) (string, error) {
	u, err := user.LookupId(strconv.FormatUint(uint64(uid), 10))
	if err != nil {
		return "", err
	}
	return u.Username, nil
}

// linkDir is a directory built to separate a link from what it points at: a
// regular file and a directory, a link to each, and one link to nothing.
//
// The dangling link is the row that says what "follow" means when following
// fails, and it has to be a name the walk can still find — a broken link is
// still a directory entry.
func linkDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	writeMode(t, dir, "f1", 0o666)
	if err := os.Mkdir(filepath.Join(dir, "d1"), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, l := range [][2]string{{"f1", "la"}, {"d1", "ld"}, {"nowhere", "dangle"}} {
		if err := os.Symlink(l[0], filepath.Join(dir, l[1])); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

// `-` is not an attribute: it says the qualifiers after it ask about what a
// symbolic link points at.
//
// Measured on zsh 5.9.2, 2026-09-10, against exactly the directory linkDir
// builds. Reading it as an attribute name is what the completion system's
// `(N-f:g+w:,-f:o+w:,-^u0u$EUID)` ran into, and the wording it produced —
// `unknown file attribute: -` — is what said the character had been looked up
// in the wrong table.
func TestTheFollowToggleAsksAboutTheTarget(t *testing.T) {
	dir := linkDir(t)
	for _, tc := range []struct{ name, list, want string }{
		{"everything, for the rows below to be read against", "N", "[d1][dangle][f1][la][ld]"},
		{"a link is a link by its own type", "N@", "[dangle][la][ld]"},
		// Following leaves only the one whose target cannot be reached,
		// which is the two halves of the toggle in a single row.
		{"and following leaves the one that goes nowhere", "N-@", "[dangle]"},
		{"a regular file is the file", "N.", "[f1]"},
		{"and following adds the link to one", "N-.", "[f1][la]"},
		{"a directory the same way", "N-/", "[d1][ld]"},
		// A second one turns it back off, which is what makes this a toggle
		// rather than a flag.
		{"twice is not at all", "N--.", "[f1]"},
		// It applies to what follows it and not to what precedes it.
		// `N` is on every row here, so an empty answer is the deleted word
		// and not a miss: `.` leaves the regular file and `-@` then asks
		// whether it is a link, which it is not.
		{"what stands in front of it is not followed", "N.-@", "[]"},
		{"a caret and it do not consume each other", "N^-.", "[d1][dangle][ld]"},
		{"in either order", "N-^.", "[d1][dangle][ld]"},
		// And it is read again from nothing in the next section: the second
		// section here does *not* follow, so the two links it leaves out of
		// the first come back.
		{"a comma reads it again from nothing", "N-@,@", "[dangle][la][ld]"},
		// `f` as well as the type tests, and the clause asks about the
		// *owner* rather than the group on purpose: a symbolic link's own
		// permission bits are 0777 on Linux and 0755 on the BSDs, so a
		// group-write question about the link would be a question about the
		// platform. Owner `rw` exactly is neither of those and is the mode
		// `f1` carries.
		{"it reaches the argument-taking qualifiers too", "N-f:u=rw:", "[f1][la]"},
		{"and on its own it narrows nothing", "N-", "[d1][dangle][f1][la][ld]"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := runQualified(t, dir, `printf "[%s]" *(`+tc.list+`)`)
			if tc.want == "" {
				if !containsSub(out, "no matches found") || st == 0 {
					t.Errorf("*(%s) = %q (status %d), want a fatal miss", tc.list, out, st)
				}
				return
			}
			if out != tc.want || st != 0 {
				t.Errorf("*(%s) = %q (status %d), want %q at 0", tc.list, out, st, tc.want)
			}
		})
	}
}

// A qualifier list may end in modifiers, and they apply to every name.
//
// Measured on zsh 5.9.2, 2026-09-10, against a directory holding `sub/x.txt`
// and `sub/y.md`. The names are lower case on purpose: this shell sorts
// matches by byte where that one collates, which is a difference of its own
// and not one this test should be asserting either way.
func TestAQualifierListMayEndInModifiers(t *testing.T) {
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, n := range []string{"x.txt", "y.md"} {
		if err := os.WriteFile(filepath.Join(dir, "sub", n), nil, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	for _, tc := range []struct{ name, src, want string }{
		{"the tail of each name", `*/*(N:t)`, "[x.txt][y.md]"},
		{"a chain, left to right", `*/*(N:t:r)`, "[x][y]"},
		// The answer is sorted *after* the modifiers rather than before:
		// these two arrive as `x.txt y.md` and leave as `md txt`.
		{"and the answer is re-sorted", `*/*(N:e)`, "[md][txt]"},
		{"a count reaches the letter", `*/*(N:h1)`, "[sub][sub]"},
		{"a substitution too", `*/*(N:s/x/Q/)`, "[sub/Q.txt][sub/y.md]"},
		// And `g` in front of it is every occurrence, which is why the `x`
		// inside `txt` goes as well.
		{"and `g` makes it every occurrence", `*/*(N:gs/x/Q/)`, "[sub/Q.tQt][sub/y.md]"},
		// Everything after the first `:` is modifier text, so a qualifier
		// written behind one is not read as a qualifier.
		{"the qualifiers end at the colon", `*/*(N:t.)`, "[x.txt][y.md]"},
		{"and stand in front of it", `*/*(N.:t)`, "[x.txt][y.md]"},
		// An unrecognized modifier stops the chain in silence, which is not
		// what the same text does to a parameter.
		{"an unknown letter is no error", `*/*(N:z)`, "[sub/x.txt][sub/y.md]"},
		{"and stops what follows it", `*/*(N:zt)`, "[sub/x.txt][sub/y.md]"},
		{"mid-chain as well", `*/*(N:t:X:u)`, "[x.txt][y.md]"},
		// Text after the letter inside one segment is ignored rather than
		// being the failure it is in `${x:ha}`.
		{"a second letter needs a colon of its own", `*/*(N:tr)`, "[x.txt][y.md]"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := runQualified(t, dir, `printf "[%s]" `+tc.src)
			if out != tc.want || st != 0 {
				t.Errorf("%s = %q (status %d), want %q at 0", tc.src, out, st, tc.want)
			}
		})
	}
	// A substitution is the one modifier that reports, and it reports the
	// way the parameter surface does.
	out, st := runQualified(t, dir, `printf "[%s]" */*(N:s); echo after`)
	if !containsSub(out, "bad substitution") || st == 0 || containsSub(out, "after") {
		t.Errorf("*(N:s) = %q (status %d), want a bad substitution, fatally", out, st)
	}
}

// linkCountDir holds one name of each link count the `l` qualifier has to tell
// apart: `g1` with one, `f1` hard-linked to `f1b` so both have two, `dir1`
// with two (itself and its `.`), `dir2` with three because it holds a
// subdirectory, and a symbolic link, which has one of its own and points at a
// name that has two.
//
// Counting a directory's links is the platform's rule rather than this
// suite's, so the fixture asserts what it built: a filesystem that does not
// count `.` and `..` would make every row below read as a bug in the shell.
func linkCountDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	for _, name := range []string{"dir1", "dir2"} {
		if err := os.Mkdir(filepath.Join(dir, name), 0o750); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Mkdir(filepath.Join(dir, "dir2", "sub"), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "f1"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Link(filepath.Join(dir, "f1"), filepath.Join(dir, "f1b")); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "g1"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("f1", filepath.Join(dir, "lnk")); err != nil {
		t.Fatal(err)
	}
	for name, want := range map[string]uint64{"dir1": 2, "dir2": 3, "f1": 2, "f1b": 2, "g1": 1, "lnk": 1} {
		if got := linksOf(t, filepath.Join(dir, name)); got != want {
			t.Fatalf("%s has %d links, want %d: this filesystem does not count links the way the fixture assumes", name, got, want)
		}
	}
	return dir
}

// linksOf is the fixture's own reading of a file's link count, so a row that
// fails says whether the shell or the filesystem was the surprise.
func linksOf(t *testing.T, path string) uint64 {
	t.Helper()
	info, err := os.Lstat(path)
	if err != nil {
		t.Fatal(err)
	}
	st, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		t.Skip("no stat structure on this platform")
	}
	return uint64(st.Nlink)
}

// TestTheLinkCountQualifier is #1700.
//
// `l` is a file's link count, and it was in neither table: not in the
// accepted set and not in the refused-by-name one, so `*(l1)` answered
// `unknown file attribute: l` where the shell lists a name. It is also the
// first qualifier here whose argument is a *number*, which is the half that
// generalizes — `L`, `a`, `m` and `c` take the same three forms.
//
// Every row is a measurement on zsh 5.9.2, 2026-09-11, against exactly the
// directory linkCountDir builds.
func TestTheLinkCountQualifier(t *testing.T) {
	dir := linkCountDir(t)
	all := "[dir1][dir2][f1][f1b][g1][lnk]"
	for _, tc := range []struct{ name, list, want string }{
		// The plain number is exact, and the two names it finds are the ones
		// nothing else points at.
		{"a number is exact", "l1", "[g1][lnk]"},
		{"and a second count", "l2", "[dir1][f1][f1b]"},
		// `+` is more and `-` is fewer, and neither takes the number itself:
		// `l-1` finds nothing where `l1` finds two, which is what says the
		// comparison is strict rather than inclusive.
		{"a plus is more than the number", "l+1", "[dir1][dir2][f1][f1b]"},
		{"a minus is fewer", "l-3", "[dir1][f1][f1b][g1][lnk]"},
		{"and fewer than one is nothing", "l-1", ""},
		{"nothing has no links at all", "l0", ""},
		{"and everything has some", "l+0", all},
		{"a leading zero is the same number", "l01", "[g1][lnk]"},
		// A number wider than the type is a count no file can carry rather
		// than bad input, which is the same answer the ownership argument
		// gives a uid nothing holds.
		{"a number no file can carry", "l99999999999999999999", ""},
		{"and the same number from below", "l-99999999999999999999", all},
		// The digits end the argument: `x` here is the permission letter and
		// not part of the count, so the pair is the one-link name whose
		// owner may execute it — the link, whose own mode is 0755.
		{"the digits end the argument", "l1x", "[lnk]"},
		// The caret turns it like any other test, and the `-` that follows a
		// link is written *before* the letter — so the two spellings of a
		// minus do not collide. `lnk` points at `f1`, which has two links.
		{"a caret turns it", "^l2", "[dir2][g1][lnk]"},
		{"the follow toggle asks the target", "-l1", "[g1]"},
		{"and the link itself has one", "l1", "[g1][lnk]"},
		{"a comma unions two counts", "l2,l1", "[dir1][f1][f1b][g1][lnk]"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := runQualified(t, dir, `printf "[%s]" *(`+tc.list+`)`)
			if tc.want == "" {
				if !containsSub(out, "no matches found: *("+tc.list+")") || st == 0 {
					t.Errorf("*(%s) = %q (status %d), want a fatal miss naming the word", tc.list, out, st)
				}
				if containsSub(out, "unknown file attribute") {
					t.Errorf("*(%s) = %q, want the letter claimed rather than refused", tc.list, out)
				}
				return
			}
			if out != tc.want || st != 0 {
				t.Errorf("*(%s) = %q (status %d), want %q at 0", tc.list, out, st, tc.want)
			}
		})
	}
}

// TestALinkCountWithNoNumberIsRefused: an argument with no digits in it is
// the one way to write this qualifier wrong, and the sentence is
// `number expected` rather than the unknown-attribute one — the letter was
// recognized and its argument was not.
//
// The last row is where the letter was met. powerlevel10k writes
// `${(%):-$1%$y(l.1.0)}`, and a reader that globbed that text reached this
// qualifier with `.1.0` behind it; zsh answers the same sentence.
func TestALinkCountWithNoNumberIsRefused(t *testing.T) {
	dir := linkCountDir(t)
	for _, list := range []string{"l", "l+", "l-", "lx", "l 1", "Nl", "l.1.0"} {
		t.Run(list, func(t *testing.T) {
			out, st := runQualified(t, dir, `printf "[%s]" *(`+list+`)`)
			if !containsSub(out, "number expected") || st == 0 {
				t.Errorf("*(%s) = %q (status %d), want `number expected` and a failure", list, out, st)
			}
		})
	}
	// And the control: a character behind a *complete* argument is a
	// qualifier again, so the refusal is about the digits and not about
	// anything following them. `.5` is the regular-file test and then a
	// letter nothing claims.
	out, _ := runQualified(t, dir, `printf "[%s]" *(l1.5)`)
	if !containsSub(out, "unknown file attribute: 5") {
		t.Errorf("*(l1.5) = %q, want the 5 named", out)
	}
}
