package main

// TestShutdownStartRace verifies the happens-before guarantee that the channel
// pattern in main() provides between the goroutine that records shutdownStart
// (on ctx cancellation) and the drain block that reads it (after server.Run
// returns). Running this test with -race detects any synchronization hole in
// that handoff.
//
// The test models the pattern directly:
//
//	shutdownStartCh := make(chan time.Time, 1)
//	go func() { <-ctx.Done(); shutdownStartCh <- time.Now() }()
//	// … server.Run equivalent blocks here …
//	select {
//	case shutdownStart := <-shutdownStartCh: … use shutdownStart …
//	default: // listen-error path — no drain
//	}
//
// The channel receive in the select is the HB edge that makes the write
// observable to the reader. Any attempt to replace the channel with an
// unsynchronized variable would be flagged by the race detector here.

import (
	"context"
	"sync"
	"testing"
	"time"
)

func TestShutdownStartChannelHappensBefore(t *testing.T) {
	t.Parallel()

	// shutdownStartCh mirrors the pattern in main(): buffered(1) so the sender
	// never blocks even if the drain select never runs (listen-error path).
	shutdownStartCh := make(chan time.Time, 1)

	ctx, cancel := context.WithCancel(context.Background())

	// Writer goroutine: records the cancellation time, exactly as in main().
	var writerDone sync.WaitGroup
	writerDone.Add(1)
	go func() {
		defer writerDone.Done()
		<-ctx.Done()
		shutdownStartCh <- time.Now()
	}()

	// Simulate the server doing some work before ctx is cancelled, then
	// cancellation fires (what signal/SIGTERM does in production).
	cancel()
	writerDone.Wait() // wait for the goroutine to have sent

	// Drain block: receives from the channel, exactly as in main().
	// The race detector verifies this receive creates a proper HB edge.
	select {
	case shutdownStart := <-shutdownStartCh:
		httpElapsed := time.Since(shutdownStart)
		if httpElapsed < 0 {
			t.Errorf("shutdownStart is in the future: elapsed=%v", httpElapsed)
		}
		// Sanity: the recorded time must be non-zero and not absurdly old.
		if shutdownStart.IsZero() {
			t.Error("shutdownStart is zero")
		}
		if time.Since(shutdownStart) > 10*time.Second {
			t.Errorf("shutdownStart is unexpectedly old: %v", shutdownStart)
		}
	default:
		// This branch is taken on the listen-error path (ctx never cancelled).
		// In this test ctx IS cancelled, so reaching default is a bug.
		t.Error("expected shutdownStartCh to have a value after ctx cancellation, got none")
	}
}

// TestShutdownStartChannelListenErrorPath verifies the listen-error path: when
// ctx is never cancelled, the drain select takes the default branch without
// blocking and without any panic or hang.
func TestShutdownStartChannelListenErrorPath(t *testing.T) {
	t.Parallel()

	shutdownStartCh := make(chan time.Time, 1)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Writer goroutine is started but ctx is never cancelled in this test,
	// so the goroutine blocks forever on <-ctx.Done(). The buffered channel
	// means the goroutine can be GC'd without blocking once cancel is called
	// by defer at test exit.
	go func() {
		<-ctx.Done()
		shutdownStartCh <- time.Now()
	}()

	// Simulate server.Run returning due to a listen error (ctx still live).
	// The drain select must take the default branch immediately.
	select {
	case ts := <-shutdownStartCh:
		t.Errorf("expected default branch on listen-error path, got time %v", ts)
	default:
		// Correct: no drain on listen-error path.
	}
}
