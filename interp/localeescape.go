// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import "fmt"

// A `\uHHHH` or `\UHHHHHHHH` naming a code point the locale's encoding cannot
// represent.
//
// The escape's *reading* is settled elsewhere — how many digits it takes, and
// that the value is the original UTF-8 rather than the range Unicode later
// kept, see [EncodeCodePoint]. This is the question after it: the two shells
// that have the escape both consult the locale before writing anything, and in
// a locale that cannot hold the character they part company.
//
// Measured 2026-09-11, bytes read with `od -An -tx1`, on zsh 5.9.2 and bash
// 5.3.15, from `echo "a\u00e9Z"` and the same word under `echo -e`:
//
//	              LC_ALL=C                        LC_ALL=en_US.UTF-8
//	zsh 5.9.2     `character not in range`, then  61 c3 a9 5a
//	              61 alone
//	bash 5.3 -e   61 5c 75 30 30 45 39 5a         61 c3 a9 5a
//
// so the two agree in a UTF-8 locale — which is what a person's terminal has,
// and where this shell was already right — and give different answers in the C
// locale. `\u0041` is `A` in both, in every locale, which is why every corpus
// row about the escape's *reading* keeps to ASCII (#1851).
//
// The same split at every site that reads the escape, measured one at a time
// rather than assumed from `echo`: `print` without `-r`, a `printf` format, a
// `%b` argument, and `$'...'`. One reader here for all of them, because a
// second copy of an escape's rules is how the control escape came to mean two things
// (#556) and how `print` came to write a replacement character where zsh
// writes the encoding (#1840).
//
// # Whether this shell has a locale at all
//
// It does, and that is decided rather than invented here: see "Locale, decided
// as a policy" in docs/spec/semantics.md, which settled it once for every
// operator (#367) and which case conversion already follows. `LC_ALL` over
// `LC_CTYPE` over `LANG`, and a plain assignment is enough — no export needed.
// The reader below is [Runner.multibyteLocale]'s, so the encoding question has
// one answer in this package rather than two.

// localeRefusesCodePoint reports whether the locale in force cannot hold a
// code point.
//
// Three states and not two, which is the part worth writing down. A locale
// variable naming UTF-8 holds everything; one naming anything else holds ASCII
// and no more; and **nothing named at all is left alone**, because the panel
// disagrees about that case and this shell's answer to the disagreement is not
// this axis's to settle. Measured 2026-09-11 under `env -i`, with no locale
// variable set anywhere:
//
//	echo "a\u00e9Z"       bash 5.3  61 c3 a9 5a  zsh 5.9.2  not in range
//	s=héllo; echo ${#s}    bash 5.3  5            zsh 5.9.2  6
//
// So bash reads an unset locale as UTF-8-capable and zsh reads it as C, and
// the split is not about this escape: the string-length row splits the same
// way, and this shell answers zsh's there. Narrowing it here would change an
// answer nobody measured for this operator while claiming to fix the one that
// was measured, so unset keeps the behavior it already had and the
// disagreement is filed on its own.
//
// ASCII and no more, for a locale naming a single-byte encoding that is not
// ASCII: bash transcodes there, writing U+00E9 as the single byte `e9` under
// `en_US.ISO8859-1`, and this shell has no charset tables. It is the limit
// [Runner.multibyteLocale] already records for the same reason — every
// non-UTF-8 encoding counts bytes — and it is stated rather than hidden, since
// the alternative is a table per charset.
func (r *Runner) localeRefusesCodePoint(n int) bool {
	if n <= 0x7f {
		// ASCII is representable in every encoding a shell is asked for, and
		// the boundary is measured rather than guessed: `\u007f` is the DEL
		// byte in both shells under `LC_ALL=C`, and `\u0080` — the very next
		// code point — is refused by zsh and written back by bash.
		return false
	}
	for _, name := range localeVariables {
		v, _ := r.getVar(name)
		if v == "" {
			continue
		}
		return !codesetIsUTF8(v)
	}
	// Nothing names a locale. See above: not this axis's question.
	return false
}

// unicodeEscapeSpelling is the escape written back the way the shell that
// leaves it standing writes it, which is **normalized** rather than echoed.
//
// Measured 2026-09-11 on bash 5.3.15 under `LC_ALL=C`, and worth the
// measurement because three separate things about it are not the input:
//
//	written       stands as     why
//	\ue9         \u00E9       padded to four digits
//	\u00e9       \u00E9       upper-cased
//	\U000000e9   \u00E9       the letter follows the value, not the input
//	\u20AC       \u20AC       unchanged when it already has that shape
//	\U1F600      \U0001F600   eight digits once it will not fit in four
//
// So the letter is chosen by whether the value fits in four digits, and the
// digit run is always four or eight. A site that wrote the source text back
// would agree with bash on exactly one of those five.
func unicodeEscapeSpelling(n int) string {
	if n <= 0xffff {
		return fmt.Sprintf(`\u%04X`, n)
	}
	return fmt.Sprintf(`\U%08X`, n)
}

// CodePointEscapeText is what one `\u` or `\U` escape becomes, with the
// locale consulted: the encoded character, or the answer the dialect gives for
// a code point the locale cannot hold.
//
// A true second return is the refusal, and the caller's part in it is to stop
// writing at that point — the text before the escape still reaches the stream,
// measured: `echo "a\u00e9Z"` under `LC_ALL=C` writes `a` and a newline in
// zsh and nothing else. [Runner.RefuseCodePoint] is the other half, and is
// called once per command however many escapes the word holds, which is also
// measured: two of them draw one complaint.
//
// Exported because a dialect's own escape reader needs the same answer and
// must not grow a second one — `print` is zsh's and lives in dialect/zsh.
func (r *Runner) CodePointEscapeText(n int) (string, bool) {
	if !r.localeRefusesCodePoint(n) {
		return EncodeCodePoint(n), false
	}
	switch r.outsideLocaleEscape() {
	case OutsideLocaleEscapeWritten:
		return unicodeEscapeSpelling(n), false
	case OutsideLocaleEscapeRefused:
		return "", true
	}
	// Unanswered, and outsideLocaleEscape has already said so and stopped the
	// script. Nothing more is written.
	return "", true
}

// RefuseCodePoint reports the refusal and abandons the script.
//
// Status 0, which is the surprising half and is measured: zsh writes
// `zsh:1: character not in range`, writes what came before the escape, and
// does not run the next command, while the script's status stays 0. A subshell
// absorbs it the way it absorbs any other abandonment, so
// `( echo "a\u00e9Z" ); echo AFTER` reaches AFTER.
//
// Exported for the same reason [Runner.CodePointEscapeText] is: `print` is a
// dialect's builtin and refuses in the same words.
func (r *Runner) RefuseCodePoint() {
	// Located as the *shell* and not as the builtin, which is measured and is
	// the tell that this is a fact about reading a word rather than about
	// `echo`: `zsh:1: character not in range`, with the same prefix from
	// `print`, from a `printf` format and from a `$\'...\'` in an assignment
	// that never reached a command at all. Runner.diagf would name the
	// builtin that happened to be running, which is right for a complaint the
	// builtin makes and wrong for this one.
	r.errf("%s\n", r.diag().Report(r.name(), r.line,
		Wording(r.diag().CodePointOutsideTheLocale, "character not in range")))
	r.status = 0
	r.ctl = controlExit
}
