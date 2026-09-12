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
| `\u` | user name, from the password database — `I have no name!` where the uid has no entry | `bhamilton`; `I have no name!` at uid 99999 in a container, in 5.3.15, in 3.2.57 and under an `argv[0]` of `sh` |
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
| `%n` | user name, from the password database — **nothing** where the uid has no entry | `bhamilton`; empty at uid 99999 in a container, where bash's `\u` draws `I have no name!` |
| `%m` `%M` | host to the first dot, and all of it | as bash's `\h` `\H` |
| `%~` | directory, `$HOME` written `~` | `~`, `~/sub`, `/` |
| `%d` `%/` | directory, untouched | `/private/tmp/p808/home` |
| `%c` `%.` | `%~` with the count defaulting to **one** | `~` at `$HOME`, `sub` below it, `/tmp` in `/tmp` |
| `%C` | `%d` with the count defaulting to **one** | `home` at `$HOME`, `/tmp` in `/tmp` |
| `%N~` `%Nd` `%Nc` `%NC` | the last N components | `%2~` drew `sub/deeper`, and `%2c` the same |
| `%-N~` `%-Nd` `%-Nc` | the **first** N components | `%-1~` drew `~`, `%-2~` `~/sub`, `%-1d` `/private` |
| `%-NF` `%-NK` | a negative color index: **nothing at all**, and the layer cleared | `%-2F` drew no bytes, where `%F{-1}` draws `\e[39m` |
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
| `%B` `%b` | bold on, and **everything** off — and both write back what they cleared, see below | `\e[1m`, `\e[0m` |
| `%U` `%u` | underline on and off | `\e[4m`, `\e[24m` |
| `%S` `%s` | standout on and off | `\e[7m`, `\e[27m` |
| `%F{c}` `%f` | foreground color, and default | `\e[31m` for `red`, `\e[39m` |
| `%K{c}` `%k` | background color, and default | `\e[44m` for `blue`, `\e[49m` |
| `%F{#rrggbb}` | a direct color, under every `TERM` | `\e[38;2;255;136;0m` for `#ff8800` |
| `%NF` `%NK` | a **count** in front of a color is the index it paints | `%2F` → `\e[32m`, `%30F` → `\e[38;5;30m`, `%2K` → `\e[42m` |
| `%E` | clear to the end of the line | `\e[K` |
| `%(x.a.b)` | a question, and one of two texts | `%(?.ok.bad)` drew `ok` |
| `%x` | the file being read | `/opt/homebrew/bin/zsh` at a prompt, the sourced file's path in one |
| a trailing `%` | **dropped, not drawn** | `PS1='x%'` drew `x`, and `print -P 'x%'` is `x` too — both readers, so it is a row of the table and bash and ksh93 draw the character |
| anything else | **nothing at all** | `%q` → nothing |

### The count in front of a code

The digits between the escape and the code are an argument to that code, and
they may carry a **minus**. Measured on zsh 5.9.2, 2026-09-12, in `/tmp/a/b/c`
and three levels under a home:

| written | drawn | |
| --- | --- | --- |
| `%2~` | `b/c` | positive: the **last** N |
| `%-2~` | `/tmp/a` | negative: the **first** N |
| `%-1~` under a home | `~` | the tilde is a unit … |
| `%-1d` spelled out | `/Users` | … and the leading slash is not |
| `%-~` | `/tmp` | a bare minus is minus one |
| `%-0~` | `/tmp/a/b/c` | nought is no limit, however it is spelled |
| `%-9~` | `/tmp/a/b/c` | past the units is the whole path |

`%c`, `%C` and `%.` are the same arithmetic with one difference, and it is the
whole of what separates them from `%~` and `%d`: an absent count means **one
component** rather than no limit, and a nought means one as well. So `%2c` is
`%2~`, `%-1c` is `%-1~`, and only `%c` and `%~` differ. That is also what makes
`%c` a different question from bash's `\W`, which has no count at all and
answers the plain basename: in `/tmp`, `\W` is `tmp` and `%c` is `/tmp`.

Every code reads the count, and the ones that count nothing ignore it exactly
as they ignore a positive one — `%-2n` is the login name and `%-2j` the job
count. The colors are the exception and the only one: a negative index there
draws **nothing whatever** and clears the layer, where the braced `%F{-1}` is a
number out of range and draws the terminal's default.

The clock is where the two languages disagree about the same fact rather
than about the spelling: measured at the same moment, bash's `\t` drew
`06:11:40` and zsh's `%*` drew `6:11:43`. Midnight says what the difference
is — zsh drew `0:17:07`, so the zero is taken off rather than replaced with
a space, and the hour keeps a digit.

### The restore, and which codes do it

There is no way on this terminal to turn boldface off on its own: `%b` writes
`\e[0m` — select-graphic-rendition nought, which clears everything, colors
included. So zsh writes the reset **and then writes back whatever else was in
effect**:

```
${(%%)'%F{031}a%b%k%F{242}x%f'}
\e[38;5;31m a \e[0m\e[38;5;31m \e[49m \e[38;5;242m x \e[39m
```

The `\e[38;5;31m` after the reset is the restore. With no color set beforehand
the same text has nothing to write back — `${(%%)'%b%k%F{242}x%f'}` is the
reset alone — which is what says this is the restore and not the reset.

**Four codes restore and the rest do not**, measured a sequence at a time
against zsh 5.9.2 under `TERM=xterm-256color`. Two of the four are surprises:
bold-*on* restores where underline-on and standout-on do not, and `%u`
restores although its `\e[24m` clears nothing but the underline.

| sequence | drew | |
| --- | --- | --- |
| `%F{red}%Ba` | `\e[31m` `\e[1m\e[31m` a | bold-on **restores** |
| `%F{red}%Ua` | `\e[31m` `\e[4m` a | underline-on does not |
| `%F{red}%Sa` | `\e[31m` `\e[7m` a | standout-on does not |
| `%F{red}a%b` | `\e[31m` a `\e[0m\e[31m` | bold-off **restores** |
| `%F{red}a%u` | `\e[31m` a `\e[24m\e[31m` | underline-off **restores** |
| `%F{red}a%s` | `\e[31m` a `\e[27m\e[31m` | standout-off **restores** |
| `%B%F{red}a%f` | `\e[1m\e[31m` a `\e[39m` | foreground-off does not |
| `%B%F{red}a%k` | `\e[1m\e[31m` a `\e[49m` | background-off does not |
| `%Ba%F{blue}` | `\e[1m` a `\e[34m` | setting a color does not |

So neither "an off code restores" nor "a sequence that resets everything
restores" is the rule: the first writes a restore after `%f`, the second
leaves one off `%u`, and both are wrong about `%B`.

**What a restore writes, and in what order.** Everything still in effect
except the code's own attribute — which the code has just written for itself,
so `%F{red}%B%Ba` is `\e[31m` `\e[1m\e[31m` `\e[1m\e[31m` and never
`\e[1m\e[1m`. The order is fixed rather than the order the text set things in:

    bold, standout, underline, foreground, background

measured both ways round — `%U%S%F{red}%K{blue}a%b` and
`%S%U%F{red}%K{blue}a%b` both restore `\e[7m\e[4m\e[31m\e[44m` — and with
bold in front of standout, from `%B%Sa%u` restoring `\e[1m\e[7m`.

An attribute the code itself cleared is left out, so `%B%U a %u` restores the
bold and not the underline. A color cleared by `%f` or `%k` is left out the
same way, which is why those two are part of this even though they never
restore: `%F{red}a%f%b` ends `\e[39m\e[0m` with nothing after it.

The whole rule in one string, which is the longest probe taken:

```
${(%%)'%F{red}%B%U%Sa%s%u%b'}
\e[31m \e[1m\e[31m \e[4m \e[7m a \e[27m\e[1m\e[4m\e[31m \e[24m\e[1m\e[31m \e[0m\e[31m
```

**Why it matters beyond the bytes.** powerlevel10k writes `%b%k` between every
pair of segments, so a shell that emits the reset alone loses the color of
every segment after the first — eight places in one real `${(%%)PROMPT}`, all
of them a missing `\e[38;5;NNm` or `\e[30m` directly after an `\e[0m` (#2075).

**The state is the shell's, not the rendering's.** It was written down here as
per-expansion and that was wrong — measured on zsh 5.9.2, a `%b` alone in its
own rendering writes back a color a *previous* rendering set:

```
v='%F{070}'; w='%b'
print -rn -- "${(%%)v}"; print -rn -- "${(%%)w}"
\e[38;5;70m  \e[0m\e[38;5;70m
```

It accumulates across every rendering the shell has done — `%F{070}` in one
and `%K{021}` in another are both written back by a `%b` in a third — and what
takes an entry out of it is a code that clears that attribute, so a `%f`
between the two renderings above leaves the reset alone. The single `%` flag,
the double one and `print -P` all share it. A subshell is handed a copy, so
the shell's state reaches a rendering inside one and a rendering inside one
never reaches back out:

| | drew |
| --- | --- |
| `print -rn -- "${(%%)v}"; (print -rn -- "${(%%)w}")` | `\e[38;5;70m` `\e[0m\e[38;5;70m` |
| `(print -rn -- "${(%%)v}"); print -rn -- "${(%%)w}"` | `\e[38;5;70m` `\e[0m` |

This is not an edge of the language. p10k binary-searches its own prompt width
through `${(%%)…}` many times before the prompt is drawn, so the state is
never empty by the time it matters, and a walk that started empty wrote
`\e[0m\e[49m\e[38;5;NNNm` where zsh writes four escapes — one color short at
every segment boundary of the drawn prompt (#2113).

### The colors

The sequences are the terminal's arithmetic rather than the dialect's, so
they are written once for whichever dialect asks: 30 to 37 are the eight
foreground colors, 40 to 47 the same backgrounds, 90 and 100 their bright
halves, 39 and 49 the defaults, `38;5;n` / `48;5;n` everything from 16 to
255, and `38;2;r;g;b` / `48;2;r;g;b` a direct color. Measured: `%F{2}` →
`\e[32m`, `%F{9}` → `\e[91m`, `%F{200}` → `\e[38;5;200m`, `%K{5}` →
`\e[45m`, `%F{#ff8800}` → `\e[38;2;255;136;0m`.

The argument arrives in one of two spellings and they are the same
argument. `%F{30}` is the braced one; `%30F` is a **count** in front of the
code, the shape `%2~` already uses, and it is read exactly as the braces
would be — measured, `%2F` is `\e[32m`, `%200F` is `\e[38;5;200m`, and
`%256F` takes the default just as `%F{256}` does. Where both are written the
braces win, and an *empty* pair of them wins too: `%2F{red}` is red and
`%2F{}` is the first color. Reading the count does not consume what follows
it, so `%30Fx` paints and then draws the `x` (#2087).

**How the braces are read**, from 91 arguments measured in both layers on
zsh 5.9.2 with `TERM=xterm-256color`. Four readings, and three of them are
surprises:

| the argument | read as | measured |
| --- | --- | --- |
| starts with a letter | a **name, by prefix** | `re` → red, `w` → white |
| an ambiguous prefix | the **first** of the eight in the terminal's numbering | `b` and `bl` → **black**, not blue |
| a run of letters that is no prefix | the default | `bogus`, `grey`, `bo`, `x9` → `\e[39m` |
| the name ends at the first non-letter | the letters before it | `red,`, `red bold`, `red;bold` → red |
| anything else | the digits at the front, as C's `strtol` | ` 2` → 2, `+9` → 9, `9x` → 9, `1red` → 1 |
| **no digits at the front** | **nought**, which is the first color | `-`, `+`, `,`, ` `, `0x9`, ` red ` → `\e[30m` |
| a number outside 0…255 | the default | `256`, `-1` → `\e[39m` |
| `#rrggbb`, `#rgb` | a **direct color**, short digits doubled | `#abc` → 170, 187, 204 |
| `#` whose hex run starts badly | nought — no number at all | `#`, `#g`, `#ggg` → `\e[30m` |
| `#` whose hex run starts well and is malformed | the default | `#0`, `#0000`, `#00g` → `\e[39m` |

So an *invalid* color has **two** answers and not one, and which of them
depends on whether the argument looked like the start of a number. An
implementation with a single answer for both is a visibly wrong color on
half of the shapes, at status 0.

The names are lower case and only lower case: `%F{RED}` and `%F{Red}` drew
the default. An empty argument is black rather than the default, since `%F`
and `%F{}` both drew `\e[30m` — which is the no-digits reading above, and
not a rule of its own.

**One thing here depends on the terminal rather than on the shell, and is
deliberately not modeled.** Measured, `%F{9}` drew `\e[91m` under
`TERM=xterm-256color` and `\e[39m` — the default — under `TERM=xterm`,
which reports eight colors; `%F{200}` drew `\e[38;5;200m` and `\e[39m` the
same way round. So an index above seven is answered by asking terminfo how
many colors there are, and below eight it is not. This implementation
models the 256-color terminal unconditionally, which is what every terminal
a person runs a shell in reports and is the wrong answer on a genuinely
eight-color one. Matching it means reading a capability database, which is
a seam this substrate has not got. The `#rrggbb` form is the evidence that
the two questions are separate: it drew the same direct-color sequence under
every `TERM` measured, `dumb` included.

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

## What a failed expansion pass costs

**Measured** 2026-09-12 against zsh 5.9.2 and bash 5.3.15, with a math
function nobody registered as the operand — `$((nofunc()))`, which both
shells call a failure at expansion time and neither refuses at parse time.

A prompt rendering is a **boundary**. An error inside the expansion pass
costs the rendering and nothing else: the diagnostic is written, the command
holding the expansion still runs, the command after it runs, and the shell
exits 0.

```
s='PRE-$((nofunc()))-POST'      v='PRE-$((nofunc()))-POST'
${(%%)s}   (zsh, PROMPT_SUBST)  ${v@P}     (bash)
```

| | zsh | bash |
| --- | --- | --- |
| diagnostic | `<file>:N: unknown function: nofunc` | `<file>: line N: nofunc(): arithmetic syntax error …` |
| the rendering is worth | `PRE-` | `PRE-$((nofunc()))-POST` |
| the command runs | yes | yes |
| the next command runs | yes | yes |
| status | 0 | 0 |

So the boundary is unanimous and only **what a given-up pass hands back**
divides them. zsh keeps what it drew; bash keeps the text it was handed,
with the substitutions simply not performed. That is
`interp.PromptStyle.FailedExpansionKeepsWhatItDrew`.

"What it drew" is the text in front of the **first** substitution, not
everything that had succeeded. With `V=MID` and
`s='PRE-${V}-$((nofunc()))-POST'`, zsh still draws `PRE-`: the `${V}` that
expanded is thrown away with the rest.

The **escape pass is not given up with it.** `s='PRE-%%-$((nofunc()))-POST'`
draws `PRE-%-`, so the table reads what the abandoned expansion left exactly
as it would read a finished one. The same holds for a visual code: with `%B`
in the same position the bold sequence is drawn and then the rendering stops.

`${x?word}` is the one operand that is not caught, and it splits the panel
the way it splits at a sourced file: with `s='PRE-${NOPEV?gone}-POST'`, zsh
reports and **ends the shell** before the command runs, where bash reports,
renders and carries on. That is `interp.Semantics.ParamErrorIsAnExitRequest`,
asked here for the same reason it is asked at a startup file.

`print -P` is the same rendering under another spelling and reaches the same
boundary: `print -P 'PRE-$((nofunc()))-POST'` writes `PRE-`, the line after
it runs, and the status is 0.

Why it is worth a section: a prompt theme's whole `PROMPT` is parameters, and
powerlevel10k's opens with `${$((_p9k_on_expand()))+}` — a math function that
is not resolvable until the theme's own `functions -M` has run. Without the
boundary, "one segment came out empty" is "nothing after this point runs"
(#2053).

## The default is a parameter, not only a fallback

**Measured** 2026-09-07 through a pseudo-terminal — no `-c`, no `-i`, `$-`
recorded in the same run to prove the session was interactive — with `PS1`
unset in the parent, `HISTFILE` on a scratch path, and the platform's global
rc files suppressed where there are any (`zsh --no-globalrcs`; macOS
`/etc/zshrc` sets `PROMPT` and masks zsh's own default with `%n@%m %1~ %# `).

Every column has a default and no two of them have the same one:

| shell | `PS1` | `PS2` | `PS4` |
|---|---|---|---|
| bash 5.3.15 | `\s-\v\$ ` | `> ` | `+ ` |
| bash-as-sh | `\s-\v\$ ` | `> ` | `+ ` |
| bash32 3.2.57 | `\s-\v\$ ` | `> ` | `+ ` |
| dash | `$ ` | `> ` | `+ ` |
| ksh93 | `$ ` | `> ` | `+ ` |
| zsh 5.9.2 | `%m%# ` | `%_> ` | `+%N:%i> ` |

Five values for one question, none of them empty, so this is a **table of
values** rather than an axis: nothing disagrees about *whether* there is a
default. The values live in `PromptStyle.Default` and `DefaultContinued`,
and each dialect's own tests assert its strings whole.

What was missing until #1421 is that these are **parameters**. The drawer had
always fallen back to them when nothing was assigned, so the screen said
`bash-5.3$ ` while `$PS1` was empty — and the most common first line of a real
`~/.bashrc`,

```sh
[ -z "$PS1" ] && return
```

reads the parameter, not the screen. With an empty `PS1` that guard fires in
an interactive session and the whole file is skipped: no aliases, no
functions, no prompt, at status 0 with nothing said.

Three rules go with the value, each measured:

- **Interactive only, in most of the panel.** With nobody to prompt, bash
  5.3.15, bash 3.2.57, bash under argv[0] `sh` and ksh93 all leave `PS1`
  *unset* — on `-c` and on a script file alike — which is exactly what the
  guard detects. dash assigns the same `$ ` it prompts with, and zsh assigns
  the **empty string**: set-and-empty is a third answer rather than a spelling
  of unset, and `${PS1+set}` tells them apart. `PromptStyle`
  says whether separately from what for that reason.
- **Only when the name is not already set.** `PS1='INH> ' bash` reaches the rc
  as `INH> `, and an inherited *empty* `PS1=` reaches it empty rather than as
  the default — a prompt of nothing somebody asked for.
- **Not exported.** With nothing inherited, bash's `export -p` names no `PS1`
  and a child's environment has none. An inherited exported one keeps its
  export attribute.

`--norc`, `--noprofile` and `-f` do not suppress any of this: with no startup
file read at all, every column still has its default in hand.

### When, relative to the startup files

Five of the six have `PS1` before the run-commands file runs. **ksh93 is the
exception**: measured through a pty with `$ENV` naming a file that prints
`${PS1+set}`, ksh93 has `PS2` and `PS4` in hand there and `PS1` *unset*, and
reads `$ ` by the time a prompt is drawn. The value is the same either way and
only the moment differs, which is why it is `PromptStyle.
DefaultsFollowTheStartupFiles` and not a `Semantics` axis.

### What is recorded, and what is not

The corpus records the **shape** — `${PS1+set}` and `${PS1:+nonempty}` — and
not the drawn text. A default prompt holds `\s` and `\v`, so what reaches the
screen is the shell's own name and version and a row holding it would rot on
the next release. The shape is what a startup file's guard actually reads and
is the same on every machine. See `prompt/the-default-prompt-is-a-parameter-at-a-prompt`,
`prompt/the-default-prompt-is-not-a-parameter-without-one` and
`prompt/the-startup-file-sees-the-default-prompt`.

### Standing differences

- **ksh93's non-interactive `PS2`.** ksh93 with nobody to prompt leaves `PS1`
  unset and has `PS2` set to `> `. The table assigns the two together, so the
  answer taken is the one `PS1` gives — `PS2` is left unset there. Recorded
  here rather than fixed, because splitting the entry per parameter would be a
  knob for one column.
- **`PS3`.** ksh93 and zsh set it (`#? ` and `?# `). It is not assigned here
  and not read: `select` writes its own prompt. Measured and recorded; not
  implemented.
- **`PS4` is read but not assigned.** The trace prefix is this parameter in
  every column and has been here since #1454 — see "The trace prefix is a
  prompt" below — but the *default* is still a fallback in the code rather
  than a value written into the parameter. Every column assigns it on every
  route, interactive or not: `+ ` in four of them and `+%N:%i> ` in zsh. The
  difference that leaves is what `unset PS4` does, and it is measured: three
  columns then draw no prefix at all and ksh93 draws `+ ` again, where this
  shell falls back to its dialect's prefix in every case. See
  `xtrace/ps4-unset-is-not-the-default-again`.

## The trace prefix is a prompt

`PS4` decides what `set -x` writes in front of each command, and the value goes
through **the prompt language of the dialect reading it** — which makes the
trace prefix the only route to a prompt escape that needs no terminal, and
therefore the only one the corpus can reach. Every other escape this shell
draws is pinned by a test that builds a session.

### Measured

2026-09-11, `env -i <shell> -c '…'`, macOS 25.5.

| | bash 5.3 | bash 3.2 | dash | ksh93 | zsh 5.9 |
| --- | --- | --- | --- | --- | --- |
| `PS4="XX "; set -x; :` | `XX :` | `XX :` | `XX :` | `XX :` | `XX :` |
| `PS4=; set -x; :` | `:` | `:` | `:` | `:` | `:` |
| `PS4='<\u>'` | `<bhamilton>` | `<bhamilton>` | `<\u>` | `<u>` | `<\u>` |
| `PS4='%n '` | `%n ` | `%n ` | `%n ` | `%n ` | `bhamilton ` |
| `PS4='+$LINENO '`, two lines | `+1`, `+2` | `+1`, `+2` | `+1`, `+2` | `+1`, `+2` | `+$LINENO` twice |
| default `$PS4` | `+ ` | `+ ` | `+ ` | `+ ` | `+%N:%i> ` |

Three readings, and they are the three halves of one question:

- **the escape table is the dialect's own.** bash reads its backslash codes,
  zsh reads its `%` codes, ksh93 has no code for `\u` and drops the backslash
  of an escape it does not know, and dash has no table at all. Those are
  exactly each dialect's `PromptStyle`, so the prefix asks the same value the
  prompt drawer asks rather than growing a second table.
- **the expansion is the same question again.** Three shells expand the value
  at every trace and zsh does not unless a script has turned prompt
  substitution on — `PromptStyle.Expand`, which is a function for this reason.
- **an escape with no answer is the drawer's policy, not a script's.** A prefix
  has to draw something, so a code in no table falls through to
  `PromptStyle.Unknown` rather than being refused by name the way `${(%)…}`
  refuses it.

### bash repeats the first character by indirection

Measured 2026-09-11, bash 5.3.15 and 3.2.57 alike:

```
$ bash -c 'set -x; eval :'          $ bash -c 'PS4="XY "; set -x; eval :'
+ eval :                            XY eval :
++ :                                XXY :
```

An `eval`, a sourced file and a command substitution each add one level; a
function call and a subshell add none, so it counts **text being read again**
rather than the depth of the stack. dash, ksh93 and zsh draw the same prefix at
every depth. `Diagnostics.TracePrefixRepeatsAtIndirection` is the answer, and
`Runner.indirection` is the count.

### Which words the trace quotes

`Diagnostics.TraceQuoting` says *how* a word that needs quoting is spelled.
**Which** words need it is a second question with a second answer, and the
panel does not split the same way on the two: bash and zsh share a
`TraceQuoting` value and disagree about three characters and one position
rule. `Diagnostics.TraceMetacharacters` is the field.

Every quoting shell agrees on whitespace, the quote characters, `$`, a
backquote, a backslash and the operators `| & ; < > ( )`. Past that, measured
2026-09-12 over 36 words handed to `echo` under `set -x`, from a script file,
`env -i PATH=/usr/bin:/bin`. Q means the shell single-quoted it; dash prints
every row bare and is left out. bash 3.2.57 agrees with 5.3.15 on every row.

| word | bash | ksh93 | zsh |
| --- | --- | --- | --- |
| `a*b`, `a?b`, `a[b`, `a]b`, `[1]`, `a{b`, `a}b`, `{a,b}` | Q | Q | Q |
| `~a`, `#a` | Q | Q | Q |
| `a~b`, `a#b` | — | Q | Q |
| `^ab`, `ab^` | Q | — | Q |
| `!ab`, `ab!`, `a!b` | Q | — | — |
| `=ab` | — | Q | Q |
| `ab=`, `a=b` | — | — | Q |
| `a@b`, `a%b`, `a+b`, `a-b`, `a,b`, `a/b`, `a:b`, `-ab`, `a-` | — | — | — |

Three facts come out of that, and the first is why one character set cannot
hold it:

- **`~` and `#` are positional in bash and not in the other two.** bash quotes
  them where they would have begun an expansion or a comment and nowhere else.
  So bash's answer is not a smaller alphabet, it is the same characters under a
  leading-only rule — which is why the field is two strings, `Anywhere` and
  `Leading`, rather than one.
- **`=` is three different answers**: never in bash, leading only in ksh93,
  anywhere in zsh.
- **`^` and `!` split the panel again**, and differently: bash quotes both, zsh
  quotes `^` and not `!`, ksh93 quotes neither.

### The brackets of a test are the one exemption

`[ 1 -lt 2 ]` traces as `'[' 1 -lt 2 ']'` in bash, `[ 1 -lt 2 ]` in ksh93 and
`[ 1 -lt 2 ']'` in zsh. `Diagnostics.TraceBareBracket` is the answer.

It is **not** "a command word is never quoted", and the rows that say so are
worth keeping because each rules out a simpler rule that fits some of the
evidence:

| written | ksh93 | zsh | what it rules out |
| --- | --- | --- | --- |
| `'a[b' x` | `'a[b' x` | `'a[b' x` | a command word is not exempt as such |
| `']' z` | `']' z` | `']' z` | the character is not exempt as such |
| `'[' 1 -lt 2 x` | `[ 1 -lt 2 x` | `[ 1 -lt 2 x` | the `[` is bare with no closer in sight, so it is the word and not the construct |
| `[ -n "]" ]` | `[ -n ']' ]` | `[ -n ']' ']'` | ksh93 exempts only the **final** operand |
| `v='['; $v 1 -lt 2 ']'` | `[ 1 -lt 2 ]` | `[ 1 -lt 2 ']'` | it is the expanded word, not the source |

Writing the brackets already quoted in the source changes nothing in any of the
three, which is the same fact from the other side: the trace is rendered from
the word the shell arrived at.
