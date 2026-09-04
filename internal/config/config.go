// Package config loads and validates runtime configuration.
package config

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
)

const (
	telegramBotTokenEnv              = "TELEGRAM_BOT_TOKEN"
	moderationDislikeThresholdEnv    = "MODERATION_DISLIKE_THRESHOLD"
	moderationLikeThresholdEnv       = "MODERATION_LIKE_THRESHOLD"
	telegramPollTimeoutEnv           = "TELEGRAM_POLL_TIMEOUT"
	dryRunEnv                        = "DRY_RUN"
	defaultModerationThreshold       = 3
	defaultTelegramPollTimeoutSecond = 30
)

// Config contains validated runtime settings.
type Config struct {
	TelegramBotToken           string
	ModerationLikeThreshold    int
	ModerationDislikeThreshold int
	TelegramPollTimeoutSeconds int
	DryRun                     bool
}

// Load reads configuration from the process environment.
func Load() (Config, error) {
	token := strings.TrimSpace(os.Getenv(telegramBotTokenEnv))
	if token == "" {
		return Config{}, errors.New("TELEGRAM_BOT_TOKEN is required")
	}

	likeThreshold, err := positiveIntFromEnv(moderationLikeThresholdEnv, defaultModerationThreshold)
	if err != nil {
		return Config{}, err
	}
	dislikeThreshold, err := positiveIntFromEnv(moderationDislikeThresholdEnv, defaultModerationThreshold)
	if err != nil {
		return Config{}, err
	}
	pollTimeout, err := positiveIntFromEnv(telegramPollTimeoutEnv, defaultTelegramPollTimeoutSecond)
	if err != nil {
		return Config{}, err
	}
	dryRun, err := boolFromEnv(dryRunEnv, true)
	if err != nil {
		return Config{}, err
	}

	return Config{
		TelegramBotToken:           token,
		ModerationLikeThreshold:    likeThreshold,
		ModerationDislikeThreshold: dislikeThreshold,
		TelegramPollTimeoutSeconds: pollTimeout,
		DryRun:                     dryRun,
	}, nil
}

func positiveIntFromEnv(name string, defaultValue int) (int, error) {
	rawValue := strings.TrimSpace(os.Getenv(name))
	if rawValue == "" {
		return defaultValue, nil
	}

	value, err := strconv.Atoi(rawValue)
	if err != nil || value <= 0 {
		return 0, fmt.Errorf("%s must be a positive integer", name)
	}

	return value, nil
}

func boolFromEnv(name string, defaultValue bool) (bool, error) {
	rawValue := strings.TrimSpace(os.Getenv(name))
	if rawValue == "" {
		return defaultValue, nil
	}

	value, err := strconv.ParseBool(rawValue)
	if err != nil {
		return false, fmt.Errorf("%s must be a boolean", name)
	}

	return value, nil
}
