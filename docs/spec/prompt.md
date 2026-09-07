# The prompt languages

What a shell does to `PS1` and `PS2` before drawing them, and what the
line editor has to know about the result.

**Governed by:** `interp.PromptStyle`, which is the dialect's answer and not
the semantics vector's — "which shell am I" is data here in the same way it
is there. It is aliased as `repl.PromptStyle`, and the alias is the point:
there is **one** table, read by two readers.

**Measured:** through a pseudo-terminal on macOS 25.5, 2026-09-05, one code
per prompt, with the drawn bytes compared as hex rather than read off the
screen — the interesting half of this is invisible characters. The panel
was `/opt/homebrew/bin/bash` 5.3.15, `/bin/bash` 3.2.57 and
`/opt/homebrew/bin/zsh` 5.9.2, each started with its startup files
suppressed, in a scratch `HOME`, and driven by typing an assignment to the
parameter and waiting for the next prompt.

## Two readers, one table

The escape language is asked for twice, and the two askers are in different
packages.

- **A prompt is drawn** from it, by the line editor, which is the whole of
  the rest of this document.
- **A script asks for the same expansion by name** — zsh's `${(%)…}`, and
  its other spelling `print -P`, which are one expansion rather than two.

They used to be two tables. `interp` carried four escapes — `%%`, `%x`,
`%N`, `%n` — and refused everything else by name; `repl.PromptStyle`
carried about forty. So the shell answered `%F{196}` when drawing a prompt
and refused it when a script wrote `print -P '%F{196}…'`: one question, two
answers, and the larger table was the one a script could not reach. The
table now lives where both readers can see it and the alias makes them the
same Go type, so a dialect fills in one table and both readers read it.

**What is still two is the resolver, and that is a fact about the readers
rather than about the language.** A drawer knows things a script's
expansion does not, and each is measured:

| code | a drawn prompt | `${(%)…}` and `print -P` | measured |
| --- | --- | --- | --- |
| `%!` `%h` | the session's history number | **refused by name** | zsh answers 0 under `-c`, which is a plausible number for a question nobody answered |
| `%y` `%l` | the terminal's name | **refused by name** | zsh answers `()` with no terminal |
| `%_` | the construct being continued | nothing | `print -P '%_'` is empty — a script that reached an expansion has parsed |
| `%{` `%}` | the editor's width markers | nothing | `print -P '%{X%}'` is `X`, and neither marker reaches the output |
| a newline | `\r\n` | `\n` | the terminal is in raw mode while a prompt is drawn, so a bare newline leaves the cursor where it was across |

A code in **no** part of the table splits the same way, and this is the
convention that let the whole surface be enumerated exactly rather than
guessed: a prompt has to draw something and draws whatever the dialect's
"never heard of it" answer is, and a script's expansion **refuses the
escape by name**. `${(%):-%q}` says which escape it could not answer where
the shell it copies drops it silently at status 0, because a prompt quietly
short of a field is the kind of wrong answer nobody reports.

## Three passes, in this order

1. **The dialect's table of codes**, below.
2. **Expansion**, if the dialect expands prompts at all.
3. **The character that stands for the history number**, where a dialect
   has one — only ksh93 does.

The order is measurable. With `x='\u'` set, bash drew `$x` as the two
characters, so a code arriving *through* expansion is text; ksh93 drew
`\!` as the number, which is its table dropping the backslash and the pass
after it reading the `!` that was left.

zsh reverses the first two when `PROMPT_SUBST` is set: with the option on
and `x='%n'`, `PS1='$x'` drew the user name, so its table is read *after*
substitution. That option is not implemented here yet; when it is, the
order is a second question and not a consequence of the first.

Both parameters take all of it. Measured with `PS2` set to a prompt of
codes: bash drew the user name, the directory, the privilege character and
a bracketed color at the continuation prompt, and zsh drew the same from
its own language. A command substitution in either is run **again at every
prompt**, which is how a prompt that names the current branch works —
bash's `PS2='@@$(echo re)##'` drew `re` each time it was drawn.

## Non-printing markers

**The rule.** `\[ ... \]` in bash and `%{ ... %}` in zsh bracket bytes that
instruct the terminal rather than filling any of it. The bytes are written;
none of them is a column.

This is the part of the prompt language that is not cosmetic. Every
calculation the line editor makes — which screen row the cursor is on, how
far back a redraw has to come, whether the line wrapped at all — counts
from the width of the prompt, so a colored prompt whose escape sequences
are counted as text draws every long line over itself. That reads as a
broken shell rather than as a wrong prompt.

Measured:

| written | bash 5.3.15 / 3.2.57 | zsh 5.9.2 |
| --- | --- | --- |
| `\[X\]` / `%{X%}` | `X` | `X` |
| the opening alone | dropped, and the rest is drawn | dropped |
| the closing alone | **a literal `02` byte on the wire** | dropped |
| nested pairs | the outer pair is taken and the inner markers are drawn as `01` and `02` | all four are taken |
| `\[\e]0;title\a\]X` | the title sequence, then `X` | — |

**Our answer:** the markers are taken out and what is between them is not
counted, matched or unmatched. bash's stray `02` is not reproduced: a
control character on the screen is not something the prompt asked for, and
nothing in the language says what an unmatched marker means.

The markers survive rendering as `01` and `02` and are removed where the
prompt is written — which is both loops, since the one without a terminal
writes the prompt itself. That representation is bash's own rather than an
invention: `PS1=$'\001\033[31m\002X'` drew the color and the `X` with
neither byte on the wire, so a literal `01` in a bash prompt *is* a `\[`.
zsh draws both bytes instead, so a zsh prompt holding a literal `01` loses
it here — invisibly, since a terminal gives it no cells either way.

## bash — the backslash language

5.3.15 and 3.2.57 agree on the whole table. Every difference between the
two was the value of the moment — the clock, the version, the history
number — and never the code.

| code | draws | measured |
| --- | --- | --- |
| `\u` | user name | `bhamilton` |
| `\h` `\H` | host to the first dot, and all of it | `Blairs-MacBook-Pro-5`, `…-5.local` |
| `\w` | directory, `$HOME` written `~` | `~`, `~/sub`, `/` |
| `\W` | its last component | `~` at `$HOME`, `sub` below it, `/` at the root |
| `\$` | `#` for root and `$` otherwise | `$` |
| `\s` `\v` `\V` | shell name, version, full version | `bash`, `5.3`, `5.3.15` |
| `\d` | date | `Sat Sep 05` |
| `\t` `\T` `\A` `\@` | clock | `15:04:36`, `03:04:36`, `15:04`, `03:04 PM` |
| `\D{fmt}` | strftime, and `\D{}` is the locale's time | `2026-09-05 15:04:44`, `15:04:44` |
| `\n` `\r` | new line, carriage return | `0d 0d 0a` and `0d` on the wire — the terminal's own translation makes the first two of those out of one `0a`, so what bash wrote is `0d 0a` |
| `\!` `\#` | history number, command number | `40`, `37` |
| `\j` `\l` | live jobs, terminal name | `0`, `ttys008` |
| `\e` `\a` | escape, bell | `1b`, `07` |
| `\\` | one backslash | `5c` |
| `\[` `\]` | non-printing markers | see above |
| `\nnn` | the byte three octal digits name | `\007` → `07`, `\101` → `A` |
| anything else | **both characters, as written** | `\q` → `\q` |

The octal escape is three digits exactly: `\0`, `\1`, `\10`, `\00` and `\8`
were each drawn as the two characters written, `\1011` drew `A1`, and
`\400` drew a NUL — the low byte of the value.

`\D{fmt}` is measured and not implemented: it needs a translation from
strftime's language to Go's layouts rather than a row in a table, and until
it has one, `\D{%F}` is drawn as written, which is what an unknown code
does.

## zsh — the percent language

| code | draws | measured |
| --- | --- | --- |
| `%n` | user name | `bhamilton` |
| `%m` `%M` | host to the first dot, and all of it | as bash's `\h` `\H` |
| `%~` | directory, `$HOME` written `~` | `~`, `~/sub`, `/` |
| `%d` `%/` | directory, untouched | `/private/tmp/p808/home` |
| `%c` `%.` | last component of `%~` | `~` at `$HOME`, `sub` below it |
| `%C` | last component of `%d` | `home` at `$HOME` |
| `%N~` `%Nd` | the last N components | `%2~` drew `sub/deeper` |
| `%#` | `#` for root and `%` otherwise | `%` |
| `%%` | one percent sign | `25` |
| `%*` `%T` | clock, **hour not padded** | `6:11:43`, `6:11`, and `0:17:07` after midnight |
| `%t` `%@` | twelve-hour clock, hour padded with a space | ` 6:11AM`, `12:17AM` |
| `%w` `%W` `%D` | dates | `Sun 6`, `09/06/26`, `26-09-06` |
| `%D{fmt}` | strftime — the braces replace the plain date | `2026-09-05 15:05:41`; `%D{%H:%M}` → `04:25`, and `%D{}` → **nothing**, so the braces being there is the question and not what is in them |
| `%!` `%h` | history number | `75`, `77` on successive prompts |
| `%j` `%?` | live jobs, last exit status | `0`, `0` |
| `%i` `%L` `%N` | line number, `$SHLVL`, the shell's own name | `106`, `2`, `/opt/homebrew/bin/zsh` |
| `%l` `%y` | terminal, with and without the `tty` | `s008`, `ttys008` |
| `%_` | what the line is still inside | `for` |
| `%{` `%}` | non-printing markers | see above |
| `%B` `%b` | bold on, and **everything** off | `\e[1m`, `\e[0m` |
| `%U` `%u` | underline on and off | `\e[4m`, `\e[24m` |
| `%S` `%s` | standout on and off | `\e[7m`, `\e[27m` |
| `%F{c}` `%f` | foreground color, and default | `\e[31m` for `red`, `\e[39m` |
| `%K{c}` `%k` | background color, and default | `\e[44m` for `blue`, `\e[49m` |
| `%E` | clear to the end of the line | `\e[K` |
| `%(x.a.b)` | a question, and one of two texts | `%(?.ok.bad)` drew `ok` |
| `%x` | the file being read | `/opt/homebrew/bin/zsh` at a prompt, the sourced file's path in one |
| a trailing `%` | **dropped, not drawn** | `PS1='x%'` drew `x`, and `print -P 'x%'` is `x` too — both readers, so it is a row of the table and bash and ksh93 draw the character |
| anything else | **nothing at all** | `%q` → nothing |

The clock is where the two languages disagree about the same fact rather
than about the spelling: measured at the same moment, bash's `\t` drew
`06:11:40` and zsh's `%*` drew `6:11:43`. Midnight says what the difference
is — zsh drew `0:17:07`, so the zero is taken off rather than replaced with
a space, and the hour keeps a digit.

The colors are the terminal's arithmetic rather than the dialect's, so they
are written once for whichever dialect asks: 30 to 37 are the eight
foreground colors, 40 to 47 the same backgrounds, 90 and 100 their bright
halves, 39 and 49 the defaults, and `38;5;n` / `48;5;n` everything above
15. Measured: `%F{2}` → `\e[32m`, `%F{9}` → `\e[91m`, `%F{200}` →
`\e[38;5;200m`, `%K{5}` → `\e[45m`. The names are lower case and only lower
case — `%F{Red}`, `%F{bogus}`, `%F{256}` and `%F{-1}` all drew the default
`\e[39m` — and an empty argument is black rather than the default, since
`%F` and `%F{}` both drew `\e[30m`.

`%D{fmt}` is implemented: the braces are read the way a color code's are
and handed to `interp.Strftime`, which is the same formatter
`printf '%(fmt)T'` writes through — a second implementation of one format
language is the thing that would drift. The clock is still each reader's
own, which is the resolver split doing what it is for: a prompt is tested
with an injected clock and a script's expansion with the runner's.

The `%(x.a.b)` ternary is measured and not implemented, and it is the one
shape left that needs a *mechanism* rather than a row: a question the
substrate can ask itself. Nor are `%i`, `%L`, `%l` and the `%N~`
truncations, each of which is a value nothing has been asked for yet; they
are recorded here so that adding one is a lookup rather than another
measuring session. Every one of them is refused by name when a script asks
for the expansion, so nothing on this list can be reached by accident.

## What the panel does with a code it has never heard of

Three shells, three answers, which is why it is asked rather than assumed:
bash drew `\q` for `\q`, ksh93 drew `q`, and zsh drew nothing at all for
`%q`.
