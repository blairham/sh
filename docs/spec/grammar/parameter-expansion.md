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

Details, each measured:

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

`(k)` is pinned with a **single** pair. zsh yields hash order for more
than one and does not promise it, so a row with two keys would record a
coin flip as evidence; the sorted order this implementation yields is
stated above and asserted in its own unit test rather than against the
panel.

### What this implementation refuses

Flags zsh has and this slice does not — `(A)` (array assignment), sorting
`(o)`/`(O)`, `(t)`, `(z)`, `(e)`, `(D)`, padding, and the rest of the
alphabet, plus the `q-`/`q+` variants and `(qqq…)` beyond four — are
refused at run time naming the flag, with the same fatal shape as an
unrecognized one. Refusing loudly is the honest answer where imitating
would answer wrong.

### Grammar

The group is read only when the dialect's `ParamExpansionFlags` is on;
elsewhere `${(…)…}` follows the bad-substitution split above — deferred to
run time via the `Bad` node everywhere but the parse-time dialect. The
parsed node carries the flag letters in order plus the two separators
(`SplitSep`, `JoinSep`); the printer writes the span back raw, so the
construct round-trips.

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

## Dialect flags

    ParamSubstitution      ${x/pat/rep} and its anchored forms
    ParamSubstring         ${x:off:len}
    ParamCaseChange        ${x^^} ${x,,} ${x~~} and their single forms — bash only
    ParamIndirection       ${!x}, ${!prefix*}, ${!a[@]} — parses in bash and ksh;
                           what ${!x} then *means* diverges (see above)
    ParamTransformations   ${x@Q} and its letter family — bash only
    ParamExpansionFlags    ${(U)x}           — zsh only
    ParamElementSelection  ${a:#pat} ${a:|b} ${a:*b} — zsh only
    BareSubscript          $a[1] and $#a, written without braces — zsh only
    SpecialParamSubscript  ${@[1]}, ${1[2]}, ${?[1]} — a subscript on a
                           parameter that is not a name — zsh only

All false for `posix`. `ParamCaseChange`, `ParamIndirection`,
`ParamTransformations`, `ParamExpansionFlags` and
`ParamElementSelection` are false for `core`:
the first and the last two are one shell's, and the `!` family is two
shells' — neither is a common denominator. `BareSubscript` is false for
both, and for the same reason as `ParamExpansionFlags`: one shell reads
those characters that way and three read them as text.
`SpecialParamSubscript` is false for both as well, and its evidence is
stronger still: every other member of the panel refuses the expansion
outright.

## What this does not cover

The pattern-matching language itself — what `*`, `?`, `[…]` and the
bracket-expression classes match — is shared with pathname expansion and
with `case`, so it belongs in its own document rather than being described
three times.
