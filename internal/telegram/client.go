package telegram

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

const (
	// DefaultBaseURL is the public Telegram Bot API endpoint.
	DefaultBaseURL = "https://api.telegram.org"

	messageUpdate         = "message"
	messageReactionUpdate = "message_reaction"
)

var (
	// ErrTokenRequired indicates that a client was configured without a bot
	// token.
	ErrTokenRequired = errors.New("telegram bot token is required")
	// ErrInvalidBaseURL indicates that the configured Telegram API base URL is
	// not an absolute HTTP URL.
	ErrInvalidBaseURL = errors.New("invalid Telegram API base URL")
)

// ClientConfig configures a Telegram Bot API client. BaseURL defaults to
// DefaultBaseURL and HTTPClient defaults to http.DefaultClient.
type ClientConfig struct {
	Token      string
	BaseURL    string
	HTTPClient *http.Client
}

// Client makes requests to the Telegram Bot API.
type Client struct {
	token      string
	baseURL    string
	httpClient *http.Client
}

// GetUpdatesOptions controls one getUpdates long-poll request. A zero limit or
// timeout asks Telegram to use its default value.
type GetUpdatesOptions struct {
	Offset         int64
	Limit          int
	TimeoutSeconds int
}

// SendMessageOptions describes one plain-text Telegram message and its
// optional entity formatting.
type SendMessageOptions struct {
	ChatID             int64
	Text               string
	Entities           []MessageEntity
	LinkPreviewOptions *LinkPreviewOptions
	ProtectContent     bool
}

// APIError is an error response returned by the Telegram Bot API.
type APIError struct {
	ErrorCode   int
	Description string
	HTTPStatus  int
}

// Error implements error.
func (err *APIError) Error() string {
	if err.Description == "" {
		return fmt.Sprintf("Telegram API error %d", err.ErrorCode)
	}

	return fmt.Sprintf("Telegram API error %d: %s", err.ErrorCode, err.Description)
}

// NewClient constructs a Telegram Bot API client.
func NewClient(config ClientConfig) (*Client, error) {
	if strings.TrimSpace(config.Token) == "" {
		return nil, ErrTokenRequired
	}

	baseURL := config.BaseURL
	if baseURL == "" {
		baseURL = DefaultBaseURL
	}

	parsedBaseURL, err := url.Parse(baseURL)
	if err != nil || (parsedBaseURL.Scheme != "http" && parsedBaseURL.Scheme != "https") || parsedBaseURL.Host == "" || parsedBaseURL.RawQuery != "" || parsedBaseURL.Fragment != "" || parsedBaseURL.User != nil {
		return nil, ErrInvalidBaseURL
	}

	httpClient := config.HTTPClient
	if httpClient == nil {
		httpClient = http.DefaultClient
	}

	return &Client{
		token:      config.Token,
		baseURL:    strings.TrimRight(baseURL, "/"),
		httpClient: httpClient,
	}, nil
}

// GetUpdates performs one getUpdates request. It always explicitly subscribes
// to new messages and non-anonymous message reaction updates.
func (client *Client) GetUpdates(ctx context.Context, options GetUpdatesOptions) ([]Update, error) {
	if options.Limit < 0 || options.Limit > 100 {
		return nil, errors.New("get updates: limit must be between 1 and 100, or zero")
	}
	if options.TimeoutSeconds < 0 {
		return nil, errors.New("get updates: timeout must not be negative")
	}

	request := getUpdatesRequest{
		Offset:         options.Offset,
		Limit:          options.Limit,
		TimeoutSeconds: options.TimeoutSeconds,
		AllowedUpdates: []string{
			messageUpdate,
			messageReactionUpdate,
		},
	}

	var updates []Update
	if err := client.call(ctx, "getUpdates", request, &updates); err != nil {
		return nil, fmt.Errorf("get updates: %w", err)
	}

	return updates, nil
}

// NextOffset returns the offset that acknowledges all supplied processed
// updates. Call it only after those updates have been processed successfully.
func NextOffset(current int64, processed []Update) int64 {
	next := current
	for _, update := range processed {
		candidate := update.UpdateID + 1
		if candidate > next {
			next = candidate
		}
	}

	return next
}

// DeleteMessage deletes one message through the Telegram Bot API.
func (client *Client) DeleteMessage(ctx context.Context, chatID int64, messageID int) error {
	request := deleteMessageRequest{
		ChatID:    chatID,
		MessageID: messageID,
	}

	var deleted bool
	if err := client.call(ctx, "deleteMessage", request, &deleted); err != nil {
		return fmt.Errorf("delete message: %w", err)
	}
	if !deleted {
		return errors.New("delete message: Telegram API returned false")
	}

	return nil
}

// SendMessage sends a plain-text message to a Telegram chat.
func (client *Client) SendMessage(ctx context.Context, options SendMessageOptions) error {
	request := sendMessageRequest{
		ChatID:             options.ChatID,
		Text:               options.Text,
		Entities:           options.Entities,
		LinkPreviewOptions: options.LinkPreviewOptions,
		ProtectContent:     options.ProtectContent,
	}

	var sentMessage struct {
		MessageID int `json:"message_id"`
	}
	if err := client.call(ctx, "sendMessage", request, &sentMessage); err != nil {
		return fmt.Errorf("send message: %w", err)
	}

	return nil
}

type getUpdatesRequest struct {
	Offset         int64    `json:"offset,omitempty"`
	Limit          int      `json:"limit,omitempty"`
	TimeoutSeconds int      `json:"timeout,omitempty"`
	AllowedUpdates []string `json:"allowed_updates"`
}

type deleteMessageRequest struct {
	ChatID    int64 `json:"chat_id"`
	MessageID int   `json:"message_id"`
}

type sendMessageRequest struct {
	ChatID             int64               `json:"chat_id"`
	Text               string              `json:"text"`
	Entities           []MessageEntity     `json:"entities,omitempty"`
	LinkPreviewOptions *LinkPreviewOptions `json:"link_preview_options,omitempty"`
	ProtectContent     bool                `json:"protect_content,omitempty"`
}

type apiResponse struct {
	OK          bool            `json:"ok"`
	Result      json.RawMessage `json:"result"`
	ErrorCode   int             `json:"error_code"`
	Description string          `json:"description"`
}

func (client *Client) call(ctx context.Context, method string, payload, result any) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("encode %s request: %w", method, err)
	}

	request, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		client.endpoint(method),
		bytes.NewReader(body),
	)
	if err != nil {
		return fmt.Errorf("build %s request", method)
	}
	request.Header.Set("Content-Type", "application/json")

	response, err := client.httpClient.Do(request)
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return ctxErr
		}
		return fmt.Errorf("send %s request", method)
	}
	defer response.Body.Close()

	var envelope apiResponse
	if err := json.NewDecoder(response.Body).Decode(&envelope); err != nil {
		return fmt.Errorf("decode %s response with HTTP status %d: %w", method, response.StatusCode, err)
	}

	if !envelope.OK {
		return &APIError{
			ErrorCode:   envelope.ErrorCode,
			Description: client.redactToken(envelope.Description),
			HTTPStatus:  response.StatusCode,
		}
	}
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return fmt.Errorf("%s returned unexpected HTTP status %d", method, response.StatusCode)
	}
	if len(envelope.Result) == 0 {
		return fmt.Errorf("%s response did not contain a result", method)
	}
	if err := json.Unmarshal(envelope.Result, result); err != nil {
		return fmt.Errorf("decode %s result: %w", method, err)
	}

	return nil
}

func (client *Client) endpoint(method string) string {
	return client.baseURL + "/bot" + url.PathEscape(client.token) + "/" + method
}

func (client *Client) redactToken(value string) string {
	redacted := strings.ReplaceAll(value, client.token, "[REDACTED]")
	escapedToken := url.PathEscape(client.token)
	if escapedToken != client.token {
		redacted = strings.ReplaceAll(redacted, escapedToken, "[REDACTED]")
	}
	return redacted
}
