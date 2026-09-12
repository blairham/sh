# Blocks: a command and its output as one unit

A **block** is one thing a person asked for and everything that came of
it: the line as typed, where and when it ran, how long it took, what it
exited with, and what it printed. A shell that keeps only the first of
those keeps a list of strings; a shell that keeps all of them can answer
"what did that do", "why did it fail", and "show that to the assistant"
without the person retyping anything.

The shape is a validated design from our own prior work, where a block
was the unit of the interactive surface. What is new here is that this
repository already has an event stream and a gate, so a block does not
get to be a third opinion about what a command is. It is the **at-rest**
form of the same unit the `Sink` streams live — see
[the event schema](sandboxing.md#the-event-schema), which owns the wire
form, and [ACP](acp.md), which consumes it in both directions.

Read `docs/design.md` first. This fills in a consumer of the seams it
describes; it does not add a seam.

## The premise this had to fix first

The north star for this work assumed history was already metadata-rich.
It was not. `repl/history.go` on the day this was written is a plain
line file — `~/.sh_history`, read at start, appended at exit, `HISTFILE`
and `HISTFILESIZE`, and nothing else. There is no cwd, no status, no
duration, no timing.

So this is two halves, and the first is the one nobody had built:

1. **The command half** — a metadata-rich record per accepted line. The
   runner already knows every field, so it is plumbing rather than new
   measurement.
2. **The output half** — what the command printed, as an addressable
   artifact, and the reference tying it to its record.

They ship separately and the first is useful alone. That is deliberate:
the output half costs something real (below), and a store that degrades
to the command half when output is off — or when a body file has been
deleted — is a store nobody has to think about backing up.

## The store

### `HISTFILE` is not touched, and that is the compatibility story

The plain-line history file keeps its format, its path, its size rule
and its append-at-exit behavior exactly as they are. The block index is
a **second file, beside it**, and never a replacement.

The alternative — make `HISTFILE` itself JSONL, with a sniff for the
old format — was considered and rejected on three counts:

- **A history file is a public interface.** People `grep ~/.sh_history`,
  pipe it to `sort | uniq -c`, and read it in an editor. Every one of
  those breaks the day the file becomes JSON, and breaks silently: the
  grep still matches, it just matches the quoting too.
- **A shared file cannot be migrated.** Two shells are open at once, and
  one of them may be a different shell entirely. A format change that a
  concurrent writer has not heard about corrupts the file rather than
  upgrading it.
- **The two files answer different questions.** The line file is the
  *recall list* — what the up arrow walks. The index is the *record* —
  what happened. Keeping them separate means each is independently
  disposable: delete the index and recall is unaffected; delete the line
  file and the record is unaffected. Neither deletion is a corruption.

Where they disagree, the line file wins for recall and the index wins
for the record. Nothing reconciles them, because nothing has to.

### Where it lives

    $SH_BLOCKS_DIR/                 named by a shell variable
      index.jsonl                   append-only, one record per line
      body/2026/09/05/<id>.out      one file per block that kept output

`SH_BLOCKS_DIR` names the store, and **naming it is how a person turns
it on**. Unset is off; empty is off, exactly as an empty `HISTFILE`
turns the history off — the same idiom, so there is one thing to learn.
There is no default location, so the one variable is both the switch and
the address: nothing else to set, and no sentinel to learn.

It used to fall back to `$XDG_STATE_HOME/sh/blocks` and then to
`$HOME/.local/state/sh/blocks`, which meant the store was on for
everybody who had a home and had history on. That was open question 1
below, and #2274 answered it: **opt-in**. The argument is recorded there
rather than repeated here.

A state directory rather than `~/.sh_blocks`, and the reason is the
output half: the index is small and the bodies are not, so this is a
growing directory of arbitrary size and that is precisely what a state
directory is for. The name is `SH_`-prefixed rather than `HIST`-shaped
because it is ours — no shell in the panel has a variable by this name,
so it cannot collide with something a person's existing rc file sets and
means differently.

It is a **shell variable**, read through `Runner.GetVar` like `HISTFILE`
and for the same reason: a line typed at the prompt can set it and mean
it, and a session that has moved its store has moved it.

### One off switch, not two

**An empty `HISTFILE` turns the block store off as well.** This is a
coupling and it is on purpose. `HISTFILE=` is what a person types when
they mean *do not remember this session* — a directory they do not want
in a file, a credential they are about to paste. A shell that honored
that for the line file and went on writing a richer record, with the
output attached, would be doing the exact opposite of what was asked in
the one moment it matters most.

The reverse does not hold: `SH_BLOCKS_DIR=` turns off the blocks and
leaves ordinary history working, because that is only saying "I do not
want the richer thing".

### Append-only, and never rewritten

`index.jsonl` is opened `O_APPEND` once per session and one record is
written per block, as a single `Write`. It is never truncated, never
rewritten, and never compacted by the shell.

That is the discipline the line file already gets right and it is worth
naming, because the obvious alternative is what every shell's history
has been bitten by: rewriting the file at exit makes the *last shell to
exit* the only session that happened, and two terminals open at once
lose one of them. Appending has no such window.

`HISTFILESIZE` bounds what a *reader* keeps, not what the file holds —
again matching the line file, where the trim happens at load. Retention
of the store is therefore a person's own business, and the layout is
built so it can be done with `rm`: the bodies are sharded by date, so
`rm -rf body/2026/08` drops a month and leaves the index intact. A
record whose body is gone reads as a block with no output kept, which is
the same state a block recorded with capture off is in. There is one
state to handle, not two.

A record is written when the block **closes** — after the accepted line
has finished running — rather than at exit. A shell that is killed keeps
everything up to the last completed command, which is the case where the
record is most wanted.

Interleaving between sessions is what `O_APPEND` gives: each write lands
at the current end. A record longer than the filesystem's atomic write
size could in principle be split by a concurrent writer, and a reader
skips a line that will not parse — which is the property JSON Lines was
chosen for and which the event schema already relies on.

### The record

One JSON object per line, the same shape discipline as the event stream:

    {"v":1,"id":"0R4K8QG0000005VJ8PB2FQ1T7C","session":"0R4K8Q...",
     "command":"make check","cwd":"/home/someone/src",
     "start":"2026-09-05T11:02:03.000000001Z","durationMs":8421,
     "status":2,"output":"body/2026/09/05/0R4K8QG0…out",
     "outputBytes":190244,"truncated":false,"streams":"merged"}

(wrapped here for width; a record is one line)

| field | type | present |
| --- | --- | --- |
| `v` | integer | always — the schema version, `1` |
| `id` | string | always — see below |
| `session` | string | always — the id of the session that wrote it |
| `command` | string | always — the line **as typed**, before expansion |
| `cwd` | string | always — `Runner.Dir` when the line was accepted |
| `start` | RFC 3339 with nanoseconds | always |
| `durationMs` | integer | always — wall clock, including zero |
| `status` | integer | always — `$?` after the line, including zero |
| `output` | string | when a body was kept — path relative to the store |
| `outputBytes` | integer | when a body was kept — the **true** total |
| `truncated` | bool | when the body is shorter than `outputBytes` |
| `streams` | string | when a body was kept — `merged` today |

The stability rules are the event schema's, verbatim and by reference:
`v` identifies the schema and a consumer refuses one it does not know; a
field name means one thing forever; new fields may be added within a
version and a consumer ignores what it does not recognize; an absent
field means the zero value. They are the same rules because they are the
same unit, and a consumer that has learned one has learned both.

The reader implements all four rather than only writing to them. A
record whose `v` is newer is skipped, so an old shell opening a store a
newer one has appended to shows everything it can rather than failing;
an unrecognized *field* is ignored, which is what makes an added field a
change nobody has to coordinate; and a record with no `status` at all is
skipped, because reading one would report a success that never happened.

That last is where this differs from the event schema in mechanism while
agreeing with it in intent. There, `status` is a pointer because a status
is present only for the end of a command, so absence is ordinary and
must be told from zero. Here every block has one — a block always exited
with something — so the field is a plain `int` that is always written,
and the distinction is needed only on the way back in, where a missing
one means the line is truncated or foreign.

Two field-level differences from the event record are deliberate:

- **There is no `seq`.** A sequence number is a total order of
  *emission* and explicitly not a causal one, which is right for a log
  and is the wrong thing for a block to lean on. A block's order is the
  order of the file, and its identity is an id that sorts by start time.
- **The timestamp is `start`, not `time`.** An event's `time` is when
  the record was written. A block's is when the command began, with a
  duration beside it. Reusing the name for the other meaning is the one
  thing the stability rules forbid outright.

The index is opened the way `cmd/sh -audit` opens its file — `O_APPEND`,
`0600`, unbuffered, one write per record straight through — because the
two have the same problem: a trail that erases the previous run, or that
loses what a buffer held when the shell died, is not a trail.

`output` is a **stored path** rather than a derived one. The reader
never recomputes the layout from the id, so the sharding above is an
implementation detail that can change without a version bump, and a
store whose bodies were moved by hand still resolves.

### The id

A block id is 26 characters of base32hex: eight bytes of Unix
nanoseconds, big-endian, followed by eight random bytes.

    0R4K8QG0000005VJ8PB2FQ1T7C

Four properties, each of which was a requirement:

- **Sortable by time.** base32hex's alphabet is `0`–`9A`–`V`, which is
  ordered, so sorting the encoded strings sorts the underlying bytes and
  therefore the timestamps. `sort` over the index is chronological.
- **Unique without coordination.** Two sessions writing at the same
  nanosecond differ in 64 random bits. Nothing has to hold a counter,
  which is what makes an append-only multi-session store possible at all.
- **Safe as a filename, on every filesystem.** No case folding to worry
  about — the alphabet is uppercase throughout — and nothing that needs
  quoting in a shell.
- **No dependency.** `math/rand/v2` and `encoding/base32` are the
  standard library. Not `crypto/rand`, and that is a shell-specific
  choice rather than a shortcut: what is wanted is uniqueness rather
  than unpredictability, and measured on macOS the first
  `crypto/rand.Read` in a process permanently opens a descriptor —
  which in a shell is not an implementation detail, because descriptor
  3 is the first number a script parks with `exec 3>f`.

A content hash was the other candidate and is wrong here: two identical
commands run an hour apart are two blocks, not one, and the whole point
of a block is *when* and *where* as much as *what*.

A session gets an id by the same construction, so `session` sorts and
compares like a block id and a store can be split by session with
`grep`. The construction lives in `internal/event` — `event.NewID` — so
that the id a block record carries and the id an audit record carries
are made the same way and by the same code.

Retrieval addresses a block by its full id or by a **recency number** —
`1` is the most recent block. Prefix matching, which a content-hash id
would have made natural, is useless here: the first half of the id is a
timestamp, so an eight-character prefix is shared by every block in an
eighteen-minute window. A decimal number and a 26-character base32hex
string cannot be confused for one another, so the two forms coexist
without a flag to disambiguate them.

## The output half

### What capture costs, and what a pseudo-terminal buys

`interp` hands a child `r.Stdout` directly. When that is an `*os.File`
the child inherits the descriptor and **is on the terminal**; when it is
anything else, `os/exec` builds a pipe and copies through it. Capturing
output through a wrapper means the writer is no longer a terminal file,
and everything downstream of `isatty` changes:

- `ls`, `grep` and `git` stop colorizing.
- `git` and `man` stop paging.
- A full-screen program — an editor, `top` — is broken outright.

That is not a rough edge to file and move on from; it is the shell
becoming worse at being a shell, which is why output capture shipped
opt-in and why #720 said it could not be on by default until a
pseudo-terminal backed it.

It is opt-in again, and for a different reason than the one #720 was
about. #720 asked what capture *costs a child*; #2274 asked what it
*writes down*. A body holds what the person was shown rather than what
they typed, which is a different class of thing from a history file and
the only part of this store whose harm a backup makes permanent. So a
pseudo-terminal answers #720 and does not answer #2274, and the output
half is asked for by name: `SH_BLOCKS_OUTPUT=terminal` keeps output
where it is free, any other value keeps it whatever it costs, and unset
or empty keeps none.

**One does now.** `repl.ptyConduit` puts a pseudo-terminal between the
shell's children and the terminal a person is looking at: the child's
fd 1 and fd 2 are the inner terminal, so `isatty` is true and nothing
downstream of it changes; the shell reads the other end, writes it where
it was going, and keeps a copy on the way past.

#### The job-control rewrite this was waiting for does not arise

This document predicted the blocker as job control rather than the pty:
"the shell owns the real terminal and must forward window size, terminal
ownership and the stop signals to the inner one … a rewrite of the
job-control path rather than an addition to it".

Measured on 2026-09-06, two of those three do not arise, because
**standard input never moves.** A child's fd 0 is the same terminal the
repl already hands to a foreground process group through `/dev/tty`, so
terminal ownership, `^C` and `^Z` are exactly where they were and the
conduit cannot move them. Only the output half is the inner terminal.
The probe, a child with its stdin on one pseudo-terminal and its stdout
on another:

    test -t 0 && echo IN_TTY   →  IN_TTY
    test -t 1 && echo OUT_TTY  →  OUT_TTY

Both — which is what a child on a terminal sees, and what a captured
child did not see before.

What is left is the third item, window size, and it is one ioctl: the
inner terminal is given the outer one's size when it is opened and again
on every `SIGWINCH`. Without it a full-screen program asks fd 1 how wide
the terminal is and is told 0 by 0.

Two more things the inner terminal has to be told, both measured:

- **Its output discipline is off** (`OPOST`), so it is a conduit and not
  a second terminal doing the work twice. A pty slave translates `\n` to
  `\r\n`; the real terminal then does it again to the `\n` that is left,
  giving `\r\r\n`. The first probe above came back as
  `"IN_TTY\r\nOUT_TTY\r\n"` through a slave in its default discipline.
- **It has to be drained before the next prompt.** A pseudo-terminal is
  a queue, so a command's last bytes may still be in it when the command
  has exited; taking the capture then would put the tail of one block at
  the head of the next, and draw the prompt on top of output still in
  flight. The drain writes a private random token into the conduit after
  the command has exited and waits for the pump to reach it — an in-band
  mark, not a wait for quiet, which is the same discipline the pty tests
  follow and for the same reason: a wait for quiet passes early on a slow
  writer and hangs on a busy one, and neither failure is visible.
- **The drain has to happen before the terminal goes back to raw mode**,
  and it did not — which is #1356, a prompt drawn where the last output
  ended. The two facts above compose into a bug: the conduit's newlines
  are bare, because its own output discipline is off, and the real
  terminal is what translates them. So the copy has to land while the
  real terminal is still in its own line discipline. The drain was on the
  other side of that switch — the command returned, raw mode came back in
  `inLineDiscipline`'s `defer`, and *then* the block was taken and the
  conduit drained — so a copy that had not caught up sent its tail out
  with `OPOST` already off.

  Captured once in nineteen runs, which is what a goroutine losing a race
  by microseconds looks like:

      BAD   …echo SECOND\r\x1b[23C\r\nSECOND\np-bash-5.3$      a bare \n
      GOOD  …echo SECOND\r\x1b[23C\r\nSECOND\r\np-bash-5.3$

  The wait now happens in that `defer`, ahead of the raw mode, so the
  helper's promise is complete: the terminal does not stop translating
  until everything the shell handed it has arrived. It waits without
  emptying the capture, because the block that records the output is read
  after it — draining and discarding there would have lost every
  command's body from the store.
- **And the pump adds the return itself where the terminal is not adding
  it**, which is the half with no boundary to hold. A **background job**
  prints while a person is typing, so there is no cooked window anywhere
  to move the write into and no moment to wait for. The pump's writer
  reads the terminal's mode and translates only when the terminal is not
  — one source of truth rather than a second switch that has to agree
  with the first, which is the shape the bug had. One ioctl per read of
  the inner terminal, so per 32KB and not per byte.

  Measured against bash 5.3.15, a job started with `&` printing six lines
  at a prompt: six bare line feeds before and none after, against none in
  bash either way. **Both halves are load-bearing**, over 25 rounds of a
  real session: the reading writer alone still left 6 rounds with a bare
  line feed, because the mode is read and then written to and at a
  boundary those two can straddle the switch; with the drain as well,
  none.

#### So the default moved, and the setting grew a third answer

`SH_BLOCKS_OUTPUT` now says one of three things:

| value | what it means |
| --- | --- |
| unset | **the default** — keep output where a pseudo-terminal can carry it, and keep none where one cannot |
| empty | keep none, whatever this session has. The gesture an empty `HISTFILE` already is |
| anything else | keep output whatever it costs, wrapper and all — for a session recording a build in a pipeline, which has no terminal to preserve |

The default keeps *nothing* rather than falling back to the wrapper,
because the wrapper is the cost the whole issue was about. A session with
no terminal, with its two streams going to two different places, or with
no store to write a body to, keeps nothing unless it asked by name.

Bodies are redacted through the same secret table the command line goes
through and written 0600 inside a 0700 directory, which is what makes
on-by-default a question about the terminal rather than about privacy.

The command half runs regardless, because it costs nothing and breaks
nothing. And the format did not move: the flag flipped, the schema did
not, which is what #495 built it for.

**The switch is read once, at session start.** Setting
`SH_BLOCKS_OUTPUT` at the prompt takes effect in the next session, not
the next command. Two reasons, and the second is the real one: what a
child sees is decided by the writer the Runner holds when it is started,
and a background job started under capture goes on writing to that
writer after the block it started in has closed. Swapping the writer
between commands would make `sleep 10 & ` lose its output to a sink
nobody is reading. Installing once means a background job's output lands
in whichever block is open when it arrives — which is exactly what a
terminal does with it too.

### One body, both streams, in write order

Standard output and standard error go to **one file, interleaved in the
order they were written**, and the record says `"streams":"merged"`.

That is what the person saw, which is the thing a block is for. Two
files would lose the interleaving, and a compiler that prints progress
on one stream and the error on the other is the case where the
interleaving *is* the information. A framed single file would keep both
and would stop the body being something you can `cat`.

The faithfulness claim is bounded and worth stating: the shell's own
writes are ordered by the mutex the sink holds, and a child's two
streams race through `os/exec`'s copy goroutines — but they race on a
terminal too, so the file is wrong in exactly the places the screen was.

`streams` exists as a field from the first version so that a later
`"split"`, with two paths, is an added field rather than a version bump.
That is the additive rule being used rather than described.

### The cap: head and tail, never the middle

`SH_BLOCKS_MAX_OUTPUT` bounds a body, defaulting to 1 MiB. Over the cap,
the **first half and the last half** are kept and the middle is dropped,
with a marker line in between saying how much went.

Head-only loses the failure, which is at the end. Tail-only loses what
was invoked and what it decided, which is at the start. A build log is
the worked example of both.

`outputBytes` records the **true** total rather than what was kept, and
`truncated` says the two differ, so a consumer is never guessing.

The cap also bounds memory, which is the part that is not about
aesthetics. Capture keeps a head buffer of at most half the cap and a
tail ring of at most half, so `cat /dev/urandom` at the prompt costs a
fixed megabyte however long it runs. A capture that buffered the whole
of a command's output would be a shell that can be made to exhaust
memory by a command that is behaving normally.

### Scrubbing is not optional here

`internal/secret` already exists, and its package documentation
anticipates exactly this consumer: a command line that matches is
**rejected whole**, output that matches is **redacted**. Both apply.

- A block whose **command line** matches is not recorded at all — no
  index record and no body. `export TOKEN=…` essentially *is* the
  credential, so a record of it with the value removed recalls nothing
  anyone wanted, and the block store must not become the copy of the
  history that the history refused to keep. This is the same rule
  `withoutCredentials` applies to the line file, applied at the same
  point on the same table.
- A **body** that matches is redacted and kept. Dropping a
  two-hundred-kilobyte build log because one line echoed a token
  destroys what the person wanted.

Redaction runs at close, over the head and the tail as text, rather than
per write. A credential can straddle two `Write` calls, and a scanner
run on each chunk would see neither half.

This multiplies what reaches disk — that is the whole feature — which is
why the scrubbing that the line file already has is a precondition of
the store rather than a follow-up to it.

## Where the boundary sits

Every file this opens goes through `internal/boundary`, with the
session's own `Gate` and `Sink`: the index, and each body.

The test `docs/design.md` sets is **who chose the path**, and
`SH_BLOCKS_DIR` is a shell variable, so a line of script chooses it in
exactly the way `HISTFILE` does. The store is therefore inside the
boundary from its first commit, which is the answer to the note that
block storage multiplies what the repl writes to disk: it multiplies
what the *gate sees*, not what escapes it. A denying policy hides the
store, and a session under one records nothing rather than failing to
start — the same reading a refused history file already gets.

A consequence worth naming with the others in
[sandboxing.md](sandboxing.md#consequences-a-user-meets-immediately): a
default-deny policy must allow the store, or there are no blocks. That
is correct, and it is the same sentence as the one about `HISTFILE`.

## What this is, in terms of the event schema

**A block is the closure of a command-start/command-end pair, plus what
the writer the caller supplied saw.** It is not a second event type and
it does not fork the schema. The correspondence, field by field:

| block field | where it comes from |
| --- | --- |
| `status` | the `status` on `EventCommandEnd` |
| `start`, `durationMs` | the `time` on the two records, if they can be paired |
| `cwd` | `Runner.Dir`, which no event carries because every event would carry it |
| `command` | the typed line — the repl's, not the interpreter's |
| `output` | the writer the repl handed the Runner, per `docs/design.md` |

The last two are the interesting ones and both are correct divisions
rather than gaps to close in `interp`:

**Output is not on an event, on purpose.** `docs/design.md` says so and
gives the reason — the streams are the caller's own `io.Writer`s, so a
consumer that wants output taps the writer it supplied rather than
receiving a second copy of what it already holds. This package *is* that
consumer, tapping that writer. Nothing had to be added to the event for
it, which is the strongest evidence the split was drawn in the right
place.

**The typed line is not on an event either.** `EventCommandStart`
carries `Action.Args` for an exec, which is argv *after* expansion:
`echo $HOME` is recorded as `["echo","/home/someone"]`. A block wants
what was typed. Only the repl has that, because only the repl has a
notion of a typed line at all — a script has statements and no lines a
person chose. So the repl supplies it, and `interp` is right not to.

### What the event schema could not carry, and how that was closed

Two things a block needs were genuinely absent when this landed, and
both were the same absence the ACP design named. Both are now fields of
the event schema, added within version 1 — see
`docs/design/sandboxing.md`.

1. **There was no id on an action or an event.** `EventCommandStart` and
   `EventCommandEnd` were paired by ordering, and ordering is what
   concurrency breaks — a background job and each half of a pipeline
   emit from their own goroutines. `seq` is documented as a total order
   of *emission* and explicitly not a causal one, which is right for a
   log and left this without an answer. Blocks never needed it for the
   *outer* pairing, because the repl owns the boundaries of a typed line
   and does its own timing; it needed it to say **which events belong to
   which block**.

   `interp.Action.ID` is that field, and the same string is on the
   `Action` a `Gate` is consulted about and on every `Event` that action
   produces. A block record may now gain an `events` field naming the
   ids it covers, which would be an added field and not a version bump.

2. **There was no session identity.** Nothing on an event said which
   shell wrote it, so blocks generated its own and put it on the block
   record as `session` — and an audit stream from the same run had no
   such field at all, which is exactly the pair of records that most
   wanted joining.

   The identity is now the front end's: `driver` makes one per
   invocation and hands the same string to the Runner and to the prompt,
   so the `session` on a block record and the `session` on an event
   record of the same run are one value. `blocks` no longer makes one,
   and `event.NewID` is where the construction lives — one generator, so
   two consumers cannot produce ids that look joinable and are not.

Neither was worked around by inventing a parallel event type. Blocks
records what only the repl knows and reads the rest from the same
source everyone else does.

## Retrieval

The store is inspected through `cmd/sh`, following the route
`-trace-events` and `-deny` already established: the seam is exercised
by the binary that exists to exercise seams.

    sh -blocks-list          # the recent blocks, one per line
    sh -blocks-list=100      # more of them
    sh -blocks-show 1        # the most recent block, record and body
    sh -blocks-show <id>     # a block by id

`-blocks-list` prints time, status, duration, id, cwd and the command,
in that order. The time is first because this is a log and that is how a
log is scanned; the status is second because a block that failed is the
one somebody came looking for; the command is last because it is the
only field with no bound on its length.

The count on `-blocks-list` is attached with `=` rather than taken as
the following word, and that is not a style choice: the scan stops at
the first word that is not one of this binary's flags, so a bare count
would eat a script operand. `-blocks-show` has no such problem, since
its argument is required.

Both read a store instead of running a shell, so they end the
invocation — through whatever gate the same command line asked for,
because a policy that hides the store has to hide it from the tool that
reads it too. A refused store reads as an empty one.

This is deliberately not a builtin. A builtin would have to live in
`interp`, which has no history, no store and no business acquiring
either — the substrate does not know that a prompt exists. It is also
not a dialect's builtin, because no shell in the panel has one and
inventing a `blocks` command inside the `bash` dialect would make that
dialect not-bash. When there is an interactive shell built on this
substrate, the command belongs to that shell.

## What is deliberately not here

- **Editing, re-running or piping a block.** Recall and re-run belong to
  an interactive surface, and the store is what such a surface would be
  built on. Shipping the store first is what makes the surface possible
  to write without also inventing a format.
- **A pseudo-terminal.** Argued above: it is the right answer for the
  output half, and it is a job-control change.
- **Compression or deduplication of bodies.** A body is a file, so
  whatever a person already uses on a directory of files works. Building
  it in would mean the bodies stop being `cat`-able, which is most of
  their value.
- **Trimming the store.** Nothing in the shell deletes a record. The
  date-sharded layout exists so `rm -rf` and `find -mtime` are the
  answer, and a shell that quietly deletes the record of what you did is
  a worse failure than a directory that grew.
- **Recording blocks for scripts.** A block is a typed line. A script
  has statements, and the thing that wants a record of a script's
  execution is the event stream, which already has one.

## Open, and flagged for the maintainer

These are defensible defaults chosen without an answer, and each is
cheap to reverse.

1. ~~**The store is on by default**~~ — **answered: opt-in** (#2274).
   Both halves. Unset `SH_BLOCKS_DIR` is no store and unset
   `SH_BLOCKS_OUTPUT` keeps nothing; naming the directory is how a person
   turns it on.

   The case made here was written about the *command half* — one small
   line per command, and free — and neither premise survived the output
   half. Capture puts a pseudo-terminal in front of every child, so it is
   not free; and it records what the person was **shown** rather than
   what they typed, which is a different risk class from a history file
   and the only harm here that a backup makes permanent.

   Three things decided it. Nothing consumes the store yet — recall and
   re-run are "deliberately not here" above — so on-by-default was
   collecting what nobody was reading. Nothing trims it, and the index is
   the one file the `rm`-based retention story below does not work on.
   And a default is cheap to loosen later and a regression to tighten,
   which is the argument for doing this before v0.0.0 rather than after.

   The counter-argument stands and was not enough: opt-in does risk a
   feature nobody meets. The answer is to turn it on **with** the
   interactive surface that reads it, and with the trimming in #2275,
   rather than to accumulate years of unread records first.
2. **`$XDG_STATE_HOME/sh/blocks` rather than `~/.sh_blocks`.** Argued
   above from the size of the bodies. The counter-argument is
   consistency with `~/.sh_history`, which is real.
3. **An empty `HISTFILE` turns blocks off too.** Argued above as
   honoring what a person meant. It is a coupling between two variables,
   which is the kind of thing that surprises someone eventually.
4. **1 MiB per body, half head and half tail.** A number with no
   measurement behind it. It is a variable so it can be argued with.
5. **`streams: merged`.** Interleaving beats separation for a person and
   loses for a machine. The field is there so the other answer can be
   added rather than swapped.
