package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// throttleRetry is how apiClient.send backs off from API Gateway's throttle
// (HTTP 429), which a restore into prod hits constantly.
type throttleRetry struct {
	maxAttempts int
	baseDelay   time.Duration
	maxDelay    time.Duration
}

var defaultThrottleRetry = throttleRetry{maxAttempts: 10, baseDelay: time.Second, maxDelay: 30 * time.Second}

// send performs req, retrying a throttled response after Retry-After or an
// exponential backoff. The app's own 429 (the daily upload limit, sent as
// problem+json) is returned as is: waiting seconds won't lift it.
func (c *apiClient) send(req *http.Request) (*http.Response, error) {
	for attempt := 1; ; attempt++ {
		resp, err := c.http.Do(req)
		if err != nil || !isThrottle(resp) || attempt >= c.retry.maxAttempts {
			return resp, err
		}
		_ = resp.Body.Close()

		delay := retryAfter(resp)
		if delay == 0 {
			delay = min(c.retry.baseDelay<<(attempt-1), c.retry.maxDelay)
		}
		slog.Info("throttled, retrying", "method", req.Method, "path", req.URL.Path, "in", delay)
		if err := sleepCtx(req.Context(), delay); err != nil {
			return nil, err
		}

		if req.Body != nil {
			body, err := req.GetBody()
			if err != nil {
				return nil, fmt.Errorf("rewinding request body: %w", err)
			}
			req = req.Clone(req.Context())
			req.Body = body
		}
	}
}

func isThrottle(resp *http.Response) bool {
	return resp.StatusCode == http.StatusTooManyRequests &&
		!strings.HasPrefix(resp.Header.Get("Content-Type"), "application/problem+json")
}

// retryAfter reads a Retry-After header in seconds; 0 if absent or unparsable.
func retryAfter(resp *http.Response) time.Duration {
	secs, err := strconv.Atoi(resp.Header.Get("Retry-After"))
	if err != nil || secs < 0 {
		return 0
	}
	return time.Duration(secs) * time.Second
}

func sleepCtx(ctx context.Context, d time.Duration) error {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
