// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package llm

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"
)

type countingLLMClient struct {
	active int64
	peak   int64
	entered chan struct{}
	release chan struct{}
}

func (c *countingLLMClient) CompletionsWithCtx(ctx context.Context, _ ChatRequest) (*ChatResponse, error) {
	n := atomic.AddInt64(&c.active, 1)
	for {
		max := atomic.LoadInt64(&c.peak)
		if n <= max || atomic.CompareAndSwapInt64(&c.peak, max, n) {
			break
		}
	}
	defer atomic.AddInt64(&c.active, -1)
	select {
	case c.entered <- struct{}{}:
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	select {
	case <-c.release:
		return &ChatResponse{}, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func TestLimitConcurrencyAndCancellation(t *testing.T) {
	underlying := &countingLLMClient{
		entered: make(chan struct{}, 3),
		release: make(chan struct{}, 3),
	}
	client := LimitConcurrency(underlying, 1)
	first := make(chan error, 1)
	go func() {
		_, err := client.CompletionsWithCtx(context.Background(), ChatRequest{})
		first <- err
	}()
	select {
	case <-underlying.entered:
	case <-time.After(time.Second):
		t.Fatal("first request did not enter")
	}
	ctx, cancel := context.WithCancel(context.Background())
	second := make(chan error, 1)
	go func() {
		_, err := client.CompletionsWithCtx(ctx, ChatRequest{})
		second <- err
	}()
	cancel()
	select {
	case err := <-second:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("waiting request error = %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("cancelled request blocked waiting for LLM semaphore")
	}
	if n := atomic.LoadInt64(&underlying.peak); n != 1 {
		t.Fatalf("peak concurrent calls = %d, want 1", n)
	}
	underlying.release <- struct{}{}
	select {
	case err := <-first:
		if err != nil {
			t.Fatalf("first request failed: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("first request did not complete")
	}
}
