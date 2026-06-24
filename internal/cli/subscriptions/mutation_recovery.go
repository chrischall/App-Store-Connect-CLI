package subscriptions

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/rudrankriyam/App-Store-Connect-CLI/internal/asc"
)

type reconciledMutationStatus string

const (
	reconciledMutationCreated    reconciledMutationStatus = "created"
	reconciledMutationSkipped    reconciledMutationStatus = "skipped"
	reconciledMutationReconciled reconciledMutationStatus = "reconciled"
)

func runReconciledMutation(
	ctx context.Context,
	readback func(context.Context) (bool, error),
	mutate func(context.Context) error,
) (reconciledMutationStatus, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}

	retryOpts := asc.ResolveRetryOptions()
	for retry := 0; ; retry++ {
		mutationErr := mutate(ctx)
		if mutationErr == nil {
			return reconciledMutationCreated, nil
		}
		if err := ctx.Err(); err != nil {
			return "", err
		}

		exists, readErr := readback(ctx)
		if readErr != nil {
			return "", fmt.Errorf("mutation and readback failed: %w", errors.Join(mutationErr, readErr))
		}
		if exists {
			return reconciledMutationReconciled, nil
		}
		if !reconciledMutationIsTransient(ctx, mutationErr) || retry >= retryOpts.MaxRetries {
			return "", mutationErr
		}

		if err := sleepWithContext(ctx, reconciledMutationRetryDelay(retryOpts, retry, mutationErr)); err != nil {
			return "", err
		}

		if err := ctx.Err(); err != nil {
			return "", err
		}
		exists, readErr = readback(ctx)
		if readErr != nil {
			return "", fmt.Errorf("mutation and pre-retry readback failed: %w", errors.Join(mutationErr, readErr))
		}
		if exists {
			return reconciledMutationReconciled, nil
		}
	}
}

func reconciledMutationIsTransient(parent context.Context, err error) bool {
	if asc.IsRetryable(err) {
		return true
	}
	return parent.Err() == nil && errors.Is(err, context.DeadlineExceeded)
}

func reconciledMutationRetryDelay(opts asc.RetryOptions, retry int, err error) time.Duration {
	if delay := asc.GetRetryAfter(err); delay > 0 {
		return delay
	}

	delay := opts.BaseDelay
	if retry > 0 && retry < 31 {
		delay *= time.Duration(1 << retry)
	}
	if opts.MaxDelay > 0 && (delay > opts.MaxDelay || delay <= 0) {
		return opts.MaxDelay
	}
	return delay
}
