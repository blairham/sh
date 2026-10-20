// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// **Borrowed text says what the parser remarked about it** (#5232): a
// here-document that runs out inside `eval`'d or sourced text gets the same
// warning it gets at top level.
//
// Nothing asked before. Remarks were written by the front end, for a script
// and for `-c`, and `eval` and `.` parse their text in interp — so every
// remark either produced was dropped. Measured 2026-09-30 against bash 5.3.20
// from /opt/homebrew/bin/bash, each row a script file.
//
// **The naming row is the one to read.** bash names `eval` on a parse error
// (`eval: line 5: syntax error…`) and does *not* name it on a remark (`line
// 3: warning: …`), in the same call. A remark is the parser speaking about the
// text, and in this dialect `eval`'d text has no name of its own, only the
// builtin's.
func TestEvalAndSourceReportTheirRemarks(t *testing.T) {
	dir := t.TempDir()
	hd := filepath.Join(dir, "hd.sh")
	if err := os.WriteFile(hd, []byte("x=$(cat <<EOF\nhi\nEOF)\necho \"[$x]\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		name, src, want string
	}{
		{
			"eval",
			"eval 'x=$(cat <<EOF\nhi\nEOF)'\necho \"[$x]\"\n",
			"[hi]\n<d>/case.sh: line 3: warning: here-document at line 1 delimited by end-of-file (wanted `EOF')\n",
		},
		{
			// Both numbers move with where the `eval` stands: bash continues
			// the caller's lines through `eval`'s text.
			"eval two lines down",
			":\n:\neval 'x=$(cat <<EOF\nhi\nEOF)'\necho \"[$x]\"\n",
			"[hi]\n<d>/case.sh: line 5: warning: here-document at line 3 delimited by end-of-file (wanted `EOF')\n",
		},
		{
			// The remark unnamed, the parse error named, one `eval` — and the
			// remark first, as the reference writes them.
			"a remark and a parse error in one eval",
			"eval 'x=$(cat <<EOF\nhi\nEOF)\nif'\necho st=$?\n",
			"st=2\n<d>/case.sh: line 3: warning: here-document at line 1 delimited by end-of-file (wanted `EOF')\n" +
				"<d>/case.sh: eval: line 5: syntax error: unexpected end of file from `if' command on line 4\n",
		},
		{
			// A sourced file is named by its own path, and counts its own lines.
			"a sourced file",
			":\n. " + hd + "\n",
			"[hi]\n<d>/hd.sh: line 3: warning: here-document at line 1 delimited by end-of-file (wanted `EOF')\n",
		},
		{
			// A remark on the read that then *fails*: the document runs out
			// inside a `$(` the text never closes, so the line reader gets
			// its remark and its error from one read, and finds nothing more.
			// The remark is written before that read's `break`, which is the
			// only thing that puts it out at all — written after, it is lost
			// with the loop.
			"a remark on the read that fails",
			":\neval 'x=$(cat <<EOF\nhi)'\necho \"[$x]\"\n",
			"[]\n<d>/case.sh: line 3: warning: here-document at line 2 delimited by end-of-file (wanted `EOF')\n" +
				"<d>/case.sh: eval: line 4: unexpected EOF while looking for matching `)'\n",
		},
		// The control: text with nothing to remark on says nothing.
		{"clean eval", "eval 'echo ok'\n", "ok\n"},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, st := runHeredocScript(t, dir, c.src)
			out = strings.ReplaceAll(out, dir, "<d>")
			if out != c.want || st != 0 {
				t.Errorf("out %q status %d, want %q at 0", out, st, c.want)
			}
		})
	}
}
