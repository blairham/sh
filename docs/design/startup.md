# How long it takes to start

A shell that starts slowly is felt twice: on every new terminal a person
opens, and on every subshell a script spawns. Neither had ever been
measured here, so this records the measurement, the method, and what it
found.

The short version: **startup is not a problem, and there is nothing to
optimize.** Both routes cost about four milliseconds, which is roughly
half of what bash costs and level with zsh on the same machine in the
same minute. An rc file of ordinary size costs nothing measurable.

## The two numbers, and why they are two

`-c` is what a script's every subshell pays. It is over before anything
is drawn, and nothing about a terminal is involved.

**Process start to first prompt** is what a person waits for. It needs a
real terminal, because a prompt is exactly the thing a shell declines to
draw without one — which is what `internal/pty` exists for.

They are measured together and against the real shells, because a
millisecond means nothing on its own. `internal/startupcost` is the
harness; the method is committed and the numbers are not, for the reason
`AGENTS.md` gives about conformance scores. Run it with `make startup`, which is:

    go test ./internal/startupcost/ -run XXX -bench . -benchtime 40x -count 3

## What it found

Apple M5 Max, macOS 25.5, Homebrew bash 5.3.15 and zsh 5.9.2, `-benchtime
40x -count 3`, median of the three runs, milliseconds. The machine had
other work on it, and the run-to-run spread was about a tenth of each
figure — which is the precision this claim needs and no more.

| | ours, bash dialect | ours, zsh dialect | bash 5.3 | zsh 5.9 |
| --- | --- | --- | --- | --- |
| `-c :` | 4.3 | 3.8 | 8.6 | 4.6 |
| first prompt, no rc | 3.7 | 3.5 | 7.7 | 8.7 |
| first prompt, with an rc | 4.2 | 4.2 | 8.4 | 9.2 |

Four things are worth taking from that.

**We are about half of bash on both routes**, and level with or ahead of
zsh. Whatever else is worth working on, this is not it.

**An rc file costs about half a millisecond**, and costs every shell in
the table the same half. The file is fifteen lines of aliases, exports
and functions, which is what an rc is mostly made of. Each shell is
pointed at the file it names for itself — `.bashrc`, `.zshrc`, in a home
directory of its own — so this is the route a real start takes and not a
back door for measuring. That route only exists here because #807 landed
first; measured against `$ENV` beforehand, the answer was the same.

**A dialect is free.** The bash and zsh columns are the same code with a
different value filled in, and they measure the same, which is what
"a preset is data rather than a branch" is supposed to mean. Dialect
preset construction was the first suspect if the number had been bad; it
is not on the board at all.

**Almost all of it is process start.** Four milliseconds is what this
machine charges to fork, exec and bring up a Go runtime. There is no
first-prompt work worth naming underneath it, so the only way to make a
prompt appear faster is to not start a process — which is a different
feature, not an optimization of this one.

## Three things the harness had to learn

All three were measurement bugs that produced confident wrong answers,
which is the failure mode worth writing down.

**A benchmark times the whole call, and the whole call is not the
start.** Ending a shell means killing a session leader that owns a
controlling terminal and reaping it, and that teardown costs zsh about
190ms where it costs the others under one. Reported as `ns/op`, that made
zsh look like it took two hundred milliseconds to draw a prompt when the
same binary draws one in eight. The benchmarks now report the time
`RunPrompt` measured rather than the time the loop took.

Ending one turned out to need care of its own. A shell is a thing that
starts other things, so the signal goes to the session rather than to the
one process; the terminal is closed first, so a shell blocked writing into
a buffer nobody is draining is let go rather than left waiting; and the
wait for it to be reaped is bounded, because an unbounded one hung a CI
run for the whole ten minutes `go test` allows — after the measurement it
was tearing down had already finished. A stuck teardown costs one leaked
process. An unbounded one costs the build.

**A shell reads more than the file it was pointed at**, and both of the
reference shells proved it on different machines — and since #1717 both of
ours do too, which is why the zsh subject here is now given `-d` the way
the reference one always was. `zsh` with `ZDOTDIR`
set also reads `/etc/zshrc`, which on macOS sets a prompt of its own;
`bash` with `--rcfile` also reads `/etc/bash.bashrc`, which on Ubuntu
does the same. Either way the sentinel this harness watches for was
silently replaced and the run waited for a mark that was never coming.

Two answers, because the shells offer two. zsh has `-d`, which turns the
global files off — so its pair is a measurement of *this* rc and nothing
else. bash has no such flag short of `--norc`, which would also refuse
the file being measured, so the rc itself ends by drawing the sentinel:
it is read last, so a prompt set there survives whatever the machine's
own files did. The cost of a system-wide interactive rc is therefore
folded into bash's with-an-rc figure on a machine that has one. macOS,
where the table above was measured, does not.

The second one is also why the harness kills a shell that has not drawn a
prompt instead of waiting: a deadline on the pseudo-terminal descriptor
never fired, because it is not a descriptor the Go runtime polls.

**The rc a harness writes is not the rc a person has.** Everything above
was measured on fifteen lines of aliases, and on that file the table is
true: we are about half of bash and level with zsh. On the maintainer's
actual configuration — a plugin manager, 31 plugins, 136 sourced `.zsh`
files — the same binary took **7.82s against real zsh's 0.14s**, which is
56x, and "unusably slow on start up, that is not a daily driver" (#1383).

Both numbers were honest. Only one of them was about a shell anybody
uses, and the instrument could see only the flattering one, which is how
a 56x gap survived a hundred merged changes in a single day. The
paragraph above saying there is "no first-prompt work worth naming"
underneath process start is exactly the conclusion a synthetic rc
licenses and a real one refutes.

What the synthetic rc had none of, and what a real one is mostly made of:

1. **Many files, sourced.** A plugin manager's cost is `source` called
   several dozen times, each file defining functions the next one wraps.
2. **Pattern work.** This is where the 7.8s was. A profile put **96% of
   the whole startup in `interp.matchHere`** — 85 calls to one
   prompt-theme substitution, whose pattern nests alternation two deep
   over `##` closures and mostly fails to match. A failing match is the
   expensive one, because a matcher that found nothing has explored
   everything: 11.2 million recursive calls for an 82-byte subject, when
   only about 5,600 distinct questions exist to ask. It grew as the
   **fourth power** of the subject's length.

So `internal/startupcost` now measures a second rc as well — forty
sourced files and one pattern-heavy substitution, the *shape* of a real
configuration rather than a copy of one, since a test cannot depend on a
plugin manager it would have to fetch. It is not subtle about the
difference. Run against the commit before the fix, the synthetic case
still reported `ours-zsh` at 5.9ms against zsh's 8.1ms — ahead — while on
the rich case `ours-bash` took 1448ms against bash's 41ms and `ours-zsh`
**never reached a prompt at all**.

**And an instrument must check its own output.** The gate that makes the
rich case honest is that the sentinel is drawn *only* if the substitution
produced the answer the whole panel gives. Without it the case would
measure a refusal: this is the third time that has happened here — an
agent's benchmark on this repository showed no regression whatsoever
because `syntax.Core()` rejected the construct under test and the output
went to `io.Discard`, so the clock faithfully timed a shell doing
nothing. A shell that will not do the work now fails to start rather than
posting a very good number, and a test deliberately moves the expected
answer out of reach to prove the gate is wired the right way round.

The first draft of the rich case fell into it immediately, and in the
dullest possible way: the pattern work sat in the rc without `setopt
extendedglob`, so every metacharacter in it was an ordinary character,
the substitution replaced nothing, and the case ran in 10ms while looking
entirely plausible.

## The gate, and the floor underneath it

Everything above measures. `make perfgate` **fails**, and it is the
release bar for v0.0.0: *"performance is a must requirement before
0.0.0, no dialect can be slower than the original"*, and *"our zsh MUST
be faster than zsh"* — faster, not comparable (#1403).

It grades all four dialect binaries against the shells they claim to be,
on the `-c` route, which is the acceptance criterion #1403 states. The
references are the oracle panel's own entries, because the shell being
compared against has to be the shell the corpus was recorded from.

Two programs per dialect, and the second one is why the first is
trustworthy:

- **bare** is `-c ':'`, a builtin that does nothing in all six panel
  members. It is what a script's every subshell pays. It cannot check
  its own work, so it is additionally required to exit 0.
- **workload** is a two-thousand-iteration `while`, a `for` over ten
  words, a `case`, and two substring expansions, written in the
  common-denominator language so that all four dialects and all four
  references run the same text. It **prints what it computed**, and a
  time is accepted only if the run also produced
  `ANS:2017:defghij:abcdefg`.

That check is the whole reason the workload is shaped that way rather
than as a bare loop. This repository has now been caught three times by
an instrument that timed a shell *refusing* the construct under test —
the clock faithfully measures a shell doing nothing, and a refusal posts
the best number in the table. A fourth trap of the same family is a
reference shell that is not installed: the gate reports that as **NOT
MEASURED** and fails on it, rather than letting a machine missing half
the panel read as green.

Three things about the method, each of which was wrong once:

**Interleaved.** Within a batch each subject gets one invocation in
turn, so a load spike lands on ours and on the original alike. Measuring
one subject to completion and then the next lets a busy machine invent a
2x difference between two copies of the same binary.

**Minimum, not mean.** Measured here under load averages between 8 and
31 — other agents run on this machine — the batch mean moved by a factor
of three between batches while the minimum moved by a tenth. The minimum
is the sample that got a clean run; a mean is a measurement of the load.
The load average is printed above every table for that reason.

**No tolerance.** Strictly faster. A gate that allowed 5% would be
answering a question nobody asked.

The gate is a target of its own and not part of `go test ./...`, for the
reason `make startup` is: it spawns several thousand processes and its
answer depends on what else the machine is doing. What keeps it from
rotting is that the tests *around* it always run — one manufactures a
subject doing strictly more work than the same shell and requires the
gate to catch it, one hands it a program that runs nothing and requires
the gate to refuse rather than score it, one checks the workload's
expected answer against every reference shell installed, and one fails
if a dialect is missing from the table altogether.

### What it found, and the part that is not ours

The floor was measured before optimizing anything, interleaved, 240
samples each, load 18:

| | size | wall-min | cpu-min |
| --- | --- | --- | --- |
| `/bin/dash -c ':'` | 120K | **1.30 ms** | 0.83 ms |
| empty **C** binary | 17K | 1.36 ms | 0.85 ms |
| empty C padded to 6.1M | 6.1M | 1.40 ms | 0.83 ms |
| empty **Go** binary | 1.7M | **1.92 ms** | 1.28 ms |
| empty Go padded to 8.2M | 8.2M | 2.08 ms | 1.42 ms |

**The Go runtime costs 0.56 ms over an empty C binary, and real dash —
doing actual work — starts faster than an empty Go binary that does
nothing.** Binary size is nearly free: 6 MB of padding costs C 0.04 ms
and Go 0.16 ms, so size is not the lever either.

So `ours-dash` faster than `/bin/dash` is **not reachable in Go**. The
gate says so rather than being widened to hide it; whether v0.0.0's bar
keeps that column is a decision for the maintainer, not for the
instrument.

An earlier attempt at these figures reported "empty C 2.06 ms, empty Go
2.05 ms, Go pays no startup penalty". That does not reproduce, and the
reason is the fourth measurement trap: a harness whose own overhead is
within an order of magnitude of what it measures is measuring itself.

### Two things that were ours, and were removed

A binary importing `driver` and a dialect, whose `main` does nothing at
all, cost 2.84 ms against the empty Go binary's 1.92. So most of our own
fixed cost was being paid **before `main` ran**, which is not where
#1403 expected it — the three vectors turned out to be free, `dash`
constructing all three and running `-c ':'` in 0.15 ms.

**`user.Current()` on every invocation.** `dialect/zsh` asked the system
for the login name in `Apply`, to fill the `%n` prompt escape. Measured
cold: 0.83–1.10 ms, and 0.81–1.29 ms with `CGO_ENABLED=0` too, so it is
macOS Directory Services either way rather than cgo. Every `zsh -c` and
every subshell paid a millisecond to learn a name that route can never
draw. It is now carried in as a *question* — `SetPromptUserFunc` — asked
when `%n` is drawn and remembered afterwards. The rule it was written
for is untouched: `interp` still does not ask the system anything, it
calls back what the binary handed it.

**A credential scanner compiled at package init.** `driver` links `repl`
so a shell can prompt, `repl` links `internal/secret` so a prompt can
redact, and Go initializes a linked package whether or not the route
that wants it is taken. `GODEBUG=inittrace=1` charged every `sh -c` in
the tree **0.34 ms and 2228 allocations** to compile a dozen regular
expressions for a prompt it was never going to draw — the largest single
entry in the whole trace, ours or the standard library's. One
`sync.OnceValue` moves it to the first line that is redacted.

Together, interleaved, 200 samples each, load 8.4:

| dialect | before | after | original |
| --- | --- | --- | --- |
| zsh | 4.07 ms | **3.19 ms** | 2.90 ms |
| ksh | 2.94 ms | **2.81 ms** | 2.77 ms |
| dash | 2.88 ms | **2.70 ms** | 1.31 ms |

### What is left, measured and not guessed

**The prelude, on every invocation.** `bash` and `zsh` ship ~190 lines
of shell defining `dirs`/`pushd`/`popd`/`__dirs_rotate`, and
`driver.Shell.source` parses and runs the whole thing before the script's
first line — every `-c`, every subshell, whether or not anything calls
them. In-process, with `-benchmem`:

| | ns/op | B/op | allocs/op |
| --- | --- | --- | --- |
| `syntax.Parse(bash prelude)` | 322,000 | 238,681 | 3,954 |
| `syntax.Parse(zsh prelude)` | 332,000 | 203,423 | 3,409 |
| `syntax.Parse(ksh prelude)` | 964 | 1,928 | 22 |
| whole `-c ':'`, bash, with prelude | 365,000 | 260,231 | 4,064 |
| whole `-c ':'`, bash, prelude removed | **26,900** | 18,512 | 54 |
| whole `-c ':'`, zsh, with prelude | 392,000 | 248,922 | 3,646 |
| whole `-c ':'`, zsh, prelude removed | **79,000** | 44,458 | 214 |

**The prelude is 93% of bash's in-process startup and 78% of zsh's** —
0.33 ms and 240 KB of allocation in a process that has not yet grown a
heap. Making it lazy needs the core to be able to install a function
definition on first reference, which is a change to argue for on its own:
`type pushd`, `declare -f`, `command -v` and completion all have to keep
seeing it, and a name-triggered source that missed one of those would be
a correctness bug wearing a speedup.

**Interpreter throughput, which is a different problem.** The gate's
workload separates startup from execution, and the difference is not
flattering. Subtracting each dialect's bare figure from its workload
figure, load 26:

| dialect | ours, work alone | original, work alone |
| --- | --- | --- |
| bash | 3.36 ms | 5.37 ms |
| zsh | 4.31 ms | **2.38 ms** |
| ksh | 4.03 ms | **1.60 ms** |
| dash | 2.89 ms | 4.16 ms |

We are ~1.8x slower than zsh and **~2.5x slower than ksh93** at running
a two-thousand-iteration arithmetic loop, and faster than both bash and
dash at the same thing. #1403 named startup as the problem; on the
workload half, startup is not where the gap is. That is its own piece of
work and it is not in this note.

**`driver`'s own init, at 0.24 ms**, is `keepTheRuntimeOffTheLowDescriptors`
— about 230 syscalls to hold the low descriptor range while the runtime
takes its own from above it. That is the price of a documented crash
class, not an oversight; cutting it means lowering the ceiling, which is
a semantic decision and not a performance one.
