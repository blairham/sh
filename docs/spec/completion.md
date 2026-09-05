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

## Listing

When the matches do not agree beyond what is typed, a second Tab prints
them. Both shells print **only the part of the name below the directory
being completed**, never the whole word:

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

## What is deliberately not here

**Menu completion.** zsh's `AUTO_MENU` is on by default, so a third Tab
in zsh starts cycling through the matches in the line rather than
listing them again. bash does not do this without being asked. It is a
distinct interaction with state of its own, and it is not specified
here.

**Programmable completion.** `complete -F`, `compdef` and the
per-command rules built on them are a language rather than a behavior,
and none of the above depends on them: every measurement above was taken
from a shell started with no startup files, so no completion system was
loaded in either shell.
