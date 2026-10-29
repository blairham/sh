// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"context"
	"io"
	"testing"
	"time"
)

// **A byte the reader goroutine has already taken is never dropped for the
// deadline** (#7021). With the answer waiting and the deadline already passed,
// a plain select picks between them at random, so a reader that cannot be
// polled lost the byte about half the time. Two hundred rounds make the coin
// visible: the chance of a coin coming up the same way every time is 2^-200.
func TestAnAnswerInHandBeatsADeadlineThatHasPassed(t *testing.T) {
	passed := make(chan struct{})
	close(passed)
	for i := range 200 {
		resp := make(chan timedAnswer, 1)
		resp <- timedAnswer{b: 'a'}
		ev, answered, expired := awaitAnswer(resp, passed)
		if !answered || ev.b != 'a' || ev.eof {
			t.Fatalf("round %d: answered %v with %+v, want the byte that was in hand", i, answered, ev)
		}
		if !expired {
			t.Fatalf("round %d: the deadline had passed and was not reported", i)
		}
	}
}

// The other three states, so the rule above is not passing because the
// function always answers: no answer and a passed deadline is a timeout, an
// answer before the deadline is an answer with time left, and an end of input
// in hand is still the end of input.
func TestAwaitAnswerTheOtherStates(t *testing.T) {
	passed := make(chan struct{})
	close(passed)
	open := make(chan struct{})

	empty := make(chan timedAnswer, 1)
	if _, answered, expired := awaitAnswer(empty, passed); answered || !expired {
		t.Errorf("nothing in hand past the deadline: answered %v expired %v, want a timeout", answered, expired)
	}

	early := make(chan timedAnswer, 1)
	early <- timedAnswer{b: 'z'}
	if ev, answered, expired := awaitAnswer(early, open); !answered || expired || ev.b != 'z' {
		t.Errorf("an answer before the deadline: %+v answered %v expired %v", ev, answered, expired)
	}

	end := make(chan timedAnswer, 1)
	end <- timedAnswer{eof: true}
	if ev, answered, _ := awaitAnswer(end, passed); !answered || !ev.eof {
		t.Errorf("an end of input in hand past the deadline: %+v answered %v, want the end", ev, answered)
	}
}

// enteringReader hands over one byte, then blocks until released. It says on
// entered which read each call was, so a read nobody should have asked for is
// visible even while it is still waiting.
type enteringReader struct {
	n       int
	entered chan int
	release chan struct{}
}

func (e *enteringReader) Read(p []byte) (int, error) {
	e.n++
	e.entered <- e.n
	if e.n == 1 {
		p[0] = 'a'
		return 1, nil
	}
	<-e.release
	return 0, io.EOF
}

// **Once the deadline has passed, nothing more is read.** A byte taken as the
// deadline passed is the last one a whole read gets; asking the goroutine for
// another would park a read that nobody waits for, and the byte it found would
// be lost to the next reader as surely as the byte above was lost to this one.
//
// The deadline is past before the read starts, so a round only says something
// when the first byte was in hand as the deadline was seen. The loop runs until
// one is, and fails if none ever is: a test that never reached its case has
// not tested it.
func TestNothingMoreIsReadOnceTheDeadlineHasPassed(t *testing.T) {
	for range 1000 {
		rd := &enteringReader{entered: make(chan int, 4), release: make(chan struct{})}
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		next, stop := (&Runner{}).timedByteSource(ctx, rd, time.Hour, true)
		if _, ev := next(); ev != evByte {
			stop()
			close(rd.release)
			continue
		}
		<-rd.entered
		if _, ev := next(); ev != evTimeout {
			t.Fatalf("the read after a late byte answered %d, want a timeout", ev)
		}
		stop()
		close(rd.release)
		// A read asked for is entered at once — the request is a rendezvous —
		// so a second of nothing is a long wait for something already late.
		select {
		case n := <-rd.entered:
			t.Fatalf("read %d was asked for after the deadline had passed", n)
		case <-time.After(time.Second):
		}
		return
	}
	t.Fatal("no round in a thousand had the byte in hand as the deadline was seen")
}
