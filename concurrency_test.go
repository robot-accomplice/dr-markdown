package main

import (
	"runtime"
	"testing"
	"time"
)

// settle gives abandoned goroutines a moment to unwind before they are counted.
// Without it a passing leak check only proves the scheduler had not caught up.
func settle() {
	for i := 0; i < 50; i++ {
		runtime.Gosched()
		time.Sleep(time.Millisecond)
	}
}

func TestOrDoneForwardsValuesUntilTheSourceCloses(t *testing.T) {
	src := make(chan int, 3)
	src <- 1
	src <- 2
	close(src)

	var got []int
	for v := range orDone(make(chan struct{}), src) {
		got = append(got, v)
	}

	if len(got) != 2 || got[0] != 1 || got[1] != 2 {
		t.Errorf("expected the source's values in order, got %v", got)
	}
}

// The reason orDone exists: a read that would otherwise block forever on a
// producer that will never send again must be abandonable.
func TestOrDoneReleasesAReaderBlockedOnASilentProducer(t *testing.T) {
	done := make(chan struct{})
	silent := make(chan int) // nothing ever sends

	out := orDone(done, silent)
	close(done)

	select {
	case _, ok := <-out:
		if ok {
			t.Fatal("a silent producer cannot have delivered a value")
		}
	case <-time.After(time.Second):
		t.Fatal("closing done did not release the reader; orDone blocks forever")
	}
}

// The failure this guards is the one people write from memory: omitting the
// SECOND select, the one around `out <- v`. With a value in hand and nobody
// reading, the forwarding goroutine blocks on the send and leaks — orDone
// leaking the exact goroutine it exists to let exit.
func TestOrDoneDoesNotLeakWhenNobodyReadsTheForwardedValue(t *testing.T) {
	settle()
	before := runtime.NumGoroutine()

	for i := 0; i < 50; i++ {
		src := make(chan int, 1)
		src <- i
		done := make(chan struct{})

		// Take nothing from the returned channel. The forwarder is now holding a
		// value with no reader, which is precisely the blocked-on-send case.
		orDone(done, src)
		close(done)
	}

	settle()
	if after := runtime.NumGoroutine(); after > before+2 {
		t.Errorf("orDone leaked: %d goroutines before, %d after. "+
			"A forwarder blocked on send is not released by closing done.", before, after)
	}
}

func TestOrClosesAsSoonAsAnyInputCloses(t *testing.T) {
	for _, tc := range []struct {
		name  string
		which int
	}{
		{"first", 0},
		{"middle", 2},
		{"last", 4},
	} {
		t.Run(tc.name, func(t *testing.T) {
			chans := make([]chan struct{}, 5)
			inputs := make([]<-chan struct{}, 5)
			for i := range chans {
				chans[i] = make(chan struct{})
				inputs[i] = chans[i]
			}

			merged := or(inputs...)
			close(chans[tc.which])

			select {
			case <-merged:
			case <-time.After(time.Second):
				t.Fatalf("closing input %d did not close the merged channel", tc.which)
			}
		})
	}
}

func TestOrHandlesDegenerateInputs(t *testing.T) {
	if or() != nil {
		t.Error("or with no inputs should be nil, which blocks forever and is the correct identity")
	}

	only := make(chan struct{})
	if got := or(only); got != (<-chan struct{})(only) {
		t.Error("or with one input should return that input rather than wrapping it in a goroutine")
	}
}

// or spawns goroutines to watch its inputs; they must go away once it fires.
func TestOrDoesNotLeakAfterFiring(t *testing.T) {
	settle()
	before := runtime.NumGoroutine()

	for i := 0; i < 50; i++ {
		chans := make([]chan struct{}, 6)
		inputs := make([]<-chan struct{}, 6)
		for j := range chans {
			chans[j] = make(chan struct{})
			inputs[j] = chans[j]
		}
		<-func() <-chan struct{} { m := or(inputs...); close(chans[3]); return m }()
	}

	settle()
	if after := runtime.NumGoroutine(); after > before+2 {
		t.Errorf("or leaked: %d goroutines before, %d after", before, after)
	}
}
