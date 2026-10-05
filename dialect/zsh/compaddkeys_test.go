// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"strings"
	"testing"
)

// `compadd -k` offers the keys of an association, and the produced ones
// count (#6143): `commands`, `functions` and `aliases` are tables this shell
// answers when read rather than stores, and `_command_names` is `compadd -k
// commands` underneath.
//
// Measured 2026-10-05 against zsh 5.9.2 from a completion widget after `gi`:
// `compadd -k commands` offers the 24 names on PATH beginning `gi`, where
// this offered none. Functions and aliases stand in for commands here so the
// row does not depend on the machine's PATH; a stored association is the
// control that worked before.
func TestCompaddKeysReadAProducedAssociation(t *testing.T) {
	for _, c := range []struct{ name, setup, table, want string }{
		{"functions", "gaone() { :; }; gatwo() { :; }\n", "functions", "gaone gatwo"},
		{"aliases", "alias gaone=x gatwo=y\n", "aliases", "gaone gatwo"},
		{"a stored table", "typeset -gA tbl; tbl=(gaone 1 gatwo 2 zz 3)\n", "tbl", "gaone gatwo"},
	} {
		t.Run(c.name, func(t *testing.T) {
			got := completionFor(t, c.setup+widgetOf("compadd -k "+c.table), "x ga")
			if strings.Join(got, " ") != c.want {
				t.Errorf("compadd -k %s offered %q, want %q", c.table, got, c.want)
			}
		})
	}
}
