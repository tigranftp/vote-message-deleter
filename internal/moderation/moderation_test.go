package moderation_test

import (
	"errors"
	"testing"

	"vote_message_deleter/internal/moderation"
)

func TestDecide(t *testing.T) {
	tests := []struct {
		name       string
		message    moderation.Message
		thresholds moderation.Thresholds
		want       moderation.Decision
	}{
		{
			name:       "zero votes",
			message:    moderation.Message{ID: "message-1"},
			thresholds: moderation.Thresholds{Positive: 2, Negative: 2},
			want:       moderation.KeepMessage,
		},
		{
			name:       "one negative vote below threshold",
			message:    moderation.Message{ID: "message-1", NegativeVotes: 1},
			thresholds: moderation.Thresholds{Positive: 2, Negative: 2},
			want:       moderation.KeepMessage,
		},
		{
			name:       "negative votes exactly reach threshold",
			message:    moderation.Message{ID: "message-1", NegativeVotes: 2},
			thresholds: moderation.Thresholds{Positive: 2, Negative: 2},
			want:       moderation.DeleteMessage,
		},
		{
			name:       "negative votes exceed threshold",
			message:    moderation.Message{ID: "message-1", NegativeVotes: 3},
			thresholds: moderation.Thresholds{Positive: 2, Negative: 2},
			want:       moderation.DeleteMessage,
		},
		{
			name:       "positive votes do not cause deletion",
			message:    moderation.Message{ID: "message-1", PositiveVotes: 100},
			thresholds: moderation.Thresholds{Positive: 1, Negative: 3},
			want:       moderation.KeepMessage,
		},
		{
			name:       "negative threshold of one",
			message:    moderation.Message{ID: "message-1", NegativeVotes: 1},
			thresholds: moderation.Thresholds{Positive: 4, Negative: 1},
			want:       moderation.DeleteMessage,
		},
		{
			name:       "higher negative threshold is respected",
			message:    moderation.Message{ID: "message-1", NegativeVotes: 4},
			thresholds: moderation.Thresholds{Positive: 4, Negative: 5},
			want:       moderation.KeepMessage,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := moderation.Decide(test.message, test.thresholds)
			if err != nil {
				t.Fatalf("Decide() error = %v", err)
			}
			if got != test.want {
				t.Errorf("Decide() = %v, want %v", got, test.want)
			}
		})
	}
}

func TestDecideRejectsInvalidThresholds(t *testing.T) {
	tests := []struct {
		name       string
		thresholds moderation.Thresholds
	}{
		{
			name:       "zero positive threshold",
			thresholds: moderation.Thresholds{Positive: 0, Negative: 2},
		},
		{
			name:       "negative positive threshold",
			thresholds: moderation.Thresholds{Positive: -1, Negative: 2},
		},
		{
			name:       "zero negative threshold",
			thresholds: moderation.Thresholds{Positive: 2, Negative: 0},
		},
		{
			name:       "negative negative threshold",
			thresholds: moderation.Thresholds{Positive: 2, Negative: -1},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			decision, err := moderation.Decide(
				moderation.Message{ID: "message-1"},
				test.thresholds,
			)

			if !errors.Is(err, moderation.ErrInvalidThreshold) {
				t.Fatalf("Decide() error = %v, want %v", err, moderation.ErrInvalidThreshold)
			}
			if decision != moderation.KeepMessage {
				t.Errorf("Decide() = %v, want %v", decision, moderation.KeepMessage)
			}
		})
	}
}
