// Package slack implements the Slack Web API transport used by Peer Bonus.
// Calls have a ten-second deadline and never retry; delivery policy belongs to
// the caller. Only Slack's API or a loopback development origin is accepted.
package slack

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const defaultBaseURL = "https://slack.com/api"
const callTimeout = 10 * time.Second

// Client is safe for concurrent use after construction.
type Client struct {
	token string
	base  string
	http  *http.Client
}

type Option func(*Client)

// WithBaseURL overrides the API endpoint. New rejects empty values and anything
// other than the default or an HTTP(S) loopback origin without a path or query.
func WithBaseURL(raw string) Option { return func(c *Client) { c.base = raw } }

// WithHTTPClient supplies a transport. New copies the client and disables
// redirects so they cannot bypass the endpoint allowlist or forward credentials.
func WithHTTPClient(client *http.Client) Option {
	return func(c *Client) { c.http = client }
}

func New(token string, opts ...Option) (*Client, error) {
	c := &Client{token: token, base: defaultBaseURL, http: http.DefaultClient}
	for _, opt := range opts {
		opt(c)
	}
	if !validBaseURL(c.base) {
		return nil, errors.New("slack: base URL must be https://slack.com/api or a loopback HTTP(S) origin")
	}
	if c.http == nil {
		c.http = http.DefaultClient
	}
	httpClient := *c.http
	httpClient.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	c.http = &httpClient
	return c, nil
}

func validBaseURL(raw string) bool {
	if raw == defaultBaseURL {
		return true
	}
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.User != nil || u.Opaque != "" || u.Path != "" || u.RawQuery != "" || u.ForceQuery || strings.Contains(raw, "#") {
		return false
	}
	switch u.Hostname() {
	case "127.0.0.1", "localhost", "::1":
	default:
		return false
	}
	host := u.Hostname()
	if host == "::1" {
		host = "[::1]"
	}
	if port := u.Port(); port != "" {
		n, err := strconv.Atoi(port)
		if err != nil || n < 1 || n > 65535 {
			return false
		}
		host += ":" + port
	}
	return u.Host == host
}

type Error struct {
	Method     string
	Code       string
	Status     int
	RetryAfter time.Duration
}

func (e *Error) Error() string {
	return fmt.Sprintf("slack: %s: %s (HTTP %d)", e.Method, e.Code, e.Status)
}

var ErrUserNotFound = errors.New("slack: user not found")

func (e *Error) Unwrap() error {
	if e.Code == "users_not_found" {
		return ErrUserNotFound
	}
	return nil
}

func IsRateLimited(err error) bool {
	var apiError *Error
	return errors.As(err, &apiError) && (apiError.Status == http.StatusTooManyRequests || apiError.Code == "ratelimited")
}

type AuthInfo struct {
	TeamID       string `json:"team_id"`
	TeamName     string `json:"team"`
	BotUserID    string `json:"user_id"`
	EnterpriseID string `json:"enterprise_id,omitempty"`
}

type Message struct {
	Text   string          `json:"text"`
	Blocks json.RawMessage `json:"blocks,omitempty"`
}

type Channel struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	IsMember bool   `json:"is_member"`
}

type envelope struct {
	OK   bool   `json:"ok"`
	Code string `json:"error"`
}

func (e envelope) status() envelope { return e }

type response interface{ status() envelope }

func (c *Client) call(ctx context.Context, method string, body any, out response) error {
	ctx, cancel := context.WithTimeout(ctx, callTimeout)
	defer cancel()
	data, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("slack: %s: encode request: %w", method, err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.base+"/"+method, bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("slack: %s: create request: %w", method, err)
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Content-Type", "application/json; charset=utf-8")
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("slack: %s: request: %w", method, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusTooManyRequests {
		return &Error{Method: method, Code: "ratelimited", Status: resp.StatusCode, RetryAfter: retryAfter(resp.Header)}
	}
	decodeErr := json.NewDecoder(resp.Body).Decode(out)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		code := out.status().Code
		if code == "" {
			code = "http_error"
		}
		apiError := &Error{Method: method, Code: code, Status: resp.StatusCode}
		if IsRateLimited(apiError) {
			apiError.RetryAfter = retryAfter(resp.Header)
		}
		return apiError
	}
	if decodeErr != nil {
		return fmt.Errorf("slack: %s: decode response: %w", method, decodeErr)
	}
	if status := out.status(); !status.OK {
		code := status.Code
		if code == "" {
			code = "unknown_error"
		}
		apiError := &Error{Method: method, Code: code, Status: resp.StatusCode}
		if IsRateLimited(apiError) {
			apiError.RetryAfter = retryAfter(resp.Header)
		}
		return apiError
	}
	return nil
}

func retryAfter(header http.Header) time.Duration {
	seconds, err := strconv.ParseInt(header.Get("Retry-After"), 10, 64)
	if err != nil || seconds <= 0 || seconds > (1<<63-1)/int64(time.Second) {
		return time.Second
	}
	return time.Duration(seconds) * time.Second
}

func (c *Client) AuthTest(ctx context.Context) (AuthInfo, error) {
	var out struct {
		envelope
		AuthInfo
	}
	err := c.call(ctx, "auth.test", struct{}{}, &out)
	return out.AuthInfo, err
}

type messageRequest struct {
	Channel string `json:"channel"`
	TS      string `json:"ts,omitempty"`
	User    string `json:"user,omitempty"`
	Message
}

func (c *Client) PostMessage(ctx context.Context, channel string, m Message) (string, error) {
	var out struct {
		envelope
		TS string `json:"ts"`
	}
	err := c.call(ctx, "chat.postMessage", messageRequest{Channel: channel, Message: m}, &out)
	return out.TS, err
}

func (c *Client) UpdateMessage(ctx context.Context, channel, ts string, m Message) error {
	var out envelope
	return c.call(ctx, "chat.update", messageRequest{Channel: channel, TS: ts, Message: m}, &out)
}

func (c *Client) DeleteMessage(ctx context.Context, channel, ts string) error {
	var out envelope
	body := struct {
		Channel string `json:"channel"`
		TS      string `json:"ts"`
	}{channel, ts}
	return c.call(ctx, "chat.delete", body, &out)
}

func (c *Client) PostEphemeral(ctx context.Context, channel, user string, m Message) error {
	var out envelope
	return c.call(ctx, "chat.postEphemeral", messageRequest{Channel: channel, User: user, Message: m}, &out)
}

func (c *Client) LookupUserByEmail(ctx context.Context, email string) (string, error) {
	var out struct {
		envelope
		User struct {
			ID string `json:"id"`
		} `json:"user"`
	}
	body := struct {
		Email string `json:"email"`
	}{email}
	err := c.call(ctx, "users.lookupByEmail", body, &out)
	return out.User.ID, err
}

// ListPublicChannels follows Slack cursors, returning at most 1,000 channels.
func (c *Client) ListPublicChannels(ctx context.Context) ([]Channel, error) {
	body := struct {
		Types           string `json:"types"`
		ExcludeArchived bool   `json:"exclude_archived"`
		Limit           int    `json:"limit"`
		Cursor          string `json:"cursor,omitempty"`
	}{Types: "public_channel", ExcludeArchived: true, Limit: 200}
	channels := make([]Channel, 0)
	for {
		var out struct {
			envelope
			Channels []Channel `json:"channels"`
			Metadata struct {
				NextCursor string `json:"next_cursor"`
			} `json:"response_metadata"`
		}
		if err := c.call(ctx, "conversations.list", body, &out); err != nil {
			return nil, err
		}
		channels = append(channels, out.Channels[:min(len(out.Channels), 1000-len(channels))]...)
		if len(channels) == 1000 || out.Metadata.NextCursor == "" {
			return channels, nil
		}
		body.Cursor = out.Metadata.NextCursor
	}
}

func (c *Client) OpenView(ctx context.Context, triggerID string, view json.RawMessage) error {
	var out envelope
	body := struct {
		TriggerID string          `json:"trigger_id"`
		View      json.RawMessage `json:"view"`
	}{triggerID, view}
	return c.call(ctx, "views.open", body, &out)
}
