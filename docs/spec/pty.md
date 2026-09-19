# Running a command under a pseudo-terminal

`zsh/zpty` is the one module of #1320's table this shell still refuses.
It offers one builtin, `zpty`, which runs a command with a
pseudo-terminal of its own so that a program insisting on an interactive
environment can be driven from a script.

This is the **spec half** of that work. Nothing here is implemented yet;
it is written down first because `CLEANROOM.md` says the wall comes
before the code, and because the demand for the module turned out to be
different from what the issue that asked for it recorded — see the last
section, which is the part to read before starting.

Sources: `zshmodules(1)`, section *THE ZSH/ZPTY MODULE*, and an oracle
run against zsh 5.9.2 (`/opt/homebrew/bin/zsh`, aarch64-apple-darwin25)
on 2026-09-19. Every table below is that run unless it says otherwise.

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
into a carriage return and a newline. A probe that tells `-e` from the
default has to run a command that does **not** echo its input.

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
| nothing could be read | non-zero |
| nothing, **because the command has finished** | 2 |
| non-blocking, nothing available yet | 1 |

Measured, and the 1-versus-2 split is finer than the manual's sentence:

    zpty P "print -n hi"
    zpty -r P l "*hi*"      first=0
    zpty -r P l2            second=1      no pattern
    zpty -r P l3 "*x*"      third=2       with a pattern

So a read with no pattern against an exited command whose output is
spent answers **1**, and a read with a pattern in the same state answers
**2**. A non-blocking read with nothing available is 1:

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

`zpty -t name` without `-r` is whether the command is still running: 0
while it is, non-zero once it is not, and it says nothing about whether
output is waiting.

The listing writes the pid, the name and the command as it was given;
`-L` writes the call that would reproduce it:

    (76813) P: 'print -n hi'
    zpty P 'print -n hi'

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

**So a subset chosen to make those two plugins take their good path is
an empty subset**, and building one anyway would be worse than the
refusal: `zsh-autosuggestions`' capture function needs `vared`,
`zle -N`, `comppostfuncs`, `compstate[insert]` and the shipped
`_main_complete` before it can produce the null-delimited output the
read waits for. The refusal a script can act on — pinned by
`dialect/zsh/zmodload_test.go` — is the right answer until that is true.

**The order of work, therefore:** the completion system first, this
module after it, and the whole builtin rather than a part of it.

## What an implementation will need that this tree does not have

Recorded so the size is visible rather than discovered:

- A **pseudo-terminal pair**, which `internal/pty` already opens, plus
  a line discipline with echo off by default and output post-processing
  left on — `internal/tty` has `Raw` and `Cbreak` and neither is that
  pair of settings.
- A way to run a command **on a goroutine with the pty as its three
  descriptors**, which is a subshell boundary this package reconstructs
  by hand (see `interp/concurrent.go`) and which no dialect can reach
  through the public seams today.
- **Per-runner state holding an `*os.File` and a live child**, which no
  dialect keeps: `zmodload`, `sched` and `zstyle` all keep theirs in a
  shell parameter under a name no script can reach, and a descriptor
  cannot go in one. `$REPLY` being the master's descriptor number is a
  hint that the master wants registering with `Runner.SetDescriptor`,
  which would leave only the name-to-descriptor map in a parameter.
- A decision about **subshells**, which the manual raises and does not
  settle for us: a cloned runner that inherited the table would delete
  its parent's commands.
