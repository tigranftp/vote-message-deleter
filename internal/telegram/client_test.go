package telegram_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	"vote_message_deleter/internal/telegram"
)

const testToken = "test-token"

func TestClientGetUpdates(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost {
			t.Errorf("method = %q, want %q", request.Method, http.MethodPost)
		}
		if request.URL.Path != "/bot"+testToken+"/getUpdates" {
			t.Errorf("path = %q, want %q", request.URL.Path, "/bot"+testToken+"/getUpdates")
		}
		if request.Header.Get("Content-Type") != "application/json" {
			t.Errorf("Content-Type = %q, want %q", request.Header.Get("Content-Type"), "application/json")
		}

		var body struct {
			Offset         int64    `json:"offset"`
			Limit          int      `json:"limit"`
			TimeoutSeconds int      `json:"timeout"`
			AllowedUpdates []string `json:"allowed_updates"`
		}
		if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
			t.Errorf("decode request: %v", err)
		}
		if body.Offset != 42 {
			t.Errorf("offset = %d, want 42", body.Offset)
		}
		if body.Limit != 25 {
			t.Errorf("limit = %d, want 25", body.Limit)
		}
		if body.TimeoutSeconds != 30 {
			t.Errorf("timeout = %d, want 30", body.TimeoutSeconds)
		}
		if !slices.Contains(body.AllowedUpdates, "message_reaction") {
			t.Errorf("allowed_updates = %v, want message_reaction", body.AllowedUpdates)
		}
		if !slices.Contains(body.AllowedUpdates, "message") {
			t.Errorf("allowed_updates = %v, want message", body.AllowedUpdates)
		}
		if slices.Contains(body.AllowedUpdates, "message_reaction_count") {
			t.Errorf("allowed_updates = %v, do not want message_reaction_count", body.AllowedUpdates)
		}
		if len(body.AllowedUpdates) != 2 {
			t.Errorf("len(allowed_updates) = %d, want 2", len(body.AllowedUpdates))
		}

		writer.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(writer, `{
			"ok": true,
			"result": [
				{
					"update_id": 43,
					"message_reaction": {
						"chat": {"id": -1001},
						"message_id": 8,
						"user": {"id": 101},
						"date": 1700000001,
						"old_reaction": [],
						"new_reaction": [{"type": "emoji", "emoji": "👎"}]
					}
				}
			]
		}`)
	}))
	defer server.Close()

	client := newTestClient(t, server)
	updates, err := client.GetUpdates(context.Background(), telegram.GetUpdatesOptions{
		Offset:         42,
		Limit:          25,
		TimeoutSeconds: 30,
	})
	if err != nil {
		t.Fatalf("GetUpdates() error = %v", err)
	}
	if len(updates) != 1 {
		t.Fatalf("len(GetUpdates()) = %d, want 1", len(updates))
	}
	if updates[0].MessageReaction == nil {
		t.Fatal("message_reaction is nil")
	}
	if updates[0].MessageReaction.Chat.ID != -1001 {
		t.Errorf("chat ID = %d, want -1001", updates[0].MessageReaction.Chat.ID)
	}
	if updates[0].MessageReaction.MessageID != 8 {
		t.Errorf("message ID = %d, want 8", updates[0].MessageReaction.MessageID)
	}
	if updates[0].MessageReaction.User == nil || updates[0].MessageReaction.User.ID != 101 {
		t.Errorf("user = %#v, want ID 101", updates[0].MessageReaction.User)
	}
}

func TestUpdateDecodesMessageAuthorAndForwardOrigin(t *testing.T) {
	var update telegram.Update
	err := json.Unmarshal([]byte(`{
		"update_id": 50,
		"message": {
			"message_id": 77,
			"chat": {"id": -100123, "type": "supergroup", "title": "Group"},
			"from": {"id": 101, "first_name": "Alice", "last_name": "Smith", "username": "alice"},
			"date": 1788464092,
			"forward_origin": {
				"type": "channel",
				"date": 1788463092,
				"chat": {"id": -100987, "type": "channel", "title": "News", "username": "news"},
				"message_id": 321
			},
			"text": "forwarded text"
		}
	}`), &update)
	if err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	if update.Message == nil {
		t.Fatal("message is nil")
	}
	if update.Message.From == nil || update.Message.From.Username != "alice" {
		t.Errorf("message author = %#v, want @alice", update.Message.From)
	}
	if update.Message.ForwardOrigin == nil || update.Message.ForwardOrigin.Type != "channel" {
		t.Fatalf("forward origin = %#v, want channel", update.Message.ForwardOrigin)
	}
	if update.Message.ForwardOrigin.Chat == nil || update.Message.ForwardOrigin.Chat.Username != "news" {
		t.Errorf("forward channel = %#v, want @news", update.Message.ForwardOrigin.Chat)
	}
	if update.Message.ForwardOrigin.MessageID != 321 {
		t.Errorf("forward message ID = %d, want 321", update.Message.ForwardOrigin.MessageID)
	}
}

func TestClientGetUpdatesReturnsAPIError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		writer.WriteHeader(http.StatusUnauthorized)
		_, _ = io.WriteString(writer, `{
			"ok": false,
			"error_code": 401,
			"description": "unauthorized token test-token"
		}`)
	}))
	defer server.Close()

	client := newTestClient(t, server)
	_, err := client.GetUpdates(context.Background(), telegram.GetUpdatesOptions{})
	if err == nil {
		t.Fatal("GetUpdates() error = nil, want Telegram API error")
	}

	var apiError *telegram.APIError
	if !errors.As(err, &apiError) {
		t.Fatalf("GetUpdates() error = %T, want *telegram.APIError", err)
	}
	if apiError.ErrorCode != 401 {
		t.Errorf("API error code = %d, want 401", apiError.ErrorCode)
	}
	if apiError.HTTPStatus != http.StatusUnauthorized {
		t.Errorf("API HTTP status = %d, want %d", apiError.HTTPStatus, http.StatusUnauthorized)
	}
	if strings.Contains(err.Error(), testToken) {
		t.Errorf("GetUpdates() error exposes bot token: %q", err)
	}
}

func TestNextOffset(t *testing.T) {
	updates := []telegram.Update{
		{UpdateID: 44},
		{UpdateID: 42},
		{UpdateID: 45},
	}

	if got := telegram.NextOffset(40, updates); got != 46 {
		t.Errorf("NextOffset() = %d, want 46", got)
	}
	if got := telegram.NextOffset(50, updates); got != 50 {
		t.Errorf("NextOffset() with newer current offset = %d, want 50", got)
	}
}

func TestClientDeleteMessage(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/bot"+testToken+"/deleteMessage" {
			t.Errorf("path = %q, want %q", request.URL.Path, "/bot"+testToken+"/deleteMessage")
		}

		var body struct {
			ChatID    int64 `json:"chat_id"`
			MessageID int   `json:"message_id"`
		}
		if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
			t.Errorf("decode request: %v", err)
		}
		if body.ChatID != -100123 {
			t.Errorf("chat_id = %d, want -100123", body.ChatID)
		}
		if body.MessageID != 77 {
			t.Errorf("message_id = %d, want 77", body.MessageID)
		}

		writer.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(writer, `{"ok":true,"result":true}`)
	}))
	defer server.Close()

	client := newTestClient(t, server)
	if err := client.DeleteMessage(context.Background(), -100123, 77); err != nil {
		t.Fatalf("DeleteMessage() error = %v", err)
	}
}

func TestClientDeleteMessageReturnsAPIError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		writer.WriteHeader(http.StatusBadRequest)
		_, _ = io.WriteString(writer, `{
			"ok": false,
			"error_code": 400,
			"description": "message can't be deleted"
		}`)
	}))
	defer server.Close()

	client := newTestClient(t, server)
	err := client.DeleteMessage(context.Background(), -100123, 77)
	if err == nil {
		t.Fatal("DeleteMessage() error = nil, want Telegram API error")
	}

	var apiError *telegram.APIError
	if !errors.As(err, &apiError) {
		t.Fatalf("DeleteMessage() error = %T, want *telegram.APIError", err)
	}
	if apiError.ErrorCode != 400 {
		t.Errorf("API error code = %d, want 400", apiError.ErrorCode)
	}
	if apiError.Description != "message can't be deleted" {
		t.Errorf("API error description = %q, want %q", apiError.Description, "message can't be deleted")
	}
}

func TestClientSendMessage(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost {
			t.Errorf("method = %q, want %q", request.Method, http.MethodPost)
		}
		if request.URL.Path != "/bot"+testToken+"/sendMessage" {
			t.Errorf("path = %q, want %q", request.URL.Path, "/bot"+testToken+"/sendMessage")
		}

		var body struct {
			ChatID             int64                        `json:"chat_id"`
			Text               string                       `json:"text"`
			Entities           []telegram.MessageEntity     `json:"entities"`
			LinkPreviewOptions *telegram.LinkPreviewOptions `json:"link_preview_options"`
			ProtectContent     bool                         `json:"protect_content"`
		}
		if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
			t.Errorf("decode request: %v", err)
		}
		if body.ChatID != -100123 {
			t.Errorf("chat_id = %d, want -100123", body.ChatID)
		}
		if body.Text != "Сообщение удалено" {
			t.Errorf("text = %q, want %q", body.Text, "Сообщение удалено")
		}
		if !body.ProtectContent {
			t.Error("protect_content = false, want true")
		}
		if body.LinkPreviewOptions == nil || !body.LinkPreviewOptions.IsDisabled {
			t.Errorf("link_preview_options = %#v, want disabled", body.LinkPreviewOptions)
		}
		if len(body.Entities) != 1 || body.Entities[0].Type != "spoiler" {
			t.Errorf("entities = %#v, want one spoiler", body.Entities)
		}

		writer.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(writer, `{"ok":true,"result":{"message_id":78}}`)
	}))
	defer server.Close()

	client := newTestClient(t, server)
	if err := client.SendMessage(context.Background(), telegram.SendMessageOptions{
		ChatID: -100123,
		Text:   "Сообщение удалено",
		LinkPreviewOptions: &telegram.LinkPreviewOptions{
			IsDisabled: true,
		},
		ProtectContent: true,
		Entities: []telegram.MessageEntity{
			{Type: "spoiler", Offset: 0, Length: 18},
		},
	}); err != nil {
		t.Fatalf("SendMessage() error = %v", err)
	}
}

func newTestClient(t *testing.T, server *httptest.Server) *telegram.Client {
	t.Helper()

	client, err := telegram.NewClient(telegram.ClientConfig{
		Token:      testToken,
		BaseURL:    server.URL,
		HTTPClient: server.Client(),
	})
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}

	return client
}
