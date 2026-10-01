// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh

import (
	"bufio"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"
)

// measuredZone is one of the three zones the zone grid was measured under, as the
// fixed offsets they had at the two instants it used — so the test needs no
// zone database on the machine running it.
func measuredZone(tz string, epoch int64) *time.Location {
	switch tz {
	case "America/New_York":
		if epoch == 1181100005 {
			return time.FixedZone("EDT", -4*3600)
		}
		return time.FixedZone("EST", -5*3600)
	case "Asia/Kolkata":
		return time.FixedZone("IST", 5*3600+1800)
	}
	return time.UTC
}

// **Every measured row of the C library's half answers as the suite's image
// does** (#5160). The rows are a sample of a grid of every letter under every
// flag, width and modifier, at four instants and in three zones, taken from
// the Debian build of zsh 5.9.2 the suite grades against; see
// strftimeglibc.go for the rules they come to.
func TestStrftimeAnswersAsTheGNULibraryDoes(t *testing.T) {
	f, err := os.Open("testdata/strftime-glibc.txt")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	rows, bad := 0, 0
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := sc.Text()
		if strings.HasPrefix(line, "#") || line == "" {
			continue
		}
		fields := strings.Split(line, "\t")
		if len(fields) != 4 {
			t.Fatalf("malformed row %q", line)
		}
		epoch, err := strconv.ParseInt(fields[1], 10, 64)
		if err != nil {
			t.Fatalf("row %q: %v", line, err)
		}
		format, err1 := strconv.Unquote(fields[2])
		want, err2 := strconv.Unquote(fields[3])
		if err1 != nil || err2 != nil {
			t.Fatalf("row %q: %v %v", line, err1, err2)
		}
		rows++
		at := time.Unix(epoch, 0).In(measuredZone(fields[0], epoch))
		if got := zshStrftime(format, at); got != want {
			bad++
			if bad <= 20 {
				t.Errorf("%s %d %q: got %q, want %q", fields[0], epoch, format, got, want)
			}
		}
	}
	if err := sc.Err(); err != nil {
		t.Fatal(err)
	}
	// The count, so a table that came up empty is a failure rather than a
	// pass over nothing.
	if rows < 1800 {
		t.Fatalf("read %d rows, want the 1805 measured", rows)
	}
	if bad > 0 {
		t.Errorf("%d of %d rows differ", bad, rows)
	}
}

// **The result has 64 bytes of room for each byte of the format**, and one
// that does not fit is refused whole. The bisected edges, measured on the
// suite's image.
func TestStrftimeRefusesAResultPastItsRoom(t *testing.T) {
	at := time.Unix(0, 0).UTC()
	for _, c := range []struct {
		format string
		n      int
		ok     bool
	}{
		{"%319d", 319, true},
		{"%320d", 0, false},
		{"x%382d", 383, true},
		{"x%383d", 0, false},
		{"ab%445d", 447, true},
		{"ab%446d", 0, false},
		{"%300d%300d", 600, true},
		{"", 0, true},
	} {
		out, ok := zshStrftimeBounded(c.format, at)
		if ok != c.ok || len(out) != c.n {
			t.Errorf("%q: %d bytes, ok %v; want %d, %v", c.format, len(out), ok, c.n, c.ok)
		}
	}
}

// **The shell's own conversions are its own only in the spellings it reads**;
// any other spelling is the C library's, which writes it back as text. Rows
// measured on the suite's image at 1181100005 (2007-06-06 03:20:05 UTC).
func TestStrftimeOwnSpellings(t *testing.T) {
	at := time.Unix(1181100005, 0).UTC()
	for _, c := range []struct{ format, want string }{
		{"%.", "000"},
		{"%-.", "000"},
		{"%_.", "000"},
		{"%^.", "000"},
		{"%#.", "000"},
		{"%E.", "000"},
		{"%O.", "000"},
		{"%-E.", "000"},
		{"%0.", ""},
		{"%5.", "00000"},
		{"%12.", "000000000"},
		{"%-12.", "000000000"},
		{"%_1.", "%_1."},
		{"%1E.", "%1E."},
		{"%^1.", "%^1."},
		{"%_E.", "%_E."},
		{"%f", "6"},
		{"%-f", "6"},
		{"%_f", "%_f"},
		{"%5f", "  %5f"},
		{"%^f", "%^F"},
		{"%K", "3"},
		{"%-K", "3"},
		{"%L", "3"},
		{"%N", "000000000"},
		{"%-N", "000000000"},
		{"%1N", "%1N"},
		{"%-y", "07"},
		{"%-1y", "7"},
		{"%-g", "7"},
	} {
		if got := zshStrftime(c.format, at); got != c.want {
			t.Errorf("%q: got %q, want %q", c.format, got, c.want)
		}
	}
}
