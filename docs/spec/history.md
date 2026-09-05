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
and dash. By hand rather than through `cmd/oracle`, because every
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
| zsh 5.9 | text, a line per entry | `setopt HIST_IGNORE_SPACE` | `HISTORY_IGNORE` glob | **kept** — `fc -l` still lists it |
| ksh93 | its own binary format | none observed | none observed | — |
| dash | none at all | — | — | — |

Three things in there are worth stating separately.

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

### The panel's knobs are not built here

`HISTCONTROL`, `HISTIGNORE`, `HIST_IGNORE_SPACE` and `HISTORY_IGNORE`
are measured above and deliberately not implemented by this change.
They are a compatibility question with a dialect answer — bash and zsh
name them differently, spell their patterns differently and disagree
about the session list — and belong with the rest of the interactive
option surface rather than with a security feature that has no dialect
axis at all.
