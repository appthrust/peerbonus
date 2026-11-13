// Package slacktest provides a deterministic, in-memory Slack Web API fake.
// New starts an httptest server and registers its cleanup. NewHandler exposes
// the same implementation for the standalone cmd/fakeslack smoke server.
package slacktest

import (
	"encoding/json"
	"fmt"
	"mime"
	"net/http"
	"net/http/httptest"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/appthrust/peerbonus/cmd/server/internal/slack"
)

type Server struct {
	URL      string
	mu       sync.Mutex
	team     slack.AuthInfo
	users    map[string]slack.User
	channels []slack.Channel
	failures map[string][]string
	calls    []Call
	messages []Posted
	views    []json.RawMessage
	nextTS   uint64
}

type ServerOption func(*Server)

func WithTeam(team slack.AuthInfo) ServerOption { return func(s *Server) { s.team = team } }

type Call struct {
	Method string         `json:"method"`
	Body   map[string]any `json:"body"`
	Auth   string         `json:"auth"`
}

type Posted struct {
	Channel  string          `json:"channel"`
	TS       string          `json:"ts"`
	ThreadTS string          `json:"thread_ts,omitempty"`
	Text     string          `json:"text"`
	Blocks   json.RawMessage `json:"blocks,omitempty"`
	Deleted  bool            `json:"deleted"`
	Updates  int             `json:"updates"`
}

func New(t testing.TB, opts ...ServerOption) *Server {
	t.Helper()
	s := NewHandler(opts...)
	server := httptest.NewServer(s)
	s.URL = server.URL
	t.Cleanup(server.Close)
	return s
}

// NewHandler creates a fake without opening a listener. It also serves GET
// /_calls with calls, messages, and views; authorization values are redacted.
// Slack methods accept JSON or form bodies. Permalinks also cover incoming
// messages not posted through this fake, provided their channel is configured.
func NewHandler(opts ...ServerOption) *Server {
	s := &Server{
		team:  slack.AuthInfo{TeamID: "T123", TeamName: "Acme", BotUserID: "UBOT"},
		users: make(map[string]slack.User), failures: make(map[string][]string),
		channels: []slack.Channel{}, calls: []Call{}, messages: []Posted{}, views: []json.RawMessage{},
	}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

func (s *Server) Team() slack.AuthInfo {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.team
}

func (s *Server) AddUser(email, userID string) {
	s.addUser(slack.User{ID: userID, Email: email})
}

// AddBot registers a user returned by users.info with is_bot set.
func (s *Server) AddBot(email, userID string) {
	s.addUser(slack.User{ID: userID, Email: email, IsBot: true})
}

func (s *Server) addUser(user slack.User) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.users[strings.ToLower(user.Email)] = user
}

func (s *Server) AddChannel(id, name string, botMember bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	channel := slack.Channel{ID: id, Name: name, IsMember: botMember}
	for i := range s.channels {
		if s.channels[i].ID == id {
			s.channels[i] = channel
			return
		}
	}
	s.channels = append(s.channels, channel)
}

// Fail queues one failure for the next call of method, without applying that
// call's side effects. Multiple failures are consumed in FIFO order per method.
func (s *Server) Fail(method, errCode string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.failures[method] = append(s.failures[method], errCode)
}

// Calls returns an independent snapshot. An empty method selects all calls.
func (s *Server) Calls(method string) []Call {
	s.mu.Lock()
	defer s.mu.Unlock()
	calls := make([]Call, 0, len(s.calls))
	for _, call := range s.calls {
		if method == "" || call.Method == method {
			call.Body = cloneJSON(call.Body).(map[string]any)
			calls = append(calls, call)
		}
	}
	return calls
}

func (s *Server) Messages() []Posted {
	s.mu.Lock()
	defer s.mu.Unlock()
	messages := slices.Clone(s.messages)
	for i := range messages {
		messages[i].Blocks = slices.Clone(messages[i].Blocks)
	}
	return messages
}

func (s *Server) Views() []json.RawMessage {
	s.mu.Lock()
	defer s.mu.Unlock()
	views := make([]json.RawMessage, len(s.views))
	for i := range s.views {
		views[i] = slices.Clone(s.views[i])
	}
	return views
}

func cloneJSON(value any) any {
	switch v := value.(type) {
	case map[string]any:
		out := make(map[string]any, len(v))
		for key, item := range v {
			out[key] = cloneJSON(item)
		}
		return out
	case []any:
		out := make([]any, len(v))
		for i, item := range v {
			out[i] = cloneJSON(item)
		}
		return out
	default:
		return value
	}
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	if r.Method == http.MethodGet && r.URL.Path == "/_calls" {
		calls := s.Calls("")
		for i := range calls {
			if calls[i].Auth != "" {
				calls[i].Auth = "Bearer [redacted]"
			}
		}
		writeJSON(w, map[string]any{"calls": calls, "messages": s.Messages(), "views": s.Views()})
		return
	}
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		w.WriteHeader(http.StatusMethodNotAllowed)
		writeError(w, "method_not_supported")
		return
	}
	var body map[string]any
	contentType, _, _ := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if contentType == "application/x-www-form-urlencoded" {
		if err := r.ParseForm(); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			writeError(w, "invalid_form")
			return
		}
		body = make(map[string]any, len(r.PostForm))
		for key := range r.PostForm {
			body[key] = r.PostForm.Get(key)
		}
	} else if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body == nil {
		w.WriteHeader(http.StatusBadRequest)
		writeError(w, "invalid_json")
		return
	}
	method := strings.TrimPrefix(r.URL.Path, "/")
	s.mu.Lock()
	defer s.mu.Unlock()
	s.calls = append(s.calls, Call{Method: method, Body: body, Auth: r.Header.Get("Authorization")})
	if failures := s.failures[method]; len(failures) > 0 {
		code := failures[0]
		s.failures[method] = failures[1:]
		if code == "ratelimited" {
			w.Header().Set("Retry-After", "1")
			w.WriteHeader(http.StatusTooManyRequests)
		}
		writeError(w, code)
		return
	}
	field := func(key string) string { value, _ := body[key].(string); return value }
	switch method {
	case "auth.test":
		writeJSON(w, struct {
			OK bool `json:"ok"`
			slack.AuthInfo
		}{true, s.team})
	case "users.lookupByEmail":
		user, ok := s.users[strings.ToLower(field("email"))]
		if !ok {
			writeError(w, "users_not_found")
			return
		}
		writeJSON(w, map[string]any{"ok": true, "user": map[string]string{"id": user.ID}})
	case "users.info":
		for _, user := range s.users {
			if user.ID == field("user") {
				writeJSON(w, map[string]any{"ok": true, "user": map[string]any{
					"id": user.ID, "is_bot": user.IsBot, "deleted": user.Deleted,
					"profile": map[string]string{"email": user.Email},
				}})
				return
			}
		}
		writeError(w, "user_not_found")
	case "chat.getPermalink":
		channel, ts := field("channel"), field("message_ts")
		if code := s.channelError(channel); code != "" {
			writeError(w, code)
			return
		}
		if ts == "" {
			writeError(w, "message_not_found")
			return
		}
		writeJSON(w, map[string]any{"ok": true, "channel": channel, "permalink": "https://example.slack.com/archives/" + channel + "/p" + strings.ReplaceAll(ts, ".", "")})
	case "conversations.list":
		start := 0
		if cursor := field("cursor"); cursor != "" {
			var err error
			start, err = strconv.Atoi(cursor)
			if err != nil || start < 0 || start > len(s.channels) {
				writeError(w, "invalid_cursor")
				return
			}
		}
		end := min(start+2, len(s.channels))
		next := ""
		if end < len(s.channels) {
			next = strconv.Itoa(end)
		}
		writeJSON(w, map[string]any{"ok": true, "channels": s.channels[start:end], "response_metadata": map[string]string{"next_cursor": next}})
	case "chat.postMessage", "chat.postEphemeral":
		channel := field("channel")
		if code := s.channelError(channel); code != "" {
			writeError(w, code)
			return
		}
		s.nextTS++
		ts := fmt.Sprintf("1700000000.%06d", s.nextTS)
		if method == "chat.postEphemeral" {
			writeJSON(w, map[string]any{"ok": true, "message_ts": ts})
			return
		}
		s.messages = append(s.messages, Posted{Channel: channel, TS: ts, ThreadTS: field("thread_ts"), Text: field("text"), Blocks: rawField(body, "blocks")})
		writeJSON(w, map[string]any{"ok": true, "channel": channel, "ts": ts})
	case "chat.update", "chat.delete":
		channel := field("channel")
		if code := s.channelError(channel); code != "" {
			writeError(w, code)
			return
		}
		for i := range s.messages {
			message := &s.messages[i]
			if message.Channel != channel || message.TS != field("ts") || message.Deleted {
				continue
			}
			if method == "chat.delete" {
				message.Deleted = true
			} else {
				message.Text = field("text")
				message.Blocks = rawField(body, "blocks")
				message.Updates++
			}
			writeJSON(w, map[string]any{"ok": true, "channel": channel, "ts": message.TS})
			return
		}
		writeError(w, "message_not_found")
	case "views.open":
		s.views = append(s.views, rawField(body, "view"))
		writeJSON(w, map[string]any{"ok": true, "view": map[string]string{"id": fmt.Sprintf("V%d", len(s.views))}})
	default:
		writeError(w, "unknown_method")
	}
}

// Slack permits posting directly to a known user ID to open that user's DM.
func (s *Server) channelError(id string) string {
	for _, channel := range s.channels {
		if channel.ID == id {
			if !channel.IsMember {
				return "not_in_channel"
			}
			return ""
		}
	}
	for _, user := range s.users {
		if user.ID == id {
			return ""
		}
	}
	return "channel_not_found"
}

func rawField(body map[string]any, key string) json.RawMessage {
	value, ok := body[key]
	if !ok {
		return nil
	}
	if data, ok := value.(string); ok {
		return json.RawMessage(data)
	}
	data, _ := json.Marshal(value) // Values were produced by encoding/json.
	return data
}

func writeError(w http.ResponseWriter, code string) {
	writeJSON(w, map[string]any{"ok": false, "error": code})
}

func writeJSON(w http.ResponseWriter, value any) { _ = json.NewEncoder(w).Encode(value) }
