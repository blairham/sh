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
reference shells proved it on different machines. `zsh` with `ZDOTDIR`
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
