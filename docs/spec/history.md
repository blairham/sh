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

**`ESC` differs and is not implemented either.** In bash a bare `ESC`
ends the search; in zsh it does nothing, being the start of a prefix.
Telling a bare `ESC` from the first byte of an arrow needs a timeout,
which this editor does not have anywhere, so an `ESC` here is read as
the start of a sequence in both dialects.

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
exports it, and `typeset -p` reports an ordinary scalar. bash has no such
parameter; the other three shells in the panel have none either.

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
