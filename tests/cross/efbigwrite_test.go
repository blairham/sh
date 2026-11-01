// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package cross_test

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"syscall"
	"testing"

	"github.com/blairham/sh/internal/dialecttest"
)

// **A write past `ulimit -f` with SIGXFSZ arranged away is the panel's
// ordinary failed write** — interp's builtinWriteStatus, keyed on the errno
// (#7172).
//
// Measured 2026-11-01 on macOS, each shell in a session of its own, against
// GNU bash 5.3.20 (also as `sh`), /bin/bash 3.2.57, /bin/dash, /bin/ksh 93u+
// and zsh 5.9.2:
//
//	trap '' XFSZ; ulimit -f 1; printf "%4000s" x > f; echo after=$?
//	  bash 5.3, as sh   printf: File too large, then printf: write error: File too large, after=1
//	  bash 3.2          printf: write error: File too large, after=1
//	  dash              dash: 1: printf: printf: I/O error, after=1
//	  ksh93             ksh: printf: write to 1 failed [File too large], after=1
//	  zsh               zsh:printf:1: write error: file too large, after=1
//
// `echo "$x" > f` with a 2000-byte $x is the same, bash 5.3 writing one line,
// and so is either with `trap 'echo caught >&2' XFSZ` in place of the ignore,
// the handler running after the complaint in all but zsh. Every column fails
// the command at 1, so the status needs no axis; what is per dialect is the
// sentence, and in two dialects which errno is quiet.
//
// bash 5.3's first line is not modeled. It is keyed on how much `printf`
// produced past the limit — 2048 bytes is one line and 2049 two, at a limit of
// 1024 — and not on the errno: a full disk draws it too. See
// docs/spec/semantics.md, "Every other errno".
//
// The stream here stands in for the file: every write fails with EFBIG, as a
// write past the limit does, so the probe never sets a limit on this process.
// `1>&1` makes it the command's own redirection, as `> f` is in the rows.
func TestAWritePastTheFileSizeLimitIsAnOrdinaryFailedWrite(t *testing.T) {
	const printfRow = `trap '' XFSZ; printf '%4000s' x 1>&1; echo "after=$?" >&2`
	const echoRow = `x=$(printf %2000s); trap '' XFSZ; echo "$x" 1>&1; echo "after=$?" >&2`
	for _, c := range []struct {
		dialect, src, want string
	}{
		{"bash", printfRow, "bash: line 1: printf: write error: File too large\nafter=1\n"},
		{"dash", printfRow, "dash: 1: printf: printf: I/O error\nafter=1\n"},
		{"ksh", printfRow, "ksh: printf: write to 1 failed [File too large]\nafter=1\n"},
		{"zsh", printfRow, "zsh:printf:1: write error: file too large\nafter=1\n"},
		{"bash", echoRow, "bash: line 1: echo: write error: File too large\nafter=1\n"},
		{"dash", echoRow, "dash: 1: echo: echo: I/O error\nafter=1\n"},
		{"ksh", echoRow, "ksh: echo: write to 1 failed [File too large]\nafter=1\n"},
		{"zsh", echoRow, "zsh:echo:1: write error: file too large\nafter=1\n"},
		// The handler runs after the complaint, and the command still fails.
		{
			"ksh", `trap 'echo caught >&2' XFSZ; printf '%4000s' x 1>&1; echo "after=$?" >&2`,
			"ksh: printf: write to 1 failed [File too large]\ncaught\nafter=1\n",
		},
	} {
		if got := pastTheFileSizeLimit(t, c.dialect, c.src); got != c.want {
			t.Errorf("%s: %s\n got %q\nwant %q", c.dialect, c.src, got, c.want)
		}
	}
}

// The rows that say what the two odd dialects are keyed on. zsh and ksh93 each
// treat EBADF as the exception and every other errno as the rule; a listing
// builtin keeps its own status in both; and ksh93's `print -u` off 1 checks
// nothing but the descriptor. Measured 2026-11-01, zsh 5.9.2 and ksh93u+:
//
//	x=$(printf %2000s); trap '' XFSZ; ulimit -f 1; <row>; echo after=$?
//	  set > f                      zsh 0, silent    ksh93 ends the shell, not modeled
//	  exec 4>f; print -u4 -r -- "$x"                ksh93 0, silent
//	  print -u1 -r -- "$x" > f                      ksh93 1, write to 1 failed
//	echo hi >&-                    zsh 0, silent    ksh93 1, silent
//	exec 3</etc/hosts; echo hi >&3 zsh 0, silent    ksh93 1, silent
func TestTheQuietErrnoIsEBADFAndOnlyEBADF(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "r"), []byte("r\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		dialect, src, want string
	}{
		{"zsh", `trap '' XFSZ; set 1>&1; echo "after=$?" >&2`, "after=0\n"},
		{"zsh", `echo hi >&-; echo "after=$?" >&2`, "after=0\n"},
		{"zsh", `exec 3<r; echo hi >&3; echo "after=$?" >&2`, "after=0\n"},
		{"ksh", `echo hi >&-; echo "after=$?" >&2`, "after=1\n"},
		{"ksh", `exec 3<r; echo hi >&3; echo "after=$?" >&2`, "after=1\n"},
		{"ksh", `trap '' XFSZ; exec 4>&1; print -u4 -r -- x; echo "after=$?" >&2`, "after=0\n"},
		{
			"ksh", `trap '' XFSZ; print -u1 -r -- x 1>&1; echo "after=$?" >&2`,
			"ksh: print: write to 1 failed [File too large]\nafter=1\n",
		},
	} {
		if got := pastTheFileSizeLimitIn(t, dir, c.dialect, c.src); got != c.want {
			t.Errorf("%s: %s\n got %q\nwant %q", c.dialect, c.src, got, c.want)
		}
	}
}

// pastTheFileSizeLimit runs src under the dialect with a standard output past
// the file-size limit, and answers with standard error alone.
func pastTheFileSizeLimit(t *testing.T, dialect, src string) string {
	t.Helper()
	return pastTheFileSizeLimitIn(t, t.TempDir(), dialect, src)
}

func pastTheFileSizeLimitIn(t *testing.T, dir, dialect, src string) string {
	t.Helper()
	p := presets[dialect]
	var e bytes.Buffer
	r := p.Runner(dialecttest.Base{Dir: dir, Stdout: fileTooLarge{}, Stderr: &e})
	if _, err := r.Run(context.Background(), p.ParseThrough(t, r, src)); err != nil {
		t.Fatalf("%s: run %q: %v", dialect, src, err)
	}
	return e.String()
}

// fileTooLarge is a stream past the file-size limit: every write fails the
// way the kernel fails one past `ulimit -f`.
type fileTooLarge struct{}

func (fileTooLarge) Write([]byte) (int, error) { return 0, syscall.EFBIG }

// **Once an EXIT trap has caught the signal, ksh93 gives up over a write past
// `ulimit -f` rather than dying of it** — the builtin's complaint, the trap
// reading the status from before the builtin, and the shell leaving at 1, as
// over `echo $((1/0))` in the same place (#7173). bash runs the trap and dies.
//
// Measured 2026-11-01, each shell in a session of its own, ksh93u+ and GNU
// bash 5.3.20:
//
//	trap 'echo t $?' EXIT; ulimit -f 1; (exit 5); printf "%4000s" x > f; echo after=$?
//	  ksh93   ksh: printf: write to 1 failed [File too large], t 5, exit 1
//	  bash    t 5, killed by XFSZ
//	the same with `exit 6` last in the trap      ksh93 6
//	the same with `(exit 4)` last in the trap    ksh93 1
//	the trap set, then `trap - EXIT`             ksh93 the complaint, exit 1
//	`kill -XFSZ $$` in place of the printf       both t 5, killed by XFSZ
//	inside `function f { …; echo in; }`          ksh93 ksh: f[1]: printf: …, after=1, t 0, exit 0
//
// The rows after them are recorded and not modeled — see
// docs/spec/semantics.md, "A write past the limit once the signal is
// caught" — and what is pinned for them is this shell's own answer:
//
//	inside `f() { …; echo in; }`                 ksh93 the complaint, t 1, killed by XFSZ
//	(printf … > f; echo in), the trap set        ksh93 ksh: write to 1 failed [File too large], t 0, exit 1
//	(printf … > f; echo in), no trap             ksh93 the whole shell killed by XFSZ
//	y=$(printf … > f; echo in), no trap          ksh93 ksh: <pid>: Exceeded file size limit, after=281
func TestACaughtWritePastTheFileSizeLimitIsAFatalErrorInOneDialect(t *testing.T) {
	const (
		trap      = `trap 'echo "t $?" >&2' EXIT; `
		write     = `printf '%4000s' x 1>&1; echo "after=$?" >&2`
		complaint = "ksh: printf: write to 1 failed [File too large]\n"
		notice    = "ksh: 0: Exceeded file size limit\n"
	)
	killed := 128 + int(syscall.SIGXFSZ)
	for _, c := range []struct {
		dialect, src string
		want         caughtOverrun
	}{
		{"ksh", trap + `(exit 5); ` + write, caughtOverrun{complaint + "t 5\n", 1, 0}},
		{"bash", trap + `(exit 5); ` + write, caughtOverrun{"t 5\n", killed, syscall.SIGXFSZ}},
		{
			"ksh", `trap 'echo "t $?" >&2; exit 6' EXIT; (exit 5); ` + write,
			caughtOverrun{complaint + "t 5\n", 6, 0},
		},
		{
			"ksh", `trap 'echo "t $?" >&2; (exit 4)' EXIT; (exit 5); ` + write,
			caughtOverrun{complaint + "t 5\n", 1, 0},
		},
		{"ksh", trap + `trap - EXIT; (exit 5); ` + write, caughtOverrun{complaint, 1, 0}},
		{"bash", trap + `trap - EXIT; (exit 5); ` + write, caughtOverrun{"", killed, syscall.SIGXFSZ}},
		// The route and not the signal.
		{"ksh", trap + `(exit 5); kill -XFSZ $$; echo after >&2`, caughtOverrun{"t 5\n", killed, syscall.SIGXFSZ}},
		{
			"ksh", trap + `function f { printf '%4000s' x 1>&1; echo in >&2; }; (exit 5); f; echo "after=$?" >&2`,
			caughtOverrun{"ksh: f[1]: printf: write to 1 failed [File too large]\nafter=1\nt 0\n", 0, 0},
		},
		// Recorded, not modeled: this shell's answers.
		{
			"ksh", trap + `f() { printf '%4000s' x 1>&1; echo in >&2; }; (exit 5); f; echo "after=$?" >&2`,
			caughtOverrun{complaint + "t 5\n", 1, 0},
		},
		{
			"ksh", trap + `(printf '%4000s' x 1>&1; echo in >&2); echo "after=$?" >&2`,
			caughtOverrun{notice + "after=153\nt 0\n", 0, 0},
		},
		{"ksh", `(printf '%4000s' x 1>&1; echo in >&2); echo "after=$?" >&2`, caughtOverrun{notice + "after=153\n", 0, 0}},
		// A substitution's own standard output is the capture, so the
		// stream past the limit reaches it by a descriptor of its own.
		{"ksh", `exec 4>&1; y=$(printf '%4000s' x >&4; echo in >&2); echo "after=$?" >&2`, caughtOverrun{"after=153\n", 0, 0}},
	} {
		if got := overrunWithATrap(t, c.dialect, c.src); got != c.want {
			t.Errorf("%s: %s\n got %+v\nwant %+v", c.dialect, c.src, got, c.want)
		}
	}
}

// caughtOverrun is how a run past the file-size limit ended: what it said,
// its status, and the signal it asked to die of.
type caughtOverrun struct {
	errs   string
	status int
	died   syscall.Signal
}

// overrunWithATrap runs src under the dialect with a standard output past the
// file-size limit and a DieBySignal that records the signal rather than ending
// this process.
func overrunWithATrap(t *testing.T, dialect, src string) caughtOverrun {
	t.Helper()
	p := presets[dialect]
	var e bytes.Buffer
	var got caughtOverrun
	r := p.Runner(dialecttest.Base{Dir: t.TempDir(), Stdout: fileTooLarge{}, Stderr: &e})
	r.DieBySignal = func(s syscall.Signal) error {
		got.died = s
		return nil
	}
	r.ListenForFatalSignals = func() {}
	st, err := r.Run(context.Background(), p.ParseThrough(t, r, src))
	if err != nil {
		t.Fatalf("%s: run %q: %v", dialect, src, err)
	}
	got.errs, got.status = e.String(), st
	return got
}
