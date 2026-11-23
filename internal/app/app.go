package app

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"
)

func Env(k, d string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return d
}
func Token() string {
	b, e := os.ReadFile(Env("TOKEN_FILE", "data/internal-token"))
	if e != nil {
		log.Fatal(e)
	}
	v := strings.TrimSpace(string(b))
	if len(v) < 32 {
		log.Fatal("internal token must contain at least 32 characters")
	}
	return v
}
func Context() (context.Context, context.CancelFunc) {
	return signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
}
func Serve(ctx context.Context, servers ...*http.Server) error {
	errc := make(chan error, len(servers))
	for _, s := range servers {
		go func(s *http.Server) { errc <- s.ListenAndServe() }(s)
	}
	var err error
	select {
	case <-ctx.Done():
	case err = <-errc:
	}
	stop, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	for _, s := range servers {
		if e := s.Shutdown(stop); e != nil {
			s.Close()
		}
	}
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}
func Server(addr string, h http.Handler) *http.Server {
	return &http.Server{Addr: addr, Handler: h, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 10 * time.Second, WriteTimeout: 15 * time.Second, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 16 << 10}
}
