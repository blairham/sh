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
| `big/u` — one match | the name and a space | the name and a space |

Four rows and three facts. The last row is the control — a keystroke
that settles the word is silent in both, which is what stops the first
fact from reading as "one shell rings more often". The core takes the
first two facts; the third and the row below them are a dialect's:

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
in zsh leaves the same keystroke writing the bell alone, and the second
keystroke lists nothing either (see the menu below).

Row three is where the two shells ask **different questions**, and it is
a dialect's answer rather than the core's: bash asks whether the *word
is settled* and rings while it is not, so a keystroke that put half a
word in still rings; zsh asks whether the *keystroke did anything* and
is silent when it did. A unique match is silent in both, so it is the
middle state alone — three outcomes rather than two, which is why
`completionOutcome` has three. Carried as
`EditorStyle.BellRingsOnAnAmbiguousCompletionThatInserts`, false for the
core and true for the one shell that answers that way; bash 5.3.20 and
bash 3.2.57 agree, so it is that shell's answer and not a version's.

zsh's own two switches are separable, measured on the ambiguous row:
`unsetopt listbeep` leaves the listing and drops the bell, `unsetopt
autolist` leaves the bell and drops the listing, and only both off is
silence. Only the second is modeled. The bell has no switch here,
because ringing when the keystroke reached the line with nothing is
what both shells do by default, and a person who wants silence is
asking a question nobody has asked yet.

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

**A block is sorted by byte, and that is a deliberate divergence from zsh
outside the C locale** (#6168). zsh sorts a listing by the locale's
collation, exactly as it sorts a pathname expansion — measured 2026-10-06
against zsh 5.9.2, a listing of `README.md readme.md Beta alpha _x b-c bc
B a1 a10 a2` draws `_x a1 a10 a2 alpha B b-c bc Beta readme.md README.md`
under `LANG=en_US.UTF-8` and `B Beta README.md _x a1 a10 a2 alpha b-c bc
readme.md` under `LC_ALL=C`, and a glob over the same names gives the same
two orders (over ten of them, on a case-insensitive file system that holds
one of the two READMEs). This shell orders a
pathname expansion by byte in every locale, for the reasons recorded in
`interp/order.go` — the platforms' collations disagree and no single
table is right on both — and the listing keeps the same order, so the two
surfaces stay one decision. `cd <TAB>` therefore lists capitalized names
first under a UTF-8 locale, where zsh mixes them in. A collation, if one is
built, is built for both surfaces at once.

Above a hundred matches, both ask before printing.

**Or a long listing is paged.** In zsh with `zsh/complist` loaded and
`LISTPROMPT` set (the completion system's `list-prompt` style leaves it set),
a listing too tall for the terminal is never asked about (#6153). It is drawn
a screenful at a time, and each screenful ends with the prompt on the
terminal's last row. Measured 2026-10-06 against zsh 5.9.2 at 120 by 40,
with a thousand names that list in fifty rows:

- The first screenful is 39 rows, and `%SAt %l %m %p%s` draws `At 39/50
  989/1000 Top` in standout.
- Return, a line feed, `^N` and the down arrow draw one row more (`40/50
  990/1000 80%`).
- Tab draws the next screenful, and at the end the line is drawn under the
  listing.
- `^G` stops and drops its key, and any other key stops and is read as usual.

`%l`, `%m` and `%p` are the last row drawn over the rows, the last match on
that row over the matches, and `Top` or the last row as a percentage. Their
capitals pad to 9, 9 and 6 columns. An empty `LISTPROMPT` draws `At %p: Hit
TAB for more, or the character to insert` in standout.

A listing that fits under the line's own rows is not paged. Neither is one in
a session without the module, where the question is asked as before. The
`listscroll` keymap's own bindings are not read; its defaults are.

**A listing the cursor goes back up over ends on its own last row.** zsh
writes no newline after it, so a listing that exactly fills the terminal
under the line scrolls nothing. At 40 rows, that is 39 rows under a one-row
line, or 38 under a two-row prompt. That threshold and
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
the action is. The listing is `autolist`'s, and it is drawn under that
option as the menu starts.

**A Tab pressed again can start the menu.** zsh's `AUTO_MENU`, on by
default, makes a completion key pressed again on a word the last one left
ambiguous put the first match in the line, and each press after it the
next, where without it the matches are listed again (#6197). Measured
2026-10-06 through a pseudo-terminal against zsh 5.9.2, `x a` typed over
`always` and `auto`:

| options | Tab 1 | Tab 2 | Tab 3 |
| --- | --- | --- | --- |
| (defaults) | `\a` and the listing | `\a` `x always` | `x auto` |
| `unsetopt automenu` | `\a` and the listing | `\a` and the listing again | `\a` and the listing again |
| `setopt menucomplete` | `\a` `x always` and the listing | `x auto` | `x always` |
| `unsetopt autolist` | `\a` | `\a` `x always` | `x auto` |
| `unsetopt autolist automenu` | `\a` | `\a` | `\a` |
| `setopt bashautolist` | `\a` | the listing | `\a` `x always` |

The menu **waits for the listing** where there is one to wait for: over
`alpha1`, `alpha2` and `zz`, the first Tab fills in `alpha`, the second
lists, and only the third starts the menu — unless `autolist` is off,
when the second does. And with `autolist` off **no key lists**, where
bash lists on the second: `bashautolist` is the option that gives zsh
bash's answer, and it takes the first key's listing away when it does.
The keystroke that starts a menu rings, by every route — the Tab, a
`menu-complete` and a `reverse-menu-complete` widget — and the ones that
walk it do not. The editor's style carries all of it:
`EditorStyle.MenuOnARepeatedCompletionOption`,
`MenuOnTheFirstCompletionOption`, `ListMatchesOnASecondKeyOption` and
`BellRingsWhenAMenuStarts`.

**Whether that fill counts as a key of the run is `listambiguous`'s
answer** (#6220). With it on and `autolist` or `bashautolist` on, the fill
stands aside: it is silent, it lists nothing, and the key after it is the
first of the run. So under `bashautolist` the Tab after the fill rings,
the next lists, and the fourth starts the menu. With it off, or with
neither listing option on, the fill *is* the first key and does what a
first key does. Measured 2026-10-06 against zsh 5.9.2, over `alpha1`,
`alpha2` and `zz`:

| options | Tab 1 | Tab 2 | Tab 3 | Tab 4 |
| --- | --- | --- | --- | --- |
| (defaults) | `alpha` | `\a` the listing | `\a` `alpha1` | `alpha2` |
| `unsetopt listambiguous` | `\a` `alpha` and the listing | `\a` `alpha1` | `alpha2` | |
| `unsetopt autolist` | `\a` `alpha` | `\a` `alpha1` | `alpha2` | |
| `setopt bashautolist` | `alpha` | `\a` | the listing | `\a` `alpha1` |
| `setopt bashautolist`, `unsetopt listambiguous` | `\a` `alpha` | the listing | `\a` `alpha1` | |

This is carried as `EditorStyle.FillStandsAsideOption`. A dialect that
names no such option keeps the fill as the first key, never listed on its
own key and rung as `BellRingsOnAnAmbiguousCompletionThatInserts` says.

A completion function decides for itself through `compstate[insert]`. It
finds `automenu-unambiguous` on a first Tab, `automenu` on the Tab after a
listing, `unambiguous` with the option off, and `menu` under
`menucomplete` or on a `menu-complete` widget; and what it leaves there
is obeyed — `menu` or `automenu`, with a `:N` for the match to start at,
starts the menu, and `unambiguous` keeps it from starting. The function is
not called again for the Tabs that walk a menu once it has started. That
is how the completion system's `menu` style reaches the line, and
`menu select` comes to the walk here: the interactive selection it asks
for is zsh/complist's, which this shell does not have (#5761), and the
line after each Tab reads as it does in zsh.

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
falls back to the completion specified here** — and so does a widget that
calls it by name (`w() { zle mycomp }`), which is how zsh-autosuggestions
reaches every completion widget it wraps. Measured 2026-10-05 against zsh
5.9.2: the wrapper's key fills in `x al` to `x alp` and lists on the next
press, with `$WIDGET` inside the function still the wrapper's (#6142). The function contributes
candidates with `compadd`, and **if it has none, the key completes
nothing**: it rings and the line stays as it was. Measured 2026-10-06
against zsh 5.9.2 over `always` and `auto` with Tab on such a widget,
`x a` stays `x a` with one bell and nothing listed for a function that
adds nothing, one whose words do not match, one that returns 1, and a
widget whose function is not defined; and with `compinit`, `cd ` stays
`cd ` in a directory with no subdirectory, where this used to hand the
key to the completion specified here and put `cd a` on the line (#6214).
A function that **stops on an error** — a refusal it printed — is the
one case still worth its own word: nothing is offered and nothing else is
asked, so a completion function that breaks costs one call, never the
key. That was the fallback's reason for existing (#2770), from before
the shipped completion system ran here.

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

**`compfiles` was smaller than zsh's, and that was not an optimisation.**
It built no glob pattern on the reading that `_path_files` could do
without one. But the array `_path_files` hands in holds the directory
already handled — `''` for the working directory — and the pattern is what
comes back, so an array left alone globbed nothing and every file
completion through the shipped `_files` was empty (#6144). It now builds
the pattern zsh 5.9.2 builds, measured from a completion widget calling it
with chosen arguments: each element glob-quoted, the skipped part, a
pattern for `$PREFIX` under the match specification (`RE`, `[Rr][Ee]` for
a case-folding one, nothing for a specification that reaches the start of
the word), then the file pattern or `*(-/)`; and `-r` narrows by a finished
first component. `compfiles.go` has the table; `-i` still ignores nothing.

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
