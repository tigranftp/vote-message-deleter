// Package moderation contains Telegram-independent message moderation rules.
package moderation

import "errors"

// ErrInvalidThreshold indicates that at least one moderation threshold is not
// greater than zero.
var ErrInvalidThreshold = errors.New("moderation thresholds must be greater than zero")

// MessageID uniquely identifies a message within the moderation domain.
type MessageID string

// Message is the vote-count snapshot used to make a moderation decision.
type Message struct {
	ID            MessageID
	PositiveVotes int
	NegativeVotes int
}

// Thresholds configures the vote counts used by moderation policy.
type Thresholds struct {
	Positive int
	Negative int
}

// Decision describes the action to take for a message.
type Decision uint8

const (
	// KeepMessage leaves the message in place.
	KeepMessage Decision = iota
	// DeleteMessage removes the message.
	DeleteMessage
)

// Decide applies the moderation policy to a message vote snapshot.
func Decide(message Message, thresholds Thresholds) (Decision, error) {
	if thresholds.Positive <= 0 || thresholds.Negative <= 0 {
		return KeepMessage, ErrInvalidThreshold
	}

	if message.NegativeVotes >= thresholds.Negative {
		return DeleteMessage, nil
	}

	return KeepMessage, nil
}
