package telegram

const (
	reactionKindEmoji = "emoji"
	likeEmoji         = "👍"
	dislikeEmoji      = "👎"
)

// ReactionDelta is the application-friendly vote change derived from one
// non-anonymous Telegram reaction update.
type ReactionDelta struct {
	ChatID       int64
	MessageID    int
	LikeDelta    int
	DislikeDelta int
	HasDislike   bool
}

// NormalizeReactionChange converts one user's old and new reaction sets to
// like and dislike deltas. Missing and unsupported reactions count as zero.
func NormalizeReactionChange(update MessageReactionUpdated) ReactionDelta {
	oldLikes, oldDislikes := countVotes(update.OldReaction)
	newLikes, newDislikes := countVotes(update.NewReaction)

	return ReactionDelta{
		ChatID:       update.Chat.ID,
		MessageID:    update.MessageID,
		LikeDelta:    newLikes - oldLikes,
		DislikeDelta: newDislikes - oldDislikes,
		HasDislike:   newDislikes > 0,
	}
}

func countVotes(reactions []ReactionType) (likes, dislikes int) {
	for _, reaction := range reactions {
		if reaction.Type != reactionKindEmoji {
			continue
		}

		switch reaction.Emoji {
		case likeEmoji:
			likes++
		case dislikeEmoji:
			dislikes++
		}
	}

	return likes, dislikes
}
