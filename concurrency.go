package main

// Concurrency primitives the host uses to give every goroutine an exit.
//
// Deliberately NOT behind a build tag. They contain no platform code, and the
// control they provide — a goroutine that cannot outlive its host — is worth
// having covered by the default test suite that runs in CI, rather than by a
// tagged build only one machine compiles.

// orDone wraps a channel so a read is abandoned when done closes, instead of
// blocking forever on a producer that will never send again.
//
// The read site would otherwise have to spell out a select every time, and the
// version people write from memory omits the SECOND select — the one guarding
// the send on `out`. Without it this function leaks the very goroutine it
// exists to let exit, because it blocks forwarding a value nobody is reading.
func orDone[T any](done <-chan struct{}, c <-chan T) <-chan T {
	out := make(chan T)
	go func() {
		defer close(out)
		for {
			select {
			case <-done:
				return
			case v, ok := <-c:
				if !ok {
					return
				}
				select {
				case out <- v:
				case <-done:
					return
				}
			}
		}
	}()
	return out
}

// or returns a channel that closes as soon as ANY of its inputs closes.
//
// Used to merge "the caller cancelled" with "the host is going away" into the
// single done channel orDone takes. Recursive rather than iterative so it works
// for any number of inputs without a goroutine per input.
func or(channels ...<-chan struct{}) <-chan struct{} {
	switch len(channels) {
	case 0:
		return nil
	case 1:
		return channels[0]
	}

	merged := make(chan struct{})
	go func() {
		defer close(merged)
		switch len(channels) {
		case 2:
			select {
			case <-channels[0]:
			case <-channels[1]:
			}
		default:
			select {
			case <-channels[0]:
			case <-channels[1]:
			case <-channels[2]:
			case <-or(append(channels[3:], merged)...):
			}
		}
	}()
	return merged
}
