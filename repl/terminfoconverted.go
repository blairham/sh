// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package repl

// A description has two readings, and which one a script sees is state.
//
// TerminalCapabilities is the description as the file stores it. The shell
// being modeled hands a script that reading only some of the time; the rest
// of the time it hands over a second one in which a handful of capabilities
// have been rewritten into termcap's obsolete names. ConvertedCapabilities is
// that second reading. Which of the two is in force, and what moves a shell
// from one to the other, belongs to the dialect that names the parameter — see
// dialect/zsh/terminfo.go — because it is a fact about one shell's history,
// not about the database.
//
// # What the conversion does, measured
//
// Against zsh 5.9.2 under `env -i`, 2026-10-02, over every one of the 2,896
// descriptions in Homebrew's ncurses database, comparing `${(kv)terminfo}`
// read before anything else had touched the terminal with the same
// enumeration read after `: $terminfo[bel]` had. Every difference is one of
// the six rules below, and no description differs in any other key or value:
//
//   - `is3` becomes `OTi2` (497 descriptions): the key `is3` is gone and
//     `OTi2` holds its bytes.
//   - `rs2` becomes `OTrs` when neither `rs1` nor `rs3` is there (718). With
//     either of them present `rs2` stays where it is (522) — `xterm-256color`
//     is one.
//   - `OTbc` is `cub1`'s bytes when `cub1` is present and is not a backspace
//     (499). It is not read from `cub1` when that is a backspace, so a stored
//     `OTbc` beside a backspacing `cub1` keeps its own value (`z29`, `h29a`).
//   - `OTug` is `xmc`'s count when `smul` is also present (196).
//   - `OTbs` is whether `cub1` is a backspace, and the stored bit only where
//     there is no `cub1` (1,181 differ from the stored bit).
//   - `OTNL` is whether `nel` is a linefeed, and `no` where there is no `nel`
//     (167).
//
// And four rows compiled with `tic` for the cases no real description has:
//
//	is3=\EA, OTi2=\EB         both kept — the rename needs the slot empty
//	rs2=\EC, OTrs=\ED         both kept, the same way
//	cub1=\EE, OTbc=\EF        OTbc=\EE — this one is overwritten
//	xmc#3, smul=…, OTug#5     OTug=5 — and this one is not
//
// The last two disagree with each other, and that is the measurement rather
// than a typo: a stored `OTbc` loses to a `cub1` it is derived from, and a
// stored `OTug` wins over an `xmc` it is derived from. `OTbs` falls back to
// its stored bit and `OTNL` does not, which is the same kind of asymmetry and
// was measured the same way on 2026-09-11.

// ConvertedCapabilities is the second reading of a description: the one in
// which the obsolete termcap capabilities are filled in from the modern ones
// they describe. The argument is not modified.
//
// The order is the argument's, with a renamed capability in its original
// place and a filled-in one at the end of its section's run — which is
// immaterial to both of this shell's spellings of the parameter, since both
// build a map.
func ConvertedCapabilities(caps []TerminalCapability) []TerminalCapability {
	if len(caps) == 0 {
		return nil
	}
	standard := map[string]string{}
	for _, c := range caps {
		if !c.Extended {
			standard[c.Terminfo] = c.Value
		}
	}
	_, hasRS1 := standard["rs1"]
	_, hasRS3 := standard["rs3"]
	_, hasOTi2 := standard["OTi2"]
	_, hasOTrs := standard["OTrs"]
	cub1, hasCub1 := standard["cub1"]
	nel, hasNel := standard["nel"]
	xmc, hasXmc := standard["xmc"]
	_, hasSmul := standard["smul"]
	_, hasOTug := standard["OTug"]

	out := make([]TerminalCapability, 0, len(caps)+2)
	for _, c := range caps {
		if c.Extended {
			out = append(out, c)
			continue
		}
		switch c.Terminfo {
		case "is3":
			if !hasOTi2 {
				c.Terminfo, c.Termcap = "OTi2", "i2"
			}
		case "rs2":
			if !hasRS1 && !hasRS3 && !hasOTrs {
				c.Terminfo, c.Termcap = "OTrs", "rs"
			}
		case "OTbs":
			if hasCub1 {
				c.Value = terminfoBoolean(boolByte(cub1 == "\b"))
			}
		case "OTNL":
			c.Value = terminfoBoolean(boolByte(hasNel && nel == "\n"))
		case "OTbc":
			if hasCub1 && cub1 != "\b" {
				c.Value = cub1
			}
		}
		out = append(out, c)
	}
	if _, stored := standard["OTbc"]; !stored && hasCub1 && cub1 != "\b" {
		out = append(out, TerminalCapability{
			Terminfo: "OTbc", Termcap: "bc", Value: cub1, Kind: StringCapability,
		})
	}
	if !hasOTug && hasXmc && hasSmul {
		out = append(out, TerminalCapability{
			Terminfo: "OTug", Termcap: "ug", Value: xmc, Kind: NumericCapability,
		})
	}
	return out
}

// boolByte is the stored spelling of a decision this file has just made, so
// that one function turns a boolean into `yes` or `no`.
func boolByte(b bool) byte {
	if b {
		return 1
	}
	return 0
}
