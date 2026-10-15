// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package syntax

import "testing"

// In the grammar that takes any brace word before a redirection operator,
// the word between the braces is the token's, written exactly as it stands —
// and the shapes that are not one brace word stay words. Without the flag
// the same text is an ordinary argument. See
// Dialect.FdVariableAnyBraceWord (#6522).
func TestAnyBraceWordBeforeARedirectionIsADescriptorVariable(t *testing.T) {
	t.Parallel()
	anyWord := Core()
	anyWord.FdVariableRedirections = true
	anyWord.FdVariableSubscript = true
	anyWord.FdVariableAnyBraceWord = true
	names := Core()
	names.FdVariableRedirections = true
	names.FdVariableSubscript = true
	for _, c := range []struct {
		src, token string // token is "" where the text must stay a word
		d          Dialect
		dialect    string
	}{
		{`echo {a-b}<f`, "{a-b}", anyWord, "any"},
		{`echo {1}<f`, "{1}", anyWord, "any"},
		{`echo {1a}>f`, "{1a}", anyWord, "any"},
		{`echo {$x}<f`, "{$x}", anyWord, "any"},
		{`echo {x"y"}<f`, `{x"y"}`, anyWord, "any"},
		{`echo {a[$i]}<f`, "{a[$i]}", anyWord, "any"},
		{`echo {v}<f`, "{v}", anyWord, "any"},
		{`echo {a,b}<f`, "", anyWord, "any"},
		{`echo {}<f`, "", anyWord, "any"},
		{`echo {a-b}x<f`, "", anyWord, "any"},
		{`echo {a-b}<f`, "", names, "names"},
		{`echo {1}<f`, "", names, "names"},
		{`echo {a[$i]}<f`, "", names, "names"},
		{`echo {v}<f`, "{v}", names, "names"},
	} {
		t.Run(c.dialect+" "+c.src, func(t *testing.T) {
			t.Parallel()
			f, err := Parse(c.src, c.d)
			if err != nil {
				t.Fatal(err)
			}
			cmd := f.Stmts[0].Expr.(*Pipeline).Cmds[0].(*SimpleCmd)
			if len(cmd.Redirs) != 1 {
				t.Fatalf("read %d redirections, want 1", len(cmd.Redirs))
			}
			got := ""
			if n := cmd.Redirs[0].N; n != nil {
				got = n.Literal()
			}
			if got != c.token {
				t.Errorf("token %q, want %q", got, c.token)
			}
			if wantArgs := map[bool]int{true: 1, false: 2}[c.token != ""]; len(cmd.Args) != wantArgs {
				t.Errorf("read %d words, want %d", len(cmd.Args), wantArgs)
			}
		})
	}
}
