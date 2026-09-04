package config_test

import (
	"strings"
	"testing"

	"vote_message_deleter/internal/config"
)

func TestLoadValidConfiguration(t *testing.T) {
	t.Setenv("TELEGRAM_BOT_TOKEN", "test-token")
	t.Setenv("MODERATION_LIKE_THRESHOLD", "7")
	t.Setenv("MODERATION_DISLIKE_THRESHOLD", "5")
	t.Setenv("TELEGRAM_POLL_TIMEOUT", "45")
	t.Setenv("DRY_RUN", "false")

	got, err := config.Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if got.TelegramBotToken != "test-token" {
		t.Error("TelegramBotToken does not match configured token")
	}
	if got.ModerationLikeThreshold != 7 {
		t.Errorf("ModerationLikeThreshold = %d, want 7", got.ModerationLikeThreshold)
	}
	if got.ModerationDislikeThreshold != 5 {
		t.Errorf("ModerationDislikeThreshold = %d, want 5", got.ModerationDislikeThreshold)
	}
	if got.TelegramPollTimeoutSeconds != 45 {
		t.Errorf("TelegramPollTimeoutSeconds = %d, want 45", got.TelegramPollTimeoutSeconds)
	}
	if got.DryRun {
		t.Error("DryRun = true, want false")
	}
}

func TestLoadUsesDefaults(t *testing.T) {
	t.Setenv("TELEGRAM_BOT_TOKEN", "test-token")
	t.Setenv("MODERATION_LIKE_THRESHOLD", "")
	t.Setenv("MODERATION_DISLIKE_THRESHOLD", "")
	t.Setenv("TELEGRAM_POLL_TIMEOUT", "")
	t.Setenv("DRY_RUN", "")

	got, err := config.Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if got.ModerationLikeThreshold != 3 {
		t.Errorf("ModerationLikeThreshold = %d, want 3", got.ModerationLikeThreshold)
	}
	if got.ModerationDislikeThreshold != 3 {
		t.Errorf("ModerationDislikeThreshold = %d, want 3", got.ModerationDislikeThreshold)
	}
	if got.TelegramPollTimeoutSeconds != 30 {
		t.Errorf("TelegramPollTimeoutSeconds = %d, want 30", got.TelegramPollTimeoutSeconds)
	}
	if !got.DryRun {
		t.Error("DryRun = false, want true")
	}
}

func TestLoadRequiresTelegramBotToken(t *testing.T) {
	t.Setenv("TELEGRAM_BOT_TOKEN", "")

	_, err := config.Load()
	if err == nil {
		t.Fatal("Load() error = nil, want missing token error")
	}
	if !strings.Contains(err.Error(), "TELEGRAM_BOT_TOKEN") {
		t.Errorf("Load() error = %q, want variable name", err)
	}
}

func TestLoadRejectsInvalidModerationThresholds(t *testing.T) {
	tests := []struct {
		name     string
		variable string
		value    string
	}{
		{name: "zero dislike threshold", variable: "MODERATION_DISLIKE_THRESHOLD", value: "0"},
		{name: "negative dislike threshold", variable: "MODERATION_DISLIKE_THRESHOLD", value: "-1"},
		{name: "non-numeric dislike threshold", variable: "MODERATION_DISLIKE_THRESHOLD", value: "three"},
		{name: "zero like threshold", variable: "MODERATION_LIKE_THRESHOLD", value: "0"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Setenv("TELEGRAM_BOT_TOKEN", "test-token")
			t.Setenv("MODERATION_LIKE_THRESHOLD", "3")
			t.Setenv("MODERATION_DISLIKE_THRESHOLD", "3")
			t.Setenv(test.variable, test.value)

			_, err := config.Load()
			if err == nil {
				t.Fatal("Load() error = nil, want invalid threshold error")
			}
			if !strings.Contains(err.Error(), test.variable) {
				t.Errorf("Load() error = %q, want variable name %q", err, test.variable)
			}
		})
	}
}

func TestLoadRejectsInvalidPollTimeout(t *testing.T) {
	tests := []string{"0", "-1", "thirty"}

	for _, value := range tests {
		t.Run(value, func(t *testing.T) {
			t.Setenv("TELEGRAM_BOT_TOKEN", "test-token")
			t.Setenv("TELEGRAM_POLL_TIMEOUT", value)

			_, err := config.Load()
			if err == nil {
				t.Fatal("Load() error = nil, want invalid poll timeout error")
			}
			if !strings.Contains(err.Error(), "TELEGRAM_POLL_TIMEOUT") {
				t.Errorf("Load() error = %q, want TELEGRAM_POLL_TIMEOUT", err)
			}
		})
	}
}

func TestLoadRejectsInvalidDryRun(t *testing.T) {
	t.Setenv("TELEGRAM_BOT_TOKEN", "test-token")
	t.Setenv("DRY_RUN", "sometimes")

	_, err := config.Load()
	if err == nil {
		t.Fatal("Load() error = nil, want invalid DRY_RUN error")
	}
	if !strings.Contains(err.Error(), "DRY_RUN") {
		t.Errorf("Load() error = %q, want DRY_RUN", err)
	}
}
