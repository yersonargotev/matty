package packsyncworkflow

import (
	"context"
	"errors"
	"fmt"
	"time"
)

type FailureKind string

const (
	FailureTransient      FailureKind = "transient"
	FailureAccess         FailureKind = "access"
	FailureProvenance     FailureKind = "provenance"
	FailureIntegrity      FailureKind = "integrity"
	FailureClassification FailureKind = "classification"
	FailureValidation     FailureKind = "validation"
	FailureOwnership      FailureKind = "ownership"
	FailureDivergence     FailureKind = "divergence"
)

type Failure struct {
	Kind       FailureKind
	RetryAfter time.Duration
	Blocker    string
	Recovery   string
	Err        error
}

func (failure Failure) Error() string {
	if failure.Err == nil {
		return string(failure.Kind)
	}
	return failure.Err.Error()
}

func (failure Failure) Unwrap() error { return failure.Err }

type Sleeper interface {
	Sleep(context.Context, time.Duration) error
}

type realSleeper struct{}

func (realSleeper) Sleep(ctx context.Context, delay time.Duration) error {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

type RetryPolicy struct {
	MaxAttempts    int
	InitialBackoff time.Duration
	Sleeper        Sleeper
}

func (policy RetryPolicy) Do(ctx context.Context, operation func() error) error {
	if operation == nil || policy.MaxAttempts < 1 || policy.MaxAttempts > 3 || policy.InitialBackoff <= 0 {
		return errors.New("retry policy requires one through three attempts and positive backoff")
	}
	sleeper := policy.Sleeper
	if sleeper == nil {
		sleeper = realSleeper{}
	}
	for attempt := 1; attempt <= policy.MaxAttempts; attempt++ {
		if err := ctx.Err(); err != nil {
			return err
		}
		err := operation()
		if err == nil {
			return nil
		}
		var failure Failure
		if !errors.As(err, &failure) || failure.Kind != FailureTransient || attempt == policy.MaxAttempts {
			return err
		}
		delay := policy.InitialBackoff << (attempt - 1)
		if failure.RetryAfter > delay {
			delay = failure.RetryAfter
		}
		if err := sleeper.Sleep(ctx, delay); err != nil {
			return fmt.Errorf("wait before transient retry: %w", err)
		}
	}
	return errors.New("unreachable retry state")
}
