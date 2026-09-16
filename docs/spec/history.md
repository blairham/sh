# The history, and what does not go in it

What an interactive shell writes down about the session, where it puts
it, and what it declines to put there.

This file covers two different kinds of thing and says which is which
throughout, because mixing them would be dishonest:

- **Measured panel behavior**, which this implementation is compatible
  with or knowingly diverges from, in the usual way.
- **A feature of this shell that no shell in the panel has.** Write-time
  credential scrubbing is ours. It is not a compatibility question, no
  amount of oracle running will produce it, and it must not be dressed
  up as measured behavior.

## What the panel keeps, and how it is told not to

Measured by hand on 2026-09-05, macOS 26 on arm64: bash 5.3.15,
bash 3.2.57 (`/bin/bash`), zsh 5.9.2, ksh93u+ 2012-08-01 (`/bin/ksh`)
and dash. By hand rather than through `internal/cmd/oracle`, because every
observation here needs an *interactive* shell with a `HISTFILE` of its
own and the corpus harness runs snippets non-interactively — the same
reason there are no corpus rows for it.

Each probe ran the shell with `-i` on a pipe, in a throwaway `HOME`
with `HISTFILE` pointing into a temporary directory, and then read the
file.

| shell | history file | leading space | a pattern | an ignored line, in the session |
| --- | --- | --- | --- | --- |
| bash 5.3 | text, a line per entry | `HISTCONTROL=ignorespace` | `HISTIGNORE` globs | **gone** — `history` cannot see it |
| bash 3.2 | text, a line per entry | `HISTCONTROL=ignorespace` | `HISTIGNORE` globs | gone |
| zsh 5.9 | text, a line per entry | `setopt HIST_IGNORE_SPACE` | `HISTORY_IGNORE` glob | **it depends on the knob** — see the correction below |
| ksh93 | its own binary format | none observed | none observed | — |
| dash | none at all | — | — | — |

**Correction, 2026-09-06.** The zsh row's last column read "**kept** —
`fc -l` still lists it", and that is the reading of `HISTORY_IGNORE`
recorded against `setopt HIST_IGNORE_SPACE`. Two knobs were measured in
one session and the answer of one was written down under the other; #571
repeated it, and it stood until building the option-spelled knobs made
something ask. Re-measured, zsh 5.9.2, `-f -i` on a pipe with a scratch
`HOME`, `ZDOTDIR` and `HISTFILE`, `fc -l` read inside the session and the
file read after `fc -W`:

| zsh 5.9.2 knob | in `fc -l` | in the file |
| --- | --- | --- |
| `HISTORY_IGNORE='echo hidden'` | **yes**, as entry 5 | no |
| `setopt HIST_IGNORE_SPACE` | no | no |
| `setopt HIST_IGNORE_DUPS` | no | no |

So zsh gives *both* answers, and which one it gives is decided by the
knob and not by the shell. bash 5.3.15, bash 3.2.57 and bash-as-sh drop
the line from `history` and from the file under all three of their rules.
**The panel therefore agrees about a blank and about a repeat, and parts
company only over a pattern** — which is a narrower conflict than the one
first recorded, and it is why the axis below belongs to the pattern knob
alone.

Three more things in there are worth stating separately.

**ksh93 has neither knob.** ` echo spaced` typed with a leading space
is in the file, and setting `HISTIGNORE` — a name it does not know —
changes nothing: both `echo one` and `export A_SECRET=x` were recorded
with it set. Its file is also not text; the entries are readable with
`strings` and the framing around them is not.

**dash keeps no history.** No file appeared, with `HISTFILE` set and
`-i` given. There is nothing to configure.

**bash and zsh disagree about what "ignored" means**, and the
disagreement is the useful part. Given a matching line, bash 5.3 leaves
it out of the history list entirely — its own `history` builtin listed
only `echo one` and the `history` call itself — while zsh 5.9 keeps it
in the list, where `fc -l` finds it, and omits it from the file. One is
"never recorded"; the other is "recorded, not saved".

## What none of them do

Every knob above is a **glob the person wrote**. `HISTIGNORE='*SECRET*'`
protects someone who predicted the shape of their own secret, in
advance, and typed the pattern into a startup file before the day they
needed it. None of the four inspects a line to decide whether it carries
a credential, and none ships a rule that knows what an AWS key id looks
like.

That is the gap this shell fills, and it is a gap rather than a
divergence: there is no measured behavior to be compatible with.

## What this shell does — ours, not measured

A line accepted at the prompt is matched against a table of credential
patterns before it can reach the history file. A line that matches is
**not written**, and a one-line notice says so where it was typed:

    sh: history: not saving this line (matched aws-access-key-id)

Four decisions, and the reasoning for each:

**The rules are content, not configuration.** They run for everyone,
with nothing to switch on, because a protection that has to be enabled
protects the people who already knew. `internal/secret` holds the
table; each rule either names a credential's own documented prefix — an
AWS key id, `ghp_`, PEM armor — or requires the text around the value to
say what it is: `password=`, `Authorization:`, a password inside a URL's
authority. There is no entropy rule, deliberately: "this looks random"
fires on a commit hash, a UUID and a base64 image, and a scrubber that
rejects ordinary lines is worse than no scrubber, because people turn it
off.

**A command line is rejected whole; output is redacted.** These are not
the same decision. A command line carrying a secret usually *is* the
secret — `export TOKEN=…` is nothing else — so a redacted skeleton
recalls nothing anyone wanted and dropping the line is proportionate.
Output is the opposite: discarding a two-hundred-kilobyte build log
because one line of it echoed a token destroys exactly what was worth
keeping, so there the credential is replaced and the rest stands. Both
halves read the same table, which is the only way the policy can be
described in one sentence.

**The check is at the write path.** Everything that reaches the file
goes through one function, so "the history file never held a
credential" is a property of the file rather than a property of one loop
remembering to ask.

**The notice is at the prompt, and the line stays recallable.** The file
is written as the shell exits, which is the worst available moment for
something a person is meant to read, so the notice is printed when the
line is accepted — directly under what was typed. And the line remains
in the session's history: this is zsh's answer from the table above
rather than bash's, taken for a reason about this feature rather than
about zsh. A refused line is very often one that is about to be retyped
— the token had a character missing — and a scrubber that also takes
away the up arrow is one people work around. Nothing that was on the
screen anyway is protected by forgetting it.

### Known limits, stated rather than discovered later

- **A bare secret with nothing around it does not match.** A value
  pasted alone, with no name and no prefix of its own, has nothing in
  the text to match on. This is the cost of refusing to guess from
  entropy and is paid knowingly.
- **`mysql -p<password>` does not match.** A one-letter option is
  ambiguous — `-p` is `--port` and `--preserve` elsewhere — and a rule
  that fired on it would fire on those.
- **A history file written before this landed is not cleaned.** The
  rules run on the way in. Lines already on disk stay there, and a shell
  that silently rewrote a file it did not write is a worse thing to
  have.

### The panel's knobs are not part of the scrubbing

`HISTCONTROL`, `HISTIGNORE`, `HIST_IGNORE_SPACE` and `HISTORY_IGNORE`
are measured above and were deliberately not part of the scrubbing
change. They are a compatibility question with a dialect answer — bash
and zsh name them differently, spell their patterns differently and
disagree about the session list — where the scrubbing is a security
feature with no dialect axis at all. All four are built now; see below.

## The history at the prompt

Measured under a pseudo-terminal on 2026-09-05, bash 5.3.15 and
zsh 5.9.2, each with a throwaway `HOME`, a `HISTFILE` of its own and a
window size of 24×80. `C-r` is a *screen* behavior and both shells draw
it incrementally — inserting characters into the line already there
rather than redrawing it — so the raw stream is the optimisation and not
the result. Every screen below was reconstructed from the bytes with a
terminal model, which is also how the cursor columns were read.

### Walking the list

Both shells agree on all of this, so none of it is an axis:

- **Up and Down walk the list**, oldest at the bottom, with a floor at
  each end rather than a wrap.
- **A half-typed line is put aside** and comes back when Down reaches
  the end of the walk.
- **An edit made to a recalled entry is kept for the rest of the line.**
  Recall `ls -la`, type `XX`, press Up and then Down, and `ls -laXX`
  comes back. The entry itself is never changed and the edits do not
  outlive the line: `^C` then Up gives the clean entry back.
- **A consecutive duplicate is recorded** unless a knob says otherwise.
  `echo a` twice leaves two entries in both.

### `C-r`, and where it is drawn

The two shells differ in the wording *and* in the placement, which is
two axes' worth of difference in one feature.

bash replaces the prompt with the search and keeps the matched line
after it, with the cursor on the first character of the match:

    (reverse-i-search)`echo': echo two

zsh leaves the prompt and the line where they are, cursor on the match,
and puts the search on a row of its own below them, with a trailing
underscore that is part of the wording rather than a cursor:

    P> echo two
    bck-i-search: echo_

Once nothing older matches, both change only the wording — `(failed
reverse-i-search)` and `failing bck-i-search:` — ring the bell, and
leave the last line that did match on the screen.

What they agree about:

| keystroke | what happens |
| --- | --- |
| a character | extends the query and searches again, from the entry on screen |
| backspace | shortens the query |
| `C-r` again | the next older match |
| `C-g` | abandons: the line and cursor go back to before the search |
| Enter | accepts the found line and **runs** it |
| `C-e`, `C-k`, an arrow, Tab | ends the search, keeps the found line, **and acts** |

That last row is the one worth writing down. `C-r cho C-e` leaves the
search, keeps `echo two`, and moves the cursor to the end of it; `C-r
cho C-k` leaves `e`. The key that closes the mode is not swallowed by
it.

**bash also highlights the match** in reverse video, and zsh does not.
That is not implemented here — the match is found and the cursor is put
on it, and nothing is drawn around it.

Re-measured 2026-09-16 and still true, and the *instrument* is the whole
difficulty: a rendered screen cannot answer this question, because replaying a
terminal consumes the escape sequences the question is about. Asked of the
rendered text, every column answers "no highlight", including bash. Asked of the
raw bytes, with a control run that types the same line without searching so the
shell's ordinary drawing is subtracted: bash adds four `\e[7m`/`\e[27m` pairs
that its control run does not, zsh adds none, and neither of ours adds any.
`internal/cmd/viprobe -raw` is the half that can see it.

**`ESC` differs and is not implemented either.** In bash a bare `ESC`
ends the search; in zsh it does nothing, being the start of a prefix.
Neither dialect ends the search here.

Re-measured 2026-09-16 with a probe that reads the *effect* rather than the
redraw — `echo ALPHAMARK` in the history, then `^R ALPH`, then `ESC`, then `X`,
then Return. bash runs `echo XALPHAMARK`: the `ESC` ended the search and the `X`
landed in the line. zsh, and both of ours, run `echo ALPHAMARK`: the `ESC` was
nothing and the `X` went on extending a query that then matched nothing.

**The reason this file used to give was wrong, and it undersold the editor.** It
said telling a bare `ESC` from the first byte of an arrow needs a timeout "which
this editor does not have anywhere". No timeout is needed and the editor already
does the harder half: it asks whether a byte is *there*, waiting no time at all,
which works because a terminal writes an escape sequence in one write. Measured
— in vi mode a lone `ESC` enters command mode, while `\e[A` delivered as a
single write is an arrow key in the same session. `docs/spec/editing.md` has the
mechanism. The search simply does not consult it: `ESC` reaches the search as a
prefix and the search has nothing bound behind it. That is a key to bind, not a
clock to build.

**ksh93's `C-r` is not this.** Measured: it echoes `^R` and takes a
whole string afterwards, non-incrementally. dash has no line editor at
all. Neither dialect answers this question, so both take the
substrate's own wording, which names no shell:

    (reverse-search)`echo': echo two

### The two sizes

`HISTSIZE` bounds the list a session can recall and `HISTFILESIZE`
bounds the file, and they are different questions. Measured: bash with
`HISTSIZE=2` and four lines in the file lets the up arrow reach two of
them and stops. `HISTFILESIZE` defaults to `HISTSIZE`'s value, so
`HISTSIZE=2` alone leaves two lines on disk, and `HISTSIZE=2
HISTFILESIZE=100` recalls two and keeps every earlier line. Zero for
either means nothing is kept.

This shell diverges on *how* the file is brought under its bound, on
purpose. bash rewrites the whole file from its in-memory list at exit,
so a bash that ran with a small `HISTSIZE` throws away what earlier
sessions wrote. This shell appends what the session added and rewrites
only when the file is over `HISTFILESIZE`, keeping the tail — the end
state is the same, and a small `HISTSIZE` does not silently delete
history the session never saw. `HISTFILESIZE=0` writes nothing rather
than emptying the file, for the same reason.

### The knobs, and the one conflict

The rules themselves are shared. A leading blank hides a line; a
duplicate is the line *immediately* before it and not any earlier one —
measured, `echo a`, `echo a`, `echo b`, `echo a` leaves three entries in
both shells with the option on; a pattern is matched against the whole
line, so `HISTIGNORE=pwd` drops `pwd` and keeps `pwd x`. Patterns are
globs and not words: `HISTIGNORE=ls*` also drops `lsof -h`.

What differs is where they are written and what "ignored" costs:

| | bash | zsh |
| --- | --- | --- |
| blank / duplicate | `HISTCONTROL=ignorespace`, `ignoredups`, `ignoreboth` | `setopt HIST_IGNORE_SPACE`, `HIST_IGNORE_DUPS` |
| patterns | `HISTIGNORE`, a colon-separated **list** | `HISTORY_IGNORE`, a **single** pattern |
| a line the *pattern* knob rejected | **gone** — the up arrow skips past it | **kept** — the up arrow recalls it |
| a line the blank or repeat rule rejected | gone | gone |

The first of those two rows is the axis:
`repl.HistoryStyle.PatternIgnoredStaysInSession`. The second is not an
axis at all, and finding that out is what the correction above is: the
panel agrees, so the rule is written once and takes no dialect answer.

All four knobs are built. zsh's two are option names rather than
variables, which is a different namespace and not a different spelling —
they are read through the dialect's own option namespace
(`interp.Runner.DialectOption`), so `setopt hist_ignore_space`,
`HIST_IGNORE_SPACE` and `histignorespace` are one request here exactly as
they are one request in zsh, and a dialect with no such option leaves the
field blank rather than naming a variable its shell does not have.
`histignorespace` and `histignoredups` are the two names that stopped
being *recorded-only* when this was built; see `docs/spec/semantics.md`.

One thing is measured and not built. bash's `HISTIGNORE` gives `&` a
meaning of its own: measured,
`HISTIGNORE=&` drops a line identical to the one before it, exactly as
`ignoredups` does. That is not implemented — a pattern of `&` is matched
literally here — because it is a second spelling of a rule the same
variable's neighbor already has.

Re-measured 2026-09-16 and still true, with a control, because the first probe
written for it had no control and lied. `HISTIGNORE=&` typed at a prompt is not
an assignment at all: the `&` ends the command, so the variable is set to the
empty string and the shell reports no duplicate dropped — which reads exactly
like a bash that has not got the feature. Quoted, and with `echo TWOBACK`,
`echo dupline`, `echo dupline` behind it, two presses of Up reach `echo TWOBACK`
in bash with `HISTIGNORE='&'` and `echo dupline` in bash without it. Ours
reaches `echo dupline` either way.

## `$histchars`: three characters, and what an empty one costs

zsh keeps the characters history expansion is spelled with in a parameter
a script can read, under two names for one value. Measured on zsh 5.9.2
with no startup files:

| | |
| --- | --- |
| `$histchars` | `!^#`, and `${(t)histchars}` is `scalar-special` |
| `${histchars[1]}` | `!` — history expansion |
| `${histchars[2]}` | `^` — quick substitution |
| `${histchars[3]}` | `#` — the comment character |
| `$HISTCHARS` | the same parameter under a second name |

A write through either name is read back through both, it takes `local`
inside a function and the outer value returns with the frame, `typeset -x`
exports it, and `typeset -p` reports an ordinary scalar.

**zsh is the only shell in the panel that ships it set.** bash recognizes the
name — the manual documents `histchars` as the characters history expansion is
spelled with, and an assignment to it is honored — but a fresh bash leaves it
**empty**, interactive or not, measured on 5.3.15 and on 3.2.57. ksh93 and
dash read it as an ordinary unset name. So the value is zsh's answer rather
than the panel's, and it is kept in this dialect rather than in the core.

The value and the two spellings are what this shell keeps. It performs no
history expansion, so nothing here *acts* on the characters — the parameter
is what a script reads, and reading it is the whole of what the readers on
this machine do with it.

**Two properties are measured and not modeled**, written down here so that
neither later reads as untested. An assignment is truncated to three
characters in zsh — `histchars=abcdef` leaves `abc` — where the whole string
stays here. And a non-ASCII assignment is refused there, with `HISTCHARS can
only contain ASCII characters` and the old value standing, where it is taken
here. Both are only visible to a script that assigns something this parameter
means nothing with.

### Why an empty value is not a small divergence

This shell had the parameter absent, which reads as the empty string, and
that is worse than it sounds because **every reader reads it a character at
a time and uses the character as the left end of a pattern**. An empty
`${histchars[1]}` does not make `[[ $word = ${histchars[1]}* ]]` ask about
nothing; it makes it `[[ $word = * ]]`, which is true of every word. The test
does not error, the parameter is not unset, and nothing anywhere is in a
state a guard could notice — the pattern simply stops discriminating.

Measured 2026-09-12 against the syntax highlighter installed on this machine,
on `echo hello | grep x`: it asks that exact question to find a history
expansion, so with the parameter empty every argument of two or more
characters was styled as one. Real zsh produced two colored runs for that
line and this shell produced three. The single-character `x` was spared only
by the second half of the test — `&& -n ${word[2]}` — which is what a
degenerate pattern looks like from the outside: not an error, but an
exception that makes no sense.

The same reader asks about `${histchars[3]}` one line further on to find a
comment, so the same emptiness classified a whole line as one whenever the
comment-aware tokenizer was chosen. That was the symptom #2536 was filed for,
and fixing the *option* that chooses that tokenizer (#2516, #2530) uncovered
this rather than completing it — the two faults had one appearance and no
connection.

## History expansion: `!!`, `!$`, `!foo` and `^old^new^`

The oldest feature in any interactive shell, and the one this tree had none
of at all until #3093 — `set -H` was `set: -H is not implemented yet`, and
`!!` reached the parser as two literal characters, so a shell used at a prompt
answered a different command from the one its user typed.

Measured on 2026-09-15, macOS 26 on arm64, through a pseudo-terminal with a
**two-row prompt** (`internal/cmd/histprobe`) and again from a script under
`set -o history; set -H`. A two-row prompt rather than a one-line `PS1`
deliberately: components can each match the shell they imitate while the
composition of them does not.

By hand rather than through `internal/cmd/oracle`, and for a reason stronger
than the one above: the harness runs each case as a **command string**, where
every shell in the panel has the expander off. A row there would have
measured the same "nothing happened" in all six columns and looked like
agreement.

### The panel

| shell | at a prompt | a script starts | the switch | `set -H` |
| --- | --- | --- | --- | --- |
| bash 5.3.20 | **on** | off | `set -o histexpand`, `set -o history` | history expansion |
| bash 3.2.57 (`/bin/bash`) | **on** | off | same | same |
| bash as `sh` | **on** | off | same | same |
| zsh 5.9.2 | **on** | off | `setopt banghist` (`set -o histexpand`) | **`rmstarsilent`** — a different option |
| ksh93u+ 2012-08-01 | **off** | off | `set -o histexpand` | history expansion |
| dash | none | none | none | `Illegal option -H` |
| BusyBox ash | none | none | none | the same shell's answer |

The third column is where a *session* starts, not what a script can reach: a
script that writes the options gets the expander in bash and gets nothing in
zsh or ksh93, which is the separate axis **The script route** below is about.

Three rows in that table are traps for an implementation that assumed one
behavior:

- **zsh's `set -H` is not this feature.** Diffing `setopt` across it shows the
  one name that moves is `rmstarsilent`. zsh reaches the expander through
  `setopt banghist` and the borrowed `set -o histexpand` alone.
- **ksh93 starts with it off, at a prompt as well as in a script.** It is the
  one shell in the panel that does, which is why
  `Semantics.HistoryExpansionAtAPrompt` is an axis and not a constant.
- **zsh's option and zsh's expander are two states.** `[[ -o banghist ]]`
  reports **on** in `zsh -c`, where nothing expands: zsh gates the expander on
  being interactive and leaves the option where it is.

### What bash was measured doing

A script holding `set -o history`, `set -H`, `echo one two three`, and then
the line in the first column. The second column is what bash echoed to
**standard error** — the expanded line, written before it runs. A row with no
echo is one bash says the expansion did not change.

| typed | expanded |
| --- | --- |
| `echo !!` | `echo echo one two three` |
| `echo !1`, `echo !-1`, `echo !e`, `echo !?two?` | `echo echo one two three` |
| `echo !$` | `echo three` |
| `echo !^`, `echo !:1` | `echo one` |
| `echo !*` | `echo one two three` |
| `echo !:0` | `echo echo` |
| `echo !:2-3` | `echo two three` |
| `^one^ONE^` | `echo ONE two three` |
| `echo !!:s/one/1/` | `echo echo 1 two three` |
| `echo "!!"` | `echo "echo one two three"` |
| `echo '!!'` | *(nothing — single quotes protect)* |
| `echo a\!b` | *(nothing — the backslash protects, and stays)* |

Against `echo /a/b/c.txt other`, the modifiers that take a path apart:
`!:1:h` is `/a/b`, `!:1:t` is `c.txt`, `!:1:r` is `/a/b/c`, `!:1:e` is
`.txt`. `:p` prints the expansion, remembers it, and runs nothing.

### The echo, the history, and the status

Three facts an implementation gets wrong by leaving them out:

- **The echo goes to standard error**, not standard output. Redirecting a
  command's output does not hide what the shell is about to run.
- **What goes into the history is the expanded text.** `echo AAA` then `!!`
  twice leaves `echo AAA`, `echo echo AAA`, `echo echo echo AAA` in the list,
  so each reference resolves against what the one before it produced.
- **A failed expansion leaves `$?` alone.** The line does not run and the
  status stays where the command before it put it — measured, `echo a`,
  `echo !nosuch`, `echo "rc=$?"` prints `rc=0`.

The three complaints are worded three ways, which is why they are
`Diagnostics` fields and not constants:

| | event not found | substitution failed |
| --- | --- | --- |
| bash | `bash: !nosuch: event not found` | `bash: :s^nope^x^: substitution failed` |
| ksh93 | `ksh: !nosuch: event not found` | `ksh: ^nope^x^: substitution failed` |
| zsh | `zsh: event not found: nosuch` | `zsh: substitution failed` |

bash names the *modifier it rewrote the line into* and ksh93 names what was
typed; zsh puts the reference last, without the character that introduced it,
and names nothing at all for a substitution.

### When a `!` is not an expansion

Measured one character at a time on bash 5.3.20, by asking
`printf '%s\n' "T<c>|!<c>|"` after a seeded history. A `!` is ordinary text
when:

- it is the last character of the line, or what follows it is one of
  `` \t\n\r=|&;()<>"' `` (a space included) — so `echo end!`, `echo hi ! there`,
  `[[ ! -e x ]]`, `! false` and `$((3 != 4))` are all safe;
- it stands **immediately** after `[`, so `[!a-z]` is still a glob. Only
  immediately: `[a!s]` *is* an event reference, and bash reads it as one;
- it stands directly after `${`, so `${!v}` is still an indirect expansion.
  The two characters together, not the brace — `$ {!s}` is an event reference.

Everything else after a `!` starts one, `,` `.` `/` `@` `+` `~` `` ` `` `[` `]`
`{` `}` and `\` included.

Quoting is the scanner's question and not the parser's, which is why the
engine here is a pass over a string rather than a stage of lexing: text inside
single quotes is never expanded, text inside double quotes is, and a `'` inside
double quotes opens nothing — `echo "it's !!"` expands where `echo '!!'` does
not. Nothing that had already tokenized the line could tell those apart from a
parameter expansion's rules.

## The script route

bash expands in a script too, once `set -o history` and `set -H` have both
been written — and it does it by expanding each **physical line as it reads
it**. That one fact is the whole of the route, and everything below is a
consequence of it. Measured 2026-09-16 on bash 5.3.20 and again on bash 3.2.57
and the same binary invoked as `sh`, which answer identically on every row.

It is **bash's row alone**, which is the third history axis,
`Semantics.HistoryExpansionInAScript`. zsh refuses `set -o history` outright
(`no such option`) and with its own `setopt banghist` written instead still
prints the two characters; ksh93 refuses the name (`bad option(s)`), takes the
`set -H` it does have, and still prints the two characters. So two shells with
an expander apiece confine it to a prompt, and one uses it wherever it reads:
`-c`, a script file and a program on standard input all expand.

### What follows from reading a line at a time

| written | what happens | why |
| --- | --- | --- |
| `set -H` on line 2 | line 3 expands, line 2 does not | the line is read before any of it runs |
| `set -H; echo !!` on one line | nothing expands | same, on one line |
| `f() {` / `echo !!` / `}` | the **definition** holds the expansion | the body was expanded when it was read, not when `f` is called |
| `echo a \` / `!! b` | expands, and the echo shows that physical line alone | a continuation line is a physical line |
| `echo "a` / `!!` / `b"` | expands | the quote carries and references expand inside `"` |
| `echo 'a` / `!!` / `b'` | left alone | the quote carries |
| `echo 'a` / `b' !!` | expands after the quote closes | the state seeds the scan; the scan still closes the quote |
| `cat <<EOD` / `x !! y` / `EOD` | left alone | a here-document's body is not shell text — quoted delimiter or not |
| `x=$(echo` / `!!)` | expands | a substitution is ordinary text to this |
| `eval 'echo !!'`, `. file`, an alias body | left alone | only the program the shell is **reading** |

What a line begins inside comes from the parser (`syntax.Parser.OpenQuote`)
rather than being worked out again from the line before it: `$'`, a backquote,
a `'` inside a double-quoted string and a here-document body are four separate
answers, and the lexer already has all four.

### The list a script builds

`set -o history` starts it, and it is the same list the `history` builtin
keeps. The two options are an **AND held continuously** rather than a
sequence: measured, `set +o history` part way through stops the expander as
well as the list — `echo !!` prints the two characters again — and a later
`set -o history` starts both again over the entries the list still holds — so a `history -s` entry is reachable from a later `!!`, and `history`
in a script reads back what the script has run.

A command joins the list **before** it runs, which is measured twice over:
`history` written in a script lists itself, and `history -s planted` followed
by `!!` recalls what was planted rather than the line that planted it.

One entry per command, however many physical lines it took, with the
newlines written as the separators the text can take — measured by reading
bash's own list back:

| written | the entry |
| --- | --- |
| `if true` / `then` / `  echo hi` / `fi` | `if true; then   echo hi; fi` |
| `for i in 1 2` / `do` / `echo $i` / `done` | `for i in 1 2; do echo $i; done` |
| `f() {` / `echo c` / `}` | `f() { echo c; }` |
| `echo a \|` / `cat` | `echo a \| cat` |
| `cat <<EOD` / `body` / `EOD` | the three lines and a newline after them |
| `echo a &` / `wait` | **two** entries — the `&` ended the command |

So a `;` is written unless the text so far ends in something that cannot take
one — `&&`, `\|\|`, `\|`, `;;`, `{`, `(`, `then`, `else`, `do`, `in`, or a blank
line — and a boundary the parser was inside a quote or a here-document at
takes a newline instead, because a `;` there would be text.

A comment line is an entry of its own. A line of **blanks** is an entry too;
only a truly empty line is not.

### The echo, and a reference nothing answers

The expanded line goes to the script's own standard error before it runs:
`exec 2>file` earlier in the script captures it, and a redirection on the
command carrying the reference does not — the expansion happens while the line
is being read, before anything that line says has taken effect. The complaint
about a reference the list cannot answer goes to the same place, in the shape
a script's diagnostics have: `s.sh: line 3: !nosuch: event not found`.

That line is then **dropped**, and dropped before the parser ever sees it,
which is visible in every line number after it. Measured: a second bad
reference on the file's line 5 is reported at `line 4`, a `$LINENO` on the
file's line 4 reads 3, and a syntax error on the file's line 5 is reported at
line 4. The status is left where the command before it put it, and the script
carries on.

### What is implemented, and what is not

Implemented: every event designator (`!!`, `!n`, `!-n`, `!string`,
`!?string?`, `!#`), every word designator (`^`, `$`, `*`, `%`, `n`, `x-y`,
`x-`, `x*`), the modifiers `h t r e p q x s/// & g a`, quick substitution, the
quoting rules above, `histchars`, the prompt route and the script route, and
the `history` builtin's tie to the list the designators index — including the
line `history -s` and `history -p` each drop from it, which is their own.

Three of those modifiers were **wrong** until the script route made them easy
to run against bash, and all three were wrong at a prompt as well. `:p` showed
the expansion and then ran the line anyway, which is the one thing it exists
not to do. `:r` and `:e` read the last `.` of the *basename* where bash reads
the last `.` of the whole word — `/a.b/c` is the discriminator, where `:r` is
`/a` — and `:e` answered an empty string for every word with no extension
where bash hands the word back whole. The eight words behind the corrected
rule are in `internal/histexpand`'s own comment.

`!{…}` is listed above as **not** implemented, and it used to be listed as
implemented. It is expanded here and is `event not found` in bash 5.3.20, bash
3.2.57 and bash-as-`sh` alike — the brace is not punctuation there, and even
`!{1}` fails with event 1 in the list. Whether any shell in the panel has the
form cannot be settled from a script, since the two that might do not expand
in one; see #3220, which is where the pty measurement goes.

**Not implemented: `shopt histverify`**, which puts the expansion back on the
editing line instead of running it. It is the one remaining piece and it is a
*line editor's* feature rather than a reader's: nothing about the expansion
changes, only what is done with the result, and the seam it needs is the one
that puts text into the editing buffer.

**One divergence, deliberately not modeled.** A reference recalling a command
that holds a here-document — `cat <<EOD` / `body` / `EOD` and then `echo !!` —
runs here and confuses bash: bash pushes the recalled text back into its
reader a line at a time, so the here-document re-opens, is never terminated,
and the body's lines are then run as commands (`x: command not found`). The
expansion is the same in both; what differs is that this shell hands the
expanded text to the parser whole. Modeling bash's answer would mean
modeling its push-back buffer, which is a property of its reader rather than
of the language.
