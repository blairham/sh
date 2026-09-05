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

The route is the same shape: read once out of the operands, handed over
as `interp.Runner.Route`, and read by three unrelated things — the status
one dialect gives a failed expansion under `-c`, the fatality another
gives a readonly reassignment, and the letters above. One field rather
than a bool per route, because a second name for one measurement is the
one that drifts. `-s` as written travels beside it, in
`Runner.StandardInputOption`, for the single invocation where the two
differ: `sh -s -c cmd` runs the command string and still shows `s`.

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

Three things in the table are **not** modeled, and all three predate the axis:

- **Which file.** The front end reads `~/.profile` for every dialect, which
  is dash's, ksh93's and POSIX's name for it, where bash reads
  `~/.bash_profile` and zsh reads `~/.zshenv`, `~/.zprofile` and `~/.zlogin`.
  Modeling it is a per-dialect list of names rather than an axis.
- **The system-wide file.** `/etc/profile` is not read at all, on any route.
- **`-l` and `--login`.** The front end has no such option, so the only way
  to be a login shell here is `argv[0]`. Adding it would also need a decision
  about `--login`, which dash refuses outright.

The interactive file is `$ENV` rather than a name of our own, and that is a
separate decision: dash and ksh93 both read `$ENV` when interactive and read
nothing else, while `~/.bashrc` and `~/.zshrc` are names those shells own.
This binary is not those shells, so it reads the one that is nobody's brand.
`$ENV` is expanded before it is opened, since `$HOME/.shrc` is the usual
spelling.

### Where it lives

`driver`'s `startup`, and `loginProfile` beside it. They are two functions
rather than one with a flag because the *script* routes want the first and
must not have the second: reaching the profile through `startup` would have
brought `$ENV` with it, and a script inheriting the settings someone wrote for
their keyboard is exactly what `$ENV` exists not to do.

The profile is sourced after the invocation's options are applied and after
the runner is built, which is what the two measurements above require, and it
is the same order the prompt route already used (#482).

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

## Two bash startup inputs this shell does not have

Recorded because an absence nobody wrote down is one that gets implemented
twice, or not at all. Neither `SHELLOPTS` nor `BASH_ENV` is read anywhere in
this tree, and until now nothing said whether that was a decision.

### Measured

Same panel and date. A scratch `HOME`, `PATH` set to one nonexistent
directory, running a script file so that the shell is not interactive.

| | bash 5.3 | bash 3.2 | bash-as-`sh` | dash | ksh93 | zsh |
| --- | --- | --- | --- | --- | --- | --- |
| `BASH_ENV=f` sources f | **yes** | **yes** | no | no | no | no |
| `$0` inside it | the script | `bash` | — | — | — | — |
| `SHELLOPTS=xtrace:nounset` in the environment | **applied**: `$-` gains `ux` | **applied** | **applied** | ignored | ignored | ignored |
| `SHELLOPTS` after startup | rewritten, sorted, with the shell's own defaults | same | same, plus `posix` | the string as given | the string as given | the string as given |
| assigning to `SHELLOPTS` | refused, `readonly variable`, 1 | refused, 1 | refused, 127, fatal | ordinary variable, 0 | ordinary variable, 0 | ordinary variable, 0 |

`BASH_ENV` is the non-interactive counterpart of `$ENV`, and the two do not
overlap: bash reads `BASH_ENV` and not `$ENV` when it is not interactive, and
bash-as-`sh` reads neither. dash, ksh93 and zsh read neither, so this is one
shell's feature rather than an axis — which is also why bash 3.2 differing
about `$0` inside the file is a version wrinkle rather than a split.

`SHELLOPTS` is not a variable that happens to be read at startup. It is bound
to `$-` in both directions, readonly, normalized to a sorted list, and
populated with the options the shell has on by default; the environment merely
seeds it. It is also the sharpest security-relevant input a shell takes, since
an inherited `SHELLOPTS=xtrace` changes what every non-interactive bash on the
machine writes to standard error.

### The decision

**Both are out of scope for now, and this is the statement of it.**

`BASH_ENV` would be small — a per-dialect variable *name* whose value is
sourced on the script routes, the shape `loginProfile` already has. It waits
on nothing but a reason to want it.

`SHELLOPTS` is not small. A two-way binding to `$-`, a readonly variable the
dialect installs, the name-to-letter mapping in both directions, and a
decision about the defaults would all have to arrive together, and a partial
one — importing the value without keeping it in step — would be worse than
none, because a script reading `SHELLOPTS` would be told something untrue.

Filed as #596 rather than left here, so that the absence is tracked and not
only stated.
