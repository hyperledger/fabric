/*
Copyright IBM Corp All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package fabhttp

import (
	"context"
	"crypto/tls"
	"net"
	"net/http"
	"os"
	"time"

	"github.com/hyperledger/fabric/common/flogging"
	"github.com/hyperledger/fabric/common/util"
	"github.com/hyperledger/fabric/core/middleware"
)

//go:generate counterfeiter -o fakes/logger.go -fake-name Logger . Logger

type Logger interface {
	Warn(args ...interface{})
	Warnf(template string, args ...interface{})
}

type Options struct {
	Logger        Logger
	ListenAddress string
	TLS           TLS
}

type Server struct {
	logger     Logger
	options    Options
	httpServer *http.Server
	mux        *http.ServeMux
	addr       string
}

func NewServer(o Options) *Server {
	logger := o.Logger
	if logger == nil {
		logger = flogging.MustGetLogger("fabhttp")
	}

	server := &Server{
		logger:  logger,
		options: o,
	}

	server.initializeServer()

	return server
}

func (s *Server) Run(signals <-chan os.Signal, ready chan<- struct{}) error {
	err := s.Start()
	if err != nil {
		return err
	}

	close(ready)

	<-signals
	return s.Stop()
}

func (s *Server) Start() error {
	listener, err := s.Listen()
	if err != nil {
		return err
	}
	s.addr = listener.Addr().String()

	go s.httpServer.Serve(listener)

	return nil
}

func (s *Server) Stop() error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	return s.httpServer.Shutdown(ctx)
}

func (s *Server) initializeServer() {
	s.mux = http.NewServeMux()
	s.httpServer = &http.Server{
		Addr:         s.options.ListenAddress,
		Handler:      s.mux,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 2 * time.Minute,
	}
}

func (s *Server) HandlerChain(h http.Handler, secure bool) http.Handler {
	if secure {
		return middleware.NewChain(middleware.RequireCert(), middleware.WithRequestID(util.GenerateUUID)).Handler(h)
	}
	return middleware.NewChain(middleware.WithRequestID(util.GenerateUUID)).Handler(h)
}

// RegisterHandler registers into the ServeMux a handler chain that borrows
// its security properties from the fabhttp.Server. This method is thread
// safe because ServeMux.Handle() is thread safe, and options are immutable.
// This method can be called either before or after Server.Start(). If the
// pattern exists the method panics.
func (s *Server) RegisterHandler(pattern string, handler http.Handler, secure bool) {
	s.mux.Handle(
		pattern,
		s.HandlerChain(
			handler,
			secure,
		),
	)
}

func (s *Server) Listen() (net.Listener, error) {
	listener, err := net.Listen("tcp", s.options.ListenAddress)
	if err != nil {
		return nil, err
	}
	tlsConfig, err := s.options.TLS.Config()
	if err != nil {
		return nil, err
	}
	if tlsConfig != nil {
		listener = tls.NewListener(listener, tlsConfig)
	}
	return listener, nil
}

func (s *Server) Addr() string {
	return s.addr
}

func (s *Server) Log(keyvals ...interface{}) error {
	s.logger.Warn(keyvals...)
	return nil
}
