// Package telegram provides the Telegram Bot API adapter used by the
// application.
package telegram

// Update represents the subset of a Telegram update used by this project.
type Update struct {
	UpdateID             int64                        `json:"update_id"`
	Message              *Message                     `json:"message,omitempty"`
	MessageReaction      *MessageReactionUpdated      `json:"message_reaction,omitempty"`
	MessageReactionCount *MessageReactionCountUpdated `json:"message_reaction_count,omitempty"`
}

// Message represents the fields of an incoming message needed to preserve its
// textual content before moderation.
type Message struct {
	MessageID       int            `json:"message_id"`
	Chat            Chat           `json:"chat"`
	From            *User          `json:"from,omitempty"`
	SenderChat      *Chat          `json:"sender_chat,omitempty"`
	Date            int64          `json:"date"`
	ForwardOrigin   *MessageOrigin `json:"forward_origin,omitempty"`
	AuthorSignature string         `json:"author_signature,omitempty"`
	Text            string         `json:"text,omitempty"`
	Caption         string         `json:"caption,omitempty"`
}

// MessageOrigin represents the fields shared by Telegram's forwarded-message
// origin variants.
type MessageOrigin struct {
	Type            string `json:"type"`
	Date            int64  `json:"date"`
	SenderUser      *User  `json:"sender_user,omitempty"`
	SenderUserName  string `json:"sender_user_name,omitempty"`
	SenderChat      *Chat  `json:"sender_chat,omitempty"`
	Chat            *Chat  `json:"chat,omitempty"`
	MessageID       int    `json:"message_id,omitempty"`
	AuthorSignature string `json:"author_signature,omitempty"`
}

// MessageReactionUpdated describes a non-anonymous reaction change.
type MessageReactionUpdated struct {
	Chat        Chat           `json:"chat"`
	MessageID   int            `json:"message_id"`
	User        *User          `json:"user,omitempty"`
	ActorChat   *Chat          `json:"actor_chat,omitempty"`
	Date        int64          `json:"date"`
	OldReaction []ReactionType `json:"old_reaction"`
	NewReaction []ReactionType `json:"new_reaction"`
}

// MessageReactionCountUpdated describes current anonymous reaction counts.
type MessageReactionCountUpdated struct {
	Chat      Chat            `json:"chat"`
	MessageID int             `json:"message_id"`
	Date      int64           `json:"date"`
	Reactions []ReactionCount `json:"reactions"`
}

// ReactionType represents the fields shared by reaction type variants needed
// by this project. Unknown variants are preserved through Type and ignored by
// normalization.
type ReactionType struct {
	Type          string `json:"type"`
	Emoji         string `json:"emoji,omitempty"`
	CustomEmojiID string `json:"custom_emoji_id,omitempty"`
}

// ReactionCount associates a reaction type with its current total.
type ReactionCount struct {
	Type       ReactionType `json:"type"`
	TotalCount int          `json:"total_count"`
}

// MessageEntity describes formatting applied to a range of outgoing text.
type MessageEntity struct {
	Type   string `json:"type"`
	Offset int    `json:"offset"`
	Length int    `json:"length"`
	URL    string `json:"url,omitempty"`
}

// LinkPreviewOptions controls link previews for an outgoing message.
type LinkPreviewOptions struct {
	IsDisabled bool `json:"is_disabled,omitempty"`
}

// Chat represents the Telegram chat fields needed by reaction updates.
type Chat struct {
	ID       int64  `json:"id"`
	Type     string `json:"type,omitempty"`
	Title    string `json:"title,omitempty"`
	Username string `json:"username,omitempty"`
}

// User represents the Telegram user fields needed by reaction updates.
type User struct {
	ID        int64  `json:"id"`
	IsBot     bool   `json:"is_bot,omitempty"`
	FirstName string `json:"first_name,omitempty"`
	LastName  string `json:"last_name,omitempty"`
	Username  string `json:"username,omitempty"`
}
