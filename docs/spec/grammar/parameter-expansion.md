# Parameter expansion forms

What the operators inside `${ … }` mean. `substitutions.md` says where the
braces end; this says what is between them.

Citation: POSIX.1-2024 XCU §2.6.2. Panel measurements as in `../oracle.md`.

## One rule explains the four conditionals

    ${x:-word}   ${x:=word}   ${x:?word}   ${x:+word}
    ${x-word}    ${x=word}    ${x?word}    ${x+word}

**The colon extends the test from "unset" to "unset or empty."** That is
the whole difference between the two rows, and it is unanimous:

| probe | unset | empty | set to `S` |
| --- | --- | --- | --- |
| `${x:-D}` | `D` | `D` | `S` |
| `${x-D}` | `D` | *(empty)* | `S` |
| `${x:+A}` | *(empty)* | *(empty)* | `A` |
| `${x+A}` | *(empty)* | `A` | `A` |

The four operators then differ only in what they do when the test fires:

| operator | when the test fires |
| --- | --- |
| `-` | substitute the word |
| `=` | assign the word to the variable, then substitute it |
| `?` | write the word as an error and exit |
| `+` | substitute the word when the test does **not** fire |

`${u:=V}` leaves `u` set to `V` afterwards, so it is an expansion with a
side effect — the only one in this document.

## The word is itself a word

It is expanded, not taken literally:

    unset u; d=DEF
    ${u:-$d}            →  DEF
    ${u:-$(echo sub)}   →  sub
    ${u:-a b}           →  a b

So the AST cannot store it as a string. It is a nested word, which is why
`substitutions.md` keeps the inner text raw rather than flattening it:
whatever parses `${ }` has to parse a word inside it.

### And it is a word in the quoting the expansion stands in

A `${ }` written inside double quotes has a body that is double-quoted
**content**, not a fresh unquoted word. What that changes is the single
quote: there it is an ordinary character, so it quotes nothing, it is not
removed, and what stands between two of them is still substituted.

Measured 2026-09-07, unanimous across bash 5.3.15, bash 3.2.57, that build
invoked as `sh`, dash, ksh93u+ and zsh 5.9.2, with `v=VAL`:

| probe | result |
| --- | --- |
| `"${u:-'$v'}"` | `'VAL'` |
| `"${u:-'$(echo hi)'}"` | `'hi'` |
| `"${u:-''}"` | `''` |
| `"${u:-'\$v'}"` | `'$v'` |
| `${u:-'$v'}` unquoted | `$v` |

The second row is the one that settles *how far* the shell looks: the
substitution inside those quotes is recognized and **performed**, rather
than scanned past to find where the body ends. The last row is what says
the rule belongs to the enclosing context and not to the body — the same
characters unquoted are an ordinary single-quoted run, quotes removed and
nothing substituted.

The same fact reaches the **parse**, and there the panel refuses as one:

    printf '[%s]' "${x:-'a$(b'}"

With the `'` an ordinary character there is nothing to close the `$(`, and
every shell in the panel fails the line — on six different wordings and
four different statuses, which is a diagnostics question rather than a
grammar one.

Two things are *not* this rule, and each is a row that a reading which
went further would get wrong.

A **double** quote in the same position does not follow it: unescaped, it
opens a run of its own and is removed, so `"${u:-"$v"}"` is `VAL` where
`"${u:-'$v'}"` is `'VAL'`. The two quote characters part company here.

A **pattern** operand does not follow it either. Its quotes quote, and are
removed: with `s=xay`, `"${s#'x'}"` trims the `x` and is `ay`, and
`"${s#'a$(b'}"` has no substitution in it to leave open. So it is the
*word* operands — `-`, `:-`, `=`, `:=`, `+`, `:+`, `?`, `:?` — that take
the enclosing quoting, and `#`, `##`, `%`, `%%`, `/`, the case operators
and a subscript that do not.

A here-document body is the same context reached by the other road: it
expands as a double-quoted string does, so `${u:-'$v'}` in one is `'VAL'`
in all six.

One consequence worth writing down, because it looked like a rule of its
own: a **process substitution** in a `${ }` operand is performed in bash
only where the expansion is unquoted. `p=${u:-<(:)}` is a path there and
`p="${u:-<(:)}"` is the five characters `<(:)`, in 5.3 and 3.2 alike —
which is not a second answer about operands but this one, since a
process substitution is no more written inside double quotes here than
anywhere else.

## Pattern removal

    ${x#pat}   shortest matching prefix removed
    ${x##pat}  longest matching prefix removed
    ${x%pat}   shortest matching suffix removed
    ${x%%pat}  longest matching suffix removed

Measured with `p=a.b.c`, unanimous:

| probe | result |
| --- | --- |
| `${p#*.}` | `b.c` |
| `${p##*.}` | `c` |
| `${p%.*}` | `a.b` |
| `${p%%.*}` | `a` |
| `${p#x}` | `a.b.c` — a pattern that does not match removes nothing |

The patterns are **glob patterns, not regular expressions**: `${p#[ab]}`
and `${p#?}` both yield `.b.c`, because a bracket expression and a `?`
each match exactly one character — the leading `a` — and neither repeats.
Doubling the operator is what selects the longer match; there is no
greediness syntax inside the pattern.

## Length, and where it diverges

`${#x}` is the length of the value — `x=abcd` gives 4, unanimously.

**In what unit** is not unanimous, and is not a property of the shell
alone:

| probe | `LC_ALL=C` | `LC_ALL=C.UTF-8` |
| --- | --- | --- |
| `s=héllo; ${#s}` | 6 everywhere | 5, and 6 in dash |
| `s=日本語; ${#s}` | 9 everywhere | 3, and 9 in dash |

So the length is what the locale's encoding calls a character, and dash
is the one member of the panel with no multibyte decoder to consult it
with. Semantics axis `MultibyteEncodingIsHonored`; the locale itself is
runtime state read off the runner rather than a second axis, and
`docs/spec/semantics.md` has the precedence, the codesets and the
undecodable byte.

`${s:off:len}` and a subscript on a scalar count the same unit, which is
the part a partial fix gets wrong: an offset in bytes against a length in
characters lands in the middle of a character. Pattern matching does
*not* follow yet (#905).

`${#@}` and `${#*}` are not unanimous:

| probe, after `set -- p q r` | dash | bash | ksh93 | zsh |
| --- | --- | --- | --- | --- |
| `${#@}` | **5** | 3 | 3 | 3 |
| `${#*}` | **5** | 3 | 3 | 3 |

dash gives the length of the joined string `p q r`; the others give the
number of positional parameters. This is the first axis measured where
dash stands alone, and it is a silent one: both answers are plausible
numbers and neither errors.

Semantics axis: `LengthOfSpecialIsCount` — dash no, bash, ksh93 and zsh
yes. This is one of the two axes `CoreSemantics()` answers, and it says
yes: the core panel is bash, ksh93 and zsh, and dash is the shell the
core was drawn to exclude.

## Extensions

None of these are POSIX, and they do not all arrive together.

| form | meaning | dash | bash | ksh93 | zsh |
| --- | --- | --- | --- | --- | --- |
| `${x/pat/rep}` | replace first match | **no** | yes | yes | yes |
| `${x//pat/rep}` | replace every match | **no** | yes | yes | yes |
| `${x/#pat/rep}` | replace an anchored prefix | **no** | yes | yes | yes |
| `${x/%pat/rep}` | replace an anchored suffix | **no** | yes | yes | yes |
| `${x:off:len}` | substring | **no** | yes | yes | yes |
| `${x^^}` `${x,,}` | upper- and lower-case | **no** | **yes** | **no** | **no** |
| `${!x}` | indirection | **no** | **yes** | *other* | **no** |
| `${a[i]}` | array element | **no** | yes | yes | yes |

Two rows deserve their own note.

**`${x^^}` is bash alone.** ksh93 reports a syntax error and zsh a bad
substitution. `shell-matrix.md` already recorded it as bash-4-only; this
places it in the bash dialect rather than the core.

Case change has a third direction: **`${x~}` toggles the first
character's case and `${x~~}` toggles every character's** — `aBc` to
`ABc` and `AbC` — and each operator takes a pattern saying which
characters to touch, exactly as `^` and `,` do: `${x~~[ab]}` on `aBc`
is `ABc`, the pattern matched case-sensitively against each character.
bash 5.3 alone; bash 3.2, dash and zsh call it a bad substitution and
ksh93 refuses it while reading (measured:
`param/case-toggle-is-newer-bash-still`). It rides the same dialect
flag as `^^` and `,,` — one shell, one family of operators, one flag.

**`${!x}` diverges four ways.** With `x=y` and `y=V`, bash yields `V`,
dash and zsh reject it, and **ksh93 yields `x`** — it is not an error
there, it means something else. That is the `&>` failure mode inside an
expansion, and the third measured instance of it.

`${a[i]}` inherits the array-base axis from `semantics.md`: with
`a=(p q r)`, `${a[1]}` is `q` in bash and ksh93 and `p` in zsh.

## The `!` that lists names instead of following one

Two more spellings open with `!` and are not indirection:

**`${!prefix*}` and `${!prefix@}` are the *names* beginning with the
prefix**, sorted, and the two suffixes differ exactly as `$*` and `$@`
do — joined into one field on IFS's first character, or one field per
name (measured: `param/the-names-with-a-prefix` and its
`-one-field-each`, `-joined` and `-are-sorted` neighbors; a prefix
matching nothing is empty with status 0, `param/no-names-with-that-prefix`).
bash and ksh93 answer alike; dash and zsh call the whole form a bad
substitution. Two shells is not a common denominator, so the form
belongs to the bash and ksh dialects and rides `ParamIndirection` —
the flag that already admits `!` after `{`.

**`${!a[@]}` and `${!a[*]}` are an array's *subscripts*, not its
elements** — with a sparse array, the subscripts actually assigned
rather than `0..n-1`. It exists to iterate an array by index, and the
corpus case is written as that loop (`param/array-indices`): bash and
ksh93 answer, zsh rejects it as a bad substitution — its arrays are
dense, so the question does not arise there — and dash has no arrays at
all. The same dialect flag governs it.

A `!` directly before an *operator* is none of these: `${!:+set}` is the
special parameter `$!` with an operator, unanimously
(`param/bang-with-an-operator-is-the-parameter`).

## Negative subscripts

A negative subscript counts back from the end, and does **not** inherit
the base axis: with `a=(one two three)`, `${a[-1]}` is `three` in bash,
ksh93 and zsh alike — zsh's positive subscripts count from 1 and its
negative ones still match the other two. Measured (oracle run,
`param/a-negative-subscript-counts-from-the-end`); bash 3.2 predates the
form and refuses it.

The same reading holds everywhere a subscript is written:

- **assignment** — `a[-1]=X` replaces the last element, unanimous
  (`param/assigning-through-a-negative-subscript`);
- **arithmetic** — `$((a[-1]))` reads and `$((a[-2]=9))` writes the same
  positions, unanimous;
- **unset** — `unset "a[-1]"` removes the last element, unanimous.

Against a **sparse** array the end is one past the highest *subscript*,
not the element count: with subscripts 0 and 5, `${a[-1]}` is the element
at 5 and `${a[-2]}` is the unassigned 4 — nothing — in bash and ksh93
(measured; zsh's arrays are dense, so the two ends never differ there).

An **associative** subscript is a key, never a position: `m[-1]` on a
declared name stores and reads the two characters `-1`, unanimous in the
three that declare them.

Out of range past the start the shells disagree three ways — bash warns
`bad array subscript` and expands to nothing (fatally, for a write), zsh
is silent (and *prepends* on a write), ksh93 is fatal either way. This
implementation reads nothing, and refuses the write with the same
diagnostic any below-base subscript gets; the divergence is recorded
rather than modeled, because none of it is behavior a script can rely on
across shells.

## What a subscript *means* — three readings that diverge

The bracket parses the same way in every shell that has subscripts at
all. What is inside it does not mean the same thing, and the three
divergences below are all one evaluator. Measured 2026-09-05 on zsh
5.9.2, bash 5.3.15, bash 3.2.57, bash as `sh`, ksh93 and dash.

### A comma: a range, or the arithmetic operator

| probe | zsh | bash 5.3 | bash 3.2 | bash-as-sh | ksh93 | dash |
| --- | --- | --- | --- | --- | --- | --- |
| `a=(w x y z); "[${a[1,3]}]"` | `[w x y]` | `[z]` | `[z]` | `[z]` | `[z]` | no arrays |
| `"[${a[0,2]}]"` | `[w x]` | `[y]` | `[y]` | `[y]` | `[y]` | — |
| `"[${a[1,2,3]}]"` | `bad substitution` (1) | `[z]` | `[z]` | `[z]` | `[z]` | — |

A subscript is an arithmetic expression, and arithmetic has a comma
operator whose value is its right operand — so `${a[1,3]}` is `${a[3]}`
in four of the five shells that read it. In zsh the comma separates the
two ends of a **range**, and a third part is not a wider range but a bad
substitution.

Both readings answer, neither reports, and the values are different: a
script cannot tell which shell it is on except by what it gets. That is
a conflict rather than an addition, so it is the semantics axis
`SubscriptCommaIsARange` and not a grammar flag. The core answers *no*,
which is both the standard's reading — POSIX has the comma operator and
no ranges — and four of the five shells'.

The axis is asked only where the two readings differ. `${a[2,2]}` is the
second element either way and needs no answer, which is what keeps a
core that has chosen no dialect from refusing a subscript both shells
agree about.

The endpoints, measured with `a=(w x y z)` and `s=hello`:

| probe | array | string |
| --- | --- | --- |
| `[2,-1]` — a negative end | `x y z` | — |
| `[-3,-2]` — both negative | `x y` | — |
| `[0,2]` — below the first | `w x` | `he` |
| `[2,99]` — past the last | `x y z` | `llo` (from `[3,99]`) |
| `[3,2]` — backwards | empty | empty |
| `[1,0]` — an end before the first | empty | empty |
| `[-5,2]` / `[-6,2]` — a start past the start | **empty** | **`he`** |

Endpoints are subscripts, so they take the base and the count-back-from-
the-end reading a single subscript takes. The last row is the one no
symmetry predicts and it is a fact rather than a derivation: a start
counted back past the *first element* leaves an array empty, where the
same reach past the first *character* of a string is clamped to it.

### A subscript on a plain string

| probe, `s=hello` | zsh | bash 5.3 | bash 3.2 | bash-as-sh | ksh93 | dash |
| --- | --- | --- | --- | --- | --- | --- |
| `"[${s[0]}]"` | `[]` | `[hello]` | `[hello]` | `[hello]` | `[hello]` | `Bad substitution` |
| `"[${s[1]}]"` | `[h]` | `[]` | `[]` | `[]` | `[]` | — |
| `"[${s[2]}]"` | `[e]` | `[]` | `[]` | `[]` | `[]` | — |
| `"[${s[2,4]}]"` | `[ell]` | `[]` | `[]` | `[]` | `[]` | — |
| `"[${s[-1]}]"` | `[o]` | `[]` + `s: bad array subscript` | same | same | `[]` | — |

zsh reaches into the string's characters. Everything else reads a scalar
as an array of one, so the base names the whole string and no other
subscript names anything. Axis `ScalarSubscriptIsACharacter`; the core
answers *no*.

**Empty, not an error, is the core's answer**, and it is measured rather
than defaulted: neither bash nor ksh93 says anything about `${s[2]}` and
both expand it to nothing at status 0. The one place bash speaks is a
*negative* subscript, where it warns `s: bad array subscript` and still
expands to nothing at status 0 while ksh93 is silent. The value is
unanimous and is what this implementation gives; bash's advisory is
recorded rather than modeled, the same treatment its out-of-range
subscript warning gets above.

Characters and not bytes — where the locale says a character is more
than a byte. `s=héllo; "${s[2]}"` is `é` and `"${s[2,3]}"` is `él` under
`LC_ALL=C.UTF-8`, and under `LC_ALL=C` the same two are the single byte
`\xc3` and the pair that spells `é`, measured on zsh 5.9.2. It is the
same `MultibyteEncodingIsHonored` question `${#s}` asks, so the two
agree by construction; reading runes unconditionally answered the UTF-8
column in both.

### A subscript on a parameter that is not a name

| probe | zsh | bash 5.3 | bash 3.2 | bash-as-sh | ksh93 | dash |
| --- | --- | --- | --- | --- | --- | --- |
| `set -- a b c; "[${@[1]}]"` | `[a]` | bad subst. (1) | bad subst. (1) | bad subst. (127) | ``syntax error … `[' unexpected`` (3) | `Bad substitution` (2) |
| `"[${*[2]}]"` | `[b]` | bad subst. | bad subst. | bad subst. | syntax error | Bad substitution |
| `set -- abcd; "[${1[2]}]"` | `[b]` | bad subst. | bad subst. | bad subst. | syntax error | Bad substitution |
| `"[${0[1]}]"`, `"[${?[1]}]"`, `"[${-[1]}]"`, `"[${$[1]}]"` | the value's characters | bad subst. | bad subst. | bad subst. | syntax error | Bad substitution |

Five refusals and one reading, and nobody means something else by it —
so this is the additive kind of split, grammar flag
`SpecialParamSubscript`, beside `ArraySubscript`. It is separate from
`ArraySubscript` because it is about the *name*: `${a[1]}` is read by
four of the six and `${@[1]}` by one.

Where the flag is off the bracket is simply not consumed, and the
leftover text takes the route every unreadable expansion takes —
deferred to the run in most of the panel, refused while reading by the
one grammar that refuses everything else while reading. Nothing here
words a diagnostic of its own. Expanding it to nothing at status 0, as
this implementation did, is the silent middle answer nobody gives.

`@` and `*` are the positional parameters **as a list**; every other
special parameter supplies a *value*, so a subscript reaches into it by
character under the axis above.

`#` and `!` are unreachable in the braced spelling for the reason the
brace-less form records below: `${#[1]}` is a length and `${![1]}` an
indirection, so there is nowhere to write them down.

### How many fields a subscripted expansion makes

Quoting, measured on zsh with `a=(x y z)` and `set -- a b c`:

| probe | fields |
| --- | --- |
| `"${a[1,2]}"` | 1, joined with the first character of IFS — as `"$a"` is |
| `"${a[@]}"` | 3 |
| `"${@[1,3]}"` | 3 |
| `"${*[1,3]}"` | 1 |
| `"${@[*]}"` | 3 |
| `"${*[@]}"` | 3 |

A range yields a list, and whether quoting joins that list is decided
the way it is decided for the bare spellings: `@` keeps its fields and
everything else joins. Either spelling of the list is enough — `@` as
the name or `[@]` as the subscript.

`${#…}` follows the same line. It is a **count** where the subscript
named several elements — `${#a[1,2]}` on `(aa bb ccc)` is 2 where the
arithmetic reading measures the three-character element it landed on —
and a **width** where it named one value, so `${#s[2,4]}` on `hello` is
3.

One measured curiosity is recorded and not reproduced: `"${@[0]}"` is
*zero* fields in zsh where `"${@[9]}"`, `"${a[0]}"` and `"${*[0]}"` are
all one empty field. It is the only subscript that behaves that way and
no script can depend on it.

## Slicing the whole array

`${a[@]:off}` and `${a[@]:off:len}` slice the **list**: the result is
elements, one field each, not a substring of anything joined. With
`a=("a b" c d)`, `"${a[@]:0:2}"` is the two fields `a b` and `c` —
which is the whole reason this is not `${x:off:len}` over joined text
(measured: `param/array-slice`, `param/array-slice-keeps-its-fields`).

Two facts ride along, both measured in the same cases:

- **The offset counts from 0 in every shell with arrays — zsh
  included**, whose *subscripts* count from 1. Like a negative
  subscript, the slice does not inherit the base axis.
- **A negative offset counts back from the end**, and needs the space:
  `${a[@]: -2}` is the last two elements, while `${a[@]:-2}` is the
  `:-` default operator on the whole array. The space is what keeps the
  two spellings apart, and it is why the operator table earlier reads
  the colon forms first.

A negative **length** is where the agreement stops. On a scalar, bash
and zsh stop that many characters short of the end and ksh93 answers
with nothing at all (`param/a-negative-substring-length`) — the
`SubstringNegativeLengthIsEmpty` axis, which the core leaves unanswered
and the ksh dialect answers yes. On the *list* the same probe splits
three ways (measured 2026-09-04, same panel): zsh stops short of the
end, ksh93 is empty again, and bash — whose scalar answer is
count-from-the-end — **refuses it**, `substring expression < 0`. This
implementation counts a list's negative length from the end, the answer
its scalar default already gives; bash's refusal is recorded here
rather than modeled, the same treatment as its out-of-range subscript
warning above.

The scalar `${x:off:len}` itself is in the extensions table above; this
section exists because the `[@]` subject changes what the numbers index
— elements rather than characters.

## An operator over the whole array

An operator written on `${a[@]}` — a trim, a replacement, a case change —
applies to **each element**, and each stays a field of its own. Measured
with `a=(aa ab)`, unanimous in the three with arrays
(`param/an-operator-distributes-over-the-elements`):

| probe | fields |
| --- | --- |
| `${a[@]#a}` | `a` `b` |
| `${a[@]%%a*}` | two empty fields |
| `${a[@]//a/X}` | `XX` `Xb` |

The operator's word is expanded **once**, not once per element: a command
substitution in the pattern runs a single time (measured in all three).

The star form is where they split
(`param/an-operator-on-the-star-subscript-diverges`): bash and ksh93
apply the operator to each element and join what is left on the first
character of IFS (`${a[*]#a}` on `(aa ab)` is `a b`), zsh joins first and
applies the operator to the joined string once (`a ab`). The two readings
often agree — a suffix trim that stops inside the last element, a pattern
that matches nothing — so the axis is asked only where they differ.

Semantics axis: `OperatorDistributesOverStarSubscript` (bash and ksh93
yes, zsh no; unanswered in the core).

`${#a[@]}` is untouched by all of this: it is the count, and the length
question is answered before any operator runs.

## The `@` transformation family

`${x@Q}` transforms the value rather than testing or editing it. The
operator is `@` followed by **exactly one letter** from a fixed set —
`${x@}`, `${x@QQ}` and `${x@ Q}` are all bad substitutions. **bash alone**
(measured on bash 5.3): dash and zsh report a bad substitution at run
time, and so does ksh93 — `${x@Q}: bad substitution`, status 1 — even
though its *other* unrecognized operators are refused while reading. An
`@` operator in a branch never taken is silent in all four, so the `@`
family is deferred to run time everywhere, including the one shell that
otherwise refuses at parse time.

**bash checks the letter only once it has a value.** `${u@QQ}` on an
*unset* `u` is empty at status 0 — no diagnostic at all — while the same
spelling on a set name is `[${x@QQ}]: bad substitution`, naming the whole
**word** rather than the expansion. The three shells without the family
reject both spellings alike. Two consequences worth stating: a probe that
forgets to set the variable measures nothing, and a script cannot use an
invalid letter to detect whether the shell has the family, because the
answer depends on the variable rather than on the shell.

A *list* has no value when it is empty, and an empty *string* has one:
`a=(); ${a[@]@Z}` and `set --; ${@@Z}` are both quiet at status 0, while
`u=; ${u@QQ}` is refused. Measured over four spellings on 2026-09-05.

**A bad letter is a failed expansion, not an unreadable word**, and bash
draws the line itself: `bash -c 'x=a; echo "${x@QQ}"'` exits **127**,
where `bash -c 'x=a; echo "${(q)x}"'` — a bad substitution for a
different reason, found while reading the word rather than while
expanding it — exits **1** from the same invocation. The 127 is the same
number the same shell gives an unset parameter under `set -u` and a
`${x?}`, and it is a property of the *route*: both are 1 when the program
comes from a file or from standard input. `${#u@Z}`, where the length
question refuses the operator before the family is reached, exits 1 like
any other unreadable word.

Measured on bash 5.3, all with the variable set unless said otherwise; an
unset variable yields empty for every letter, with `set -u` complaining
first as it would for `$x`:

| letter | meaning | measured |
| --- | --- | --- |
| `Q` | value quoted for reuse as input | `a b'c` → `'a b'\''c'`; empty → `''` |
| `E` | backslash escapes expanded, `$'…'` rules | `a\tb` → `a<TAB>b`; `\101` → `A` |
| `P` | prompt expansion | `\u at \w` → the user and directory; plain text unchanged |
| `A` | an assignment statement that reproduces the variable | below |
| `a` | attribute letters | `declare -irx v=1` → `irx`; no attributes → empty |
| `K` | keys and quoted values | below |
| `k` | keys and values as separate words | below |
| `L` | lower-case the whole value | `abC dEf` → `abc def` |
| `U` | upper-case the whole value | `abC dEf` → `ABC DEF` |
| `u` | upper-case the first character | `abC dEf` → `AbC dEf` |

**`@Q` picks its quoting by content.** A value with no unprintable
character is single-quoted, the quote itself spelled `'\''`, backslashes
and `$` left alone: `has\backslash` → `'has\backslash'`. Any control
character (or byte that is not character-shaped) switches the whole value
to `$'…'`: `\a \b \t \n \v \f \r` by name, ESC as `\E`, `'` as `\'`,
`\` as `\\`, `"` kept plain, anything else unprintable as three-digit
octal **per byte** — DEL is `$'\177'`, U+0085 is `$'\302\205'`.

**Whether printable multibyte text survives is a question about the
locale, not about `@Q`.** In a UTF-8 locale `café` → `'café'`; under
`LC_ALL=C`, which is what the oracle runs with (`oracle.md`), the same
value is `$'caf\303\251'`, because no byte of it is character-shaped to a
shell reading one byte at a time. **This implementation does not have a
locale** and always takes the UTF-8 reading, so it answers `'café'` where
a `LC_ALL=C` bash answers with the octal. That is a deliberate divergence
rather than a gap: a Go program decoding UTF-8 is the behavior a user of
this library gets on every machine, and the alternative is a locale
database this substrate has no other reason to carry. It is also why no
corpus row pins a multibyte `@Q` — a row would record the C-locale answer
and grade the implementation against an environment it does not model.

**Transformations distribute over a whole array.** `"${a[@]@Q}"` is one
quoted word per element and `"${a[*]@Q}"` joins them; the same holds for
`"${@@Q}"` and `"${*@Q}"` over the positional parameters, and for every
letter. An empty or unset array is zero fields, status 0.

**`@A` writes the statement that would recreate the variable.** A name
with no attributes is `x='a b'` (the value `@Q`-quoted); attributes put
`declare -irx v='1'` in front, the letters in a fixed order rather than
the order they were set — measured `a A i r t x l u`, so `declare -xril`
prints `irxl` and `declare -tirx` prints `irtx`; a name
whose value is unset drops the `='…'` half — `declare -A h` for an
associative table read without a subscript. A positional parameter has no
name to write and yields empty, while `"${@@A}"` yields the words
`set -- 'one' 't w'`, one field each. On `"${a[@]@A}"` the fields are the
words of the statement: `declare` `-a` `a=([0]="1" [1]="x y")` — the
elements double-quoted, an associative table's list with a trailing space
before the `)`.

**`@K` and `@k` list keys and values; on anything but a stored array they
are `@Q`.** `"${a[@]@K}"` is a *single* field, keys bare and values
double-quoted (`\"` and `\\` escaped, `$'…'` when control characters
force it): `0 "one" 1 "t w"` — an associative table's pairs each carry a
trailing space instead of the joining space. `"${a[@]@k}"` is the same
pairs as *separate* fields, values unquoted: `0` `one` `1` `t w`. A
scalar, a scalar read as `${x[@]}`, and the positional parameters all
answer with the `@Q` quoting and no keys at all.

**bash 3.2 does not refuse the family on an `[@]` subscript — it ignores
it.** `${x@Q}` on a scalar is a bad substitution there, as expected of a
build predating the operator, but `${a[@]@Q}`, `${a[@]@U}` and
`${a[@]@K}` all yield the array's *plain elements*, status 0. The
subscript swallows the trailing `@`-letter rather than the expansion
being rejected, so a script that means to detect the missing feature by
watching for the error gets no error and quietly wrong values instead.
Recorded by `param/transform-distributes-over-an-array` and
`param/transform-keys-and-values`.

**It is bash 3.2's alone.** ksh93 and zsh both refuse `${a[@]@Q}` — a
bad substitution at status 1, exactly as they refuse the scalar spelling
— so a dialect that reproduced the swallow reproduced the wrong shell.
This implementation did: a subscript is read before the operator is, and
an expansion the grammar had already marked unreadable still carried the
`[@]`, so the array shape answered and the refusal was never reached.

Citation: oracle runs against bash 5.3.15, ksh93u+ 2012-08-01, zsh 5.9.2
and dash on 2026-09-05. The corpus rows under `param/transform-…` pin the
panel's answers for every letter — `transform-quotes-for-reuse` (`Q`),
`transform-expands-escapes` (`E`), `transform-prompt-escapes` (`P`),
`transform-writes-an-assignment` (`A` and `a`),
`transform-keys-and-values` (`K` and `k`), `transform-case-letters`
(`L`, `U`, `u`) — plus `transform-takes-exactly-one-letter` for the
one-letter rule, `transform-distributes-over-an-array` for the
distribution, and `transform-deferred-in-a-branch-never-taken` for when
the rejection happens.

Two claims above are deliberately **not** pinned, and both for the same
reason: their answer is not a property of the shell. `@P` on `\t` or
`\w` records the clock or the directory, so the row probes `\n` and `\\`
only; and multibyte `@Q` records the locale, as above.

### What this implementation answers

Nine of the ten letters are implemented from the table above. **`@P` is
refused out loud** — `${x@P}: the @P transformation is not implemented` —
because prompt expansion is the front end's language, over state (`\u`,
`\w`, `\h`, the clock) the interpreter does not hold, and returning the
value unchanged would be right only for a value carrying no escape:
a silent wrong answer for every value that carries one. The corpus row
`param/transform-prompt-escapes` therefore records a conformance
difference on purpose, which is the honest shape for a deferred feature.

The four differences #577 recorded are settled, three as answers a
dialect gives and one as core behavior:

- **What the sentence names** is `Diagnostics.BadSubstitutionNames`, with
  three values because the panel gives three. bash names the run of the
  word that shares the expansion's quoting, with the quote characters
  off; ksh93 names the whole word exactly as written; dash and zsh name
  nothing at all, so their wordings have no verb and never reach it.
- **Whether the letter is checked on a name with no value** is
  `Semantics.TransformLetterCheckedOnlyWhenValued`, reached only by a
  grammar that *has* the family. It is an axis with one measured answer
  on purpose: making an operator's validity depend on what a variable
  holds is not a rule anything should inherit by having a `@` family, so
  the shell that does it has to say so.
- **The status of a bad letter** follows
  `Diagnostics.ExpansionFailureStatusFromCommandString`, the field the
  unset-parameter route already used, on the strength of the measurement
  above: it is the same 127-under-`-c` rule, and every other unreadable
  operator keeps the ordinary fatal status.
- **Abandoning at the first** is not an axis. Every column stops at the
  first expansion it cannot answer — for a bad substitution and for
  `set -u` alike — so the word stops at its first failing span and the
  command stops at its first failing word.

The `@`-letter and the diagnostic rows are
`param/bad-substitution-names-the-word`,
`param/bad-substitution-names-only-one-quoting`,
`param/bad-substitution-stops-at-the-first` and
`param/an-unset-name-stops-at-the-first-too`.

## Parenthesized expansion flags — zsh only

`${(flags)name}` opens with a parenthesized flag group before the parameter.
zsh alone parses it; the other three treat the whole expansion as a bad
substitution — bash and dash at run time when the expansion is reached, ksh93
while reading (`` `x}' unexpected ``), which is the same split the
`BadSubstitutionAtParseTime` flag already records. Found in the wild:
`/opt/homebrew`'s `ruby-lsp-activate.sh` opens with `${(%):-%x}`, the idiom
for "the path of the file being sourced", so a parser without the construct
refuses the file.

All measurements below are of zsh 5.9.2 (Homebrew, arm64). The zsh manual
(`zshexpn(1)`, Rules) confirms the ordering observed.

### The flags in scope, measured

| flag | meaning | probe | result |
| --- | --- | --- | --- |
| `(U)` / `(L)` | upper- / lowercase every letter | `x=abC; ${(U)x}` | `ABC` |
| `(q)` | quote with backslashes | `x='a b$c'\''; ${(q)x}` | `a\ b\$c\'` |
| `(qq)` | quote in single quotes | `x="a b"; ${(qq)x}` | `'a b'` |
| `(qqq)` | quote in double quotes | `x="a b"; ${(qqq)x}` | `"a b"` |
| `(qqqq)` | quote as `$'…'` | `x="a b"; ${(qqqq)x}` | `$'a b'` |
| `(f)` | split at newlines | `x=$'a\nb'; ${(f)x}` | two words `a`, `b` |
| `(s:sep:)` | split at sep | `x=a:b:c; ${(s.:.)x}` | `a`, `b`, `c` |
| `(j:sep:)` | join with sep | `a=(x y z); ${(j.,.)a}` | `x,y,z` |
| `(@)` | keep array fields in `"…"` | `a=(x "y z" ""); "${(@)a}"` | 3 fields, empty kept |
| `(P)` | value is a further name | `y=hello; x=y; ${(P)x}` | `hello` |
| `(k)` | keys of an associative array | `typeset -A m=(k1 v1); ${(k)m}` | `k1` |
| `(v)` | with `(k)`: key and value pairs | `${(kv)m}` | `k1 v1` interleaved |
| `(%)` | expand prompt `%` escapes | `${(%):-%x}` | see below |
| `(M)` | substitute what the pattern took | `v=hello; ${(M)v#h*l}` | `hel` |
| `(o)` / `(O)` | sort a list up / down | `a=(c a b); ${(@o)a}` | `a b c` |
| `(n)` | sort by the numbers in the words | `a=(10 9 1); ${(@n)a}` | `1 9 10` |
| `(i)` | sort with case folded away | `a=(B a); ${(@i)a}` | `a B` |
| `(u)` | keep the first of each repeat | `a=(b a b); ${(@u)a}` | `b a` |
| `(a)` | order by the index, not the text | `a=(c a b); ${(@a)a}` | `c a b` |
| `(Q)` | remove one level of quoting | `v="'a b'"; ${(Q)v}` | `a b` |
| `(c)` | with `${#…}`: characters, joined | `a=(abc de f); ${(c)#a}` | `8` |
| `(w)` | with `${#…}`: words | `v="a b  c"; ${(w)#v}` | `3` |
| `(W)` | with `${#…}`: words, empties too | `v="a b  c"; ${(W)#v}` | `4` |

Details, each measured:

- **The ordering flags are one step, and `u` is not a sort.** `o` and `O`
  order a list up and down, `n` reads each run of digits in a word as a
  number — `x1 x9 x10`, and `a1b a2b a10b`, so the digits need not be the
  whole word — and `i` folds case, which makes ties reachable and they
  keep the order the elements were written in. `i` sorts on its own.
  `u` keeps the first of each repeat, folds nothing, and leaves the order
  alone; with a sort beside it either order of the two agrees, and only
  `u` written alone says which runs first.

  **The locale decides what `o` means.** Under `LC_ALL=C`, which is what
  the corpus runs in, it is byte order and `(B a C b)` comes back
  `B C a b`. Under a UTF-8 locale the same shell orders by the collation
  and answers `a b B C`. This implementation is byte order in both, so a
  non-C locale's collation is a **measured gap** rather than a claim.

  **`(a)` is the sort key rather than a sort.** It orders by the
  element's own position, so ascending is the order the elements were
  written in and `O` reverses it — `a=(c a b); ${(@a)a}` is `c a b` and
  `${(@aO)a}` is `b a c`, in either spelling. And it *wins* over the
  comparisons rather than composing with them: `${(oa)a}`, `${(na)a}`
  and `${(ia)a}` are all `c a b` on that data, where a reading in which
  `o` still decided would answer `a b c`. `u` still runs ahead of it.

  So `(a)` belongs with the sorts and not with `(A)` and `(e)`, which do
  change what the subject is. The two differ only in case and in nothing
  else, which is the mistake this note exists to stop.

  Where the step sits is measured rather than read off the rule numbers,
  and it is **last of the group**: after the operator
  (`a=(zb ya); ${(@o)a#z}` is `b ya`), after the case conversion
  (`a=(B a); ${(@oU)a}` is `A B`), after splitting, after joining — so
  `${(oj.-.)a}` is `c-a-b` unsorted, one word being already in order,
  and so is `"${(o)a}"`, which the quoted join has already made one word
  — and after the prompt escapes and the quoting too:

  | probe | zsh 5.9.2 | if the sort ran first |
  | --- | --- | --- |
  | `a=("%x" "*"); ${(@%o)a}` | `*` then the path | the path then `*` |
  | `a=("a b" "a!"); ${(@qo)a}` | `a!` then `a\ b` | `a\ b` then `a!` |
  | `a=("'z'" b); ${(@Qo)a}` | `b` then `z` | `z` then `b` |

  The last two are what moved it. `*` sorts ahead of a path only once
  `%x` has become one, `a!` ahead of `a\ b` only once the space has
  become a backslash, and `b` ahead of `z` only once the quotes have
  gone. This implementation had the step between the case conversion and
  the quoting, which passed every row above that does not involve `%`,
  `q` or `Q` and was wrong on all three of these.

- **`(M)` reads a match from the other side, and reaches exactly two
  operators.** On the four trims it substitutes the part the pattern *took*
  rather than the part it left, with the operator still choosing how much:
  `${(M)v#h*l}` on `hello` is `hel` and `${(M)v##h*l}` is `hell`, against
  `lo` and `o` without it. A pattern that matches nothing substitutes
  nothing — `${(M)v#zzz}` is empty where `${v#zzz}` is the whole value —
  and an empty pattern takes the empty string. On `:#` it keeps the
  elements the pattern matched instead of dropping them:
  `a=(f1 f22 f333); ${(M@)a:#f2*}` is `f22` where `${(@)a:#f2*}` is `f1
  f333`.

  Everywhere else it does nothing, and that was measured one operator at a
  time rather than reasoned from the name: a replacement, an anchored
  replacement, a substring, the conditionals, `:|`, `:*` and an expansion
  with no operator at all are all what they would have been without it.
  Written twice it is written once.

- **Order of application.** Operators run before flags: `${(U)x:-def}` on an
  unset `x` is `DEF`, `${(U)x#h}` on `hello` is `ELLO`, `${(U)u:=def}`
  assigns `def` and substitutes `DEF`. `(P)` is the exception and runs
  *first*: `${(P)x:-def}` with `x=y` and `y=val` is `val`, and with the
  target unset the default fires on the resolved value. The manual's rule
  list agrees: name replacement (4), double-quoted joining (5), modifiers
  (7), forced joining (10), splitting (11), case (12), prompt escapes (13),
  quoting (14).
- **Arrays are elementwise.** `a=(ab cd); ${(U)a}` is `AB CD`; `${(q)a}` on
  `("x y" z)` is `x\ y z`. In double quotes without `(@)` the elements are
  first joined with the first character of `$IFS` (rule 5); `"${(@U)a}"`
  and `"${(U)@}"` keep one field per element — `$@` and an `[@]` subscript
  keep their fields without needing `(@)`.
- **Splitting forces fields even inside quotes.** `"${(s.:.)x}"` on `a::b`
  is two fields; only `"${(@s.:.)x}"` keeps the empty one as a third. An
  unquoted result always drops empty words. `(f)` on an empty value is no
  field unquoted and one empty field as `"${(@f)x}"`. `(s)` on an array
  joins with `$IFS`'s first character before splitting (rule 10):
  `a=(a:b c:d); ${(s.:.)a}` is `a`, `b c`, `d`. An empty separator
  `(s::)` splits into characters. Separator delimiters may be any
  punctuation — `(s.:.)`, `(s:,:)` — or the matched pairs `()`, `[]`,
  `{}`, `<>`; the separator may be several characters.
- **`(q)` in detail.** Backslash-escapes space and `` ` ``, `$`, `"`, `'`,
  `\`, `*`, `?`, `[`, `]`, `(`, `)`, `{`, `}`, `<`, `>`, `|`, `;`, `&`,
  `~`, `#`, `^`, `=`; leaves `!`, `%`, `:`, `,`, `.`, `/`, `@`, `-`, `_`,
  `+` and alphanumerics alone; renders each control or non-UTF-8 byte as
  its own `$'…'` segment — `$'\n'`, `$'\t'`, `$'\a'`, `$'\b'`, `$'\f'`,
  `$'\r'`, `$'\v'` by name, anything else as three-digit octal like
  `$'\033'`. An empty value is `''`. `(qq)` wraps in single quotes with
  `'` written as `'\''`, unconditionally — `plain` becomes `'plain'`.
  `(qqq)` wraps in double quotes escaping `\`, `` ` ``, `"`, `$`.
  `(qqqq)` wraps in `$'…'` escaping `'`, `\`, `!` and control bytes as in
  `(q)`. Multibyte UTF-8 passes through every form.
- **`(Q)` removes quoting and expands nothing**, and that pair is the
  whole flag. `v='"$x"'` with `x` set is `$x`, two characters, where a
  reading that handed the value to the parser would answer `hi`;
  `$(echo hi)` and a backquote stay text; a tilde and a `*` stay text.
  It makes no fields either — `${(@Q)v}` on `"'a b' c"` is one word.
  `'…'` takes its contents whole, `"…"` gives a backslash meaning before
  `"`, `` ` ``, `$` and `\` and drops a `\`-newline pair altogether while
  leaving the backslash in place anywhere else (`"a\qb"` is `a\qb`),
  `$'…'` is decoded, and a bare backslash escapes whatever follows it.
  `$"` is not a quote, so `$"x"` is `$x`.

  **An opener with no closer comes back as written** — `'unterm` is
  `'unterm` — rather than as an error, a truncation, or the contents
  without the opener. A substitution is a verbatim region for the same
  reason: `"a$(b"c")d"` is `a$(b"c")d`, so the quotes inside it neither
  closed the outer one nor were removed, and `"a$(x b"` comes back whole
  because the unclosed group takes the quote with it. `` ` ``…`` ` ``,
  `$(…)` (parentheses counted, so `$(b(c))` is one) and `${…}` all
  behave that way, inside quotes and out.

  Written beside `q` it runs **after** it, which the manual's single
  rule 14 does not say: `${(Qq)v}` and `${(qQ)v}` on `'a b'` are both
  `'a b'`, the round trip, where a `Q` that ran first would have left
  `a\ b`.
- **`(c)`, `(w)` and `(W)` change what a length counts**, and do nothing
  at all to a value — `${(@c)a}` is the plain expansion. The **last** of
  the three written wins: `${(cw)#v}` on `a b` is 2 and `${(wc)#v}` is 3.

  `c` is the characters of the words joined, separators included, so
  `a=(abc de f); ${(c)#a}` is 8 and not 6. **The separator it counts is
  a space and not `$IFS`'s first character** — measured, `IFS=:` leaves
  it at 8 — and a `j` argument does replace it: `${(cj.--.)#a}` is 10.

  `w` counts words and `W` counts the empty ones too, an element at a
  time with the totals added — `a=(a: :b)` with `(s.:.)` is 3 and 4,
  where a join first would have made both one lower. The separator is
  the `s` argument where one was written, a newline where `f` was
  (`${(fw)#v}` on `a\nb\n` is 3), and `$IFS` otherwise — and that last
  is a different *rule* rather than a different value:

  Each cell below is `w` / `W`, measured on the same value read three
  ways:

  | value | `$IFS` default | `IFS=:` | `(s.:.)` |
  | --- | --- | --- | --- |
  | `""` | 0 / 0 | 0 / 0 | 1 / 1 |
  | `"::"` | 1 / 1 | 3 / 3 | 1 / 3 |
  | `":a:"` | 1 / 1 | 3 / 3 | 2 / 3 |
  | `"a b  c"` | 3 / 4 | 1 / 1 | 1 / 1 |

  Under `$IFS`, `W` lets every separator delimit while `w` collapses a
  run of IFS *whitespace* and trims it from both ends — so a
  non-whitespace `IFS` makes the two agree. A trailing separator still
  opens a field for `w`, which ordinary word splitting absorbs, and it
  is the trailing *run* that decides rather than the last byte:
  `IFS=': '` counts `a: ` as 2 and `a ` as 1. With an explicit
  separator, `W` is every field the literal split makes while `w`
  collapses runs of it and drops a leading empty field but keeps a
  trailing one; an empty separator counts characters, and an empty value
  is one field where under `$IFS` it is none.

  **The length is taken before the double-quoted join and before the
  split**, which is not where the rule numbers put it: `"${(U)#a}"` on
  `(abc de f)` is 3 and not 8, `"${(Uj.-.)#a}"` is 3 as well, and
  `${(s.:.)#v}` on `:a:` is the value's 3 characters rather than the
  fields the separator would have made. This implementation answered 8
  to the first of those until #935's second change.
- **`(k)`/`(v)` order.** zsh yields hash order, which it does not promise;
  this implementation yields sorted key order, the same deterministic
  answer `${m[@]}` already gives. `(k)` on anything that is not an
  associative array is a no-op, and `(v)` matters only beside `(k)`.
- **`(%)` prompt escapes.** `%x` and `%N` both name the file being read:
  under `-c` the shell's own name (`zsh`), in a script the script's path,
  in a sourced file the sourced file's path. Inside a function `%N` is the
  function's name while `%x` stays the defining file. `%%` is a literal
  `%`. The escapes apply to the value — `x="%x"; ${(%)x}` expands — and
  elementwise on arrays. `%n` is the user the shell runs as. zsh
  implements its whole prompt language here (`%M` the host, `%~` the
  directory, `%D` the date); this implementation carries only `%x`, `%N`,
  `%n` and `%%`, and refuses anything else loudly rather than answering
  wrong. Louder than the shell, in fact — measured, `${(%):-%zz}` in zsh
  expands `%z` to nothing at all and carries on at 0, where this refuses
  by name at 1. Silence is the worse of the two failures, so the
  difference is deliberate.

  **`%n` is a fact about the process, not about the environment.** It is
  the login name for the shell's real uid, and it ignores `USER`,
  `LOGNAME` and `USERNAME` — assigned inside the shell or injected before
  it starts. Measured on zsh 5.9.2 and on bash's `\u`, which ignores them
  too, so this is unanimous wherever the escape exists at all. Reading one
  of those variables would make `env USER=someone-else zsh` draw the wrong
  person. `USERNAME` is not even assignable: zsh answers `failed to change
  group ID` and leaves it, because the name is bound to the uid.

  So the value is *carried in* rather than read here —
  `Runner.SetPromptUser`, filled in by the shell binaries beside the `$UID`
  they already read — which keeps the process lookup out of a package that
  may be embedded twice in one program. A runner nobody told refuses `%n`
  with the rest rather than expanding it to nothing.

  The same escape is `%n` in a prompt, and it means the same thing there:
  measured in both positions in one shell. The prompt renderer in `repl/`
  resolves it from `$USER` and `$LOGNAME` instead, which is a divergence
  from the panel that this does not close — see the note in that file.
- **An empty name is legal once flags are present.** `${(U)}` is an empty
  string, and `${(%):-%x}` — the wild idiom — has no name at all: the `:-`
  fires and the flags apply to the substituted word. `${()x}` with empty
  parens is `${x}`.
- **An unrecognized flag is a runtime error**, not a parse one: `zsh:1:
  error in flags near position 4 in '${(Y)x}'`, status 1, and the line is
  abandoned; inside a branch never taken it is never diagnosed
  (`if false; then : ${(!)x}; fi` runs clean). The position is 1-based and
  counts from the `$`. A missing closing parenthesis is diagnosed the same
  way at the first character that is not a flag: `${(Ux}` errors at
  position 5.

### What the corpus pins

The table above is measured, and the rows that hold the panel to it are
`param/expansion-flags-are-one-dialects` (`(U)`, and the three-way split
on the shells without the construct), `-split-and-join` (`(s)`, `(j)`),
`-quote-four-ways` (`(q)` through `(qqqq)`), `-split-at-newlines`
(`(f)`), `-at-keeps-array-fields` (`(@)`, against the IFS join a quoted
array otherwise gets), `-name-indirection` (`(P)`),
`-keys-and-values` (`(k)`, `(kv)`), `-run-after-the-operator` (the
ordering rule and `(P)`'s exception to it), and
`param/prompt-percent-names-the-script` (`(%)`).

The flags that order, count and unquote have rows of their own:
`param/the-ordering-flags-sort-a-list`, `-and-case`,
`param/the-unique-flag-is-not-a-sort`,
`param/the-index-is-an-ordering-key` (`(a)`),
`param/the-length-flags-count-characters-and-words` (`(c)`, `(w)`,
`(W)`), `param/the-unquoting-flag-removes-one-level` (`(Q)`), and two
rows for where the steps sit —
`param/a-length-is-taken-before-the-joining-and-the-split` and
`param/where-the-unquoting-and-ordering-steps-sit`.

`(k)` is pinned with a **single** pair. zsh yields hash order for more
than one and does not promise it, so a row with two keys would record a
coin flip as evidence; the sorted order this implementation yields is
stated above and asserted in its own unit test rather than against the
panel.

### What this implementation refuses

Flags zsh has and this slice does not — `(A)` (array assignment), `(e)`
(expand the result again), `(z)` (split by shell parsing), `(t)`, `(D)`,
padding, and the rest of the alphabet, plus the `q-`/`q+` variants and
`(qqq…)` beyond four — are refused at run time naming the flag, with the
same fatal shape as an unrecognized one. Refusing loudly is the honest
answer where imitating would answer wrong, and the refusal is asserted
whole rather than assumed: `TestTheUnbuiltFlagsAreStillRefusedByName`
holds it as each letter is built, because a letter added to the
implemented set is a letter taken out of the guarantee that let this list
be enumerated exactly.

Three of them were measured while the rest of #935 was built, and the
measurements are here so the next change starts from them:

- **`(e)`** expands the result again — parameters, command substitutions
  and arithmetic. `w=zz; v='$w'; ${(e)v}` is `zz` and
  `v='$(echo hi)'; ${(e)v}` is `hi`; a bare `1+2` is *not* arithmetic and
  stays `1+2`.
- **`(A)`** makes an assignment an array assignment:
  `${(A)x::=a b c}` substitutes `a b c` and leaves `x` an array of three,
  which `${(t)x}` reports as `array`. It needs the assignment side of
  the expansion rather than the value side, which is why it is not
  beside the others.
- **`(z)`** splits the value the way the shell reads a line, keeping the
  quoting: `v='echo "a b" c'` is three words, the middle one still
  `"a b"` — which is the pair `${(Q)${(z)line}}` real configuration is
  written with. Operators are words of their own (`;`, `|`, `&&`, `;;`,
  `(`, `)`), a newline becomes `;`, an `IO` number joins the redirection
  after it so `2>&1` is `2>&` and `1`, and an unterminated quote is no
  error at all — `echo "unterminated` is two words, the second one with
  its opening quote still on it.

  **`(z)` needs one lexical decision this slice has not taken**: a `#`
  is not a comment there. `v="a # b"` is three words, and that does not
  change with `interactive_comments` either way, so the split runs a
  lexer in a mode where the character is ordinary. Everything else it
  needs, `syntax.Lexer` already answers — including the unterminated
  quote, whose token arrives with the right text before the error does.
  Whether "a `#` begins a comment" becomes a `syntax.Dialect` value, and
  whether a runtime option rather than a dialect is what varies it, is
  the question standing in front of the flag.

### Grammar

The group is read only when the dialect's `ParamExpansionFlags` is on;
elsewhere `${(…)…}` follows the bad-substitution split above — deferred to
run time via the `Bad` node everywhere but the parse-time dialect. The
parsed node carries the flag letters in order plus the two separators
(`SplitSep`, `JoinSep`); the printer writes the span back raw, so the
construct round-trips.

## A tilde at the front: `${~spec}` — zsh only

A `~` written between the `${` and the parameter makes the *result* of the
substitution eligible for tilde expansion and filename generation, whatever
the `GLOB_SUBST` option is set to. zsh alone has it. The other four call the
whole expansion unreadable, in the same three-way split every unreadable
expansion follows: bash 5.3, bash 3.2, bash-as-sh and dash say `bad
substitution` when the expansion is reached, ksh93 refuses it while reading
(`` `~' unexpected ``) — which `BadSubstitutionAtParseTime` already records.

Found in the wild, and this is why the construct is here rather than on a
list: `~/.zi/bin/zi.zsh:141-159` is eighteen of them in a row —

    ZI[HOME_DIR]=${~ZI[HOME_DIR]}
    …
    ZPFX=${~ZPFX}

— so a shell that refuses the substitution leaves eighteen variables *empty*
and every path the plugin manager later builds is rooted at nothing. The
construct is not a stylistic corner; refusing it corrupts state rather than
merely reporting.

All measurements below are of zsh 5.9.2 (Homebrew, arm64), taken 2026-09-06
against fixtures in a scratch directory with `HOME` pointed into it.

### What it does, measured

With `g='DIR/inn*.sh'` matching three files, `t='~/zz'` a directory, and
`typeset -A A; A[h]='~/zz'`:

| written | result |
| --- | --- |
| `${g}` | `DIR/inn*.sh` — the value, unchanged |
| `${~g}` | the three matched paths, as three words |
| `"${~g}"` | `DIR/inn*.sh` — quoting suppresses it |
| `${~t}` | `HOME/zz` |
| `${~t}/sub` | `HOME/zz/sub` |
| `${~A[h]}` | `HOME/zz` — a subscript is no obstacle |
| `${~1}`, `${~@}` | the positional parameters, each one expanded |

Three behaviors carry the construct and each is asserted rather than
inferred from the absence of an error:

- **Quoting suppresses it**, exactly as quoting suppresses ordinary filename
  generation. `"${~p}"` is the value unchanged.
- **The unquoted word form splits into as many words as the pattern
  matched.** `w=( ${~g} )` is a three-element array, not one holding a
  pattern.
- **`${~~p}` turns it back off**, which is how a nested use says "not here".

### The count is parity, and it overrides the option

`GLOB_SUBST` is the option that makes *every* expansion's result eligible.
The written tildes do not toggle it — they decide the answer outright, on the
parity of how many were written, measured under the option both ways:

| written | `unsetopt globsubst` | `setopt globsubst` |
| --- | --- | --- |
| `${g}` | the value | the matches |
| `${~g}` | the matches | the matches |
| `${~~g}` | the value | the value |
| `${~~~g}` | the matches | the matches |
| `${~~~~g}` | the value | the value |

So one tilde is "yes" and two are "no" from either starting point, and only
a `spec` with no tilde at all consults the option. The same table holds for
the tilde half: `${t}` is `~/zz` with the option off and `HOME/zz` with it
on, while `${~t}` is `HOME/zz` and `${~~t}` is `~/zz` regardless.

`GLOB_SUBST` itself is recorded-and-inert in this implementation, so the
zero-tilde row is the dialect's `GlobExpansionResults` answer and nothing
else reaches it yet.

### Where the tilde may be written

After the parenthesized flag group and before everything else. Measured:

| written | zsh |
| --- | --- |
| `${(U)~g}` | flags then tilde: read, uppercased, then matched |
| `${~(U)g}` | `bad substitution` — the group may not follow the tilde |
| `${(U)~#g}` | the *length*, so the tilde precedes `#` as well |
| `${#~g}` | `bad substitution` |
| `${~+x}` | `${+x}`, the is-it-set count, with the tilde applied to it — `${+x}` is a gap here, so both spellings are a `bad substitution` |
| `${~!x}` | `bad substitution`, as `${!x}` is here anyway |
| `${~}` | the empty string, no error — as `${}` is in this shell |

`^` and `=` share the position and are interchangeable with it — `${^~a}`
and `${~^a}` agree, `${=~g}` and `${~=g}` agree — and neither is in this
slice.

### It applies to the value the expansion came to

The operator runs first and the tilde marks what the operator produced, which
is the same ordering the flag group has:

    v='XDIR/inn*.sh'; ${~v#X}    →  the three matched paths
    ${~undef:-DIR/inn*.sh}       →  the three matched paths
    v='~/zz'; ${~#v}             →  4, the length of `~/zz`

The last one is the ordering seen from the other side: `#` has already
reduced the value to a number, and a number holds no tilde and no
metacharacter, so the flag has nothing left to do.

Tilde expansion runs before filename generation, on the *head* of the value
and nowhere else:

    v='~/*'      →  every entry of HOME
    v='a:~/zz'   →  `a:~/zz`, the tilde is not at the head
    v='~/zz ~/qq' split   → *both* directories: the value split first and
                             every field it produced is at the head of a
                             word of its own

A list expands elementwise, and each element is the head of its own word:
`a=('~/zz' '~/qq'); ${~a[@]}` is both expanded, while `X${~a[@]}` is
`X~/zz` and `HOME/qq` — the prefix took the first element out of head
position and left the second in it. **Field splitting makes the same
list**, which is measured and was recorded here the other way round until
it was: `setopt shwordsplit; v='~/zz ~/qq'; ${~v}` is both directories, and
`${=~v}` is both without the option. Splitting runs first and the fields it
made are the heads, so the two spellings of a list agree.

`~user` and a named directory (`hash -d`) both resolve in zsh; this
implementation leaves `~user` as written, the same limit its literal tilde
expansion already has, and has no named directories at all.

### The other half is the pattern half

Marking the value a pattern is the same question `GlobExpansionResults`
already answers, so the flag reaches every place that answer is read, and
that is measured rather than assumed:

    p='a*'; [[ abc == ${~p} ]]        →  true   (${p} alone is false)
    p='a*'; case abc in ${~p})        →  matches
    v=abc; p='a*'; ${v#${~p}}         →  `bc`   (${p} alone leaves `abc`)

And the run-time option still wins over it, because the option is about the
filesystem pass and not about what the word is: `setopt noglob; ${~g}` is
the value unchanged. A pattern matching nothing is `no matches found` and
fatal, which is `GlobNoMatchIsError`; under `nullglob` the word is dropped.

### Grammar

The tilde run is read only when the dialect's `ParamTildeFlag` is on;
elsewhere a leading `~` is not a name and the expansion follows the
bad-substitution split above. The parsed node carries `TildeFlags`, the
number of tildes written, so parity is the interpreter's to take and the
printer keeps writing the span back raw — the construct round-trips as
source text.

`ParamTildeFlag` and `ParamCaseChange` never coexist in a dialect, and would
not collide if they did: bash's case-toggle `~` follows the name (`${x~}`)
and this one precedes it.

### What the corpus pins

`param/the-tilde-flag-is-one-dialects` (the three-way split),
`-quoting-suppresses-it`, `-doubled-turns-it-off`, `-expands-a-tilde`
and `-marks-a-pattern-operand`.

### What this implementation does not match

Two divergences, both inherited from where this implementation does its
literal tilde expansion — the first span of a word — rather than from the
flag:

- `""${~t}` and `${empty}${~t}` expand here, because nothing was
  accumulated in front of them, which agrees with zsh. But `x${~t}` does
  not, and neither does zsh's; the disagreement is narrower than it looks
  and no measured case is missed by it.
- `q=a:${~t}` is `a:HOME/zz` in zsh: a substituted tilde is eligible in
  every position a written one would be, and an assignment's colons are
  such a position. Here the colon rule (`expandColonTildes`) reads literal
  spans only, so the substituted tilde stays. `q=a:~/zz` written out is
  correct in both.

Separately, `${}` and `x${}y` are the empty string in zsh and a `bad
substitution` here. `${~}` is right because the tilde relaxes the empty-name
rule the way a flag group does; `${}` is a gap of its own. `${+x}` — the
is-it-set count — has its own section below, and `${~+x}` reads.

## A `+` at the front: `${+name}` — zsh only

A `+` written between the `${` and the parameter asks whether the parameter
is **set**, and substitutes `1` or `0` rather than its value. zsh alone has
it. The other four call the whole expansion unreadable, in the same
three-way split every unreadable expansion follows: bash 5.3, bash 3.2,
bash-as-sh and dash say `bad substitution` when the expansion is reached,
ksh93 refuses it while reading (`` `+' unexpected ``) — which
`BadSubstitutionAtParseTime` already records.

**It never fails**, and that is the whole reason a script writes it: it is
the guard that stands in front of everything else, so a shell that raises an
error where the answer is `0` turns every guard into a hard stop rather than
a false.

Found in the wild, and this is the scale of it: `${+` appears **138 times**
in `~/.zi/bin/zi.zsh`, 42 of them `${+functions[…]}` — an order of magnitude
more than any other single construct that file gated on. The 42 are the
spelling almost every caller uses to read `$functions`, so a shell with that
parameter and without this flag cannot be asked the question the parameter
exists to answer.

All measurements below are of zsh 5.9.2 (Homebrew, arm64), taken 2026-09-06.

### What it answers

| written | result |
| --- | --- |
| `v=1; ${+v}` | `1` |
| `${+NOPE}` | `0` |
| `v=1; unset v; ${+v}` | `0` |
| `v=; ${+v}` | `1` — **set-ness is not emptiness** |
| `typeset v` (no value) | `1` |
| `a=(x y z); ${+a}` | `1`, one field — not the element count and not the join |
| `typeset -A m; ${+m}` | `1` |
| `${+a[2]}`, `${+a[9]}` | `1`, `0` — an element rather than the name |
| `${+m[k]}`, `${+m[nope]}` | `1`, `0` |
| `${+functions[f]}` | `1` when the function is defined |
| `${+aliases[x]}`, `${+commands[ls]}`, `${+options[xtrace]}` | the same question of each special table |
| `set -- A B; ${+0} ${+1} ${+2} ${+3}` | `1 1 1 0` |
| `${+10}` | `0` with two parameters — a positional may be several digits |

`v=; ${+v}` is the row that separates this from `${v:+1}`, which fires on
the empty value, and from `${v+1}`, which agrees here and disagrees
elsewhere: an implementation that reused either would get most of the table
right.

### It does not trip `set -u`

    set -u; echo ${+NOPE}     →  0, and the shell carries on
    set -u; echo ${NOPE}      →  `NOPE: parameter not set`, fatal

Asking whether a name exists without reading it is the whole of the
construct. The same holds through a flag group: `set -u; v=nosuchvar;
${(P)+v}` is `0`.

### The name it may carry is a name or a positional, and nothing else

Measured by asking for each, and the answer is not the one "is it set"
suggests:

| written | zsh |
| --- | --- |
| `${+x}`, `${+_x1}`, `${+0}`, `${+10}` | the count |
| `${+@}` `${+*}` `${+#}` `${+?}` `${+$}` `${+!}` `${+-}` | `bad substitution` |
| `${+}` | `bad substitution` — and `${~}` is the empty string, so this flag does *not* relax the empty name the way the tilde and the group do |
| `${(U)+}`, `${~+}` | `bad substitution` as well |
| `${++x}` | `bad substitution` — there is no doubled spelling, so no parity |
| `${+${x}}` | `bad substitution` |

The refusal is **deferred to the run**, like every other unreadable
expansion in this dialect: `if false; then echo ${+?}; fi` is silent.

### Where the `+` may be written

After the parenthesized flag group and after the tilde run, and in front of
everything else:

| written | zsh |
| --- | --- |
| `${~+x}` | the count, with the tilde applied to it |
| `${+~x}` | `bad substitution` — the tilde precedes, never follows |
| `${=+x}` | the count; `${=x}`'s flag shares the tilde's slot and so precedes this one too |
| `${+=x}` | `bad substitution` |
| `${^+x}` | the count — the third character of that slot, and not a flag this implementation has |
| `${+^x}` | `bad substitution` |
| `${~=+x}`, `${=~+x}` | the count either way, since the slot's characters are interchangeable within it |
| `${(U)+x}` | the count; a value transformation has nothing to transform |
| `${+(U)x}` | `bad substitution` |
| `${+#x}` | `bad substitution` — a length may not follow it |
| `${#+x}` | **not this construct at all**: the `#` is read first, so this is `$#` with `+x` as an alternate word, and it is `x` for any set `#` |

`(P)` is the one flag that reaches the answer, because it changes *which*
parameter is being asked about rather than what its value becomes:

    w=1; v=w;         ${(P)+v}  →  1
    v=nosuchvar;      ${(P)+v}  →  0     (${(P)v} is empty, ${+v} is 1)
    v=abc;            ${(U)+v}  →  1     (not an uppercased anything)
    a=(1 2); ${(j.-.)+a}        →  1

### Beside an operator it has no effect at all

Not the answer, and not a refusal either — the expansion is exactly what it
would have been without the `+`:

    v=abc;  ${+v#a}    →  bc
    v=abc;  ${+v:-x}   →  abc
    v=abc;  ${+v+y}    →  y
            ${+nope:-D}→  D
            ${+nope#a} →  the empty string
            ${+v=W}    →  W, and v is assigned
    v=abc;  ${+v[2]#a} →  b

So the count is the answer only for a plain reference. A length cannot
co-occur — `${+#v}` is refused while reading and `${#+v}` never carries the
flag — so the operator is the whole of the test.

### Grammar

The `+` is read only when the dialect's `ParamSetTestFlag` is on; elsewhere
a leading `+` is not a name and the expansion follows the bad-substitution
split above. The parsed node carries `SetTest`, a **bool** and not a count —
which is the one place this differs from `ParamTildeFlag`, and it is
measured rather than assumed: `${++x}` is a bad substitution, so there is no
second `+` for a parity to be about. It asks a question rather than setting
a mode.

What may follow the `+` is checked where the `+` is read rather than after
the name scan, because `${+#v}` would otherwise be read as a length over a
set test — a shape the shell that has the construct does not have.

The printer writes the span back raw, so the construct round-trips as
source text.

### What the corpus pins

`param/the-set-test-flag-is-one-dialects` (the three-way split, with the
empty-but-set name in the same row), `-does-not-trip-nounset` (with the
`alive` that a shell which gave up could not print) and
`-has-no-effect-beside-an-operator`.

### What this implementation does not match

`${+00}` is `1` in zsh and `0` here, which is inherited rather than the
flag's: `${00}` is the shell's own name there and the empty string here, so
a positional written with a leading zero is a gap of its own and this
follows it.

`${#+v}` is `v` in zsh — the `$#` reading above — and a `bad substitution`
here, which is a gap this change neither opened nor closed: `${#}` taking
an alternate word is the `${}` half of the same issue.

## An equals at the front: `${=spec}` — zsh only

An `=` written between the `${` and the parameter splits the *result* of the
substitution into words on `IFS`, whatever the `SH_WORD_SPLIT` option is set
to. zsh alone has it. The other four call the whole expansion unreadable, in
the same three-way split every unreadable expansion follows: bash 5.3, bash
3.2, bash-as-sh and dash say `bad substitution` when the expansion is
reached, ksh93 refuses it while reading (`` `=' unexpected ``) — which
`BadSubstitutionAtParseTime` already records.

Found in the wild, and this is why the construct is here rather than on a
list: `is-at-least`, the version predicate in zsh's own function library,
splits both of its operands with it —

    argv1=(${=1})
    argv2=(${=2:-$ZSH_VERSION})

— so a shell that refuses the substitution leaves both version arrays
**empty**, compares nothing, and answers "at least" for *every* version, at
status 0 and with no diagnostic. `~/.zi/bin/zi.zsh:205` branches on exactly
that call. A refusal names itself; this does not, and the wrongness travels.

All measurements below are of zsh 5.9.2 (Homebrew, arm64), taken 2026-09-06.

### What it does, measured

With `v='a b c'` and a function reporting `$#` and each field:

| written | fields |
| --- | --- |
| `${v}` | 1: `a b c` — the value, unchanged |
| `${=v}` | 3: `a`, `b`, `c` |
| `"${=v}"` | 3: `a`, `b`, `c` — **quoting does not suppress it** |
| `x${=v}y` | 3: `xa`, `b`, `cy` — the fields join the text around them |
| `${=v//b/x y}` | 4 — the operator runs first and the flag splits what it left |
| `${=#v}` | 1: `5` — a length is a number, which holds no separator |
| `${=a[@]}`, `${=@}` | the elements, each split |

Three behaviors carry the construct and each is asserted rather than
inferred from the absence of an error — the bug this replaces returned a
plausible value at status 0:

- **Quoting does not suppress it.** This is where the flag parts company
  with its sibling `${~spec}`, whose whole construct quoting turns off.
- **`${==v}` turns it back off**, which is how a nested use says "not here".
- **A context that never splits is not overridden.** `x=${=v}` is the value
  unchanged, and so are `[[ ${=v} = "a b" ]]`, a `case` subject and a
  here-document body.

### The count is parity, and it overrides the option

`SH_WORD_SPLIT` is the option that makes *every* unquoted expansion's result
split. The written `=` characters do not toggle it — they decide the answer
outright, on the parity of how many were written, measured under the option
both ways:

| written | `unsetopt shwordsplit` | `setopt shwordsplit` |
| --- | --- | --- |
| `${v}` | 1 field | 3 fields |
| `${=v}` | 3 | 3 |
| `${==v}` | 1 | 1 |
| `${===v}` | 3 | 3 |
| `${====v}` | 1 | 1 |

So one `=` is "yes" and two are "no" from either starting point, and only a
`spec` with no `=` at all consults the option. The zero-`=` row is the
dialect's `SplitParamExpansion` answer.

### Where the `=` may be written

The same slot the tilde flag uses, and the two are interchangeable within
it. Measured:

| written | zsh |
| --- | --- |
| `${(U)=v}` | flags then `=`: read, uppercased, split |
| `${=(U)v}` | `bad substitution` — the group may not follow the run |
| `${=#v}` | the *length*, so the run precedes `#` as well |
| `${=~v}`, `${~=v}` | the same expansion: both flags, in either order |
| `${=}` | the empty string, no error — as `${~}` is |
| `${#=v}` | **not this flag**: `${#=word}` is `$#` with a default assigned to it, so `set -- p q` makes it `2`. A gap here either way |

### It is one string that is split, not each element

A list is joined on `IFS`'s first character and the join is what splits,
which is measured and is the difference a per-element implementation gets
wrong:

    a=(' x ' y); "${=a[@]}"   → 3 fields: ``, `x`, `y`
                                (splitting each element would give 4)
    a=(); "${=a[@]}"          → one empty field, where `"${a[@]}"` is none

That is `UnquotedListJoinsOnIFS` reached from the other side: the flag says
"read this as one value and split it", and what a list comes to as one value
is already answered.

### The edges, quoted and unquoted

The split is field splitting on `IFS`, with one difference between the two
quotings — quoted, a delimiter at either end of the value still separates:

| `v` | `${=v}` | `"${=v}"` |
| --- | --- | --- |
| `a b` | 2: `a`, `b` | 2: `a`, `b` |
| `a  b` | 2 | 2 — a run of whitespace is one delimiter either way |
| `' a '` | 1: `a` | 3: ``, `a`, `` |
| `'  '` | 0 fields | 2, both empty |
| `''` | 0 fields | 1, empty |
| `a::b` with `IFS=:` | 3: `a`, ``, `b` | 3, the same |

`IFS` set and empty disables the stage entirely, as it does everywhere else:
`IFS=; ${=v}` is the value.

### Beside a flag group it is a step *inside* the group

`${(U)=v}` does not split what the group produced — it splits at the group's
own splitting rule, which runs before the ordering, the prompt escapes, the
quoting and the case conversion. Three measurements fix it there:

    v='c a b'; ${(o)=v}   →  a b c    the sort sorts fields, not one word
    v='a b';   ${(q)=v}   →  a, b     the quoting ran after the split
    v=' a ';   "${(U)=v}" →  ``, A, `` the fields survive the quotes

`(f)` and `(s)` *are* that rule with a separator of their own, so an `=`
beside either adds nothing: `v='a b'; ${(s.,.)=v}` is one field, and
`${(s.,.)==v}` still splits — the group decides and the parity is never
consulted.

### Grammar

The `=` run is read only when the dialect's `ParamSplitFlag` is on;
elsewhere a leading `=` is not a name and the expansion follows the
bad-substitution split above. The parsed node carries `SplitFlags`, the
number written, so parity is the interpreter's to take and the printer keeps
writing the span back raw — the construct round-trips as source text.

It cannot collide with the `${x=word}` assignment: that `=` follows a name
and this one precedes it, so position tells them apart before either is
read.

### What the corpus pins

`param/the-split-flag-is-one-dialects` (the three-way split),
`-reaches-through-quotes`, `-doubled-turns-it-off` and
`-does-not-reach-an-assignment`.

### What this implementation does not match

- **A trailing non-whitespace separator.** `IFS=:; v='a:'; ${=v}` is two
  fields in zsh, the second empty, and one here. That is not this flag: it
  is the splitter, which absorbs a trailing delimiter the way POSIX, bash,
  ksh93 and dash all do and zsh alone does not — `setopt shwordsplit;
  IFS=:; $v` divides the panel the same way. An axis of its own, and the
  quoted form above is right because the edge-keeping rule is written here.
- `GLOB_SUBST` is recorded-and-inert, so `setopt globsubst; ${=g}` splits
  but does not then match — which the tilde flag's section already records
  for its own half.
- `${v::=word}`, the always-assign operator, is unread, so `${=v::=p q}` is
  too.

## A subscript without braces — zsh only

Everything above is written `${ … }`. zsh also reads a subscript and a
length on a parameter written **without** braces, and the difference is
in the grammar rather than in what a shared syntax means: the same
characters are a different number of words in the two shells.

Measured 2026-09-05 on zsh 5.9.2, bash 5.3.15, bash 3.2.57 and dash,
with `a=(x y z)`:

| probe | zsh | bash 5.3 | bash 3.2 | dash |
| --- | --- | --- | --- | --- |
| `"[$a[1]]"` | `[x]` | `[x[1]]` | `[x[1]]` | no arrays |
| `[$a[1]]` unquoted | `no matches found: [x]` | `[x[1]]` | `[x[1]]` | no arrays |
| `"[$#a]"` | `[3]` | `[0a]` | `[0a]` | `[0a]` |

To zsh, `$a[1]` is one expansion of the first element and `$#a` is the
array's count. To everything else, `$a[1]` is `$a` followed by the three
characters `[1]` — a glob pattern, once the quotes come off — and `$#a`
is `$#` followed by the letter `a`.

Both halves fail quietly in the direction that matters. `$#a` is a
number in either reading, so a script that tests it against zero tests
the count in one shell and the string `0a` in the other, and nothing
says so. `$a[1]` is louder only by accident: zsh's default `nomatch`
turns the *other* direction into an error, so a bash script's
`$dir[0-9]*` read as a subscript would fail visibly, which is why the
form is behind a flag and not in the core.

### Which parameters take one

Not all of them, and the two operators stop in different places.

A **subscript** follows a name, `@` and `*`. It does not follow a
positional digit: `set -- abcd; echo "[$1[2]]"` is `[abcd[2]]` in every
shell in the panel, zsh included
(`array/a-subscript-without-braces-is-not-a-positional`). Exactly one is
read — `"[$a[1][1]]"` is `[x[1]]`, not a character of `x`
(`array/a-subscript-without-braces-is-read-once`).

zsh also subscripts the remaining specials: `$?[1]`, `$-[2]`, `$$[1]`
and `$0[2]` all index the parameter's value. The brace-less spelling
leaves those out, because the parsed form has nowhere to put them — the
inner text of a span is what `${ … }` would hold, and there `${#[1]}` is
a length and `${![1]}` an indirection. The **braced** spellings are read,
under `SpecialParamSubscript` above; it is the two the span cannot write
down that are recorded rather than modeled.

A **length** takes a name, a digit, `@` or `*`: `$#a` is a count, `$#0`
and `$#1` are the lengths of those parameters, and `$#@` and `$#*` are
the number of positional parameters. It stops before `#` and before `!`
— `$##` is the count and then a literal `#`, and `$#!` the count and then
a literal `!`, both exactly as in bash
(`array/a-length-without-braces-stops-at-two-specials`).

### Grammar

Grammar flag `BareSubscript`, consumed by the **lexer**, because the
word boundary is what changes and nothing downstream can recover it once
the spans are cut. The span it produces is the one `${ … }` would have
produced — `$a[1]` and `${a[1]}` are one node — so the parser, the
interpreter and the subscript's arithmetic are unchanged, and the
printer writes the braced spelling back.

Where the subscript may reach is the quoting's question rather than a
fixed set of characters. Inside double quotes a blank and a newline are
ordinary text, so `"$a[1 ]"` and `"$a[1` + newline + `]"` are both the
first element; unquoted, either one ends the word. A closing quote stops
the scan in the quoted case, which `"$a[" ]` shows: there is a `]` in
that line and no subscript.

An unquoted `[` that never closes before the word does is not a
subscript here and its characters stay literal. zsh commits to the
subscript instead and reports `invalid subscript` at run time — for the
unclosed unquoted form and for `"$a[" ]` alike; the shape fails either
way, and the difference is the wording.

## A subscript's own flag group — zsh only

A subscript may open with a parenthesized group of flags of its own,
`${a[(re)value]}`, which makes the subscript a **search** rather than a
count. It is not the `${(flags)name}` group above in a second position;
the two are different constructs that happen to share a punctuation.

Measured 2026-09-06 with `a=(alpha beta gamma beta delta)`:

| shell | `echo "[${a[(r)beta]}]"` |
| --- | --- |
| zsh 5.9.2 | `[beta]` |
| bash 5.3.15 | `(r)beta: arithmetic syntax error in expression (error token is "beta")` |
| bash-as-sh | the same, naming the invocation instead of the script |
| bash 3.2.57 | `(r)beta: syntax error in expression (error token is "beta")` |
| ksh93u+ | `(r)beta: arithmetic syntax error` |
| dash | `Syntax error: "("` — at `a=( … )`, having no arrays to subscript |

One shell has it and the rest read the same characters as arithmetic, so
it is an **additive** grammar flag rather than a semantics axis. The
evidence for "additive" is stronger than the usual four-against-one,
because the shell that has the construct *also* reads the arithmetic:
`${a[(z)2]}` is `bad math expression: operator expected at `2'` in zsh
and an arithmetic failure in each of the others. There is no text this
flag gives a second meaning to. It gives a meaning to text that had none.

### The whole flag set, measured by exhaustion

Every ASCII letter was asked twice — bare, `${a[(X)2]}`, and with an
argument, `${a[(X:1:)2]}` — and the ones that did not fall back to
arithmetic are the flag set. There are thirteen and no others:

    w f p e i I r R k K        no argument
    b n s                      an argument, in any delimiter pair

Four of them select, and they are mutually exclusive — the **last one
written** wins, `${a[(ri)be*]}` being `2` and `${a[(ir)be*]}` being
`beta`:

| written | is |
| --- | --- |
| `${a[(r)pat]}` | the first element the pattern matches, or nothing |
| `${a[(R)pat]}` | the last such element |
| `${a[(i)pat]}` | the index of the first match, or one past the last element |
| `${a[(I)pat]}` | the index of the last match, or one before the first |

and three modify the search:

| written | is |
| --- | --- |
| `${a[(e)…]}` | the operand is a plain string, not a pattern |
| `${a[(n:expr:)…]}` | the expr'th match rather than the first |
| `${a[(b:expr:)…]}` | the search starts at element expr rather than at the end |

`(re)` is the combination the scripts use: the first element **equal**
to the operand. With `[[ -z … ]]` in front of it that is the idiom for
"is this directory already on the path", which is what
`~/.zi/bin/zi.zsh:161-193` is doing six times.

### A group that selects nothing changes nothing

`${a[()2]}` and `${a[(e)2]}` are both the second element. A group with
no `r`, `R`, `i` or `I` in it says how a search would have run and no
search was asked for, so the subscript behind it is read exactly as a
subscript without a group: arithmetic, `@`, `*`, a range, or an
associative key. `(e)` alone is the sharpest case — `${a[(e)beta]}` is
**empty**, because `beta` is still an arithmetic expression there and an
unset name is zero.

### What the operand is: the text as written, and there is no quoting

A pattern, unless `(e)`. What it is a pattern *of* is the subscript's
text with its substitutions performed and **nothing else touched** — a
subscript is not a quoting context at all, and the quote characters in
one are ordinary characters. Measured three ways, with
`b=('"beta"' beta)` — the first element's value is six characters:

| written | is | so |
| --- | --- | --- |
| `${b[(r)"beta"]}` | `"beta"` | the quotes are matched, not removed |
| `${b[(r)"$h"]}`, `h=beta` | `"beta"` | the value was substituted *inside* them |
| `${b[(re)"beta"]}` | `"beta"` | and exact matching sees them too |

That single rule explains the pattern half without an axis of its own:
a substituted value's metacharacters are simply live, because nothing
escaped them.

    g='be*'; ${a[(r)$g]}   # beta
    ${(@)a:#$g}            # removes nothing, in the same shell

The second line is the dialect's ordinary answer for the result of an
expansion. The first is not an override of it — it is a position where
the question never arises.

It is never matched against the *filesystem*: `${a[(re)be*]}` in a
directory with no `be*` in it is empty rather than `no matches found`.

### `n` and `b`, measured

Both arguments are arithmetic expressions rather than numerals, and
neither is parameter-expanded: `(rn:1+1:)` is the second match, `(rn:k:)`
with `k` unset is the first, and `(rb:$one:)` fails in the arithmetic on
the `$`.

- `n` below one is one: `(rn:0:)` is the first match.
- `n` past the last match answers the no-match value — `(rn:9:)` empty,
  `(in:9:)` one past the end, `(In:9:)` zero.
- `b` names an element in the array's own base, and a negative one counts
  back from the end: with five elements `b:-1:` is the fifth and `b:-5:`
  the first.
- `b` **outside** the array is not clamped to its nearest end. With five
  elements `${a[(Ib:6:)*a]}` is `0` and `${a[(ib:6:)*a]}` is `6`: neither
  direction searches at all, where clamping would have found element 5.
- Forward searches (`r`, `i`) run from `b` upward; reverse ones (`R`,
  `I`) run from `b` downward, so `${a[(Rb:3:)*a]}` is `gamma`.

### A name that holds nothing, and an array that holds nothing

They are different. `${nosucharray[(i)x]}` is **empty**, while a declared
but empty array answers `${b[(i)x]}` with `1` and `${b[(I)x]}` with `0`.
An unset name is not searched at all.

### Grammar

Grammar flag `ArraySubscriptFlags`. The group is read by the **parser**,
where the subscript's text is still text — `ParamExpr.IndexFlags` carries
it and `ParamExpr.Subscript()` is the operand behind it, so every reading
of a subscript asks one accessor and none of them sees the group.
`ParamExpr.Index` keeps the subscript as written, which is what a
diagnostic naming `a[(re)x]` needs.

Malformed is not an error. A character the group cannot carry, an
argument-taking flag with no argument, an argument with no closing
delimiter and a group with no closing parenthesis all mean the
parentheses were never a group — the subscript stands as written and is
read as arithmetic. That is measured (`${a[(z)2]}`, `${a[(n)2]}` and
`${a[(n:2)x]}` are all `bad math expression`) and it is what makes the
flag purely additive: with the flag off, nothing reads differently.

The **lexer** needs it too, for the brace-less spelling: `(` ends a word,
so `$a[(r)b]` was a syntax error naming the parenthesis — which is worse
than a wrong answer, because it takes the whole file with it. A group the
grammar can read is stepped over as a unit and nothing else about where a
bare subscript ends changes.

### What this implementation carries, and what it refuses by name

`r R i I e n b` are carried, for an ordinary array and for the positional
parameters.

`w f p k K s` are read by the grammar and **refused by name** when the
subscript is reached — `${a[(w)x]}: the (w) subscript flag is not
implemented` — for the reason the expansion flags are: a subscript flag
answered wrong returns a plausible element at status 0, which is the one
failure this repository exists to avoid.

Two more refusals are about the *target* rather than the letter, and both
replace a silent wrong answer:

- On an **associative array** a search reads keys for `i` and `I` and
  values for `r` and `R`, and `I` and `R` there answer with *every* match
  rather than one, in the hash's order. A different construct wearing the
  same letters. Before this, `${h[(r)v1]}` looked up a key literally
  called `(r)v1` and quietly found nothing.
- On a **scalar** a search is a search for a substring and what comes
  back is a character position: `s="one two three"; ${s[(r)two]}` is `t`.

`a[(r)y]=Q` — a flag group on the left of an **assignment** — is a
parse error here and an element replacement in zsh. The group is read
from a subscript's text and an assignment's subscript is cut by the lexer
before there is any text to read, so it needs its own change rather than
this one.

An index reported by `(i)` or `(I)` is the base plus the element's
*position*, which is the same number only while the array has no gaps.
It has none in the shell that has this construct — `a=(x); a[5]=y` there
leaves five elements and `${a[(i)y]}` is 5 — so the two readings agree
everywhere this can be written today.

### What this implementation does not match

A backslash before an **ordinary** character. zsh keeps both characters —
with an element whose value is the four characters `bet\a`,
`${a[(r)bet\a]}` finds it and does *not* find `beta` — where the pattern
language this implementation shares between `case`, `[[ ]]` and pathname
expansion reads `\a` as an escaped `a`. A backslash before a
metacharacter agrees: `${a[(r)be\*]}` finds an element whose value is
`be*` in both. The disagreement is one character wide and belongs to
patterns.md rather than to this construct, which is why it is recorded
here and not worked around here.

### What the corpus pins

`array/a-subscript-takes-its-own-flag-group` (the six-shell split),
`array/a-subscript-flag-group-searches-both-ways`, `-exact-matching`,
`-unknown-flag-is-arithmetic`, `-operand-is-text-as-written`,
`-that-selects-nothing` and `-counts-matches-and-moves-the-start`.

## Choosing elements: `:#`, `:|` and `:*` — zsh only

Three operators that change **which elements** a value has, where the
substring above changes which characters each one has.

    ${a:#pattern}   drop the elements the pattern matches
    ${a:|other}     drop the elements the array named `other` holds
    ${a:*other}     keep only those

They share the `:` with the substring, and that is the whole difficulty:
the single character after the colon decides which construct was
written, before anything can be evaluated. Grammar flag
`ParamElementSelection`, for the reason `BareSubscript` is one — the
panel does not disagree about what these characters *mean*, it cuts the
word in different places, and three shells cut it three ways:

| `v=hello` | bash 5.3 / 3.2 / as-`sh` | dash | ksh93 | zsh |
| --- | --- | --- | --- | --- |
| `${v:#hel*}` | `operand expected`, 1 | `Bad substitution`, 2 | `lo` | *empty* |
| `${v#hel*}` | `lo` | `lo` | `lo` | `lo` |

bash reads an offset and hands `#hel*` to arithmetic. **ksh93 accepts
the same six characters and means `${v#hel*}` by them** — the colon is
simply ignored, so it is prefix removal and not exclusion. Only zsh has
the operator, and it matches the pattern against the **whole** value.
Treating `:#` as `#` with a colon in front would answer ksh93's question
inside zsh's grammar, silently.

### What the operator does

The pattern is matched whole and never against a part, which is the
entire difference from `#`. Consequences, all measured:

    a=(one two three); ${(@)a:#t*}     one
    a=(one two);       ${(@)a:#*}      no fields at all
    a=("" one);        ${(@)a:#}       one — the empty pattern matches
                                       the empty string and nothing else
    a=(One two);       ${(@)a:#one}    One two — case-sensitive

A scalar is the one-element list it is, so the answer is the value or
nothing — and *nothing* here is one empty field, where an emptied list
is no field at all. `set -- "${v:#hel*}"` leaves `$#` at 1 and
`set -- "${(@)a:#*}"` leaves it at 0.

The pattern is built from the operand's **word**, so whether a
metacharacter out of a parameter is a metacharacter is
`GlobExpansionResults` — the same axis that decides whether
`p='et*'; echo $p` expands. In the shell that has the operator that axis
is false, so `p='t*'; ${(@)a:#$p}` removes nothing.

`:|` and `:*` take the **name** of another array rather than a word, and
compare elements for **equality** rather than by pattern: an element
`t*` in the other array removes only a literal `t*`. A name nothing is
stored under is an empty set, quietly — the difference keeps everything,
the intersection keeps nothing, and neither complains.

### The disambiguation is exactly one character

Every other spelling after a colon is what it always was, and all of
them are unanimous across the panel:

    ${v:2}  ${v:2:2}  ${v: -2}  ${v:-alt}  ${v:=set}  ${v:?msg}  ${v:+set}

`${v:}` is too short to carry an operator and stays the substring with an
empty offset. A grammar that widened the rule by one character would
break the shapes every shell shares rather than the ones only zsh has.

## A process substitution in an operand — bash only

    unset u
    ${u:-<(:)}      →  /dev/fd/63   in bash
                    →  <(:)         in ksh93, zsh and dash
    v='<:'; ${v#<(:)}
                    →  ``           in ksh93 and zsh — a `<` and the group `(:)`
                    →  `<:`         in bash (the pattern is a path) and dash

Whether `<(cmd)` opens a process substitution **inside a `${…}` operand**
is a question about the position, not about the shell: ksh93 and zsh both
have the construct in an ordinary word and neither reads one here. Every
operand answers alike — the pattern of `${v#…}`, the replacement of
`${v/…/…}` and the word of `${v:-…}`.

Grammar flag: `ProcessSubstitutionInParamOperand` — bash only; false for
`core` and `posix`, on the additive rule, since it is a construct one
shell admits in a place the others do not.

It matters most where the operand is a pattern, because there the wrong
answer is silent: the text the substitution *encloses* is never the
pattern in any column, and reading it that way trimmed a bare `x` off
`${v#<(x)}` and matched `case x in <(x))` (#902).

## An expansion where a name belongs — zsh only

    v=abc
    ${${v}}              →  abc
    ${${v}#a}            →  bc
    ${${${v}#a}%c}       →  b
    ${(U)${(L)v}}        →  ABC
    ${$(echo abc)#a}     →  bc

zsh alone. bash 3.2, bash 5.3 and dash answer `bad substitution` for
every one of them and ksh93 a syntax error, so this is a grammar flag —
`NestedParamExpansion` — and not a semantics axis: without it there is no
parameter name at the front of `${${v}#a}` at all, so the characters are
unreadable rather than differently read, and there is nothing for a value
to switch between.

It is not a corner. `${${0:#$ZSH_ARGZERO}:-${(%):-%N}}` is how a plugin
manager installed on this machine finds the file being sourced, and the
form appears in two of the third-party files a real startup reads.

**The inner expansion is the whole of the name position.** Measured:
`${x${v}}` and `${${v}x}` are both a bad substitution *in zsh too*, so
text either side of the nesting is not a longer name. Every operator may
follow the inner brace — trims, replacement, substring, the four
conditionals, `:#`, a modifier — and the length prefix may sit in front
of the whole thing. Depth is not limited to one.

The inner need not be another `${…}`: a `$(…)` command substitution and a
`$((…))` arithmetic expansion stand in the same position. The backquoted
spelling does not, which is measured rather than assumed — ``${`echo x`#a}``
is a bad substitution in zsh, so the two spellings of command
substitution part company exactly here.

### What is read and not implemented

Two shapes are recognized and refused **by name**, because answering them
approximately is the failure this document exists to prevent:

    ${${a[@]}}     an inner that comes to a list
    ${${v}[2]}     a subscript on the result

In zsh the first keeps its fields and the outer operator applies to each
of them — `${${a}#o}` on `(one two)` is `ne` and `two` — and the second
indexes the string the inner came to. Joining the first would answer with
one plausible field, and dropping the second would answer with the
unindexed value; both would be silent. `a nested expansion of a list is
not implemented` and `a subscript on a nested expansion is not
implemented` are what they say instead.

## Dialect flags

    ParamSubstitution      ${x/pat/rep} and its anchored forms
    ParamSubstring         ${x:off:len}
    ParamCaseChange        ${x^^} ${x,,} ${x~~} and their single forms — bash only
    ParamIndirection       ${!x}, ${!prefix*}, ${!a[@]} — parses in bash and ksh;
                           what ${!x} then *means* diverges (see above)
    ParamTransformations   ${x@Q} and its letter family — bash only
    ParamExpansionFlags    ${(U)x}           — zsh only
    ParamTildeFlag         ${~x}, the tilde-and-filename flag — zsh only
    ParamSplitFlag         ${=x}, the split-into-words flag — zsh only
    ParamSetTestFlag       ${+x}, the is-it-set count — zsh only
    ParamElementSelection  ${a:#pat} ${a:|b} ${a:*b} — zsh only
    BareSubscript          $a[1] and $#a, written without braces — zsh only
    ArraySubscriptFlags    ${a[(re)v]}, a flag group inside the brackets
                           — zsh only
    SpecialParamSubscript  ${@[1]}, ${1[2]}, ${?[1]} — a subscript on a
                           parameter that is not a name — zsh only
    ProcessSubstitutionInParamOperand
                           ${u:-<(:)} carries one — bash only
    NestedParamExpansion   ${${v}#a}, an expansion where a name belongs
                           — zsh only

All false for `posix`. `ParamCaseChange`, `ParamIndirection`,
`ParamTransformations`, `ParamExpansionFlags`, `ParamTildeFlag`,
`ParamSplitFlag`, `ParamSetTestFlag` and
`ParamElementSelection` are false for `core`:
the first and the last two are one shell's, and the `!` family is two
shells' — neither is a common denominator. `BareSubscript` is false for
both, and for the same reason as `ParamExpansionFlags`: one shell reads
those characters that way and three read them as text.
`SpecialParamSubscript` is false for both as well, and its evidence is
stronger still: every other member of the panel refuses the expansion
outright. `ArraySubscriptFlags` is false for both on evidence stronger
again: the four shells that lack it read `${a[(r)v]}` as arithmetic, and
so does the shell that has it whenever the group is not one it knows. `NestedParamExpansion` is false for both on the same evidence:
the other five columns refuse it, in three different wordings.

## What this does not cover

The pattern-matching language itself — what `*`, `?`, `[…]` and the
bracket-expression classes match — is shared with pathname expansion and
with `case`, so it belongs in its own document rather than being described
three times.
