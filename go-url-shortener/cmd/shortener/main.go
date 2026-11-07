// Command shortener runs the URL shortener HTTP server.
package main

import (
	"context"
	"errors"
	"flag"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/nidhalom/go-url-shortener/internal/ratelimit"
	"github.com/nidhalom/go-url-shortener/internal/server"
	"github.com/nidhalom/go-url-shortener/internal/store"
)

func main() {
	addr := flag.String("addr", ":8080", "listen address")
	base := flag.String("base-url", "http://localhost:8080", "public base URL used in short_url")
	dataFile := flag.String("data", "", "JSON file for persistence (empty = in-memory only)")
	rate := flag.Float64("rate", 5, "allowed requests per second per IP")
	burst := flag.Int("burst", 20, "burst size per IP")
	flag.Parse()

	var st store.Store
	if *dataFile != "" {
		fs, err := store.NewJSONFile(*dataFile)
		if err != nil {
			log.Fatal(err)
		}
		st = fs
	} else {
		st = store.NewMemory()
	}

	srv := &http.Server{
		Addr:              *addr,
		Handler:           server.New(server.Config{Store: st, Limiter: ratelimit.New(*rate, *burst, nil), BaseURL: *base}),
		ReadHeaderTimeout: 5 * time.Second,
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	go func() {
		<-ctx.Done()
		shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		srv.Shutdown(shutdown)
	}()

	log.Printf("listening on %s", *addr)
	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatal(err)
	}
}
