package main

import (
	"context"
	"log"
	"net/http"
	"os/signal"
	"syscall"

	"another-chi-router/handler"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)

	err := run(ctx, cancel)
	if err != nil {
		log.Fatal("service failed start")
	}

	<-ctx.Done()
}

func run(ctx context.Context, cancel context.CancelFunc) (err error) {
	defer cancel()

	errChan := make(chan error)
	go func() {
		err = <-errChan
		log.Println("service failed start")
		cancel()
	}()

	api := handler.New()
	ws := handler.NewWebsocket()

	middlewares := chi.Middlewares{
		middleware.Logger,
		middleware.CleanPath,
		middleware.RequestID,
		middleware.RealIP,
		middleware.Recoverer,
	}

	go func() {
		r := chi.NewRouter()
		r.Use(middlewares...)
		r.Mount("/api", api.Routes())

		log.Println("server started on :8080")
		if serveErr := http.ListenAndServe(":8080", r); serveErr != nil {
			errChan <- serveErr
		}
	}()

	go func() {
		r := chi.NewRouter()
		r.Use(middlewares...)
		r.Mount("/ws", ws.Routes())

		log.Println("websocket server started on 127.0.0.1:8090")
		if serveErr := http.ListenAndServe("127.0.0.1:8090", r); serveErr != nil {
			errChan <- serveErr
		}
	}()
	<-ctx.Done()

	return err
}
