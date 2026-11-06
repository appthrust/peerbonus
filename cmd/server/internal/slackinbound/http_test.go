package slackinbound

import (
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestBodyLimitAcceptsExactLimit(t *testing.T) {
	_, slack, o := fixture()
	body := commandForm().Encode() + "&padding="
	body += strings.Repeat("x", maxBodyBytes-len(body))
	require.Len(t, body, maxBodyBytes)
	require.Equal(t, http.StatusOK, serve(New(o), "/slack/commands", body).Code)
	require.NotEmpty(t, slack.view)
}

func TestOnlySignedBodySuppliesIdentity(t *testing.T) {
	s, _, o := fixture()
	form := commandForm()
	form.Del("team_id")
	r := signedRequest("/slack/commands?team_id=T123", form.Encode(), testNow)
	w := httptest.NewRecorder()
	New(o).ServeHTTP(w, r)
	require.Equal(t, http.StatusForbidden, w.Code)
	require.Empty(t, s.memberLookups)
}

func TestFormAndRouteBoundary(t *testing.T) {
	for _, tc := range []struct {
		name, path, method, contentType, body string
		status                                int
	}{
		{"method", "/slack/commands", "GET", "application/x-www-form-urlencoded", "", 405},
		{"unknown path", "/slack/events", "POST", "application/x-www-form-urlencoded", "", 404},
		{"json", "/slack/commands", "POST", "application/json", `{}`, 415},
		{"bad form", "/slack/commands", "POST", "application/x-www-form-urlencoded", "%XX", 400},
		{"bad payload", "/slack/interactions", "POST", "application/x-www-form-urlencoded", "payload=%7B", 400},
		{"missing payload", "/slack/options", "POST", "application/x-www-form-urlencoded", "", 400},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, _, o := fixture()
			r := signedRequest(tc.path, tc.body, testNow)
			r.Method = tc.method
			r.Header.Set("Content-Type", tc.contentType)
			w := httptest.NewRecorder()
			New(o).ServeHTTP(w, r)
			require.Equal(t, tc.status, w.Code)
			require.Empty(t, s.memberLookups)
			if tc.status == 405 {
				require.Equal(t, "POST", w.Header().Get("Allow"))
			}
		})
	}
}

func TestCommandResponseRejectsNonSlackDestinations(t *testing.T) {
	for _, target := range []string{"", "http://hooks.slack.com/commands/test", "https://hooks.slack.com.evil.test/x", "https://127.0.0.1/x", "https://hooks.slack.com:444/x", "https://user:password@hooks.slack.com/x"} {
		t.Run(target, func(t *testing.T) {
			_, _, o := fixture()
			o.HTTPClient = &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
				t.Fatal("disallowed response_url reached the transport")
				return nil, nil
			})}
			form := commandForm()
			form.Set("text", "balance")
			form.Set("response_url", target)
			w := serve(New(o), "/slack/commands", form.Encode())
			require.Equal(t, http.StatusBadGateway, w.Code)
			require.Equal(t, "Slack response unavailable\n", w.Body.String())
		})
	}
}

func TestCommandResponseDoesNotFollowRedirects(t *testing.T) {
	_, _, o := fixture()
	calls := 0
	o.HTTPClient = &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		calls++
		return &http.Response{StatusCode: http.StatusFound, Header: http.Header{"Location": {"https://elsewhere.example.test/"}}, Body: io.NopCloser(strings.NewReader(""))}, nil
	})}
	form := commandForm()
	form.Set("text", "balance")
	require.Equal(t, http.StatusBadGateway, serve(New(o), "/slack/commands", form.Encode()).Code)
	require.Equal(t, 1, calls)
	// The constructor does not mutate the caller's shared HTTP client.
	require.Nil(t, o.HTTPClient.CheckRedirect)
}

func TestOptionsLabelHonorsSlackCharacterLimit(t *testing.T) {
	label := strings.Repeat("あ", 80)
	require.Equal(t, strings.Repeat("あ", 75), optionLabel(label))
	require.Equal(t, "short", optionLabel("short"))
}

func TestCommandFormCharsetAndEscaping(t *testing.T) {
	_, slack, o := fixture()
	form := commandForm()
	form.Set("user_name", "Alice & Bob+日本語")
	r := signedRequest("/slack/commands", form.Encode(), testNow)
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded; charset=utf-8")
	w := httptest.NewRecorder()
	New(o).ServeHTTP(w, r)
	require.Equal(t, http.StatusOK, w.Code)
	require.NotEmpty(t, slack.view)
	decoded, err := url.ParseQuery(form.Encode())
	require.NoError(t, err)
	require.Equal(t, form, decoded)
}
