// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package llm

import "context"

// concurrencyLimitedClient independently bounds requests to a gateway. Unlike
// review --concurrency, this limit covers all phases, including plan,
// main_task, comment re-location and background compression.
type concurrencyLimitedClient struct {
	inner LLMClient
	slots chan struct{}
}

// LimitConcurrency limits in-flight model requests. Non-positive values leave
// the client unchanged. Waiting honors cancellation and does not consume a slot.
func LimitConcurrency(inner LLMClient, max int) LLMClient {
	if max <= 0 {
		return inner
	}
	return &concurrencyLimitedClient{inner: inner, slots: make(chan struct{}, max)}
}

func (c *concurrencyLimitedClient) CompletionsWithCtx(ctx context.Context, req ChatRequest) (*ChatResponse, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case c.slots <- struct{}{}:
	}
	defer func() { <-c.slots }()
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return c.inner.CompletionsWithCtx(ctx, req)
}
