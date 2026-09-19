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
  clears the other. A scalar read as a list is a one-element list, since
  dash has no arrays and a list still has to be expressible there.
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
copied out of a session works in the file and back again.

It is re-read when its mtime changes, so editing it takes effect on the
next prompt and there is no reload command.

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

### The right prompt is new here

Nothing in this tree draws a right prompt today. `RPROMPT`/`RPS1` appears
only in `dialect/zsh/promptnames.go`, as the name-aliasing measurement,
and no drawing code reads it.

Building it in `repl` is therefore a capability addition and not a theme
detail: bash, ksh, dash and ash get a right prompt they have never had,
and the zsh dialect gets to *name* the one the substrate draws. It has to
answer, at minimum:

- what happens when the typed line reaches it — hide, and redraw when the
  line shrinks back **(unmeasured — settle against real zsh, which is the
  only shell in the panel that has one)**;
- what happens on a terminal too narrow to hold both sides;
- that it is not written into scrollback, so a resized window does not
  leave fragments behind.

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

Transient redrawing is a `repl` capability for the same reason the right
prompt is, and it is the piece most likely to interact badly with the
blocks store and with `groundForPrompt` — settle that interaction before
writing it, not after.

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

### A segment can be a shell function

The interpreter is in this process. So a segment's content may be
produced by **a shell function in the session**, called in-process with
no fork, and this works in whatever dialect the session is running —
a bash function in a bash session, a zsh function in a zsh one.

This is a capability no external prompt program can have, and it is the
escape hatch that keeps the no-fork rule honest: someone who genuinely
needs to run something gets to, deliberately and visibly, instead of the
engine growing a forking segment for every tool in the world.

It is constrained accordingly:

- **Off unless configured.** The default configuration calls nothing, and
  `prompt show` names every function a configuration installs, so the
  cost is visible rather than discovered.
- **A function that fails or is missing costs its segment and nothing
  else.** The provider's panic guard covers a panic; a non-zero return
  and an unset name are ordinary outcomes and render nothing.
- **A function that does not terminate is the hard case**, and the honest
  answer is the async contract below rather than a timeout invented here
  **(unmeasured — settle before implementing)**.

### The roster

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

## Presets

A preset is a set of assignments, so it is data. The layout pass spans
"no backgrounds, a space separator" to "a background per segment and
powerline arrows", which is the whole range prompts live in, so a preset
costs no code.

The set ships as data and grows as looks are wanted. Two rules govern it:

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

1. Right-prompt behavior when the typed line reaches it, and on a
   terminal too narrow for both sides. zsh is the only column in the
   panel that has one, so it is the only place to measure it.
2. The interaction between transient redraw, `groundForPrompt`, and the
   blocks store.
3. What an async segment draws before its answer arrives.
4. Containment for a segment function that does not terminate.
5. The per-prompt cost of the render itself, against the plain prompt it
   replaces, on a cold page cache as well as a warm one.

Each of these is a measurement to run before the code that depends on it
is written.

## Related

- #1323 — the decision that the engine lives in the substrate, and the
  architecture.
- #1314 — repository status as a shell capability, and async segments.
- #1320 — the primitives an unmodified external theme would need, which
  is a different route to a different goal and stays useful on its own.
- `repl/promptprovider.go`, `repl/promptrender.go`, `repl/repl.go` and
  `dialect/zsh/promptnames.go` — the seam as it stands in this tree.
- `docs/spec/prompt.md` — what a dialect does to a prompt *parameter*,
  which is the neighboring question and not this one.
