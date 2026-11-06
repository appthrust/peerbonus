// fakeslack runs the same Slack fake used by package tests. For example:
//
//	go run ./cmd/server/internal/slack/slacktest/cmd/fakeslack \
//	  --listen 127.0.0.1:0 --user alice@example.com=U1 --channel C1=thanks:member
//
// It prints URL=<origin> once. GET /_calls returns recorded calls, persistent
// messages (including updates/deletions), and opened views, with auth redacted.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/appthrust/peerbonus/cmd/server/internal/slack/slacktest"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := run(ctx, os.Args[1:], os.Stdout, os.Stderr); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	fake := slacktest.NewHandler()
	flags := flag.NewFlagSet("fakeslack", flag.ContinueOnError)
	flags.SetOutput(stderr)
	listen := flags.String("listen", "127.0.0.1:0", "HTTP listen address")
	flags.Func("user", "email=UID (repeatable)", func(raw string) error {
		email, id, ok := strings.Cut(raw, "=")
		if !ok || email == "" || id == "" {
			return errors.New("user must be email=UID")
		}
		fake.AddUser(email, id)
		return nil
	})
	flags.Func("channel", "id=name[:member] (repeatable; member, true, or false)", func(raw string) error {
		id, name, ok := strings.Cut(raw, "=")
		if !ok || id == "" || name == "" {
			return errors.New("channel must be id=name[:member]")
		}
		name, membership, hasMembership := strings.Cut(name, ":")
		if name == "" {
			return errors.New("channel name must not be empty")
		}
		member := false
		if hasMembership {
			if membership == "member" {
				member = true
			} else {
				var err error
				member, err = strconv.ParseBool(membership)
				if err != nil {
					return errors.New("channel membership must be member, true, or false")
				}
			}
		}
		fake.AddChannel(id, name, member)
		return nil
	})
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	if flags.NArg() != 0 {
		return errors.New("fakeslack does not accept positional arguments")
	}
	listener, err := net.Listen("tcp", *listen)
	if err != nil {
		return err
	}
	server := &http.Server{Handler: fake, ReadHeaderTimeout: 5 * time.Second}
	defer server.Close()
	if _, err := fmt.Fprintf(stdout, "URL=http://%s\n", listener.Addr()); err != nil {
		_ = listener.Close()
		return err
	}
	finished := make(chan struct{})
	defer close(finished)
	go func() {
		select {
		case <-ctx.Done():
			shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			_ = server.Shutdown(shutdown)
		case <-finished:
		}
	}()
	if err := server.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}
