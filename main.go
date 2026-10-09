package main

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/pem"
	"errors"
	"log"
	"os"
	"os/signal"
	"sync/atomic"
	"syscall"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/ssh"
	"github.com/charmbracelet/wish"
	"github.com/charmbracelet/wish/activeterm"
	"github.com/charmbracelet/wish/bubbletea"
	"github.com/charmbracelet/wish/logging"
	"github.com/charmbracelet/wish/ratelimiter"
	gossh "golang.org/x/crypto/ssh"
	"golang.org/x/time/rate"
)

func env(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

const maxSessions = 50

func main() {
	addr := env("LISTEN_ADDR", ":2222")
	hostKey := env("HOST_KEY_PATH", ".ssh/id_ed25519")
	site := env("SITE_URL", "https://miro.zip")
	// Off unless set, so local testing doesn't bump the real counters.
	var cnt *counters
	if api := os.Getenv("COUNTERS_URL"); api != "" {
		cnt = newCounters(api)
	}

	store := newStore(site)
	go store.refreshLoop(10 * time.Minute)

	var active atomic.Int32

	// The key is read directly rather than with WithHostKeyPath, which
	// chmods the key's folder and fails on read-only or shared folders.
	// Without a key file (local testing) a throwaway key is made in memory.
	keyPEM, err := os.ReadFile(hostKey)
	if err != nil {
		log.Println("no host key at", hostKey, "- using a temporary one")
		if keyPEM, err = tempHostKey(); err != nil {
			log.Fatalln("could not create host key:", err)
		}
	}

	s, err := wish.NewServer(
		wish.WithAddress(addr),
		wish.WithHostKeyPEM(keyPEM),
		wish.WithIdleTimeout(10*time.Minute),
		wish.WithMaxTimeout(time.Hour),
		// Middlewares run last to first: logging sees every connection,
		// the rate limiter and session cap run before any work is done,
		// and the TUI only starts for a real interactive terminal.
		wish.WithMiddleware(
			bubbletea.Middleware(func(s ssh.Session) (tea.Model, []tea.ProgramOption) {
				pty, _, _ := s.Pty()
				return newModel(store.get(), cnt, s, pty.Window.Width, pty.Window.Height, bubbletea.MakeRenderer(s)),
					[]tea.ProgramOption{tea.WithAltScreen()}
			}),
			rejectCommands,
			activeterm.Middleware(),
			limitSessions(&active),
			ratelimiter.Middleware(ratelimiter.NewRateLimiter(rate.Every(time.Second), 10, 1000)),
			logging.Middleware(),
		),
	)
	if err != nil {
		log.Fatalln("could not create server:", err)
	}

	done := make(chan os.Signal, 1)
	signal.Notify(done, os.Interrupt, syscall.SIGTERM)
	log.Println("listening on", addr)
	go func() {
		if err := s.ListenAndServe(); err != nil && !errors.Is(err, ssh.ErrServerClosed) {
			log.Fatalln(err)
		}
	}()

	<-done
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := s.Shutdown(ctx); err != nil && !errors.Is(err, ssh.ErrServerClosed) {
		log.Fatalln(err)
	}
}

func tempHostKey() ([]byte, error) {
	_, key, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, err
	}
	block, err := gossh.MarshalPrivateKey(key, "")
	if err != nil {
		return nil, err
	}
	return pem.EncodeToMemory(block), nil
}

// rejectCommands refuses "ssh miro.zip <command>" so nothing but the TUI runs.
func rejectCommands(next ssh.Handler) ssh.Handler {
	return func(s ssh.Session) {
		if len(s.Command()) > 0 {
			wish.Fatalln(s, "No commands here, just run: ssh miro.zip")
			return
		}
		next(s)
	}
}

// limitSessions caps concurrent sessions so the Pi can't be swamped.
func limitSessions(active *atomic.Int32) wish.Middleware {
	return func(next ssh.Handler) ssh.Handler {
		return func(s ssh.Session) {
			if active.Add(1) > maxSessions {
				active.Add(-1)
				wish.Fatalln(s, "Too many visitors right now, try again in a bit.")
				return
			}
			defer active.Add(-1)
			next(s)
		}
	}
}
