package main

import (
	"context"
	"errors"
	"time"

	"github.com/yersonargotev/matty/internal/packsync"
	"github.com/yersonargotev/matty/internal/packsync/githubsource"
	"github.com/yersonargotev/matty/internal/packsyncworkflow"
)

type retryingSource struct {
	source packsync.Source
	policy packsyncworkflow.RetryPolicy
}

func newRetryingSource(source packsync.Source) retryingSource {
	return retryingSource{source: source, policy: packsyncworkflow.RetryPolicy{MaxAttempts: 3, InitialBackoff: time.Second}}
}

func (source retryingSource) Releases(ctx context.Context, config packsync.SourceConfig) (releases []packsync.Release, err error) {
	err = source.policy.Do(ctx, func() error {
		releases, err = source.source.Releases(ctx, config)
		return classifySourceFailure(err)
	})
	return releases, err
}

func (source retryingSource) ResolveRelease(ctx context.Context, config packsync.SourceConfig, release packsync.Release) (candidate packsync.Candidate, err error) {
	err = source.policy.Do(ctx, func() error {
		candidate, err = source.source.ResolveRelease(ctx, config, release)
		return classifySourceFailure(err)
	})
	return candidate, err
}

func (source retryingSource) ResolveCommit(ctx context.Context, config packsync.SourceConfig, commit string) (candidate packsync.Candidate, err error) {
	err = source.policy.Do(ctx, func() error {
		candidate, err = source.source.ResolveCommit(ctx, config, commit)
		return classifySourceFailure(err)
	})
	return candidate, err
}

func (source retryingSource) WithSnapshot(ctx context.Context, candidate packsync.Candidate, temporaryRoot string, visit func(string) error) (err error) {
	return source.policy.Do(ctx, func() error {
		err = source.source.WithSnapshot(ctx, candidate, temporaryRoot, visit)
		return classifySourceFailure(err)
	})
}

func classifySourceFailure(err error) error {
	if err == nil {
		return nil
	}
	var response githubsource.HTTPError
	if errors.As(err, &response) {
		return packsyncworkflow.ClassifyHTTPFailure(response.StatusCode, response.RetryAfter, err)
	}
	return packsyncworkflow.ClassifyNetworkFailure(err)
}
