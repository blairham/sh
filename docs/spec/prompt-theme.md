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

**Sourced:** powerlevel10k's published documentation and the settings
found in real `.p10k.zsh` files, which are data rather than an
implementation; starship's preset TOMLs; and the maintainer's own prior
prompt engine, which `CLEANROOM.md`'s green list covers in full. Where
this document says *upstream*, it means powerlevel10k's observable
behavior, never its source. Facts carried from the prior engine are
marked **(prior)**; facts that still have to be measured before the code
that depends on them is written are marked **(unmeasured)** and are
listed together at the end.

**Decided by the maintainer, 2026-09-18**, on top of #1323's decision of
2026-09-07:

- **Spec first, then port.** This document is the wall; the port is
  written against it.
- **Keep the upstream spellings.** `POWERLEVEL9K_*`, `p10k.conf`,
  `~/.p10k.zsh`. A line pasted out of an existing configuration has to
  work, and the names are a compatibility claim rather than a borrowed
  one.
- **The interpreter may be used for configuration, including at render
  time.** This shell has a real zsh dialect, which the prior engine did
  not; see *Bringing an existing configuration across*.

## Why this is not in a dialect

A theme engine in `dialect/zsh` would be a prompt that exists only if you
are running the zsh dialect, which is the inversion `AGENTS.md` bars: a
dialect would own a capability the core lacks. The engine therefore lives
in `repl`, which imports no dialect and names a shell only in comments,
and that property has to survive this work.

The consequence is the point of the whole exercise. A shell script run
under `dash` gets the same prompt as one run under `zsh`, because the
front end draws it and the language never sees it.

### What each dialect decides

Only the names. The engine is driven by the front end, so every row of
this table describes what a *script* can reach, not what is drawn.

| dialect | names it can offer | what it cannot change |
| --- | --- | --- |
| `zsh` | `RPROMPT`/`RPS1`, `precmd`/`preexec`, `${(%)…}` over theme markup | that the theme draws, or what it draws |
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

This is what makes the same rendered prompt correct in all five dialects
without a per-dialect pass, and it is why a theme's `%F{31}` is expanded
by the engine's own reader (below) and not by `interp.RenderPromptValue`.

### Width

Measured with `repl`'s existing cell-width machinery — `cellwidth.go` and
the generated East Asian tables — and **not** a new dependency. The prior
engine used `uniseg` for this; this tree generates its Unicode tables
rather than importing them, and the layout needs exactly one number: the
on-screen width of a string with escape sequences discounted, counted in
grapheme clusters so an emoji or a combining sequence costs its rendered
width once.

A prompt that miscounts by one wraps the terminal and smears the frame on
every keystroke, so this is the number to get right before any of the
decoration.

## The configuration namespace

Upstream's configuration is roughly 565 `POWERLEVEL9K_*` shell variables.
That is not 565 features; it is **one** mechanism applied to every
segment, and modeling it as a keyed store rather than a Go struct is
what keeps a preset data instead of code **(prior)**.

- Keys are stored **without** the prefix and upper-cased:
  `DIR_FOREGROUND`, `LEFT_PROMPT_ELEMENTS`, `MODE`.
- On input, `POWERLEVEL9K_`, `POWERLEVEL10K_` and `P9K_` are all accepted
  and stripped, in any case, so a pasted line works whatever it was
  spelled as **(prior)**.
- A key holds **either** a scalar or a list, never both. Setting one
  clears the other. A scalar read as a list is a one-element list, which
  is how shell arrays and strings interchange upstream **(prior)**.
- **Set-to-empty is an answer**, distinct from absent: an empty prefix
  and a suppressed icon are both configured states, so the lookup has to
  offer "is this set at all" separately from "what is it" **(prior)**.
- A malformed numeric or truth value **degrades to the default**. A
  prompt is not the place to report a typo by not drawing.

### The three-step chain

Every per-segment setting resolves through the same three steps, most
specific first, then the caller's default **(prior)**:

    POWERLEVEL9K_<SEGMENT>_<STATE>_<KEY>    DIR_NOT_WRITABLE_FOREGROUND
    POWERLEVEL9K_<SEGMENT>_<KEY>            DIR_FOREGROUND
    POWERLEVEL9K_<KEY>                      FOREGROUND

An empty state drops the first step. Hyphens in a segment name become
underscores. This chain is why a preset can set one `FOREGROUND` and have
every segment inherit it while any segment, or any single state of one
segment, overrides — and it is why adding a segment costs no new
configuration plumbing.

### Layering

Later wins over earlier, per key, shallow:

1. the preset (`POWERLEVEL9K_PRESET`, default `lean`)
2. `$XDG_CONFIG_HOME/sh/p10k.conf`
3. `POWERLEVEL9K_*` set in the session

A list set by a later layer **replaces** the earlier list outright rather
than merging, because "my elements are these" cannot mean anything else
**(prior)**.

The file is re-read when its mtime changes, so editing it takes effect on
the next prompt and there is no reload command **(prior)**.

The engine records, per setting, which layer it came from, so the
`prompt show` surface can say where a value came from rather than only
what it is.

### Colors

Three spellings are accepted for every color setting, because presets in
the wild use all three **(prior)**:

| spelling | example | note |
| --- | --- | --- |
| xterm-256 index | `4`, `31`, `255` | rendered as `38;5;N` even for 0–7 |
| 24-bit hex | `#1e66f5` | rendered as `38;2;R;G;B` |
| name | `blue`, `bright-blue`, `brblue` | eight base names; `br`/`bright-` prefix |

An **unrecognized** name resolves to *unset*, not to a wrong color: unset
emits nothing and leaves the terminal's default, which is the honest
rendering of "I did not understand this".

### Markup inside a value

Presets do not store "the frame prefix is `╭─` in color 242" as two
settings; they store the string `%242F╭─` **(prior)**. That markup is
zsh's prompt-escape vocabulary, and reading it is not interpreting zsh —
it is a colour markup with a fixed vocabulary. The subset that presets
actually use:

    %F{c}  %<n>F   foreground on      %f      foreground off
    %K{c}  %<n>K   background on      %k      background off
    %B %b          bold on/off        %U %u   underline on/off
    %%             a literal %

Anything outside the subset **passes through untouched** rather than
being guessed at.

Substitution inside a value is limited to `${NAME}` against a
caller-supplied lookup, which is how a content expansion reaches
`P9K_CONTENT` and `P9K_VISUAL_IDENTIFIER`. Command substitution and
arithmetic are not part of the markup; where a configuration wants real
shell, that goes through the route in *Bringing an existing configuration
across* and not through this reader.

The appearance is emitted **in full at every change** rather than as a
delta, so that pieces stay correct when the layout concatenates them
**(prior)**.

### Icons, and what `MODE` selects

`MODE` picks an icon table. Three tables are carried — `nerdfont-v3`
(the default), `ascii`, and one awesome variant — against upstream's
eight **(prior)**.

The rule that matters is the honesty one: a mode that is not carried is
**served by `nerdfont-v3` and says so** through `prompt show`. Silently
substituting a different glyph set is how a prompt ends up full of boxes
with no explanation.

Icons belong in the table, not in the segments. In the prior engine they
were hardcoded per segment for a while, and the symptom was exact:
`MODE=ascii` did not produce an ASCII prompt, and a global visual-
identifier override had nothing to default to **(prior)**.

## Layout

One algorithm, per side of each line **(prior)**:

    for each segment that rendered:
      emit a separator, whose colors are decided by whether this
        segment's background differs from the previous one
      emit whitespace, icon and content in the segment's own colors
    emit the side's end symbol in the last background

With no backgrounds and the separator set to a space, that loop produces
a lean two-line prompt. With a background per segment and the separator
set to a powerline arrow, **the same loop** produces a framed one. This
genericity is the whole reason a look from another project costs data and
no code.

- `newline` in an elements list splits the side into lines; the number of
  lines is the larger of the two sides' counts.
- The **last line is the one being typed on**. Its right side becomes a
  true right prompt, so the editor can hide it when the typed line grows
  into it rather than baking it into text that would then wrap.
- Earlier lines are banner lines: their right side is placed by filling
  the gap to the measured width, and they end where their content ends.
- A **single space** separates the end of the left prompt from what the
  person types. It is not any of the whitespace parameters and not the
  last-segment end symbol: measured against a real prompt, it survives
  every one of those being set empty, so it is modeled as a fixed suffix
  on the line being typed on **(prior)**.

### The right prompt is new here

Nothing in this tree draws a right prompt today. `RPROMPT`/`RPS1` appears
only in `dialect/zsh/promptnames.go`, as the name-aliasing measurement,
and no drawing code reads it.

Building it in `repl` is therefore a capability addition and not a theme
detail: bash, ksh, dash and ash get a right prompt they have never had,
and the zsh dialect gets to *name* the one the substrate draws. It has to
answer, at minimum:

- what happens when the typed line reaches it (hide, and redraw when the
  line shrinks back) **(unmeasured — settle against real zsh)**;
- what happens on a terminal too narrow to hold both sides;
- that it is not written into scrollback, so a resized window does not
  leave fragments behind.

### Transient prompt

    TRANSIENT_PROMPT = always | same-dir | off

Once a command has run, its prompt collapses to the bare prompt
character, so scrollback is output rather than twenty copies of a
two-line frame. This is the single setting most responsible for a themed
prompt staying usable **(prior)**.

`same-dir` is inverted by necessity: the trim happens when the line is
accepted, before the command has run, so nothing can yet know whether it
will change directory. A prompt is therefore trimmed unless the
**previous** command moved, which leaves one full prompt as a landmark
just below the `cd` rather than just above it **(prior)**. The
difference is stated in the documentation rather than hidden.

Transient redrawing is a `repl` capability for the same reason the right
prompt is, and it is the piece most likely to interact badly with the
blocks store and with `groundForPrompt` — settle that interaction before
writing it, not after.

## Segments

**The rule, which is where the speed comes from:** a segment may read the
context it is given, environment variables, and files. It may not fork,
may not dial, and may not block **(prior)**. Anything needing a
subprocess or a network round trip arrives pre-computed and possibly
stale, or does not arrive.

A segment may **decline to render**. Layout is a separate pass over
whatever survived, which is why an absent tool costs no space rather than
an empty box.

### The context a segment is allowed to know

Working directory, home, user, host, the last pipeline's exit status and
duration, job count, terminal width, whether this shell is root, whether
the session is remote, the render clock, an environment lookup, the
previous prompt's directory, and repository status when somebody has
computed it **(prior)**. `repl.PromptInfo` already carries most of this;
the gaps are width, root, remote, and the previous directory.

Filesystem lookups that walk upward for a marker file (`.python-version`,
`.tool-versions`) are **memoized for the life of one render**, because a
dozen version-manager segments each walking independently is a hundred
stats per prompt. One render is the memo's whole lifetime, so nothing is
ever served stale **(prior)**.

### The roster

The prior engine implements, and the port carries:

    anaconda asdf aws aws_eb_env azure background_jobs chezmoi_shell
    command_execution_time context cpu_arch detect_virt dir direnv fvm
    gcloud goenv google_app_cred haskell_stack jenv kubecontext lf luaenv
    midnight_commander nix_shell nnn nodeenv nodenv nvm os_icon
    per_directory_history perlbrew phpenv plenv prompt_char proxy pyenv
    ranger rbenv rvm scalaenv status terraform time todo toolbox vcs
    vim_shell virtualenv xplr yazi

System state stays inside the no-fork rule: `load`, `swap`, `disk_usage`,
`ip` and `vpn_ip` are files, one syscall, or an interface enumeration.
Availability is **per metric, not per segment** — `ram` and `battery` on
macOS want cgo (`host_statistics64`, IOKit) and therefore render nothing
there, which is what an absent reading should look like **(prior)**.
Interface enumeration costs more than a whole lean prompt, so it reads a
short-TTL cache rather than the kernel per prompt **(prior)**.

Not carried, and each for a stated reason: the dialing half of the
network group (`public_ip`, `nordvpn`), the task trackers, and the
`*_version` family that shells out to each tool — the version *managers*
answer most of that need by reading a pin file, which is a different
question anyway (what the project is pinned to, not what is on `PATH`).

### An element with no implementation says so by name

A configured element that has no segment renders **nothing**, and is
named under `not yet` by `prompt show` **(prior)**. The same convention
applies to a setting that is stored faithfully and not acted on: a
setting has three states — absent, honored, and *set and ignored* — and
the third is the one that used to be invisible. The person configured
something, the tool reported it, and the prompt quietly did something
else. That list is written down and shrinks as things are implemented.

This is the same silent-wrong-answer rule the rest of this repository
holds to, applied to a prompt.

## Repository status

Split by what it costs **(prior)**:

- **The branch** is a handful of stats and one small read, done
  synchronously, cached on the mtime of the files that would change it.
  It is what people navigate by and it must never be late.
- **The counts** (modified, untracked, ahead, behind) need a full index
  versus working-tree walk. That is the expensive half, and it is exactly
  why upstream hands it to a separate process.

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

## Bringing an existing configuration across

This is where this shell diverges from the prior engine, and it is the
maintainer's decision of 2026-09-18.

The prior engine pattern-reads `~/.p10k.zsh` and takes 304 of 310
settings from a real 1,720-line configuration. **What it cannot take is
the code** — a configuration generated by `p10k configure` defines a
shell function, points `VCS_CONTENT_EXPANSION` at it, and honoring that
would have meant running zsh to draw a prompt **(prior)**.

This shell has a zsh dialect. So:

- **Configuration is evaluated, not pattern-read.** `~/.p10k.zsh` is
  sourced in a zsh-dialect runner and the parameter namespace is
  harvested from it, which takes the conditionals and the version gate
  correctly rather than approximately. The gate is real code: `[[
  $ZSH_VERSION == (5.<1->*|<6->*) ]]` is the line every configuration
  opens with, and #1217 is the record of the whole configuration failing
  to load because of it.
- **`CONTENT_EXPANSION` may call the function.** The interpreter is in
  the process; calling `my_git_formatter` costs no fork. This closes the
  one documented gap in the prior port.
- **Evaluation happens in a runner of the zsh dialect regardless of the
  session's dialect**, because the file is a zsh file. A bash session
  importing a `.p10k.zsh` is reading data written in another language,
  which is a thing this shell can do and no other prompt can.
- **The render path is not made dialect-dependent by this.** A content
  expansion that calls a function is a *configured* cost, taken only by
  configurations that ask for one, and it is named in `prompt show` so
  the cost is visible. The default configuration calls nothing.
- **A function that fails, hangs or is missing costs its segment and
  nothing else** — the panic guard already covers the provider; this
  needs the same containment for a non-terminating call, and the honest
  answer for "it did not finish" is the async contract above rather than
  a timeout invented here **(unmeasured — settle before implementing)**.

Settings still not honored are reported at import by name rather than
dropped silently or half-interpreted into something that looks nearly
right.

## Presets

A preset is a set of assignments, so it is data. Carried from the prior
engine: `lean` (default), `classic`, `rainbow`, `pure`, `lean-8colors`,
`robbyrussell`, plus `pastel-powerline` and `tokyo-night` transcribed
from starship's preset TOMLs and `agnoster` rebuilt from the published
look **(prior)**.

**Naming a preset after someone else's project is a claim about
fidelity.** Every preset declares what it does not reproduce, at the
moment it is applied — not in `prompt show`, because the saved file is a
resolved list of settings and there is no way to read a choice back out
of it **(prior)**. The recurring gap is always the same one: the upstream
runs a command per prompt and no segment here forks.

## What no configuration at all must look like

A default prompt that is fast and plain is a better first minute than a
rich one that needs a file. With no elements configured the engine draws
a bare prompt character and a continuation, and that path must not be
slower than the prompt it replaces.

## Budget and measurement

- The budget is **per prompt**, not per keystroke. #1323 says the prompt
  re-renders on every keystroke that changes the line; that is the prior
  engine's note and it is not true here — `repl.Shell.prompt` fires
  hooks and checks the window size, so it runs once per prompt. The
  in-process conclusion is unaffected: a round trip per prompt is still
  excluded by `docs/design/plugins.md`, and the engine is in-process.
- The number goes in a **benchmark**, not a test, and is stated in the
  documentation: a budget belongs where people read it, and a regression
  should still be one command away **(prior)**.
- Width and color are verified against a real terminal through the pty
  harness, which is the only instrument that sees what is actually drawn.
  A test that strips ANSI cannot tell a working theme from a broken one.
- The comparison that matters is against a real zsh running the real
  theme, on the same machine, measured the same way — not against this
  shell's own previous number.

## What is not yet measured

Written down rather than assumed, because a spec entry with no citation
is a guess:

1. Right-prompt behavior when the typed line reaches it, and on a
   terminal too narrow for both sides.
2. The interaction between transient redraw, `groundForPrompt`, and the
   blocks store.
3. What an async segment draws before its answer arrives.
4. Containment for a `CONTENT_EXPANSION` function that does not
   terminate.
5. Whether the harvested namespace from an evaluated `.p10k.zsh` matches,
   setting for setting, what the same file produces under real zsh — the
   import's own correctness check, and the one that would catch a quiet
   divergence.

Each of these is a measurement to run in the spec phase, against the real
binaries, before the code that depends on it is written.

## Citations

- powerlevel10k, Roman Perepelitsa and contributors, MIT — published
  documentation and the settings found in real configuration files. This
  is a reimplementation of observable behavior; its source is not read.
- starship, ISC — preset TOMLs, transcribed as data.
- The maintainer's earlier prompt engine, green-listed by `CLEANROOM.md`
  in full: 42 files and roughly 6,900 lines including tests, which
  corrects the 26 files / ~6,500 lines quoted in #1323.
- `repl/promptprovider.go`, `repl/promptrender.go`, `repl/repl.go` and
  `dialect/zsh/promptnames.go` for the seam as it stands in this tree.
- #1323 (the decision and the architecture), #1314 (repository status and
  async), #1320 (the primitives an unmodified external theme would need),
  #1217 (the version gate).
