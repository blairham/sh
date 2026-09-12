# Substitutions

`$(…)`, `` `…` ``, `$((…))`, `${…}` and `<(…)` — specifically **where
they end**, which is what the lexer needs to know. What is *inside* them
is a later document: this one is about delimiting, because a word's
boundaries depend on it and nothing else can be tokenized until they are
right.

Citation: POSIX.1-2024 XCU §2.6.2–§2.6.4. Panel measurements as in
`../oracle.md`.

## A substitution does not end the word

    $(printf a)b            →  one word, "ab"
    x$(printf "a b")y       →  two fields, [xa] and [by]

Unanimous. The first shows that a substitution is *part of* a word rather
than a word of its own. The second shows why that matters: the result is
field-split like any other unquoted expansion, and the literal text on
either side attaches to the first and last resulting fields rather than
becoming separate words.

A lexer that emitted a substitution as its own token would make both
impossible to reconstruct.

## The closing delimiter is not found by counting

This is the rule that decides the implementation:

| probe | all four |
| --- | --- |
| `echo "[$(echo ")" )]"` | `[)]` |
| `echo "[$(echo "a)b")]"` | `[a)b]` |

A `)` inside quotes **does not close the substitution**. Scanning for a
balanced paren by counting is therefore wrong, and wrong in a way that
truncates the substitution and silently changes the program.

Finding the end requires tracking quoting while scanning — the same
quoting rules as `tokenization.md`, applied recursively. In practice the
scanner is the lexer again, run to a closing token.

Nesting works for the same reason and needs no separate rule:

    $(echo "$(echo deep)")  →  deep

## `$((` is arithmetic, and a nested subshell needs a space

    $((1+2))        →  3       arithmetic
    $( (echo sub) ) →  sub     a subshell inside a substitution

Unanimous. `$((` is taken as the start of arithmetic, so a command
substitution whose first construct is a subshell must be written with a
space. That is the only disambiguation available and it belongs to the
lexer, since by the time the parser sees tokens the choice has been made.

## Backticks

    `echo hi`               →  hi
    `echo \`echo deep\``    →  deep

The older form. It nests only with backslash escaping, which is why the
`$( )` form exists and why this one is worth supporting but never
recommending. Its delimiter is a backtick that is not backslash-escaped.

### One shell says so, and only when it is not going to run

ksh93 remarks on every backquote substitution it reads:

    $ ksh -n bq.sh
    bq.sh: warning: line 1: `...` obsolete, use $(...)

Measured 2026-09-12 on 93u+ 2012-08-01, from a script file, from `-c`
and from standard input. dash, bash 5.3, bash-as-`sh`, bash 3.2 and zsh
read a backquote without a word, so this is one column's and the
wording is a `Diagnostics` field, `BackquoteObsolete`, empty in the
rest.

**It is said only while the shell is not going to execute**, which is
the half that decides where the rule lives:

| route | ksh93 |
| --- | --- |
| `ksh -n bq.sh` | one line per backquote |
| `ksh bq.sh` | nothing at all |
| ``ksh -n -c 'x=`echo hi`'`` | one line |
| ``ksh -c 'x=`echo hi`'`` | nothing |
| a backquote typed at a prompt | nothing |

So the parse produces the remark either way and the front end decides
whether to say it — `interp.RemarkOnlyWhenNotRunning` answers that per
remark kind, because the other remark this substrate has, a
here-document that ran to the end of the input, is said either way.

It accompanies a refusal as well, warning first:

    $ ksh -n bqu.sh
    bqu.sh: warning: line 1: `...` obsolete, use $(...)
    bqu.sh: syntax error at line 1: ``' unmatched

which is the same shape the here-document remark has, and the reason
remarks are read whether or not the parse also failed.

The line is inside the sentence rather than in the location in front of
it — `<script>: warning: line 1: …` and not `<script>: line 1: warning:
…` — which is the split this shell's syntax errors already make and
which `Diagnostics.RemarkNamesItsOwnLine` records.

Corpus: `core/a-backquote-substitution-under-a-syntax-check`,
`core/a-backquote-substitution-when-it-runs`.

## `${…}`

`${x:-y}` and its relatives lex as one word, and the delimiting rule is
the same as for `$( )` — track quoting, stop at the matching close. That
is what makes a `}` inside a quoted section ordinary rather than the end
of the expansion:

    ${x:-"a}b"}   →  a}b   unanimous
                          (`subst/brace-in-quotes-does-not-close`)

Counting braces without tracking quotes would stop at the first `}` and
leave `b"}` behind as text. The **operators inside** (`:-`, `#`, `%`,
`/`, `:offset:length`, `[index]`) are a separate specification this
document does not attempt. The lexer only has to find the end.

### Tracking the quoting is not enough on its own

A quoted run inside the body can hold a substitution, and what that
substitution holds is a **program**, not more of the run's text. Its
quoting is the program's own, so a quote character inside it is not a
quote character of the run:

| probe | all six |
| --- | --- |
| `${x:-"$( echo 'a"b' )"}` | `a"b` |
| `${x:-"$( echo "c'd" )"}` | `c'd` |
| `${x:-"$(( 1 + $( echo 1 ) ))"}` | `2` |
| ``${x:-"`echo 'e"f'`"}`` | `e"f` |

    (core/a-substitution-inside-an-expansion-brings-its-own-quoting)
    (core/what-a-nested-substitution-holds-is-not-a-delimiter)
    (core/the-older-substitution-spelling-brings-its-own-quoting-too)

"Its own quoting" is the short way of saying "it is a program", and the
parentheses go with it. A `case` arm's `)` closes nothing here either,
one level further in than the rule above:

    ${x:-"$( echo `case a in a) echo y;; esac` )"}   →  y   all six

    (core/a-case-arm-inside-backquotes-inside-an-expansion)

The scan for the closing `}` therefore has to step over such a
substitution whole rather than read across it. A scan that does not takes
the `"` in `'"'` as the run's closer, continues from the `'` after it as
though a single-quoted string began there, and swallows the rest of the
input.

The failure is not always a refusal, which is what makes it worth a rule
of its own. Two of the shape in one line put the stray quotes back in
balance:

    printf '[%s]' "${x:-"$( echo 'a"b' )"}" "${y:-"$( echo "c'd" )"}"
      →  [a"b][c'd]     all six
      →  [a"b} c'd]     a scan that reads across, exit status 0

One field where there are two, the grammar's own `}` in the output, and
no diagnostic anywhere.

    (core/a-swallowed-quote-changes-the-program-rather-than-refusing-it)

**A nested `${ }` is not one of these.** Its body is a word rather than a
program, and the panel divides on it — `${x:-"${y:-'"'}"}` is accepted by
bash alone and refused by the other five — so it is a dialect question
and not part of this rule.

### Every scanner that has to find a closing bracket reads the same way

The rule is not the expansion's; it is the *substitution's*, so it holds
wherever one can be written. `(( … ))` is the site that had it wrong
longest, and the C-style `for` header — which is that token cut on its
own semicolons — inherited the mistake:

| probe | bash 5.3 | ksh93 | zsh |
| --- | --- | --- | --- |
| `(( $(case q in q) echo 7;; esac) ))` | runs | runs | runs |
| `for (( $(case q in q) echo 7;; esac) ;; ))` | runs | runs | runs |
| `for (( ${ case q in q) true;; esac; };; ))` | runs | runs | `bad substitution` |

Measured 2026-09-12; zsh's last row is that shell not having the brace
form of a substitution at all, which is a different fact and is recorded
in `parameter-expansion.md`. Counting parentheses stopped the arm at its
pattern and left the rest of the arithmetic looking like a stray `)`, or — once
the header was cut before it was read — like a header holding three
separators. `$((` is the one shape left to the counting, deliberately: it
holds an expression rather than a program, so its two opening parentheses
balance against its two closers on their own.

## `${ cmd;}` — a substitution that runs in the current shell

A space after the brace turns the same delimiters into a **command**
substitution whose command runs in the shell itself rather than in a
subshell, so an assignment inside it survives:

    echo "[${ echo hi;}]"     →  [hi]   in bash 5.3 and ksh93
                                        bad substitution in bash 3.2,
                                        dash and zsh

Measured 2026-09-05 across the panel. bash 3.2 is on the refusing side,
which dates the construct rather than disputing it — but the two shells
that have it are bash 5.3 and ksh93, so it is a two-shell construct and
not core.

**The space is the entire grammar.** `${x}` is a parameter and `${ x}` is
a command, and the decision is made on the one character after the brace
rather than by trying to read a name and failing — which is possible
because a parameter name may not begin with a blank. Measured, the
characters that make it a command are exactly space, tab and newline.

The body is delimited exactly as the parameter form is: a `}` inside
quotes does not close either, and both nest. Grammar flag:
`CurrentShellSubstitution`, on for bash and ksh, off elsewhere including
the core.

## Process substitution: `<(cmd)` and `>(cmd)`

A command run with one end of a pipe, expanding to a **path** the other
end can be opened by. It belongs here because it is delimited exactly
like the forms above — matching parentheses, quoting tracked — and
because the only thing the lexer has to decide is where it ends.

Core: bash, ksh93 and zsh have it and dash does not, calling the `(`
unexpected. Grammar flag: `ProcessSubstitution` (on in the core, off for
`posix` and `dash`). Measured: `procsub/reads-a-command-as-a-file`,
`procsub/two-of-them-in-one-command`, `procsub/feeds-a-loop`,
`procsub/a-redirection-where-a-target-belongs`.

**The `(` must be adjacent to the `<` or `>`.** With a space the text is
a redirection followed by a subshell, and every shell in the panel
refuses it — including the three that have the construct:

    cat < (echo hi)   →  syntax error in dash, bash 5.3, bash 3.2,
                         ksh93 and zsh alike

So adjacency is not a convention this implementation adopted; it is the
grammar, and it is why the construct is the lexer's rather than the
parser's.

**It is unquoted only, and nothing is lost by that.** `"<(echo hi)"` is
its own ten characters of text in every shell in the panel, dash
included, and the reason is that what the construct produces is a
*path*: a quoted path is still a path, so there is nothing for the
quoting to change. The lexer therefore reads the form outside quotes and
nowhere else, and that is a completeness statement rather than an
omission. Pinned by `procsub/quoted-is-not-a-substitution`; the claim
said "the five characters" until the case was written, which is a count
of nothing in the example beside it and the sort of arithmetic a
sentence nobody can run keeps.

**What the path looks like is the shell's business, not the grammar's.**
Measured with `echo <(true)`: bash answers `/dev/fd/63`, ksh93
`/dev/fd/3`, zsh `/dev/fd/11`. This implementation answers with a named
pipe in a temporary directory of its own instead, because `/dev/fd`
requires the descriptor to survive `exec` and clearing Go's
close-on-exec flag leaks it into every later command. All the spec
requires is a path the child can open; the numbers above are recorded so
a reader does not mistake one shell's for the specification.

**In bash and zsh the substitution is part of the word; in ksh93 it is a
word of its own.** This one has no axis behind it yet:

| probe | bash 5.3 | ksh93 | zsh |
| --- | --- | --- | --- |
| `set -- x<(true)y; echo $#` | 1 | **3** | 1 |
| the fields | `x/dev/fd/63y` | `x`, `/dev/fd/4`, `y` | `x/dev/fd/11y` |

Ours follows bash and zsh, which is the two-of-three answer and the one
the corpus's cases exercise.

The **read** side works wherever the construct does, as an argument and
as a redirection target: `cat <(echo hi)` and `cat < <(echo hi)` both
answer `hi` in bash 5.3, bash 3.2, ksh93 and zsh. The **write** side
splits by position in the panel's ksh93 (AJM 93u+ 2012):
`echo hi | tee >(cat > f)` works there as an ordinary argument, while
`echo hi > >(cat)` — the same substitution as a redirection *target* —
fails with `cannot create` naming an unprintable path. bash and zsh take
both.

**It is the sharpest evidence that a dialect is a runtime switch rather
than a build-time identity**, and the evidence is a single binary:
bash 3.2 has process substitution as `bash` and refuses it as `sh`,
which is `argv[0]` alone changing the language. Re-measured 2026-09-05
with the panel's `bash-as-sh` route: bash 3.2 as `sh` reports a syntax
error at the `(`, and bash 5.3 as `sh` keeps the construct — so the
version and the invocation are two variables and a claim naming only one
of them is incomplete.

### How long the pipe has to last

A real shell forks for `<(cmd)` and hands the child a descriptor, so the
writer exists from the moment the word is expanded. Here it is a named pipe
whose writer *polls* for a reader — see `interp/procsubst.go` — and the poll's
give-up condition is the pipe's name going away at the end of the command that
produced it. That pairing holds for every open that **waits**: a blocking open
parks in the kernel until the writer arrives, so the two rendezvous while the
command is still running.

It does not hold for an open that does not wait. `sysopen -r -o nonblock -u fd
<(cmd)` returns at once and the shell keeps the descriptor for a later command,
so the command that named the path finishes with the writer still polling —
and the unlink was then read as "nobody opened it", losing the substituted
command outright. Measured 2026-09-11 against zsh 5.9.2: two runs in ten
answered end of file where zsh answered the command's output every time.

So a substitution's pipe **keeps its name while one of this shell's own
descriptors is open on it**, and goes with the shell's directory afterwards.
Nothing else about the lifetime moves: a pipe nobody opened is still removed
with the command that named it, which is what keeps a long session from
filling its directory and what ends the writer's wait (#1750).

The residual difference is the poll itself, and it is worth writing down
rather than leaving to be rediscovered: a reader that reads the instant it has
opened can see a pipe with no writer in it, which is end of file. A script
that sleeps between the open and the read — which is what the workload this
came from does — cannot see it.

**And one read is not the output.** A single `read(2)` on a pipe returns what
has arrived, so a substitution whose body writes twice answers one read with
however much of it had been written by then — `[one]` where the whole of it is
`onetwo`, 4 runs in 15 on this machine. That is true of the shell being copied
as well and is not a divergence; what differs is the odds, because there the
writer is a process already running and here it is a goroutine the poll has
just let go. A reader that wants the output reads **to end of input**, and a
test that reads once is measuring the scheduler (#1907).

Two consequences of keeping the name follow, and the second was a defect for a
day. The end-of-file nudge — the repeated last-writer close in
`nudgeFifoEOF`, which exists because a reader can come out of `open` into a
pipe whose end-of-file has already gone past — used to be ended by the unlink:
ENOENT said there was no pipe to tell through. With the name kept and a reader
that stays for the session, neither of its two answers could ever arrive, and
it went round every twenty milliseconds for the life of the shell, per
substitution. It has a deadline now: a hundred milliseconds of repeating a
transition whose race is between two system calls, after which a reader that
is still there is one holding the pipe for its own reasons.

### When a writing body's output lands

`>(cmd)` is the direction whose body has nobody waiting for it. `<(cmd)`
writes into the pipe, so the command that named the path reads it to an
end; `=(cmd)` has run to completion before the path exists at all. A
writing body writes into the **shell's own output**, which nothing
downstream ends and nothing in the shell reads.

**No shell in the panel loses it.** Measured 2026-09-12 on Linux with

    printf "PIPE\n" | tee >(read -r v; printf "[%s]" "$v") >/dev/null

bash 5.2, zsh 5.9 and ksh93 all answer `[PIPE]` — 200 runs each, idle
and again with the machine oversubscribed two to one, 1200 answers and
no empty one. In a forking shell that costs nothing: the body is a
process holding the shell's standard output, so whatever is reading that
stream reads until the body has closed it too.

**When the shell stops is a disagreement.** With a body that outlives
its input, zsh waits for it and bash and ksh93 do not:

| probe | bash 5.2 | ksh93 | zsh 5.9 |
| --- | --- | --- | --- |
| `printf x \| tee >(sleep 3) >/dev/null` | 0s | 0s | **3s** |
| `echo >(sleep 3)` | 0s | 0s | **3s** |
| `echo hi > >(sleep 3)` | 0s | 0s | **3s** |
| `exec > >(cat); echo hi` | 0s | 0s | 0s |
| `…; printf AFTER` after the first row | `AFTER[PIPE]` | `AFTER[PIPE]` | **`[PIPE]AFTER`** |

**This implementation waits, in every dialect**, which is zsh's answer
and is not a preference: here the body is a goroutine and the output is
the caller's `io.Writer`, so a body cannot outlive the shell the way
bash's and ksh93's process does. The choice is between zsh's timing and
losing the bytes, and the bytes are the part the whole panel agrees on.
Before the wait this shell answered the empty string in 49 runs in 300
of the binary under that load, and in 4 of 60 of the test covering it
(#2183). The ordering the wait costs — `[PIPE]AFTER` where bash and
ksh93 say `AFTER[PIPE]` — is a real disagreement and is filed as an axis
to add rather than left unrecorded.

**Except where the script is itself still holding the pipe.** `exec >
>(cat)` hands the shell's output to a body that reads until that end
closes, and the end is now the shell's: waiting there is waiting for a
descriptor only this shell can close. zsh does not wait for that shape
either — the one 0s row above among its 3s ones — so the exception is
the construct's rather than this implementation's. It is the same clause
that keeps the pipe's *name* while one of this shell's own descriptors
is open on it, above.

### Which standard input the body reads — an axis

A substitution's body is a shell of its own and has to read *something*.
Two candidates: the standard input of the command whose word the
substitution stands in, or the standard input of the **shell**. They are
the same stream almost everywhere, which is why the obvious probe cannot
see the question — `cat <(cat)` reads the shell's input in every shell
that has the construct, because the command's input *is* the shell's.

They part inside a **pipeline element**, whose input is the pipe.
Measured 2026-09-11 and re-measured across the panel, with the shell's
own standard input a file holding `OUTER`:

| probe | bash 5.3 | bash 3.2 | bash as `sh` | ksh93 | zsh 5.9.2 |
| --- | --- | --- | --- | --- | --- |
| `printf "PIPE\n" \| cat <(cat)` | `PIPE` | `PIPE` | `PIPE` | `PIPE` | **`OUTER`** |
| `printf "PIPE\n" \| cat =(cat)` | — | — | — | — | **`OUTER`** |
| `cat <(cat)` | `OUTER` | `OUTER` | `OUTER` | `OUTER` | `OUTER` |

dash has no such construct in any row. So the panel **splits**: this is
an axis and not a correction. `ProcessSubstitutionBodyReadsTheShellsInput`
is it — zsh `Yes`, bash, bash-as-`sh`, bash 3.2 and ksh93 `No`, and the
POSIX base `No` because the standard has no construct to answer for.

The third row is the control, and it is why the divergence was silent:
without a pipeline the two readings name one stream and nothing
distinguishes them. What goes wrong under the wrong answer is quiet too —
nothing errors, the body simply eats the pipe the outer command was
going to read.

**The reading behind zsh's answer bounds the axis.** A pipeline element's
pipe is one of that element's *redirections*, and a redirection is
applied after the element's words have been expanded — so a substitution
performed while expanding them is still looking at the shell's own input.
Everything that happens *after* that point sees the pipe, and zsh agrees
with the rest of the panel at every one of them:

| probe | every shell with the construct |
| --- | --- |
| `printf "PIPE\n" \| { cat <(cat); }` | `PIPE` |
| `printf "PIPE\n" \| ( cat <(cat) )` | `PIPE` |
| `f() { cat <(cat); }; printf "PIPE\n" \| f` | `PIPE` |
| `printf "PIPE\n" \| eval 'cat <(cat)'` | `PIPE` |
| `printf "PIPE\n" \| cat < <(cat)` | `PIPE` |

A compound command's body, a function's body and an `eval`'s program all
run once the element's redirections are in place; a substitution written
as a redirection **operand** is expanded with them rather than before
them. So the answer reaches one simple command's words and stops there —
a fix written as "a pipeline element's substitutions read the shell's
input" gets the last row wrong, in the accepting direction, silently.

Two more shapes, both measured and both the same split as the first row:
a substitution **nested** in another (`printf "PIPE\n" | cat <(cat
<(cat))`) answers `OUTER` in zsh, because the outer body is a shell whose
own input is whatever the axis handed it and the inner body inherits
that; and a **backgrounded** pipeline (`printf "PIPE\n" | cat <(cat) &
wait`) answers `OUTER` too, so what the body reads is the shell's real
input rather than the empty one an asynchronous job is often given.

**`>(cmd)` cannot observe it.** That spelling hands its body the reading
end of its own pipe, which replaces whatever the body would otherwise
have read, so all five answer alike. The axis is still asked in the one
place all three spellings are prepared — `substRunner` in
`interp/procsubst.go` — which is what keeps `=(cmd)` from drifting away
from `<(cmd)`. Deciding it per spelling is exactly the shape #1933 was
filed to prevent, since the file form arrived after the pipe forms and
would have been the second copy.

The corpus writes each body with the shell's own `read` rather than an
external `cat`. The two are measured to answer identically in every
column; what the external spelling adds is a process started with the
element's descriptor, which is #2144 and is older than this axis.

Measured: `procsub/a-body-in-a-pipeline-reads-the-shells-input`,
`procsub/a-file-substitutions-body-in-a-pipeline`,
`procsub/a-body-outside-a-pipeline-reads-the-same-input`,
`procsub/a-body-in-a-grouped-pipeline-element`,
`procsub/a-body-in-a-function-called-from-a-pipeline`,
`procsub/a-body-in-a-redirection-operand-of-a-pipeline-element`,
`procsub/a-nested-body-in-a-pipeline`,
`procsub/a-body-in-a-backgrounded-pipeline`,
`procsub/a-writing-body-in-a-pipeline-reads-its-own-pipe`.

## Process substitution to a file: `=(cmd)`

The same construct with a **regular file** where the two spellings above
have a pipe. The command runs to completion, its output lands in the
file, and the word becomes that file's path.

Grammar flag: `ProcessSubstitutionToFile`, off in the core and on for
`zsh` alone. Measured 2026-09-11 on zsh 5.9.2 against the whole panel —
dash, bash 5.3, that binary as `sh`, bash 3.2 and ksh93 all read the `=`
as an ordinary character and report the `(` — so this is additive
grammar for one dialect rather than a disagreement, and there is no
semantics axis behind it. Measured:
`procsub/a-file-rather-than-a-pipe`,
`procsub/a-file-substitution-outruns-a-pipe-buffer`,
`procsub/a-file-substitution-discards-the-bodys-status`,
`procsub/a-file-substitution-lives-as-long-as-its-command`,
`procsub/a-file-substitution-opens-only-at-a-word`,
`procsub/a-file-substitution-in-a-condition`,
`procsub/a-quoted-file-substitution-is-text`.

**A file is what the construct is for.** `diff =(sort a) =(sort b)`
seeks in both operands and an editor opens one; neither is something a
pipe can do. It follows that the body cannot be left running beside the
reader: there is nobody to fill the buffer for and nothing to deadlock
against, so it runs to completion first. `wc -c < =(head -c 200000
/dev/zero)` answers `200000`, which is well past any pipe buffer and is
the probe that separates the two readings — a shell that handed over a
pipe here would stop rather than answer wrong.

**Where it opens is most of the grammar, and it is narrow.** Unlike
`<(` and `>(`, which open a substitution anywhere in an unquoted word,
`=(` opens one in exactly two positions:

| probe | zsh 5.9.2 |
| --- | --- |
| `echo =(echo hi)` | a path |
| `echo =(echo hi)x` | that path with `x` behind it |
| `a=(=(echo hi))` | a path, the front of an array element being the front of a word |
| `a==(echo hi)` | a path, the front of an assignment's **value** |
| `a[1]==(echo hi)`, `a+==(echo hi)`, `typeset a==(echo hi)` | a path |
| `echo x=(echo hi)` | `missing end of string` |
| `echo a=b=(echo hi)` | `missing end of string` |
| `echo a==(echo hi)` | `missing end of string` |
| `echo =(echo hi)=(echo ho)` | `missing end of string` |
| `cat > a==(echo hi)` | `missing end of string` |
| `case a=(b) in a*)` | matches: the subject is the literal `a=(b)` |
| `echo \=(echo hi)`, `"=(echo hi)"`, `'=(echo hi)'` | the text as written |

The last four rows are what a rule of "an `=` in front of a `(`" gets
wrong, and the reason the position has to be asked about rather than the
characters: an `=` is an ordinary character everywhere else in a word,
and a `(` behind one already means an array literal or a pattern group.
Taking every `=(` for a substitution is the failure #1288 recorded,
where a pattern with an `=` in the middle of it stopped a real prompt
theme from parsing.

The two accepting positions are the front of a **word** and the front of
an assignment's **value**, and the second is only where an assignment may
be written at all: the same word is refused as an argument, as a
redirection's target and as a `case` subject.

**Its lifetime is the command that named it**, exactly as a pipe's is:
`f==(echo hi); cat $f` is `No such file or directory`. So the file is
removed by the same step that removes the pipes, at the end of the
command, and the path a script captured out of one leads nowhere
afterwards.

**The body's status is discarded.** `cat =(false)` and `cat =(exit 7)`
are both 0, and `echo =(nosuchcmd)` prints the body's diagnostic, prints
a path, and still exits 0. A word expands to a path or it fails to
expand; a command that ran and failed still wrote the file it was given.
`=()` and `=(:)` each give a valid file of zero bytes.

**What the file looks like** is the shell's business the way the pipe's
path is. zsh writes it under `$TMPPREFIX` — default `/tmp/zsh` — and
ignores `TMPDIR`, and the mode is `0600`, which is also why naming one
as a command is `permission denied` at status 126 rather than running
it. This implementation keeps the mode and puts the file in the same
per-shell directory as its pipes, for the reason recorded under the
pipes: a path chosen by the interpreter belongs to the Runner rather
than to the process, and there is no panel behavior to match because no
other shell has the construct.

**In a condition it is refused, with the same sentence as the other two
and a different status.** `[[ x == =(x) ]]` is
`process substitution =(x) cannot be used here` at status **1**, where
`[[ x == <(x) ]]` is the identical sentence at status **2**. Both
abandon the rest of the input. The status is the only thing that tells
them apart, and it is a fact about the spelling rather than about the
shell — no second shell has the file form to disagree about it.

## `$(<file)` — the substitution that reads a file

A command substitution whose **whole body is one input redirection and
nothing else** is a special form: the file is opened, its bytes become
the substitution's result, and no command runs.

    printf 'hello\n' > f
    printf "[%s]" "$(<f)"    →  [hello]  in zsh 5.9.2, bash 5.3,
                                         bash 3.2, bash as `sh`,
                                         bash --posix and ksh93
                             →  []       in dash

Measured 2026-09-10 across the whole panel. Grammar flag:
`ReadFileSubstitution`, on in `Core()` and therefore in bash, ksh and zsh;
off for POSIX and dash.

**It is additive rather than a disagreement**, and that is the reason it
is a grammar flag rather than a semantics axis. dash reads the same text
as an ordinary redirection with no command name: the file is opened, so a
name that will not open is still reported, and nothing runs, so nothing is
written and the substitution is empty. The shell without the form does not
mean something *else* by the text — it has no form to mean anything by.

### It is not the null-command hook

zsh runs a command consisting only of redirections through `NULLCMD` and
`READNULLCMD`, so `<f` at a prompt pages the file. It would be easy to
conclude that `$(<f)` is that mechanism seen through a substitution, and
it is not. Measured 2026-09-10 on zsh 5.9.2, with `READNULLCMD` set to a
function that prints `CHANGED`:

    <f                  →  CHANGED     the hook
    $(<f)               →  hello       the form
    $(:; <f)            →  CHANGED     the hook again
    $(<f; :)            →  CHANGED
    cat < a*b           →  the file    a target is matched as a pattern
    $(<a*b)             →  no such file or directory: a*b

The last pair is the same conclusion from the other side: zsh matches an
ordinary redirection target as a pattern and does not match this one, so
the two operands travel different paths. A hook the form does not consult,
and an expansion the form does not do, is a different mechanism.

The distinction is load-bearing rather than trivia. Implementing the form
as "a command with no name copies its input to its output" would make `<f`
print the file in bash and ksh93, where measurably it prints nothing, and
would make `$(<f; :)` print it everywhere, where measurably only zsh does.

### What is the form and what is not

Unanimous among the five columns that have it, measured 2026-09-10:

| body | result | why |
| --- | --- | --- |
| `$(<f)` | the file | the form |
| `$(< f)` | the file | a space before the operand changes nothing |
| `` `<f` `` | the file | the older spelling of the same substitution |
| `$(0<f)` | the file | standard input written out is still standard input |
| `$(<$name)`, `$(<~/x)` | the file | the operand expands as a redirection target does |
| `$(<f echo hi)` | `hi` | a command word takes the file as its input |
| `$(x=1 <f)` | empty | an assignment prefix leaves an ordinary redirection |
| `$(<f <g)` | empty | one redirection, not the first of several |
| `$(3<f)` | empty | a descriptor other than standard input (bash 3.2 dissents) |
| `$(<<<hi)` | empty | a here-string body is not a filename |
| `$(<f; :)`, `$(:; <f)` | empty | not the whole body — zsh's hook answers these |

The result is trimmed of trailing newlines exactly as any other command
substitution's is, and unquoted it is field-split the same way. A name
that will not open is reported in the dialect's own words, the
substitution expands to nothing, and its status is the one that dialect
gives any redirection that would not open — 1 in zsh, bash and ksh93, 2 in
dash. Under `set -e` that status ends the script.

### What this entry does not settle

Three corners where the panel does not agree, each left for a measurement
of its own rather than folded into this form:

- **A directory operand.** zsh 5.9.2 says `error when reading …: is a
  directory` and leaves status 1; bash 5.3, ksh93 and dash say nothing at
  status 0, and bash 3.2 says nothing at status 1. Three answers, so an
  axis rather than a rule.
- **A pattern in the operand.** bash matches it — `$(<a*b)` reads `aXXb` —
  where zsh, ksh93 and dash report the pattern as the name. That is the
  ordinary redirection-target question asked in this position.
- **zsh's `NULLCMD` / `READNULLCMD`.** The hook itself, which is a
  mechanism of its own and is now implemented as one — see
  "A command that is only redirections" in `docs/spec/semantics.md`. The
  boundary this section draws is what it is built against: the form
  intercepts the body before any command runs, so a substitution never
  reaches the hook and pointing `READNULLCMD` somewhere else still reads
  the file.

## What this does not cover

The internal grammar of each form: arithmetic operators and their
precedence, the parameter-expansion operator set, and what a
`${x/pat/rep}` pattern means. Those are needed by expansion, not by
tokenization, and each is its own document. Recorded here so their
absence is a known gap rather than an oversight.
