package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/appthrust/appserver-go/appserver/httpx"
	"github.com/appthrust/appserver-go/appserver/launcher"
	"github.com/appthrust/appserver-go/appserver/pgstore/testutil"
	"github.com/stretchr/testify/require"
)

func TestConfigValidation(t *testing.T) {
	t.Setenv("PLATFORM_OIDC_ISSUER", "")
	t.Setenv("PLATFORM_OIDC_ISSUER_CA_PEM", "")
	staticDir := t.TempDir()
	for _, origin := range []string{"https://dashboard.example.test", "http://127.0.0.1:1", "http://localhost:8080"} {
		_, err := newHandler(staticDir, origin, "pj-test", nil, nil, nil)
		require.NoError(t, err)
	}
	for _, origin := range []string{"http://dashboard.example.test", "https://user:pass@example.test", "https://example.test/path", "https://example.test?q=1", ""} {
		handler, err := newHandler(staticDir, origin, "pj-test", nil, nil, nil)
		require.EqualError(t, err, "PLATFORM_API_URL must be an HTTPS origin (HTTP is allowed only for loopback hosts)")
		require.Nil(t, handler)
	}
	handler, err := newHandler(staticDir, "https://example.test", "BAD", nil, nil, nil)
	require.EqualError(t, err, "PLATFORM_PROJECT_ID must be a non-empty DNS label (at most 63 lowercase alphanumeric or hyphen characters, starting and ending with alphanumeric)")
	require.Nil(t, handler)
	for _, origin := range []string{"http://example.test", "https://example.test/", "https://example.test/#"} {
		t.Setenv("PEERBONUS_PUBLIC_URL", origin)
		_, _, _, err := peerbonusHandler("http://127.0.0.1:1", "pj-test")
		require.Error(t, err)
	}
	t.Setenv("PEERBONUS_PUBLIC_URL", "")
	for _, origin := range []string{"ftp://example.test", "http://example.test/path", "https://user:pass@example.test"} {
		t.Setenv("PEERBONUS_DIRECTORY_URL", origin)
		_, _, _, err := peerbonusHandler("http://127.0.0.1:1", "pj-test")
		require.Error(t, err)
	}
}

func TestHandlersRejectInvalidIdentityCA(t *testing.T) {
	t.Setenv("PLATFORM_OIDC_ISSUER", "https://issuer.example.test")
	t.Setenv("PLATFORM_OIDC_ISSUER_CA_PEM", "invalid PEM")
	peerbonus, inbound, cleanup, err := peerbonusHandler("https://platform.example.test", "pj-test")
	require.EqualError(t, err, "OIDC CA PEM must contain only PEM certificates")
	require.Nil(t, peerbonus)
	require.Nil(t, inbound)
	require.Nil(t, cleanup)
	handler, err := newHandler(t.TempDir(), "https://platform.example.test", "pj-test", nil, nil, nil)
	require.EqualError(t, err, "OIDC CA PEM must contain only PEM certificates")
	require.Nil(t, handler)
}

func TestPendingSurfaceAndRelease(t *testing.T) {
	for _, key := range []string{"PEERBONUS_DATABASE_URL", "DATABASE_URL", "PEERBONUS_PUBLIC_URL", "PEERBONUS_DIRECTORY_URL", "PEERBONUS_SLACK_BOT_TOKEN", "PEERBONUS_SLACK_SIGNING_SECRET", "PEERBONUS_SLACK_API_URL", "PLATFORM_OIDC_ISSUER", "PLATFORM_OIDC_ISSUER_CA_PEM"} {
		t.Setenv(key, "")
	}
	t.Setenv("PEERBONUS_DIRECTORY_PUBLIC_URL", "https://directory.example.test")
	for key, value := range map[string]string{"PEERBONUS_RELEASE_VERSION": "release", "PEERBONUS_SOURCE_REVISION": "", "PEERBONUS_ARTIFACT_DIGEST": "digest"} {
		t.Setenv(key, value)
	}
	peerbonus, inbound, cleanup, err := peerbonusHandler("http://127.0.0.1:1", "pj-test")
	require.NoError(t, err)
	defer cleanup()
	handler, err := newHandler(t.TempDir(), "http://127.0.0.1:1", "pj-test", peerbonus, inbound, peerbonus.Ready)
	require.NoError(t, err)
	for path, status := range map[string]int{"/healthz": 200, "/readyz": 200, "/api/peerbonus/v1/me": 503, "/releasez": 200, "/api/unknown": 404, "/slack/commands": 404} {
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, httptest.NewRequest(http.MethodGet, path, nil))
		require.Equal(t, status, w.Code, path)
		if path == "/releasez" {
			require.JSONEq(t, `{"releaseVersion":"release","artifactDigest":"digest"}`, w.Body.String())
			require.Equal(t, "no-store", w.Header().Get("Cache-Control"))
		}
	}
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/config", nil))
	var config map[string]string
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &config))
	require.Equal(t, map[string]string{"projectId": "pj-test", "publicUrl": "", "directoryUrl": "https://directory.example.test"}, config)
	t.Setenv("PEERBONUS_DATABASE_URL", "postgres://postgres@127.0.0.1:1/peerbonus?sslmode=disable&connect_timeout=1")
	pending, pendingInbound, stop, err := peerbonusHandler("http://127.0.0.1:1", "pj-test")
	require.NoError(t, err)
	defer stop()
	handler, err = newHandler(t.TempDir(), "http://127.0.0.1:1", "pj-test", pending, pendingInbound, pending.Ready)
	require.NoError(t, err)
	for path, status := range map[string]int{"/healthz": 200, "/readyz": 503, "/api/peerbonus/v1/me": 503} {
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, httptest.NewRequest(http.MethodGet, path, nil))
		require.Equal(t, status, w.Code, path)
	}
}

func TestLauncherAndStaticBoundaries(t *testing.T) {
	issuer := testutil.NewOIDCIssuer(t)
	bearer := issuer.Bearer(t)
	t.Setenv("PLATFORM_OIDC_ISSUER", issuer.Server.URL)
	t.Setenv("PLATFORM_OIDC_ISSUER_CA_PEM", issuer.CAPEM)
	t.Setenv("PEERBONUS_OIDC_AUDIENCES", "appthrust-platform-api")
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, bearer, r.Header.Get("Authorization"))
		switch r.URL.Path {
		case "/api/v1/projects/pj-test":
			httpx.WriteJSON(w, 200, map[string]string{"id": "pj-test", "name": "Test Project"})
		case "/api/v1/projects/pj-test/app-installations":
			httpx.WriteJSON(w, 200, map[string]any{"data": []any{map[string]string{"definitionId": "peerbonus", "phase": "Ready", "url": "https://peerbonus.example.test"}}})
		case "/api/v1/application-stack-definitions":
			httpx.WriteJSON(w, 200, map[string]any{"data": []any{map[string]string{"id": "peerbonus", "title": "Peer Bonus"}}})
		default:
			t.Errorf("unexpected platform route %s", r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	defer upstream.Close()
	root := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, "index.html"), []byte("<main>Peer Bonus</main>"), 0600))
	require.NoError(t, os.Mkdir(filepath.Join(root, "assets"), 0700))
	require.NoError(t, os.WriteFile(filepath.Join(root, "assets", "app.js"), []byte("export {}"), 0600))
	handler, err := newHandler(root, upstream.URL, "pj-test", nil, nil, nil)
	require.NoError(t, err)
	request := httptest.NewRequest(http.MethodGet, "/launcher", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, request)
	require.Equal(t, 401, w.Code)
	request.Header.Set("Authorization", bearer)
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, request)
	require.Equal(t, 200, w.Code, w.Body.String())
	var snapshot launcher.Snapshot
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &snapshot))
	require.Equal(t, "Test Project", snapshot.Project.Name)
	require.Len(t, snapshot.Entries, 1)
	require.True(t, snapshot.Entries[0].Current)
	require.Equal(t, "Peer Bonus", snapshot.Entries[0].Name)
	for path, status := range map[string]int{"/": 200, "/members/person": 200, "/assets/app.js": 200, "/assets/missing.js": 404, "/api/unknown": 404} {
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, httptest.NewRequest(http.MethodGet, path, nil))
		require.Equal(t, status, w.Code, path)
		if path == "/assets/app.js" {
			require.Equal(t, "public, max-age=31536000, immutable", w.Header().Get("Cache-Control"))
		}
		if path == "/members/person" {
			require.Equal(t, "<main>Peer Bonus</main>", w.Body.String())
			require.Equal(t, "nosniff", w.Header().Get("X-Content-Type-Options"))
		}
	}
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/", nil))
	require.Equal(t, 405, w.Code)
}

func TestReleaseFileAndUnavailable(t *testing.T) {
	for _, key := range []string{"PLATFORM_OIDC_ISSUER", "PLATFORM_OIDC_ISSUER_CA_PEM", "PEERBONUS_RELEASE_VERSION", "PEERBONUS_SOURCE_REVISION", "PEERBONUS_ARTIFACT_DIGEST", "PEERBONUS_RELEASE_FILE"} {
		t.Setenv(key, "")
	}
	handler, err := newHandler(t.TempDir(), "http://127.0.0.1:1", "pj-test", nil, nil, nil)
	require.NoError(t, err)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/releasez", nil))
	require.Equal(t, 503, w.Code)
	stamp := filepath.Join(t.TempDir(), "release.json")
	require.NoError(t, os.WriteFile(stamp, []byte(`{"sourceRevision":"file-revision"}`), 0600))
	t.Setenv("PEERBONUS_RELEASE_FILE", stamp)
	handler, err = newHandler(t.TempDir(), "http://127.0.0.1:1", "pj-test", nil, nil, nil)
	require.NoError(t, err)
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/releasez", nil))
	require.Equal(t, 200, w.Code)
	require.JSONEq(t, `{"sourceRevision":"file-revision"}`, w.Body.String())
	require.Equal(t, "no-store", w.Header().Get("Cache-Control"))
}

func TestSlackInboundMountBypassesAPIAndSPA(t *testing.T) {
	t.Setenv("PLATFORM_OIDC_ISSUER", "")
	t.Setenv("PLATFORM_OIDC_ISSUER_CA_PEM", "")
	inbound := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/slack/commands", r.URL.Path)
		require.Empty(t, r.Header.Get("Authorization"))
		w.WriteHeader(http.StatusAccepted)
	})
	handler, err := newHandler(t.TempDir(), "http://127.0.0.1:1", "pj-test", nil, inbound, nil)
	require.NoError(t, err)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/slack/commands", nil))
	require.Equal(t, http.StatusAccepted, w.Code)
}

func TestSlackAPIURLValidationBeforeDatabase(t *testing.T) {
	for _, key := range []string{"PEERBONUS_DATABASE_URL", "DATABASE_URL", "PEERBONUS_PUBLIC_URL", "PEERBONUS_DIRECTORY_URL", "PEERBONUS_SLACK_BOT_TOKEN", "PEERBONUS_SLACK_SIGNING_SECRET", "PLATFORM_OIDC_ISSUER", "PLATFORM_OIDC_ISSUER_CA_PEM"} {
		t.Setenv(key, "")
	}
	for _, raw := range []string{"https://slack.com/api", "https://example.test", "http://10.0.0.1", "ftp://localhost", "http://localhost/path", "http://user:secret@localhost", "http://localhost?x=1", "http://localhost#", "http://localhost#fragment", "http://localhost/"} {
		t.Run(raw, func(t *testing.T) {
			t.Setenv("PEERBONUS_SLACK_API_URL", raw)
			api, inbound, stop, err := peerbonusHandler("http://127.0.0.1:1", "pj-test")
			require.EqualError(t, err, "PEERBONUS_SLACK_API_URL must be a loopback HTTP or HTTPS origin without a path, query, or fragment")
			require.Nil(t, api)
			require.Nil(t, inbound)
			require.Nil(t, stop)
		})
	}
	for _, raw := range []string{"", "http://localhost:8080", "http://127.0.0.1:8080", "https://[::1]:8443"} {
		t.Run(raw, func(t *testing.T) {
			t.Setenv("PEERBONUS_SLACK_API_URL", raw)
			t.Setenv("PEERBONUS_SLACK_BOT_TOKEN", "test-bot-token")
			api, inbound, stop, err := peerbonusHandler("http://127.0.0.1:1", "pj-test")
			require.NoError(t, err)
			defer stop()
			require.NotNil(t, api)
			w := httptest.NewRecorder()
			inbound.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/slack/commands", nil))
			require.Equal(t, http.StatusNotFound, w.Code)
		})
	}
}
