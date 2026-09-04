package app

import (
	"context"
	"errors"
	"fmt"
	"log"
	"time"

	"vote_message_deleter/internal/moderation"
	"vote_message_deleter/internal/telegram"
)

const defaultRetryDelay = 2 * time.Second

var (
	// ErrUpdateSourceRequired indicates that a runner has no Telegram update
	// source.
	ErrUpdateSourceRequired = errors.New("update source is required")
	// ErrHandlerRequired indicates that a runner has no update handler.
	ErrHandlerRequired = errors.New("update handler is required")
	// ErrInvalidPollTimeout indicates that the Telegram long-poll timeout is not
	// positive.
	ErrInvalidPollTimeout = errors.New("poll timeout must be positive")
)

// UpdateSource receives one batch of Telegram updates.
type UpdateSource interface {
	GetUpdates(ctx context.Context, options telegram.GetUpdatesOptions) ([]telegram.Update, error)
}

// RunnerConfig configures the long-polling application loop.
type RunnerConfig struct {
	UpdateSource       UpdateSource
	Handler            *Handler
	PollTimeoutSeconds int
	RetryDelay         time.Duration
	DryRun             bool
	Logger             *log.Logger
}

// Runner receives Telegram updates and processes them one at a time.
type Runner struct {
	updateSource       UpdateSource
	handler            *Handler
	pollTimeoutSeconds int
	retryDelay         time.Duration
	dryRun             bool
	logger             *log.Logger
}

// NewRunner constructs a Telegram long-polling runner.
func NewRunner(config RunnerConfig) (*Runner, error) {
	if config.UpdateSource == nil {
		return nil, ErrUpdateSourceRequired
	}
	if config.Handler == nil {
		return nil, ErrHandlerRequired
	}
	if config.PollTimeoutSeconds <= 0 {
		return nil, ErrInvalidPollTimeout
	}

	retryDelay := config.RetryDelay
	if retryDelay == 0 {
		retryDelay = defaultRetryDelay
	}
	if retryDelay < 0 {
		return nil, errors.New("retry delay must not be negative")
	}

	logger := config.Logger
	if logger == nil {
		logger = log.Default()
	}

	return &Runner{
		updateSource:       config.UpdateSource,
		handler:            config.Handler,
		pollTimeoutSeconds: config.PollTimeoutSeconds,
		retryDelay:         retryDelay,
		dryRun:             config.DryRun,
		logger:             logger,
	}, nil
}

// Run polls and processes updates until the context is canceled. Telegram and
// processing errors are logged and retried without advancing past the failed
// update.
func (runner *Runner) Run(ctx context.Context) error {
	var offset int64

	for {
		if ctx.Err() != nil {
			return nil
		}

		updates, err := runner.updateSource.GetUpdates(ctx, telegram.GetUpdatesOptions{
			Offset:         offset,
			TimeoutSeconds: runner.pollTimeoutSeconds,
		})
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			runner.logger.Printf("Telegram polling error: %v; retrying in %s", err, runner.retryDelay)
			if !waitForRetry(ctx, runner.retryDelay) {
				return nil
			}
			continue
		}

		processingFailed := false
		for index, update := range updates {
			if err := runner.processUpdate(ctx, update); err != nil {
				if ctx.Err() != nil {
					return nil
				}
				runner.logger.Printf("update processing error update_id=%d: %v; retrying in %s", update.UpdateID, err, runner.retryDelay)
				processingFailed = true
				break
			}

			offset = telegram.NextOffset(offset, updates[index:index+1])
		}

		if processingFailed && !waitForRetry(ctx, runner.retryDelay) {
			return nil
		}
	}
}

func (runner *Runner) processUpdate(ctx context.Context, update telegram.Update) error {
	result, err := runner.handler.HandleUpdate(ctx, update)
	if !result.Relevant {
		return err
	}

	decisionLabel := runner.decisionLabel(result.Decision)
	if err != nil {
		if errors.Is(err, ErrOriginalMessageUnavailable) {
			runner.logger.Printf(
				"moderation decision: SKIP_DELETE chat_id=%d message_id=%d likes=%d dislikes=%d: %v",
				result.ChatID,
				result.MessageID,
				result.Likes,
				result.Dislikes,
				err,
			)
			return nil
		}
		return fmt.Errorf(
			"moderation decision: %s chat_id=%d message_id=%d likes=%d dislikes=%d: %w",
			decisionLabel,
			result.ChatID,
			result.MessageID,
			result.Likes,
			result.Dislikes,
			err,
		)
	}

	runner.logger.Printf(
		"moderation decision: %s chat_id=%d message_id=%d likes=%d dislikes=%d",
		decisionLabel,
		result.ChatID,
		result.MessageID,
		result.Likes,
		result.Dislikes,
	)

	return nil
}

func (runner *Runner) decisionLabel(decision moderation.Decision) string {
	switch decision {
	case moderation.KeepMessage:
		return "KEEP"
	case moderation.DeleteMessage:
		if runner.dryRun {
			return "WOULD_DELETE"
		}
		return "DELETE"
	default:
		return fmt.Sprintf("UNKNOWN_%d", decision)
	}
}

func waitForRetry(ctx context.Context, delay time.Duration) bool {
	timer := time.NewTimer(delay)
	defer timer.Stop()

	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}
