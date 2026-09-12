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
- `/bin/bash` 3.2.57, the same way, wherever the answer might be a version's
  and not the shell's. It agrees with 5.3.15 on everything asked of it here,
  including all three of the disagreements added for undo and `M-.`.
- `/opt/homebrew/bin/zsh` 5.9.2, started `-f -i`, so no startup files.
- All with `TERM=xterm`, `LANG=en_US.UTF-8` and a 24×80 window.

The line that a key sequence produced is read back out of the shell's own
**history file** rather than off the screen. A shell writes the accepted line
there verbatim; the screen has the redraws, the escape sequences and the
prompt mixed into it, and `fc -l` adds a format of its own. Each probe line
starts with `:`, so that whatever the keys build is a command that runs and
prints nothing; where the cursor ended up is measured by typing a `Z` after
the key and seeing where it landed in the recorded line.

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
| `M-.` `M-_` | insert the last argument of the line before, at the cursor |
| `^_` `^X^U` | take the last change back |

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

### The last argument of the line before

`M-.` is among the most-pressed keys either shell has: `cd some/deep/path`, then
`ls M-.`. Measured, in both:

- It inserts **at the cursor** and adds no space of its own.
- Pressed again straight away it **walks a line further back**, taking out what
  the press before it put in — so the presses count lines, not words.
- A keystroke in between ends the walk. The next press starts again at the most
  recent line and inserts a **second copy**: `M-.`, `Q`, `M-.` on a history
  ending `: b1 b2` gives `b2Qb2`.
- `M-_` is the same key.
- The last argument is the last **word**, and quotes hold a word together and
  come across with it: after `: p 'x y'` the key inserts `'x y'`, apostrophes
  and space included. A backslash does not — after `: p a\ b` it inserts `b`.
  Trailing whitespace is not a word, and a line of one word gives that word,
  which is the command name.

### Taking a change back

`^_`, and `^X^U` for the same thing. It is the companion to a kill and the
reason a kill is safe to press: `^Y` puts back what the last kill took, and it
covers only a kill — a mistyped word, a completion that chose the wrong name,
an `M-.` that walked one line too far are none of them things `^Y` can answer.

What both shells do:

- A kill taken back comes back **whole and in place**, cursor included: `^U`
  then `^_` on `echo one two` leaves the line as it was with the cursor at the
  end of it.
- A yank is a change like any other, so `^_` takes the yanked text back off.
- A key that left the line as it found it is **not a change**: `^W`, then `^K`
  at the end of the line where there is nothing to kill, then `^_` still brings
  the word back. An undo that appeared to do nothing and had to be pressed
  twice would be worse than no undo.
- The stack **belongs to the line**. `^_` at a fresh prompt does nothing,
  however much was edited on the line before it.

## Where the two shells disagree

Nine of them, and all nine are on the daily path. Each is a named field on
`repl.EditorStyle`, with bash's answer as the zero value. One more field there
is about completion rather than about a key, and `docs/spec/completion.md`
owns it.

| line and key | bash | zsh | field |
| --- | --- | --- | --- |
| `M-b` on `echo /usr/local/bin` | `echo /usr/local/`⎸`bin` | `echo `⎸`/usr/local/bin` | `WordCharacters` |
| `^U` with the cursor at the start of `echo one two` | the line is unchanged | the line is emptied | `KillToStartOfLineTakesTheWholeLine` |
| `^W` on `echo a+b` | `echo ` | `echo a+` | `KillWordBeforeCursorUsesWordCharacters` |
| `M-f` from the start of `echo one two` | `echo`⎸` one two` | `echo `⎸`one two` | `ForwardWordStopsBeforeTheNextWord` |
| `^T` at the start of `echo abc` | the line is unchanged | `ce`⎸`ho abc` | `TransposeAtTheStartSwapsTheFirstTwo` |
| `echo abcdef` typed a character at a time, then `^_` | the line is emptied | `echo abcde` | `UndoTakesBackOneKeystrokeAtATime` |
| `echo one two`, `^A`, `^K`, `^_` | `echo one two`⎸ | ⎸`echo one two` | `UndoRestoresTheCursorToWhereItWas` |
| three lines behind the prompt and four presses of `M-.` | the inserted word comes off the line | the oldest line's last word stays | `LastArgumentStaysOnTheOldestLine` |
| `   ab` in vi command mode, `$`, `I` | ⎸`   ab` | `   `⎸`ab` | `ViInsertAtStartOfLineSkipsLeadingBlanks` |

### What one undo step is

The last two of those are one question each, and the first of them decides the
data structure, so it is worth stating plainly. **In bash one step is one
change and a run of typing is one change; in zsh one step is one keystroke.**

A *run*, not the line: `echo abc`, `^B`, `d`, `^_` leaves `echo abc` in bash, so
a keystroke that is not typing ends the run. Deletes never join — three
backspaces and one `^_` puts one character back in both.

The same answer decides an `M-.` walk. Two presses and one `^_` leaves bash in
front of the first press and zsh at what the first press inserted, which is why
this is one field and not two.

That makes the structure a **stack of snapshots of the line**, pushed before
each change, rather than a list of edits to invert: the snapshot is the same
shape whatever key made the change, and at the length of a command line copying
it costs nothing beside the redraw that follows. The field then only decides
whether a keystroke that continues what the one before it was doing pushes
another snapshot or is covered by the one already there.

### Where the cursor lands after an undo

zsh puts it back where it was when the change was made; bash puts it after the
text the undo has just put back. They agree on a backward kill, where those are
the same place, and part company on a kill that went forwards:

| line and key | bash | zsh |
| --- | --- | --- |
| `echo one two`, `^W`, `^_` | `echo one two`⎸ | `echo one two`⎸ |
| `echo one two`, `^A`, `^K`, `^_` | `echo one two`⎸ | ⎸`echo one two` |
| `echo one two`, `^B^B^B`, `^K`, `^_` | `echo one two`⎸ | `echo one `⎸`two` |
| `echo one two`, `^A`, `^D`, `^_` | `e`⎸`cho one two` | ⎸`echo one two` |
| `echo one two`, `^W`, `^A`, `^Y`, `^_` | ⎸`echo one ` | ⎸`echo one ` |

bash's answer is *computed* rather than recorded: the text the undo put back is
what lies between the common prefix and the common suffix of the line as it is
and the line as it was, and the cursor goes at the far end of it. That
describes every case in the table, and one it does not is written down below.

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

`^X` is the second prefix, and it behaves the same way: measured, `^X` then a
`q` puts nothing in the line in either shell. The only sequence behind it that
means anything here is `^X^U`.

### There is no timeout after a bare Escape, in any of them

This was written down here as a gap on the assumption that both shells wait a
bounded time and then treat an Escape as Escape alone. **Measured, none of them
does**, and the assumption was wrong rather than approximately right:

| what was typed | bash 5.3.15 | bash 3.2.57 | zsh 5.9.2 |
| --- | --- | --- | --- |
| `\e`, then `b` 25ms later | `M-b` | `M-b` | `M-b` |
| `\e`, then `b` 500ms later | `M-b` | `M-b` | `M-b` |
| `\e`, then `b` 1s later | `M-b` | `M-b` | `M-b` |
| `\e`, then `b` 6s later | `M-b` | `M-b` | `M-b` |
| `\e[`, then `A` 3s later | Up | Up | Up |
| `\e`, then Return 2s later | the line is **not** accepted | the same | a newline goes in the line |

So the wait is unbounded in all three, and a shell that gave up on a half-read
key after some number of milliseconds would be the only one that did. There is
also no honest number to pick: every measurement says the wait has no end.

What all three *do* have is a way out, and they have it without deciding to.
Their editors leave the terminal's `ISIG` on, so the kernel turns a `^C` into a
signal wherever the editor happens to be: measured, `\e` then `^C` abandons the
line in all three and the next prompt is a fresh one.

This implementation takes the terminal fully raw — `^C` has to arrive as a byte
for the line to be abandoned without racing a read already in progress — so the
same rescue is written down rather than inherited. **A `^C` part-way through a
key sequence is not a byte of the key: it abandons the line.** That holds after
a bare `\e`, inside a control sequence, between its parameters and its final
byte, after `\eO`, inside an old-style mouse report, and after `^X`.

Two Escapes are the other way out, and it needs nothing: measured, `\e\e` then
`b` types a `b` in both shells, so the second Escape ends the first and the key
after it is an ordinary key. That falls out of reading by shape — `\e` followed
by anything unrecognized is dropped whole — so it was already true here.

A deadline was considered and is not what the code does. It would need one, and
`repl/terminal_unix.go` is why there is no cheap one to reach for: raw mode
takes the descriptor with `Fd()`, which detaches the file from Go's poller for
good, so `SetReadDeadline` on it answers `ErrNoDeadline` afterwards. The
terminal itself could supply one through `VMIN`/`VTIME`. Neither is worth
building for behavior no measured shell has.

**All of that is about emacs mode, and vi mode is the other way round.** There
the Escape has a meaning of its own — it leaves insert — so waiting for the
byte after it is waiting to find out whether a key that already means something
meant something else, and all three shells put a timer on exactly that wait.
Measured: `\e[D` typed as one burst moves the cursor left in bash 5.3.15 and in
zsh 5.9.2 alike, and the same three bytes with 1.2 seconds after the Escape
leave insert mode and are then read as two command-mode keys — `[` on nothing
and `D` deleting to the end of the line. bash calls the wait `keyseq-timeout`
and defaults it to half a second; zsh calls it `KEYTIMEOUT` and defaults it to
four tenths. See the command-mode section below for what this implementation
asks instead, and why it is not a number.

## vi command mode

The second state the editor can be in, where a letter is a motion rather than a
character. `set -o vi` asks for it in bash; `bindkey -v` and `set -o vi` both
ask for it in zsh, and **only the second of those sets the option** — measured,
`bindkey -v` gives a working command mode and leaves `set -o` reporting
`emacs off` and `vi off`. That is why the question repl asks is a dialect's
(`Shell.ViEditing`) rather than interp's editing mode read directly.

Measured 2026-09-12, the same way as everything else here and with one
addition: the cursor is read back rather than reasoned about. Type a line,
press Escape, press the keys under test, then `i` and an `X`, then Return, and
`fc -ln -1` prints the line the shell accepted — so the `X` marks the character
the cursor was on and the rest of the line says what the edit did. Each table
cell below is such a line, with `⎸` in place of the marker. bash 3.2.57 agrees
with bash 5.3.15 on every row asked of it.

### Getting between the two states

| keys | what happens |
| --- | --- |
| `Escape` in insert mode | leave insert, and **step the cursor back one**: `true alpha beta gamm`⎸`a`. At the start of the line it stays. |
| `Escape` in command mode | nothing at all, and in particular no second step back |
| `i` | insert at the cursor |
| `a` | insert after the cursor; at the end of the line that is the end |
| `A` | insert at the end of the line |
| `I` | insert at the start — **and this is the one disagreement**, below |
| Return | accept the line, from command mode as much as from insert |
| `^C` | abandon the line, wherever in a command it arrives |

### Motions

On `true alpha beta gamma` unless another line is named.

| keys | result |
| --- | --- |
| `0` | ⎸`true alpha beta gamma` |
| `^` on `   true alpha beta gamma` | `   `⎸`true alpha beta gamma` |
| `$` | `true alpha beta gamm`⎸`a` |
| `l`, and `space`, from `0` | `t`⎸`rue alpha beta gamma` |
| `3l` from `0` | `tru`⎸`e alpha beta gamma` |
| `l` at the last character | does not move |
| `h` from `$` | `true alpha beta gam`⎸`ma` |
| `h` at the first character | does not move |
| `3|` | `tr`⎸`ue alpha beta gamma` — the column, counted from 1 |
| `w` from `0` | `true `⎸`alpha beta gamma` |
| `3w` from `0` | `true alpha beta `⎸`gamma` |
| `w` at the last word | does not move |
| `b` from `$` | `true alpha beta `⎸`gamma` |
| `e` from `0` | `tru`⎸`e alpha beta gamma` |
| `fa` from `0` | `true `⎸`alpha beta gamma` |
| `Fa` from `$` | `true alpha beta g`⎸`amma` |
| `ta` from `0` | `true`⎸` alpha beta gamma` |
| `Ta` from `$` | `true alpha beta ga`⎸`mma` |
| `fa` then `;` | `true alph`⎸`a beta gamma` |
| `fa`, `;`, `;`, `,` | `true alph`⎸`a beta gamma` |
| a find of a character that is not there | nothing moves, and an operator in front of it takes nothing |

A count goes in front of any of them, and `0` is a digit once one of the other
nine has begun a count: `10l` from the start moves ten characters.

**A word here is not the word the rest of this editor uses.** Every other word
key asks a dialect, because the two shells disagree and one of them disagrees
with itself; the vi motions ask nobody. Three runs: blanks, the letters and
digits and `_`, and everything else. Measured on `true a-b.c def`:

| keys | result |
| --- | --- |
| `ww` from `0` | `true a`⎸`-b.c def` |
| `WW` from `0` | `true a-b.c `⎸`def` |
| `bb` from `$` | `true a-b.`⎸`c def` |
| `BB` from `$` | `true `⎸`a-b.c def` |
| `ee` from `0` | `true `⎸`a-b.c def` |
| `EE` from `0` | `true a-b.`⎸`c def` |

`_` is inside a word — `ww` on `true a_b cd` reaches `cd` — and `$` and `/` are
not. zsh's own `WORDCHARS` holds `-` and `.` and its `M-f` walks straight past
them; its `w` still stops.

### Edits, and the operator grammar

| keys | result |
| --- | --- |
| `x` from `0` | ⎸`rue alpha beta gamma` |
| `3x` from `0` | ⎸`e alpha beta gamma` |
| `x` at the end | `true alpha beta gam`⎸`m` — the cursor cannot stay past the last character |
| `x` with a count past the end of `abc`, from `l` | ⎸`a` |
| `X` from `$` | `true alpha beta gam`⎸`a` |
| `rZ` from `0` | ⎸`Zrue alpha beta gamma` |
| `2rZ` from `0` | `Z`⎸`Zue alpha beta gamma` |
| `r` then Escape | nothing at all |
| `~` from `0` | `T`⎸`rue alpha beta gamma` |
| `3~` from `0` | `TRU`⎸`e alpha beta gamma` |
| `dw` from `0` | ⎸`alpha beta gamma` |
| `d2w`, and `2dw` | ⎸`beta gamma` |
| `dw` on `true   alpha` from `0` | ⎸`alpha` — the blanks go too |
| `dw` at the last word | `true alpha beta gam`⎸`m` — **to the end of the line**, although a bare `w` there does not move |
| `db` from `$` | `true alpha beta `⎸`a` |
| `de` from `0` | ⎸` alpha beta gamma` — inclusive, where `dw` is not |
| `d$` from `l` | ⎸`t` |
| `d0` from `$` | ⎸`a` |
| `dfa` from `0` | ⎸`lpha beta gamma` |
| `dd` | the line is emptied |
| `D` from `l` | ⎸`t` |
| `d` then a key that is not a motion, or then Escape | nothing at all |
| `cw` from `0` | ⎸` alpha beta gamma`, in insert mode — **`cw` is `ce`** |
| `cw` on the last character | `true alpha beta gamm`⎸ |
| `c$` from `l`, and `C` | `t`⎸ |
| `cc`, and `S` | the line is emptied, in insert mode |
| `yw` from `0`, then `$p` | `true alpha beta gammatrue`⎸` ` |
| `yw` from `0`, then `$P` | `true alpha beta gammtrue`⎸` a` |
| `x` then `p` | `r`⎸`tue alpha beta gamma` — the character swap |
| `u` | take the last change back |

A count may go in front of the operator as well as in front of the motion, and
they multiply. What a delete or a yank took is what `p` and `P` put back, and
it is the same buffer `^Y` holds: a word killed with `^W` is a word `p` puts
back, and it outlives the line.

**A key command mode has nothing on does nothing**, and above all does not type
itself into the line: measured, `z` and `q` leave all three shells exactly as
they were.

### Where the shells disagree about it

| line and keys | bash | zsh | where |
| --- | --- | --- | --- |
| `   ab`, `$`, `I` | ⎸`   ab` | `   `⎸`ab` | `ViInsertAtStartOfLineSkipsLeadingBlanks` |
| `0`, `x`, `u` | `t`⎸`rue …` | ⎸`true …` | `UndoRestoresTheCursorToWhereItWas`, the field `^_` already had |
| `0`, `fa`, `;` after a `t` rather than an `f` | does not move | moves to the next one | not a field — below |
| `Y` and `yy`, then `p` | the characters go back on the same line | a new line is opened below | not built — below |
| `o` and `O` | nothing | a new line is opened | not built |
| `U` | the whole line comes back | nothing | not built |
| `^R` | reverse history search | redo | not built |
| `G` | fetches a history entry | nothing | not built |
| `_` | the last argument of the line before | nothing | not built |
| `^K`, `^U`, `^W` in command mode | kill, as in emacs mode | nothing | not built |

### The Escape that is also the first byte of an arrow key

This is the one question a command mode asks that an emacs one does not, and
both shells answer it with a timer — the measurement is in the Escape section
above. **This editor asks whether a byte is *there*, and waits no time at all
for one.** A terminal writes an escape sequence in a single write, so the bytes
after the Escape have already been delivered by the time the question is asked;
a person pressing Escape delivers one byte and nothing follows it.

Two sources, because input reaches the editor two ways: bytes already taken off
the terminal and sitting in the editor's buffer, which `select` cannot see, and
the descriptor itself. Missing the first is the same mistake the descriptor
watcher documents having made once.

No number was invented, and there is no honest one to invent: the two shells
that have the wait disagree about its length. What this costs is a sequence
split across two writes by something slow in between — a link with a stall in
the middle of it — which the shells' timers cover and this does not. #1427
records it.

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

## An editing action the shell performs

A key can be bound to something the editor does not implement: a shell
function, run with the line in front of it and allowed to change it. That is
what every interactive shell with a line editor offers a person, and it is why
the vocabulary of `repl.Widget` constants cannot be the whole story — the
action is code the shell was handed at run time, so there is nothing to
enumerate.

**The capability is the round trip and nothing else.** The editor hands out the
line as it stands, something outside it runs, and the line comes back:
`repl.Line` is the whole exchange — the buffer and a cursor counted in
characters from 0 — and `repl.Shell.RunWidget` is the one seam. A binding is
therefore `repl.Binding`, which is either one of the editor's actions or the
*name* of one of the shell's; a name rather than a callable, because what
running it means is the dialect's. `repl/shellwidget.go` carries the split.

Both halves of the answer are put back in range rather than trusted, since an
action that walked off the end of the line is asking for the end, and the
redraw afterwards is unconditional — an opaque action may have moved the
cursor, rewritten the line, printed something, or none of the three, and there
is no way to tell which.

What each shell calls this is that shell's. `zle -N` defines one in zsh and the
line arrives in `$BUFFER` with the cursor in `$CURSOR`; `dialect/zsh/zle.go`
holds all of it, along with the measurement of what a widget can read and write
and what the four line parameters do to each other.

bash's `bind -x` is the same capability under a different name, and what rides
the seam there is the *command text* rather than a function's name, because
that is what bash binds: `bind -x '"\C-t": some command'`. `dialect/bash/bindx.go`
holds the measurement, taken through a pseudo-terminal against bash 5.3.15.
The line arrives in `$READLINE_LINE` with the cursor in `$READLINE_POINT`,
**counted in characters** — a five-character six-byte line reads 5 — and either
parameter alone is enough to change what the editor draws. The command runs in
the current shell, so what it changes stays changed; its status goes in and
does not come out, so `$?` at the next prompt is what the last *command* left
and not what the key returned; and all of its parameters are gone again by the
next prompt, which `${READLINE_LINE-UNSET}` is what tests for.

Three standing differences from bash, recorded rather than filled in with a
value that would be a lie:

- **`$READLINE_MARK`** is bash's position of the mark, and this editor has no
  mark. Any number here would be one invented rather than measured.
- **`$READLINE_ARGUMENT`** is bash's numeric argument, which bash leaves
  *unset* when no argument was typed. This editor has no numeric argument at
  all, so it is always in the state bash produces itself for a plain keypress.
- **bash exports all of them**, so an external program bound to a key reads
  them out of its environment. Here they are produced parameters, which is what
  makes them vanish again at the end of the call; a shell function reads them
  exactly as bash's are read, and an external program does not see them.

The command runs while the **editor** holds the terminal and not the shell:
`stty -a` from a bound command reports `-icanon -echo` in bash, where the same
`stty` as an ordinary command reports `icanon echo`. That is true here by
construction rather than by arrangement — this editor holds the terminal the
same way for the length of the call — and it is why anything that prints from a
key is printing into a raw-mode terminal.

**A completion widget** — zsh's `zle -C name completer function` — is the same
round trip with one thing taken away. It is how that shell's completion system
installs every widget it owns, so refusing the letter cost a real startup
fifteen lines and left the shell with none of them; `dialect/zsh/zle.go` holds
the definition, the two listing spellings, and the closed set of eight builtin
completion widgets the middle word may name.

What is different at run time was measured through a pseudo-terminal with a key
bound to one, and it is worth stating because it is the half that *refuses*:
inside a completion widget the four line parameters are **read-only** —
`${(t)BUFFER}` reports `scalar-local-readonly-special` where an ordinary widget
reports `scalar-local-special`, and each of the four assignments answers
`read-only variable:` and stops the function. A completion widget looks at the
line and offers candidates; it does not rewrite it. That much is built here.

What is not built is the completion *context* the completer names. zsh gives
such a widget `compstate`, `words`, `CURRENT`, `PREFIX` and the `compadd`
builtin, which is the whole of how candidates are produced and displayed, and
this shell has no completion system for any of it to describe — `zsh/complete`
and `zsh/computil` are both in the roster of modules it declines to load. So a
completion widget defined here registers, lists, aliases, deletes and runs its
function, and the function can read the line; it cannot yet offer a completion.
The editor's own completion is unaffected, because a key left on its default
binding never reaches the widget table.

**A callback on a descriptor** — zsh's `zle -F`, and how a plugin in that shell
does asynchrony — needed this read loop to wait on more than the terminal, and
it has that now: `repl.Shell.WatchedDescriptors` is asked before every wait and
`repl.Shell.DescriptorReady` is called for whichever of them woke. The waiting
is all this package contributes. What a descriptor *means*, which function is
called for it and what the shell calls the command that armed one are the
dialect's, in `dialect/zsh/zlewatch.go`.

One spelling that would need more than this seam is still refused by name
rather than half-built, which is the rule the tree keeps for a gap:

- **An action of the shell's asking the editor to perform one of the
  editor's** — `zle end-of-line` from inside a widget. The editor's actions
  read the terminal and redraw, so running one from inside a call is
  re-entering the read loop rather than transforming the line.

## Work put aside until a time

A shell can be told to run a command later — zsh's `sched`. Nothing about that
is the line editor's, and it is here because the *moment* is: a shell can only
notice that a time has passed at a boundary between commands, a script has no
such boundary, and a prompt is made of them. So `repl.Shell.RunScheduled` is
called once before every prompt, ahead of the prompt hook and in the terminal's
own line discipline, and it is told nothing and hands back nothing — what a
scheduled entry is and what running one means belong to the dialect that has
one.

**One measured difference is not implemented and is written down rather than
papered over.** Driven through a pseudo-terminal, zsh fires an elapsed entry
from the *idle* read: `sched +2` at an untouched prompt runs about two seconds
later with nobody typing, so its wait for a key has a timeout on it. Here the
entry runs at the next prompt. For a person who is typing that is the same
moment or near enough; for an idle terminal it is later, and for one never
touched again it is never. Firing on time needs the same change to how a key is
read that `zle -F` needs, which is why both are named instead of approximated.

## Where it lives

`repl/editor.go` — the read loop, the redraw and `place`, which is the row and
column arithmetic. The prompt's half of that count comes from `drawnPrompt` —
see `docs/spec/prompt.md`, which is the same question asked about the text the
shell was given rather than the text the person is typing. `repl/words.go` — word boundaries, the kills and the
yank. `repl/escape.go` — the sequence reader. `repl/undo.go` — the snapshot
stack and where the cursor lands. `repl/lastarg.go` — the `M-.` walk and the
word it inserts. `repl/shellwidget.go` — the round trip an action outside the editor gets, and
the one seam a scheduled command reaches a prompt through.
`repl/editorstyle.go` — the fields, each carrying the
line it was measured on, and one more that `docs/spec/completion.md` owns; `dialect/bash/editorstyle.go` and
`dialect/zsh/editorstyle.go` hold the answers.

`repl/defaultkeys.go` — the key each of these actions arrives on with nothing
rebound, which is the editor's own dispatch stated as data so that a dialect's
key-listing command has a whole keymap to print without keeping a copy of one.
Its test types every key in the table and compares the accepted line against
the same action reached through the override layer, which is what holds the
table and the switch together; the one hand-written copy that preceded it had
already drifted, and was missing `M-^H` and all four numbered spellings of Home
and End. `dialect/zsh/bindkey.go` and `dialect/bash/bind.go` are the two
vocabularies over it.

`repl/vi.go` — the command mode: the motions, the operator-and-motion grammar
and the count in front of either. It is the editor's and not a dialect's, which
is the same rule `repl/widgets.go` states: a motion is not a vocabulary, and
written in one dialect it would have been written twice. What each dialect
contributes is two questions only a shell can answer — whether this session
edits the vi way, and what somebody bound into the command keymap.

**The keymap is the structural half.** `repl.Keymap` is which of the editor's
two tables is current, and `Shell.KeyBindings` is asked for one. Before that
there was one table and it was the insert map, so a binding written with
`bind -m vi-command` or `bindkey -M vicmd` was stored and could never fire —
both dialects' code said so in their own comments. Measured under a pty in both
shells: `^Xz` bound to `beginning-of-line` in the command map moves the cursor
in command mode and does nothing in insert mode.

`repl.Widget` gained three names and no more: `WidgetViCommandMode`,
`WidgetViInsertMode` and `WidgetViAppendMode`, which both shells already name —
`vi-movement-mode`, `vi-insertion-mode` and `vi-append-mode` in one,
`vi-cmd-mode`, `vi-insert` and `vi-add-next` in the other — and which people
already bind (`bindkey -M viins jk vi-cmd-mode`). The motions are deliberately
not among them: a Widget is what one key does on its own, and a motion is half
of an action whose other half is the operator reading the *range* it names.
`repl/widgets.go` carries the whole of that decision.

`repl/crlf.go` and `Shell.inLineDiscipline` are the two halves of one rule:
**while the editor holds the terminal, nothing may reach it with a newline the
terminal will not translate.** Raw mode turns `OPOST` off, so a bare line feed
moves down without returning the carriage and the next thing drawn starts
wherever the last one ended. The session's own messages take the first half —
they are written through a translating writer. Everything the *shell* hands the
terminal takes the second: the terminal is put back in its own line discipline
for the length of it, and the mode is not taken away again until the output has
arrived.

That second clause is #1356 and it was missing, by two routes. Under a block
store a command's output goes through a pseudo-terminal whose own discipline is
off and a goroutine copies it to the real one, so the command returning and its
last bytes arriving are two events — and raw mode was coming back between them.
That is the *boundary*, and it is held: the copy is waited for before the mode
is taken away.

The other route has no boundary to hold, because a **background job** prints
while a person is typing. There the copy adds the carriage return itself,
deciding by *reading* the terminal's mode rather than by keeping a second copy
of it — which is the same rule the rest of this file follows, and the reason
the test asks the terminal what mode it is in rather than trusting its own
bookkeeping. Measured, a job started with `&` printing six lines at a prompt:
six bare line feeds before and none after, against none in bash 5.3.15 either
way.

**A descriptor handler is a third route, and it takes the terminal back only if
it writes.** A handler is offered its descriptor while the editor waits for a
key, and a descriptor at the end of its input is readable for ever — so one
that never removes itself is called tens of thousands of times a second, which
is what the shell being modeled does with the same arrangement. Restoring for
each of those calls left the terminal in its *own* line discipline for **52%**
of an idle prompt with the streams going straight to it, and **96%** under a
block store where the restore also waits for the copy; that shell never hands
it back for a handler at all, because its editor's raw mode keeps `OPOST` and
`ONLCR` and drops only `ICANON` and `ECHO`. The window is not cosmetic: a `^D`
typed into one is read back as a NUL on Linux and a `^C` becomes a signal
rather than the byte the editor reads, which is a control key at an idle prompt
being closer to a coin toss than to a race (#1463, #1448).

So the restore happens at the handler's **first byte** and not before, and
under a block store not at all — there the copy is already adding the carriage
returns by reading the terminal's mode, which is the paragraph above. Matching
that shell's raw mode instead would end the flap for every handler at once and
is the larger change: the same mode keeps `ISIG`, which turns the `^C` this
editor reads as a byte into a signal.

## Measured, and deliberately not a field

Four places where the two shells differ and the difference is written down
here instead of being answered by every dialect ever added. The precedent is
the `^W`, `^K`, `^W`, `^Y` join above; the test is whether anyone's fingers
would notice.

- **A history recall is undoable in zsh and not in bash.** With `: X` typed and
  Up pressed, `^_` puts `: X` back in zsh and does nothing in bash. Up *and*
  Down and then `^_` agrees again — bash empties the line and zsh takes a
  character off it, which is just the granularity field. bash's answer is taken:
  browsing is not a change. Fielding it would mean deciding what an accepted
  line does to the stack as well, for a sequence that is Up followed
  immediately by undo.
- **`^T` is where bash's cursor rule stops describing bash.** `echo abc`, `^T`,
  `^_` leaves bash at `echo ab`⎸`c` where the prefix-and-suffix rule says
  `echo abc`⎸ — bash records a swap as two changes rather than one. zsh puts the
  cursor back where it was, as it does everywhere. A swap is neither an
  insertion nor a removal and it is not worth a second field to say so.
- **`\e` then Return.** bash swallows it and zsh puts a newline in the line.
  This drops it, which is bash's answer.
- **`;` after a `t` rather than an `f`.** On `true alpha beta gamma`, `0`, `ta`
  leaves the cursor at `true`⎸` alpha …` in both; the `;` after it does not
  move in bash and steps to `true alp`⎸`ha …` in zsh. bash's answer is taken,
  and it is also vi's own. A field would have to be answered by every dialect
  ever added for a pair of keystrokes nobody presses expecting to stay put.

## What is still missing

Measured to exist in both shells and not implemented here, so that the gap is
written down rather than looked like an oversight. Incremental history search
was on this list and is not any more — `^R` is `repl/search.go` and
`docs/spec/history.md`.

- **A numeric argument** — `M-3 M-.`. This is a mechanism rather than a key: in
  both shells the count belongs to every command, so building it for one key
  would be half of it. The two also count in **opposite directions**, measured
  with `: w1 w2 w3 w4` as the previous line:

  | | `M-0` | `M-1` | `M-2` | `M-3` | `M-4` | `M-5` | `M--` |
  | --- | --- | --- | --- | --- | --- | --- | --- |
  | bash 5.3.15 | `:` | `w1` | `w2` | `w3` | `w4` | nothing | `w3` |
  | zsh 5.9.2 | `:` | `w4` | `w3` | `w2` | `w1` | `:` | `w1` |

  bash counts words from the start of the line and zsh counts them from the end,
  with zsh's negative arguments counting from the start instead — coherently,
  where bash's are not (`M--` gives `w3` and `M--1` gives nothing). So it needs
  its own field as well as its own mechanism.
- **`M-y`** — walk back through earlier kills. This keeps one kill rather
  than a ring, so there is nothing to walk.
- **Case and other word operators** — `M-u`, `M-l`, `M-c`.
- **The rest of vi command mode.** What is built is in the section above; these
  are measured to exist and are not:
  - `.`, which repeats the last change, and `U`, which takes the whole line
    back. Both bash-only among the two, and `.` needs the last command
    recorded as a value rather than performed and forgotten.
  - `Y`, `yy`, `o` and `O`. All four are *line*-wise in zsh and need a buffer
    that can hold more than one line before there is anything to put; bash has
    neither `o` nor `O` at all.
  - `/`, `?`, `n` and `N` — a search started from command mode. `^R` is built
    and reaches the same history; this is the other way in, and in zsh the key
    it shares with bash means something else (`^R` is redo there).
  - `%`, the matching bracket, and `G`, which fetches a history entry by
    number in bash and does nothing in zsh.
  - `R`, overwrite mode, and `_`, which inserts the last argument of the line
    before in bash.
  - **What vi *insert* mode restricts.** Measured, the two shells narrow the
    insert keymap when vi editing is on and they narrow it differently: bash
    leaves `^A`, `^E`, `^B`, `^F` and `^K` doing nothing and keeps `^W`, `^U`,
    `^T`, `^Y` and `^_`; zsh keeps `^W`, `^U` and `^L` and lets the rest
    **insert themselves as literal control characters**. Here the insert map
    is still the editor's emacs dispatch, so those keys go on working. That is
    a whole keymap's worth of disagreement and its own change.
