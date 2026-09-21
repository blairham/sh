// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"os"
	"regexp"
	"strconv"
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// devFdPath is the shape every shell in the panel expands `<(cmd)` and
// `>(cmd)` to, with the number left for the caller to read.
var devFdPath = regexp.MustCompile(`^/dev/fd/([0-9]+)$`)

// What the word expands to, which is the whole of #2893.
//
// Measured 2026-09-15 on all three invocation routes: `printf '%s' <(true)` is
// a path under /dev/fd in bash 5.3, ksh93u+ and zsh 5.9.2 alike, and this
// shell answered a named pipe in a private directory under `$TMPDIR`. Both are
// pipes and both read correctly, so what differed was the name — a path that
// leaks the temporary directory into anything echoing its arguments, does not
// repeat between runs, and leaves a file in the filesystem for something to
// remove.
//
// The number is not asserted, because it is not the shell's to promise: it is
// the lowest descriptor this shell had free, exactly as bash's 63 and ksh93's
// 3 are that shell's own. What a script can depend on is the shape.
func TestAProcessSubstitutionExpandsToADescriptorPath(t *testing.T) {
	for _, tc := range []struct{ name, src string }{
		{"the reading form", `printf '%s' <(true)`},
		{"the writing form", `printf '%s' >(true)`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := run(t, tc.src, nil)
			if st != 0 {
				t.Fatalf("status %d, want 0", st)
			}
			if !devFdPath.MatchString(out) {
				t.Errorf("the word expanded to %q, want a /dev/fd path", out)
			}
		})
	}
}

// And nothing is left behind for anything to remove, which is the second of
// the three things the name cost.
//
// The whole of the shell's temporary directory rather than one path: a pipe
// that was still a file would be somewhere in there, whatever it was called.
func TestAProcessSubstitutionWritesNothingToTheFilesystem(t *testing.T) {
	tmp := t.TempDir()
	out, st := run(t, `cat <(echo hi) >/dev/null; echo done`, func(r *Runner) {
		r.Env = append(testPATH(), "TMPDIR="+tmp)
	})
	if st != 0 || out != "done\n" {
		t.Fatalf("out = %q status %d, want the substitution to work", out, st)
	}
	if ents, err := os.ReadDir(tmp); err != nil {
		t.Fatal(err)
	} else if len(ents) != 0 {
		t.Errorf("%d entries under TMPDIR, want none — a pipe is a descriptor and not a file", len(ents))
	}
}

// The descriptor reaches the command that was handed the path, and no other
// command the shell runs.
//
// This is the failure that made `/dev/fd` look impossible, and it is what a
// per-command table answers where clearing close-on-exec does not: a later
// command holding a `>(cmd)`'s *writing* end open means the body never reads
// end-of-file. `echo x | tee >(tr a-z A-Z); sleep 0.4` produced nothing at
// all when the flag was cleared, because `sleep` was holding the pipe.
//
// The programs are named without a path because `tee` is not in the same
// directory on every platform this builds for; PATH is the test harness's own
// and reaches both of the two the corpus already uses.
//
// The `sleep` is what makes it a probe rather than a coincidence: without it
// the shell reaches the end of the script and closes everything, so a leaked
// descriptor and a contained one look the same. With it, a leak is a body
// that has not finished when the run ends and output that is not there.
//
// **A leak shows up here as a hang and not as a failed comparison**, which is
// worth knowing before reading a stuck run as an unrelated flake: asking
// F_DUPFD for the parked end instead of F_DUPFD_CLOEXEC — the whole of the
// old arrangement, in one word — takes this package past its own deadline
// rather than printing a diff. The body is blocked reading a pipe whose
// writing end a later command is holding, and there is nothing to time out
// but the test binary.
func TestALaterCommandDoesNotInheritASubstitutionsDescriptor(t *testing.T) {
	out, st := run(t, "echo x | tee >(tr a-z A-Z) >/dev/null\nsleep 0.4\n", nil)
	if st != 0 {
		t.Fatalf("status %d, want 0", st)
	}
	if out != "X\n" {
		t.Errorf("out = %q, want %q — a later command is holding the writing end open", out, "X\n")
	}
}

// And a substitution's own body does not inherit it either, which is the same
// failure one step closer in: the body of a `>(cmd)` is reading the other side
// of the very pipe whose writing end it must not hold.
//
// A C shell arranges this by closing the descriptor in the child it forks.
// Here it falls out of the structure — a body runs in a clone, and a clone
// carries no substitutions of its own — and this is what says so, because a
// table built from the shared list would deadlock exactly here.
func TestASubstitutionsBodyDoesNotInheritItsOwnDescriptor(t *testing.T) {
	out, st := run(t, `echo x | tee >(cat) >/dev/null`, nil)
	if st != 0 {
		t.Fatalf("status %d, want 0", st)
	}
	if out != "x\n" {
		t.Errorf("out = %q, want %q — the body is holding its own pipe open", out, "x\n")
	}
}

// The path leads to a pipe, and to the end the operator named.
//
// A probe on the *object* rather than on the name, which is what says the
// change of spelling did not change the plumbing: `test -r` on the reading
// form and `test -w` on the writing one, and `test -p` for the kind. All three
// are what bash 5.3, ksh93u+ and zsh 5.9.2 answer.
func TestADescriptorPathStillLeadsToAPipe(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{"the reading form is readable", `test -r <(true) && echo yes`, "yes\n"},
		{"the writing form is writable", `test -w >(true) && echo yes`, "yes\n"},
		{"and it is a pipe", `test -p <(true) && echo yes`, "yes\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := run(t, tc.src, nil)
			if st != 0 || out != tc.want {
				t.Errorf("out = %q status %d, want %q at 0", out, st, tc.want)
			}
		})
	}
}

// The number is above the range a script names by hand, and that is not
// tidiness: it is where the table childFiles builds will place the descriptor
// in the command, and a script's own `exec 3>out` is an entry in that same
// table. Two things on one number is one of them lost, and which one would
// depend on map iteration order.
func TestASubstitutionsDescriptorIsAboveTheNumbersAScriptNames(t *testing.T) {
	out, st := run(t, `printf '%s' <(true)`, nil)
	if st != 0 {
		t.Fatalf("status %d, want 0", st)
	}
	m := devFdPath.FindStringSubmatch(out)
	if m == nil {
		t.Fatalf("the word expanded to %q, want a /dev/fd path", out)
	}
	n, err := strconv.Atoi(m[1])
	if err != nil {
		t.Fatal(err)
	}
	if n < 10 {
		t.Errorf("the pipe is on descriptor %d, want one above the nine a script names by hand", n)
	}
}

// A descriptor the script put on a number of its own still reaches the
// command, with a substitution live beside it.
//
// The two are in one table and this is what says they do not collide: the
// substitution's end is parked above the script's range, so `exec 3>` and
// `<(cmd)` in the same command are both there in the child.
func TestAScriptsOwnDescriptorSurvivesBesideASubstitution(t *testing.T) {
	dir := t.TempDir()
	out, st := run(t, `exec 3>`+dir+`/three
/bin/cat <(/bin/echo sub)
/bin/sh -c '/bin/echo viathree >&3'
exec 3>&-
cat `+dir+`/three`, nil)
	if st != 0 {
		t.Fatalf("status %d, want 0", st)
	}
	if want := "sub\nviathree\n"; !strings.Contains(out, want) {
		t.Errorf("out = %q, want %q in it", out, want)
	}
}

// A descriptor number is reused, so two substitutions in one script can wear
// one path — and the shell has to tell them apart anyway.
//
// This is the hazard a name in a directory did not have. A substitution's end
// is closed at the end of the command that named it, and the next one takes
// the number it gave up, so `/dev/fd/11` names one pipe and then another. The
// shell asks whether one of its own descriptors is still open on a pipe, to
// decide whether the body is the command's to wait for or the shell's; asked
// by path, it answered that the second pipe was held by the first one's
// descriptor, never waited for it, and dropped what its body wrote.
//
// `exec > >(cat)` is the shape that reaches it, because that is the one
// substitution whose pipe the *shell* keeps: its number is freed while the
// script still holds the pipe, so the substitution on the next line is handed
// exactly that number.
func TestTwoSubstitutionsOnOneDescriptorNumberAreToldApart(t *testing.T) {
	out, st := run(t, `exec > >(cat)
printf hi > >(tr a-z A-Z)`, nil)
	if st != 0 {
		t.Fatalf("status %d, want 0", st)
	}
	if out != "HI" {
		t.Errorf("out = %q, want %q — the second pipe was taken for the first", out, "HI")
	}
}

// Which directory the path is named after, which is
// Semantics.SubstitutionPathPrefersProcSelfFd.
//
// Measured 2026-09-21: zsh 5.9.2 writes `/proc/self/fd/11` in the pinned
// Linux image and `/dev/fd/11` on the panel machine, while bash 5.3.20,
// ksh93u+ and BusyBox ash write `/dev/fd/N` in both — so in one image, with
// one /dev/fd symlink pointing at /proc/self/fd, one shell writes the target
// and the rest write the link. The axis carries the rest of the measurement,
// including why the directory is looked for at run time.
//
// Only the string moves, and the rows say so: each one opens the path it was
// given and reads the body's output through it. A reading that named a
// directory the command cannot open would pass an assertion on the prefix
// alone.
func TestWhichDirectoryAProcessSubstitutionIsNamedAfter(t *testing.T) {
	// The preferred answer is only reachable where the directory is, which
	// is the platform half of the question and not the shell's.
	preferred := "/dev/fd/"
	if info, err := os.Stat("/proc/self/fd"); err == nil && info.IsDir() {
		preferred = "/proc/self/fd/"
	}
	for _, tc := range []struct {
		name   string
		answer Answer
		want   string
	}{
		{"preferring /proc/self/fd", Yes, preferred},
		{"naming /dev/fd", No, "/dev/fd/"},
		// Unanswered reads as /dev/fd, which is the fallback every shell in
		// the panel takes and the answer four of the five take everywhere.
		{"unanswered", Unspecified, "/dev/fd/"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			prefer := func(r *Runner) {
				sem := *r.Semantics
				sem.SubstitutionPathPrefersProcSelfFd = tc.answer
				r.Semantics = &sem
			}
			out, st := run(t, `printf '%s' <(true)`, prefer)
			if st != 0 {
				t.Fatalf("status %d, want 0", st)
			}
			if !strings.HasPrefix(out, tc.want) {
				t.Errorf("the word expanded to %q, want a path under %s", out, tc.want)
			}
			if _, err := strconv.Atoi(strings.TrimPrefix(out, tc.want)); err != nil {
				t.Errorf("the word expanded to %q, want %s and a descriptor number", out, tc.want)
			}
			// And the name still names the pipe: the axis is the string and
			// the descriptor is untouched by it.
			body, st := run(t, `cat <(echo hi)`, prefer)
			if body != "hi\n" || st != 0 {
				t.Errorf("cat through the path gave %q at %d, want %q at 0", body, st, "hi\n")
			}
		})
	}
}
