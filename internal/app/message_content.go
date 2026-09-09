package app

import (
	"fmt"
	"net/url"
	"strings"
	"sync"
	"time"
	"unicode/utf16"

	"vote_message_deleter/internal/telegram"
)

const (
	messageContentTTL        = 24 * time.Hour
	maxStoredMessageContents = 10_000
	spoilerEntityType        = "spoiler"
	textLinkEntityType       = "text_link"
)

type messageReference struct {
	label       string
	url         string
	originalURL string
}

type storedMessageContent struct {
	content      string
	originalDate int64
	author       messageReference
	forwarded    *messageReference
	storedAt     time.Time
	preserved    bool
}

type messageContentStore struct {
	mu       sync.Mutex
	messages map[messageKey]storedMessageContent
}

func newMessageContentStore() *messageContentStore {
	return &messageContentStore{messages: make(map[messageKey]storedMessageContent)}
}

func (store *messageContentStore) remember(message telegram.Message) {
	content := message.Text
	if content == "" {
		content = message.Caption
	}
	if strings.TrimSpace(content) == "" {
		content = "Сообщение без текста или подписи"
	}

	store.mu.Lock()
	defer store.mu.Unlock()

	now := time.Now()
	store.pruneExpired(now)
	key := messageKey{chatID: message.Chat.ID, messageID: message.MessageID}
	if _, exists := store.messages[key]; !exists && len(store.messages) >= maxStoredMessageContents {
		store.removeOldest()
	}
	originalDate := message.Date
	forwarded := forwardedMessageReference(message.ForwardOrigin)
	if message.ForwardOrigin != nil && message.ForwardOrigin.Date > 0 {
		originalDate = message.ForwardOrigin.Date
	}
	store.messages[key] = storedMessageContent{
		content:      content,
		originalDate: originalDate,
		author:       messageAuthorReference(message),
		forwarded:    forwarded,
		storedAt:     now,
	}
}

func (store *messageContentStore) load(key messageKey) (storedMessageContent, bool) {
	store.mu.Lock()
	defer store.mu.Unlock()

	store.pruneExpired(time.Now())
	message, ok := store.messages[key]
	return message, ok
}

func (store *messageContentStore) markPreserved(key messageKey) {
	store.mu.Lock()
	defer store.mu.Unlock()

	message, ok := store.messages[key]
	if !ok {
		return
	}
	message.preserved = true
	store.messages[key] = message
}

func (store *messageContentStore) remove(key messageKey) {
	store.mu.Lock()
	defer store.mu.Unlock()

	delete(store.messages, key)
}

func (store *messageContentStore) pruneExpired(now time.Time) {
	for key, message := range store.messages {
		if now.Sub(message.storedAt) >= messageContentTTL {
			delete(store.messages, key)
		}
	}
}

func (store *messageContentStore) removeOldest() {
	var oldestKey messageKey
	var oldestTime time.Time
	for key, message := range store.messages {
		if oldestTime.IsZero() || message.storedAt.Before(oldestTime) {
			oldestKey = key
			oldestTime = message.storedAt
		}
	}
	if !oldestTime.IsZero() {
		delete(store.messages, oldestKey)
	}
}

func deletionSpoilerMessage(
	chatID int64,
	messageID int,
	content string,
	originalDate int64,
	author messageReference,
	forwarded *messageReference,
	dislikes int,
	dislikers []messageReference,
) telegram.SendMessageOptions {
	var text strings.Builder
	var entities []telegram.MessageEntity

	text.WriteString("🗑 Сообщение скрыто голосованием.\n\n")
	text.WriteString(fmt.Sprintf("ID оригинала: %d\n", messageID))
	writeReferenceLine(&text, &entities, "Автор: ", author)
	if forwarded != nil {
		writeReferenceLine(&text, &entities, "Переслано от: ", *forwarded)
		if forwarded.originalURL != "" {
			writeReferenceLine(&text, &entities, "Оригинал: ", messageReference{
				label: forwarded.originalURL,
				url:   forwarded.originalURL,
			})
		}
	}
	text.WriteString(fmt.Sprintf("Время оригинала: %s\n", formatOriginalMessageTime(originalDate)))
	text.WriteByte('\n')
	text.WriteString("Удалённое содержимое:\n")

	spoilerOffset := utf16Length(text.String())
	text.WriteString(content)
	text.WriteString("\n\n")
	entities = append(entities, telegram.MessageEntity{
		Type:   spoilerEntityType,
		Offset: spoilerOffset,
		Length: utf16Length(content),
	})

	text.WriteString(fmt.Sprintf("Отрицательных реакций: %d.\n", dislikes))
	writeReferenceListLine(&text, &entities, "Дизлайкнули: ", dislikers)

	return telegram.SendMessageOptions{
		ChatID: chatID,
		Text:   text.String(),
		LinkPreviewOptions: &telegram.LinkPreviewOptions{
			IsDisabled: true,
		},
		ProtectContent: true,
		Entities:       entities,
	}
}

func writeReferenceLine(
	text *strings.Builder,
	entities *[]telegram.MessageEntity,
	prefix string,
	reference messageReference,
) {
	text.WriteString(prefix)
	writeReference(text, entities, reference)
	text.WriteByte('\n')
}

func writeReferenceListLine(
	text *strings.Builder,
	entities *[]telegram.MessageEntity,
	prefix string,
	references []messageReference,
) {
	text.WriteString(prefix)
	if len(references) == 0 {
		text.WriteString("данные недоступны")
		text.WriteByte('\n')
		return
	}

	for index, reference := range references {
		if index > 0 {
			text.WriteString(", ")
		}
		writeReference(text, entities, reference)
	}
	text.WriteByte('\n')
}

func writeReference(
	text *strings.Builder,
	entities *[]telegram.MessageEntity,
	reference messageReference,
) {
	label := reference.label
	if label == "" {
		label = "неизвестен"
	}
	offset := utf16Length(text.String())
	text.WriteString(label)
	if reference.url != "" {
		*entities = append(*entities, telegram.MessageEntity{
			Type:   textLinkEntityType,
			Offset: offset,
			Length: utf16Length(label),
			URL:    reference.url,
		})
	}
}

func messageAuthorReference(message telegram.Message) messageReference {
	if message.SenderChat != nil {
		reference := chatReference(*message.SenderChat, 0)
		reference.label = withAuthorSignature(reference.label, message.AuthorSignature)
		return reference
	}
	if message.From != nil {
		return userReference(*message.From, true)
	}

	return messageReference{label: "неизвестен"}
}

func forwardedMessageReference(origin *telegram.MessageOrigin) *messageReference {
	if origin == nil {
		return nil
	}

	var reference messageReference
	switch origin.Type {
	case "user":
		if origin.SenderUser == nil {
			return &messageReference{label: "неизвестный пользователь"}
		}
		reference = userReference(*origin.SenderUser, false)
	case "hidden_user":
		reference.label = strings.TrimSpace(origin.SenderUserName)
		if reference.label == "" {
			reference.label = "скрытый пользователь"
		}
	case "chat":
		if origin.SenderChat == nil {
			return &messageReference{label: "неизвестный чат"}
		}
		reference = chatReference(*origin.SenderChat, 0)
		reference.label = withAuthorSignature(reference.label, origin.AuthorSignature)
	case "channel":
		if origin.Chat == nil {
			return &messageReference{label: "неизвестный канал"}
		}
		reference = chatReference(*origin.Chat, origin.MessageID)
		reference.label = withAuthorSignature(reference.label, origin.AuthorSignature)
	default:
		return &messageReference{label: "неизвестный источник"}
	}

	return &reference
}

func userReference(user telegram.User, allowIDLink bool) messageReference {
	name := strings.TrimSpace(strings.Join([]string{user.FirstName, user.LastName}, " "))
	username := strings.TrimPrefix(strings.TrimSpace(user.Username), "@")
	if username != "" {
		if name == "" {
			name = "@" + username
		} else {
			name += " (@" + username + ")"
		}
		return messageReference{
			label: name,
			url:   "https://t.me/" + url.PathEscape(username),
		}
	}
	if name == "" {
		name = "неизвестный пользователь"
	}

	reference := messageReference{label: name}
	if allowIDLink && user.ID > 0 {
		reference.url = fmt.Sprintf("tg://user?id=%d", user.ID)
	}
	return reference
}

func chatReference(chat telegram.Chat, messageID int) messageReference {
	label := strings.TrimSpace(chat.Title)
	username := strings.TrimPrefix(strings.TrimSpace(chat.Username), "@")
	if username != "" {
		if label == "" {
			label = "@" + username
		} else {
			label += " (@" + username + ")"
		}
		link := "https://t.me/" + url.PathEscape(username)
		if messageID > 0 {
			link += fmt.Sprintf("/%d", messageID)
		}
		reference := messageReference{label: label, url: link}
		if messageID > 0 {
			reference.originalURL = link
		}
		return reference
	}
	if label == "" {
		label = "неизвестный чат"
	}
	return messageReference{label: label}
}

func withAuthorSignature(label, signature string) string {
	signature = strings.TrimSpace(signature)
	if signature == "" {
		return label
	}
	return label + " — " + signature
}

func formatOriginalMessageTime(timestamp int64) string {
	if timestamp <= 0 {
		return "неизвестно"
	}

	return time.Unix(timestamp, 0).Local().Format("02.01.2006 15:04:05 -07:00")
}

func utf16Length(value string) int {
	return len(utf16.Encode([]rune(value)))
}
