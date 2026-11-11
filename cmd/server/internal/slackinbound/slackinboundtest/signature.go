// Package slackinboundtest supplies request signing for local Slack input tests.
package slackinboundtest

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"time"
)

// SignRequest returns X-Slack-Signature for body. Set
// X-Slack-Request-Timestamp to strconv.FormatInt(ts.Unix(), 10), and send body
// unchanged (including application/x-www-form-urlencoded escaping).
func SignRequest(secret string, ts time.Time, body []byte) (sigHeader string) {
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = io.WriteString(mac, "v0:"+strconv.FormatInt(ts.Unix(), 10)+":")
	_, _ = mac.Write(body)
	return "v0=" + hex.EncodeToString(mac.Sum(nil))
}

// SignedJSONRequest creates an Events API request signed over the exact JSON.
func SignedJSONRequest(secret string, ts time.Time, path string, body []byte) *http.Request {
	r := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("X-Slack-Request-Timestamp", strconv.FormatInt(ts.Unix(), 10))
	r.Header.Set("X-Slack-Signature", SignRequest(secret, ts, body))
	return r
}
