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

One row is a real gap rather than a cosmetic one: `add-zsh-hook -k` passes
`-k` to `autoload`, and this shell's `autoload` names `-k` as not
implemented (`dialect/zsh/autoload.go`, `autoloadUnimplemented`). The
complaint is correct and it is the shell's, not the function's — passing
the letter and saying nothing would be worse.

What arrives at the *caller*, though, is a status, and that one disagrees.
`-z` and `-k` name two different autoload styles, so zsh refuses the pair:

| | zsh 5.9.2 | ours |
| --- | --- | --- |
| `add-zsh-hook -k precmd kf` | `0`, silent | `0`, and the complaint |
| `add-zsh-hook -Uzk precmd kf2` | **`1`** | **`0`** |
| `add-zsh-hook -q precmd kf3` | `1` | `1` |

The hook is installed either way in both shells, so the status is the whole
of the difference — which is why the corpus row suppresses standard error:
the diagnostic names the line the call stands on in the function file, and
that is a fact about whose file it is rather than about the behavior.
**#2149.**

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

## What is not shipped, and why

`compinit` — and with it `compdef`, `compdump` and the completion system
it initializes — is **out of scope on purpose**. It is an order of
magnitude larger than the four above put together, and it exists to drive
a completion system this shell does not have yet. A real startup file that
calls it still gets `compinit: function definition file not found`, and
that is an honest gap rather than a silent one.

`vcs_info` is the same answer for the same reason, and it turned up in the
measurement rather than in the issue: it is a VCS status subsystem with a
style system under it, not a function. One call in this machine's rc.

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
