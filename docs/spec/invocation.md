# Invocation

What the front end decides before anything runs: which route the shell
was invoked by, which operand becomes `$0`, and whether there is a person
at the other end. `driver/` is the whole of it — nothing outside that
directory implements how a shell is invoked.

This file covers the last of those. The rest is in `AGENTS.md` under
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

### What is not implemented, and why

**`c` and `s` split the panel and are left unmodeled.** Reading down the
table:

- `c` for a command string: bash and ksh93 show it, dash and zsh do not.
  A clean two-two split with no majority to follow.
- `s` for the standard-input route: all four show it, whether `-s` was
  written or not, and including at a prompt — that half is unanimous.
  But ksh93 alone also shows `s` under `-c`, where the other three show
  nothing, so the letter cannot be modeled as "the input came from
  standard input" without taking a side on `-c`.

Neither is implemented and neither has an axis: `i` was the question
issue #472 asked, and inventing an axis for a letter nothing has needed
yet would be answering a question nobody put. The corpus reads `$-` as
membership (`case $- in *i*)`) precisely so that a case does not depend
on the spelling.

**The letters a shell adds only when interactive are a second vector,
also unmodeled.** bash adds `H`, zsh adds `Z`, and ksh93 trades `h` for
`m` and `E`; dash adds nothing. That is per-dialect startup state rather
than one axis, and it is a different question from whether the shell is
interactive at all.

**ksh93 is the only shell that turns the monitor on for `-i
script.sh`** (`m` in its row, absent from the other three). So being
interactive and having job control are not the same fact, which is why
`Interactive` and `JobControl` are separate fields rather than one.

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

`interp` asks a related question for `select` and `read -p` and answers
it differently — a character device, excepting the null device — because
`interp` cannot import `repl` and the approximation is documented in
`semantics.md` as a knowing difference. The two are not the same
question: the front end asks about the *invocation*, and a builtin asks
about whatever stream `-u` resolved to.
