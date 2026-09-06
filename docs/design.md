# Architecture

A shell parser and interpreter in Go whose core language is the common
denominator of real shells, whose dialects are presets over a semantics
vector, and whose execution is observable and gateable from the inside.

Read `CLEANROOM.md` before writing code, and `docs/spec/` before
designing behavior.

## The three layers

1. **Grammar** — a variant set on the parser. Constructs are additive:
   a dialect enables constructs the core does not have. See
   `spec/shell-matrix.md`.
2. **Semantics** — a vector of named switches for the places dialects
   disagree about identical syntax. See `spec/semantics.md`.
3. **Dialects** — named presets (`posix`, `core`, `bash`, `zsh`, `ksh`)
   that select a variant set and a semantics vector. A dialect is a
   *table of values*, never a code path.

A dialect is runtime state, re-read as execution proceeds — `set -o
posix` halfway through a script governs the rest of it, including the
parse of the rest of it. Parser configuration is therefore derived from
interpreter state on demand, never fixed at construction.

## Agent, AI and sandbox are core, not wrappers

These are first-class requirements, and the reason is structural rather
than aspirational.

**A policy enforced outside the interpreter does not reach inside it.**
`eval`, `source`, command substitution and subshells all re-enter the
interpreter with their own input. Anything that must hold universally —
a sandbox boundary, an audit trail, a permission prompt — must live at
the point of execution, or it is bypassed by the first `eval`. A shell
that sandboxes by wrapping the process it spawns has sandboxed nothing
that matters.

So the seams are interpreter-internal from line one.

**All three requirements are the same shape.** Sandboxing gates
execution. An agent protocol streams what happened and relays permission
questions. An AI assistant needs structured context about what ran and
what failed. Each is either *gating* an action or *observing* one, so
two primitives serve all three:

| primitive | question it answers | serves |
| --- | --- | --- |
| **gate** | may this action proceed — allow, deny, or ask? | sandbox policy; agent permission prompts |
| **event** | what just happened, structured? | agent streaming; AI context; audit; trace |

Every action that leaves the interpreter's own memory passes both:
process execution, file open, file stat, directory read, each redirect,
and a signal aimed at a process. That set is the syscall-shaped surface
of a shell, and it is the complete boundary.

The sixth arrived late and is the reason this sentence is worth
distrusting. `kill` began as an external program, where it was an exec
like any other and the gate saw it; making it a builtin — which a shell
has to, so that a trapped `kill -INT $$` is delivered rather than
raced — moved `kill -9 1234` from inside the boundary to outside it, and
for a while a denying policy could not stop the shell from ending
somebody else's process and no audit trail recorded that it had. Two
independent sweeps found it, which says the claim above is the kind that
has to be re-checked against the code rather than read.

The same sweep found the other shape of the same mistake, and it is
worth naming because nothing about it looks like a hole. `cd -P` and
`pwd -P` report where a directory *is* rather than the name it was
reached by, which means following every symlink on the way; the standard
library will do that in one call, and that call lstats and reads each
component through the os package. So the gate saw one stat — of the
answer, on a path it had no say in reaching — while a subtree it was
refusing had already been walked through and reported on. The lesson is
that an action can leave the boundary inside a library call that looks
like arithmetic on a string: the walk is now written out over the gated
primitives, one component at a time.

The exemptions are deliberate and each is documented on the action
vocabulary itself. The scaffolding a process substitution stands on —
the temporary directory made for its pipes, the mkfifo, their removal —
is the interpreter's own plumbing on paths the script never chooses, and
gating it would let a policy refuse the mechanism while believing it
refused an access; the access is the open of the pipe, and that is gated.
A signal a script aims at *this* shell is the other: it never reaches the
kernel at all, because a trap for it runs the shell's own handler and an
untrapped fatal one stops the script and leaves the dying to the driver,
so there is no action to refuse. The gate therefore sits at the system
call rather than at the builtin, and the two statements stay one
statement. `fg` and `bg` are outside it for a narrower reason: they
resume a job this shell started, with a fixed SIGCONT to a group that
already passed the exec gate on its way to existing, so a policy that
did not want that process running had its say when it started.

**The front end has accesses too, and they were the tail of this.** A
shell opens files before any script runs and after it has finished: the
program named on the command line, `~/.profile` and `$ENV`, the history
a session keeps. Those went to the os package directly, which was
invisible while nothing supplied a gate and stopped being invisible the
moment `driver.Shell` grew one — a policy handed to a binary that then
read the very path it refuses is a boundary that reports more than it
enforces. All three are inside it now, through `internal/boundary`, and
the test of which side a front-end access falls on is **who chose the
path**: a script operand comes from the invocation, `$ENV` and `HISTFILE`
from shell variables a typed line can set, so all of them are the
policy's business. The vocabulary stays the interpreter's; the front end
only asks.

**And the front end's opens are verified, not only consulted about.**
`internal/boundary` used to answer a bool and leave the open to its
caller, so nine call sites each did their own `os.Open` afterwards and
the check was about a *name* while the read was about an object — the
defect `docs/design/sandboxing.md` closes for the interpreter, still
open for the shell. `HISTFILE` is the one a typed line can aim. The
boundary makes the descriptor itself now and asks the kernel what the
open reached, which is the only arrangement where the check cannot be
forgotten: there is no longer a way to ask this package's permission and
then open something else. The platform half is shared with `interp`
through `internal/opened` rather than copied, because a syscall wrapper
that exists twice drifts and the copy that drifts is the one nobody is
reading.

The rest of the front end's own file work is outside, and here is the
whole list, so that silence is never the record:

- **`/dev/tty`, opened to hand the terminal to a process group.** A fixed
  path, chosen by the front end, and the open reads nothing — the
  descriptor exists to name the terminal in an ioctl. Gating it would
  stop `^C` and `^Z` reaching commands while protecting no file. A
  redirection a script writes to `/dev/tty` is an ordinary open and is
  gated.
- **`os.Stat(os.DevNull)`, deciding whether a stream is a terminal.** The
  same shape: a fixed interpreter-chosen path, asked about a descriptor
  the caller already handed over, teaching the shell nothing about the
  filesystem. Gating it would let `-deny /dev/null` turn the null device
  into a terminal, which changes behavior without refusing anything
  reachable.
- **Tab completion's directory listing.** `<Tab>` lists a directory a
  person is typing into, and by the rule above that path was chosen by
  the policy's subject — so this one is on the wrong side of the line
  and is here as a known gap rather than as a decision. Issue #951
  carries the argument; the guard in `internal/boundary`'s tests names
  it so it cannot go quiet.
- **`umask` and the resource limits.** These name no path and no program.
  They do not perform an access; they change what a later access creates
  with or is allowed to do, and that later access is itself gated and
  recorded. They are also already *hooks* — nil in a library, filled in
  only by a binary that is a shell — so a program that wants a say
  installs its own and can refuse, where an event kind could only report
  after the fact. That is the same promise `ActionInherit` is documented
  for declining to make.

What a refusal looks like follows the rule the file probes set. A denied
signal answers EPERM — the errno for a process this one may not signal —
so `kill` reports it in each dialect's own wording for that, and a policy
hiding a process is indistinguishable from a kernel refusing one, exactly
as a denied stat is indistinguishable from a path that is not there.

**What lives here and what does not.** The substrate owns the gate, the
event stream, and the structured representation of an error — what
failed, its status, its position in the source. What a command *wrote* is
deliberately not copied into that record: the streams are the caller's
own io.Writers, handed over before anything ran, so a consumer that wants
output taps the writer it supplied rather than receiving a second copy of
what it already holds. The substrate does not
own OS sandbox backends, protocol transports, or model providers. A
library that imports an AI SDK is a library nobody adopts; the seams are
here, the implementations sit above.

**A binary supplies the value; the interpreter owns the meaning.**
`driver.Shell` carries a `Gate` and an `Events` sink and hands both to
every Runner it builds — one place, so the `-c` route, a script file,
standard input and a prompt cannot disagree about whether a shell is
gated. Nil is the default and means what it has always meant: allow
everything, discard every event, one nil check on the hot path. What
the interpreter *asks about*, what a decision does, and what an event
carries are not the front end's to change; a driver that decided any of
that would be a policy applied outside execution, which is the thing
this whole seam exists to rule out.

The rule that a seam nothing reaches is a seam nothing grades applies
here as hard as anywhere. The gate was unit-tested from its first commit
and no shipped binary ever set one, so the conformance harness and the
wild sweep both ran ungated and a hole in the boundary would have looked
exactly like a shell that works. `cmd/sh` therefore has a debug route
onto it — `-trace-events` prints the stream, `-deny` refuses an action —
and that route is a way to *watch* the gate, not a sandbox.

**A hole in the route has the same property as a missing route.** `-deny`
was a list of paths, and a signal names a process rather than a file, so
for a while a signal could be watched and not refused: nothing failed,
and the surface that exists to grade the seam had a kind it could not
speak about. It now takes a rule in `internal/policy`'s language — a
selector and a pattern, `-deny signal`, `-deny exec:/usr/bin/**`, with a
bare path as the shorthand it always had — so the vocabulary is the
action kinds rather than paths, and there is one matcher rather than two.
A test reads the `ActionKind` block out of `interp/seams.go` and fails
when a kind lands that `-deny` cannot refuse, because a vocabulary
without that guard only moves the gap to the next kind.

The distinction is worth stating because the boundary is drawn around
the interpreter and not around the process tree. `-deny /secret` hides
`/secret` from the shell's own file tests, globs and redirections, and
does nothing at all to a `cat /secret/f` the shell was allowed to start:
the gate refuses *the shell's* accesses, and a child process makes its
own. Containing what a command does once it is running is the job of an
OS sandbox backend, which sits above this and is what a real `Gate`
implementation would reach for.

**The gate is asked about a name, and a name is not a file.** It is
handed the path the interpreter holds, and the interpreter does not
resolve links — so two names for one file are two questions to it, and a
rule about one says nothing about the other. There are three answers to
that, not one, and they act at three different times.

A name the *platform* fixes is answered when the rule is read: `/tmp` is
`/private/tmp` on every macOS machine, so a policy expands that one
once, at parse time, and matches either spelling. An ordinary symlink is
answered at the *open*, by asking the kernel what the descriptor it just
returned actually holds — which reads nothing the gate was not already
given, and has no time-of-check race in it, because a descriptor pins an
object. And two *real* names for one object — a hard link, a bind mount
— are not answered here at all, because that needs matching on identity
rather than on paths, which is the OS backend above.

This paragraph exists because it is the second thing a reader of the
seam has had to rediscover; `design/sandboxing.md` has all three.

[design/sandboxing.md](design/sandboxing.md) is the shipped policy that
fills the seam: the policy format, the default posture, what a refusal
looks like, and the event schema its consumers share. The agent-protocol
consumer of that seam is designed in [design/acp.md](design/acp.md), and
[design/blocks.md](design/blocks.md) is the at-rest form of the same
unit: a command and its output, recorded.
[design/plugins.md](design/plugins.md) is a builtin whose implementation
is another process: the same seams, remoted, so a command can be written
in a language other than Go — and what launching one costs at the gate.

## The process model is decided now

Background jobs are **real process groups**, not goroutines.

This is recorded as a decision because it is the expensive kind. A shell
built on goroutine-backed jobs cannot later give honest answers for job
control, signal delivery, terminal ownership or `set -m`, and cannot
place a sandbox boundary or a permission gate around a process group
that does not exist. Retrofitting it means rewriting execution.

## The runtime's descriptors are kept out of a script's reach

A shell written in Go shares its descriptor table with a runtime that
opens descriptors of its own and does not say which. A script names
numbers — `exec 5>f` — and `exec cmd` has to put that file on that
number before the `execve`. When the number is one the runtime holds,
the process does not misbehave, it dies: `netpoll failed` on Linux,
where the poller's epoll descriptor was overwritten, and `signal_recv:
inconsistent state` on macOS, where the signal pipe was.

It is not bad luck, and that is the part that made it fixable. Opening
the script leaves the low numbers briefly free, so the poller lands on
5 — which is the number a script parks on next after 3 and 4. Two low
numbers, both chosen by accident, colliding by construction.

So the shell takes those numbers first. Before anything else runs,
`driver` holds every descriptor from 3 to 99, makes the runtime open
everything it opens lazily — the poller, and on macOS the signal pipe
— and hands the range straight back. The runtime ends up above 99,
where nothing a script names can reach it, and the hold is over before
`main`: measured at 50µs on macOS and 37µs on Linux, an order of
magnitude below the run-to-run spread of the startup measurement in
[design/startup.md](design/startup.md).

The window between placing a descriptor and the `execve` is still
there. It cannot be closed — `execve` on a multithreaded process kills
the other threads inside the call, and until it has they are running
against a table that has already changed. What is fixed is that there
is no longer anything in the window to hit.

**The limit, recorded rather than claimed away.** The ceiling is 99
because that is where a hand-written descriptor number stops. Only bash
can *name* a descriptor above 9 at all — dash, ksh93 and zsh read `exec
20>f` as a command called `20` — the automatic form `exec {v}>f`
allocates a number the kernel has just said is free and so cannot
collide by construction, and 424 shell scripts shipped on a developer
machine name nothing above 5. A bash script that names a descriptor
just above the ceiling by hand does still meet the runtime's own. It no
longer dies of it: the shell knows those numbers and leaves them alone,
so the command runs with that one descriptor missing, which is what
every other unplaceable number already costs it.

## What is deliberately not here

- **fish.** fish is not a Bourne descendant and shares no grammar below
  the command name — assignment is a command, blocks end with `end`,
  every variable is a list. There is no switch that turns this core into
  fish; it would be a separate front-end. The parser's own vocabulary
  reflects this: dialects are bash, ksh, mksh and zsh.
- **A bash-complete long tail.** Compatibility is measured and published,
  not claimed. The core is the common denominator; bash's corners belong
  to the bash dialect and arrive on evidence of demand.
