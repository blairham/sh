// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/blairham/sh/dialect/bash"
	"github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// The job and lookup long tail, measured against bash 5.3 (2026-09-04):
// job specs by name, wait -n, disown, type's letters, ulimit -a's table and
// the directory stack.

func TestWaitJobSpecs(t *testing.T) {
	out, _ := runBash(t, t.TempDir(), `{ exit 3; } &
wait %1; echo st=$?
wait %1; echo miss=$?`)
	if !strings.Contains(out, "st=3") {
		t.Errorf("got %q, want the job's status through %%1", out)
	}
	if !strings.Contains(out, "wait: %1: no such job") || !strings.Contains(out, "miss=127") {
		t.Errorf("got %q, want the missing spec refused at 127", out)
	}
}

func TestWaitAmbiguousName(t *testing.T) {
	out, _ := runBash(t, t.TempDir(), `{ exit 3; } &
{ exit 4; } &
wait %{; echo st=$?`)
	if !strings.Contains(out, "wait: {: ambiguous job spec") || !strings.Contains(out, "st=127") {
		t.Errorf("got %q, want the ambiguous refusal at 127", out)
	}
}

func TestWaitN(t *testing.T) {
	out, _ := runBash(t, t.TempDir(), `{ exit 3; } &
wait -n; echo st=$?
wait -n; echo empty=$?`)
	if !strings.Contains(out, "st=3") || !strings.Contains(out, "empty=127") {
		t.Errorf("got %q, want the first finisher's status and a silent 127", out)
	}
}

func TestDisownRemovesTheJob(t *testing.T) {
	out, _ := runBash(t, t.TempDir(), `{ exit 0; } &
disown
jobs
echo st=$?
disown; echo none=$?`)
	if strings.Contains(out, "[1]") {
		t.Errorf("got %q, want the job gone from the listing", out)
	}
	if !strings.Contains(out, "disown: current: no such job") || !strings.Contains(out, "none=1") {
		t.Errorf("got %q, want the bare disown refused in bash's words", out)
	}
}

func TestKillMissingJob(t *testing.T) {
	out, _ := runBash(t, t.TempDir(), `kill %9; echo st=$?`)
	if !strings.Contains(out, "kill: %9: no such job") || !strings.Contains(out, "st=1") {
		t.Errorf("got %q, want the job complaint at 1", out)
	}
}

// type: -a lists everything, -p speaks only for files, -P always searches,
// -f leaves functions out.
func TestTypeLetters(t *testing.T) {
	dir := t.TempDir()
	exe := filepath.Join(dir, "tool")
	if err := os.WriteFile(exe, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	out, _ := runBash(t, dir, `PATH=`+dir+`
tool() { :; }
type -a tool
type -p tool; echo p=$?
type -P echo; echo P=$?
type -f tool
type -p nosuchzz; echo miss=$?`)
	if !strings.Contains(out, "tool is a function\n") || !strings.Contains(out, "tool is "+exe+"\n") {
		t.Errorf("got %q, want -a listing the function and the file", out)
	}
	// -p on a function prints nothing at all; -P finds nothing for a
	// builtin name PATH does not hold, silently.
	if !strings.Contains(out, "p=0") || !strings.Contains(out, "P=1") {
		t.Errorf("got %q, want -p silent 0 and -P silent 1", out)
	}
	if !strings.Contains(out, "miss=1") {
		t.Errorf("got %q, want a silent 1 for -p on nothing", out)
	}
}

// -a with a function prints the body after the sentence, as plain type does.
func TestTypeAllPrintsTheBody(t *testing.T) {
	out, _ := runBash(t, t.TempDir(), `f() { echo hi; }
type -a f`)
	want := "f is a function\nf () \n{ \n    echo hi\n}\n"
	if out != want {
		t.Errorf("got %q, want %q", out, want)
	}
}

// ulimit -a renders bash's table — asserted whole, over fake limits, so the
// page is exact and nothing outside the test moves.
func TestUlimitListing(t *testing.T) {
	f, err := syntax.Parse("ulimit -a", bash.Dialect())
	if err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	sem, diag := bash.Semantics(), bash.Diagnostics()
	r := &interp.Runner{Stdout: &buf, Stderr: &buf, Semantics: &sem, Diagnostics: &diag, Name: "bash", Dialect: presetDialect()}
	r.GetRlimit = func(res interp.Resource) (int64, int64, error) {
		switch res {
		case interp.ResourceFileSize:
			// 12345 of bash's 1024-byte blocks.
			return 12345 * 1024, 12345 * 1024, nil
		case interp.ResourceOpenFiles, interp.ResourceProcesses:
			return 256, 256, nil
		}
		return interp.RlimitInfinity, interp.RlimitInfinity, nil
	}
	r.SetRlimit = func(interp.Resource, int64, int64) error { return nil }
	bash.Apply(r)
	if _, err := r.Run(context.Background(), f); err != nil {
		t.Fatal(err)
	}
	want := "core file size              (blocks, -c) unlimited\n" +
		"data seg size               (kbytes, -d) unlimited\n" +
		"file size                   (blocks, -f) 12345\n" +
		"max locked memory           (kbytes, -l) unlimited\n" +
		"max memory size             (kbytes, -m) unlimited\n" +
		"open files                          (-n) 256\n" +
		"pipe size                (512 bytes, -p) 1\n" +
		"stack size                  (kbytes, -s) unlimited\n" +
		"cpu time                   (seconds, -t) unlimited\n" +
		"max user processes                  (-u) 256\n" +
		"virtual memory              (kbytes, -v) unlimited\n"
	if buf.String() != want {
		t.Errorf("got %q, want %q", buf.String(), want)
	}
}

// The directory stack: pushd prints the stack, a bare pushd swaps, popd
// pops and the empty stack refuses — located and named the way bash locates
// and names a builtin's refusal, because to the script that is what it is.
func TestDirectoryStack(t *testing.T) {
	home := t.TempDir()
	out, _ := runBashPrelude(t, home, `HOME=`+home+`
cd
pushd /tmp; echo st=$?
pushd; echo sw=$?
dirs
popd; echo p=$?
popd; echo p2=$?`)
	for _, want := range []string{
		"/tmp ~\nst=0\n",
		"~ /tmp\nsw=0\n",
		"~ /tmp\n",
		"/tmp\np=0\n",
		"bash: line 7: popd: directory stack empty\np2=1\n",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("got %q, want %q in it", out, want)
		}
	}
}

// The axes, pinned by name.
func TestJobAxes(t *testing.T) {
	s := bash.Semantics()
	for _, tc := range []struct {
		axis string
		got  interp.Answer
	}{
		{"JobSpecsByName", s.JobSpecsByName},
		{"AmbiguousJobNameIsRefused", s.AmbiguousJobNameIsRefused},
		{"WaitReportsAMissingJob", s.WaitReportsAMissingJob},
		{"WaitNWaitsForTheNextJob", s.WaitNWaitsForTheNextJob},
		{"DisownRemovesTheJob", s.DisownRemovesTheJob},
	} {
		if tc.got != interp.Yes {
			t.Errorf("%s = %v, want yes", tc.axis, tc.got)
		}
	}
}

// `jobs`' letters. bash has the widest set in the panel, and two of them are
// refused by name rather than accepted and thrown away.
func TestJobsOptionLetters(t *testing.T) {
	out, _ := runBash(t, t.TempDir(), `/bin/sleep 0.3 & echo "bang=$!"
jobs -p
wait`)
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) != 2 || lines[1] != strings.TrimPrefix(lines[0], "bang=") {
		t.Errorf("got %q, want the job's process id and nothing else", out)
	}

	out, _ = runBash(t, t.TempDir(), `jobs -n; echo n=$?
jobs -x true; echo x=$?
jobs -Q; echo q=$?`)
	if !strings.Contains(out, "jobs: -n is not implemented yet") ||
		!strings.Contains(out, "jobs: -x is not implemented yet") {
		t.Errorf("got %q, want the two letters bash has named as missing", out)
	}
	if !strings.Contains(out, "jobs: -Q: invalid option") ||
		!strings.Contains(out, "jobs: usage: jobs [-lnprs] [jobspec ...] or jobs -x command [args]") {
		t.Errorf("got %q, want a letter nobody has refused with the usage line", out)
	}
}

// `-r` and `-s` pick a state, and with both the last letter given decides.
func TestJobsStateFiltersLastLetterWins(t *testing.T) {
	out, _ := runBash(t, t.TempDir(), `/bin/sleep 0.3 & jobs -r; echo "--"; jobs -s; echo "--"; jobs -rs; wait`)
	parts := strings.Split(out, "--")
	if len(parts) != 3 {
		t.Fatalf("got %q, want three listings", out)
	}
	if !strings.Contains(parts[0], "[1]") {
		t.Errorf("got %q, want -r to list the running job", parts[0])
	}
	if strings.Contains(parts[1], "[1]") || strings.Contains(parts[2], "[1]") {
		t.Errorf("got %q / %q, want -s and -rs to list nothing", parts[1], parts[2])
	}
}

// The rotating half of the directory stack (#468). `+N` counts the current
// directory as entry 0 and turns the stack until entry N is the one the
// shell stands in; `-N` counts from the other end; `popd +N` takes an entry
// out and leaves the shell where it is.
func TestDirectoryStackRotates(t *testing.T) {
	home := t.TempDir()
	out, _ := runBashPrelude(t, home, `HOME=`+home+`
cd /
pushd /tmp >/dev/null; pushd /usr >/dev/null
pushd +1; echo "pwd=$PWD"
pushd -0; echo "pwd=$PWD"
popd +1; echo "pwd=$PWD"`)
	for _, want := range []string{
		"/tmp / /usr\npwd=/tmp\n",
		"/usr /tmp /\npwd=/usr\n",
		"/usr /\npwd=/usr\n",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("got %q, want %q in it", out, want)
		}
	}
}

// The refusals, whole: the sentence, the name in front of it, and the location
// in front of that. Every one is a line bash writes verbatim, and the location
// is the half no shell function could reach before #603 — which is why the
// comparison is by whole line rather than by a fragment of one.
func TestDirectoryStackRefusals(t *testing.T) {
	home := t.TempDir()
	out, _ := runBashPrelude(t, home, `HOME=`+home+`
cd /
pushd /tmp >/dev/null
pushd +9; echo "r=$?"
popd -9; echo "o=$?"
dirs +9; echo "d=$?"
popd >/dev/null; pushd +1; echo "e=$?"
dirs -q; echo "q=$?"
pushd -n /etc; echo "n=$?"
popd foo; echo "a=$?"
pushd /no/such/dir-xyz; echo "c=$?"`)
	wantWholeLines(t, out,
		"bash: line 4: pushd: +9: directory stack index out of range",
		"bash: line 5: popd: -9: directory stack index out of range",
		// `dirs` drops the sign where `pushd` and `popd` keep it.
		"bash: line 6: dirs: 9: directory stack index out of range",
		// A stack with nothing in it is a different sentence from an index
		// that is merely too big.
		"bash: line 7: pushd: directory stack empty",
		"bash: line 8: dirs: -q: invalid number",
		// The usage line that follows carries no location, which is bash's
		// own shape: only the first line of a refusal is placed.
		"dirs: usage: dirs [-clpv] [+N] [-N]",
		// Refused by name rather than read as a directory called `-n`.
		"bash: line 9: pushd: -n is not implemented yet",
		// And a third wording for a word that is neither an index nor an
		// option, which `popd` alone has.
		"bash: line 10: popd: foo: invalid argument",
		"popd: usage: popd [-n] [+N | -N]",
		// The complaint a builtin *inside* the function raised. `cd` did the
		// work and no shell in the panel says so: the name the script used is
		// the name the refusal carries.
		"bash: line 11: pushd: /no/such/dir-xyz: No such file or directory",
	)
	for _, want := range []string{"r=1\n", "o=1\n", "d=1\n", "e=1\n", "q=2\n", "n=2\n", "a=2\n", "c=1\n"} {
		if !strings.Contains(out, want) {
			t.Errorf("got %q, want %q in it", out, want)
		}
	}
}

// `dirs`' letters. They do not bundle here — `dirs -lv` is a malformed index,
// not two options — and `-c` empties the stack in silence.
func TestDirsLetters(t *testing.T) {
	home := t.TempDir()
	out, _ := runBashPrelude(t, home, `HOME=`+home+`
cd
pushd / >/dev/null; pushd /tmp >/dev/null
dirs -p; echo "--"
dirs -v; echo "--"
dirs -l; echo "--"
dirs +1; echo "--"
dirs -lv; echo "b=$?"
dirs -c; dirs; echo "c=$?"`)
	for _, want := range []string{
		"/tmp\n/\n~\n--\n",
		" 0  /tmp\n 1  /\n 2  ~\n--\n",
		"/tmp / " + home + "\n--\n",
		"/\n--\n",
		"bash: line 8: dirs: -lv: invalid number\n",
		"/tmp\nc=0\n",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("got %q, want %q in it", out, want)
		}
	}
}
