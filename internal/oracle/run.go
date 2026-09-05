// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package oracle

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"regexp"
	"strings"
	"syscall"
	"time"
)

// RunTimeout bounds a single snippet. A shell that waits on stdin would
// otherwise hang the whole run, so stdin is closed unless the case supplies
// some — and a supplied one is a finite string that ends. Both are needed,
// because a snippet can block on things other than input.
const RunTimeout = 10 * time.Second

// Result is what one shell did with one case.
type Result struct {
	// Stdout and Stderr are what the shell wrote to each stream, normalized
	// and with the trailing newline removed.
	//
	// They are recorded apart because a diagnostic is part of the behavior:
	// two shells that print the same thing to different streams have not
	// behaved the same way, and a merged capture cannot tell. That claim was
	// written here while the harness took cmd.CombinedOutput() and could not
	// make it good — a diagnostic printed to standard output read exactly
	// like one printed to standard error, so a whole class of regression was
	// invisible unless the snippet separated the streams itself with `2>`.
	// A handful of cases do that; the rest were being graded on a merge.
	//
	// What a merge bought was the order the two streams interleaved in, and
	// that is given up here. It was never worth much: a shell block-buffers
	// standard output into a pipe and writes standard error unbuffered, so
	// the merged order was the order the buffers happened to flush rather
	// than the order the shell wrote. A case that means to pin ordering
	// across the two streams still says so the only reliable way, by
	// redirecting one of them where it can see it.
	Stdout string
	Stderr string

	// Status is the exit status, or -1 if the shell could not be run at all.
	// It is recorded separately because a case can produce identical output
	// with a different status, and that is still a difference.
	//
	// It is also -1 for a shell that died by a *signal*, because there is no
	// exit status in that case: the wait status carries a signal number
	// instead, and Go reports the absence as -1. Which signal is in Signal.
	Status int

	// Signal is the signal that killed the shell, or 0 if it exited normally.
	//
	// Without it, every way of dying by a signal looks the same in the
	// record, because Status is -1 for all of them: a shell that ended on
	// SIGINT and one that ended on SIGKILL were indistinguishable, and so
	// were a shell killed by a signal and one the harness could not run at
	// all. That made a shell's whole exit-on-signal discipline untestable —
	// the convention that a shell dying of an untrapped fatal signal kills
	// *itself* with that same signal, so its own caller sees the signal
	// rather than a status. Every case that wanted to look at a signal had
	// to hide the death inside a nested child and read the number the parent
	// shell computed from it, which measures the parent's arithmetic rather
	// than the child's death.
	//
	// Zero is "no signal" and not a signal number: 0 is the valid argument
	// to kill(2) that sends nothing, so nothing is ever killed by it.
	Signal syscall.Signal

	// TimedOut marks a snippet the shell never finished.
	TimedOut bool
}

// harnessError is the Result for a case that could not be measured.
//
// It goes to Stderr because that is what it is — a diagnostic — and because
// a reader scanning the record for the stream a message came out on should
// not find the harness's own failures filed under a shell's output.
func harnessError(err error) Result {
	return Result{Stderr: "harness error: " + err.Error(), Status: -1}
}

// ArgSnippet and ArgScript are the placeholders a Case.Args may use to say
// where its snippet goes: as the word after `-c`, or as a file the shell is
// asked to run. Case.Stdin honors them too, which is how a case says its
// program arrives on standard input. See Case.Args and Case.Stdin, which
// document the contract.
//
// They are spelled with braces because no shell gives that spelling a meaning
// in an argument, so a placeholder can never be confused with a word a case
// actually meant to pass.
const (
	ArgSnippet = "{snippet}"
	ArgScript  = "{script}"
)

// Exec executes one case in one shell.
//
// The snippet is passed with -c rather than written to a file unless the case
// asks otherwise: some behavior depends on how input is read, and a case that
// cares says so. See Case.Script, and Case.Args for a case that spells its
// whole invocation out.
func Exec(ctx context.Context, sh Found, c Case) Result {
	if err := c.validate(); err != nil {
		return harnessError(err)
	}
	ctx, cancel := context.WithTimeout(ctx, RunTimeout)
	defer cancel()

	// The case's own name for the shell wins over the panel entry's, and it
	// is applied to the whole Found so that the normalization below strips
	// the name that was actually used. Overwritten on the copy Exec was
	// handed, which is a value: the panel itself is not edited by running a
	// case against it.
	if c.Argv0 != "" {
		sh.Argv0 = c.Argv0
	}

	// The other half of the scrub below: a shell inherits how it was launched
	// through its signal dispositions as surely as through its environment.
	scrubSignalDispositions()

	dir, err := os.MkdirTemp("", "oracle-")
	if err != nil {
		return harnessError(err)
	}
	// The scratch directory is this run's alone, so a failed removal is a
	// leaked temp dir rather than a wrong measurement; nothing here can act
	// on it usefully.
	defer func() { _ = os.RemoveAll(dir) }()

	cmd := command(ctx, sh, c, dir)
	cmd.Dir = dir
	// A snippet must not inherit a user's environment: HOME, IFS or PATH
	// from the developer's shell would make the record depend on whose
	// machine produced it. Standard input is the same argument and is
	// settled in command(), which knows what the case asked for.
	// A buffer per stream rather than one shared one. os/exec sends both to
	// the same pipe when they are the same writer, which is what
	// CombinedOutput does and what this exists not to do.
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err = cmd.Run()
	res := Result{
		Stdout: normalize(stdout.String(), sh, dir),
		Stderr: normalize(stderr.String(), sh, dir),
		Status: 0,
	}

	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		res.TimedOut = true
		res.Status = -1
		return res
	}
	var ee *exec.ExitError
	switch {
	case errors.As(err, &ee):
		res.Status = ee.ExitCode()
		res.Signal = signalOf(ee.ProcessState)
	case err != nil:
		// Whatever the shell managed to write before the harness failed is
		// not a measurement of anything, so it is replaced rather than
		// annotated.
		res = harnessError(err)
	}
	return res
}

// ignorableSignals are the signals a caller can leave ignored and the Go
// runtime will report as ignored. Every one is a signal a shell reports
// through `trap`, dies of, or declines to die of, so an inherited disposition
// for any of them is a difference the record would attribute to the shell.
//
// SIGKILL and SIGSTOP are absent because they cannot be caught, so they cannot
// be ignored either. SIGURG and SIGPROF are absent because the Go runtime owns
// them — preemption and profiling — and taking them over would break the
// harness to guard against a state the runtime does not permit anyway.
//
// The four job-control signals are here, and are also the one thing this
// cannot reach today: the runtime will not report an *inherited* ignore for
// them, so the condition is never detected. Listing them costs nothing —
// nothing is taken over that is not reported — and it means the day the
// runtime starts reporting them, they are already covered. See the comment on
// scrubSignalDispositions.
var ignorableSignals = []os.Signal{
	syscall.SIGHUP, syscall.SIGINT, syscall.SIGQUIT, syscall.SIGILL,
	syscall.SIGTRAP, syscall.SIGABRT, syscall.SIGFPE, syscall.SIGBUS,
	syscall.SIGSEGV, syscall.SIGSYS, syscall.SIGPIPE, syscall.SIGALRM,
	syscall.SIGTERM, syscall.SIGCHLD, syscall.SIGXCPU, syscall.SIGXFSZ,
	syscall.SIGVTALRM, syscall.SIGWINCH, syscall.SIGUSR1, syscall.SIGUSR2,
	syscall.SIGTSTP, syscall.SIGTTIN, syscall.SIGTTOU, syscall.SIGCONT,
}

// ignoredSink receives the signals taken over because the caller had ignored
// them, and nothing ever reads it. That is not an oversight and not a leak:
// os/signal never blocks on delivery, so a full channel means the signal is
// dropped — which is precisely what the caller asked for by ignoring it. A
// goroutine draining this was written first and does nothing a mutation could
// detect, which is how it came out again.
var ignoredSink = make(chan os.Signal, 1)

// scrubSignalDispositions makes a child start with default signal
// dispositions whatever the harness was started with.
//
// This is the same argument as the environment scrub in Exec, arriving by a
// route that is easier to miss. A disposition of *ignored* survives exec —
// that is the whole of what `nohup` does — so a shell measured by a harness
// launched under `nohup` starts with SIGHUP already ignored. It reports
// `trap -- ” SIGHUP`, and `kill -HUP $$` leaves it alive to print what came
// after. Over the whole panel that moved a conformance run from 1110/1116 to
// 1101/1116 and made `oracle-check` drift, so the project's headline
// instrument was a function of how the harness happened to be launched: a
// terminal, a CI runner, a supervisor and a background shell can each hand it
// a different answer. Two runs that disagreed would read as a flaky
// implementation, which is the most expensive misreading available here.
//
// It cannot be done to the child, because a disposition belongs to the
// process that forks and there is nothing to run between fork and exec. It is
// done to the harness instead: signal.Notify replaces SIG_IGN with a handler,
// and exec resets a *handled* signal to its default in the child. Draining
// and discarding is what keeps the harness's own behavior the one its caller
// asked for — `nohup` still means this process ignores hangups; it no longer
// means the shells it measures do, because os/signal drops what it cannot
// deliver rather than blocking.
//
// Nothing guards it against running per case, because it needs no guard: the
// loop disarms itself, since a signal that has been taken over stops being
// reported as ignored. Running it per case is what lets a disposition set
// after the first measurement still be caught.
//
// # The four this cannot cover, and why it does not try
//
// SIGTSTP, SIGTTIN, SIGTTOU and SIGCONT leak, and the choice to let them is
// deliberate. The Go runtime keeps an inherited SIG_IGN for those four —
// stopping a process that was started with stopping turned off would be
// wrong — and does not report that it has: signal.Ignored answers false for
// all four while a child still sees `trap -- ” SIGTSTP`. Measured across
// every signal a shell can have an opinion about, they are the only ones
// where the report and the child disagree.
//
// Taking them over blind is possible and costs more than it buys, because it
// cannot be undone. A Go program stops on a SIGTSTP raised at itself; the
// same program after one signal.Notify does not, and neither signal.Stop nor
// signal.Reset gives the stop back. So a blind takeover would permanently
// cost the harness its Ctrl-Z, and a forwarder that restores the disposition
// and re-raises cannot put it back either — measured, not assumed.
//
// Asking a shell instead does not work portably. POSIX says a signal ignored
// on entry cannot be trapped, so `trap : TSTP; trap` would say which it was —
// but only bash honors it. dash and zsh install the trap regardless, so a
// probe would be right only when bash happened to be the shell probing.
//
// What is left is a hole no launcher opens. `nohup`, a CI runner, a
// supervisor and a non-interactive shell's background job ignore SIGHUP,
// SIGINT or SIGQUIT, and all three are covered. If a Go release starts
// reporting the four truthfully, move them into ignorableSignals; the test
// named for this gap fails when that day comes.
func scrubSignalDispositions() []os.Signal {
	var taken []os.Signal
	for _, sig := range ignorableSignals {
		if signal.Ignored(sig) {
			taken = append(taken, sig)
		}
	}
	if len(taken) == 0 {
		return nil
	}
	signal.Notify(ignoredSink, taken...)
	return taken
}

// signalOf is the signal a process died of, or 0.
//
// The wait status is the only place the number survives: an exit status and a
// signal death are alternatives in it, which is why ExitCode() has to answer
// -1 for the second. Anything that is not a wait status this package
// understands reports no signal rather than guessing.
func signalOf(st *os.ProcessState) syscall.Signal {
	ws, ok := st.Sys().(syscall.WaitStatus)
	if !ok || !ws.Signaled() {
		return 0
	}
	return ws.Signal()
}

// validate reports a case whose invocation cannot be built.
//
// It is checked here rather than only in the corpus test because a harness
// error has to be visible in the record: a case that quietly ran something
// other than what it says would be measured, and the measurement is the
// whole product.
func (c Case) validate() error {
	if len(c.Args) == 0 {
		return nil
	}
	if c.Script {
		return errors.New("Case.Args and Case.Script are exclusive: put ArgScript in Args instead")
	}
	// Counted as substrings, because that is how they are replaced. A word
	// that merely contains a placeholder still names the snippet's place, so
	// counting whole words would let `-c{snippet}` and a second `{snippet}`
	// through as though only one of them said anything.
	n := 0
	for _, a := range c.Args {
		n += strings.Count(a, ArgSnippet) + strings.Count(a, ArgScript)
	}
	if n > 1 {
		return errors.New("Case.Args names the snippet's place more than once")
	}
	return nil
}

// command builds the invocation: the shell's own flags first — which is where
// the binary under test is told which dialect to be — and then either the
// case's own argv or the harness's default `-c` and snippet.
func command(ctx context.Context, sh Found, c Case, dir string) *exec.Cmd {
	// Written to a file and run as an argument, because a few behaviors
	// differ between a script and -c: a readonly reassignment is fatal in one
	// and not the other, which is how the contaminated-probe trap in
	// oracle.md was found.
	script := func() string {
		path := filepath.Join(dir, "case.sh")
		_ = os.WriteFile(path, []byte(c.Snippet+"\n"), 0o600)
		return path
	}

	args := append([]string(nil), sh.Args...)
	switch {
	case len(c.Args) > 0:
		// Replaced wherever they appear rather than only as a whole word,
		// which is what Case.Stdin has always done. An invocation that needs
		// the snippet *inside* a larger word — `-c` with its command string
		// written against the letter, the shape a hand and a generated
		// command line both produce — could otherwise only spell the text a
		// second time, and nothing kept the two copies in step. The case then
		// tested something other than what it recorded, silently, from the
		// first edit to either one.
		for _, a := range c.Args {
			a = strings.ReplaceAll(a, ArgSnippet, c.Snippet)
			// The script file is written only if something asks for it, so
			// a case that never names it leaves no file behind.
			if strings.Contains(a, ArgScript) {
				a = strings.ReplaceAll(a, ArgScript, script())
			}
			args = append(args, a)
		}
	case c.Script:
		args = append(args, script())
	default:
		args = append(args, "-c", c.Snippet)
	}

	cmd := exec.CommandContext(ctx, sh.Path, args...)
	// A snippet must not inherit a user's environment: HOME, IFS or PATH from
	// the developer's shell would make the record depend on whose machine
	// produced it. A case may *add* to these four and may override one of
	// them by naming it — which is execve's own rule for a repeated name and
	// the only reading that lets a case ask about HOME — but it cannot lose
	// them by accident. Built here rather than in Exec so that a value may
	// name the scratch script the same way an argument does.
	cmd.Env = []string{
		"PATH=/usr/bin:/bin:/usr/sbin:/sbin",
		"HOME=" + dir,
		"LC_ALL=C",
		"TERM=dumb",
	}
	for _, kv := range c.Env {
		kv = strings.ReplaceAll(kv, ArgSnippet, c.Snippet)
		if strings.Contains(kv, ArgScript) {
			kv = strings.ReplaceAll(kv, ArgScript, script())
		}
		cmd.Env = append(cmd.Env, kv)
	}
	// Every route is invoked under the name its panel entry gives it. Which
	// shell a column names is settled by argv[0] and by nothing else, so a
	// route that skipped this recorded a *different shell* under the same
	// heading: the script route did, and the column headed bash-as-sh held
	// plain bash for every case that runs from a file. That is the
	// mislabeled-column hazard MustReport exists to prevent, and it is the
	// kind that never looks wrong — the rows were consistent with each other
	// and disagreed only with their own heading.
	//
	// It mattered by more than a name. Honoring it moved 18 of the 79 rows,
	// 13 of them in the recorded status, in four families: a failed special
	// builtin is fatal, so a readonly reassignment stops the script (status
	// 0 against the truthful 1); aliases are expanded in a non-interactive
	// script, so a command that was not found is found; `trap` prints INT
	// rather than SIGINT; and `trap` takes the POSIX usage. Every one of the
	// 18 old values was byte-identical to the plain-bash cell beside it,
	// which is the bug stated as evidence. A column whose Why is "argv[0]
	// alone changes the language" was the one column not being told its
	// argv[0].
	if sh.Argv0 != "" {
		cmd.Args[0] = sh.Argv0
	}
	// Standard input is closed unless the case asked for some. A nil Stdin is
	// os/exec's own spelling of that, and it is the right default twice over:
	// a shell with nothing to read must not wait for a terminal, and a case
	// that says nothing about input must not be handed whatever the harness
	// itself was started with.
	//
	// The placeholders are honored here too, so a case whose *program*
	// arrives on standard input writes it once as its Snippet. The script
	// file is only written if something asks for it.
	if c.Stdin != "" {
		in := strings.ReplaceAll(c.Stdin, ArgSnippet, c.Snippet)
		if strings.Contains(in, ArgScript) {
			in = strings.ReplaceAll(in, ArgScript, script())
		}
		cmd.Stdin = strings.NewReader(in)
	}
	return cmd
}

// normalize removes what is true of this machine rather than of this shell.
//
// Diagnostics quote the shell's own path and the script's path, both of which
// differ between a laptop and a CI runner. Without this the golden file would
// record where it was generated and drift for everyone else.
func normalize(s string, sh Found, dir string) string {
	rep := strings.NewReplacer(
		dir+"/case.sh", "<script>",
		dir, "<tmp>",
		sh.Path, "<shell>",
	)
	s = rep.Replace(s)
	// Some snippets mask their own digits before the harness sees anything —
	// the `times` cases do, because the figures are real timings — and `sed
	// -E "s/[0-9]+/N/g"` cannot tell a timing from the digits in the shell's
	// own path. So a binary living under a directory with a number in it,
	// which is every macOS TMPDIR, arrived here already spelled differently
	// from sh.Path and went unreplaced: `make conformance-dialects` scored
	// zsh one lower than the same corpus run against the same code built in
	// /tmp. Where the binary sits is not a fact about the shell, so the
	// masked spelling is replaced too.
	if masked := digitRuns.ReplaceAllString(sh.Path, "N"); masked != sh.Path {
		s = strings.ReplaceAll(s, masked, "<shell>")
	}
	// A shell names itself by basename at the start of a diagnostic. Replacing
	// that basename anywhere would corrupt ordinary words — with sh, "can't
	// shift that many" became "can't <shell>ift that many" — so it is anchored
	// to the start of a line and required to be followed by a colon.
	s = diagPrefix(filepath.Base(sh.Path)).ReplaceAllString(s, "<shell>:")
	s = tracePrefix(filepath.Base(sh.Path)).ReplaceAllString(s, "+<shell>:")
	s = usageBlock(s, filepath.Base(sh.Path))
	// A shell invoked under another name reports *that* name, not its
	// binary's — bash-as-sh says "sh:", which the line above cannot match.
	if sh.Argv0 != "" {
		s = diagPrefix(sh.Argv0).ReplaceAllString(s, "<shell>:")
		s = tracePrefix(sh.Argv0).ReplaceAllString(s, "+<shell>:")
		s = usageBlock(s, sh.Argv0)
	}
	// A shell asked to be interactive with no terminal names the process
	// group it could not set, which is this run's own process id: a different
	// number every time, and one that says nothing about the shell. Masked
	// rather than dropped, because *that it named one* is part of the
	// complaint — and left in, it made every regeneration of the record a
	// diff on every case that asks about `-i`.
	s = processGroupID.ReplaceAllString(s, "process group (N)")
	s = strings.TrimRight(s, "\n")
	// Newlines are shown as ~ so a result stays one table cell. Real output
	// containing ~ is rare enough that the ambiguity has not bitten; if it
	// does, this is the place to escape it.
	return strings.ReplaceAll(s, "\n", "~")
}

// digitRuns is the masking a snippet applies to its own output, spelled the
// same way `sed -E "s/[0-9]+/N/g"` spells it.
var digitRuns = regexp.MustCompile(`[0-9]+`)

// processGroupID matches the process id in a job-control complaint. See
// normalize, which explains why it is masked rather than kept.
var processGroupID = regexp.MustCompile(`process group \(\d+\)`)

// diagPrefix matches a shell naming itself at the start of a diagnostic.
// tracePrefix matches a shell naming itself inside a `set -x` line, which is
// the one place the name is not at the start.
//
// zsh writes `+zsh:1> echo a b`, so the line-anchored rule above cannot see
// it — and without this the trace cases compared our path against zsh's name
// and every one of them looked like a divergence.
func tracePrefix(base string) *regexp.Regexp {
	return regexp.MustCompile(`(?m)^\+` + regexp.QuoteMeta(base) + `:`)
}

func diagPrefix(base string) *regexp.Regexp {
	return regexp.MustCompile(`(?m)^` + regexp.QuoteMeta(base) + `:`)
}

// usageBlock rewrites a shell naming itself in the usage text it prints when
// it refuses an invocation, which is the other place the name is not at the
// start of a line.
//
// A shell refusing an option prints `Usage: ksh [-cilrs…]`, and ksh93 writes
// the *last element* of the word it was invoked by there where bash writes
// the whole of it. So the path replacement above catches bash's and cannot
// catch ksh93's, and the corpus compared `Usage: ksh` with `Usage: our-ksh` —
// a difference in where the binary lives rather than in what the shell does.
//
// A block rather than a line, because the usage text runs over several and
// names the shell on more than one of them:
//
//	Usage:	sh [GNU long option] [option] ...
//		sh [GNU long option] [option] script-file ...
//
// The second line is indented and so out of reach of anything anchored to
// `Usage:`, which left a literal `sh` in the bash-as-sh column where every
// other column read `<shell>`. That was cosmetic — argv[0] is fixed by the
// harness, so the record was stable — but an asymmetry in a normalized record
// is exactly what a later reader mistakes for a finding, and nothing in the
// row said whether the bare name was deliberate (#667).
//
// It is scoped to the block rather than matching an indented name anywhere,
// which would be a rule about indentation rather than about usage text and
// would rewrite any output that happens to list the shell's own name. The
// block opens on the `Usage:` line and closes at the first line that is not
// indented — `GNU long options:` in the text above — so the option lists
// after it are outside it and untouched.
func usageBlock(s, base string) string {
	header, cont := usagePrefix(base), usageContinuation(base)
	lines := strings.Split(s, "\n")
	inUsage := false
	for i, line := range lines {
		switch {
		case header.MatchString(line):
			lines[i] = header.ReplaceAllString(line, "${1}<shell>")
			inUsage = true
		case !inUsage:
		case cont.MatchString(line):
			lines[i] = cont.ReplaceAllString(line, "${1}<shell>")
		case !indented(line):
			inUsage = false
		}
	}
	return strings.Join(lines, "\n")
}

func indented(line string) bool {
	return strings.HasPrefix(line, " ") || strings.HasPrefix(line, "\t")
}

// usagePrefix opens a usage block: the name right after `Usage:`. Anchored to
// the start of a line and required to follow `Usage:` so an ordinary word
// spelled like the shell is left alone.
func usagePrefix(base string) *regexp.Regexp {
	return regexp.MustCompile(`(?m)^(Usage:[ \t]+)` + regexp.QuoteMeta(base) + `\b`)
}

// usageContinuation is the name at the head of a line continuing one, which
// only usageBlock may apply and only inside a block it has already opened.
func usageContinuation(base string) *regexp.Regexp {
	return regexp.MustCompile(`(?m)^([ \t]+)` + regexp.QuoteMeta(base) + `\b`)
}
