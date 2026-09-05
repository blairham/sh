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

`driver`'s `sayVerbose`, and the position it carries. The position is a line
number **and a byte offset**: the number is what the parser reports and the
offset is what makes the walk one pass, and neither can be derived from the
other once the text is growing a line at a time. Lines read while the option is
off are walked past rather than skipped, for the same reason — the offset has
to keep up or the line after a mid-script `set -v` cannot be found.
