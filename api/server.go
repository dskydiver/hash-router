package api

import (
	"context"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"gitlab.com/TitanInd/hashrouter/interfaces"
)

type Server struct {
	server http.Server

	address         string
	shutdownTimeout time.Duration

	log interfaces.ILogger
}

func NewServer(address string, log interfaces.ILogger, router *gin.Engine) *Server {
	// mux.HandleFunc("/connections", connectionsController.ServeHTTP)
	// mux.HandleFunc("/change-dest", func(w http.ResponseWriter, r *http.Request) {
	// host := r.URL.Query().Get("host")
	// user := r.URL.Query().Get("user")
	// pwd := r.URL.Query().Get("pwd")
	// minerController.ChangeDestAll(host, user, pwd)
	// 	w.Write([]byte("success"))
	// })

	server := http.Server{Addr: address, Handler: router}

	return &Server{
		address:         address,
		server:          server,
		shutdownTimeout: 5 * time.Second,
		log:             log,
	}
}

func (s *Server) Run(ctx context.Context) error {
	return s.listenAndServe(ctx)
}

func (s *Server) listenAndServe(ctx context.Context) error {
	serverErr := make(chan error, 1)
	go func() {
		// Capture ListenAndServe errors such as "port already in use".
		// However, when a server is gracefully shutdown, it is safe to ignore errors
		// returned from this method (given the select logic below), because
		// Shutdown causes ListenAndServe to always return http.ErrServerClosed.
		if err := s.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			s.log.Error("http server error", err)
			serverErr <- err
		}
	}()

	s.log.Infof("http server is listening: %s", s.server.Addr)

	var err error
	select {
	case <-ctx.Done():
		ctx, cancel := context.WithTimeout(ctx, s.shutdownTimeout)
		defer cancel()
		err = s.server.Shutdown(ctx)
		s.log.Warn("http server closed")
	case err = <-serverErr:
	}

	return err
}
