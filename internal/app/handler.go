// Package app coordinates Telegram updates with the moderation domain.
package app

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"vote_message_deleter/internal/moderation"
	"vote_message_deleter/internal/telegram"
)

// ErrMessageDeleterRequired indicates that deletion was requested without a
// configured Telegram deletion dependency.
var ErrMessageDeleterRequired = errors.New("message deleter is required")

var (
	// ErrMessageSenderRequired indicates that pre-deletion preservation was
	// enabled without a configured message sender.
	ErrMessageSenderRequired = errors.New("message sender is required")
	// ErrOriginalMessageUnavailable indicates that the bot did not observe the
	// original text or caption and therefore cannot safely preserve it.
	ErrOriginalMessageUnavailable = errors.New("original message content is unavailable")
)

// MessageDeleter deletes a message from its source chat.
type MessageDeleter interface {
	DeleteMessage(ctx context.Context, chatID int64, messageID int) error
}

// MessageSender sends a formatted plain-text message to a Telegram chat.
type MessageSender interface {
	SendMessage(ctx context.Context, options telegram.SendMessageOptions) error
}

// HandlerOption customizes update handling.
type HandlerOption func(*Handler)

// WithPreDeletionSpoiler enables publishing a protected spoiler copy before a
// live deletion. In dry-run mode neither the copy nor the deletion is sent.
func WithPreDeletionSpoiler(sender MessageSender, dryRun bool) HandlerOption {
	return func(handler *Handler) {
		handler.messageSender = sender
		handler.preserveBeforeDelete = true
		handler.dryRun = dryRun
	}
}

// Result describes the outcome of handling one Telegram update.
type Result struct {
	Relevant  bool
	ChatID    int64
	MessageID int
	Likes     int
	Dislikes  int
	Decision  moderation.Decision
}

// Handler accumulates non-anonymous reaction changes for the lifetime of the
// process and applies moderation policy to the resulting in-memory totals.
type Handler struct {
	deleter              MessageDeleter
	messageSender        MessageSender
	thresholds           moderation.Thresholds
	tracker              *voteTracker
	contents             *messageContentStore
	preserveBeforeDelete bool
	dryRun               bool
}

// NewHandler constructs a non-anonymous reaction update handler.
func NewHandler(deleter MessageDeleter, thresholds moderation.Thresholds, options ...HandlerOption) *Handler {
	handler := &Handler{
		deleter:    deleter,
		thresholds: thresholds,
		tracker:    newVoteTracker(),
		contents:   newMessageContentStore(),
	}
	for _, option := range options {
		option(handler)
	}

	return handler
}

// HandleUpdate remembers textual content from new messages and applies one
// non-anonymous message_reaction update. Other update types are ignored.
func (handler *Handler) HandleUpdate(ctx context.Context, update telegram.Update) (Result, error) {
	if update.Message != nil {
		handler.contents.remember(*update.Message)
		return Result{}, nil
	}
	if update.MessageReaction == nil {
		return Result{}, nil
	}

	delta := telegram.NormalizeReactionChange(*update.MessageReaction)
	state := handler.tracker.apply(
		update.UpdateID,
		delta,
		reactionActorFromUpdate(*update.MessageReaction),
	)
	result := Result{
		Relevant:  true,
		ChatID:    delta.ChatID,
		MessageID: delta.MessageID,
		Likes:     state.counts.likes,
		Dislikes:  state.counts.dislikes,
	}

	decision, err := moderation.Decide(moderation.Message{
		ID:            moderation.MessageID(fmt.Sprintf("%d:%d", delta.ChatID, delta.MessageID)),
		PositiveVotes: state.counts.likes,
		NegativeVotes: state.counts.dislikes,
	}, handler.thresholds)
	result.Decision = decision
	if err != nil {
		return result, fmt.Errorf("decide moderation for chat %d message %d: %w", delta.ChatID, delta.MessageID, err)
	}

	if decision != moderation.DeleteMessage {
		return result, nil
	}
	if handler.dryRun {
		return result, nil
	}
	if handler.deleter == nil {
		return result, ErrMessageDeleterRequired
	}
	if handler.preserveBeforeDelete {
		if err := handler.preserveMessage(ctx, result, state.dislikers); err != nil {
			return result, err
		}
	}
	if err := handler.deleter.DeleteMessage(ctx, delta.ChatID, delta.MessageID); err != nil {
		return result, fmt.Errorf("delete chat %d message %d: %w", delta.ChatID, delta.MessageID, err)
	}
	handler.contents.remove(messageKey{chatID: delta.ChatID, messageID: delta.MessageID})

	return result, nil
}

func (handler *Handler) preserveMessage(
	ctx context.Context,
	result Result,
	dislikers []messageReference,
) error {
	if handler.messageSender == nil {
		return ErrMessageSenderRequired
	}

	key := messageKey{chatID: result.ChatID, messageID: result.MessageID}
	message, ok := handler.contents.load(key)
	if !ok {
		return fmt.Errorf(
			"preserve chat %d message %d: %w",
			result.ChatID,
			result.MessageID,
			ErrOriginalMessageUnavailable,
		)
	}
	if message.preserved {
		return nil
	}

	options := deletionSpoilerMessage(
		result.ChatID,
		result.MessageID,
		message.content,
		message.originalDate,
		message.author,
		message.forwarded,
		result.Dislikes,
		dislikers,
	)
	if err := handler.messageSender.SendMessage(ctx, options); err != nil {
		return fmt.Errorf("preserve chat %d message %d: %w", result.ChatID, result.MessageID, err)
	}
	handler.contents.markPreserved(key)

	return nil
}

type messageKey struct {
	chatID    int64
	messageID int
}

type voteCounts struct {
	likes    int
	dislikes int
}

type reactionActorKey struct {
	isChat bool
	id     int64
}

type reactionActor struct {
	key       reactionActorKey
	reference messageReference
	available bool
}

type trackedDisliker struct {
	key       reactionActorKey
	reference messageReference
}

type trackedMessage struct {
	counts       voteCounts
	lastUpdateID int64
	dislikers    []trackedDisliker
}

type voteState struct {
	counts    voteCounts
	dislikers []messageReference
}

type voteTracker struct {
	mu       sync.Mutex
	messages map[messageKey]trackedMessage
}

func newVoteTracker() *voteTracker {
	return &voteTracker{messages: make(map[messageKey]trackedMessage)}
}

func (tracker *voteTracker) apply(
	updateID int64,
	delta telegram.ReactionDelta,
	actor reactionActor,
) voteState {
	tracker.mu.Lock()
	defer tracker.mu.Unlock()

	key := messageKey{chatID: delta.ChatID, messageID: delta.MessageID}
	message := tracker.messages[key]
	if updateID > 0 && updateID <= message.lastUpdateID {
		return message.snapshot()
	}

	message.counts.likes = max(0, message.counts.likes+delta.LikeDelta)
	message.counts.dislikes = max(0, message.counts.dislikes+delta.DislikeDelta)
	if actor.available {
		message.setDisliker(actor, delta.HasDislike)
	}
	message.lastUpdateID = updateID
	tracker.messages[key] = message

	return message.snapshot()
}

func (message *trackedMessage) setDisliker(actor reactionActor, hasDislike bool) {
	for index, disliker := range message.dislikers {
		if disliker.key != actor.key {
			continue
		}
		if hasDislike {
			message.dislikers[index].reference = actor.reference
			return
		}
		message.dislikers = append(message.dislikers[:index], message.dislikers[index+1:]...)
		return
	}

	if hasDislike {
		message.dislikers = append(message.dislikers, trackedDisliker{
			key:       actor.key,
			reference: actor.reference,
		})
	}
}

func (message trackedMessage) snapshot() voteState {
	dislikers := make([]messageReference, 0, len(message.dislikers))
	for _, disliker := range message.dislikers {
		dislikers = append(dislikers, disliker.reference)
	}

	return voteState{
		counts:    message.counts,
		dislikers: dislikers,
	}
}

func reactionActorFromUpdate(update telegram.MessageReactionUpdated) reactionActor {
	if update.User != nil {
		return reactionActor{
			key: reactionActorKey{
				id: update.User.ID,
			},
			reference: userReference(*update.User, true),
			available: true,
		}
	}
	if update.ActorChat != nil {
		return reactionActor{
			key: reactionActorKey{
				isChat: true,
				id:     update.ActorChat.ID,
			},
			reference: chatReference(*update.ActorChat, 0),
			available: true,
		}
	}

	return reactionActor{}
}
