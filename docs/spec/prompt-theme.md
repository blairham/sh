# The prompt theme engine

A prompt drawn from a configuration rather than from a parameter: what
the engine reads, what it emits, and which half of it belongs to a
dialect.

**Governed by:** nothing on either vector. That is the claim this
document exists to keep true — a theme is not a semantic difference
between shells, so no `interp.Semantics` axis and no `syntax.Dialect`
flag decides any of it. What a dialect carries is a *value*, in the
pattern `PromptStyle` and `HookStyle` already establish, and the value
decides what is **nameable from inside the language**, never whether the
prompt exists.

**This is a design spec, and that is unusual here.** Every other entry
under `docs/spec/` records what real shells do, because the construct
existed before this implementation did. A prompt theme has no external
specification: nobody else's engine defines what ours must draw, and
there is no oracle column to compare against. So most of what follows is
**decided**, not observed, and the honest form of a citation for a
decision is the reasoning that produced it. Where a fact *is* an
observation — a terminal's behavior, what this tree already does — it is
cited. Where something has to be measured before it can be implemented,
it is marked **(unmeasured)** and collected at the end.

That distinction is load-bearing. `CLEANROOM.md`'s wall exists to keep
someone else's *expression* out of this tree; a document of our own
design decisions clears it by construction, and a document that quietly
transcribed another project's configuration surface would not, however
the transcription was spelled.

**Decided by the maintainer, 2026-09-19:** this is a clean
implementation with **its own configuration vocabulary**. It is not a
compatibility layer for any existing prompt program, it does not read
any other project's configuration file, and it does not claim any other
project's parameter names. Capability parity is the goal; a shared
namespace is not.

**And decided 2026-09-19, after the above:** the engine must be able to
**import a configuration from another prompt program** — powerlevel10k
first, because it is what the maintainer runs today, then starship — and
**draw what that program draws**. Import writes our vocabulary and is
one-way; nothing at render time reads another project's file. See
*Importing a configuration from another prompt*, which is the section
that turns "looks the same" into something that can fail.

**And the binding constraint, decided the same day:** the engine must be
**expandable and configurable without ever building the shell**. Not
"mostly" — a person who wants a segment this tree has never heard of
must be able to have one, from a file and a session, on a binary they
installed from a release. See *Extending it without building the shell*,
which is the section the rest of this design has to satisfy rather than
a feature listed among others.

## Why this is not in a dialect

A theme engine in `dialect/zsh` would be a prompt that exists only if you
are running the zsh dialect, which is the inversion `AGENTS.md` bars: a
dialect would own a capability the core lacks. The engine therefore lives
in `repl`, which imports no dialect and names a shell only in comments,
and that property has to survive this work.

The consequence is the point of the whole exercise. A script run under
`dash` gets the same prompt as one run under `zsh`, because the front end
draws it and the language never sees it.

### What each dialect decides

Only the names. The engine is driven by the front end, so every row of
this table describes what a *script* can reach, not what is drawn.

| dialect | names it can offer | what it cannot change |
| --- | --- | --- |
| `zsh` | `RPROMPT`/`RPS1`, `precmd`/`preexec` | that the theme draws, or what it draws |
| `bash` | `PROMPT_COMMAND` | the same |
| `ksh` | its own hook spelling | the same |
| `dash` | nothing — it has no name for any of this | the same |
| `ash` | nothing | the same |

A dialect with no name for the right prompt still gets a right prompt.
That is the difference between a capability in the substrate and a
feature of a language, and it is the sentence to re-read whenever
somebody proposes moving a piece of this into `dialect/`.

## The render contract

### Replacing the prompt, not contributing to it

`repl.PromptProvider` today **prepends** to the expanded parameter:
`contributed(false) + prompt("PS1", …)`. A theme has to *be* the prompt,
so the seam gains a second shape — a provider that answers the whole
prompt and suppresses the parameter — rather than a theme pretending to
be a prefix.

Both shapes keep the existing panic guard. A prompt that took the session
down over a decorative segment would be worse than the line that did.

`PS1` is not consulted while a theme is drawing, and is not modified.
Turning the theme off restores whatever the person had, because it was
never touched.

### What crosses the seam is drawn text

The engine emits **terminal bytes with width markers**, never a prompt
language. It does not emit `%#`, `\w`, or anything else a dialect's table
would have to read: the escape tables in `prompt.md` describe what a
*person's parameter* means, and a theme has no parameter. Escape runs are
wrapped in `repl.NonPrinting` so the editor's width arithmetic is not
charged for them.

This is what makes one rendered prompt correct in all five dialects with
no per-dialect pass, and it is why theme markup is read by the engine's
own reader (below) rather than by `interp.RenderPromptValue`.

### Width

Measured with `repl`'s existing cell-width machinery — `cellwidth.go` and
the generated East Asian tables — and **not** a new dependency. This tree
generates its Unicode tables rather than importing them, and the layout
needs exactly one number: the on-screen width of a string with escape
sequences discounted, counted in grapheme clusters so an emoji or a
combining sequence costs its rendered width once.

A prompt that miscounts by one wraps the terminal and smears the frame on
every keystroke, so this is the number to get right before any of the
decoration.

## The configuration namespace

A rich prompt is a lot of settings — a per-segment color, background,
icon, prefix, suffix and visibility, times several dozen segments. The
way to carry that without drowning in plumbing is **one generic
mechanism applied uniformly**: a keyed store plus a lookup, rather than a
Go struct with a field per knob. That is what keeps a preset *data* and
makes adding a segment cost no configuration code.

Settings are named `SH_PROMPT_<KEY>`, matching this tree's existing
`SH_BLOCKS_*` and `SH_WORD_SPLIT`.

They are read **through the shell's own variables, not the process
environment** — `Runner.GetVar`, the rule the block store and `HISTFILE`
already follow, and the reason is the same: a session can set one at the
prompt and mean it. It is also what makes configuration work identically
in all five dialects, since every dialect has variables even when it has
no hooks and no arrays.

- Keys are stored **without** the `SH_PROMPT_` prefix and upper-cased:
  `DIR_FOREGROUND`, `LEFT_ELEMENTS`, `ICONS`.
- A key holds **either** a scalar or a list, never both. Setting one
  clears the other. Either can be read as the other, because dash has no
  arrays and a list still has to be expressible in a session running one:
  **a scalar read as a list is its words**, and a list read as a scalar is
  its elements joined by a space, which is the spelling a list is written
  in everywhere else in this namespace. *(This sentence used to say a
  scalar read as a list is a one-element list. Only splitting makes the
  reason it gave true — a one-element list does not let a dash session
  name three elements — and the configuration file below already spells a
  list that way.)*
- **Set-to-empty is an answer**, distinct from absent: an empty prefix
  and a suppressed icon are both configured states, so the lookup offers
  "is this set at all" separately from "what is it".
- A malformed numeric or truth value **degrades to the default**. A
  prompt is not the place to report a typo by not drawing.

### The three-step chain

Every per-segment setting resolves through the same three steps, most
specific first, then the caller's default:

    SH_PROMPT_<SEGMENT>_<STATE>_<KEY>    DIR_NOT_WRITABLE_FOREGROUND
    SH_PROMPT_<SEGMENT>_<KEY>            DIR_FOREGROUND
    SH_PROMPT_<KEY>                      FOREGROUND

An empty state drops the first step. Hyphens in a segment name become
underscores. This chain is why a preset can set one `FOREGROUND` and have
every segment inherit it while any segment, or any single state of one
segment, overrides — and it is why adding a segment costs no new
configuration plumbing.

### Layering

Later wins over earlier, per key, shallow:

1. the preset (`SH_PROMPT_PRESET`)
2. the configuration file
3. `SH_PROMPT_*` set in the session

A list set by a later layer **replaces** the earlier list outright rather
than merging, because "my elements are these" cannot mean anything else.

The engine records, per setting, which layer it came from, so the
`prompt show` surface can say where a value came from rather than only
what it is.

### The configuration file

Named by `SH_PROMPT_CONFIG`, **named or nowhere**. An unset variable is a
person who has not asked for a file, and inventing a location for them is
what the block store deliberately stopped doing.

The format is the namespace written down, one setting per line, so that
what `prompt show` prints and what the file holds are the same
vocabulary:

    LEFT_ELEMENTS = dir vcs newline prompt_char
    DIR_FOREGROUND = 31
    TRANSIENT = always

The `SH_PROMPT_` prefix is accepted on input and stripped, so a line
copied out of a session works in the file and back again. A `#` begins a
comment and a blank line is nothing.

A value may be wrapped in matching quotes, and that is there for exactly
one reason: a value whose whitespace matters. A separator of a single
space and a suffix of two are both real settings, and unquoted the line
that holds one is indistinguishable from a line that empties it.
Everything else is written unquoted.

A line that is not an assignment, and an assignment whose name is not
spelled the way this namespace spells one, are **named rather than
dropped**. The file is read on the way to drawing a prompt, and a prompt
is not the place to report a typo by not drawing — but the silent half of
that is the failure this repository treats as its worst, so what was read
and not honored is reported.

It is re-read when its mtime changes, so editing it takes effect on the
next prompt and there is no reload command. The stat is once per prompt
rather than once per lookup: that is the cost of an edit taking effect
without a command, and per lookup it would be that cost times the size of
the namespace.

### Colors

Three spellings, because a person writing a configuration reaches for
whichever they know and a preset carrying exact colors needs the precise
one:

| spelling | example | note |
| --- | --- | --- |
| xterm-256 index | `4`, `31`, `255` | rendered as `38;5;N` even for 0–7 |
| 24-bit hex | `#1e66f5` | rendered as `38;2;R;G;B` |
| name | `blue`, `bright-blue` | eight base names, `bright-` prefix |

Indices render as `38;5;N` rather than the terse `30`–`37` range even for
0–7: it is the same color on every terminal that supports either, and one
code path is one fewer thing to get wrong.

An **unrecognized** name resolves to *unset*, not to a wrong color: unset
emits nothing and leaves the terminal's default, which is the honest
rendering of "I did not understand this".

### Markup inside a value

A setting like a frame prefix is one string carrying both text and
appearance — `╭─` in color 242 is one value, not two settings — so values
carry a small markup vocabulary, read by the engine and by nothing else:

    %F{c}   foreground on      %f      foreground off
    %K{c}   background on      %k      background off
    %B %b   bold on/off        %U %u   underline on/off
    %%      a literal %

The spelling is deliberately the one people who configure prompts already
have in their fingers. **Reading it is not interpreting zsh**: it is a
fixed-vocabulary color markup with no expansion, no substitution and no
control flow, and anything outside the vocabulary **passes through
untouched** rather than being guessed at.

Substitution inside a value is limited to `${NAME}` against a
caller-supplied lookup, which is how a segment's content template reaches
the values the segment computed. Command substitution and arithmetic are
not part of the markup; a configuration that wants real shell uses the
route in *A segment can be a shell function*.

The appearance is emitted **in full at every change** rather than as a
delta, so pieces stay correct when the layout concatenates them.

### Icons

`SH_PROMPT_ICONS` picks an icon table: `nerdfont` (the default),
`ascii`, or `none`. Icons live in a table, not in the segments — a
segment that hardcodes its glyph makes `ICONS=ascii` a lie, and makes a
global icon override have nothing to default to.

A table that is not carried is **served by the default and says so**
through `prompt show`. Silently substituting a different glyph set is how
a prompt ends up full of boxes with no explanation.

## Layout

One algorithm, per side of each line:

    for each segment that rendered:
      emit a separator, whose colors are decided by whether this
        segment's background differs from the previous one
      emit whitespace, icon and content in the segment's own colors
    emit the side's end symbol in the last background

With no backgrounds and the separator set to a space, that loop produces
a lean two-line prompt. With a background per segment and the separator
set to a powerline arrow, **the same loop** produces a framed one. This
genericity is why a new look costs data and no code, and it is the single
most important structural decision in the engine.

- `newline` in an elements list splits the side into lines; the number of
  lines is the larger of the two sides' counts.
- The **last line is the one being typed on**. Its right side becomes a
  true right prompt, so the editor can hide it when the typed line grows
  into it rather than baking it into text that would then wrap.
- Earlier lines are banner lines: their right side is placed by filling
  the gap to the measured width, and they end where their content ends.
- A **single space** separates the end of the left prompt from what the
  person types, as a fixed suffix on the line being typed on. It is not
  routed through any whitespace setting, because a setting that empties
  it produces a prompt that cannot be read.

### The settings the loop reads

Written down because the loop is generic and the names are the whole of the
configuration surface — a reader who knows these knows every look the engine
can draw.

| setting | what it decides |
| --- | --- |
| `LEFT_ELEMENTS`, `RIGHT_ELEMENTS` | the elements of each side, `newline` splitting the side into lines |
| `LEFT_START_SYMBOL`, `RIGHT_START_SYMBOL` | what opens a side, in the first segment's background |
| `LEFT_SEGMENT_SEPARATOR`, `RIGHT_SEGMENT_SEPARATOR` | what goes between two segments whose backgrounds differ, drawn from one into the other |
| `LEFT_SUBSEGMENT_SEPARATOR`, `RIGHT_SUBSEGMENT_SEPARATOR` | what goes between two segments sharing a background, so the boundary is a hairline |
| `LEFT_END_SYMBOL`, `RIGHT_END_SYMBOL` | what closes a side, trailing the last background into the terminal's own |
| `FIRST_PREFIX`/`FIRST_SUFFIX`, `MIDDLE_*`, `LAST_*` | the frame around a line, differing for the first line, the last, and any between |
| `GAP_CHAR`, `GAP_FOREGROUND` | what fills a banner line between its two halves |
| `ADD_NEWLINE` | a blank line above the prompt |
| `CONTINUATION` | the prompt for the rest of an unfinished construct |

Every one of the separators and the frame settings also resolves through the
three-step chain, so a single segment may carry its own separator without a
preset having to give every other segment one.

Per segment, through the chain: `FOREGROUND`, `BACKGROUND`, `BOLD`,
`UNDERLINE`, `WHITESPACE`, `PREFIX`, `SUFFIX`, and `CONTENT`.

### The content template is the configuration's, not the segment's

`CONTENT` defaults to `${ICON}${CONTENT}` and is the **only** place a
segment's output is composed. That is what keeps a segment's text *text*: the
template is markup and is read as markup, and what the segment computed
arrives through `${CONTENT}` and is never expanded. A directory holding a
percent sign is drawn rather than read, and a segment does not have to know
that a `%` means anything.

It is also where an imported configuration's content expansion lands, which is
why it is a setting rather than a property of the segment.

### Markup inside a segment returns to the segment

`%f` and `%k` mean *back to what this segment is drawn in*, not back to the
terminal's own. Anything else knocks a background out from underneath a value
that colored one word of itself, which is a framed prompt with a hole in it.
Outside a segment — a frame prefix, say — the base is the terminal's own and
the two readings coincide.

### The right prompt is new here

Nothing in this tree draws a right prompt today. `RPROMPT`/`RPS1` appears
only in `dialect/zsh/promptnames.go`, as the name-aliasing measurement,
and no drawing code reads it.

Building it in `repl` is therefore a capability addition and not a theme
detail: bash, ksh, dash and ash get a right prompt they have never had,
and the zsh dialect gets to *name* the one the substrate draws.

#### Measured, because zsh is the only place the answers exist

zsh 5.9.2 driven through a pseudo-terminal, 2026-09-19, with `zsh -f -i`
on a scratch `HOME` and `ZDOTDIR`, `TERM=xterm-256color`, `LC_ALL=C`,
`PS1='L> '` (3 cells). The line is grown a character at a time and
repainted with `^L`, which makes zsh draw the whole prompt again, so what
is being read is what it *would* draw rather than what happens to be left
on the screen.

**It ends one cell short of the right edge.** The bytes are the whole
answer — 40 columns, `RPROMPT='RIGHT'`, nothing typed:

    L> \x1b[K\x1b[31CRIGHT\x1b[36D

The cursor is at column 4 after `L> `, forward 31 puts it at 35, and
`RIGHT` fills 35 through 39 of 40. **Column 40 is left blank.** With ten
characters typed it is `\x1b[21C` from column 14 — the same column 35 —
so the right prompt is placed against the right-hand edge and not
relative to the line.

**It is drawn while the line is at least one blank cell short of it**, and
that threshold is exactly `columns − right − 2`. Six configurations, and
the widest left-plus-typed width that still keeps it:

| columns | right prompt | widest line keeping it | `columns − right − 2` |
| --- | --- | --- | --- |
| 40 | 5 | 33 | 33 |
| 40 | 1 | 37 | 37 |
| 40 | 10 | 28 | 28 |
| 30 | 5 | 23 | 23 |
| 80 | 5 | 73 | 73 |
| 20 | 3 | 15 | 15 |

The two cells are the blank column at the right edge and one blank column
between the line and the right prompt. **A terminal too narrow for both
sides is the same rule and not a special case**: at 7 columns with a
5-cell right prompt the threshold is 0, the 3-cell `PS1` already exceeds
it, and nothing is drawn — measured, not derived.

**It is not redrawn per keystroke, and it is erased by the line's own
erase.** Growing the line one character at a time, the keystroke that
crosses the threshold is the only one that carries anything with it:

    to 29 columns:  "x"
    to 30 columns:  "x"
    to 31 columns:  "x\x1b[K"        <- crosses; the erase takes it off
    to 32 columns:  "x"

So zsh draws it once, leaves it on the screen while the line grows under
it, and loses it to the `\x1b[K` that the crossing redraw writes anyway.
**It comes back on its own** when the line shrinks back below the
threshold, with no repaint forced.

**zsh leaves it in scrollback.** Accepting a line that is showing one
writes `\x1b[?2004l\r\r\n` and nothing else — no erase of the row, no
repositioning. The right prompt of every command stays on the screen
above its output.

#### What we do with that

The first three are behavior to reproduce, and the formula is the
implementation: draw the right prompt at `columns − width` when
`left + 1 + width + 1 ≤ columns`, and otherwise draw nothing. The banner
lines' gap fill already refuses to place a right half it cannot fit, and
this is the same refusal one row down.

The last one is a **deliberate departure**, and it is the reason this
document asked the question. A right prompt left in scrollback is a
fragment: the row it is on was laid out for a terminal of the width it had
at the time, so a resized window smears it, and it is decoration that
nobody reads twice attached to output people do. So ours is erased on the
row the line was accepted on — which is work the transient prompt is doing
anyway, and is why the two are one change rather than two.

### Transient prompt

    SH_PROMPT_TRANSIENT = always | same-dir | off

Once a command has run, its prompt collapses to the bare prompt
character, so scrollback is output rather than twenty copies of a
two-line frame. Of everything in this document, this is the setting most
responsible for a rich prompt staying usable over a long session.

`same-dir` is inverted by necessity: the trim happens when the line is
accepted, **before** the command has run, so nothing can yet know whether
it will change directory. A prompt is therefore trimmed unless the
*previous* command moved, which leaves one full prompt as a landmark just
below the `cd` rather than just above it. The difference is stated in the
documentation rather than hidden.

#### Built, 2026-09-19

`repl.Shell.Transient` is a `func() string` asked as each line is
accepted: an empty answer, or none wired, leaves the prompt as drawn.
**The `always` / `same-dir` / `off` policy stays above `repl`**, because
`same-dir` has to know whether the previous command changed directory —
shell state, which this package does not read. `repl` asks; the front end
answers. That is the `PromptProvider` bargain and it is what keeps the
package free of any dialect.

The trim reaches further up than a redraw does. `redraw` returns with
`\r`, goes up as far as the line came down, and rewrites from the
prompt's *last* row — the rows above are written once and left alone,
which is what stops a two-row prompt laddering. A trim is the one case
those rows must not be left alone, so it goes up past them
(`e.row + leadRows(prompt.lead)`) before erasing to the end of the
screen.

It returns the prompt now on the screen, and that return is the whole
interface: `toLastRow` counts rows from the prompt's width and can only
count correctly against the width the screen is actually wearing.

#### Where the redraw goes, settled against the code rather than guessed

Transient redrawing is a `repl` capability for the same reason the right
prompt is, and this document said it was the piece most likely to
interact badly with the blocks store and with `groundForPrompt`. Read,
2026-09-19, and it is less entangled than that:

- **`editor.endLine` is the seam.** It already does the three things in
  the order a trim needs: redraw the line if the accept came out of a
  paste and it was never drawn, `toLastRow` to get below a wrapped line,
  then `before + "\r\n"`. A transient redraw is one more step *in front*
  of that — re-render with the trimmed prompt, repaint the prompt's rows,
  and let the existing `toLastRow` do the counting, which it can only do
  correctly once the rows it counts are the rows on the screen.
- **`groundForPrompt` cannot collide with it**, because it runs at the
  top of the *next* `readLine`, after the command. The two are on
  opposite sides of the command's own output, and there is no ordering
  between them to get wrong. What *is* on the same side is
  `markUnfinished`, which pads a partial row with `cols − mark` spaces
  and depends on where the cursor is — so the trim goes before
  `toLastRow` and never between `toLastRow` and the mark.
- **The blocks store writes nothing to the screen.** Its marks are the
  conduit's own, in band on a captured stream rather than on the
  terminal, so a trimmed prompt is invisible to it. What it records is
  the command's facts, and a prompt collapsing after the line was
  accepted changes none of them.

The remaining care is the one `drawnPrompt` already documents: a prompt
with a newline in it is drawn in two pieces, and only the last row is
rewritten on a redraw, because `\r` returns to the row the cursor is on.
A trim that shortens a two-row prompt to one row therefore has to move up
and erase rather than rewrite in place — which is the same arithmetic
`toLastRow` does, in the other direction.

## Segments

**The rule, which is where the speed comes from:** a segment may read the
context it is given, shell and environment variables, and files. It may
not fork, may not dial, and may not block. Anything needing a subprocess
or a network round trip arrives pre-computed and possibly stale, or does
not arrive.

This is the whole difference between a prompt that is drawn and a prompt
that is *computed*. A segment that runs `node --version` to show a
version has moved a process spawn onto the path between pressing return
and seeing a prompt; reading the nearest pin file answers a different and
usually better question — what the project is pinned to, rather than what
happens to be on `PATH`.

A segment may **decline to render**. Layout is a separate pass over
whatever survived, which is why an absent tool costs no space rather than
an empty box.

### The context a segment is allowed to know

Working directory, home, user, host, the last pipeline's exit status and
duration, job count, terminal width, whether this shell is root, whether
the session is remote, the render clock, a variable lookup, the previous
prompt's directory, and repository status when somebody has computed it.
`repl.PromptInfo` already carries the status, duration, job count and
directory; the gaps are width, root, remote, and the previous directory.

Filesystem lookups that walk upward for a marker file are **memoized for
the life of one render**, because a dozen version-manager segments each
walking independently is a hundred stats per prompt. One render is the
memo's whole lifetime, so nothing is ever served stale.

### The roster, which is a convenience and not the mechanism

The segments compiled in are the ones common enough that everyone would
otherwise write them. They are **not** a privileged class: a built-in
segment may use no fact and no capability that a segment arriving from
outside the binary cannot also have. The moment a built-in needs a
private interface, the extension path has become second-class and the
constraint above is broken.

The first set, which is what a prompt is actually looked at for:

    dir  vcs  status  command_execution_time  background_jobs
    context  prompt_char  time  newline

Then, in rough order of how often they earn their space: the version
managers (each a pin-file read), the cloud and cluster context segments
(each an environment variable or a config file), the shell-state segments
(virtualenv, nix shell, direnv), and the system metrics.

System metrics stay inside the no-fork rule — load, swap, disk usage and
interface addresses are files or one syscall. Availability is **per
metric, not per segment**: some readings need cgo on some platforms and
therefore render nothing there, which is what an absent reading should
look like. Interface enumeration costs more than a whole plain prompt, so
it reads a short-TTL cache rather than the kernel per prompt.

### What the first set reads, beyond the settings every segment has

Built rather than proposed, so the names are written down: these are on top
of the per-segment `FOREGROUND`, `BACKGROUND`, `BOLD`, `UNDERLINE`,
`WHITESPACE`, `PREFIX`, `SUFFIX`, `ICON` and `CONTENT` every segment
resolves through the chain.

| setting | segment | what it decides |
| --- | --- | --- |
| `DIR_MAX_DEPTH` | `dir` | how many trailing components survive; `0`, the default, is no truncation |
| `DIR_TRUNCATION` | `dir` | what stands in for what was dropped, `…` by default |
| `STATUS_OK` | `status` | whether the segment draws after a success; off by default |
| `COMMAND_EXECUTION_TIME_THRESHOLD` | `command_execution_time` | seconds below which nothing is drawn, `3` by default |
| `COMMAND_EXECUTION_TIME_PRECISION` | `command_execution_time` | digits after the seconds, `0` by default |
| `BACKGROUND_JOBS_ALWAYS` | `background_jobs` | whether a zero is drawn; off by default |
| `CONTEXT_ALWAYS` | `context` | whether an ordinary local session draws one; off by default |
| `TIME_FORMAT` | `time` | a POSIX date format, `%H:%M:%S` by default |

Three of those defaults are the same decision written three times, and it is
worth naming once: **a segment whose answer is the same every day declines**.
The status after a success, the job count when there are none, and the
context of an ordinary local session are all facts the person already has, so
each is off until a configuration asks — and each asks through a setting
rather than through a second element name, because a segment that draws under
two names is two things to configure.

The clock's format language is **the interpreter's own strftime**, which is
what `printf '%(fmt)T'` writes through. One reader of a format language, for
the reason that function is exported at all: two would drift the first time a
conversion was fixed in either, and no test on either side could see it.

A segment offers what it computed to the content template by name —
`${FULL}` and `${LAST}` beside the directory's `${CONTENT}`, `${USER}` and
`${HOST}` beside the context's, `${CODE}` beside the status's. That is the
`${NAME}` substitution above, and it is what keeps the arrangement the
configuration's rather than the segment's.

### Icons, as carried

Three tables: `nerdfont`, which is the default, `ascii`, and `none`. They are
written in the configuration file's own format and read by its own reader,
which is what makes "a shipped table and a downloaded one are the same
format" checkable rather than asserted — a compiled table that were a Go map
would be an arrangement a downloaded one could not express.

`nerdfont` carries a glyph only where the codepoint is one of the
long-standing positions a font patch fixes. Where it is not, the entry is the
`ascii` spelling rather than a guess: a glyph this tree is not sure of is a
box on somebody's screen, and a box says nothing about what the segment is.

A configuration may name a glyph itself through `ICON`, which resolves
through the same three-step chain — so the bare key is a global override, and
setting one to empty suppresses a single icon without turning the table off.

### An element with no implementation says so by name

A configured element that has no segment renders **nothing**, and is
named under `not yet` by `prompt show`. The same convention applies to a
setting that is stored faithfully and not acted on: a setting has three
states — absent, honored, and *set and ignored* — and the third is the
one that is invisible by default. The person configured something, the
tool reported it, and the prompt quietly did something else.

That list is written down in the code and shrinks as things are
implemented. It is the same silent-wrong-answer rule the rest of this
repository holds to, applied to a prompt.

## Extending it without building the shell

This is a requirement, so it gets a section rather than a paragraph.
**Every part of a prompt must be reachable from outside the binary**:
the look, the settings, and the segments themselves. A person on a
released binary who wants a segment nobody here has thought of must be
able to have one.

Four layers, cheapest first. Each is complete on its own — nobody has to
reach the next one to get what they want.

| layer | what it adds | what it costs | who it is for |
| --- | --- | --- | --- |
| variables | any setting, any dialect | nothing | "make the directory blue" |
| a file | a whole look, shareable | nothing | "use my preset on every machine" |
| a shell function | a new segment | one in-process call | "show me the thing my project has" |
| a plugin | a segment that does real work | a process, asynchronously | "read my cluster / my daemon / my API" |

### Presets and icon tables are files

A preset is a set of assignments and an icon table is a lookup, so both
are **data, loadable from a path**, not entries in a compiled map. A
look someone publishes is a file you point a variable at; a glyph set
for a font this tree has never seen is the same.

The compiled-in presets are seeded from exactly the same format, so
there is no arrangement a shipped preset can express and a downloaded
one cannot.

### A segment can be a shell function

The interpreter is in this process. So a segment's content may be
produced by **a shell function in the session**, called in-process with
no fork, and this works in whatever dialect the session is running — a
bash function in a bash session, a zsh function in a zsh one. Define it
in your startup file, name it in an elements list, and it draws.

This is a capability no external prompt program has, and it is the
everyday answer to the constraint: the common case for "a segment that
does not exist yet" is a few lines of shell over a file or a variable,
and that should cost a person nothing but their own rc.

It is also the escape hatch that keeps the no-fork rule honest. Someone
who genuinely needs to run something gets to, deliberately and visibly,
instead of the engine growing a forking segment for every tool in the
world.

Constrained accordingly:

- **Off unless configured.** The default configuration calls nothing,
  and `prompt show` names every function a configuration installs, so
  the cost is visible rather than discovered.
- **A function that fails or is missing costs its segment and nothing
  else.** The provider's panic guard covers a panic; a non-zero return
  and an unset name are ordinary outcomes and render nothing.
- **A function that does not terminate is the hard case**, and the
  honest answer is the async contract below rather than a timeout
  invented here **(unmeasured — settle before implementing)**.

### A segment can be a plugin, and it is async by construction

Some segments have to do real work: query a cluster, ask a daemon, read
something over a socket. Those belong outside this process, and
`docs/design/plugins.md` already has the transport, the handshake, the
trust model and the failure semantics for that. What it does not yet
have is a role for this, and the role it needs is a **third** one beside
command and observer.

The exclusion that role has to answer is the hot-path one, and the
answer is in two parts.

**First, a prompt is not per-keystroke.** `repl.Shell.prompt` fires
hooks and checks the window size, so it runs once per prompt line.
#1323's reasoning to the contrary is the thing this document corrects.

**Second, and this is what makes it unconditional: a plugin segment
never blocks a prompt.** It does not answer a request while the shell
waits — it **publishes**, and the prompt draws with what it has and is
redrawn when something new arrives. So the process boundary is not on
the path between pressing return and seeing a prompt, at all, ever, and
the latency of a slow or wedged plugin is bounded by nothing because it
is not being waited on.

That is the same mechanism #1314 needs for repository status, which is
the point: **one async contract, two users**, rather than a bespoke
timeout for the prompt.

Consequences worth stating before the role is written:

- A plugin segment's first draw usually shows nothing, and what it shows
  instead is the per-segment question already open above.
- A plugin that dies loses its segments, and they render nothing. The
  prompt does not report a dead plugin on every line; the host's
  existing failure reporting does it once.
- A plugin declares its segment names at the handshake, the same way the
  command role declares its command names, so a name collision is
  **reported rather than silently resolved**.

### How an element name resolves

An element in a configuration is a name, and the name is looked up in
one order, most specific first:

1. a shell function in the session
2. a segment declared by a plugin
3. a segment compiled in
4. nothing — and `prompt show` names it under `not yet`

The session wins because it is the most local thing and the person
writing it is present. A collision at any step is **named** rather than
quietly resolved — a person whose segment stopped drawing because a
release added a built-in of the same name has been silently overruled by
their own shell, which is the failure class this repository treats as
its worst.

## Importing a configuration from another prompt

A person arriving here has a prompt they already like, and often a
configuration file they have been tuning for years. The engine must take
that file and draw what it drew.

**This is a converter, not a compatibility surface**, and the difference
is the whole reason it does not reopen the vocabulary decision above:

- import runs **once**, reads the other project's file, and writes
  **our** settings into **our** configuration file;
- nothing on the render path ever reads another project's file, name or
  format;
- the imported result is an ordinary configuration afterwards —
  editable, shareable, and indistinguishable from one written by hand.

### The claim is testable or it is not made

"It looks the same" is an opinion until something can fail. So fidelity
is defined as a comparison against the real program, and it needs an
instrument in the family of `make oracle`, `make acp` and `make
sandbox`: drive the real prompt, drive ours, compare.

**Compare a cell grid, not a byte string.** Two different SGR spellings
paint the same screen — `38;5;31` and `31` are the same color, and an
attribute can be set in either order — so a byte diff fails on prompts
that are identical to look at. The comparison is over **rendered cells**:
per cell, the grapheme, foreground, background and attributes. That is
what "looks exactly the same" means, stated so a machine can check it.

The context has to be pinned on both sides or the diff is noise: working
directory, repository state, exit status, command duration, job count,
terminal width, and the clock. Every one of those is an input to some
segment.

Rules this instrument inherits from the ones already here:

- **Never strip ANSI.** A harness that compares plain text cannot tell a
  working theme from a colorless one, which is the blind spot that has
  hidden broken rendering in this tree before.
- **Drive it through a pty with a multi-row prompt.** A one-row `PS1`
  cannot catch a frame that is correct in its pieces and wrong in its
  nesting.
- **Fixture against the real configuration**, not a reduced one written
  to pass. A generated 1,720-line file with 280 settings is the case that
  matters; a ten-line file proves nothing about it.

### powerlevel10k: evaluate the file, do not parse it

The file is a zsh program, and this shell has a zsh dialect. So import
**sources it in a zsh-dialect runner and harvests the parameter
namespace** rather than pattern-matching assignments out of the text.

That is not a refinement. Two shapes in a real generated configuration
defeat a text reader outright:

- **The version gate the file opens with.** A range inside a group,
  `[[ $ZSH_VERSION == (5.<1->*|<6->*) ]]`, decides whether any of the
  configuration is applied at all — and it is the construct #1217 was
  filed for.
- **Names built by brace expansion.** One line,
  `POWERLEVEL9K_PROMPT_CHAR_{OK,ERROR}_VIINS_CONTENT_EXPANSION='❯'`,
  is **two** settings. A text reader has to reimplement brace expansion
  to see them; an evaluator gets them for free.

A parser would have to become a zsh interpreter to be correct. We have
one, and it is in the process.

### Content expansions are the case that decides "most" from "all"

A generated configuration points a segment's content at shell code. The
real one on this machine reads:

    POWERLEVEL9K_VCS_CONTENT_EXPANSION='${$((my_git_formatter(1)))+${my_git_format}}'

That calls a shell function for its side effect, in arithmetic context,
and uses the `${…+…}` form to substitute the variable the function set.
There is no reading of that which is not "run this shell code".

Honoring it means calling the function, which costs no fork here, and
which the engine already allows through *A segment can be a shell
function*. Import therefore carries the function definitions it finds
alongside the settings, and the segment calls them.

Where a configuration's code cannot be carried, the setting is **named
at import** rather than dropped silently or half-interpreted into
something that looks nearly right.

### starship: a second converter, not the same one

starship's configuration is TOML with a different model — per-module
`format` and `style` strings, a `$fill`, and modules rather than a flat
namespace — so it is a separate converter that shares the fidelity
harness and the output format.

It is second because powerlevel10k is what is in use. It is worth doing
because the two together are most of the themed prompts in the world, and
because one of them being a hand translation of the other is a fixture
we already have: the same prompt from two sources is a stronger test than
either alone.

### What cannot be identical, said before anyone is surprised

- **Repository counts** until #1314 lands: the branch is drawn
  synchronously, the counts arrive or do not. The upstream ships a daemon
  for this and its prompt waits for it; ours does not wait.
- **Anything the source computes by forking.** starship runs
  `node --version` to fill a version module; no segment here forks. The
  pin-file segments answer a neighboring question and are **not**
  substituted in silently, because "what this project is pinned to" and
  "what is on `PATH` right now" are different answers.
- **Instant prompt**, which has no counterpart: it exists because the
  real prompt is not ready for tens of milliseconds, and it is accepted
  and reported rather than implemented.

Each of these is a line in the import report, not a footnote in a
document nobody reads at the moment they hit it.

## Repository status

Split by what it costs:

- **The branch** is a handful of stats and one small read, done
  synchronously, cached on the mtime of the files that would change it.
  It is what people navigate by and it must never be late.
- **The counts** (modified, untracked, ahead, behind) need a full index
  versus working-tree walk. That is the expensive half, and it is the
  reason every fast prompt in existence either caches it or hands it to a
  separate process.

A prompt with no scanner attached shows the branch and no counts. **It
never waits for either.**

#1314 owns the capability half of this — a resident per-repository cache,
watch-based invalidation rather than polling, background refresh, and the
same cache serving completion. Two rules from it belong here because they
constrain what the prompt may draw:

- Behavior where watches are unavailable must degrade to something
  honest, never to a silently stale answer. **A prompt that confidently
  shows the wrong branch is the worst failure this repository has.**
- What a not-yet-ready segment draws is a behavior question to settle,
  not to invent. Options are: nothing, the previous value marked stale,
  or a placeholder. Settle it against a measured prompt, and write the
  answer here.

## Async segments

The existing provider contract is **synchronous and deadline-free by
design**, and `repl/promptprovider.go` states why: a deadline reports
something other than what happened, and abandoning the work leaks the
goroutine doing it. A theme does not get to quietly weaken that.

So async is a contract change with three parts, and none of them is a
timeout on the in-process seam:

1. a segment declares it has no answer *yet* and the prompt draws without
   it;
2. whoever computes the answer publishes it, and the prompt is **redrawn
   in place** — which is a `repl` capability the transient prompt needs
   anyway;
3. what is drawn in the meantime is stated per segment, per the rule
   above.

### Built, 2026-09-21

`repl.PromptPublisher` is the second shape of the seam, and the sentence
that separates it from the refusal above is short: **a provider is asked,
and a publisher tells.** Nothing is waited for, so nothing is abandoned
and no deadline has to report something other than what happened.

A session hands a publishing theme one function at the start and takes it
back at the end. Calling it says *a redraw would differ*; it is not a
request for one, and the session decides — a prompt that renders the same
is not written to the screen at all. That is what makes a publisher which
cannot tell whether anything changed a cheap thing to be rather than a
flickering one.

**The wake is a descriptor because of where the session waits.** An editor
between keystrokes is blocked in `select(2)` on the terminal and on
whatever the shell armed, and a Go channel cannot wake that. So `repl`
owns a pipe for the length of a session and hands a *function* out for it;
nothing outside `repl` learns a descriptor is involved, which is what keeps
the engine free of the editor. It goes into the same set `zle -F`'s
descriptors go into, because there is one place in a shell that is idle
with a descriptor in its hand and that is it.

**The redraw goes up past the rows a keystroke never touches.** The leading
rows are written once — every redraw after that rewrites the last row
alone, which is what stops an Up arrow leaving a ladder of prompts behind
it — and a segment on the upper row is exactly the case those rows are not
left alone in. It is the same arithmetic `trimPrompt` does for the
transient prompt, in the same direction, and it is why the test for it is
driven through a pseudo-terminal with a **two-row** prompt: with one row
the pieces can each be right while the nesting is wrong, and only the
screen shows it.

**And the hooks do not fire again.** `precmd` and its neighbors belong to
the prompt *line*; a prompt redrawn because a segment arrived must not fire
a person's hooks a second time. So the prompt half of `beforeReading` is
its own function and the redraw uses that half alone.

### What a not-yet-ready segment draws: nothing, and the source decides

Settled 2026-09-21, and this is the answer to the first of the open
questions below.

`repl` invents no placeholder, no spinner and no ellipsis. A segment draws
**what it has**, which before its answer arrives is usually nothing, and
the layout is a separate pass over whatever rendered — so an unanswered
segment costs no space, exactly as an absent tool does.

Three reasons, and the first is the one that decides it:

- **There is nothing to measure.** zsh has no asynchronous prompt segments
  of its own; every async prompt in the wild is user code. A placeholder
  invented here would be this implementation's taste presented as behavior.
- **It is the line `PromptProvider` already draws.** The text is drawn as
  it stands and no separator is added, because something that wanted one
  would have no way to take it back. A placeholder is a separator with a
  glyph on it.
- **It halves the number of times the prompt moves.** A segment arriving
  changes the prompt's width once. A placeholder changes it twice, and the
  second change is the one nobody asked for.

A source that *wants* to say "working on it" says so by rendering that
text — it is the source's answer, in the source's words, and the engine
neither supplies it nor knows about it.

### What a chatty publisher costs

The other open question below, answered by construction rather than by a
measurement, because the bound is in the mechanism:

- **At most one byte is ever in the wake.** A publisher that fires a
  thousand times while a command runs writes one byte and the other 999
  are free, because the editor re-renders the prompt from scratch rather
  than applying what was published. Publishing is idempotent and
  coalescing.
- **So the redraw rate is the editor's, not the publisher's.** A wake is
  served where a keystroke would have been read, one per pass, and a
  publish that arrives during a redraw is served by the next pass.
- **A publish that changes nothing writes nothing.** The rendered prompt
  is compared with the one on the screen and an identical one is dropped
  before any escape sequence is emitted.

What is *not* bounded, deliberately, is how long a publisher takes to have
an answer. That is the point of it not being asked.

## Presets

A preset is a set of assignments, so it is data. The layout pass spans
"no backgrounds, a space separator" to "a background per segment and
powerline arrows", which is the whole range prompts live in, so a preset
costs no code.

Presets ship as files and are loaded from files — see *Presets and icon
tables are files*, which is the half of this that the no-rebuild
constraint decides. A preset somebody publishes and a preset compiled in
are the same format, so neither can express an arrangement the other
cannot.

Two further rules govern the ones shipped here:

- **A preset named after another project is a claim about fidelity.** Any
  preset that reproduces a published look declares what it does *not*
  reproduce, at the moment it is applied — not in `prompt show`, because
  a saved configuration is a resolved list of settings and there is no
  way to read a choice back out of it.
- **A look is reproduced from what is published, not from source.** A
  color table transcribed from a project's documented preset is data; a
  theme that is a program has nothing to transcribe and is rebuilt from
  the published appearance or not at all.

## What no configuration at all must look like

A default prompt that is fast and plain is a better first minute than a
rich one that needs a file. With no elements configured the engine draws
a bare prompt character and a continuation, and that path must not be
slower than the prompt it replaces.

### But a wired theme is not a configured one

Those are two different questions and conflating them would take a prompt
away from somebody who set one. A front end wires a theme into every
interactive session — it is a capability of the substrate, so every
dialect binary carries one — and **the theme draws only when a
configuration asks for it**: an elements setting *present* on one side,
whether from a variable, a file or a preset. Otherwise it reports that it
is not drawing and the prompt is the person's own parameter, exactly as it
would be with no theme wired at all.

Present, not non-empty. **Set-to-empty is an answer here as everywhere
else in this namespace**: emptying the elements is a themed prompt with no
segments in it, and unsetting them is no theme at all. Reading the two the
same way would leave no way to say the first, and would make a preset that
empties one side turn the whole theme off.

So the engine's bare prompt is for *a configuration that names no
elements*, not for *a session that named no configuration*. The second is
somebody's `PS1`, and a theme that drew a bare `$` over it would be the
silent-wrong-answer class applied to the most visible line on the screen.

It is also what makes the wiring free: a session that never configures a
prompt costs one variable lookup per prompt and nothing else.

## Budget and measurement

- The budget is **per prompt**, not per keystroke. #1323 says the prompt
  re-renders on every keystroke that changes the line; that is not true
  in this tree — `repl.Shell.prompt` fires hooks and checks the window
  size, so it runs once per prompt. The in-process conclusion is
  unaffected: a round trip per prompt is still excluded by
  `docs/design/plugins.md`, and the engine is in-process.
- The number goes in a **benchmark**, not a test, and is stated in the
  documentation: a budget belongs where people read it, and a regression
  should still be one command away.
- Width and color are verified through the pty harness, which is the only
  instrument that sees what is actually drawn. A test that strips ANSI
  cannot tell a working theme from a broken one, and a one-row `PS1`
  cannot tell a working multi-line prompt from a broken one.
- The comparison that matters is against a real shell running a real
  themed prompt, on the same machine, measured the same way — not against
  this shell's own previous number.

## What is not yet measured

Written down rather than assumed, because a spec entry that presents a
guess as a fact is worse than an absent entry:

1. Containment for a segment function that does not terminate.
2. **The fidelity claim itself.** Until the cell-grid comparison exists
   and runs against a real configuration, "draws what it draws" is
   asserted and not shown — and it is the one claim in this document a
   person can check by looking.
3. The per-prompt cost of the render itself, against the plain prompt it
   replaces, on a cold page cache as well as a warm one.

Two came off this list on 2026-09-19 and are written up where they
belong rather than here: right-prompt behavior is under *The right
prompt is new here*, and the transient redraw's interaction with
`groundForPrompt` and the blocks store is under *Transient prompt*.

Two more came off on 2026-09-21, and both are under *Async segments*:
what a not-yet-ready segment draws is **nothing, and the source decides**,
and what a chatty publisher costs is bounded by the mechanism rather than
by a measurement — one byte in the wake, however often it is raised.

Each of these is a measurement to run before the code that depends on it
is written.

## Related

- #1323 — the decision that the engine lives in the substrate, and the
  architecture.
- #1314 — repository status as a shell capability, and async segments.
- #1320 — the primitives an unmodified external theme would need, which
  is a different route to a different goal and stays useful on its own.
- `docs/design/plugins.md` — the transport, handshake, trust model and
  failure semantics a segment role would be built on, and the hot-path
  exclusion it has to answer.
- `repl/promptprovider.go`, `repl/promptrender.go`, `repl/repl.go` and
  `dialect/zsh/promptnames.go` — the seam as it stands in this tree.
- `docs/spec/prompt.md` — what a dialect does to a prompt *parameter*,
  which is the neighboring question and not this one.
