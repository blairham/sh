# Running a command under a pseudo-terminal

`zsh/zpty` was the last module of #1320's table this shell refused. It
offers one builtin, `zpty`, which runs a command with a pseudo-terminal
of its own so that a program insisting on an interactive environment can
be driven from a script.

**It is implemented as of #3748**, on a new public seam on the substrate
— see *The decision that had to be made first*, which is the part to read
before the rest. This document stays what it was: a behavioral contract
measured from the reference and written in our own words. What changed is
that there is now code answering to it.

Sources: `zshmodules(1)`, section *THE ZSH/ZPTY MODULE*, and oracle runs
against zsh 5.9.2 (`/opt/homebrew/bin/zsh`, aarch64-apple-darwin25) on
2026-09-19, 2026-09-20 and 2026-09-21. Every table below is the first run
unless it says otherwise.

## The forms

    zpty [ -e ] [ -b ] name [ arg ... ]     start
    zpty -d [ name ... ]                    delete
    zpty -w [ -n ] name [ string ... ]      write
    zpty -r [ -mt ] name [ param [ pat ] ]  read
    zpty -t name                            is it still running
    zpty [ -L ]                             list

## Starting

**The arguments are joined with spaces and run as if passed to `eval`.**
They are not an argv. The measurement that settles it is a parse error
rather than a quoting difference:

    v=inherited
    zpty P print -n "<$v>"

    (zpty):3: parse error near `>'
    zpty:4: no such pty command: P

The word the shell built was `print -n <inherited>`, and re-reading that
made `<inherited>` a redirection. Two further facts come with it: the
name is **not** registered when the command will not parse, and a whole
pipeline is a legal command — `zpty P "print -n abc | tr a-z A-Z"` reads
back `ABC`.

**The command is the calling shell.** A function defined in the caller
runs under the pty, and a parameter set in the caller is readable there:

    f() { print -n "from-f"; }
    zpty P f            →  reads back  from-f

**And it really is a terminal.** `zpty P "[[ -t 1 ]] && print -n TTY"`
reads back `TTY`.

**`$REPLY` is set to the master descriptor.** Measured as a small
number — 11 in a `-f` shell — and it is a descriptor this shell can use:
`sysread -i $REPLY got` reads the command's output into `got`. The
manual recommends `-r` and `-w` over that, and records the descriptor so
a ZLE handler can watch it.

| probe | answer |
| --- | --- |
| `zpty P cat` | 0 |
| `zpty P cat` a second time | `pty command name already used: P`, 1 |
| `zpty P` | `missing command`, 1 |
| a command that will not parse | the parse error, and no name registered |

### `-e` and `-b`

`-e` sets the pseudo-terminal up so input characters are echoed; without
it they are not. **This is not visible through `cat`**, which echoes
what it is given either way, so the pair below is a *control that does
not discriminate* and is recorded as such rather than as evidence:

    zpty A cat;    zpty -w A one; zpty -r A l    →  one\r\n
    zpty -e B cat; zpty -w B one; zpty -r B l    →  one\r\n

The `\r` in both is the terminal's output discipline turning the newline
into a carriage return and a newline.

**The probe that does discriminate runs a command that neither reads nor
writes**, so that anything coming back is the terminal's own echo and
nothing else. Measured 2026-09-20 on zsh 5.9.2, every pty non-blocking so
that no read can hang:

    zpty -b -e E 'sleep 2'; zpty -w E hello; sleep 0.4; zpty -r E l
    status 0, l = "hello\r\n"

    zpty -b    D 'sleep 2'; zpty -w D hello; sleep 0.4; zpty -r D l
    status 1, l unset

So the manual's sentence holds and **echo is off by default**, which is
the setting an implementation has to reach for rather than inherit:
`internal/tty` has `Raw` and `Cbreak` and neither is echo-off with output
post-processing left on.

`read` is the other non-discriminating command and is worth naming
beside `cat`, because it looks like it should work. `zpty -b D 'read -r
x'` answers `hello` too, without `-e`: the *command* puts the terminal
into a mode of its own, so what comes back says nothing about how the
pty was set up. The tell for a usable probe is a command that touches
the terminal not at all.

`-b` makes the pseudo-terminal non-blocking in both directions, which is
what changes the reading rules below.

## Reading

`zpty -r [ -mt ] name [ param [ pattern ] ]`, and it is three commands
sharing a letter.

**With only a name**, the output is copied to this shell's standard
output. Blocking, copying continues until the command exits;
non-blocking, only what is immediately available is copied. The status
is 0 if anything was copied.

    zpty P "print -n hi"; zpty -r P      →  writes `hi`, status 0

**With a param**, at most one line is read and stored there.

    zpty P cat; zpty -w P hello; zpty -r P line
    line = "hello\r\n"      status 0

**With a pattern as well**, output is read until the whole string read
so far matches the pattern, and the read stops **at the match**:

    zpty P cat
    zpty -w P "abcDONExyz"
    zpty -r P line "*DONE*"
    line = "abcDONE"        status 0

`xyz` is still in the pty, unread. That is the shape `zsh-autosuggestions`
is written against, which reads until `*\0*\0`.

### The statuses

| state | status |
| --- | --- |
| something was read | 0 |
| nothing, **because the command has finished** | 2 |
| nothing, and the command is still running | 1 |

**Re-measured 2026-09-21, and this table replaces a finer split that does
not reproduce.** The earlier reading here was that a read with no pattern
against a spent, exited command answers 1 and a read with a pattern in
the same state answers 2. Six reads, every one of them *without* a
pattern so that nothing could hang:

    zpty -b G 'print -n hi'
    zpty -r G a     0, a = hi
    zpty -r G b     2      nothing left, and no pattern anywhere in the row
    zpty -r G c     2
    zpty -b H true
    zpty -r H d     2      the command wrote nothing at all
    zpty -b I 'sleep 1'
    zpty -r I f     1      still running
    …after it exits…
    zpty -r I g     2

So it is the **command** that decides between the two failures and not
the pattern. The one row that gave 1 where this table says 2 —
`zpty -r A l2` after a *pattern* read had drained the same pty — is
sequence-dependent and is not modeled; it is the same build whose pattern
reads against a spent command hang outright, recorded below.

A non-blocking read with nothing available is 1, which both readings
agree on:

    zpty -b P cat; zpty -r P line     →  status 1, line unset

**`-m`** narrows the success condition to the pattern alone: without it,
a command that has exited with at least one character still readable is
0 even though the pattern did not match. **`-t`** polls first and
returns 1 immediately when nothing is available; measured, it also
leaves the parameter as it found it rather than emptying it.

## Writing

`zpty -w [ -n ] name [ string ... ]`. The strings are joined with spaces
and a newline is added unless `-n` is given:

    zpty P cat; zpty -w P a b c; zpty -r P l      →  l = "a b c\r\n"

With no string at all, this shell's standard input is copied to the
pseudo-terminal, `-n` is not applied, and the copy may stop short when
the pty is non-blocking:

    print -n "piped" | zpty -w P                  →  status 0

The command under the pty sees all of this as if it were typed, so a tty
driver character — word-erase, line-kill, end-of-file — means to it what
it would mean from a keyboard.

## Deleting, testing and listing

`zpty -d name ...` deletes the named commands and `zpty -d` alone
deletes every one. Deleting sends `HUP` to the process. The manual warns
that doing it in a subshell kills the command and leaves the parent
confused about the state — which is a statement about the *parent's*
bookkeeping, and is the reason the table below cannot simply be cloned
into a subshell here.

**Measured, 2026-09-20**, since the shape of that confusion is what an
implementation has to choose about:

    zpty -b P 'sleep 3'
    ( zpty )            lists `(pid) P: 'sleep 3'`, status 0
    ( zpty -d P )       status 0
    zpty -t P           status 1 — the command is gone
    zpty -d P           status 0 — and the **name** is still the parent's

So a subshell **sees** the table and its delete reaches the process,
while the parent keeps the entry: `-d` there succeeds rather than
answering `no such pty command`. That is a live child killed out from
under a table that still names it, and it is the half of this module
that a cloned runner here would have to be told about explicitly — the
subshells in this tree are cloned Runners in one process rather than
forks, so the sharing is not something that happens by default and has
to be arranged.

`zpty -t name` without `-r` is whether the command is still running: 0
while it is, non-zero once it is not, and it says nothing about whether
output is waiting.

The listing writes the pid, the name and the command as it was given;
`-L` writes the call that would reproduce it:

    (76813) P: 'print -n hi'
    zpty P 'print -n hi'

**Newest first, and a command that has ended prints `(finished)` where a
pid would go.** Measured 2026-09-20, three commands started as P, Q and R
and then both spellings asked:

    (finished) R: true
    (65093) Q: cat
    (65091) P: 'print -n hi'

    zpty R true
    zpty -e -b Q cat
    zpty -b P 'print -n hi'

Three things come out of that one run. The order is the reverse of the
order they were started in, under both spellings. `-L` writes the flags,
`-e` before `-b`. And the command is quoted only where it needs quoting —
`cat` bare, `print -n hi` in single quotes.

`(finished)` is a state and not a number, and *which* commands are in it
is that build's reaping rather than a contract: R had ended and said so
while P had ended and still showed a pid. Here it is answered from the
command itself, so one that has ended says `finished` every time.

A name nobody started is `no such pty command: name` at status 1 from
`-r`, `-w`, `-d` and `-t` alike.

### Two hangs in the reference build, which are not behavior to model

Measured on zsh 5.9.2, macOS, with a **live** `cat` under the pty:
`zpty` (the listing), `zpty -L` and `zpty -t NAME` all block
indefinitely. The same three answer immediately once the command has
exited. `zpty -r -m P l "*NOPE*"` against an exited command blocks as
well, where the manual says it reads at most a megabyte and then fails.

Those are faults in that build. A shell that reproduced them would be
choosing to hang; the answers to model are the ones the manual states
and the exited-command rows measure.

## What the module is worth here, measured

#3748 is titled for two installed plugins that "degrade quietly"
without this module. Re-measured on this machine on 2026-09-19,
**neither plugin reaches its `zpty` line with the configuration
installed**, so the degradation the issue records is not happening:

- **`zsh-autosuggestions`.** Its `zmodload zsh/zpty 2>/dev/null ||
  return` is inside `_zsh_autosuggest_strategy_completion`, which is
  called only when `completion` is in `$ZSH_AUTOSUGGEST_STRATEGY`. The
  plugin's own default is `(history)` and nothing in this machine's
  startup files overrides it — read out of a live session as
  `strategy=(history)`. The function is defined and never called.
- **`zi`.** Its `zpty -b` is in the `service` ice path (the `p1`/`s1`
  load types) and no `zi ice` in this machine's startup files asks for
  `service`. Its other line, `zmodload zsh/zpty zsh/system 2>/dev/null`,
  is a bare probe whose status is discarded.

What does use it on this machine is zsh's own shipped `nslookup` and
`run-help`, which drive an interactive program through a pty. That is a
smaller and more honest demand than the issue states, and it is a demand
for the **whole** builtin rather than for a subset.

**So a subset chosen to make those two plugins take their good path was
an empty subset**, and that is why the whole builtin is what landed:
`zsh-autosuggestions`' capture function needs `vared`, `zle -N`,
`comppostfuncs`, `compstate[insert]` and the shipped `_main_complete`
before it can produce the null-delimited output its read waits for.

**Re-checked 2026-09-20 and again on 2026-09-21.** Three of the
prerequisites that sentence rests on have arrived as builtins — `vared`,
`zle -N` and `compstate` are all answered here now — so the claim was
worth re-testing rather than restating. It survives: `comppostfuncs` is
nowhere in this tree, and the shipped completion system does not load,
measured with the host's own `FPATH` rather than an empty one:

    autoload -Uz compinit; compinit -u -d …; whence -w _main_complete
    _main_complete: none        and `compdef: none` beside it

That is an argument about which *plugin* starts working, and it is not
an argument about this module: `nslookup` and `run-help` drive an
interactive program through a pty and need nothing of the completion
system, and the guard both plugins write — `zmodload zsh/zpty
2>/dev/null || return` — now answers 0 because the builtin is really
there rather than because a table entry says so.

## The decision that had to be made first

`zpty NAME cmd` runs a command **on a goroutine with the pseudo-terminal
as its three descriptors, under a handle the shell keeps**. That is a
subshell boundary, and this package reconstructs those by hand because
its subshells are cloned Runners in one process rather than forks — see
`interp/concurrent.go` and `interp/coproc.go`, which was the same shape
with pipes instead of a terminal.

**No dialect could reach it.** The three extension points a dialect has
are the vectors, a registered builtin and a sourced prelude; a builtin is
handed an `*interp.Runner` and there was nothing public on it that
started a command concurrently with descriptors of the caller's choosing.
`startCoproc` was unexported, `Jobs` only lists, and `Runner.Run` is
synchronous. So this was not a builtin anybody could sit down and write:
it needed **a new public seam on the substrate**, and what shape that
seam took was a design decision rather than an implementation detail.

### What was decided

The seam goes in the substrate and is `startCoproc` **generalized**
rather than a second mechanism beside it — both because a dialect may
never own a capability the core lacks, and because a boundary written
twice is a boundary whose next fix lands in one copy. `interp/coproc.go`
is now the pipe-making half of it.

    interp.ConcurrentCommand   what to run: a name, the command's own
                               three descriptors, whether those close
                               with the command, and the body
    interp.Concurrent          the handle: Name, Ident, Running, Stop
    Runner.StartConcurrent     start one and keep it under its name
    Runner.ConcurrentNamed     find one again
    Runner.ForgetConcurrent    take a name out of *this* runner's table
    Runner.OpenNearEnd         put the shell's own end of it in the
                               descriptor table, as plumbing rather than
                               as a file the script opened

Four things about that shape are answers to measurements rather than
taste, and each is written up where it lives:

- **A pty command is not a job.** `zpty -b P 'sleep 2'` leaves `jobs`
  empty and `$!` at 0, so the seam keeps a table of its own and settles
  the job with no process at once — a body made only of builtins never
  reaches the point where a background job's pid would settle, so waiting
  for one would have made `zpty` block until its command had finished.
- **The table is the subshell's and the command in it is shared**, which
  is exactly the four rows under *Deleting, testing and listing*.
- **The command's ends do not always close with the command.** A pipe's
  far end must, or a coprocess that has exited leaves its reader waiting.
  A pseudo-terminal's must not: measured on macOS 2026-09-21, closing the
  terminal side **discards whatever the control side has not read**, so a
  command that printed and exited would read back empty. `zpty` keeps
  both ends until the entry is deleted and stops its reads on the command
  having finished instead.
- **The shell's own end is plumbing.** Registered with
  `Runner.OpenDescriptor` it is duplicated into the next shell started
  beside this one, and the pair then stops answering — three ptys started
  in a row all read back nothing. `OpenNearEnd` is the same registration
  with the two exclusions a coprocess's near ends already had.

## What it needed besides the seam, and where each of those landed

- A **pseudo-terminal pair**, which `internal/pty` already opened, plus a
  line discipline with echo off by default and output post-processing
  left on — `internal/tty` had `Raw` and `Cbreak` and neither is that
  pair of settings. `internal/tty.SetEcho` is the third, and the probe
  that discriminates is in *Starting* above.
- **Per-runner state holding an `*os.File` and a live command.** The
  files are in the runner's descriptor table, which is `zsystem flock`'s
  arrangement; the names, the numbers, the flags and the command text are
  in a shell parameter no script can spell, which is `sched`'s and
  `zstyle`'s; and the live command is the seam's table. All three are
  copied by a subshell, which is what the rows below want.
- An answer for **subshells** matching the rows in *Deleting, testing and
  listing*: the table is visible in one and a delete there reaches the
  command while the parent keeps the name.
- A **sandbox row with the feature**, per `AGENTS.md`. `zpty NAME cmd`
  starts a program on a terminal the shell owns, `-w` writes to its
  standard input and `-r` reads its output, and there is no path anywhere
  in it — the `${(k)mapfile}` shape the ledger keeps missing.
  `module/zpty-exec` in `internal/sandboxcheck` grades `contained` under
  both policy shapes, because the command under the terminal runs in a
  Runner of its own and its exec passes the gate like any other.

## What is deliberately not reproduced

- **The three hangs** recorded above are faults in that build, and a
  shell that reproduced them would be choosing to hang.
- **`(zpty)` as a source in a parse error's location.** The reference
  sites a parse failure of the joined command at a source of its own —
  `(zpty):3: parse error near ...` — and this shell has no second source
  to site it in, so the name is written where a builtin's name goes and
  the line is the caller's.
- **The pid in a listing.** A command here is a goroutine rather than a
  process, so the number beside a live one is the identity the substrate
  invented for it.
- **A read's leftovers on failure.** That build leaves an argument word or
  a previous read's text in the parameter of a read that failed; the
  parameter is left exactly as it was found, which is what `-t` is
  measured to do and what the manual says.
