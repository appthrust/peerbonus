// Package slackinboundtest supplies request signing for local Slack input tests.
package slackinboundtest

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"io"
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
