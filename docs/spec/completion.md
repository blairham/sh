# Completion

What the Tab key does to the word under the cursor.

Everything here was measured by driving Tab through a pseudo-terminal
against `bash 5.3.15` (`--norc --noprofile -i`) and `zsh 5.9.2` (`-f -i`)
on `darwin/arm64`, in a fixture directory of deliberately awkward names.
Each run typed a line, pressed Tab, typed a marker so a trailing space
would be visible, and read the resulting buffer back off the terminal.
No shell's source was consulted; see `CLEANROOM.md`.

This is the **built-in** behavior. A public seam through which something
outside the shell contributes candidates is a separate question and is
not specified here.

## The two kinds, and what decides between them

The first word of a command completes against commands; anything after
it completes against filenames. **Except when the word contains a
slash** — then it is a path even in command position, and only the
things that could actually run are offered: directories, and files with
an execute bit.

Measured, in a directory holding a non-executable `plain.txt` and a
directory `onlydir`:

    ./pl<TAB>    bash: (nothing)   zsh: (nothing)
    ./onl<TAB>   bash: ./onlydir/  zsh: ./onlydir/

A command word with no slash is never a filename: `pla<TAB>` at the
start of a line offers nothing, though `plain.txt` is right there.

## Where the word begins

The word is bounded by whitespace and by the characters that end a word
in the grammar — `; | & < > ( )` — and neither boundary counts when it
is quoted or backslash-escaped. `a\ b` is one word with a space in it,
and so is `"a b"`.

    : file o<TAB>      bash: : file onlydir/   zsh: : file onlydir/
    : file\ o<TAB>     bash: : file\ one.txt   zsh: : file\ one.txt
    : "file o<TAB>     bash: : "file one.txt"  zsh: : "file one.txt"

The first of those is not a failure to complete `file one.txt`: the
unescaped space ended the word, so `o` is what was completed, and it
became `onlydir/`.

Both shells break the word at `;` — `: a;pla<TAB>` completes nothing,
because `pla` is then in command position and no command is named that.

bash breaks words at more than this: readline's word-break set also
holds `@ = : "` and `'`, which is why bash escapes those characters and
zsh does not. The core follows zsh and breaks only at whitespace and at
the grammar's own word-enders, because breaking at `:` and `=` splits
`--opt=value` and `host:path` in the middle of a path someone is typing.

## What is inserted, and what follows it

A single match replaces the word. What follows it says whether the word
is finished:

| match is | suffix | measured |
| --- | --- | --- |
| a file | a space | `: pla<TAB>` → `: plain.txt ` |
| a directory | `/` and nothing else | `: only<TAB>` → `: onlydir/` |

The slash with no space is the whole point: the next Tab continues into
the directory. `: sub/nes<TAB>` → `: sub/nested.txt `.

Several matches fill in as far as they agree and stop, with no suffix:

    : file<TAB>   bash: : file\    zsh: : file\
    : di<TAB>     bash: : dir      zsh: : dir

The second is worth reading twice. `dir` is a directory and `dirfile` is
a file; the common prefix is `dir`, and **no slash is added**, because
`dir` is not yet a decision.

The agreement is computed on the names and not on the escaped text. Two
names `x$a` and `x&b` agree on `x`, and the inserted text is `x` — never
the `x\` that a naive prefix of `x\$a` and `x\&b` would give.

Nothing at all matches: the line is left alone.

## Escaping

The inserted text has to survive being read back by the parser, so a
character that means something to the grammar is escaped, and how it is
escaped depends on the quoting the word is already inside.

**Outside quotes**, a backslash. Measured with one file per character,
completed uniquely; both shells escape all of:

    space  tab  !  "  $  &  '  (  )  *  ;  <  >  ?  [  \  `  {  |

and neither escapes any of `% + , - .`. The two disagree on the rest:

| character | bash | zsh |
| --- | --- | --- |
| `= @ :` | escapes | leaves bare |
| `# ] ^ }` | leaves bare | escapes |

Both disagreements are cosmetic — a backslash before a character the
grammar does not treat specially means the character itself — so the
core escapes what its own grammar gives a meaning to and does not carry
a dialect axis for the difference.

`~` and `#` are escaped **only at the start of a word**, which is the
only place they are special. Measured:

    : ~lea<TAB>   bash: : \~lead.txt    zsh: (nothing — zsh reads a
                                        leading ~ as a user name only)
    : #lea<TAB>   bash: : \#lead.txt    zsh: : \#lead.txt
    : -lea<TAB>   bash: : -lead.txt     zsh: : -lead.txt

**Inside double quotes**, only the characters that are still special
there. zsh escapes ``" \ $ ` !``; bash escapes `" \` and nothing else,
which is a bug rather than a dialect: a file named `a$b.txt` completes
in real bash to `"a$b.txt"`, and running that line expands `$b` and
opens a file that is not the one that was completed.

    : "a<TAB>     bash: : "a$b.txt"     zsh: : "a\$b.txt"

The core follows zsh, less the `!`: history expansion is not
implemented here, so `!` is not special in any context and a backslash
before it would say something untrue about the shell. Reproducing bash's
answer for `` $ `` and `` ` `` would mean shipping a Tab key that writes
a line meaning something other than the file it just named, and the
difference is invisible until the name has a `$` in it.

**Inside single quotes**, only the quote itself, closed and reopened
around a backslashed one — `'` becomes `'\''`. Both shells agree, and
nothing else inside single quotes is special so nothing else is touched.

The opening quote is part of the word and stays where it was typed. A
completion that finishes the word closes it; one that does not, does
not:

    : "file o<TAB>   bash: : "file one.txt"    zsh: : "file one.txt"
    : 'file o<TAB>   bash: : 'file one.txt'    zsh: : 'file one.txt'
    : "only<TAB>     bash: : "onlydir/         zsh: : "onlydir/
    : "file<TAB>     bash: : "file             zsh: : "file

The third leaves the quote open because a directory is not a finished
word, and the fourth because the matches only agree that far.

## Tilde

A `~` at the start of a word names a directory to read, and **stays a
tilde in the line**. Neither shell expands it into the text it inserts:

    : ~/Deve<TAB>          both: : ~/Developer/
    : ~/Developer/git<TAB> both: : ~/Developer/github.com/
    : ~roo<TAB>            both: : ~root/

`~name` with no slash yet is a *user name*, not a filename — which is
why zsh offers nothing for `~lea` even with a file called `~lead.txt` in
the directory. A completed user name gets the `/` that a directory gets,
and no space.

Inside quotes a tilde is not special and is not treated as one:
`: "~lea<TAB>` completes the file, not a user.

## Hidden files

**Semantics axis: `EditorStyle.CompletionMatchesHiddenFiles`** — bash
yes, zsh no; the core's answer is no.

A name beginning with a dot is offered when the word being completed
begins with one. Whether it is offered when the word does *not* is where
the two shells part. Measured with `.hidden` in the fixture and the
whole directory listed on a double Tab:

    bash   : <TAB><TAB>   .hidden  a$b.txt  bracket[1].txt  dir/ …
    zsh    : <TAB><TAB>   a$b.txt  bracket[1].txt  dir/ …

bash's readline documents this as `match-hidden-files`, and it is on by
default. zsh's rule is the one that keeps Tab usable in a home
directory, and it is the core's.

The two also differ on `.` and `..`:

    : .<TAB>   bash: ambiguous between ./ ../ and .hidden
               zsh:  : .hidden

The core follows zsh here as well and offers only real entries, so
`.<TAB>` on this fixture completes `.hidden` outright. `../` remains
typable; it is one character longer than a Tab.

## The bell, and when the matches are drawn

A completion keystroke that puts nothing on the line says so. Measured
2026-09-19 through a pseudo-terminal, in a tree whose `big/` holds
`aa00/` through `aa11/`, with the word completed through each shell's
own shipped completion system:

| typed, then one Tab | bash 5.3.20 | zsh 5.9.2 |
| --- | --- | --- |
| `big/aa0` — ten matches, agreeing on nothing more | `\a` | `\a` and the listing |
| `big/zzznope` — nothing matches | `\a` | `\a`, and nothing else at all |
| `big/a` — twelve matches agreeing on `aa` | `\a` and the `a` | the `a` |

Three facts, and the core takes the first two:

**A keystroke that reaches the line is silent, and one that does not
rings.** The two silent cases a person cannot tell apart from the
line — a word that matched nothing, and a word whose matches agree on
nothing past what is typed — are the same keystroke, and both ring.

**The bell is the only thing written.** Row two is `\a` and no redraw
in both shells, which is what makes it an assertion rather than a
count: nothing moves, so nothing is repainted.

**Whether the matches are drawn on that same keystroke is a dialect's
answer.** zsh draws them with the bell; bash rings and waits for a
second Tab, which then lists and is itself silent. Specified with the
rest of the editor's style, as
`EditorStyle.ListMatchesWithoutASecondKeyOption` — named for the option
because it is one a person turns off, and measured: `unsetopt autolist`
in zsh leaves the same keystroke writing the bell alone.

Row three is the one divergence the core does not take. bash rings for
an ambiguous completion even when it fills something in; zsh rings only
when nothing reached the line, and that is what this editor does.

zsh's own two switches are separable, measured on the ambiguous row:
`unsetopt listbeep` leaves the listing and drops the bell, `unsetopt
autolist` leaves the bell and drops the listing, and only both off is
silence. The bell's switch is not modeled here — the bell is what both
shells do by default and what this editor always does.

## Listing

When the matches do not agree beyond what is typed, they are printed —
on that keystroke or on a second one, per the axis above. Both shells
print **only the part of the name below the directory being
completed**, never the whole word:

    : sub/<TAB><TAB>
    nested.txt   nested2.txt  other.txt

Both mark a directory with a trailing `/` in the listing, both fill
columns downward, and both put two spaces between them.

They differ in one respect: bash prints raw names and zsh prints the
names as they would be inserted, escapes and all — `file\ one.txt` in
zsh against `file one.txt` in bash. The core prints the escaped form,
because a listing whose entries can be typed is the more useful of the
two and because it is the only one that makes a trailing space in a name
visible.

Above a hundred matches, both ask before printing. That threshold and
the wording of the question are specified with the rest of the editor's
style; see `EditorStyle.ListQuery`.

## Completion on a key of its own

Tab is one of several keys a completion can be on, and the rest do
different things with the same matches. Measured 2026-09-18 through a
pseudo-terminal against zsh 5.9.2 under `-i` with a scratch rc, in a
directory holding `uniq_alpha`, `uniq_beta`, `uniq_gamma` and `zzsolo`,
with each action on a key of its own so that Tab's own two-keystroke
rule could not be mistaken for the action:

| typed | pressed | what happened |
| --- | --- | --- |
| `cat uniq` | list only | the three names listed, the line untouched |
| `cat uniq` | list only, again | listed again — there is no second-keystroke rule |
| `cat zzs` | list only | `zzsolo` listed, and still not inserted |
| `cat uniq` | menu | `cat uniq_alpha` |
| | again, three times | `uniq_beta`, `uniq_gamma`, then round to `uniq_alpha` |
| `cat uniq` | menu backwards | `cat uniq_gamma`, then `uniq_beta` |
| `cat zzs` | menu | `cat zzsolo ` — one match is an ordinary completion |

**The listing a menu draws on its first press is not the menu's.** With
`autolist` turned off the same keystroke inserts `uniq_alpha` and draws
nothing, while the listing key goes on listing. A menu implemented as
"complete, and also list" passes the first observation and is not what
the action is.

**A menu lasts exactly as long as the keystrokes are adjacent.** Anything
else pressed between two of them ends it, and the next press starts a
fresh completion over whatever the line now says.

**Deleting or listing, decided by the cursor.** Where the cursor has a
character under it the character goes; where it has none the matches are
listed, on an empty line included. The empty line is the case a guess
gets wrong, because this is what `^D` is bound to in zsh and `^D` on an
empty line ends the session — but on any other key the same action
offered to list all 1064 commands. Ending input belongs to the key.

**Completing the prefix.** zsh separates completing the whole word under
the cursor from completing only the text before it. This editor only
ever does the second: it completes `line[start:point]` and replaces
exactly that, leaving whatever follows the cursor alone. Measured with
the cursor put after `uniq` in `cat uniqXYZ`, the prefix spelling
inserted the `_` the three matches agree on and left `XYZ` where it was,
while the whole-word spelling found nothing and rang the bell.

**The bell.** zsh rings one for a completion that matches nothing and
for the ambiguous insertion a menu makes. This editor rings it for the
first of those — see *The bell, and when the matches are drawn* above,
which is an editor-wide rule rather than one about these keys — and not
yet for the second, which belongs to the menu.

## What is deliberately not here

**A menu drawn as a selection.** zsh's `zsh/complist` draws the match
the menu stands on in inverse video inside the listing and lets the
arrow keys move over it. The cycling itself is specified above; the
*selection* — a listing with a cursor in it — is a distinct interaction
with a screen of its own and is not specified here.

**Programmable completion.** `complete -F`, `compdef` and the
per-command rules built on them are a language rather than a behavior,
and none of the above depends on them: every measurement above was taken
from a shell started with no startup files, so no completion system was
loaded in either shell.

What a real startup file does is worth stating, because it is what a
person actually runs. zsh's `compinit` ends by putting a completion
widget on the Tab key — `zle -C complete-word .complete-word
_main_complete` — and that widget's function is the whole of the
per-command language above.

**A key bound to such a widget asks the widget's function first and
falls back to the completion specified here.** The function contributes
candidates with `compadd`; if it has none — because it is not defined,
because it failed, or because it had nothing to say about this word —
the answer above stands unchanged, and Tab goes on completing filenames
and command names exactly as specified. That ordering is the rule and
not a recovery: a completion function that breaks costs one call, never
the key. A shell whose Tab broke the moment a startup file was read
would be worse off than one with no completion system at all, and that
is what the fallback exists to make impossible.

So a completion somebody writes by hand runs here. Measured 2026-09-15
through a pseudo-terminal, with `_c() { compadd checkout cherry
cherry-pick }` on a `zle -C` widget bound to Tab: `git chec<TAB>`
completes to `git checkout `, and a second Tab on `git che<TAB>` lists
all three — the same three, in the same order, that zsh lists for the
same widget.

**The shipped completion system runs too.** `_main_complete` reaches
`_complete` → `_normal` → `_dispatch` → `_arguments`, and `_arguments`
is `comparguments` — one of the eight builtins of `zsh/computil`, all
eight of which are in the table and answering. Measured 2026-09-16
through a pseudo-terminal against `compinit` on this machine's own zsh
functions, with Tab left where `compinit` put it, `cmd/zsh` beside
`/bin/zsh` on the same line and the same rc:

    uname -<TAB>      here: -a  -m  -n  -p  -r  -s  -v
                      zsh:  the same seven, each with its description
    uname -a<TAB>     here: uname -ap        zsh: uname -ap
    git che<TAB>      here: check-attr check-ignore check-mailmap
                            check-ref-format checkout checkout-index
                            cherry cherry-pick
                      zsh:  the same eight, sorted, each described
    git checkout -    here: -q --quiet -f --force -b -B -d --detach …
                      zsh:  the same set, with descriptions
    git log --for     here: --format=        zsh: --format=
    gzip -c<TAB>      here: the twenty-three stacked words
                      zsh:  the same twenty-three, in two blocks

So the names and their order are zsh's, and the rest of an option stack
is offered. What zsh draws *beside* a name is not: see below.

**One of the eight is smaller than zsh's.** `compfiles` builds no glob
pattern and prunes nothing: that one is an optimisation `_path_files`
can do without, and the conservative answers it gives are correct rather
than approximate.

Two others were, until the editor's seam grew somewhere to put a row.
`compgroups` created groups nothing could order and `compdescribe`
collapsed its answer to one group per definition, because a completion
was a replacement word and nothing else. A completion now carries the
row a listing draws for it and the block it is drawn in (#3041, #3232),
so `compgroups` is the order the blocks come out in and `compdescribe`
splits a definition whose rows are not all described into the described
half and the bare one — which is what `gzip -c` is two blocks of.

**An option's own argument is described.** `comparguments -D` answers for
the argument the cursor is standing in, and where the cursor is standing in
an *option's* argument rather than one of the command's it says so: the tag
is `option-S-1` rather than `argument-1`, and the message and action are the
option spec's own. That is what `gzip -S<TAB>` and
`git checkout --orphan=<TAB>` want, and it was answered with nothing until
#3229.

Where an option's argument may be written is the whole of the rule, and the
forms differ. Measured 2026-09-18 through a pseudo-terminal from inside a
`zle -C` widget, one spec set, the cursor at the end of each line:

| form | `cmd -x` | `cmd -x ` |
| --- | --- | --- |
| `-f+[…]:file:` attached or a word | the option's | the option's |
| `-d-[…]:dir:` attached only | the option's | the command's first |
| `-o=[…]:out:` after `=` or a word | the command's first | the option's |
| `-e=-[…]:eq:` after `=` only | nothing | the command's first |
| `-n[…]` no argument | nothing | the command's first |
| `-T[…]:one::two:` two words | nothing | the option's, first |

The third row is the one a guess gets wrong twice over: `-o` alone is not
read as an option at all — `$line` holds it and `$opt_args` is empty — and
`-o=` is. Inside a stack of single-letter options every form but the last
attaches, because a stack is written without separators by definition.

**And what a shipped completion reaches is not decided here alone.**
Several completions stop short somewhere outside `zsh/computil` — `_nl`
and `_od` on the `(R)` expansion flag, and `_file_modes` on the
`zsh/complete` conditions. `git checkout <branch>` was a third and is
not: a `:q` modifier written after a bare parameter expansion left a
literal `:q` for `compadd` to stop reading options at, and the unbraced
spelling of a modifier list is read now (#3127). See
`docs/spec/semantics.md`.

**Descriptions are carried.** `compadd -d` replaces the drawn text of a
match, `-X` heads the block it is in and `-x` says something about a
block whether or not it has anything in it. Measured 2026-09-18 through
a pseudo-terminal, this shell and zsh 5.9.2 against the same widget
function, the same two-row prompt and the same line — one block of
described options a row each, one block of bare ones packed under it:

    options
    -d  -- decompress
    -f  -- force overwrite
    -1  -2

drawn identically by both. What a *shipped* completion draws as
`name  -- sentence` is a display string `compdescribe` built and padded
before `compadd` ever saw it, which is why the row and not a description
beside a name is the thing the seam carries.
