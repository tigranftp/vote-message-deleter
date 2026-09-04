package telegram_test

import (
	"testing"

	"vote_message_deleter/internal/telegram"
)

func TestNormalizeReactionChange(t *testing.T) {
	tests := []struct {
		name           string
		oldReaction    []telegram.ReactionType
		newReaction    []telegram.ReactionType
		wantLikes      int
		wantDislikes   int
		wantHasDislike bool
	}{
		{
			name:        "no reaction to like",
			newReaction: []telegram.ReactionType{emojiReaction("👍")},
			wantLikes:   1,
		},
		{
			name:        "like to no reaction",
			oldReaction: []telegram.ReactionType{emojiReaction("👍")},
			wantLikes:   -1,
		},
		{
			name:           "no reaction to dislike",
			newReaction:    []telegram.ReactionType{emojiReaction("👎")},
			wantDislikes:   1,
			wantHasDislike: true,
		},
		{
			name:         "dislike to no reaction",
			oldReaction:  []telegram.ReactionType{emojiReaction("👎")},
			wantDislikes: -1,
		},
		{
			name:           "like to dislike",
			oldReaction:    []telegram.ReactionType{emojiReaction("👍")},
			newReaction:    []telegram.ReactionType{emojiReaction("👎")},
			wantLikes:      -1,
			wantDislikes:   1,
			wantHasDislike: true,
		},
		{
			name:         "dislike to like",
			oldReaction:  []telegram.ReactionType{emojiReaction("👎")},
			newReaction:  []telegram.ReactionType{emojiReaction("👍")},
			wantLikes:    1,
			wantDislikes: -1,
		},
		{
			name:        "unrelated emoji",
			newReaction: []telegram.ReactionType{emojiReaction("🔥")},
		},
		{
			name: "unknown reaction types",
			oldReaction: []telegram.ReactionType{
				{Type: "custom_emoji", CustomEmojiID: "custom-1"},
			},
			newReaction: []telegram.ReactionType{
				{Type: "future_reaction", Emoji: "👍"},
				{Type: "paid"},
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := telegram.NormalizeReactionChange(telegram.MessageReactionUpdated{
				Chat:        telegram.Chat{ID: -1001},
				MessageID:   77,
				OldReaction: test.oldReaction,
				NewReaction: test.newReaction,
			})

			if got.ChatID != -1001 {
				t.Errorf("ChatID = %d, want -1001", got.ChatID)
			}
			if got.MessageID != 77 {
				t.Errorf("MessageID = %d, want 77", got.MessageID)
			}
			if got.LikeDelta != test.wantLikes {
				t.Errorf("LikeDelta = %d, want %d", got.LikeDelta, test.wantLikes)
			}
			if got.DislikeDelta != test.wantDislikes {
				t.Errorf("DislikeDelta = %d, want %d", got.DislikeDelta, test.wantDislikes)
			}
			if got.HasDislike != test.wantHasDislike {
				t.Errorf("HasDislike = %t, want %t", got.HasDislike, test.wantHasDislike)
			}
		})
	}
}

func emojiReaction(emoji string) telegram.ReactionType {
	return telegram.ReactionType{Type: "emoji", Emoji: emoji}
}
