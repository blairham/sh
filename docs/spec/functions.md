# The autoloadable functions this shell ships

One dialect has a *function search*: `autoload -Uz name` marks a name, and
calling it reads a file called `name` off `$fpath` and makes the file's
contents the function's body. `docs/spec/semantics.md` has the mechanism;
this file is about the **payload** — the function files this shell ships,
what each one is measured to do, and where the implementation here parts
company with the shell it was measured against.

The default search is two directories derived from where the binary sits
(`driver/functiondirs.go`), and until #1968 there was nothing in either of
them. A real `~/.zshrc` reaches four of these names before it has done
anything of its own, and every one of them failed at the call with
`function definition file not found`.

## The decision

**What a shell's default `$fpath` holds is a statement about what the shell
is**, which is why #1282 was a decision and not a defect. Four options were
on the table: ship nothing and keep the honest refusal; point the default at
another installation's library; ship a small library of our own; or fire the
hooks first and then decide. The maintainer took the last two together —
**#1281 first, then a library of our own** — and this file is the record of
what that came to.

Both halves are load-bearing and neither is worth much alone:

* **#1281 made the hooks fire.** `precmd`, `preexec` and their `_functions`
  arrays were registered correctly and never run, so a shell that shipped
  `add-zsh-hook` before that landed would have shipped a function that puts a
  name in a list nothing reads — a stub in all but spelling, and exactly the
  silent failure this repository's honest-refusal convention exists to
  prevent.
* **#1968 shipped the files.** Four of them rather than the two the decision
  named, because the same measurement that counted `is-at-least` and
  `add-zsh-hook` found `colors` and `regexp-replace` on the same startup path
  for the same cost.

The join is the thing neither issue could test on its own, and it has its own
acceptance test now: a startup file registers a hook through the *shipped*
`add-zsh-hook`, a command is typed at a real prompt, and the hook has to run
(`cmd/zsh`, `TestAHookTheShippedFunctionRegisteredFiresAtThePrompt`). Each
half was already graded — the file against zsh's answers, the firing against
an array a test filled in Go — and both passed, in that order, while this
shell registered hooks byte-identically to zsh's and ran none of them.

**Reading another installation's library stays reachable by hand** and always
did; `FPATH=/opt/homebrew/share/zsh/5.9.2/functions` is a line in a startup
file. The decision was only about what the *default* is, and the default is
now this shell's own.

### What this does not reach

`compinit`, and therefore an arbitrary real `~/.zshrc`. That was true of
every option on the table and the decision was taken knowing it: completion
is a project rather than a function file, and `$fpath` moves none of it —
the file itself is refused, `zmodload zsh/complete` refuses by name, and
`compdef` and `compctl` are reached and are not there. On this
machine's own rc the four files take the startup from 14 complaints to 3,
and the three are `compinit` twice and `vcs_info` once. See *What is not
shipped, and why* at the end of this file.

**Every sentence in that paragraph is conditioned on the default search**,
which is how it was measured: `FPATH` out of the environment, so the two
directories this shell derives are what answers. A real rc does not leave
it that way — it puts the system zsh's `fpath` in front — and under that
condition, as of 2026-09-14, `compinit` is **found and runs**: the dump is
read and `_comps` fills with the same 2020 entries the reference shell
holds. What is refused has moved one link down the chain, to `zmodload
zsh/complete` and the `compset`/`compstate` the widget then calls. The
outcome is unchanged and the mechanism and the message are not. #2770 has
the measurement; `docs/install.md` has what it means for somebody deciding
whether to live in this shell.

## Provenance

Every behavior below was learned two ways, and **neither of them was
reading zsh's function files or zsh's source**, which `CLEANROOM.md`'s red
list forbids:

* **the published description** — `zshcontrib(1)`, section *Manipulating
  Hook Functions* for `add-zsh-hook` and *Other Functions* for the rest;
* **black-box observation of the installed binary** — call the function,
  vary the input, record the output and the status. `docs/spec/oracle.md`
  is the method; the difference here is only that the thing being asked is
  a function rather than a construct.

Where the two disagree the binary wins, and each such case is called out
below, because a script in the wild was written against the binary.

Nothing here is a transcription. Each function is written from the
description of what it does plus a table of inputs and answers, and the
tables are in this file so that the next person can check the implementation
against the *behavior* rather than against anyone's code.

**Twelve of those rows are corpus cases** — the `function library` category
in `internal/oracle/case.go`, which runs each snippet through every panel
shell and records what came back. That matters more than a count of passing
tests: every other record of these functions is an expectation somebody
typed, and a case re-measures. The four panel members without such a name
are in the same row and do not fail alike — three report the call not found
and carry on, and ksh93 stops at the `autoload -Uz` line, where `autoload`
is `typeset -fu` and those are not its letters. That is the fact worth
keeping beside the other two: these are zsh's functions, and pointing
`$fpath` somewhere does not give them to anybody else.

Measured against **zsh 5.9.2** (Homebrew) on macOS, 2026-09-11, every probe
under `env -i HOME=… zsh -f`, so no startup file is speaking.

## `is-at-least [ needed [ present ] ]`

Zero when `present` is at least `needed`, one when it is not. `present`
defaults to `$ZSH_VERSION`; so does an **empty** second argument, because
the default is taken with `:-` — measured, `is-at-least 5.9.2 ''` is 0 on a
5.9.2 shell. No arguments at all is 0. A third argument is ignored.

The rule, as implemented:

1. split both strings on `.` and `-`;
2. drop every segment that is not wholly digits;
3. pair what is left left-to-right, counting a missing segment as zero;
4. the first pair that differs decides.

Step 2 is the reading the manual's "leading non-number parts ignored"
leaves open, and it is settled by measurement rather than by taste:

| needed | present | zsh | why |
| --- | --- | --- | --- |
| `2.6-beta3` | `2.6` | 0 | `beta3` is dropped, so the two are equal |
| `2.6-dev-3` | `2.6` | 1 | `dev` is dropped and `3` is not, and `3 > 0` |
| `2.6-0` | `2.6` | 0 | a missing segment is zero, and `0 = 0` |
| `5.9.2-dev-1` | `5.9.2` | 1 | same shape, and the one a `-dev` build has |
| `3.1.1-zefram1` | `3.1.1` | 0 | and equal in the other direction too |
| `5.0.0` | `5.0` | 0 | trailing zeros do not make a version newer |
| `4.3.10` | `4.3.9` | 1 | segments are numbers, not strings |

**1600 of 1600 pairs agree.** The panel is every zsh release spelling from
`2.6-17` to `6.0` crossed with itself, `-beta`, `-zefram` and `-dev`
suffixes included. That sweep is too wide for a corpus row and is run by
hand, with the case list in `dialect/zsh/shippedfunctions_test.go`; the four
readings the sweep turns on — component counts, numeric ordering, a
non-numeric segment, the defaulted second argument — are corpus cases, under
`fnlib/is-at-least-*`. The two are the same measurement at two widths, and
the narrow one is the one that re-runs.

A corpus row cannot ask about `$ZSH_VERSION` itself: this shell reports
`5.9.2-blairham` and the shell it is compared against reports `5.9.2`, so a
row naming a version near either would record the version rather than the
comparison. The defaulting row uses bounds far enough away — `5.0` and `99`
— that only the default's *presence* is what moves it.

**Where it diverges, and why that is the right call.** A segment that mixes
digits and letters in one piece — `1a`, `x2`, `3x` — is ignored here and is
*not* ignored by zsh, which reads a number out of it in a way that is not
self-consistent: measured, `abc` is at once no greater than `0` and greater
than `1a`, and `1a` compares equal to `1` while `1a` and `1b` do not
compare equal to each other. That is not an ordering, and reproducing it
would mean reproducing an accident. 295 of 3600 deliberately pathological
pairs differ; none of them is a version string anybody ships.

## `add-zsh-hook [ -L | -dD ] [ -Uzk ] hook function`

The hooks are `chpwd`, `precmd`, `preexec`, `periodic`, `zshaddhistory`,
`zshexit` and `zsh_directory_name`, and each has a `<hook>_functions`
array. Measured:

| probe | zsh |
| --- | --- |
| `add-zsh-hook precmd f1` twice | `(f1)`, not `(f1 f1)` |
| `add-zsh-hook -d precmd f1` | removes by **exact name**; `f*` removes nothing |
| `add-zsh-hook -D precmd 'f*'` | removes by **pattern** |
| removing the last element | `${+precmd_functions}` is **0** — the array is unset, not emptied |
| `-d` for a name that is not there | 0, silent |
| `-d` for a hook nothing ever set | 0, silent |
| `add-zsh-hook precmd notyetdefined` | autoloads the name, with or without `-Uzk` |
| `add-zsh-hook nosuchhook f1` | usage on **stderr**, status 1 |
| no operands, or three | the same usage, status 1 |
| `-L precmd` | `typeset -g -a precmd_functions=( f1 )` |
| `-L` with no hook | every hook array that is set |
| `-L nosuchhook`, or a hook that is unset | 0, silent |
| an unknown option letter | `add-zsh-hook:N: bad option: -q`, status 1 |

Three divergences, all of them the *shell* rather than the function:

* `-L` writes `typeset -a` where zsh writes `typeset -g -a`. zsh adds the
  `-g` when the listing is made from *inside a function*, so that what it
  writes would recreate a global rather than declare a local; ours never
  does. **#2041.**
* the line number in `bad option` is the line `getopts` stands on in *this*
  file, which is not the line it stands on in theirs.
* `-L` with no hook lists in the order the hooks are named above. zsh lists
  in the order its own parameter table happens to hold them — measured,
  neither that order, nor alphabetical, nor the order they were added —
  so there is nothing there to match.

The letters `-U`, `-z` and `-k` are handed to `autoload`, and **its status is
the function's**, with the hook installed whatever it answered. `-z` and `-k`
name two different autoload styles, so `autoload` refuses the pair, and
measured 2026-10-03 on zsh 5.9.2:

| probe | status | `precmd_functions` after |
| --- | --- | --- |
| `add-zsh-hook -k precmd kf` | `0`, silent | `(kf)` |
| `add-zsh-hook -Uzk precmd kf2` | `1` | `(kf kf2)` |
| `add-zsh-hook -Uzk precmd kf` with `kf` already there | `1` | `(kf)` |
| `add-zsh-hook -d -Uzk precmd kf` | `0` | removed |
| `add-zsh-hook -q precmd kf3` | `1` | unchanged |

The third row is what fixes the order: `autoload` runs *before* the check for
a name already in the array, since a name found there still answers 1. This
shell's copy ignored the status and answered 0 to the second and third rows
until #5714 (#2149). The corpus row suppresses standard error, because the
diagnostic names the line the call stands on in the function file, and that
is a fact about whose file it is rather than about the behavior.

## `colors`

Fills eight associative arrays and two scalars, all global, and is 0. Every
key and every value was read back out of a live shell and is reproduced
byte for byte; `dialect/zsh/shippedfunctions_test.go` asserts the whole
table.

| parameter | holds | hidden? |
| --- | --- | --- |
| `color`, `colour` | every name to its code and every code to a name, both directions, colors and attributes alike | no |
| `fg`, `bg` | eleven names to the escape that sets the color and leaves the intensity alone | yes |
| `fg_bold`, `bg_bold` | the same with `01;` in front | yes |
| `fg_no_bold`, `bg_no_bold` | the same with `22;` in front | yes |
| `reset_color`, `bold_color` | `ESC[00m` and `ESC[01m` | yes |

The eleven keys of `fg` are the eight base colors, plus `default`, `gray`
and `grey`. `gray` and `grey` are black and run one way only: `$color[30]`
answers `black`, never either of them. `colour` is a **separate** array
holding the same pairs — measured, `color[zzz]=9` leaves `$colour[zzz]`
empty.

"Hidden" is `typeset -H`: the value is withheld from a `typeset -p`
listing, which is what keeps `typeset -p fg` from writing eleven escape
sequences. It is measured rather than chosen — `${(t)fg}` is
`association-hideval` and `${(t)color}` is plain `association`.

The manual and the binary disagree about the attribute set, and the binary
wins: `zshcontrib(1)` names `standout` and `no-standout`, and 5.9.2 has
`italic` (03) and `no-italic` (23) in their place. A prompt theme in the
wild was written against the shell.

One difference remains, and it is this shell's rather than this file's:
`${(t)fg}` answers `association-hide` here where zsh answers
`association-hideval`. `dialect/zsh/parameters.go` records why `hideval` is
not written — and now has a measurement that says `typeset -H` should write
it and `hide` belongs to `-h`. **#2042.**

## `regexp-replace var regexp replace`

A global search and replace over the string a *named* variable holds, with
POSIX extended regular expressions. The variable is changed in place; 0
when at least one replacement happened, 1 when none did.

The replacement text is expanded once per match, so `$MATCH` in it is the
text that match consumed. Through the expansion flag that asks for exactly
that, `${(e)…}`, rather than through an `eval` of the text inside quotes:
measured, a replacement holding a lone `"` is replaced literally
(`regexp-replace v b 'x"y'` over `abc` is `ax"yc`), where an eval reports
`unmatched "` and drops it. That corner is in the table below because it is
the one place a reasonable implementation is visibly wrong. `MATCH`, `match`, `MBEGIN`, `MEND`, `mbegin` and
`mend` are declared local, which is measured: `$MATCH` is empty at the top
level after the call.

| subject | regexp | replace | zsh |
| --- | --- | --- | --- |
| `hello world` | `o` | `0` | `hell0 w0rld` |
| `aaa` | `a*` | `X` | `X` |
| `abc` | `x*` | `Y` | `YaYbYc` |
| `abcabc` | `b*` | `-` | `-a--c-a--c` |
| `hello` | `z` | `X` | unchanged, status 1 |
| `aXaXa` | `^a` | `Z` | `ZXaXa` |
| `abc` | `(a)(b)` | `${match[2]}${match[1]}` | `bac` |
| `abc` | `b` | `x&y` | `ax&yc` — `&` is not special |
| `abc` | `b` | `x"y` | `ax"yc` — and not an `unmatched "` |
| `abc` | `b` | `a\b` | `aa\bc` |

The empty match is what shapes the loop: a pattern that can match nothing
matches between every pair of characters, so the loop carries the character
it sat in front of over untouched and goes on from the next one. And `^`
anchors what is *left* of the subject rather than the original string,
which is why `^a` over `aXaXa` replaces once — `zshcontrib(1)` warns of
exactly that.

**It needs `=~` to report what it matched**, which is the one piece of
engine this issue added: `interp.Runner.SetRegexCaptureReport`, called from
the zsh dialect's `Apply`, publishes a successful `=~` through the same
parameters a reporting *pattern* already fills. Measured against 5.9.2 with
`[[ 'hello world' =~ 'o (w[a-z]+)d' ]]`:

    MATCH=o world  MBEGIN=5  MEND=11
    match=(worl)   mbegin=(7)  mend=(10)

and a *failing* match leaves all six holding what the match before it put
there — the reporting pattern's own rule, and the opposite of the dense
`BASH_REMATCH`-shaped record beside it, which is emptied on every
evaluation. Both are in `interp/regexmatch.go`.

One divergence: an **empty** regular expression. zsh's engine refuses it
(`failed to compile regex: empty (sub)expression`, status 1); Go's
`regexp` compiles it and matches the empty string everywhere, so
`regexp-replace v '' Y` over `abc` is `YaYbYc` and 0 here. That is the
regular-expression engine rather than this function, and it is the same
answer `[[ x =~ '' ]]` gives. **#2043.**

## `add-zle-hook-widget [ -L | -dD ] [ -Uzk ] hook widgetname`

The hooks are `isearch-exit`, `isearch-update`, `line-pre-redraw`,
`line-init`, `line-finish`, `history-line-set` and `keymap-select`, written
with or without the `zle-` prefix. The widgets for each are kept in a zstyle
on the context `zle-<hook>`, style `widgets`, which is what zshcontrib(1)
says and what `zstyle -L` shows. Measured 2026-10-02 on zsh 5.9.2 under
`-f -c` (#5393):

| probe | zsh |
| --- | --- |
| the first registration of any hook | `zstyle zle-hook types` set to the seven hooks |
| `add-zle-hook-widget line-init foo` | `zstyle zle-line-init widgets 1:foo` |
| the same widget again | not added twice |
| a widget already on `zle-line-init` (`zle -N zle-line-init myinit`) | kept as `0:user:myinit`, under a widget `user:myinit` |
| the special widget afterwards | `zle -N zle-line-init azhw:zle-line-init` |
| `-d line-init foo`, then a new widget | the next number is one past the highest — the gap stays |
| `-D line-init 'q*'` | removes by pattern |
| `-L` | the `widgets` styles, as `zstyle -L` writes them |
| `-Uz keymap-select kk`, `kk` no widget | autoloaded with `-Uz` and made a widget |
| an unknown hook, or one operand | the usage on standard error, status 1 |

The dispatcher runs the widgets in numeric order, each as `zle <name> -Nw --
"$@"`. **It is never reached here yet**: this shell's line editor does not
call the special widgets at all, so registering works and nothing fires.
That is #5398.

## What the four are worth, measured against a real startup file

This machine's own `~/.zshrc` — Powerlevel10k, `zi` with turbo-mode
plugins, `zsh-syntax-highlighting` — driven through the shell twice, with
`FPATH` taken out of the environment so the default search is what answers.
The binaries are installed trees, one built from `main` and one from this
change, so both find whatever their own `share/sh/functions` holds.

**The whole file, with its deferred hooks fired** — `zsh -i -c 'source
~/.zshrc; for f in $precmd_functions; do $f; done'`, which is the route that
reaches everything, because `zi`'s turbo mode defers half the plugin loads
to `precmd`:

| complaint | before | after |
| --- | ---: | ---: |
| `add-zsh-hook: function definition file not found` | 5 | 0 |
| `is-at-least: …` | 4 | 0 |
| `colors: …` | 2 | 0 |
| `compinit: …` | 2 | 2 |
| `vcs_info: …` | 1 | 1 |
| **total** | **14** | **3** |
| `zsh-syntax-highlighting: failed loading add-zsh-hook.` | 2 | 0 |

**A real interactive session on a pseudo-terminal**, same rc: 10 before, 1
after — `add-zsh-hook` 6, `is-at-least` 2, `colors` 1, `compinit` 1, and
the syntax-highlighting failure, down to the single `compinit`. The counts
are lower than the run above and the reason is **#2046**: with this rc, an
interactive session loses its terminal 0.3 seconds in, at the
instant-prompt block, so it never reaches the rest of the file. That is a
defect of its own, it is on `main` as well, and it is the reason a pty
acceptance run cannot yet be the whole measurement.

Every probe behind the tables above was re-run against zsh 5.9.2 through
the same file on `FPATH`: `is-at-least`, `colors`, `regexp-replace`, the
alias row and the `=~` capture parameters come back **byte-identical**, and
`add-zsh-hook` differs on the one `typeset -g` line of #2041.

## `compinit`, `compaudit`, `compdump` and `bashcompinit`

`compinit` used to be out of scope here on the reasoning that it initializes
a completion system this shell does not have. That was the wrong question:
a startup file calls it whether or not Tab is driven by it, and then calls
`compdef` and reads `_comps` — zi replays its recorded `compdef` calls, and a
framework's completion setup calls `bashcompinit` — so its absence was a
startup error on every real rc (#5393). What is shipped is the **tables and
the names**, written from zshcompsys(1) and measured against zsh 5.9.2 under
`-f -c` with a directory of fixture files, 2026-10-02:

| probe | zsh |
| --- | --- |
| `#compdef foo bar=baz -p "x*"` | `_comps[foo]`, `_comps[bar]` the file's name; `_services[bar]=baz`; `_patcomps["x*"]` — split on blanks, quotes kept |
| `#compdef -P pat qq` | both words to `_postpatcomps` |
| `#autoload` | autoloaded, nothing recorded |
| a file with neither line | not autoloaded |
| `${(t)_comps}` | `association-hideval` |
| afterwards | `compdef`, `compaudit`, `compdump` are functions; `compinit` is autoloadable again |
| `compdef fn cmd`, `-n`, `-d`, `name=service` | set, keep, remove, set with a service |
| a world-writable `$fpath` entry | `compaudit`: the heading on stderr, the directory on stdout, 1 |
| the same under `compinit`, no `-u`/`-i`, no terminal | `not interactive and can't open terminal`, a blank line, `compinit: initialization aborted`, 1, and no `compinit` left |
| `compinit -i` | the insecure directory left out |
| `complete -F f c1`, `complete -o nospace -W 'q r' c2` | `_comps[c2]` is `_bash_complete -o nospace -W q\ r`; `complete -p` lists them back |
| `compgen -W 'alpha beta' -- al` | `alpha` and `beta`: the word is **ignored** |
| `compgen -P pre`/`-S suf` | each result between them |
| `compgen` with nothing to list | one empty line |

Two divergences, both deliberate:

* **No dump is written.** zsh writes `${ZDOTDIR:-$HOME}/.zcompdump`, or the
  `-d` file, and reads it back next time; a dump written here would be read
  by the other shell too, and the two must not trade files. `-d`, `-D` and
  `-C` are taken and change nothing else, and `compdump` does nothing at 0.
* **Tab is not rebound to `_main_complete`**, which this shell does not ship,
  and `_bash_complete` completes nothing: this shell's own completion stays,
  and what compinit records is what a startup file and a plugin read back.

And one wording: an `-F` function `compgen` cannot find is the shell's
`command not found`, located at `compgen`'s own line where zsh's names an
anonymous function's.

## The contrib widgets: written from the manual, graded against the binary

#5894 asked for the functions a real `~/.zshrc` reaches that this library did
not have, most used first. Each is written from `zshcontrib(1)` and from black-box
runs of zsh 5.9.2 through a pseudo-terminal — typing the keys, reading the line
back through a widget that writes `$BUFFER` and `$CURSOR` to a file — and never
from zsh's own function files. Where a widget needed an editor action this shell
did not have, the action came first, as its own change: the beginning search,
`up-line`, `down-line` and `copy-region-as-kill` (#5910), and `$CUTBUFFER`
(#5916).

Every row in the tables below was re-run against this shell's copy through the
same probe and gave the same line and cursor, unless the row says otherwise.
`cmd/zsh/contribwidgetspty_test.go` drives the shipped files through a real
session for the rows that decide each widget's shape.

### `up-line-or-beginning-search`, `down-line-or-beginning-search`

Up (or down) a line inside a line that holds several; from the first (or last)
line, through history to the nearest entry beginning with what is before the
cursor. History `: echo apple`, `: ls one`, `: echo banana`, `: echo apple`,
`: ls two`, `: echo cherry`, typed:

| typed, then | line, cursor |
| --- | --- |
| `: ec`, up | `: echo cherry`, 13 |
| up | `: echo apple`, 12 — the fourth entry |
| up | `: echo banana`, 13 |
| up, up | `: echo apple` (the first), then the same again: nothing older |
| down | `: echo apple` (the fourth) — forward from banana |
| down, down | `: echo cherry`, then `: ec`, 4: the typed line, cursor where it was |
| `zzz`, cursor at 1, up | `zzz`, 3 — the cursor goes to the end anyway |
| `zzz`, cursor at 1, down | `zzz`, 1 — and here it does not |
| `echo abc⏎def ghi`, cursor 13, up | cursor 4: up a line, the column kept |
| up again | cursor 8: a search for `echo`, nothing, the end of the line |
| `: ec⏎q`, cursor 1, up | `: echo cherry`: on the first line it searches |

What is searched for is fixed by the first press of a run — the next press of
either widget restores the cursor to where that one started before searching
again — and a move to another line of the buffer is not part of a run. The
partner differs in one thing, measured rather than mirrored: it moves the cursor
to the end of the line only when something matched.

### `edit-command-line`

The line goes to a temporary file, an editor is run on it with the terminal as
its input, and what the file holds afterwards is the line.

| probe | zsh 5.9.2 |
| --- | --- |
| the editor | the `editor` style in `:zle:<widget>`, else `$VISUAL`, else `$EDITOR`, else `vi`; the variables split into words |
| the file | `${TMPPREFIX:-/tmp/zsh}`, six random characters, `.zsh`; the line and a newline; removed afterwards |
| an editor whose name contains `vim` | `-c 'normal! <N>go' --` before the file, N the cursor's byte offset plus one |
| one whose name contains `emacs` | `+<line>:<column>`, the column in bytes from 1 |
| any other — `nano`, `vi`, `mg`, `hx` | the file alone |
| `Vim`, `EMACS` | the file alone: the match is case-sensitive |
| what comes back | every trailing newline removed; leading ones kept |
| the cursor | the same offset, clamped to the new line |
| the editor exits 1 | the file is read back anyway |

**One difference, and it is the shell's.** At a continuation prompt zsh hands
the editor `$PREBUFFER` and the line together and puts the whole back as one
line. This shell has no `$PREBUFFER` (#5931), so only the current line is
edited.

### `url-quote-magic`, `urlglobber`

Replaces `self-insert`. A character typed into a word that looks like a URL goes
in behind a backslash when the shell would read it specially. Typed one
character at a time:

| typed | zsh 5.9.2 |
| --- | --- |
| `curl http://x.y/?a=1&b=2` | `curl http://x.y/\?a\=1\&b\=2` |
| `curl "http://x.y/?a=1`, `curl 'http://x/?a` | unchanged: an open quote |
| `curl x.y/?a=1`, `curl mailto:x?a`, `curl HTTP://x/?a` | unchanged: no listed scheme, case and all |
| `curl "http"://x/?` | `\?`: the scheme is read from the word unquoted |
| `echo $(curl http://x/?`, `a=http://x/?` | unchanged: the word is not a URL |
| `curl <http://x/?` | `\?`: the redirection is a word of its own |
| `noglob curl ftp://h/*?;` | `*?\;`: a globbing command and a local scheme, separators only |
| `noglob curl http://x/*?;` | `\*\?\;`: http is not local |
| `ls; noglob curl ftp://h/*;` | `\*\;`: the command name is the line's first word |
| `curl http://x/a\?`, `curl http://x/\\?` | `\?`, `\\\?`: an escaped character is left alone |

With the default styles the characters quoted are `!#&()*;<=>?[]^{|}~`; `"$%'+,-./:@_` and the backquote are not. Two styles decide it per scheme
— `url-metas` and `url-seps` in `:url-quote-magic:<scheme>` — and a character
is quoted only if it is in one of them **and** the shell's own `(q)` would quote
it: with `url-metas` set to `%?`, a typed `%` is still left bare. The defaults
are put in place when the file is loaded and do not replace a style already
set, which is measured: a `url-metas` set before the first keystroke survives.

`urlglobber cmd args…`, reached as `globurl`, globs the path part of each local
URL — `ftp://h/p/*.txt` becomes the files under `h/p/`, a host of `localhost`
or nothing means the path from `/` — passes other URLs with a listed scheme as
they are, and globs every other argument as usual. A `file://` URL is globbed
whole and so matches nothing, which is what zsh does.

**One difference, and it is the shell's.** zsh's `(q)` quotes the history
character at an interactive prompt and this shell's does not (#5933), so a typed
`!` stays bare here where zsh writes `\!`. The function asks `(q)`, as zsh's
does, and will follow when the flag does.

#5879 reported the opposite failure — every character of a URL quoted — with
zsh's own `url-quote-magic` read through `$fpath`. With this one, the same
keystrokes give zsh's line.

## What is not shipped, and why

#5894 is shipping the contrib functions in batches, most used first, and the
ones not yet written are listed here until they are. Next: `select-word-style`
and the `-match` widgets, then `bracketed-paste-magic`, `zmv`, `zargs` and
`run-help`, then `promptinit`, `zcalc` and `zed`, then `vcs_info`.

## Loading, and aliases

A function file is read with the alias table live unless `-U` said
otherwise, which is measured and is the reason every real script writes
`autoload -Uz`: an alias a startup file defined before the call would
otherwise rewrite the body of the function it is loading. That is a
property of the engine (`dialect/zsh/autoload.go`, #1993) and not of these
files — nothing a function file can contain protects it — so the files
carry no defense and the test suite pins the engine's from this side: an
alias that would visibly corrupt one of these bodies is defined, the
function is loaded with `-Uz`, and the body has to be unaffected.
