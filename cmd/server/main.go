package main

import (
	"context"
	"errors"
	"flag"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/appthrust/appserver-go/appserver/platformenv"
	"github.com/appthrust/appserver-go/appserver/requestlog"
)

func main() {
	listen := flag.String("listen", ":8080", "HTTP listen address")
	defaultStaticDir := os.Getenv("PEERBONUS_STATIC_DIR")
	if defaultStaticDir == "" {
		defaultStaticDir = "/srv/peerbonus"
	}
	staticDir := flag.String("static-dir", defaultStaticDir, "static SPA peerbonus")
	flag.Parse()
	if flag.NArg() != 0 {
		log.Fatal("usage: peerbonus-server [--listen :8080] [--static-dir /srv/peerbonus]")
	}
	// Validate platform configuration before opening the database.
	platform, err := platformenv.FromEnv()
	if err != nil {
		log.Fatal(err)
	}
	if err := platformenv.ValidateIdentityListen(*listen, platform.OIDCIssuer); err != nil {
		log.Fatal(err)
	}
	upstreamURL, projectID := platform.APIURL, platform.ProjectID
	peerbonus, inbound, closePeerbonus, err := peerbonusHandler(upstreamURL, projectID)
	if err != nil {
		log.Fatal(err)
	}
	defer closePeerbonus()
	handler, err := newHandler(*staticDir, upstreamURL, projectID, peerbonus, inbound, peerbonus.Ready)
	if err != nil {
		log.Fatal(err)
	}
	server := newServer(*listen, handler, os.Stderr)
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	go func() {
		<-ctx.Done()
		shutdown, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdown); err != nil {
			log.Printf("shutdown: %v", err)
		}
	}()
	log.Printf("peerbonus-server listening on %s", *listen)
	if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatal(err)
	}
}

func newServer(addr string, handler http.Handler, logOutput io.Writer) *http.Server {
	return &http.Server{
		Addr:              addr,
		Handler:           requestlog.Wrap(handler, requestlog.Options{Service: "peerbonus", Output: logOutput}),
		ReadHeaderTimeout: 10 * time.Second,
	}
}
