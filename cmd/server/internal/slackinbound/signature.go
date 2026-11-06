package slackinbound

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net/http"
	"strconv"
	"sync"
	"time"
)

const (
	maxBodyBytes    = 64 << 10
	signatureWindow = 5 * time.Minute
	replayTTL       = 10 * time.Minute
)

type replaySet struct {
	mu        sync.Mutex
	entries   map[string]time.Time
	nextPrune time.Time
}

func (s *replaySet) seen(key string, now time.Time, record bool) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	// Prune on traffic, at most once per minute; no background goroutine or
	// scan of all requests on every signature is needed.
	if !now.Before(s.nextPrune) {
		for key, expires := range s.entries {
			if !now.Before(expires) {
				delete(s.entries, key)
			}
		}
		s.nextPrune = now.Add(time.Minute)
	}
	if expires, exists := s.entries[key]; exists && now.Before(expires) {
		return true
	}
	if record {
		s.entries[key] = now.Add(replayTTL)
	}
	return false
}

func (h *handler) verify(w http.ResponseWriter, r *http.Request, now time.Time) ([]byte, bool) {
	timestamp := r.Header.Get("X-Slack-Request-Timestamp")
	seconds, err := strconv.ParseInt(timestamp, 10, 64)
	if err != nil {
		return nil, false
	}
	at := time.Unix(seconds, 0)
	if at.Before(now.Add(-signatureWindow)) || at.After(now.Add(signatureWindow)) {
		return nil, false
	}
	signature := r.Header.Get("X-Slack-Signature")
	if len(signature) != len("v0=")+sha256.Size*2 || signature[:3] != "v0=" {
		return nil, false
	}
	key := timestamp + ":" + signature
	if h.replays.seen(key, now, false) || r.ContentLength > maxBodyBytes {
		return nil, false
	}
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxBodyBytes))
	if err != nil {
		return nil, false
	}
	mac := hmac.New(sha256.New, []byte(h.signingSecret))
	_, _ = io.WriteString(mac, "v0:"+timestamp+":")
	_, _ = mac.Write(body)
	expected := "v0=" + hex.EncodeToString(mac.Sum(nil))
	if !hmac.Equal([]byte(signature), []byte(expected)) {
		return nil, false
	}
	// Check and insert atomically after authentication, so concurrent copies
	// cannot both enter the ledger path or open two views.
	if h.replays.seen(key, now, true) {
		return nil, false
	}
	return body, true
}
