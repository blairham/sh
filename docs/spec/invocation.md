# Invocation

What the front end decides before anything runs: which route the shell
was invoked by, which operand becomes `$0`, and whether there is a person
at the other end. `driver/` is the whole of it — nothing outside that
directory implements how a shell is invoked.

This file covers the last of those, and how much of a program arriving on
standard input the shell takes at a time. The rest is in `AGENTS.md` under
"One driver, four dialects" and is not yet written down here.

## Deciding to prompt

**The rule.** A shell prompts when `-i` says so, or when it was given
nothing to run and its standard input **is a terminal**. Anything else is
a script: a redirect from a file or a device, a pipe, a closed
descriptor, a script named as an operand, or `-c`.

**"Is a terminal" is an ioctl, not a mode bit.** Asking for the terminal
attributes — `TIOCGETA` on the BSDs, `TCGETS` on Linux, what
`isatty(3)` is — is the only test that answers the question. Whether the
file is a *character device* is a strictly weaker question, and the null
device passes it.

That is not a hypothetical. `sh < /dev/null` is the canonical cron,
systemd, CI and harness invocation, and the whole point of redirecting
there is that the shell must wait for nobody. Under the mode-bit test it
took the prompt route, the line editor asked the null device for raw
mode, the ioctl answered `ENOTTY`, and the shell exited 2 with
`operation not supported by device`. `/dev/zero`, `/dev/random` and every
other character device were the same. (#509.)

### Measured

Panel: bash 5.3.15, bash 3.2.57, dash, ksh93u+ 2012-08-01, zsh 5.9, on
macOS 25.5 — measured 2026-09-04. "Prompt" means `PS1` reached standard
error; the terminal rows were driven through a pseudo-terminal pair,
which is also how the tests reach the positive case.

No operands, no `-i`:

| standard input | prompt? | what happens instead |
| --- | --- | --- |
| a terminal | **yes**, all four | the session; `i` is in `$-` |
| `/dev/null` | no, all four | end of input at once, status 0 |
| `/dev/zero` | no, all four | read as a script: bash, dash and zsh read the NUL bytes forever; ksh93 refuses with a syntax error naming the zero byte |
| a regular file | no, all four | read and run as a script |
| a pipe | no, all four | read and run as a script |
| an empty pipe | no, all four | nothing to run, status 0 |
| a closed descriptor | no, all four | dash, ksh93 and zsh exit 0 silently; bash 5.3 fails with `error creating buffered stream: Bad file descriptor` at status 126 |

`/dev/tty` could not be measured: the session doing the measuring has no
controlling terminal to open, which is itself the reason the positive
case needs a pseudo-terminal rather than an inherited one.

With something to run, the terminal stops mattering — unanimously:

| invocation | at a terminal | not at a terminal |
| --- | --- | --- |
| `sh script.sh` | runs the script, no prompt | runs the script, no prompt |
| `sh -c 'cmd'` | runs the string, no prompt | runs the string, no prompt |

`-s` is the explicit "read standard input" spelling and does not change
the answer: `sh -s` at a terminal prompts and `echo x | sh -s` does not,
exactly as with no operands at all.

### `-i`

`-i` overrides the terminal test in both directions of the panel's
agreement and one direction of its disagreement.

- **With no operands**, all four accept it away from a terminal: they
  print a prompt, read the lines, and say only that job control is off.
  The *editor* is what needs a terminal, and it is the editor that goes
  away — the prompt does not. (ksh93 is inconsistent about drawing the
  prompt itself away from a terminal, printing it on a pipe and not on a
  regular file; it sets the flag either way.)
- **With operands**, the panel splits. All four run the script. dash then
  prompts; bash, ksh93 and zsh exit when the script ends. The core
  follows the three: an operand is work, and work means no prompt.
- **`-i` always puts `i` in `$-`** — in all four, on every route,
  including `-c` and a script operand, and whether or not a prompt is
  ever drawn. Without `-i`, `i` appears only where the shell decided to
  prompt on its own. See "Interactive, and `$-`" below.

## Interactive, and `$-`

**The rule.** A shell is interactive when `-i` was given, or when it
decided to prompt on its own. That is a wider question than whether it
prompts: `-i script.sh` runs the script *and* is interactive while it
does, which is why the two are separate facts and not one. Being
interactive has two observable consequences the panel agrees on — `i`
appears in `$-`, and aliases are expanded whatever the dialect would do
in a script.

The front end decides it and hands it to the runner as
`interp.Runner.Interactive`, beside `JobControl`. `interp` never asks the
process: a Runner embedded in another program would be told about the
*embedder's* invocation, which is not this shell's.

### Measured

Same panel and date as above. `$-` read as membership rather than as a
string, because the spelling is the part that splits — see below.

| route | bash 5.3 | dash | ksh93 | zsh |
| --- | --- | --- | --- | --- |
| `sh script.sh` | `hB` | *(empty)* | `hB` | `569X` |
| `sh -i script.sh` | `hiBH` | `i` | `imBE` | `569XZi` |
| `sh -c cmd` | `hBc` | *(empty)* | `chsB` | `569X` |
| `sh -i -c cmd` | `hiBHc` | `i` | `icmsBE` | `569XZi` |
| `sh -s` < pipe | `hBs` | `s` | `hsB` | `569Xs` |
| `sh -i -s` < pipe | `hiBHs` | `si` | `imsBE` | `569Xis` |
| `sh` < pipe | `hBs` | `s` | `hsB` | `569Xs` |
| `sh -i` < pipe | `hiBHs` | `si` | `imsBE` | `569XZis` |
| `sh` < file | `hBs` | `s` | `hsB` | `569Xs` |
| `sh` at a terminal | `hiBHs` | `si` | `imsBE` | `569XZis` |
| `sh -i` at a terminal | `hiBHs` | `si` | `imsBE` | `569XZis` |
| `sh -s` at a terminal | `hiBHs` | `si` | `imsBE` | `569XZis` |

The terminal rows were driven through a pseudo-terminal pair. Every row
was measured with a scratch `HOME` and no startup files, because an
interactive shell reads them and a developer's `.zshrc` is not evidence
about zsh. zsh needs an empty `~/.zshrc` to exist at all, or
`zsh-newuser-install` runs instead of the shell and eats the first
keystroke.

**`i` is unanimous in both directions** and so needs no axis: present in
every `-i` row and in every terminal row, absent in every other row, in
all four. It is implemented.

Alias expansion goes with it, also unanimous: `alias hi='echo aliased';
hi` in a script run as `sh -i script.sh` prints `aliased` in all four.
bash is the one that does *not* expand it without `-i`, which is what
makes the row evidence rather than a coincidence of three shells that
always expand.

### The route letters, `c` and `s`

**The rule.** `c` is in `$-` when the program came from a command string
and the dialect says so. `s` is there when the program came from standard
input, or `-s` was written, or the program came from a command string and
the dialect counts that too.

The route itself is the front end's — `interp.Runner.Route`, carried in
beside `Interactive` and for the same reason. Two of the three ways `s`
gets there are unanimous and need no axis; the third and the whole of `c`
are `Semantics.CommandStringShowsSInDollarDash` and
`Semantics.CommandStringShowsCInDollarDash`.

Both are *read* rather than `ask`ed. A dialect that answers nothing shows
no letter, which is exactly what an unanswered `DefaultOptionLetters`
does; refusing the expansion would break `case $- in *e*)`, the ordinary
errexit check, in every script running under a preset that has not
chosen.

#### Measured

Same panel, 2026-09-05. Membership, since the spelling is what splits.

| invocation | bash 5.3 | bash 3.2 | dash | ksh93 | zsh |
| --- | --- | --- | --- | --- | --- |
| `sh script.sh` | — | — | — | — | — |
| `sh -c cmd` | `c` | `c` | — | `c` `s` | — |
| `sh -s < pipe` | `s` | `s` | `s` | `s` | `s` |
| `sh < pipe` | `s` | — | `s` | `s` | `s` |
| `sh < file` | `s` | — | `s` | `s` | `s` |
| `sh` at a terminal | `s` | — | `s` | `s` | `s` |
| `sh -s -c cmd` | `c` `s` | `c` `s` | `s` | `c` `s` | `s` |

Three things to read out of it:

- **`c` is two against two.** bash and ksh93 show it, dash and zsh do
  not, and there is no majority to follow. The POSIX preset answers yes
  from the text rather than from a vote: `$-` is defined as the option
  flags specified on invocation, and `-c` is one of them.
- **`s` is unanimous for the standard-input route** — with `-s` written
  or without it, from a pipe, from a file, and at a prompt, which is the
  same route with a person on the other end. That half is a rule and not
  an axis.
- **`s` under `-c` is ksh93 alone.** Read down its column and its rule is
  "no script file was named" where the other three's is "the program came
  from standard input"; the two agree on every other row. The last row is
  what keeps the letter from being read off the route alone: `-c` wins
  about where the program comes from and *all four* still show `s` when
  `-s` was written.

**bash 3.2 dissents on the implicit standard-input route**, showing `s`
only where `-s` was written. That is a shell disagreeing with its own
later build rather than a panel disagreement, so it is recorded here and
not modeled; the bash dialect follows 5.3, as it does everywhere else.

### The login letter, `l`

**The rule.** `l` is in `$-` when the shell was started as a login shell
and the dialect says so. Login-ness itself is the front end's —
`interp.Runner.LoginShell`, carried in beside `Interactive` and `Route`
and for the same reason: `interp` never saw an argument vector, and no
`set` letter turns login-ness on for the option table to have written.
Whether the letter is written is `Semantics.LoginShowsLInDollarDash`,
*read* rather than `ask`ed, exactly as the two route letters above are.

Either route to login-ness counts, because both are measured to: an
explicit `-l` or `--login`, and a dashed `argv[0]` with no option written
at all — which is what `login` and every terminal emulator's "run as a
login shell" does. The two routes differ over which startup files get
read, which is a different question (see `LoginProfileWhenNonInteractive`),
and not over this letter.

#### Measured

Same panel, 2026-09-06. Membership, since the spelling is what splits.

| invocation | dash | bash 5.3 | bash-as-`sh` | bash 3.2 | ksh93 | zsh |
| --- | --- | --- | --- | --- | --- | --- |
| `-lc cmd` | — | — | — | — | `l` | `l` |
| `-l -c cmd` | — | — | — | — | `l` | `l` |
| `--login -c cmd` | *refused* | — | — | — | `l` | `l` |
| `argv[0]` dashed, `-c cmd` | — | — | — | — | `l` | `l` |
| `argv[0]` undashed, `-c cmd` | — | — | — | — | — | — |
| `-l script.sh` | — | — | — | — | `l` | `l` |
| `-l -s`, program on standard input | — | — | — | — | `l` | `l` |
| `-c cmd` | — | — | — | — | — | — |

Whole strings for the first row, which is why every case pins membership:
bash writes `hBc`, ksh93 `chsBl`, zsh `569Xl` and dash nothing at all.

Four things to read out of it:

- **Four against two, so it is an axis.** ksh93 and zsh show the letter;
  dash and all three bash columns do not. There is no majority to follow.
- **bash's silence is a decision and not an omission.** It keeps the fact
  in `shopt login_shell`, which reads `on` for exactly the invocations
  this letter would mark and `off` at status 1 otherwise, so the shell
  answers the question somewhere else. Neither ksh93 nor zsh has `shopt`
  at all.
- **`login_shell` is an indicator, and so is `restricted_shell`.** They are
  the only two of the sixty names bash 5.3.15 lists that a `shopt -s` or
  `shopt -u` does not move: measured a name at a time in bash 5.3.15 and
  bash 3.2.57, both report status 0 with nothing on standard error and both
  leave the name exactly where it was — `shopt -s login_shell` in a shell
  that is not one leaves it off, and `shopt -u login_shell` in a login shell
  leaves it on. Every other name really moves. That is a request taken and
  ignored rather than a request refused, which matters because `shopt -p` in
  a login shell dumps `shopt -s login_shell` and an agent harness sources
  that dump back before every command (#1709).
- **The spelling does not matter and the route does not matter.** Every
  column answers the same for a bundle, for unbundled letters, for the
  long word, for a dashed `argv[0]`, and on the command-string, script
  and standard-input routes alike. The letter is about what the shell
  *is*, not how it was told.
- **dash refuses `--login`** outright — `Illegal option --` at status 2 —
  which is why its `Login` spelling is `-l` alone.

**zsh's letter is also a `set` option.** Measured: `zsh -c 'set -l'` exits 0
and `$-` becomes `569Xl`; `zsh -lc 'set +l'` exits 0 and `$-` becomes
`569X`; `setopt login` and `unsetopt login` do the same, and its `set -o`
listing carries `login on` for a login shell. So in that one shell
login-ness is a mutable option rather than only a startup fact. The option
half is implemented since #1727 — `setopt login`, `unsetopt login`,
`[[ -o login ]]` and the three listings all move and all read the
invocation's answer when nothing has moved them, which is how a login shell
can be told apart from that namespace at all. It is the opposite of bash's
answer for its own spelling two bullets up, where the request is taken and
ignored, and the two were measured a name at a time rather than one read off
the other. The **letter** is still recorded: ours reports the startup fact
and refuses `set -l`, so `set +l` does not take the letter back out.
**ksh93 is the mirror image and stranger**: `set -l` is a bad option
there, and its own `set -o` listing says `login_shell off` in the very
invocation whose `$-` contains `l` — a shell disagreeing with itself,
recorded and not modeled. Neither is implemented, and neither is asked by
a case.

### The letters an interactive shell starts with are a second vector

**The rule.** What `$-` begins with is `Semantics.DefaultOptionLetters` for a
script and `Semantics.InteractiveOptionLetters` for an interactive shell, and
the second **replaces** the first rather than adding to it. An empty second
string means the shell has no separate answer and the first stands for both.

It is a different question from whether the shell is interactive at all, which
is unanimous and needs no axis; this is what being interactive *turns on*, and
the panel disagrees about it four ways.

#### Measured

Same panel and date. `sh -i script.sh` with a scratch `HOME`, so the shell is
interactive without a terminal — which is the whole of the condition, measured:
`-i` alone is enough and no prompt is needed.

| shell | a script | `-i script.sh` | what moved |
| --- | --- | --- | --- |
| bash 5.3 | `hB` | `hiBH` | adds `H` |
| bash 3.2 | `hB` | `hiB` | adds nothing |
| dash | *(empty)* | `i` | adds nothing |
| ksh93 | `hB` | `imBE` | **drops `h`**, adds `m` and `E` |
| zsh | `569X` | `569XZi` | adds `Z` |

**ksh93 is why the field replaces rather than appends.** A "letters to add"
field would have recorded three shells correctly and one wrongly, and the one
it got wrong is the only one that makes the shape visible. What moved is real
and was checked from the other side with `set -o`: `trackall` is on for a
script and off when interactive, which is the `h`; `rc` — the option that reads
`$ENV` — comes on only when interactive, which is the `E`.

bash's `H` is the same check: `histexpand` and `history` are both off for a
script and on under `-i`, and the letter is `histexpand`'s.

**bash 3.2 dissents against its own later build**, and by route as well: it
answers `hiB` for `-i script.sh` and `hiBHc` for `-i -c`. 5.3 is the panel
member that counts, as it is everywhere else.

#### `m` is not in the field, and must not be

Every shell in the panel that reports `m` reports it *because* the monitor is
running — `set -m; echo $-` shows the letter in bash and ksh93 alike — so the
letter has to come from `Runner.monitor` or not at all. Writing it into a
startup string would report a monitor that is not there, and being interactive
and having job control would stop being separate facts, which is exactly why
`Interactive` and `JobControl` are separate fields.

The table above is measured **without a terminal**, and that turns out to be
half the question. See below.

### An interactive shell runs the monitor, and one shell needs no terminal to

**The rule, and it is a rule.** `-i script.sh` **through a pseudo-terminal**
turns the monitor on in every shell in the panel. Measured 2026-09-05 with a
scratch `HOME` and a scratch `HISTFILE`:

| shell | no terminal anywhere | through a pseudo-terminal |
| --- | --- | --- |
| bash 5.3 | off, `hiBH` | **on**, `himBH` |
| dash | off, `i` | **on**, `mi` |
| ksh93 | **on**, `imBE` | **on**, `imBE` |
| zsh | off, `569XZi` | `m` in `$-` — `569XZim` — and jobs announced, while its own `set -o` still lists `monitor off` |

So the earlier reading — that ksh93 is *the only shell* that turns the monitor
on here, and that this is therefore one shell against three and an axis rather
than a rule — was an artifact of measuring only the terminal-less case. Every
shell does it. **ksh93 is unique only in not needing a terminal to do it.**

That is `Semantics.InteractiveMonitorNeedsATerminal`: bash yes · dash yes ·
ksh93 no · zsh yes. The preset says **yes**, and this is the rarer case where
the text does not decide: XCU enables `-m` by default for interactive shells
and puts no terminal in that sentence, but defines job control throughout in
terms of a controlling terminal, so the sentence is silent about having none
rather than permissive about it. Silent text gets the answer that claims less
— a shell with no terminal does not report a monitor — which is three of the
four as well.

**It is not `MonitorNeedsATerminal` asked twice**, and bash separates them in
one binary: `bash -c 'set -m'` with no terminal turns the monitor on, and
`bash -i script.sh` with no terminal leaves it off. An explicit request from a
script and the automatic one an interactive shell makes for itself are two
questions, and one shell answers them differently.

#### Which terminal counts

**A terminal on any one of the three standard streams**, and that is measured
rather than chosen. A pseudo-terminal on standard input alone, on standard
output alone, and on standard error alone each turn the monitor on in bash,
dash and zsh. A *controlling* terminal with all three redirected elsewhere does
**not** — all three report it off.

So the question the front end has to answer is about the descriptors it was
handed, which is the only question it could have answered: `interp` may not ask
the process what it holds, and `driver.Shell.hasTerminal` is where the answer
is established. It is a different question from `Interactively`, which asks
only about standard input because a prompt is drawn on the stream the lines
come from.

#### One shell disagrees with itself

The announcement and the remark were measured on this route too, and each has
a section of its own below. What is left here is neither.

**zsh disagrees with itself** and what is recorded is the state its other two
readers report. With a terminal it puts `m` in `$-` and announces its jobs
while `set -o` still lists `monitor off`. Ours is consistent across all three
readers, which matches zsh on two of them.

### An interactive shell that cannot have job control says so, in two of the six

The monitor an interactive shell turns on for itself is one it can be denied,
and two shells remark when they are.

Measured 2026-09-05 with **no terminal on any of the three standard streams**,
scratch `HOME` and scratch `HISTFILE`:

| shell | what it says before the program runs |
| --- | --- |
| bash 5.3.15 | `bash: cannot set terminal process group (11143): Inappropriate ioctl for device` **and then** `bash: no job control in this shell` |
| bash 3.2.57 | `bash: no job control in this shell` — that line **alone** |
| bash 3.2 as `sh` | `sh: no job control in this shell` — that line alone, with its own name |
| dash | `<name>: 0: can't access tty; job control turned off` |
| ksh93u+ | nothing — it needs no terminal and runs the monitor anyway |
| zsh 5.9.2 | nothing — it drops the monitor without a word |

That is `Diagnostics.NoJobControlAtStartup`, empty for the two that stay quiet
and empty in the base: the standard does not have the shell remark on this.

**When it is said** is the same question the monitor answers, one step on: an
interactive shell wanted the monitor and had no terminal to run one on, which
is `Semantics.InteractiveMonitorNeedsATerminal` saying yes with no terminal
anywhere. So it is written where that decision is made, and the two shells
that stay quiet are the one that needs no terminal and the one that simply
drops it.

**On every interactive route and on none of the others.** Measured: the same
line comes out of `-i script.sh`, `-i -c` and `-i -s` alike, and out of a plain
script or a plain `-c` never.

#### Who is named, which is not the same answer twice

| shell | `-i script.sh` | `-i -c` and `-i -s` |
| --- | --- | --- |
| bash (all three) | its own name | its own name |
| dash | the **script's** path | its own name |

dash is naming `$0` on both, and bash is naming itself on both. That is
`Diagnostics.NoJobControlAtStartupNamesTheScript`, dash alone — a field rather
than a reading of `InvocationNamesTheUnreadLine`, which dash is also the only
shell to set. One shell answering two questions the same way is not evidence
that they are one question.

Note that bash writes its name two ways in the same area: `bash` here, and the
whole path it was invoked by for a script it could not open — `/opt/homebrew/bin/bash: /nonesuch/s.sh: No such file or directory`. This
shell writes what it was invoked by throughout, which is the convention
everywhere else in it and not a decision taken here.

#### The pid line is not reproduced, and that is a decision

bash 5.3.15 writes a line above the remark that carries its own process id:
`bash: cannot set terminal process group (11143): Inappropriate ioctl for
device`. It is left out, for three reasons in order of weight.

1. **Two of the three bash members do not write it.** bash 3.2.57 and bash 3.2
   run as `sh` write the second line alone. So it is one version's extra line
   rather than bash's wording, and reproducing it would make this shell agree
   with one member of the panel and disagree with two.
2. **There is nothing here whose failure it would describe.** It is bash
   reporting a `tcsetpgrp` that returned `ENOTTY`. This shell makes no such
   call on this path, so the line would be a report of an event that did not
   happen.
3. **It carries a pid**, which no script can act on — it is the shell's own —
   and which means no recording of it is the same twice.

#### What the corpus cannot say about this

Nothing, and the third reason above is why: the corpus runs bash 5.3.15, whose
first line has a different number in it every run. `docs/spec/invocation.md`
already records that as the reason none of the `-i script.sh` grid is a corpus
case. The evidence is the table above, the per-dialect wordings in
`dialect/*/`, and a driver test that runs the front end with files on all three
streams and asserts the whole line.

### The announcement splits where the monitor does not

The monitor is unanimous on `-i script.sh` with a terminal. **What the shell
says about the job is not**, and this is where the panel comes apart.

Measured 2026-09-05 through a pseudo-terminal, scratch `HOME` and scratch
`HISTFILE`, on `sh -i script.sh` running

```
echo one
sleep 0.3 &
echo two
sleep 1.2
echo three
```

| shell | the job starting | the job ending |
| --- | --- | --- |
| bash 5.3.15 | — | — |
| bash 3.2.57 | — | — |
| bash 3.2 as `sh` | — | — |
| dash | — | `[1] + Done                       sleep 0.3` |
| ksh93u+ | `[1]	<pid>` | `[1] +  Done                    sleep 0.3 &` |
| zsh 5.9.2 | `[1] <pid>` | `[1]  + done       sleep 0.3` |

So `Runner.JobControl` cannot follow the monitor here: three shells have
somebody to tell on this route and bash has nobody. That is
`Semantics.InteractiveScriptAnnouncesJobs` — bash no · dash yes · ksh93 yes ·
zsh yes — and dash's blank first column is the *other* axis,
`AnnouncesBackgroundJob`, which it alone answers no. Both are read, and dash is
why they are two fields.

#### It is not about where the commands come from

The obvious reading of bash's silence — that a shell announces a job only to
somebody typing at it — is wrong, and the grid is what rules it out. The same
script on four interactive routes, same panel, same date, `$-` read from
inside:

| shell | `-i script.sh` | `-i` at a terminal | `-i < script` | `-i -c` |
| --- | --- | --- | --- | --- |
| bash 5.3.15 | — · — <br> `himBH` | start · end <br> `himBHs` | start · end <br> `himBHs` | start · — <br> `himBHc` |
| bash 3.2.57 | — · — <br> `himB` | start · end <br> `himBH` | start · end <br> `himBH` | start · end <br> `himBHc` |
| bash 3.2 as `sh` | — · — <br> `himB` | start · end <br> `himBH` | start · end <br> `himBH` | start · end <br> `himBHc` |
| dash | — · end <br> `mi` | — · end <br> `smi` | — · end <br> `smi` | — · — <br> `mi` |
| ksh93u+ | start · end <br> `imBE` | start · end <br> `imsBE` | start · end <br> `imsBE` | start · end <br> `icmsBE` |
| zsh 5.9.2 | start · end <br> `569XZim` | start · end <br> `569XZims` | — | start · end <br> `569XZim` |

`-i < script` puts the program on a **pipe**: there is no terminal to read
commands from, and bash announces both anyway. `-i -c` draws no prompt at all,
and bash announces the start there too. bash is quiet on exactly one
interactive route, the one whose program is a **named file**, which is why the
axis names the route rather than the terminal.

The `-i -c` column is a different split — bash, ksh93 and zsh announce and dash
does not — and so it is a different axis, deliberately not taken here. The zsh
`-i < script` cell is empty because that invocation does not finish: zsh draws
its prompt and waits, and nothing was measured rather than something guessed
at.

#### The notice rides on the monitor

Measured on the same route with **no terminal anywhere**, so the monitor is
off in every shell but one:

| shell | monitor | the job starting | the job ending |
| --- | --- | --- | --- |
| bash 5.3.15 | off | — | — |
| bash 3.2.57 | off | — | — |
| bash 3.2 as `sh` | off | — | — |
| dash | off | — | — |
| ksh93u+ | **on** | `[1]	<pid>` | `[1] +  Done                    sleep 0.3 &` |
| zsh 5.9.2 | off | — | — |

dash and zsh, which say something about the job *with* a terminal, say nothing
without one; ksh93, which needs no terminal for the monitor, announces both
ends without one. So the notice is not a second question about the terminal —
it is gated on the monitor the shell is already running, and the axis exists
for the one dialect that runs a monitor and stays quiet anyway.

That is why `Runner.SetInteractiveJobNotices` reads the monitor rather than
taking a terminal, and why it has to be called after `SetInteractiveMonitor`.

#### Turning the monitor off turns the job back into a plain child

The rows above are about a monitor that was never granted. Turning one **off**
at a prompt — `set +m`, `unsetopt monitor` — is the other direction, and it has
to change what the shell does and not only what it says about itself.

Measured 2026-09-10 and again 2026-09-11, interactive on a pseudo-terminal and
non-interactively, by having the job ask the kernel for its own group:

| | with the monitor | without it |
| --- | --- | --- |
| bash 5.3.15 | its own group | **the shell's** |
| bash 3.2.57 | its own group | **the shell's** |
| ksh93u+ | its own group | **the shell's** |
| dash | its own group | **the shell's** |
| zsh 5.9.2 | its own group | **the shell's** |

Unanimous, so it is shared ground rather than an axis, and the same answer a
plain non-interactive `sh -c 'cmd &'` gives — the monitor is off there too.
This shell gave a background job a group of its own on every route (#1738),
which promises exactly what the option says is not happening: a job in a group
of its own is one the shell can signal as a group, hand the terminal to, and
stop.

Three things follow from taking the group away, and each is measured rather
than reasoned about:

- **The start notice is an axis.** With the monitor off, bash 5.3.15, bash
  3.2.57 and ksh93 still print `[1] <pid>` and zsh and dash say nothing —
  `Semantics.AnnouncesBackgroundJobWithoutTheMonitor`. It narrows
  `AnnouncesBackgroundJob` rather than replacing it: a dialect that announces
  nothing at a prompt announces nothing here either.
- **The finish notice is not.** With the monitor off no shell in the panel
  says anything when the job ends — not even the two that still announce its
  start — so `FinishedJobNotices` is silent and there is nothing to ask. dash's
  late report of a finished job with an empty command is its own oddity,
  measured and not reproduced.
- **`kill %1`, `bg` and `fg` reach the process instead.** They reach a job
  through a *group* led by the job's pid, and with no group of its own that
  group is the shell's: signaling it would reach the shell, every other job it
  started and, on a terminal, the whole foreground group. So a job that leads
  no group is signaled as a process — `Job.ownGroup` is what says which, and
  it is written with the pid because the two are one fact about one process.

The process signal goes through the gate and the group signal does not, and
that difference is deliberate: the group is the embedder's hook being called,
and the process is this package asking the kernel itself. Every signal `interp`
delivers is visible to the boundary — see `interp/signalgate.go`.

**The foreground half is unchanged.** A command the front end is watching still
gets a group of its own whether or not the monitor is on, because there the
group is not a promise about job control but the only way a shell that is
watching can stop or interrupt what it is watching.

#### Where the notice goes, and when

**To standard error, between commands.** Measured on the same route with the
two writable streams separated: dash and ksh93 both write the `Done` row to
standard error and neither writes anything to standard output — the stream a
prompt already writes its own notices to.

It lands between the command the job outlived and the command after it, and
**also after the last command when there is none**: `sleep 0.3 &` followed by
`sleep 1.2` and nothing else still prints the row in dash, ksh93 and zsh. So
the front end reports after each line rather than before the next one, which
is the only placement that has a last time.

#### What the corpus cannot say about this

Nothing here is a corpus case, and the reason is recorded on
`invoke/a-script-is-not-interactive`: `-i` away from a terminal makes bash and
dash announce that job control is off, and bash's line carries a pid, so the
output is not the same twice. The evidence is the table above, the per-dialect
answers in `dialect/*/`, and the front end's own test that the invocation
chooses between the two vectors.

### The order of the letters is the shell's, and it is not the order they were set in

Everything above reads `$-` as *membership*, which is what a script does
and what the axes model. The **string** is a separate fact and no shell
in the panel builds it the same way. Measured 2026-09-05, `set -f; set
-u; set -e` and then `echo "[$-]"`
(`special/dollar-dash-orders-the-letters-its-own-way`):

| shell | `$-` |
| --- | --- |
| dash | `ufe` |
| bash 5.3, bash-as-`sh`, bash 3.2 | `efhuBc` |
| ksh93 | `cefhsuB` |
| zsh | `569Xefu` |

None of them is the order the options were written in, and only two
resemble each other. bash sorts the lowercase letters and keeps the ones
it started with as a suffix; ksh93 sorts everything it holds; zsh puts
its digits first; and dash's `ufe` is neither sorted nor chronological —
it is its own option table's order, which is a fact about a table nobody
outside dash can see.

So a script may test `case $- in *e*)` and may not compare `$-` against a
string, and an implementation has no order to inherit: it has to pick
one, per dialect, the way it picks the letters. The whole string is also
recorded by route in `special/dollar-dash-in-full` and
`special/dollar-dash-in-full-from-a-script`, which is where the
route-dependence above shows up as text rather than as membership — ksh93
carries `s` for a command string and drops it for a script, landing on
exactly bash's `hB`.

## Where it lives

`driver.Interactively` is the prompt decision and is the only place that
makes it, so a dialect binary has a prompt by existing rather than by
copying one. The ioctl behind it is `repl.IsTerminal`, next to the rest
of the termios code, because a second implementation of "is this a
terminal" is how the two answers drift apart.

Being interactive is the other fact, and it is carried rather than
decided twice: `driver` sets it once, on the way out of reading the
operands, and hands it to `interp.Runner.Interactive`. It was decided
per route once, in the one branch that had nothing to run, and `sh -i
script.sh` therefore ran the script with `-i` dropped on the floor
(#472).

The route is the same shape: read once out of the operands, handed over
as `interp.Runner.Route`, and read by three unrelated things — the status
one dialect gives a failed expansion under `-c`, the fatality another
gives a readonly reassignment, and the letters above. One field rather
than a bool per route, because a second name for one measurement is the
one that drifts. `-s` as written travels beside it, in
`Runner.StandardInputOption`, for the single invocation where the two
differ: `sh -s -c cmd` runs the command string and still shows `s`.

`interp` asks a related question for `select` and `read -p`, and it now
gets the same answer. It used to get a weaker one — a character device,
excepting the null device — because `interp` cannot import `repl`; the
ioctl is in `internal/tty` since #525, which both import, and
`repl.IsTerminal` is a forwarder to it that keeps its exported name for
the front end. The two are still not the same *question*: the front end
asks about the **invocation**, and a builtin asks about whatever stream
`-u` resolved to. They are one implementation of "is this a terminal".

## A program on standard input shares the descriptor with itself

**The rule.** When the program is on standard input, the shell holds *one*
descriptor: the program and the program's own input are the same stream. What
the shell has not read yet is what a `read` in the script finds, what a command
the script starts inherits, and what an `exec 0<` replaces. The shells split
over one thing only — **how much the shell takes at a time**.

- **A line at a time** — bash 5.3, bash 3.2, bash-as-sh, ksh93, zsh. The line
  after the one being run is still on the descriptor.
- **In blocks** — dash. What the block took has left the descriptor, so the
  script finds only what had not arrived yet.

`Semantics.StdinProgramReadInBlocks` is the axis. It is a bool rather than an
`Answer`: the panel is four to one, so a common denominator exists, and
"refuse to read a piped script" is not an answer any shell could ship.

### Measured

Panel: bash 5.3.15, bash 3.2.57, dash, ksh93u+ 2012-08-01, zsh 5.9, on macOS
25.5 — measured 2026-09-05, by piping each program into each shell with no
operands. bash 3.2 and bash-as-sh answer with bash throughout and are left out
of the table.

| the program, piped in | dash | bash | ksh93 | zsh |
| --- | --- | --- | --- | --- |
| `read x` / `echo "[$x]"` / `DATA-LINE` / `echo end` | `[]`, then DATA-LINE not found **at line 3**, then `end` | `read` takes line 2, so nothing is printed; DATA-LINE not found **at line 2**, then `end` | as bash | as bash, with no line in its wording |
| `read x; echo "$? [$x]"`, nothing after | `1 []` | `1 []` | `1 []` | `1 []` |
| `read x` ×3 then `A` / `B` / `C` | all three read fail at 1; A, B and C run as commands | the three reads take A, B and C | as bash | as bash |
| `while read -r l; do echo "got:$l"; done` then `A` / `B` / `C` | A, B and C run as commands | `got:A`, `got:B`, `got:C` | as bash | as bash |
| `echo one` / `cat` / `NOT-A-COMMAND` / `echo end` | `one`, then NOT-A-COMMAND not found, then `end` | `one`, then `cat` prints the rest of the program | as bash | as bash |
| `cat <<EOF` / `body` / `EOF` / `echo after` | `body`, `after` | same | same | same |
| `echo one \` / `two` / `echo three` | `one two`, `three` | same | same | same |
| `if true` / `then` / `echo yes` / `fi` / `echo done` | `yes`, `done` | same | same | same |
| `printf 'A\nB\n' > d.txt` / `exec 0< d.txt` / `echo never` | `never`, then A and B run as commands | A and B run as commands; `echo never` is never read | as bash | as bash |

The same programs delivered as a **file operand** are unanimous and are the
block answer throughout, which is the control: a script named on the command
line is opened separately from standard input, so nothing is shared and there
is nothing to disagree about. `sh < script` is *not* the file route — it is
this one, and it answers exactly as a pipe does in all five.

### What the line numbers say

A line handed to the script is never parsed, so it is never counted. With
`read x` on line 1 taking line 2, bash and ksh93 report the physical line 3 as
**line 2**. That falls out of numbering by what the parser consumed rather than
by what arrived, and it is why the front end carries the count across the
pieces it reads — `syntax.NewParserAt`.

### dash's block is a buffer, not a decision

Measured from the other side: with the same program produced *slowly* — the
`read` line written, a second's pause, then the data line — dash's `read` gets
the data, exactly as bash's does. The block is however much had arrived, so
where its boundary falls is a fact about the producer's timing rather than
about dash. All five also run the first line before a second one written a
second later arrives, so none of them waits for end of input.

What that means for the model is that the two answers are one rule with two
units, not two rules: **read a piece of standard input, run what parses, come
back for more from whatever descriptor 0 is now.** dash's piece is a block and
everyone else's is a line. It is why `exec 0<` mid-program behaves as it does
in dash too — the rest of the block runs first, and *then* the file becomes the
program.

### How much is fetched is not how much is taken

The unit above is what the shell is *entitled to take*. How many system calls
it costs to take it is a separate question, and the answer turns on whether
the descriptor can be rewound rather than on which shell is running.

Measured, same panel and same date, by running each program in the table above
twice — once piped in, once as `sh < program` from a regular file. **Every
shell produces byte-identical output and status on both routes**, for all nine
programs. That is the observation the fetch rests on: a shell may read as far
ahead as it likes on a descriptor it can put back, because putting it back is
indistinguishable from never having read.

On a pipe it is not indistinguishable, because an over-read cannot be given
back. There the line has to be taken a byte at a time, and that is what the
cost of this route is:

| a 20 000-line, 249 KB program, best of seven, macOS 25.5 | |
| --- | --- |
| bash 5.3, `< file` | 0.055 s |
| bash 5.3, piped | 0.127 s |

The two-fold difference between a real bash's own two routes is the same fact
from the other side: the rewindable one is cheaper, and it is cheaper by about
what a system call per byte costs.

### Why it matters

`curl … | sh` is this route. Taking the whole input for every dialect — which
is what the front end did until #470 — meant a script's `read` found end of
input and the data the script was reading was executed as commands instead.

### Where it lives

`driver`'s `program`, which is the only thing that reads a program. The
descriptor is asked for on every refill rather than captured once, because the
script may point it somewhere else.

The line reader has the two fetches. Where the descriptor rewinds it reads a
buffer and pushes back everything past the newline, relative to the read that
caused the overshoot — relative rather than to a computed position, because
between two lines the script has run a command and a command that read
descriptor 0 has moved it. Where the descriptor does not rewind it takes one
byte at a time, because reading past the line is exactly the bug. Whether it
rewinds is asked of the descriptor once and remembered against it, since a
pipe, a socket and a terminal are all `*os.File` and the type answers nothing
(#567).

## A parse failure on standard input does not end every shell

**The rule.** A line the shell cannot parse is reported and the shell stops —
except in zsh, and except when the program arrived on **standard input**, where
zsh reports the line and reads the next one.

    printf 'echo one\n{ fi; }\necho three\n' | sh

    dash  → one, the complaint, status 2
    bash  → one, the complaint, status 2
    ksh93 → one, the complaint, status 3
    zsh   → one, the complaint, three, status 0

`Diagnostics.StdinProgramSurvivesAParseFailure` is the answer, beside
`CommandStringParsedWhole` because it is the same kind of question — how the
front end reads a program, per route, for one dialect — and this is the route
that one does not cover.

**It is the route and not the text.** The same three lines in a file stop zsh
too, with nothing after the complaint and status 1. That is why the answer
cannot be folded into the parse failure's status, which knows nothing about how
the program arrived, and why the front end is where it is asked.

**The status is left behind, not chosen.** Whatever runs after the bad line
reports as it always would:

    printf '{ fi; }\nexit 7\n'   | zsh   → 7
    printf '{ fi; }\nfalse\n'    | zsh   → 1
    printf 'echo one\n{ fi; }\n' | zsh   → 1

The last is the shell being left with the parse status because nothing ran
after the failure. So the mechanism is: record `StatusForParseError`, then read
on — not "exit 0 after a bad line", which the first two rows disprove.

**Recovery is per line and repeats.** Two bad lines are two complaints and two
recoveries, ending in 0. A failure inside a construct spanning several lines
recovers the same way: `echo one`, `if :; then`, `  fi fi`, `fi`, `echo five`
piped in produces two complaints and then `five`.

The implementation has one requirement worth stating, because getting it wrong
loops forever rather than failing: the parser keeps its error until the pending
text is *replaced*, so the failed line has to be retired before the next one is
read. That is `program.fill(true)`, the same call the reader makes when
everything in hand has been parsed.

### Measured

Panel: bash 5.3.15, bash 3.2.57, dash, ksh93u+ 2012-08-01, zsh 5.9.2, on macOS
25.5 — measured 2026-09-05. Recorded as
`syntax/standard-input-reads-on-past-a-parse-failure` and the three rows around
it: the file route as the control, `exit 7` after the bad line for the status,
and a program whose last line is the bad one for the other half of it.

## A refused line at a prompt sets `$?`

**The rule.** A line the parser refuses at an interactive prompt is reported,
nothing runs, and `$?` becomes the dialect's parse-failure status. The session
goes on to the next line.

    false
    fi
    echo "st=$?"

    bash 5.3    → the complaint, st=2
    bash-as-sh  → the complaint, st=2
    bash 3.2    → the complaint, st=258
    dash        → the complaint, st=2
    ksh93       → the complaint, st=3
    zsh 5.9.2   → the complaint, st=1

**It is a status being set, not one being left.** Substituting `true` for
`false`, or `(exit 5)`, moves no cell: every column answers the same number
whatever ran before the refused line. That is what makes it worth stating —
ours left `$?` where the previous command put it, so a refused line reported
the status of the command *before* it, and `$?` is not only read by the next
command. It is what a prompt draws a failure indicator from and what the prompt
hooks are handed, so the shell said "the last thing succeeded" at the exact
moment it did not.

**It is the same number the shell would have exited with**, which is why the
front end asks `Diagnostics.StatusForParseError` here rather than carrying a
second answer: the same table decides a script's exit status, standard input's
recorded status in the section above, and this. One column parts company —
bash 3.2 answers 258 at a prompt where it exits 2 — and 258 is not a number a
process can exit with, so this is a bash 3.2 quirk about the number's width
rather than a second question. No dialect here targets bash 3.2.

**The error is what is asked about, not just the fact of one.** Some failures
this parser finds while reading are *run*-time failures to a panel shell — a
`for` whose name is not a name is bash's example — and carry that shell's
run-time status instead of its syntax status. Handing the front end the
dialect's function rather than a number is what keeps the prompt and the script
from drawing that line in different places.

**Route.** Measured through a pseudo-terminal and again with `-i` on a pipe;
the two agree in every column but one. dash with `-i` on a *pipe* stops at the
complaint and exits 2 rather than reading on, and dash at a terminal reports 2
and carries on — so the terminal is the route this rule is about, and the pipe
is standard input's question in the section above.

### Measured

Panel: bash 5.3.15, bash 3.2.57, dash, ksh93u+ 2012-08-01, zsh 5.9.2, on macOS
25.5 — measured 2026-09-07, with an isolated `HOME`, `ZDOTDIR`, `HISTFILE` and
`ENV=/dev/null`. Not in the corpus: it drives no prompt, and every route it has
is a non-interactive one.

## Writing the input back: `set -v`

**The rule.** Under `-v` the shell writes each **physical line of its input**
to standard error as it reads it, before running anything that line says. It
is the input that is echoed and not the commands found in it: a comment and a
blank line are echoed although they run nothing, a here-document body is echoed
although it is never parsed as input, and a compound command's lines are all
echoed before the first iteration of it runs. The line that turns the option on
is not echoed by the shell it turns on — lines read while it was off are spent,
not saved.

The front end is where this lives, and it has to be: the runner knows only
whether the option is on, and the raw text belongs to whatever read it.

### What it writes back when the input is wrong

**The rule.** The shell writes back what it **read** before it says anything
about it. That covers three shapes and they are one rule, measured unanimous
2026-09-05 from a script file with `-v` on the invocation, both streams
together and apart:

| the input | what comes out |
| --- | --- |
| `echo one` / `fi` | `echo one`, its output, **`fi`**, then the syntax error |
| `echo one` / `if true; then` | both lines, then the complaint about the input running out |
| `echo one` / `cat <<END` / `body` | all three lines, then the here-document remark, then the body |

The echo used to be driven by the logical lines the parser handed *back*, so a
line the parser refused was never written at all and the complaint arrived
alone (#772). The remark had the same shape from the other side: it was said
as soon as it was known, which put it in front of the lines it was about.

**How far it goes** is where the *reader* stopped, and that is not the line the
dialect blames. An unterminated quote is blamed on the line it was opened on in
bash and on the line after the input ended in dash and zsh, and all three write
back every line they read:

    echo one
    cat "abc
    echo three

is three echoed lines in all three shells, with bash's complaint naming line 2.
Where there *is* an offending token the walk stops on its line: `echo one` /
`fi` / `echo three` echoes two lines and not three.

**One shell reads further than that, and it is not modeled.** ksh93 takes its
input in blocks and writes back everything it had taken, so `echo one` / `fi` /
`echo three` echoes all three lines there, and a trailing blank line is echoed
*before* the last command runs. The amount is a property of its buffer rather
than of the language, so the corpus rows are shaped to put the failure on the
last line, where the question does not arise. ksh93 also prefixes a syntax
error with the line it had reached — `./s.sh: line 2: syntax error at line 3:`
— but only when `set -v` was turned on from inside the script, which is the
other reason those rows put the letter on the invocation.

### Which standard error it writes to

**The rule.** Descriptor 2 **as the script has pointed it**, read at the moment
of the echo — not the stream the shell was started with. Measured 2026-09-05
from a script file, unanimous across dash, bash 3.2, bash 5.3, bash-as-`sh`,
ksh93 and zsh:

| the script says | where the echo goes |
| --- | --- |
| `exec 2>&1` | the output, and standard error is left empty |
| `exec 2>/dev/null` | nowhere, until something points it back |
| a redirection on a *command* — `echo one 2>&1` | standard error, unmoved |
| the `exec` line itself | the descriptor it is about to replace |

The last two rows are the same fact as the first and need no rule of their own.
The echo happens when the line is *read*, and both a per-command redirection
and the `exec`'s own effect arrive when the line is *run*, which is later.

It matters beyond tidiness: with the two streams apart, the position of a
here-document's terminator against the body the command wrote is invisible, and
joining them is how a script asks. A front end holding its own stream loses the
echo out of the joined text, so the question could not be asked at all — which
is why the corpus row that asks it had to put the here-document body on
descriptor 2 instead (#582), a workaround for exactly this (#771).

### It walks the text once, and used to walk it once per line

The echo has to find the text of the lines it has not written yet. Recovering
them from a line *number* means splitting the whole program on newlines, and
the echo is asked once per logical line, so an n-line script split an n-line
string n times: a slice of every line in the program allocated and discarded
for every line echoed, which is n² work to write n lines.

Measured, best of five, macOS 25.5, on a program of assignments and nothing
else so that what is timed is the echoing rather than the running:

| lines | `-v`, before | `-v`, after | the same script with no `-v` | bash 5.3 `-v` |
| --- | --- | --- | --- | --- |
| 1 000 | 0.014 s | 0.005 s | 0.004 s | 0.006 s |
| 2 000 | 0.041 s | 0.006 s | 0.005 s | 0.006 s |
| 4 000 | 0.147 s | 0.010 s | 0.008 s | 0.008 s |
| 8 000 | 0.575 s | 0.016 s | 0.012 s | 0.012 s |
| 16 000 | 2.304 s | 0.028 s | 0.021 s | 0.020 s |

Eight times the lines was fifty times the work, which is the shape of an n²
and not of an n; it is a factor of four across the board now, and `-v` costs
about a third again on top of running the script rather than a hundred times
it. `set -v` is what people reach for when a script is misbehaving, so a shell
that gets slower the more it has to say is at its worst exactly when it is
being asked for help (#580).

### Where it lives

`driver`'s `sayVerbose`, the writer it is handed, and the position it carries.
`executeLines` says the echo and the remarks together, in that order and in
every branch, because "write back what was read before saying anything about
it" is one rule and three places to get it wrong.
The writer is `interp.Runner.Err` — the runner's own descriptor 2 — read afresh
at every call, which is the whole of the rule above; `Shell.errf` is the other
thing and is where a *diagnostic* goes, however the script has arranged its
descriptors. The position is a line
number **and a byte offset**: the number is what the parser reports and the
offset is what makes the walk one pass, and neither can be derived from the
other once the text is growing a line at a time. Lines read while the option is
off are walked past rather than skipped, for the same reason — the offset has
to keep up or the line after a mid-script `set -v` cannot be found.

## Startup files

**The rule.** A shell reads two kinds of file before it starts, and they
answer different questions. A **login** shell reads a *profile* — what a
person wants set once, for everything started from that session. An
**interactive** shell reads a *run-commands* file — what only makes sense at
a prompt, and what a script must not inherit.

A shell is a **login shell** when the first character of `argv[0]` is a dash.
That is a convention rather than a flag because there is nowhere else to put
it: `login` and every terminal emulator's "run as a login shell" exec the
shell with no arguments of its own. `-l`, and `--login` in the shells that
have it, say the same thing explicitly.

Being a login shell and being interactive are independent, and the four
combinations are not four rules. Three of them are unanimous; the fourth is
where the panel splits.

### Measured

Panel: bash 5.3.15, bash 3.2.57, bash-as-`sh`, dash, ksh93u+ 2012-08-01, zsh
5.9, on macOS 25.5 — measured 2026-09-05. Every row used a scratch `HOME`
containing a marker file for every name any of them reads, since a developer's
own `.profile` is not evidence about anything. `PATH` was set to a single
nonexistent directory before each run, so that the `path_helper` call in
`/etc/profile` and `/etc/zprofile` shows whether the *system* file was read as
well as the user's. The interactive rows were driven through a
pseudo-terminal pair.

| | bash 5.3 | dash | ksh93 | zsh |
| --- | --- | --- | --- | --- |
| interactive, login | `~/.bash_profile` | `~/.profile` | `~/.profile`, `~/.kshrc` | `~/.zshenv`, `~/.zprofile`, `~/.zshrc`, `~/.zlogin` |
| interactive, not login | `~/.bashrc` | *(nothing)* | `~/.kshrc` | `~/.zshenv`, `~/.zshrc` |
| **script, login** | ***(nothing)*** | `~/.profile` | `~/.profile` | `~/.zshenv`, `~/.zprofile`, `~/.zlogin` |
| script, not login | *(nothing)* | *(nothing)* | *(nothing)* | `~/.zshenv` |

The system-wide file goes with the user's throughout: every cell that names a
profile also read `/etc/profile` — `/etc/zprofile` for zsh — and every cell
that names nothing read nothing.

**The third row is the only disagreement, and it is one shell against three.**
bash reads nothing for a login shell with a script to run; dash, ksh93 and zsh
each read theirs. It holds on all four non-interactive routes — a script
operand, `-c`, a program on standard input, and `-s` — so it is a fact about
the shell rather than about the route, and bash 3.2 and bash invoked as `sh`
answer with bash 5.3.

**bash is not declining to be a login shell.** `shopt login_shell` is on for
`argv[0] = -bash`, and `$-` is unchanged. What it declines is reading a
startup file at all when it is not interactive — and an explicit `--login`
overrides that:

| non-interactive, with the option written out | bash 5.3 | dash | ksh93 | zsh |
| --- | --- | --- | --- | --- |
| `-l` | `~/.bash_profile` | `~/.profile` | `~/.profile` | its three |
| `--login` | `~/.bash_profile` | **refused**: `dash: 0: Illegal option --`, status 2 | `~/.profile` | its three |

So the split above is about login-ness *inferred from argv[0]*, and `-l` makes
the panel unanimous the other way. bash invoked as `sh` reads `~/.profile`
rather than `~/.bash_profile` under `-l`, which is the POSIX name for the file
and the reason the two spellings are one row here.

### What the profile sees

A profile is run *by* the shell that is about to run the script, and sees what
that shell sees. Measured in dash and ksh93, on the script route:

- `$0` and `$#` are the script's own. `-dash script.sh A B` runs a profile
  that reports `$0` as `script.sh` and `$#` as 2. On `-c` they are the
  command string's — `$0` is the name operand where there is one and the
  shell's own name where there is not — so the profile is behind the
  parameters rather than in front of them.
- The invocation's options are already in effect. `-x` traces the profile's
  own lines.
- `exit 3` in it exits 3 and the script never runs.
- A file that is not there is not a failure, in any of them.

### What is modeled, and what is not

`Semantics.LoginProfileWhenNonInteractive` is the third row and nothing else.
It is a bool: there is no third thing to do, and "refuse to start" is not an
answer any shell could ship. dash, ksh93 and zsh answer true and the POSIX
preset does too; bash alone answers false.

Its zero value is `false`, which is the *minority* answer — the opposite of
the way `StdinProgramReadInBlocks` is named, and deliberately. The majority
behavior here is to read a file out of the invoking person's home directory,
and a `Semantics` nobody has filled in belongs to a library embedder or to a
test rather than to a shell. Neither should touch a home directory because a
field was left at its default.

The interactive rows ask nothing, because all four read a profile there. Only
the script routes had a question.

One thing in the table is **not** modeled: the **system-wide file**.
`/etc/profile` is not read at all, on any route. Everything else in it is,
and the rest of this section is how.

### Which file, per dialect

`~/.profile` for every dialect was the whole of it, and it made a person's
`~/.bashrc` unreachable — the file was never opened by any route, so an
interactive session read none of an alias, a function, an `export` or a `PS1`
that somebody had written (#807). The names are the dialect's, in four slots:

| slot | when | bash | dash | ksh93 | zsh |
| --- | --- | --- | --- | --- | --- |
| unconditional | always | — | — | — | `.zshenv` |
| profile | login | `.bash_profile`, `.bash_login`, `.profile` | `.profile` | `.profile` | `.zprofile` |
| run-commands | interactive | `.bashrc` | `$ENV` | `$ENV` | `.zshrc` |
| late profile | login, after the run-commands file | — | — | — | `.zlogin` |

Read in that order, which is measured: `zsh -l -i` reads `.zshenv`,
`.zprofile`, `.zshrc` and `.zlogin`, so a person's `.zlogin` sees what their
`.zshrc` did.

Three properties of the table are behavior rather than naming, and each is a
field of its own:

- **The profile is a chain and exactly one link runs.** bash reads the first
  of its three that exists and looks at no other. Measured by removing them
  one at a time: with all three present it reads `.bash_profile`, with that
  gone `.bash_login`, with both gone `.profile`. Every other shell has one
  name, for which the rule reduces to itself.
  `Semantics.LoginStartupFiles`, whitespace-separated, most preferred first.
- **An interactive login shell reads the run-commands file in one shell and
  not the other.** `zsh -l -i` reads `.zshrc`; `bash -l -i` reads
  `.bash_profile` and stops, which is why every bash tutorial tells a person
  to source `~/.bashrc` from their `~/.bash_profile` by hand. This is the
  panel's one disagreement about ordering and the reason the four
  combinations of login and interactive are not four independent facts.
  `Semantics.InteractiveStartupFileWhenLogin`. It is asked only where the
  dialect has a file of its own name: a shell whose run-commands file is
  `$ENV` reads it in both cases, so `-sh -i` reads `~/.profile` and then
  `$ENV` in dash, ksh93 and bash-as-`sh` alike.
- **One dialect looks for all four under a directory a variable names.**
  `ZDOTDIR` redirects every zsh file rather than one of them, and it is read
  afresh for each — which is what lets a person's own `~/.zshenv` set it and
  have the rest follow. Set to the empty string it is *not* the home
  directory: measured, such a shell reads none of its files.
  `Semantics.StartupDirectoryVariable`.

### `$ENV`, and what POSIX mode does to the run-commands file

`$ENV` is the standard's interactive startup file, and it is read by a shell
that has no file of its own name — dash, ksh93 and the POSIX preset — **and by
any shell in POSIX mode**. Measured: `bash -i` reads `~/.bashrc` and does
nothing with `$ENV`, while bash invoked as `sh` reads `$ENV` and does nothing
with `~/.bashrc`. The same holds for zsh under that name.

That makes this the interactive half of what
`Semantics.NonInteractiveStartupVariable` records for `$BASH_ENV`, and the two
halves differ in one respect worth stating: the non-interactive file is
**suppressed** in POSIX mode and the interactive one is **substituted**. The
standard has an interactive startup file and no other kind, so there is
something for the mode to fall back to here and nothing there.

The mode is asked of the *runner* and of how the shell was **named**, not of a
second axis — the same reasoning as #691 and #733. Both, because they are not
the same moment: being called `sh` turns the mode on after the startup files
have run, which is measured (`set -o` in a file read by `-sh -i` reports
`posix off`), so a front end that only asked the runner would pick its file
before the answer existed.

`$ENV` is expanded before it is opened, since `$HOME/.shrc` is the usual
spelling. bash 5.3 goes further under `--posix` and reads *no profile at all*,
where bash 3.2 and bash-as-`sh` both read one; that is a difference between
two builds of one shell rather than a rule, so the `sh` answer — the one the
whole panel agrees on — is what is implemented.

ksh93 has one more measured default that is not modeled: with `ENV` unset
entirely it falls back to `$HOME/.kshrc`. That is a per-build default of a
variable rather than a startup slot, and no other shell in the panel has one.

### The invocation options

Four kinds, and they are the reason any of this is repairable. **A startup
file that breaks has to be escapable**: a shell whose only `~/.zshrc` fails
every time it starts is a shell a person cannot repair from.

| | bash | dash | ksh93 | zsh |
| --- | --- | --- | --- | --- |
| make it a login shell | `-l`, `--login` | `-l` | `-l`, `--login` | `-l`, `--login` |
| skip every startup file | — | — | — | `-f`, `--no-rcs` |
| skip the profile | `--noprofile` | — | — | — |
| skip the run-commands file | `--norc` | — | — | — |
| name the run-commands file | `--rcfile F`, `--init-file F` | — | — | — |

`Semantics.StartupFileOptions`, whitespace-separated spellings per role. A
single-dash entry of one letter also matches inside a bundle, so `-if` is `-i`
and `-f`; a double-dash entry matches a whole word. dash refuses `--login`
outright — `dash: 0: Illegal option --`, status 2 — which is why it names only
the short spelling.

`-f` is not the POSIX `-f`. zsh spends the letter on its escape hatch and
still expands patterns under it — measured, `zsh -f -c 'echo /etc/pas*'`
prints the file — which is why the spelling is per-dialect data rather than a
letter this front end has an opinion about. And its reach is *every* file:
`zsh -f -l -i` reads no `.zshenv`, no `.zprofile`, no `.zshrc` and no
`.zlogin`.

**An explicit login option is stronger than a dashed `argv[0]`.** It reads the
profile even where there is a script to run and the dialect says a login shell
started by argv[0] would not: `bash --login -c cmd` reads its profile where
`exec -a -bash bash -c cmd` reads nothing. So
`LoginProfileWhenNonInteractive` is about login-ness *inferred* from argv[0],
and the option overrides it. `--noprofile` beats both.

`--rcfile` **replaces** the run-commands file and loses to everything that was
already going to skip one: `bash --norc --rcfile f -i` reads neither, in either
order, and `bash --rcfile f -l -i` reads its profile, because a login bash was
not going to read a run-commands file at all.

Two measured behaviors of bash are deliberately **not** reproduced. Its long
options are only recognized *before* the single-letter ones — `bash -i --norc`
treats `--norc` as a script path and fails to open it — which is a parser
accident rather than a rule, and the core accepts them anywhere. And bash's
usage block, which the Diagnostics vector reproduces verbatim because it is a
measured diagnostic, lists a dozen long options this front end does not have
(`--debug`, `--help`, `--posix`, `--restricted`, `--version` and the rest);
trimming it would make the dialect stop sounding like the shell it names, so
the text is left as bash's own.

A long option the dialect does not name is **refused** rather than read as a
bundle of letters, which is measured and unanimous across the panel. That is
not only a better diagnostic: the letters of `--rcfile` include a `c`, so a
shell reading it as a bundle would take the next word as a command string and
run it.

### Where it lives

`driver`'s `startup`, one function reached by every route.

It was two — one the prompt used and one the script routes reached past — and
that split is exactly what made `~/.bashrc` unreachable. The routes do not
differ in *which* files they read, only in what they are: the unconditional
file is read by a script as much as by a prompt, and `-i` makes a shell
interactive on every route, so `sh -i script.sh` and `sh -i -c cmd` both read
the run-commands file. Measured on both.

The files are sourced after the dialect's prelude, after the invocation's
options are applied and after the runner is built, which is what the
measurements above require and is the same order the prompt route already used
(#482). After the prelude matters on its own: the prelude is shell and moves
shell state exactly as a script does, so a person redefining a prelude
function in their `~/.bashrc` wins only because theirs runs second.

## Called `sh`: the name starts the shell in POSIX mode

**The rule.** A shell invoked under the standard's own name — `sh` — starts in
POSIX mode. The name is the last element of `argv[0]` with one leading dash
removed, so `sh`, `/bin/sh`, `./sh` and the login spelling `-sh` are all it.

The mode itself is `Semantics.RedirectErrorOnSpecialBuiltinFatal` and whatever
else is ever measured to move with it; see `semantics.md`. What is written down
here is only where it *starts*, which is the front end's half: how a program
arrived and what it was called is `driver`'s fact and nothing else's, the same
reasoning that put `$0`, the alias route and login-ness there.

**It is not an axis.** The same binary answers both ways depending on the word
it was exec'd with, so a dialect field keyed on it would record the accident
and lose the rule — which is exactly what #691 found when it moved the answer
out of the panel's `bash-as-sh` column and into a mode. This is the other half
of that: the dialect supplies where the mode starts and the front end overrides
it when the name says `sh`.

### Measured

Panel: bash 5.3.15, bash 3.2.57, dash, ksh93u+ 2012-08-01, zsh 5.9.2, on macOS
25.5 — measured 2026-09-05. Every row was run under a chosen `argv[0]` with a
scratch `HOME`, the snippet `exec 3>/nope/x; echo after`, which is the axis the
mode moves.

| `argv[0]` | bash 5.3 | bash 3.2 | dash | ksh93 | zsh |
| --- | --- | --- | --- | --- | --- |
| its own name | `after`, 0 | `after`, 0 | stops, 2 | stops, 1 | `after`, 0 |
| `sh` | **stops, 1** | **stops, 1** | stops, 2 | stops, 1 | **stops, 1** |
| `/bin/sh`, `./sh` | stops, 1 | — | — | — | stops, 1 |
| `-sh` | stops, 1 | stops, 1 | — | — | stops, 1 |
| `--sh` | `after`, 0 | — | — | — | — |
| `shx`, `SH`, `bsh` | `after`, 0 | — | — | — | see below |

**Two shells move and two have nowhere to move to.** dash and ksh93 keep the
standard's rule under every name, so they answer alike in both rows; the
evidence is bash and zsh, which are the two with a mode at all and which each
change under the same word. `set -o` confirms it directly for bash — `posix on`
called `sh`, `posix off` called `bash` — and `emulate` does for zsh, reporting
`sh` and `zsh` respectively.

**The two that move do not read the name the same way, and the core takes the
narrower reading.** bash matches the basename exactly. zsh takes the *first
letter* of it, after stripping a leading `-` and a leading `r`: `s` and `b`
mean `sh`, `k` means `ksh`, `c` means `csh`, and anything else is zsh — so
`bash`, `shx`, `shell` and even `s` are all sh emulation there while `ash`,
`dash`, `fish`, `xsh` and `mysh` are not, and `rsh` is while `rzsh` is not. The
front end reads `sh` and nothing else: it is the word both shells agree on, it
is the only one anything is really invoked by, and taking zsh's extras would
put a shell called `bash` into POSIX mode.

**One dash and not any number.** `-/bin/sh` is the name and `--sh` is not,
which is what makes the strip a login convention rather than general
punctuation.

### Where in startup it happens

Last, and both halves are measured.

**After the invocation's own options.** `sh +o posix -c 'exec 3>/nope/x; echo
after'` still stops where `bash +o posix -c` the same string carries on. It is
not the option loop being ignored — `+o errexit` on the same invocation is
honored, and so is `-x` — so the name is read after the options and outranks
the one of them it collides with.

**After the startup files.** A `~/.profile` read by `-sh -l` reports `posix
off`, and the script that follows it stops all the same. So the profile runs in
the shell the dialect describes and the mode arrives after it.

**But before the file a non-interactive shell sources out of a named
variable**, because the mode *suppresses* that file — see "The file a shell
reads when it is not going to prompt" below. The two questions compose here
and nowhere else: a shell called `sh` reads no such file, which is the same
absence the standard's own posix option produces, and neither needs a second
answer keyed on the invocation. Pinned by
`env/a-file-named-for-a-non-interactive-shell-is-not-read-when-called-sh`
against the row for the same case under the shell's own name.

### Leaving it again

The mode is a mode and not a written-down answer, which matters most on the way
out. `sh -c 'set +o posix; exec 3>/nope/x; echo after'` prints `after` at 0 —
the shell's *own* answer is back, not the standard's opposite — and entering it
again stops it again.

That is why the front end goes through the runner's own knob rather than
writing the axis. Entering records the answer the dialect held; leaving puts
that back. A front end that set the axis directly would leave nothing to
restore *and* nothing to notice: the mode would read as off, and `set +o posix`
would be granted by doing nothing at all, which is the one failure this shape
prevents.

### Where it lives

`driver.PosixNamed`, beside `LoginShell` — the two facts read off the same
word, asked in the same two places, since the prompt route never reaches where
the script routes read them. `interp.Runner.SetPosixMode` is the knob, exported
for exactly this: whether a shell *has* the name `set -o posix` is a dialect's
answer and only one of the panel declares it, so a front end routing this
through `SetNamedOption` would be refused by the second shell that needs it.
The name and the mode are two questions.

`Case.Argv0` is how the corpus asks it. `Case.Args` is what comes after
`argv[0]` and can never be the word itself, so until it existed the only
record of this was the panel's own `bash-as-sh` column — one shell, and for the
whole corpus rather than for the case that means to ask.

## `+c`: the sign of the option letter

**The rule.** Both signs of the `c` letter select the command string —
unanimous, and pinned by `invoke/plus-c-still-runs-the-command`. One shell
then names the operands differently for the plus spelling: ksh93 leaves
`$0` as the command string and makes every operand a positional
parameter. `Semantics.PlusSignedCommandStringIsDollarZero`.

### Measured

Panel: bash 5.3.15, bash 3.2.57, dash, ksh93u+ 2012-08-01, zsh 5.9, on
macOS 25.5 — measured 2026-09-05, with `HOME` a scratch directory holding
an empty `.zshrc`. Snippet `echo "$0|$#|$*"; true`, invoked
`sh +c SNIPPET name a`:

| shell | result |
| --- | --- |
| bash 5.3 | `name\|1\|a` |
| bash 3.2 | `name\|1\|a` |
| dash | `name\|1\|a` |
| ksh93 | `<the command string>\|2\|name a` |
| zsh | `name\|1\|a` |

The same for the bundle `+ce` and for `+c -- SNIPPET name a`: the sign
belongs to the *word* the letter was written in, and nothing between the
letter and its string changes the answer. With no operand at all the
naming still splits — three of the four keep the shell's own name in `$0`
and ksh93 puts the command string there.

**A bool rather than a three-state answer.** Three of the four read `+c`
as `-c` in every respect, so a common denominator exists, and refusing an
invocation every shell in the panel runs is not an answer any shell could
ship. False is that majority, and it is the standard's reading too: POSIX
has no plus spelling of the option, so reading `+c` as the option it
spells invents nothing.

### Two more things ksh93 does with `+c`, measured and not modeled

Both were found while pinning the naming, and neither is reproduced.

**The operands also reach the program as literal words.** `ksh +c 'echo
A' x y` prints `A x y`, and `ksh +c 'echo $1' foo bar` prints `foo foo
bar` — the parameters are set *and* the operand words arrive at the
program's last command. They are not re-parsed: `ksh +c 'echo A' '; echo
B'` prints `A ; echo B` on one line rather than running two commands, so
this is not the operands being joined to the command string and parsed.
But `ksh +c 'echo A;' x y` prints `A` and then reports `x: not found`,
which the appending story does not explain either. The two observations
do not agree with each other, which is the reason this is recorded rather
than modeled.

**A one-word command string is looked up on PATH and run as a file.**
`ksh +c 'echo'` reports `echo: echo: cannot execute [Exec format error]`
and exits 126 — it found `/bin/echo` and refused the binary — and `ksh +c
'nosuchcmd_xyz'` reports `nosuchcmd_xyz: not found`. `ksh -c 'echo'`
prints an empty line from the builtin, so this is the plus spelling
alone.

The corpus works around the first of these rather than pinning it: the
snippet in `invoke/plus-c-names-itself` ends in `true`, a command that
ignores the arguments it is handed, so the appended words stay out of the
case's output. The `Why` on the case says so.

## `-c` and `-s` together: who names the operands

**The rule.** An invocation carrying both a command string and the
standard-input option runs the command string — unanimously — and the two
routes then disagree about what to call the operands after it. The
command string's rule is that the first operand is `$0` and only the rest
are parameters; the standard-input rule is that no operand is `$0`, so
the shell keeps its own name and every operand is a parameter. Which rule
applies is `Semantics.StdinOptionNamesTheOperands`.

### Measured

Panel: bash 5.3.15, bash 3.2.57, dash, ksh93u+ 2012-08-01, zsh 5.9, on
macOS 25.5 — measured 2026-09-05, with `HOME` a scratch directory holding
an empty `.zshrc`. Snippet `echo "$0|$#|$*"`, invoked
`sh -sc SNIPPET name a`:

| shell | result | rule |
| --- | --- | --- |
| bash 5.3 | `name\|1\|a` | the command string's |
| bash 3.2 | `name\|1\|a` | the command string's |
| dash | `name\|1\|a` | the command string's |
| ksh93 | `<shell>\|2\|name a` | standard input's |
| zsh | `<shell>\|2\|name a` | standard input's |

Identical for `-sc`, `-cs`, `-s -c` and `-c -s`: neither the order of the
two letters nor whether they are bundled changes any answer, so this is a
fact about the shell and not about the spelling.

**With no operand the question does not arise.** `sh -sc SNIPPET` is
`<shell>|0|` in all five: the two rules name the same nothing. So the axis
is asked only where an operand follows the command string, and a dialect
that has not answered still runs the common case.

**Where the program comes from is a different question, and it is
unanimous.** All five run the command string and none of them reads
standard input for a program — pinned by `invoke/c-outranks-standard-input`
and by #522, which fixed `-sc CMD` running `s` as a command.

**Nothing to break the tie.** POSIX gives `-c` and `-s` separate synopses
and says the standard-input route is assumed only when `-c` is absent, so
it never describes an invocation carrying both. `PosixSemantics()`
therefore leaves the axis unanswered, and a shell built on the substrate
without choosing refuses such an invocation with a usage error rather
than picking a side.

## Refusing an option

**The rule.** An option the shell does not have is refused before anything
runs, on every route and in both spellings — `sh -q script.sh`,
`sh -o nosuchoption -c cmd`. What it *exits with* is the dialect's.

### Measured

Panel: bash 5.3.15, bash 3.2.57, bash-as-`sh`, dash, ksh93u+ 2012-08-01, zsh
5.9, on macOS 25.5 — measured 2026-09-05. `-q`, `-j`, `-z` and `-A` are the
four letters every one of the six refuses, which is what makes them the
probe; a letter one shell happens to have — zsh answers to `-Q` — measures
nothing.

| | bash 5.3 | bash 3.2 | bash-as-`sh` | dash | ksh93 | zsh |
| --- | --- | --- | --- | --- | --- | --- |
| `sh -q …` | 2 | 2 | 2 | 2 | 2 | **1** |
| `sh -o nosuchoption …` | 2 | 2 | 2 | 2 | 2 | **1** |
| `set -q` in a script | 2, carries on | 2, carries on | 2, stops | 2, stops | 2, stops | **1**, stops |
| `set -o nosuchoption` | 2, carries on | 2, carries on | 2, stops | 2, stops | 2, stops | **1**, stops |

**The letter and the name are one question.** Every shell reports the same
status for both and ends the script or does not in the same way, on both
routes, so one dialect value answers both: `Diagnostics.SetInvalidOptionStatus`,
zero meaning the 2 that five of the six report, with
`Semantics.BadSetOptionNameFatal` deciding whether the script survives.

The letter asked neither question until #483. It reported the front end's own
2 for everybody, so `zsh -q` exited 2 where zsh exits 1 — while
`zsh -o nosuchoption` already exited 1, because only the long spelling had a
way to carry an answer back — and `set -q` inside a dash script carried on
where dash stops.

**The wording is the dialect's too, and it is a different sentence on each
route.** Measured 2026-09-05, both spellings, both signs.

Inside a script, where the builtin is speaking:

| shell | `set -q` | `set -o zzznosuch` |
| --- | --- | --- |
| bash 5.3 | `<shell>: line 1: set: -q: invalid option` and `set: usage: set [-abefhkmnptuvxBCEHPT] [-o option-name] [--] [-] [arg ...]` | `<shell>: line 1: set: zzznosuch: invalid option name`, no usage |
| dash | `<shell>: 1: set: Illegal option -q` | `<shell>: 1: set: Illegal option -o zzznosuch` |
| ksh93 | `<shell>: set: -q: unknown option` and `Usage: set [-sabefhkmnprtuvxBCGH] [-A name] [-o[option]] [arg ...]` | the same usage line, under `set: zzznosuch: bad option(s)` |
| zsh | `<shell>:set:1: bad option: -q` | `<shell>:set:1: no such option: zzznosuch` |

At an invocation, where nothing has been read:

| shell | `sh -q -c cmd` | `sh -o zzznosuch -c cmd` |
| --- | --- | --- |
| bash 5.3 | `<shell>: -q: invalid option` and the whole `Usage:\t<shell> [GNU long option] …` block | `<shell>: line 0: <shell>: zzznosuch: invalid option name`, no usage |
| dash | `<shell>: 0: Illegal option -q` | `<shell>: 0: Illegal option -o zzznosuch` |
| ksh93 | `ksh: -q: unknown option` and `Usage: ksh [-cilrsDEabefhkmnprtuvxBCGH] [-R file] [-o[option]] [arg ...]` | the same usage line, under `ksh: zzznosuch: bad option(s)` |
| zsh | `<shell>: bad option: -q` — no location, where the run-time form has `set:1:` | `<shell>: no such option: zzznosuch` |

Three things follow, and each is a dialect answer rather than a rule:

- **The sign is a verb in two of the four.** bash and ksh93 echo the `+` of
  `set +q` back; dash and zsh write `-q` whichever way they were asked. So
  `Diagnostics.SetInvalidOptionLetter` takes both the spelling as written and
  the bare letter, and each wording uses the one it means.
- **The usage line follows the letter, and the name only in ksh93.** bash's
  letter is refused the way any of its builtins' bad letters are, usage line
  and all; its bad `-o` name earns a sentence of its own with nothing under
  it. `Diagnostics.SetInvalidOptionNameUsage` is that difference.
- **At an invocation nobody names `set`.** Three of the four say the same
  sentence with the builtin's name gone, after the plain invocation prefix —
  which is where dash's nought comes from, the same `0:` its unopenable-script
  diagnostic writes. The usage block is the *shell's* there rather than
  `set`'s (`Diagnostics.InvocationUsage`), and ksh93 names itself in it by the
  last element of the word it was invoked by where bash spells the whole of
  it. bash is the exception on the long spelling alone: it hands that one to
  the builtin and prints its own name where `set`'s would stand, location
  included — `Diagnostics.InvocationNameRefusalNamesTheShell`.

**A letter the dialect *has* is a different answer.** `set -b` is bash's and
this shell does not implement it; calling it invalid would tell a script
something untrue about bash. Those letters ride
`Diagnostics.UnimplementedOptionLetters["set"]` and are said to be missing,
which is the rule every other builtin's letters already follow. Measured
2026-09-05 by asking each shell for all fifty-two letters in both signs:
bash has `abefhkmnoprtuvxBCEHPT`, dash `abefimnosuvxCEIV`, ksh93
`abefhkmnoprstuvxABCGH`, and zsh has every letter but `b`, `c`, `j`, `q` and
`z`.

**One wrinkle is measured and deliberately not modeled.** bash exits **1**,
with no usage block, when a letter it *has* comes before the bad one in the
same word: `bash -eq -c cmd` is 1 where `bash -qe -c cmd` is 2. dash and
ksh93 answer 2 either way and zsh 1 either way, so bash is alone and only in
one of the two orders. An axis for the position of a letter within a bundle
would be a field asked once.

## Naming a version: `--version`

**The rule.** Three of the four shells in the panel answer an invocation
option that asks for their version, and the answer **ends the invocation** —
nothing after the option is read and nothing before it runs. The spelling, the
text, the stream it goes to and the status the shell exits are all the
dialect's; a dialect naming no spelling refuses the word the way it refuses any
unknown long option.

It matters more than a version string usually does. `--version` is how
installers, editors and agents decide a shell is usable at all, so a shell that
refuses it is read as broken by everything that probes without a fallback —
which is what this shell was until #1713, in every dialect, while every version
*parameter* was already right.

### Measured

2026-09-11, `env -i <shell> --version`, macOS 25.5.

| shell | writes | on | exits |
| --- | --- | --- | --- |
| bash 5.3.15 | `GNU bash, version 5.3.15(1)-release (aarch64-apple-darwin25.4.0)` and five more lines | stdout | 0 |
| bash 3.2.57 | `GNU bash, version 3.2.57(1)-release (arm64-apple-darwin25)` and one more | stdout | 0 |
| zsh 5.9.2 | `zsh 5.9.2 (aarch64-apple-darwin25.4.0)` | stdout | 0 |
| ksh93u+ | `  version         sh (AT&T Research) 93u+ 2012-08-01` | **stderr** | **2** |
| dash | `/bin/dash: 0: Illegal option --` | stderr | 2 |

Three facts, and none follows from the others: whether the shell knows the
option, which stream carries the answer, and what it exits. ksh93 is why all
three are modeled — it names its version *and* exits a failure for having been
asked, because the word reaches its generic option reader rather than a case of
its own.

Three more, measured the same day:

- **It ends the invocation.** `--version -c 'echo hi'` prints the version and
  never runs the command, in all three that have it.
- **It is read wherever it stands**, and repeating it changes nothing —
  except in bash, whose long options must come before any short one:
  `bash -x --version` is `--: invalid option` at status 2 where zsh and ksh93
  answer. That is bash's GNU long-option reader rather than a fact about the
  version option, and it is **not modeled**: this front end reads a long
  option in any position.
- **An operand ends the options first.** `sh -c 'echo hi' --version` runs the
  command and hands the word to the script in all three.

### What this shell writes

The version is the dialect's own — what *this* shell implements, not what a
panel member installed on the same machine reports. Each line carries the tag
that `$BASH_VERSION`, `$ZSH_VERSION` and `$KSH_VERSION` already carry, so a
test that parses the digits passes and anything printing the string sees at
once that it is not upstream.

| dialect | writes | on | exits |
| --- | --- | --- | --- |
| bash | `GNU bash, version 5.3.15(1)-blairham (arm64-darwin)` | stdout | 0 |
| zsh | `zsh 5.9.2-blairham (arm64-darwin)` | stdout | 0 |
| ksh | `  version         sh (blairham) 93u+ 2026-09-02` | stderr | 2 |
| dash | refused, as every unknown long option is | stderr | 2 |
| `sh` | `sh <build version> (arm64-darwin)` | stdout | 0 |

**The first line and no more.** The real bash follows its version with a
copyright notice and a statement of the GPL; reproducing that would be a false
statement about this code's license as well as its authorship. The first line
is the one `--version | head -1` reads anyway.

**The substrate's own line is the binary's, not a dialect's.** `interp` holds
no build's version and may not name a shell, so `cmd/sh` fills the core's in
from the string its build writes — see `coreSemantics` there. The other
dialects answer for themselves, the refusing one included:
`sh -dialect dash --version` is refused.

### Where it lives

`interp.Semantics.VersionOption` — spelling, text, stream, status — read by
`driver`, which answers it in `announceVersion` before any route is chosen.
Nothing in `interp` reads the field, the same way nothing in it reads
`StartupFileOptions`: both are invocation inputs, and the front end is where an
invocation is read.

## Two startup inputs the environment carries

Neither is an axis: one shell in the panel reads both names and the other three
do nothing at all with either, so what is modeled is a per-dialect *name* and
not a disagreement about behavior.

### Measured

Panel: bash 5.3.15, bash 3.2.57, bash-as-`sh`, dash, ksh93u+ 2012-08-01, zsh
5.9.2, on macOS 25.5 — measured 2026-09-05. A scratch `HOME`, `PATH` set to one
nonexistent directory, running a script file so that the shell is not
interactive. bash-as-`sh` is the same 5.3 binary through a link named `sh`.

| | bash 5.3 | bash 3.2 | bash-as-`sh` | dash | ksh93 | zsh |
| --- | --- | --- | --- | --- | --- | --- |
| `BASH_ENV=f` sources f | **yes** | **yes** | no | no | no | no |
| `$0` inside it | the script | `bash` | — | — | — | — |
| `SHELLOPTS=xtrace:nounset` inherited | **applied**: `$-` gains `ux` | **applied** | **applied** | ignored | ignored | ignored |
| `SHELLOPTS` after startup | rewritten, sorted, with the shell's own defaults | same | same, plus `posix` | the string as given | the string as given | the string as given |
| assigning to `SHELLOPTS` | refused, `readonly variable`, 1 | refused, 1 | refused, 127, fatal | ordinary variable, 0 | ordinary variable, 0 | ordinary variable, 0 |

## The file a shell reads when it is not going to prompt

**The rule.** A shell that has such a file names the variable holding its path,
and the front end expands that value and sources it before the program runs —
on a script operand, on `-c` and on a program arriving on standard input alike.
`Semantics.NonInteractiveStartupVariable`, empty for a shell that has none.

It is the exact counterpart of `$ENV`, and the two never overlap: the shell that
has this reads this and not `$ENV` when it is not interactive, and reads neither
at a prompt, where it has a file of its own name instead. That is the whole
reason it is a second function beside `loginProfile` rather than a flag on
`startup` — reaching it through `startup` would bring `$ENV` with it, which is
what `$ENV` exists not to do.

### What it sees, and when

The same three things the login profile sees, and for the same reason — it is
run *by* the shell that is about to run the program:

- `$0`, `$#` and the positional parameters are the invocation's. On `-c` with
  operands, `$0` is the name operand and `$1` onward are the rest.
- The invocation's options are already in effect, so `-x` traces its lines.
- `exit 3` in it exits 3 and the program never runs. A file that is not there
  is not a failure, and neither is an empty or unset value.

### POSIX mode suppresses it

Measured, and it is why this is one field rather than two: the shell that has
the file reads nothing when started with the standard's own posix option, and
nothing when invoked as `sh`. Those are two spellings of one mode, so the
absence in the `sh` column is not a second fact about a second name — it is the
mode, which the front end asks the runner about (`interp.Runner.PosixMode`)
rather than deriving a second time from the invocation. See "Called `sh`" above
and #691.

| non-interactive, with `BASH_ENV` set | reads it? |
| --- | --- |
| bash 5.3 | **yes** |
| bash 3.2 | **yes** |
| bash 5.3 `--posix` | no |
| bash 5.3 as `sh` | no |
| dash, ksh93, zsh | no |

**One version wrinkle, measured and not modeled.** bash 3.2 gives the sourced
file `$0` = the shell's own name where bash 5.3 gives it the script's path. 5.3
is the panel member that counts, and every other answer about the file is the
same in both.

## The option list the environment carries

**The rule.** A shell that has it names the variable
(`interp.Runner.SetShellOptions`), and the variable is then bound to the option
state in both directions: reading it gives the long names of every option that
is on, and what it *held at startup* has already turned those options on.

It is not a variable that happens to be read at startup. Four properties, and a
partial implementation of them would be worse than none — a script reading a
stale copy would be told something untrue about what the shell is doing, which
is the one failure this name has that plain absence does not:

- **Produced, not stored.** `set -x` changes what it says and `set +x` changes
  it back, because the value is computed when it is read. This is the half a
  copy taken at startup gets wrong.
- **Normalized.** Sorted, long names, the shell's own defaults included — so
  what comes back out is never what went in, and the way to read it is
  membership: `case ":$SHELLOPTS:" in *:xtrace:*)`, exactly as `$-` is read.
- **Readonly.** Assignment is refused, in the dialect's ordinary readonly
  wording and with its ordinary fatality; there is no sentence of its own.
- **Seeded from the environment**, which is the part that matters most: an
  inherited `xtrace` changes what every non-interactive shell below it writes
  to standard error.

### Where in startup the seeding happens

**After the argument vector and before the files**, both measured, and the
first of those is the opposite of every other startup input:

- `SHELLOPTS=xtrace sh +x -c '…'` still traces. The environment wins over an
  option written out, so it is read second.
- A `~/.profile` read by a shell launched with `SHELLOPTS=xtrace` is itself
  traced, and so is the non-interactive startup file above. So it is read
  before them.

A prompt reads it too: an inherited `xtrace` traces the lines a person types.

### An unknown name costs only itself

Measured: a name the shell does not have draws a complaint at **line 0** —
nothing has been read — and every good name in the same value is still applied.
The shell carries on at status 0. An empty piece between two colons is a name
of nothing and draws the same complaint.

The wording is the third shape this refusal has, and it is the plainest of
them. All three are the same sentence with a different amount in front of it:

| where the name came from | what is said |
| --- | --- |
| a script's own `set -o zzz` | `<shell>: line 1: set: zzz: invalid option name` |
| an invocation's `-o zzz` | `<shell>: line 0: <shell>: zzz: invalid option name` |
| the environment | `<shell>: line 0: zzz: invalid option name` |

### What a child is handed

The value a command inherits is **recomputed**, not the string this shell was
launched with. Measured: a shell handed `xtrace` that then runs `set +x` hands
its children a list without it. The entry the shell was born with is the only
place a stale value could reach a command, so that is where it is replaced.

The export attribute itself is ordinary: the name reaches a child because it
arrived in the environment, or because a script exported it, and not otherwise.

### Two defaults differ from the shell that has this, on purpose

The names in the list are this shell's own state and not a claim about anyone
else's. `hashall` is off here because nothing is hashed, and `emacs` is on
because the line editor really does read those keys — both already recorded in
`interp/setoptions.go`. Reporting either one the other way round to match a
listing would be the lie this whole design avoids, which is also why every
corpus case here asks about membership of a name it set itself.

### Where it lives

`interp/shellopts.go` — the produced value, the readonly mark and
`ApplyInheritedShellOptions`, which the front end calls. It is a call rather
than something the Runner does for itself because it is a startup action: a
library Runner handed an environment is not entitled to change its embedder's
options on the strength of a name in it.

`Case.Env` is how the corpus asks about either of these. The harness hands every
case the same four entries, and a snippet cannot put anything into the
environment of the shell already running it, so until this existed neither
startup input could be graded at all — only described. A value may name the
scratch script with `ArgScript`, which is what lets a case point the startup
file at its own snippet.

## `return` in a startup file

A startup file **is** a sourced script, so a `return` in one is legal. It is
how a real `~/.bashrc` bails out early, most often as the guard
`[ -z "$PS1" ] && return`.

**Measured** 2026-09-07 through a pseudo-terminal — no `-c`, no `-i` — with
the rc file as the whole probe and `$?` read at the first prompt. The probe is
**unconditional** (`echo BEFORE; return 3; echo AFTER`) on purpose: the
natural guard cannot tell the hypotheses apart, because where `PS1` is set the
guard never fires and the `return` never runs.

Unanimous on what happens: every column prints `BEFORE`, stops reading the
file there, and **diagnoses nothing**.

The split is over the argument, and over nothing else:

| rc file | bash 5.3.15 | bash32 | bash-as-sh | dash | ksh93 | zsh |
|---|---|---|---|---|---|---|
| `return 3` | 0 | 0 | 0 | 3 | 3 | 3 |
| `false; return 3` | 1 | 1 | 1 | 3 | 3 | 3 |
| `false; return` | 1 | 1 | 1 | 1 | 1 | 1 |
| `(exit 5)` | 5 | 5 | 5 | 5 | 5 | 5 |

The last two rows are what make it about the argument. bash does carry a
startup file's status out — `(exit 5)` leaves 5 — and a `return` with no
argument means the last command's status everywhere. The only thing bash
discards is the number written on the `return` itself, and only when the
`return` is at the **top level of the startup file**: an rc running
`f(){ return 3; }; f`, or `. inner.sh` where inner returns 3, leaves 3 in bash
too. `Semantics.StartupFileReturnCarriesItsArgument`.

Recorded as `startup/a-return-in-a-startup-file-stops-it`,
`startup/a-startup-files-return-argument`, `startup/a-startup-files-bare-return`
and `startup/a-startup-file-carries-its-status-out`.

### The neighboring routes, for contrast

| where the `return` is | bash 5.3.15 / bash32 | bash-as-sh | dash / ksh93 / zsh |
|---|---|---|---|
| a startup file | obeyed, silent | obeyed, silent | obeyed, silent |
| a file given to `.` | obeyed, status is the argument | same | same |
| the top level of a script | refused, script carries on, exit 0 | refused, script **stops**, exit 2 | obeyed, script ends with the argument |
| an interactive prompt | refused, session continues | refused, session continues | accepted silently |

The third row is `Semantics.ReturnOutsideAFunctionIsRefused` plus the
special-builtin fatality that POSIX mode adds, and it was already right. The
first row is the one that was wrong: the front end parsed a startup file for
the sake of a diagnostic that names it as a file, and then handed the
statements to the entry point for a *script's own top level* — so there was
nothing to return from, and the refusal that belongs to the third row fired in
the first. `interp.Runner.RunStartupFile` is the entry point that does not.

## The two options no shell has: `--policy` and `--audit`

**The rule.** The shared front end reads two long options that belong to
this implementation rather than to any shell it models: `--policy FILE`
installs a policy as the gate, and `--audit FILE` writes the event stream.
Both take their value attached with `=` or as the next word, both are
refused when they name no path, and both are read on **every** binary
built on `driver` — `sh`, `bash`, `zsh`, `ksh` and `dash`.

Long form only. The substrate driver `sh` additionally keeps its own
single-dash spellings — `-policy`, `-audit`, `-deny`, `-trace-events` —
because its whole flag namespace is already its own; the dialect binaries
get `--policy` and `--audit` and nothing else.

They are installed **before the operands are read**, because a script
operand is opened through the gate: `bash --policy p script.sh` is a
policy that decides whether the shell may read the program it was pointed
at. A policy that will not load ends the invocation with the usage status
rather than leaving a shell that quietly runs unsandboxed.

### Measured

Adding an option no shell has is normally the thing this front end refuses
to do — a `./bash` that accepts a flag bash rejects is not evidence that
the core is a library. What makes these two safe is that every shell in
the panel **rejects them outright**, so the spelling shadows nothing.

Panel: bash 5.3.15, bash 3.2.57, zsh 5.9.2, ksh93u+ 2012-08-01 and dash,
on macOS 25.5 — measured 2026-09-12. Invoked
`<shell> --policy /tmp/p.policy -c 'echo RAN'`, and again as
`<shell> --policy -c 'echo RAN'`:

| shell | what it said | status | ran the command |
| --- | --- | --- | --- |
| bash 5.3.15 | `--policy: invalid option` + usage | 2 | no |
| bash 3.2.57 | `--policy: invalid option` + usage | 2 | no |
| zsh 5.9.2 | `no such option: policy` | 1 | no |
| ksh93u+ | `policy: bad option(s)` + usage | 2 | no |
| dash | `Illegal option --` | 2 | no |

`--audit` is refused identically by all five, in the same words with the
name changed. Neither form — with a value or without — ran the command in
any shell, so there is no invocation of a real shell whose meaning either
spelling could change.

**The single-dash spelling is ruled out by the same instrument.** Real
bash *accepts* `-acp` as the bundle `-a -c -p` and sets `allexport`, so a
one-dash word here would shadow working behavior rather than add a flag.
That is why the dialect binaries take the long form only.

The decision and the rest of its reasoning are in
`docs/design/sandboxing.md` under *The flags live in `driver`, so every
binary has them* (#1826, #1334).
