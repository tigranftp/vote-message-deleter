package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"vote_message_deleter/internal/app"
	"vote_message_deleter/internal/config"
	"vote_message_deleter/internal/moderation"
	"vote_message_deleter/internal/telegram"
)

const httpTimeoutGracePeriod = 10 * time.Second

func main() {
	logger := log.New(os.Stdout, "", log.LstdFlags)
	if err := run(logger); err != nil {
		logger.Printf("bot stopped with error: %v", err)
		os.Exit(1)
	}
}

func run(logger *log.Logger) error {
	runtimeConfig, err := config.Load()
	if err != nil {
		return fmt.Errorf("load configuration: %w", err)
	}

	pollTimeout := time.Duration(runtimeConfig.TelegramPollTimeoutSeconds) * time.Second
	telegramClient, err := telegram.NewClient(telegram.ClientConfig{
		Token: runtimeConfig.TelegramBotToken,
		HTTPClient: &http.Client{
			Timeout: pollTimeout + httpTimeoutGracePeriod,
		},
	})
	if err != nil {
		return fmt.Errorf("create Telegram client: %w", err)
	}

	deleter := app.NewDeletionGate(telegramClient, runtimeConfig.DryRun)
	handler := app.NewHandler(deleter, moderation.Thresholds{
		Positive: runtimeConfig.ModerationLikeThreshold,
		Negative: runtimeConfig.ModerationDislikeThreshold,
	}, app.WithPreDeletionSpoiler(telegramClient, runtimeConfig.DryRun))
	runner, err := app.NewRunner(app.RunnerConfig{
		UpdateSource:       telegramClient,
		Handler:            handler,
		PollTimeoutSeconds: runtimeConfig.TelegramPollTimeoutSeconds,
		DryRun:             runtimeConfig.DryRun,
		Logger:             logger,
	})
	if err != nil {
		return fmt.Errorf("create bot runner: %w", err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	logger.Printf(
		"bot started dry_run=%t dislike_threshold=%d long_polling=true poll_timeout_seconds=%d",
		runtimeConfig.DryRun,
		runtimeConfig.ModerationDislikeThreshold,
		runtimeConfig.TelegramPollTimeoutSeconds,
	)
	if err := runner.Run(ctx); err != nil {
		return fmt.Errorf("run bot: %w", err)
	}

	logger.Print("bot stopped")
	return nil
}
