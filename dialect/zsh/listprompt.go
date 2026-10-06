// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh

import (
	"slices"
	"strconv"
	"strings"

	"github.com/blairham/sh/interp"
	"github.com/blairham/sh/repl"
)

// LISTPROMPT: the prompt zsh/complist draws under each screenful of a
// completion listing too tall for the terminal, and the parameter whose being
// set is what turns the paging on (#6153). repl's listscroll.go draws the
// listing; this is what is drawn under it.
//
// zshmodules(1) gives it the attribute escapes a prompt has — `%B`, `%S`,
// `%U`, `%F`, `%K`, their partners, and `%{…%}` — and three pairs of its own.
// Measured 2026-10-06 through a pseudo-terminal against zsh 5.9.2, a thousand
// matches in fifty rows on forty, the first screenful drawn:
//
//	LISTPROMPT='%SAt %l %m %p%s'      \e[7mAt 39/50 989/1000 Top\e[27m
//	LISTPROMPT='[%L][%M][%P]'         [39/50    ][989/1000 ][Top   ]
//	after one row more                [40/50    ][990/1000 ][80%   ]
//	LISTPROMPT=''                     \e[7mAt Top: Hit TAB for more, or the
//	                                  character to insert\e[27m
//
// and with twelve thousand matches `[39/706   ][11335/12000][Top   ]`. So `%l`
// is the last row drawn over the rows there are, `%m` the last match on that
// row over the matches there are, and `%p` is `Top` on the first screenful
// and otherwise the last row drawn as a whole percentage of them — 40 of 50
// is 80%, 41 is 82%. The capital forms pad on the right, to nine columns for
// the first two and six for the third, and a longer value is not cut. An
// empty LISTPROMPT is the default prompt above, and an unset one, or one set
// without zsh/complist loaded, pages nothing.

// listPromptParameter is the parameter, and listPromptDefault what an empty
// one draws.
const (
	listPromptParameter = "LISTPROMPT"
	listPromptDefault   = "%SAt %p: Hit TAB for more, or the character to insert%s"
)

// ListScrollPrompt is the prompt drawn under a screenful of a paged listing
// that stands where v says, and whether this session pages listings at all.
// See repl.Shell.ListScrollPrompt.
func ListScrollPrompt(r *interp.Runner, v repl.ListScrollView) (string, bool) {
	if !slices.Contains(zmodloadLoaded(r), moduleComplist) {
		return "", false
	}
	format, set := r.GetVar(listPromptParameter)
	if !set {
		return "", false
	}
	if format == "" {
		format = listPromptDefault
	}
	return explanationText(r, listPromptPositions(format, v), 0), true
}

// listPromptPositions writes the three pairs of position escapes into the
// format, each value's own percent signs doubled, and leaves every other
// escape for the prompt walker that draws the rest.
func listPromptPositions(format string, v repl.ListScrollView) string {
	var b strings.Builder
	for i := 0; i < len(format); i++ {
		if format[i] != '%' || i+1 == len(format) {
			b.WriteByte(format[i])
			continue
		}
		var value string
		width := 0
		switch format[i+1] {
		case 'l', 'L':
			value = strconv.Itoa(v.LastLine) + "/" + strconv.Itoa(v.Lines)
			width = 9
		case 'm', 'M':
			value = strconv.Itoa(v.LastMatch) + "/" + strconv.Itoa(v.Matches)
			width = 9
		case 'p', 'P':
			switch {
			case v.Top:
				value = "Top"
			case v.Lines > 0 && v.LastLine >= v.Lines:
				value = "Bottom"
			case v.Lines > 0:
				value = strconv.Itoa(v.LastLine*100/v.Lines) + "%"
			}
			width = 6
		default:
			// Another escape, `%%` included: the walker's.
			b.WriteString(format[i : i+2])
			i++
			continue
		}
		if format[i+1] >= 'A' && format[i+1] <= 'Z' {
			for len(value) < width {
				value += " "
			}
		}
		b.WriteString(strings.ReplaceAll(value, "%", "%%"))
		i++
	}
	return b.String()
}
