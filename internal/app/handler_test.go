package app_test

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"
	"unicode/utf16"

	"vote_message_deleter/internal/app"
	"vote_message_deleter/internal/moderation"
	"vote_message_deleter/internal/telegram"
)

func TestHandlerDeletesAtNonAnonymousDislikeThreshold(t *testing.T) {
	deleter := &recordingDeleter{}
	handler := app.NewHandler(deleter, moderation.Thresholds{Positive: 3, Negative: 3})

	for index := 1; index <= 3; index++ {
		result, err := handler.HandleUpdate(
			context.Background(),
			reactionUpdate(int64(index), -100123, 77, nil, []telegram.ReactionType{emojiReaction("👎")}),
		)
		if err != nil {
			t.Fatalf("HandleUpdate() %d error = %v", index, err)
		}
		if result.Dislikes != index {
			t.Errorf("HandleUpdate() %d dislikes = %d, want %d", index, result.Dislikes, index)
		}

		wantDecision := moderation.KeepMessage
		if index == 3 {
			wantDecision = moderation.DeleteMessage
		}
		if result.Decision != wantDecision {
			t.Errorf("HandleUpdate() %d decision = %v, want %v", index, result.Decision, wantDecision)
		}
	}

	if len(deleter.calls) != 1 {
		t.Fatalf("DeleteMessage() calls = %d, want 1", len(deleter.calls))
	}
	if deleter.calls[0].chatID != -100123 || deleter.calls[0].messageID != 77 {
		t.Errorf("DeleteMessage() call = %#v, want chat -100123 message 77", deleter.calls[0])
	}
}

func TestHandlerAppliesReactionRemovalAndReplacement(t *testing.T) {
	handler := app.NewHandler(&recordingDeleter{}, moderation.Thresholds{Positive: 10, Negative: 10})
	tests := []struct {
		name         string
		oldReaction  []telegram.ReactionType
		newReaction  []telegram.ReactionType
		wantLikes    int
		wantDislikes int
	}{
		{
			name:        "add like",
			newReaction: []telegram.ReactionType{emojiReaction("👍")},
			wantLikes:   1,
		},
		{
			name:         "add dislike",
			newReaction:  []telegram.ReactionType{emojiReaction("👎")},
			wantLikes:    1,
			wantDislikes: 1,
		},
		{
			name:         "replace like with dislike",
			oldReaction:  []telegram.ReactionType{emojiReaction("👍")},
			newReaction:  []telegram.ReactionType{emojiReaction("👎")},
			wantDislikes: 2,
		},
		{
			name:         "remove dislike",
			oldReaction:  []telegram.ReactionType{emojiReaction("👎")},
			wantDislikes: 1,
		},
	}

	for index, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result, err := handler.HandleUpdate(
				context.Background(),
				reactionUpdate(int64(index+1), -100123, 77, test.oldReaction, test.newReaction),
			)
			if err != nil {
				t.Fatalf("HandleUpdate() error = %v", err)
			}
			if result.Likes != test.wantLikes {
				t.Errorf("Likes = %d, want %d", result.Likes, test.wantLikes)
			}
			if result.Dislikes != test.wantDislikes {
				t.Errorf("Dislikes = %d, want %d", result.Dislikes, test.wantDislikes)
			}
		})
	}
}

func TestHandlerDeduplicatesUpdateID(t *testing.T) {
	handler := app.NewHandler(&recordingDeleter{}, moderation.Thresholds{Positive: 3, Negative: 3})
	update := reactionUpdate(100, -100123, 77, nil, []telegram.ReactionType{emojiReaction("👎")})

	first, err := handler.HandleUpdate(context.Background(), update)
	if err != nil {
		t.Fatalf("first HandleUpdate() error = %v", err)
	}
	second, err := handler.HandleUpdate(context.Background(), update)
	if err != nil {
		t.Fatalf("second HandleUpdate() error = %v", err)
	}
	if first.Dislikes != 1 || second.Dislikes != 1 {
		t.Errorf("duplicate dislikes = (%d, %d), want (1, 1)", first.Dislikes, second.Dislikes)
	}
}

func TestHandlerKeepsMessagesIndependent(t *testing.T) {
	handler := app.NewHandler(&recordingDeleter{}, moderation.Thresholds{Positive: 3, Negative: 2})

	first, err := handler.HandleUpdate(
		context.Background(),
		reactionUpdate(1, -100123, 77, nil, []telegram.ReactionType{emojiReaction("👎")}),
	)
	if err != nil {
		t.Fatalf("first HandleUpdate() error = %v", err)
	}
	second, err := handler.HandleUpdate(
		context.Background(),
		reactionUpdate(2, -100123, 88, nil, []telegram.ReactionType{emojiReaction("👎")}),
	)
	if err != nil {
		t.Fatalf("second HandleUpdate() error = %v", err)
	}
	if first.Dislikes != 1 || second.Dislikes != 1 {
		t.Errorf("message dislikes = (%d, %d), want (1, 1)", first.Dislikes, second.Dislikes)
	}
	if first.Decision != moderation.KeepMessage || second.Decision != moderation.KeepMessage {
		t.Errorf("decisions = (%v, %v), want KeepMessage", first.Decision, second.Decision)
	}
}

func TestHandlerIgnoresAnonymousCountUpdates(t *testing.T) {
	handler := app.NewHandler(&recordingDeleter{}, moderation.Thresholds{Positive: 3, Negative: 3})

	result, err := handler.HandleUpdate(context.Background(), telegram.Update{
		UpdateID: 1,
		MessageReactionCount: &telegram.MessageReactionCountUpdated{
			Chat:      telegram.Chat{ID: -100123},
			MessageID: 77,
		},
	})
	if err != nil {
		t.Fatalf("HandleUpdate() error = %v", err)
	}
	if result.Relevant {
		t.Error("HandleUpdate() Relevant = true, want false")
	}
}

func TestDryRunPreventsDeleteMessage(t *testing.T) {
	realDeleter := &recordingDeleter{}
	handler := app.NewHandler(
		app.NewDeletionGate(realDeleter, true),
		moderation.Thresholds{Positive: 1, Negative: 1},
	)

	result, err := handler.HandleUpdate(
		context.Background(),
		reactionUpdate(1, -100123, 77, nil, []telegram.ReactionType{emojiReaction("👎")}),
	)
	if err != nil {
		t.Fatalf("HandleUpdate() error = %v", err)
	}
	if result.Decision != moderation.DeleteMessage {
		t.Errorf("decision = %v, want %v", result.Decision, moderation.DeleteMessage)
	}
	if len(realDeleter.calls) != 0 {
		t.Errorf("real DeleteMessage() calls = %d, want 0", len(realDeleter.calls))
	}
}

func TestLiveModeAllowsDeleteMessage(t *testing.T) {
	realDeleter := &recordingDeleter{}
	handler := app.NewHandler(
		app.NewDeletionGate(realDeleter, false),
		moderation.Thresholds{Positive: 1, Negative: 1},
	)

	result, err := handler.HandleUpdate(
		context.Background(),
		reactionUpdate(1, -100123, 77, nil, []telegram.ReactionType{emojiReaction("👎")}),
	)
	if err != nil {
		t.Fatalf("HandleUpdate() error = %v", err)
	}
	if result.Decision != moderation.DeleteMessage {
		t.Errorf("decision = %v, want %v", result.Decision, moderation.DeleteMessage)
	}
	if len(realDeleter.calls) != 1 {
		t.Fatalf("real DeleteMessage() calls = %d, want 1", len(realDeleter.calls))
	}
}

func TestHandlerPublishesSpoilerBeforeDeleting(t *testing.T) {
	const originalDate = int64(1_788_464_092)

	actions := &recordingModerationActions{}
	handler := app.NewHandler(
		actions,
		moderation.Thresholds{Positive: 1, Negative: 1},
		app.WithPreDeletionSpoiler(actions, false),
	)

	stored, err := handler.HandleUpdate(context.Background(), telegram.Update{
		UpdateID: 1,
		Message: &telegram.Message{
			MessageID: 77,
			Chat:      telegram.Chat{ID: -100123},
			Date:      originalDate + 3600,
			From: &telegram.User{
				ID:        101,
				FirstName: "Алиса",
				LastName:  "Иванова",
				Username:  "alice",
			},
			ForwardOrigin: &telegram.MessageOrigin{
				Type:      "channel",
				Date:      originalDate,
				MessageID: 321,
				Chat: &telegram.Chat{
					ID:       -100987,
					Title:    "Новости",
					Username: "public_news",
				},
			},
			Text: "текст, который нужно скрыть 🫣",
		},
	})
	if err != nil {
		t.Fatalf("store HandleUpdate() error = %v", err)
	}
	if stored.Relevant {
		t.Error("store HandleUpdate() Relevant = true, want false")
	}

	result, err := handler.HandleUpdate(
		context.Background(),
		userReactionUpdate(
			2,
			-100123,
			77,
			telegram.User{ID: 202, FirstName: "Борис", LastName: "Петров", Username: "boris"},
			nil,
			[]telegram.ReactionType{emojiReaction("👎")},
		),
	)
	if err != nil {
		t.Fatalf("reaction HandleUpdate() error = %v", err)
	}
	if result.Decision != moderation.DeleteMessage {
		t.Errorf("decision = %v, want %v", result.Decision, moderation.DeleteMessage)
	}
	if len(actions.events) != 2 || actions.events[0] != "send" || actions.events[1] != "delete" {
		t.Fatalf("action order = %v, want [send delete]", actions.events)
	}
	if len(actions.messages) != 1 {
		t.Fatalf("SendMessage() calls = %d, want 1", len(actions.messages))
	}

	message := actions.messages[0]
	if message.ChatID != -100123 {
		t.Errorf("SendMessage() chat ID = %d, want -100123", message.ChatID)
	}
	if !message.ProtectContent {
		t.Error("SendMessage() ProtectContent = false, want true")
	}
	if message.LinkPreviewOptions == nil || !message.LinkPreviewOptions.IsDisabled {
		t.Errorf("SendMessage() LinkPreviewOptions = %#v, want disabled", message.LinkPreviewOptions)
	}
	if !strings.Contains(message.Text, "Отрицательных реакций: 1") {
		t.Errorf("SendMessage() text = %q, want reaction count", message.Text)
	}
	if !strings.Contains(message.Text, "ID оригинала: 77") {
		t.Errorf("SendMessage() text = %q, want original message ID", message.Text)
	}
	if !strings.Contains(message.Text, "Дизлайкнули: Борис Петров (@boris)") {
		t.Errorf("SendMessage() text = %q, want disliker tag", message.Text)
	}
	wantTime := time.Unix(originalDate, 0).Local().Format("02.01.2006 15:04:05 -07:00")
	if !strings.Contains(message.Text, "Время оригинала: "+wantTime) {
		t.Errorf("SendMessage() text = %q, want original time %q", message.Text, wantTime)
	}
	spoiler, ok := findEntity(message.Entities, "spoiler", "")
	if !ok {
		t.Fatalf("SendMessage() entities = %#v, want spoiler", message.Entities)
	}
	if got := formattedEntityText(message.Text, spoiler); got != "текст, который нужно скрыть 🫣" {
		t.Errorf("spoiler text = %q", got)
	}
	authorLink, ok := findEntity(message.Entities, "text_link", "https://t.me/alice")
	if !ok {
		t.Fatalf("SendMessage() entities = %#v, want author link", message.Entities)
	}
	if got := formattedEntityText(message.Text, authorLink); got != "Алиса Иванова (@alice)" {
		t.Errorf("author link text = %q", got)
	}
	dislikerLink, ok := findEntity(message.Entities, "text_link", "https://t.me/boris")
	if !ok {
		t.Fatalf("SendMessage() entities = %#v, want disliker link", message.Entities)
	}
	if got := formattedEntityText(message.Text, dislikerLink); got != "Борис Петров (@boris)" {
		t.Errorf("disliker link text = %q", got)
	}
	forwardLink, ok := findEntity(message.Entities, "text_link", "https://t.me/public_news/321")
	if !ok {
		t.Fatalf("SendMessage() entities = %#v, want forwarded channel link", message.Entities)
	}
	if got := formattedEntityText(message.Text, forwardLink); got != "Новости (@public_news)" {
		t.Errorf("forward link text = %q", got)
	}
	if !strings.Contains(message.Text, "Оригинал: https://t.me/public_news/321") {
		t.Errorf("SendMessage() text = %q, want visible original link", message.Text)
	}
}

func TestHandlerDeletesMessagesWithoutText(t *testing.T) {
	for _, payload := range []string{
		`"photo":[{"file_id":"photo","width":100,"height":100}]`,
		`"sticker":{"file_id":"sticker","type":"regular","width":100,"height":100,"is_animated":false,"is_video":false}`,
		`"video":{"file_id":"video","width":100,"height":100,"duration":1}`,
		`"text":"  \n\t"`,
	} {
		t.Run(strings.SplitN(payload, ":", 2)[0], func(t *testing.T) {
			for _, dryRun := range []bool{false, true} {
				actions := &recordingModerationActions{}
				handler := app.NewHandler(actions, moderation.Thresholds{Positive: 3, Negative: 2}, app.WithPreDeletionSpoiler(actions, dryRun))
				var update telegram.Update
				if err := json.Unmarshal([]byte(`{"update_id":1,"message":{"message_id":77,"chat":{"id":-100123},`+payload+`}}`), &update); err != nil {
					t.Fatal(err)
				}
				if _, err := handler.HandleUpdate(context.Background(), update); err != nil {
					t.Fatal(err)
				}
				for id := int64(2); id <= 3; id++ {
					result, err := handler.HandleUpdate(context.Background(), reactionUpdate(id, -100123, 77, nil, []telegram.ReactionType{emojiReaction("👎")}))
					if err != nil {
						t.Fatal(err)
					}
					if id == 2 && (result.Decision != moderation.KeepMessage || len(actions.events) != 0) {
						t.Fatalf("below threshold: result = %#v, actions = %v", result, actions.events)
					}
					if id == 3 && result.Decision != moderation.DeleteMessage {
						t.Fatalf("threshold reached: result = %#v", result)
					}
				}
				if dryRun {
					if len(actions.events) != 0 {
						t.Fatalf("dry-run actions = %v, want none", actions.events)
					}
					continue
				}
				if strings.Join(actions.events, ",") != "send,delete" {
					t.Fatalf("actions = %v, want [send delete]", actions.events)
				}
				if got := actions.deleteCalls[0]; got != (deleteCall{chatID: -100123, messageID: 77}) {
					t.Fatalf("DeleteMessage() = %#v", got)
				}
				notice := actions.messages[0]
				spoiler, ok := findEntity(notice.Entities, "spoiler", "")
				if !ok || formattedEntityText(notice.Text, spoiler) != "Сообщение без текста или подписи" {
					t.Fatalf("notice = %#v, want nonempty placeholder spoiler", notice)
				}
			}
		})
	}
}

func TestHandlerListsOnlyCurrentDislikers(t *testing.T) {
	actions := &recordingModerationActions{}
	handler := app.NewHandler(
		actions,
		moderation.Thresholds{Positive: 1, Negative: 2},
		app.WithPreDeletionSpoiler(actions, false),
	)

	_, err := handler.HandleUpdate(context.Background(), telegram.Update{
		UpdateID: 1,
		Message: &telegram.Message{
			MessageID: 77,
			Chat:      telegram.Chat{ID: -100123},
			Text:      "сообщение для голосования",
		},
	})
	if err != nil {
		t.Fatalf("store HandleUpdate() error = %v", err)
	}

	leftVoter := telegram.User{ID: 201, FirstName: "Снявший", Username: "left_voter"}
	aliceUpdate := userReactionUpdate(
		4,
		-100123,
		77,
		telegram.User{ID: 202, FirstName: "Алиса", Username: "alice_vote"},
		nil,
		[]telegram.ReactionType{emojiReaction("👎")},
	)
	updates := []telegram.Update{
		userReactionUpdate(2, -100123, 77, leftVoter, nil, []telegram.ReactionType{emojiReaction("👎")}),
		userReactionUpdate(3, -100123, 77, leftVoter, []telegram.ReactionType{emojiReaction("👎")}, nil),
		aliceUpdate,
		aliceUpdate,
		userReactionUpdate(
			5,
			-100123,
			77,
			telegram.User{ID: 203, FirstName: "Евгений"},
			nil,
			[]telegram.ReactionType{emojiReaction("👎")},
		),
	}
	for index, update := range updates {
		if _, err := handler.HandleUpdate(context.Background(), update); err != nil {
			t.Fatalf("reaction HandleUpdate() %d error = %v", index, err)
		}
	}

	if len(actions.messages) != 1 {
		t.Fatalf("SendMessage() calls = %d, want 1", len(actions.messages))
	}
	message := actions.messages[0]
	if strings.Contains(message.Text, "Снявший") || strings.Contains(message.Text, "left_voter") {
		t.Errorf("SendMessage() text = %q, contains user who removed dislike", message.Text)
	}
	if !strings.Contains(message.Text, "Дизлайкнули: Алиса (@alice_vote), Евгений") {
		t.Errorf("SendMessage() text = %q, want current dislikers", message.Text)
	}
	if _, ok := findEntity(message.Entities, "text_link", "https://t.me/alice_vote"); !ok {
		t.Errorf("SendMessage() entities = %#v, want @alice_vote link", message.Entities)
	}
	if _, ok := findEntity(message.Entities, "text_link", "tg://user?id=203"); !ok {
		t.Errorf("SendMessage() entities = %#v, want ID link for user without username", message.Entities)
	}
}

func TestHandlerShowsHiddenForwardedAuthorWithoutLink(t *testing.T) {
	actions := &recordingModerationActions{}
	handler := app.NewHandler(
		actions,
		moderation.Thresholds{Positive: 1, Negative: 1},
		app.WithPreDeletionSpoiler(actions, false),
	)

	_, err := handler.HandleUpdate(context.Background(), telegram.Update{
		UpdateID: 1,
		Message: &telegram.Message{
			MessageID: 77,
			Chat:      telegram.Chat{ID: -100123},
			Date:      1_788_464_092,
			From: &telegram.User{
				ID:        101,
				FirstName: "Боб",
			},
			ForwardOrigin: &telegram.MessageOrigin{
				Type:           "hidden_user",
				Date:           1_788_400_000,
				SenderUserName: "Скрытый автор",
			},
			Text: "пересланный текст",
		},
	})
	if err != nil {
		t.Fatalf("store HandleUpdate() error = %v", err)
	}

	_, err = handler.HandleUpdate(
		context.Background(),
		reactionUpdate(2, -100123, 77, nil, []telegram.ReactionType{emojiReaction("👎")}),
	)
	if err != nil {
		t.Fatalf("reaction HandleUpdate() error = %v", err)
	}
	message := actions.messages[0]
	if !strings.Contains(message.Text, "Переслано от: Скрытый автор") {
		t.Errorf("SendMessage() text = %q, want hidden forwarded author", message.Text)
	}
	for _, entity := range message.Entities {
		if entity.Type == "text_link" && formattedEntityText(message.Text, entity) == "Скрытый автор" {
			t.Errorf("hidden forwarded author unexpectedly linked: %#v", entity)
		}
	}
}

func TestHandlerDoesNotDeleteWhenSpoilerPublishingFails(t *testing.T) {
	wantErr := errors.New("send failed")
	actions := &recordingModerationActions{sendErr: wantErr}
	handler := app.NewHandler(
		actions,
		moderation.Thresholds{Positive: 1, Negative: 1},
		app.WithPreDeletionSpoiler(actions, false),
	)

	_, err := handler.HandleUpdate(context.Background(), telegram.Update{
		UpdateID: 1,
		Message: &telegram.Message{
			MessageID: 77,
			Chat:      telegram.Chat{ID: -100123},
			Caption:   "подпись к медиа",
		},
	})
	if err != nil {
		t.Fatalf("store HandleUpdate() error = %v", err)
	}

	_, err = handler.HandleUpdate(
		context.Background(),
		reactionUpdate(2, -100123, 77, nil, []telegram.ReactionType{emojiReaction("👎")}),
	)
	if !errors.Is(err, wantErr) {
		t.Fatalf("reaction HandleUpdate() error = %v, want wrapped %v", err, wantErr)
	}
	if len(actions.deleteCalls) != 0 {
		t.Errorf("DeleteMessage() calls = %d, want 0", len(actions.deleteCalls))
	}
}

func TestHandlerDoesNotDeleteWithoutOriginalContent(t *testing.T) {
	actions := &recordingModerationActions{}
	handler := app.NewHandler(
		actions,
		moderation.Thresholds{Positive: 1, Negative: 1},
		app.WithPreDeletionSpoiler(actions, false),
	)

	_, err := handler.HandleUpdate(
		context.Background(),
		reactionUpdate(1, -100123, 77, nil, []telegram.ReactionType{emojiReaction("👎")}),
	)
	if !errors.Is(err, app.ErrOriginalMessageUnavailable) {
		t.Fatalf("HandleUpdate() error = %v, want %v", err, app.ErrOriginalMessageUnavailable)
	}
	if len(actions.events) != 0 {
		t.Errorf("actions = %v, want none", actions.events)
	}
}

func TestHandlerDoesNotPublishDuplicateSpoilerWhenDeletionIsRetried(t *testing.T) {
	wantErr := errors.New("delete failed")
	actions := &recordingModerationActions{deleteErr: wantErr}
	handler := app.NewHandler(
		actions,
		moderation.Thresholds{Positive: 1, Negative: 1},
		app.WithPreDeletionSpoiler(actions, false),
	)

	_, err := handler.HandleUpdate(context.Background(), telegram.Update{
		UpdateID: 1,
		Message: &telegram.Message{
			MessageID: 77,
			Chat:      telegram.Chat{ID: -100123},
			Text:      "исходный текст",
		},
	})
	if err != nil {
		t.Fatalf("store HandleUpdate() error = %v", err)
	}

	update := reactionUpdate(2, -100123, 77, nil, []telegram.ReactionType{emojiReaction("👎")})
	for attempt := 1; attempt <= 2; attempt++ {
		_, err = handler.HandleUpdate(context.Background(), update)
		if !errors.Is(err, wantErr) {
			t.Fatalf("HandleUpdate() attempt %d error = %v, want wrapped %v", attempt, err, wantErr)
		}
	}

	if len(actions.messages) != 1 {
		t.Errorf("SendMessage() calls = %d, want 1", len(actions.messages))
	}
	if len(actions.deleteCalls) != 2 {
		t.Errorf("DeleteMessage() calls = %d, want 2", len(actions.deleteCalls))
	}
	if len(actions.events) != 3 || actions.events[0] != "send" || actions.events[1] != "delete" || actions.events[2] != "delete" {
		t.Errorf("action order = %v, want [send delete delete]", actions.events)
	}
}

func TestHandlerDryRunDoesNotPublishSpoiler(t *testing.T) {
	actions := &recordingModerationActions{}
	handler := app.NewHandler(
		app.NewDeletionGate(actions, true),
		moderation.Thresholds{Positive: 1, Negative: 1},
		app.WithPreDeletionSpoiler(actions, true),
	)

	result, err := handler.HandleUpdate(
		context.Background(),
		reactionUpdate(1, -100123, 77, nil, []telegram.ReactionType{emojiReaction("👎")}),
	)
	if err != nil {
		t.Fatalf("HandleUpdate() error = %v", err)
	}
	if result.Decision != moderation.DeleteMessage {
		t.Errorf("decision = %v, want %v", result.Decision, moderation.DeleteMessage)
	}
	if len(actions.events) != 0 {
		t.Errorf("actions = %v, want none", actions.events)
	}
}

func TestHandlerReturnsDeleteError(t *testing.T) {
	wantErr := errors.New("delete failed")
	handler := app.NewHandler(
		&recordingDeleter{err: wantErr},
		moderation.Thresholds{Positive: 1, Negative: 1},
	)

	result, err := handler.HandleUpdate(
		context.Background(),
		reactionUpdate(1, -100123, 77, nil, []telegram.ReactionType{emojiReaction("👎")}),
	)
	if !errors.Is(err, wantErr) {
		t.Fatalf("HandleUpdate() error = %v, want wrapped %v", err, wantErr)
	}
	if result.Decision != moderation.DeleteMessage {
		t.Errorf("decision = %v, want %v", result.Decision, moderation.DeleteMessage)
	}
}

type deleteCall struct {
	chatID    int64
	messageID int
}

type recordingDeleter struct {
	calls []deleteCall
	err   error
}

type recordingModerationActions struct {
	events      []string
	messages    []telegram.SendMessageOptions
	deleteCalls []deleteCall
	sendErr     error
	deleteErr   error
}

func (actions *recordingModerationActions) SendMessage(_ context.Context, options telegram.SendMessageOptions) error {
	actions.events = append(actions.events, "send")
	actions.messages = append(actions.messages, options)
	return actions.sendErr
}

func (actions *recordingModerationActions) DeleteMessage(_ context.Context, chatID int64, messageID int) error {
	actions.events = append(actions.events, "delete")
	actions.deleteCalls = append(actions.deleteCalls, deleteCall{chatID: chatID, messageID: messageID})
	return actions.deleteErr
}

func (deleter *recordingDeleter) DeleteMessage(_ context.Context, chatID int64, messageID int) error {
	deleter.calls = append(deleter.calls, deleteCall{chatID: chatID, messageID: messageID})
	return deleter.err
}

func reactionUpdate(
	updateID int64,
	chatID int64,
	messageID int,
	oldReaction []telegram.ReactionType,
	newReaction []telegram.ReactionType,
) telegram.Update {
	return telegram.Update{
		UpdateID: updateID,
		MessageReaction: &telegram.MessageReactionUpdated{
			Chat:        telegram.Chat{ID: chatID},
			MessageID:   messageID,
			OldReaction: oldReaction,
			NewReaction: newReaction,
		},
	}
}

func userReactionUpdate(
	updateID int64,
	chatID int64,
	messageID int,
	user telegram.User,
	oldReaction []telegram.ReactionType,
	newReaction []telegram.ReactionType,
) telegram.Update {
	update := reactionUpdate(updateID, chatID, messageID, oldReaction, newReaction)
	update.MessageReaction.User = &user
	return update
}

func emojiReaction(emoji string) telegram.ReactionType {
	return telegram.ReactionType{Type: "emoji", Emoji: emoji}
}

func formattedEntityText(text string, entity telegram.MessageEntity) string {
	encoded := utf16.Encode([]rune(text))
	return string(utf16.Decode(encoded[entity.Offset : entity.Offset+entity.Length]))
}

func findEntity(entities []telegram.MessageEntity, entityType, entityURL string) (telegram.MessageEntity, bool) {
	for _, entity := range entities {
		if entity.Type == entityType && (entityURL == "" || entity.URL == entityURL) {
			return entity, true
		}
	}
	return telegram.MessageEntity{}, false
}
