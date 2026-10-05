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
The `url-globbers` default is `noglob` and every alias, ordinary or global, whose
text runs `noglob`, `urlglobber` or `globurl` with something after it, so
`alias mmv='noglob zmv'` counts and `alias ng=noglob` does not. That is what
zsh's default answers and it is computed here by a function of this file's own.

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

### `select-word-style` and the `-match` widgets

`select-word-style STYLE` sets three styles in the context `:zle:*` and, the
first time, puts eight widgets in place of the editor's own: `forward-word`,
`backward-word`, `kill-word`, `backward-kill-word`, `transpose-words`,
`capitalize-word`, `up-case-word` and `down-case-word`, each made from its
`-match` function. What each letter sets, read back with `zstyle -L`:

| style | `word-style` | `word-chars` | `skip-whitespace-first` |
| --- | --- | --- | --- |
| `b` | `standard` | `''` | `true` |
| `n` | `standard` | `$WORDCHARS` | `false` |
| `s` | `shell` | — | `false` |
| `w` | `space` | — | `false` |
| `d` | left alone | deleted | deleted |
| `B` `N` `S` `W` | the same, with `-subword` | | |

Nothing else is deleted, so `bash` then `shell` leaves `word-chars ''`. Any other
argument, `q` included, prints the usage on standard error and answers 1. Run as
a widget with no argument it reads one letter. Return explains each letter and
asks again, and a key that is not a letter is skipped. The usage and the
question are worded here, not copied: zsh's text is its documentation, so what
is matched is the stream, the status and the synopsis line.

All eight are written over `match-words-by-style`, which splits the line into
seven parts around the cursor: start, the word before it, what lies between that
word and the cursor, what lies at the cursor before the next word (the
`skip-chars` included), that word, what follows it, and the rest. Each widget is
then a formula over the parts:

| widget | does |
| --- | --- |
| `forward-word` | moves past the blanks at the cursor if there are any, else past the word and what follows it; with `skip-whitespace-first`, past the blanks *and* the word |
| `backward-word` | moves back over what lies between and the word before |
| `kill-word`, `backward-kill-word` | kill the same text; straight after a kill, the kill joins it (after it for forward, in front for backward) |
| `transpose-words` | the line becomes start, word after, between, at, word before, and the rest, with the cursor after the word before |
| the three case widgets | pass the blanks and change the word after them with `(C)`, `(U)` or `(L)` |

The split was learned by calling zsh's own `match-words-by-style` from a widget
and printing the seven parts: 177 rows over five buffers, the cursor at every
interesting place, eight styles. The widgets were measured by calling each from a
widget and recording the line, the cursor, `$CUTBUFFER` and the status: 544 rows
across `bash`, `normal`, `whitespace` and `B`, plus the line's edges. Both tables
are committed as `dialect/zsh/testdata/wordstyle-*.tsv`, and
`dialect/zsh/wordstyle_test.go` runs every row through this shell's copies.
`cmd/zsh/wordstylepty_test.go` drives the keys through a real session. Measured
beyond the manual:

* with no word before the cursor, everything before it goes into the start and
  "between" is empty. With no word after it, everything after goes into "at
  the cursor";
* subwords are split on each side of the cursor separately. A subword starts
  at an upper-case character that follows a non-upper-case one, or that is
  followed by one, so `XMLHttpReq` is `XML`, `Http`, `Req`. When the cursor is
  inside a subword, "what follows the word" is empty and that text joins the
  rest;
* `down-case-word` answers 0 where there is no word; its two fellows answer 1;
* `delete-whole-word-match` removes the word the cursor is inside or at the
  start of, or the one it has just passed at the end of the line, and does
  nothing on the blanks between two words. Made into `kill-whole-word-match`,
  what it removes becomes the kill.

**A difference that is deliberate.** In the `shell` style, when the cursor is
inside a word, zsh puts the character after that word into the word as well:
from inside `foo-bar` in `foo-bar baz`, `kill-word` kills `ar ` and the line
becomes `foo-bbaz`; `forward-word` from inside `e\ f;g` stops after the `;`.
It happens in every inside-a-word row and in none where the cursor is between
words, and joining two words with a kill is not a behavior anyone could want, so
it is not reproduced. This shell's `shell` style splits at the word's own end.
So the `shell` and `S` rows are measured and left out of the committed tables:
31 of the 96 `shell` rows and 30 of the 96 `S` rows leave a different line, every one for this reason; the rest leave the same line.
The `default` rows are left out too, because `d` leaves the word style alone and
so those rows measure whatever style the row before them set.

**Three differences that are the shell's**, filed rather than worked around:
`zle w` from inside another widget answers 0 whatever `w`'s function returned
(#5939), so the widgets' own statuses reach a caller only when called directly;
assigning `NUMERIC` in a widget does nothing (#5941), so a negative count,
which the widgets hand to their opposite as a positive one, does not arrive;
and `zle -M` is not implemented (#5942), so the widget form of
`select-word-style` reads its letter without showing the question.

`select-word-match`, the vi text object, is not shipped. It selects a region
with the mark, and this editor has neither a mark nor a region to select.

### `bracketed-paste-magic`, `backward-extend-paste`, `quote-paste`

Replaces `bracketed-paste`. The paste is read with `zle .bracketed-paste
PASTED`, each key of it is replayed through `zle .read-command`, and a key bound
to one of the `active-widgets` runs that widget (`self-*` by default, so a
`self-insert` replaced by `url-quote-magic` quotes a pasted URL). Every other key
goes in as itself. The result goes back into the line as a paste: pushed back
behind the closing marker and read by the editor's own paste, so it is drawn and
undone the way a paste is. Pastes measured through a pty against zsh 5.9.2:

| setup, typed, pasted | zsh 5.9.2 |
| --- | --- |
| `x `, paste `echo a⏎echo b` | both lines in the line, nothing run |
| with `url-quote-magic`, paste `curl http://x/?a=1&b` | `curl http://x/\?a\=1\&b` |
| after the paste, one undo | the line as it was before the paste |
| `active-widgets ''` | the paste goes in literally |
| the style deleted | still `self-*`: deleting it is not emptying it |
| `inactive-keys =` | `=` goes in as itself, the rest through the widget |
| `active-widgets 'self-*' undefined-key` | a key sequence bound to nothing is dropped |
| a `paste-init` function | sees `PASTED` and the line; what it leaves in `PASTED` is what goes in |
| a `paste-finish` function | the same, after the widgets |
| `paste-init backward-extend-paste`, typed `curl http://x`, paste `/?a` | `curl http://x/\?a`: the word on the line is handed to the widgets with the paste |
| `paste-finish quote-paste`, `quote-style qqq` | the paste goes in quoted with `(qqq)` |
| `é ü` | `é ü` |

Loading the file sets `active-widgets` to `self-*` if nothing has set it, and
defines the two helpers.

Two differences, neither one in this file. **The editor drops control
characters from a paste** (`repl/paste.go` says why: there is no caret notation
to draw them in), so a pasted `^A` or tab never reaches the widgets. zsh keeps
them. And an `inactive-keys` entry holding a pattern character — `?`, `*`,
`[` — has no effect in zsh, where `=` and `&` work. Here they are taken as the
literal key sequences the manual says they are.

### `zmv`

`zmv [ -finqQsvwW ] [ -C | -L | -M | -{p|P} program ] [ -o optstring ] srcpat
dest` renames every file matching `srcpat` to `dest`, expanded once per file with
the parenthesized parts as `$1`, `$2`, … and the name as `$f`. Measured by running
zsh's own `zmv` in scratch directories and recording what it printed and what the
directory held afterwards: 44 cases, committed as `dialect/zsh/testdata/zmv.tsv`
and run against this copy by `dialect/zsh/contrib2_test.go`. Among them:

| case | zsh 5.9.2 |
| --- | --- |
| `zmv -n '(*).lis' '$1.txt'` | `mv -- a.lis a.txt`, one line a file, nothing done |
| `-C`, `-L`, `-Ls` | `cp --`, `ln --` (a hard link), `ln -s --` |
| `-o '-v -i'` | `mv -v -i -- a.lis a.txt` |
| `-p echo`, `-P echo` | `echo -- a.lis a.txt`, `echo a.lis a.txt` |
| a name `dest` leaves unchanged | passed over, status 0 |
| two files to one name | `zmv: error(s) in substitution:`, then `b.lis and a.lis both map to same.txt`, status 1, nothing done |
| an existing file | `file exists: b.txt`, unless `-f` |
| an empty result | `` `a.lis' expanded to an empty string `` |
| `-w '**/*.lis' '$1$2.txt'`, `-W '*.lis' '*.txt'` | the wildcards become the parts |
| `(**/)(*)` | recursive, and deepest first: a directory's contents before the directory |
| a name with a space, under `-n` | `mv -- 'a b.lis' 'a b.txt'`: quoted only where needed |
| `-i` | the line, `Execute? `, one key; `y` runs it |
| `A.LIS` to `a.lis` on a filesystem that ignores case | done: the "existing" file is the same file |
| an unknown option | `zmv: unrecognized option: -z`, status 1 |

Differences: the usage is this file's own words, for the reason given for
`select-word-style`. The line number in `zmv:N: no matches found` is this file's.
And the deepest-first order is worked out here. The `od` glob qualifier that
gives it arrived after the function was written (#5951).

### `zargs`

`zargs [ option … -- ] [ input … ] [ -- command [ arg … ] ]` is xargs with its
input on the command line. Measured by running zsh's own `zargs`: 37 cases,
committed as `dialect/zsh/testdata/zargs.tsv` and run against this copy. The
shape of it:

* the default command is `print -r --`, and its words are not counted by `-n`;
* `-n N` counts the args with the inputs, and a run that would hold no input
  is `zargs: argument list too long` and status 1;
* `-l`/`-L N` counts inputs only; `-l` alone, or followed by something that is
  not a number, is 1;
* `-i`/`-I str`/`--replace` put each input where the first `{}` (or `str`) is
  in each arg, one input a run;
* `-e` alone makes every word an input; `--eof=` makes an empty word the end;
* an option this does not know is not an option: it and what follows are
  inputs;
* the status is 123 if any run answered 1 to 125, 124 for 255, 125 for a run
  killed by a signal, 126 or 127 as the run answered; the last three stop the
  runs after.

Where zsh's own is broken, this one is not, on the `is-at-least` precedent: zsh's
`--max-args=N` fails on the `=` (`bad math expression`); its `--null` does not
split where `-0` does; and its `-s` is not monotonic (`-s 14` fits two inputs a
run and `-s 20` refuses every one, and `-s 3` loops forever printing). Here `-s`
counts the run's characters, words joined by single spaces. The usage,
`--version` and the `-p` refusal without a terminal are worded here, and a
command that is not found is reported at this file's location, where zsh's
reports it from an `eval`.

### `run-help`

Help for a command word. The file of that name in `$HELPDIR` goes through
`${PAGER:-more}`, by name. Otherwise each meaning `whence -va` gives is printed and
followed by the manual that covers it:

| word | zsh 5.9.2 |
| --- | --- |
| a help file | `$PAGER $HELPDIR/word` |
| a program | `ls is /bin/ls`, then `man ls`, with every word given (`man sudo ls`) |
| an alias | `ll is an alias for ls -l`, then help for `ls` |
| a function, or not found | the line, then `man word` |
| a builtin | `echo is a shell builtin`, then `man zshbuiltins` |
| a reserved word | `typeset is a reserved word`, then `man zshmisc` |
| more than one meaning | `Press any key for more help or q to quit`, in reverse video, between them; with no terminal, `not interactive and can't open terminal`, and on |
| no word | `Here is a list of topics for which special help is available:`, a blank line, and the topics in columns two wider than the widest |

A `run-help-<word>` function, where one is defined, is given the rest of the
words in place of the manual. zsh's own answers that case with `shift count
must be <= $#` when called with words, so there was no behavior to copy.

Differences that are the shell's: for a word holding a slash, this shell's
`whence -va` describes the file where zsh's says `not found` (#5956). The topic
columns are laid out here because `print -c` is not implemented (#5957), and the
rest of the words are passed with `shift` because `"${@[2,-1]}"` gives one empty
word when there is nothing to give (#5955).

The default `run-help` alias for `man` stays, as it is in zsh, so a startup file
switches to this function with `unalias run-help` and `autoload -Uz run-help`.
This shell ships no help files, so with `HELPDIR` unset every word goes to its
manual.

### `promptinit`, and the `off`, `default` and `restore` themes

`promptinit` finds every `prompt_<name>_setup` file on `$fpath`, autoloads it,
lists the names in `prompt_themes`, and defines `prompt`. Measured with three
themes of the test's own first on `$fpath` — 25 rows, committed as
`dialect/zsh/testdata/promptinit.tsv` and run against this copy by
`dialect/zsh/contrib3_test.go`. Among them:

| call | zsh 5.9.2 |
| --- | --- |
| `prompt tiny x` | the setup run with `x`; `prompt_theme` is `tiny x` |
| what the setup leaves in `prompt_opts` | turned on, and the rest of `bang cr percent sp subst` off; it starts as `cr percent sp` |
| `prompt tiny; prompt zap` | tiny's `prompt_cleanup` commands run, and its `prompt_tiny_*` hooks removed |
| `prompt restore` | the prompts and their options as they were before the first theme |
| `prompt off` | `%# `, `> `, `?# `, `+> `, and no right-hand prompt |
| `prompt default` | `%m%# `, `%_> `, `?# `, `+%N:%i> ` |
| `prompt -c` | `Current prompt theme with parameters is:` and the theme, or `Current prompt is not a theme.` |
| `prompt -h theme` | the theme's setup run in a subshell, then its `prompt_<name>_help` under `Help for <name> theme:`, or `No help available for <name> theme.`; and how to preview, try and keep it |
| `prompt -p theme …` | the same subshell; the theme's own `prompt_<name>_preview`, or `<name> theme:` and its `PS1` with `command arg1 arg2 ... argn` after it, a carriage return first when `cr` is in `prompt_opts`; a line putting the colors back before the first and after each |
| `prompt -p nosuch` | `Unknown theme: nosuch` between those lines |
| a theme that is not there | the usage, at status 0 |

Differences: the usage, and what `-s` says, are this file's own words, for the
reason given for `select-word-style`. A bad option is reported as
`prompt: bad option: -z`, where zsh's names an inner function of its own. This
shell ships the three utility themes and none of the fifteen decorative
ones (`adam1`, `bart`, `walters` and the rest): each is a design somebody
drew, and a theme a person wants is a file they put on `$fpath`, which
`promptinit` finds the same way. `prompt -p` with no themes previews every
theme but the current one, as zsh's does; zsh's waits on the terminal between
its own themes, so that call has no row.

### `zcalc`, `zmathfuncdef`

`zcalc [ -erf ] [ -#base ] [ expression … ]` is a calculator on the shell's
arithmetic. Measured by running zsh's own `zcalc -e` over 137 calls, committed
as `dialect/zsh/testdata/zcalc.tsv`, and through a pseudo-terminal for the
session (`cmd/zsh/zcalcpty_test.go`). The shape of it:

* each line's result is `$N`, N the number in the prompt, and `ans`;
* a result the arithmetic writes with no point in it is printed through
  `printf %d` — an integer is itself, and `1e100` is the largest integer —
  one written `7.` as it is, and the rest through `%g` (`0.333333`);
* `:sci N`, `:fix N`, `:eng N` are `%.Ng`, `%.Nf`, `%.NE`, the digits
  required and the blanks around them not; `:raw` is the value as it is
  stored; `:norm` goes back. Each prints `ans` in the new form;
* `:local names` declares them local to the calculator, and `:f`, `:func` or
  `:function name body` hands its words to `zmathfuncdef`; any other line
  starting with `:` is zsh's one-line refusal;
* a line that is one parameter — `$3`, `$name`, `${1}` — prints its value as
  stored and is not a result, so `$N` past the last result prints an empty
  line;
* the arguments of a run without `-e` are evaluated, and are the first
  results, shown as `1> value`;
* an empty line, `q` or `:q` ends it, and so does an empty argument under
  `-e`; a line ending in a backslash, or leaving a `(` open, is carried on
  behind `...` and the prompt;
* `-r` is reverse Polish, with the operands on `stack`.

`zmathfuncdef name body` makes a math function of an expression. How many
arguments it takes is read from the body, which the manual says must hold to
its forms: the mandatory ones run from `$1` for as long as each next number is
there as `$N` or `${N}` — `$3` alone takes none, and `$10` is not `$1` — and
the optional ones carry on for as long as each next number is `${N:-…}`. All
17 measured bodies are in the test. `zmathfuncdef name` removes it.

Differences: the session reads its lines with `vared`, whose prompt is
expanded here before it is handed over (#5965) and whose result is copied out
as a string (#5966); neither shows. `zmathfuncdef` with no arguments lists the
math functions in a form that defines them again, as the manual says; zsh's
own prints nothing, measured with two defined.

### `vcs_info`

`vcs_info [ user-context ]` sets `vcs_info_msg_0_`, `vcs_info_msg_1_`, … from
the repository the current directory is in, for a prompt to show. It is
configured with `zstyle` under `:vcs_info:<vcs>:<user-context>:<repo-root-name>`,
and running it defines `vcs_info_hookadd`, `vcs_info_hookdel`,
`vcs_info_lastmsg`, `vcs_info_printsys` and `vcs_info_setsys`. To measure it,
zsh's own `vcs_info` was run in 31 git repositories built in fixed states:
clean, changed, unborn, detached, bare, a linked worktree, inside `.git`, and
part-way through a merge, rebases of three kinds, an am, cherry-picks, a
revert and a bisect. That is 160 rows, committed as
`dialect/zsh/testdata/vcs_info.tsv` beside `vcs_info-fixtures.sh`, which
builds the repositories, and `dialect/zsh/vcsinfo_test.go` runs them against
this copy. git 2.56.0 and Apple git 2.54.0 agreed on every row. The shape of
it:

| what | zsh 5.9.2 |
| --- | --- |
| the defaults | formats ` (%s)-[%b]%u%c-`, actionformats ` (%s)-[%b\|%a]%u%c-`, nvcsformats empty, max-exports 2 |
| before anything is looked at | every `vcs_info_msg_N_` is unset, and max-exports of them set again, empty |
| `%b` | the branch; during a rebase, the branch being rebased; detached, the name `git name-rev` gives (`main~1`, `tags/v1`), or the short hash when nothing names it |
| `%a` | `rebase-i` for a rebase-merge directory holding `interactive` (so `rebase --merge` too) and `rebase-m` otherwise, `rebase` or `am` for a rebase-apply one, `merge`, `cherry`, `cherry-seq`, `bisect`; nothing for a revert |
| `%i` | the full hash, with `get-revision`; nothing on an unborn branch |
| `%c`, `%u` | with `check-for-changes`, stagedstr (`S`) for a change in the index and unstagedstr (`U`) for one in the tree; an untracked file is neither; `check-for-staged-changes` gives `%c` alone |
| `%R`, `%r`, `%S` | the top of the work tree, its last part, and the directory below it (`.` at the top); inside `.git` or a bare repository, the git directory |
| `%m` | the patch in progress, `%p (%n applied)` by default: the commit and its subject for a merge or a cherry-pick, the done list for a rebase-merge, the patch files for a rebase-apply; nothing otherwise |
| patch-format | `%p` the newest applied, `%u` and `%c` the unapplied count, `%n` the applied count, `%a` the two together |
| `enable`, `disable`, `disable-patterns` | as the manual says; a directory they rule out gets nvcsformats |
| a context's repo-root-name | the repository's last part, for the format styles |
| `zformat` | does the expanding, so `%10.10b` pads and cuts, `%%` is `%`, and an unknown `%x` stays |
| hooks | `start-up` (ret 1 ends, 2 is no system), `pre-get-data` (the same), `no-vcs`, `post-backend`, `pre-addon-quilt`, `set-message` once for each message with the message number and format, `gen-applied-string`, `gen-unapplied-string` and `set-patch-format`, with `hook_com` holding what the manual lists, each key with its `_orig`; a change made by `set-message` lasts into the next message |
| the order | the static functions (`vcs_info_hookadd`) before the ones the hooks style names, not after as the manual says; a function that returns nonzero ends the chain |
| no system found | the `no-vcs` hook runs, and nvcsformats is looked up, with `-quilt-` as the system |
| `vcs_info_lastmsg` | `$vcs_info_msg_N_: "…"` for each, prompt escapes expanded unless `use-prompt-escapes` is false |

Differences, each on purpose:

* **git is the one backend.** A directory under Mercurial, Subversion,
  Bazaar or the rest reads as a directory under no system, and
  `vcs_info_printsys` lists only `git`, with a header in this file's own
  words. Quilt is not supported: `use-quilt` does nothing and `%Q` is always
  empty. The flavors zsh detects inside git (`git-svn`, `git-p4`) are reported
  as plain `git`.
* **Where zsh's copy breaks, this one does not**, on the `is-at-least`
  precedent. `enable NONE` with an nvcsformats set, and a lowercase `none`
  (which the manual says is allowed), both fail in zsh's copy trying to
  typeset `vcs_info_msg_-1_`. Here they leave the messages empty. And with
  more nvcsformats than max-exports, zsh's copy sets one fewer than
  max-exports. Here it sets max-exports of them.
* The `max-exports` warning, the usage of `vcs_info_hookadd` and
  `vcs_info_hookdel`, and the `debug` lines are in this file's own words.
  They go where zsh's go, which is standard output for all three.
* nopatch-format, used when a patch system is active with nothing applied
  yet, defaults to `no patch applied`. No state could be built that shows
  zsh's default.

Found on the way: a `:#` filter over a nested, quoted command substitution
subscripts as a scalar (#5978), which the function works around.

### `cdr`, `chpwd_recent_dirs` and their helpers

`chpwd_recent_dirs` is a chpwd hook that keeps a list of the directories
changed to, newest first, in `${ZDOTDIR:-$HOME}/.chpwd-recent-dirs`. `cdr N`
goes back to the Nth of them. `chpwd_recent_filehandler` reads and writes the
file, and `chpwd_recent_add` puts a directory at the front. The hook keeps a
change only in an interactive shell, so these were measured by typing into
`zsh -f -i` on a pipe. The 36 rows are committed as `cmd/zsh/testdata/cdr.tsv`
and run against this copy through the same kind of session by
`cmd/zsh/cdr_test.go`.

| what | zsh 5.9.2 |
| --- | --- |
| which changes are kept | a `cd` at the prompt, with the hook called as a hook or from a `chpwd` function; not one made by a function the line called, by `eval`, in a subshell, or with `cd -q` |
| the file | one directory a line, quoted as `$'…'`; a directory already there moves to the front |
| `cdr -l` | `N` padded to four, a space, the directory quoted with `~` put back; the current directory is not offered |
| `cdr`, `cdr N` | the first, or the Nth |
| `cdr -r` | `reply` set to the list, nothing printed; without `-r`, `reply` is left alone |
| `cdr -P pattern` | every directory the pattern matches taken out; `-p` is the same when standard output is not a terminal |
| `recent-dirs-max` | 20 by default; 0 or less, no limit |
| `recent-dirs-file` | several files: the first is written, the rest are read after it, and `+` is the default file |
| `recent-dirs-prune parent` | a change straight down drops the directory it came from |
| `recent-dirs-default` | a word that is not a number, or more than one, goes to `cd` |

Differences:
* **`pattern:` prune elements are zsh patterns here, as the manual says.**
  zsh's copy hands them to the regular-expression matcher, which on this
  machine refuses the manual's own example `pattern:/tmp(|/*)` (`failed to
  compile regex`) and keeps the directory anyway.
* When there are fewer directories than asked for, zsh says `Not enough
  directories (-1 possibilities)` with -1 whatever the count. Here the
  message is this file's own and gives the real count. The usage is this
  file's own too. Both go to the same streams as zsh's: standard error for
  the first, standard output for the usage.
* `cdr -l` puts `~` back for `$HOME` only. The `(D)` flag that would also
  put back a named directory is not implemented yet (#5980).
* With `recent-dirs-pushd`, zsh's `pushd` prints the stack in an interactive
  shell. This shell's does not yet (#5982).
* `cdr -e` edits the list with `vared`. It has no row, because there is no
  terminal to edit at.

### `catch`, `throw`

Exceptions on top of `{ … } always { … }`. `throw name` raises an error
carrying the name, and `catch pattern` in the always part answers 0 if the
name matches, clearing the error so the script carries on. 15 rows, measured
against zsh's own copies, are committed in `dialect/zsh/testdata/contrib5.tsv`
together with the rows below them. What they show:

* an exception thrown from a nested function lands in the nearest always
  part;
* a `throw` in the always part outlives it, as an error in the try part
  would, so one caught there can be thrown on;
* `catch` outside an always part, or after a try part that raised nothing,
  answers 1 and sets nothing;
* after a catch, `CAUGHT` holds the name and `EXCEPTION` is unset;
* an unhandled exception ends the script, at status 1;
* the `noglob` alias for `catch` exists only once `catch` has run, so before
  that an unquoted pattern is globbed.

### `zmathfunc`, `zstyle+`

`zmathfunc` defines `min` and `max` (one argument or more) and `sum` (none or
more), for integers and floats mixed. Of two equal arguments the first wins:
`min(2, 2.0)` is `2`. `zstyle+ context style value + subcontext style value …`
sets each style with the first context, plus the word after a `+` when there
is one. As in zsh, the `+` has to be a word of its own: the manual's
`+':baz'`, written as one word, is just a value.

## What is not shipped, and why

#5894 is shipping the contrib functions in batches, most used first, and the
ones not yet written are listed here until they are. Next: the smaller
widgets, `history-search-end`, `smart-insert-last-word`, `copy-earlier-word`
and `incarg`.

`zed` is not shipped yet. It edits a file or a function in the line editor
under a keymap of its own, built from `main` with `bindkey -N` and selected
with `bindkey -A`, and this shell refuses both (#5969).

`select-word-match` is left out for good, for the reason given with the word
styles above: it needs a mark and a region, and this editor has neither.

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
