# Line editing

What the keys do while a line is being typed at a prompt.

This is the surface a person touches most and the one a specification says
least about: POSIX describes `vi` mode and says nothing about the emacs-style
bindings every shell actually starts in. So all of it here is measurement.

## How this was measured

Under a pseudo-terminal, driving a real shell with its own terminal on one
end and a test on the other:

- `/opt/homebrew/bin/bash` 5.3.15, started `--norc --noprofile -i` with
  `INPUTRC=/dev/null`, so no startup file and no inputrc of the machine's.
- `/opt/homebrew/bin/zsh` 5.9.2, started `-f -i`, so no startup files.
- Both with `TERM=xterm`, `LANG=en_US.UTF-8` and a 24×80 window.

The line that a key sequence produced is read back out of the shell's own
**history file** rather than off the screen. A shell writes the accepted line
there verbatim; the screen has the redraws, the escape sequences and the
prompt mixed into it, and `fc -l` adds a format of its own.

**Type one keystroke at a time.** This is not a nicety. Written as one burst,
bash appears to join a kill onto the kill before it across an insert between
them — `^W`, `Z`, `^W`, `^Y` gives `echo one Ztwo`. Typed at 25ms a
keystroke, the same shell gives `echo one Z`, agreeing with zsh. Pending
input is read by a path that never sees the keystroke in between, so the
first reading measures the harness rather than the shell. Anything measured
in a burst has to be confirmed slowly before it is written down.

## What the two shells agree about

Same key, same line, same result in bash 5.3 and zsh 5.9:

| key | what it does |
| --- | --- |
| `^A` `^E` | to the start of the line, to the end |
| `^B` `^F` | back one character, forward one |
| `^H` `Delete` (0x7f) | delete the character before the cursor |
| `^D` | on an empty line, end of input; otherwise delete the character under the cursor, and nothing at the end of a line |
| `^K` | kill from the cursor to the end of the line |
| `^L` | clear the screen, keeping the line |
| `^T` | swap the two characters around the cursor and step past them; at the end of the line, swap the last two and stay |
| `^Y` | put the last kill back at the cursor |
| `M-b` | back to the start of the word |
| `M-d` | kill forward to the end of the word |
| `M-Delete`, `M-^H` | kill back to the start of the word |
| `M-B` `M-F` `M-D` | the same as the lower-case letters |
| `\e[A` `\e[B` `\eOA` `\eOB` | the previous line of history, the next |

Three more facts about the kill, all measured:

- **Consecutive kills join into one piece**, in the order the text was on the
  line rather than the order it was killed in: `^W^W^Y` on `echo one two`
  gives the line back unchanged. A keystroke that is not a kill starts the
  next one afresh.
- **A kill outlives the line it came from.** A word killed on one line yanks
  back on the next. Only the *joining* is reset by an accepted line.
- **A kill of nothing does not empty it.** `^K` at the end of a line leaves
  the previous kill available to `^Y`.

There is one more disagreement here, and it is deliberately not a field.
`^W`, then `^K` at the end of the line where there is nothing to kill, then
`^W` again, then `^Y`: bash gives `echo one ` and zsh gives `echo one two`, so
a kill of nothing breaks the join in bash and does not in zsh. A field would
have to be answered by every dialect ever added, for a sequence of keystrokes
nobody types. This implementation takes bash's answer for both.

## Where the two shells disagree

Five of them, and all five are on the daily path. Each is a named field on
`repl.EditorStyle`, with bash's answer as the zero value.

| line and key | bash | zsh | field |
| --- | --- | --- | --- |
| `M-b` on `echo /usr/local/bin` | `echo /usr/local/`⎸`bin` | `echo `⎸`/usr/local/bin` | `WordCharacters` |
| `^U` with the cursor at the start of `echo one two` | the line is unchanged | the line is emptied | `KillToStartOfLineTakesTheWholeLine` |
| `^W` on `echo a+b` | `echo ` | `echo a+` | `KillWordBeforeCursorUsesWordCharacters` |
| `M-f` from the start of `echo one two` | `echo`⎸` one two` | `echo `⎸`one two` | `ForwardWordStopsBeforeTheNextWord` |
| `^T` at the start of `echo abc` | the line is unchanged | `ce`⎸`ho abc` | `TransposeAtTheStartSwapsTheFirstTwo` |

### What counts as a word

bash counts letters and digits and nothing else. zsh counts those plus the
contents of its `WORDCHARS`, which is `*?_-.[]~=/&;!#$%^(){}<>` by default.
The variable is a claim; the keystroke is the fact, so each was checked by
pressing `M-b`:

| line | bash | zsh |
| --- | --- | --- |
| `echo foo_bar` | `echo foo_`⎸`bar` | `echo `⎸`foo_bar` |
| `echo a-b.c` | `echo a-b.`⎸`c` | `echo `⎸`a-b.c` |
| `echo a=b` | `echo a=`⎸`b` | `echo `⎸`a=b` |
| `echo a+b` | `echo a+`⎸`b` | `echo a+`⎸`b` |
| `echo a:b` | `echo a:`⎸`b` | `echo a:`⎸`b` |
| `echo a,b` | `echo a,`⎸`b` | `echo a,`⎸`b` |
| `echo a@b` | `echo a@`⎸`b` | `echo a@`⎸`b` |

`+`, `:`, `,` and `@` are in neither list and break a word in both.

A word is made of *characters*, not of cells: `M-b` twice on `echo 日本語 x`
lands in front of the whole of `日本語` in both shells, so three wide
characters are one word.

### The one place bash disagrees with itself

`^W` and `M-Delete` both kill the word before the cursor and bash gives them
different words. On `echo a+b`, `^W` leaves `echo ` and `M-Delete` leaves
`echo a+`. So `KillWordBeforeCursorUsesWordCharacters` is a field of its own
rather than a use of `WordCharacters`: in zsh both keys agree, and in bash
they do not.

## What a terminal sends, and what a shell does with it

The same key is not the same bytes twice. Home is `\e[H` on one terminal,
`\eOH` on the same terminal once a full-screen program has left it in
application cursor mode, and `\e[1~` or `\e[7~` on others; End is `\e[F`,
`\eOF`, `\e[4~` or `\e[8~`.

Measured, with `echo abc` typed first and an `X` typed after the key:

| sent | bash | zsh |
| --- | --- | --- |
| `\e[H` | Home | nothing, nothing typed |
| `\eOH` | Home | nothing, nothing typed |
| `\e[1~` | nothing, nothing typed | **`~` typed into the line** |
| `\e[7~` | **`~` typed into the line** | **`~` typed into the line** |
| `\e[4~` | **`~` typed** | **`~` typed** |
| `\e[8~` | **`~` typed** | **`~` typed** |
| `\e[3~` | delete forward | **`~` typed** |
| `\e[1;5C` | forward a word | **`;5C` typed** |
| `\e[1;3C` | forward a word | **`;3C` typed** |
| `\e[1;2C` | **`C` typed** | **`;2C` typed** |
| `\e[1;5A` | nothing | **`;5A` typed** |
| `\e[5~` `\e[6~` | nothing | **`~` typed** |
| `\ez`, `\e^A` | nothing, nothing typed | nothing, nothing typed |
| `\e[M !!` (a mouse click) | **` !!` typed**, and `!!` then expands to the previous command | **` !!` typed** |

Both shells look a spelling up in a keymap and, when the lookup fails, give
the rest of the sequence back to the reader — which types it. That is the
worst failure a key can have: a key that does nothing is incomplete, and a
key that puts `;5C` in the middle of a command is broken.

**This implementation reads a sequence by its shape instead**, which is what
makes an unknown key impossible to type rather than merely unlikely:

- `\e[` begins a control sequence: any number of parameter bytes (0x30–0x3F),
  any number of intermediate bytes (0x20–0x2F), then exactly one final byte
  (0x40–0x7E). Read to the final byte, then decide what it meant. The
  parameters are a first number and, after a semicolon, a modifier which is
  one more than a bitmask of the keys held down.
- `\eO` is SS3: exactly one byte follows and names the key.
- `\e` and anything else is a Meta key; two bytes, and unknown ones are
  dropped together.
- The **one** input sequence that is not shaped like this is the older mouse
  report, `\e[M` followed by three bytes of button and coordinates. They are
  counted off by hand. The newer encoding puts its numbers in the parameters
  and needs nothing special. A terminal left reporting the mouse by a
  full-screen program that did not turn it off again is not a rare state.

So Home is all four of `\e[H`, `\eOH`, `\e[1~` and `\e[7~`; End is all four
of `\e[F`, `\eOF`, `\e[4~` and `\e[8~`; Delete is `\e[3~`; and a sideways
arrow with Alt or Ctrl held moves by a word, which is bash's answer for both
`\e[1;5C` and `\e[1;3C`. Shift alone moves by one, like the bare arrow.

## Characters wider than one cell

Which characters a terminal draws in two cells is settled in `cellwidth.go`,
and it is the one thing here not settled by running a shell — under a pty
there is no terminal, and asking another shell's editor where it wrapped
reports its width table rather than the truth.

What the editor owes that table is arithmetic:

- **The cursor moves in columns**, so `\e[nC` and `\e[nD` count cells and not
  characters. One `日` to the left of the cursor is two columns.
- **A terminal does not split a wide character across the right-hand edge.**
  It wraps early and leaves the last cell of the row blank. So the row a
  character lands on has to be found by walking the line and adding widths,
  not by dividing the total width by the terminal's. Dividing is right until
  the first line that wraps, and from there every row is out by one, so the
  redraw comes back up to the wrong row and paints the prompt into the middle
  of the command.
- **Editing is by character.** One backspace removes a whole `日`, and one
  `^B` steps over it; the width is the screen's business and not the text's.

## Where it lives

`repl/editor.go` — the read loop, the redraw and `place`, which is the row and
column arithmetic. The prompt's half of that count comes from `drawnPrompt` —
see `docs/spec/prompt.md`, which is the same question asked about the text the
shell was given rather than the text the person is typing. `repl/words.go` — word boundaries, the kills and the
yank. `repl/escape.go` — the sequence reader. `repl/editorstyle.go` — the
five fields, each carrying the line it was measured on;
`dialect/bash/editorstyle.go` and `dialect/zsh/editorstyle.go` hold the
answers.

`vi` mode is not implemented. It is a separate surface with its own modes and
its own key table, and it is not the default in either shell.

## What is still missing

Measured to exist in both shells and not implemented here, so that the gap is
written down rather than looked like an oversight:

- **Undo** — `^_` and `^X^U`, which both shells have and which is the natural
  companion to a kill.
- **`M-.`** — insert the last argument of the previous line, which is among
  the most-pressed keys either shell has.
- **`M-y`** — walk back through earlier kills. This keeps one kill rather
  than a ring, so there is nothing to walk.
- **Incremental history search** — `^R`.
- **Case and other word operators** — `M-u`, `M-l`, `M-c`.
- **A timeout after a bare `\e`.** Pressing Escape and nothing else leaves
  this waiting for the byte that names the key. Both real shells wait a
  bounded time and then treat it as Escape alone.
