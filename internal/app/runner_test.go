package app_test

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log"
	"strings"
	"testing"
	"time"

	"vote_message_deleter/internal/app"
	"vote_message_deleter/internal/moderation"
	"vote_message_deleter/internal/telegram"
)

func TestRunnerProcessesUpdateAndAdvancesOffset(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	source := &scriptedUpdateSource{
		batches: [][]telegram.Update{
			{
				reactionUpdate(100, -100123, 77, nil, []telegram.ReactionType{emojiReaction("👎")}),
			},
		},
		cancel: cancel,
	}

	realDeleter := &recordingDeleter{}
	handler := app.NewHandler(
		app.NewDeletionGate(realDeleter, true),
		moderation.Thresholds{Positive: 3, Negative: 1},
	)
	var logs bytes.Buffer
	runner, err := app.NewRunner(app.RunnerConfig{
		UpdateSource:       source,
		Handler:            handler,
		PollTimeoutSeconds: 30,
		RetryDelay:         time.Millisecond,
		DryRun:             true,
		Logger:             log.New(&logs, "", 0),
	})
	if err != nil {
		t.Fatalf("NewRunner() error = %v", err)
	}

	if err := runner.Run(ctx); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if len(source.options) != 2 {
		t.Fatalf("GetUpdates() calls = %d, want 2", len(source.options))
	}
	if source.options[0].Offset != 0 {
		t.Errorf("first offset = %d, want 0", source.options[0].Offset)
	}
	if source.options[1].Offset != 101 {
		t.Errorf("second offset = %d, want 101", source.options[1].Offset)
	}
	if len(realDeleter.calls) != 0 {
		t.Errorf("real DeleteMessage() calls = %d, want 0", len(realDeleter.calls))
	}
	logOutput := logs.String()
	for _, expected := range []string{
		"WOULD_DELETE",
		"chat_id=-100123",
		"message_id=77",
		"likes=0",
		"dislikes=1",
	} {
		if !strings.Contains(logOutput, expected) {
			t.Errorf("log output = %q, want %q", logOutput, expected)
		}
	}
}

func TestRunnerSkipsDeletionWhenOriginalContentIsUnavailable(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	source := &scriptedUpdateSource{
		batches: [][]telegram.Update{
			{
				reactionUpdate(100, -100123, 77, nil, []telegram.ReactionType{emojiReaction("👎")}),
			},
		},
		cancel: cancel,
	}
	actions := &recordingModerationActions{}
	handler := app.NewHandler(
		actions,
		moderation.Thresholds{Positive: 3, Negative: 1},
		app.WithPreDeletionSpoiler(actions, false),
	)
	var logs bytes.Buffer
	runner, err := app.NewRunner(app.RunnerConfig{
		UpdateSource:       source,
		Handler:            handler,
		PollTimeoutSeconds: 30,
		RetryDelay:         time.Millisecond,
		Logger:             log.New(&logs, "", 0),
	})
	if err != nil {
		t.Fatalf("NewRunner() error = %v", err)
	}

	if err := runner.Run(ctx); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if len(source.options) != 2 || source.options[1].Offset != 101 {
		t.Errorf("poll options = %#v, want second offset 101", source.options)
	}
	if len(actions.events) != 0 {
		t.Errorf("actions = %v, want none", actions.events)
	}
	if !strings.Contains(logs.String(), "moderation decision: SKIP_DELETE") {
		t.Errorf("log output = %q, want SKIP_DELETE", logs.String())
	}
}

func TestRunnerStopsWhenContextIsCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	source := &blockingUpdateSource{started: make(chan struct{})}
	handler := app.NewHandler(&recordingDeleter{}, moderation.Thresholds{
		Positive: 3,
		Negative: 3,
	})
	runner, err := app.NewRunner(app.RunnerConfig{
		UpdateSource:       source,
		Handler:            handler,
		PollTimeoutSeconds: 30,
		Logger:             log.New(io.Discard, "", 0),
	})
	if err != nil {
		t.Fatalf("NewRunner() error = %v", err)
	}

	done := make(chan error, 1)
	go func() {
		done <- runner.Run(ctx)
	}()

	select {
	case <-source.started:
	case <-time.After(time.Second):
		t.Fatal("runner did not start polling")
	}
	cancel()

	select {
	case err := <-done:
		if err != nil {
			t.Errorf("Run() error = %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("runner did not stop after context cancellation")
	}
}

func TestRunnerRetriesTemporaryPollingError(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	source := &retryingUpdateSource{cancel: cancel}
	handler := app.NewHandler(&recordingDeleter{}, moderation.Thresholds{
		Positive: 3,
		Negative: 3,
	})
	var logs bytes.Buffer
	runner, err := app.NewRunner(app.RunnerConfig{
		UpdateSource:       source,
		Handler:            handler,
		PollTimeoutSeconds: 30,
		RetryDelay:         time.Millisecond,
		Logger:             log.New(&logs, "", 0),
	})
	if err != nil {
		t.Fatalf("NewRunner() error = %v", err)
	}

	if err := runner.Run(ctx); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if source.calls != 2 {
		t.Errorf("GetUpdates() calls = %d, want 2", source.calls)
	}
	if !strings.Contains(logs.String(), "Telegram polling error") {
		t.Errorf("log output = %q, want polling error", logs.String())
	}
}

type scriptedUpdateSource struct {
	batches [][]telegram.Update
	options []telegram.GetUpdatesOptions
	cancel  context.CancelFunc
}

func (source *scriptedUpdateSource) GetUpdates(ctx context.Context, options telegram.GetUpdatesOptions) ([]telegram.Update, error) {
	source.options = append(source.options, options)
	if len(source.batches) > 0 {
		batch := source.batches[0]
		source.batches = source.batches[1:]
		return batch, nil
	}

	source.cancel()
	return nil, ctx.Err()
}

type blockingUpdateSource struct {
	started chan struct{}
}

func (source *blockingUpdateSource) GetUpdates(ctx context.Context, _ telegram.GetUpdatesOptions) ([]telegram.Update, error) {
	close(source.started)
	<-ctx.Done()
	return nil, ctx.Err()
}

type retryingUpdateSource struct {
	calls  int
	cancel context.CancelFunc
}

func (source *retryingUpdateSource) GetUpdates(ctx context.Context, _ telegram.GetUpdatesOptions) ([]telegram.Update, error) {
	source.calls++
	if source.calls == 1 {
		return nil, errors.New("temporary network failure")
	}

	source.cancel()
	return nil, ctx.Err()
}
