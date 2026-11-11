package main

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/appthrust/appserver-go/appserver/identity"
	"github.com/appthrust/appserver-go/appserver/link"
	"github.com/appthrust/appserver-go/appserver/pending"
	"github.com/appthrust/appserver-go/appserver/platformclient"
	"github.com/appthrust/peerbonus/cmd/server/internal/api"
	"github.com/appthrust/peerbonus/cmd/server/internal/notify"
	"github.com/appthrust/peerbonus/cmd/server/internal/slack"
	"github.com/appthrust/peerbonus/cmd/server/internal/slackinbound"
	"github.com/appthrust/peerbonus/cmd/server/internal/store"
)

const storeOpenTimeout = 30 * time.Second

// storeRetryInitial is the first wait after a failed database connection.
// Tests shorten it.
var storeRetryInitial = time.Second

func peerbonusHandler(upstreamURL, projectID string) (*pending.Handler, *pending.Handler, func(), error) {
	auth, err := identity.New(identity.FromEnv("PEERBONUS"))
	if err != nil {
		return nil, nil, nil, err
	}
	publicURL := os.Getenv("PEERBONUS_PUBLIC_URL")
	if publicURL != "" {
		origin, err := parseOrigin(publicURL)
		if err != nil || origin.Scheme != "https" || origin.Path != "" || strings.Contains(publicURL, "#") {
			return nil, nil, nil, errors.New("PEERBONUS_PUBLIC_URL must be an HTTPS origin without a path, query, or fragment")
		}
	}
	if raw := os.Getenv("PEERBONUS_DIRECTORY_URL"); raw != "" {
		origin, err := parseOrigin(raw)
		if err != nil || (origin.Scheme != "http" && origin.Scheme != "https") {
			return nil, nil, nil, errors.New("PEERBONUS_DIRECTORY_URL must be an HTTP or HTTPS origin")
		}
	}
	token := os.Getenv("PEERBONUS_SLACK_BOT_TOKEN")
	secret := os.Getenv("PEERBONUS_SLACK_SIGNING_SECRET")
	slackAPIURL := os.Getenv("PEERBONUS_SLACK_API_URL")
	var slackOptions []slack.Option
	if slackAPIURL != "" {
		origin, err := parseOrigin(slackAPIURL)
		if err != nil || (origin.Scheme != "http" && origin.Scheme != "https") || origin.Path != "" || strings.Contains(slackAPIURL, "#") || (origin.Hostname() != "localhost" && !net.ParseIP(origin.Hostname()).IsLoopback()) {
			return nil, nil, nil, errors.New("PEERBONUS_SLACK_API_URL must be a loopback HTTP or HTTPS origin without a path, query, or fragment")
		}
		slackOptions = append(slackOptions, slack.WithBaseURL(slackAPIURL))
	}
	var client *slack.Client
	if token != "" || slackAPIURL != "" {
		candidate, err := slack.New(token, slackOptions...)
		if err != nil {
			return nil, nil, nil, err
		}
		// Validate overrides even without a token, but leave Slack unconfigured.
		if token != "" {
			client = candidate
		}
	}
	platform := platformclient.New(upstreamURL, projectID, platformclient.WithHTTPClient(auth.HTTPClient()))
	links := link.NewClient(link.NewResolver(platform, "peerbonus", os.LookupEnv, map[string]link.Required{"directory": {URLEnv: "PEERBONUS_DIRECTORY_URL", PublicURLEnv: "PEERBONUS_DIRECTORY_PUBLIC_URL"}}), nil)
	peerbonus := pending.Serve(api.Unavailable())
	inbound := pending.Serve(http.NotFoundHandler())
	dsn := os.Getenv("PEERBONUS_DATABASE_URL")
	if dsn == "" {
		dsn = os.Getenv("DATABASE_URL")
	}
	if dsn == "" {
		// Nothing to wait for: the API reports the missing configuration itself.
		peerbonus.Swap(api.Unavailable())
		return peerbonus, inbound, func() {}, nil
	}
	seed := os.Getenv("PEERBONUS_DEV_SEED") == "1"
	directoryPublicURL := os.Getenv("PEERBONUS_DIRECTORY_PUBLIC_URL")
	stop := pending.Connect("peerbonus database", storeRetryInitial, func(ctx context.Context) (io.Closer, error) {
		openCtx, cancel := context.WithTimeout(ctx, storeOpenTimeout)
		defer cancel()
		database, err := store.Open(openCtx, dsn)
		if err != nil {
			return nil, err
		}
		if seed {
			if err := database.SeedDev(openCtx, time.Now().UTC()); err != nil {
				_ = database.Close()
				return nil, err
			}
		}
		var closer io.Closer = database
		if client != nil {
			workerDone := make(chan struct{})
			worker := notify.New(notify.Options{Store: database, Slack: client, PublicURL: publicURL, Now: time.Now, Logger: slog.Default()})
			go func() {
				defer close(workerDone)
				_ = worker.Run(ctx)
			}()
			inbound.Swap(slackinbound.New(slackinbound.Options{Store: database, Slack: client, SigningSecret: secret, PublicURL: publicURL, Now: time.Now}))
			closer = &peerbonusStore{Store: database, workerDone: workerDone}
		}
		peerbonus.Swap(api.New(database, auth, platform, links, directoryPublicURL, api.SlackOptions{Client: client, SigningSecretConfigured: secret != "", PlatformAPIURL: upstreamURL}))
		return closer, nil
	})
	return peerbonus, inbound, stop, nil
}

// pending.Connect cancels the worker context before closing the store.
type peerbonusStore struct {
	*store.Store
	workerDone <-chan struct{}
}

func (s *peerbonusStore) Close() error {
	<-s.workerDone
	return s.Store.Close()
}
