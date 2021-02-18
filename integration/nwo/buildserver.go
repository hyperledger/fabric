/*
Copyright IBM Corp All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package nwo

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gexec"
)

type BuildServer struct {
	server *http.Server
	lis    net.Listener
}

func NewBuildServer(args ...string) *BuildServer {
	return &BuildServer{
		server: &http.Server{
			Handler: &buildHandler{args: args},
		},
	}
}

func (s *BuildServer) Serve() {
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	Expect(err).NotTo(HaveOccurred())

	s.lis = lis
	go s.server.Serve(lis)
}

func (s *BuildServer) Shutdown() {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	defer gexec.CleanupBuildArtifacts()

	s.server.Shutdown(ctx)
}

func (s *BuildServer) Components() *Components {
	Expect(s.lis).NotTo(BeNil())
	return &Components{
		ServerAddress: s.lis.Addr().String(),
	}
}

type artifact struct {
	mutex  sync.Mutex
	input  string
	output string
}

func (a *artifact) build(args ...string) error {
	a.mutex.Lock()
	defer a.mutex.Unlock()

	if a.output != "" {
		return nil
	}

	output, err := gexec.Build(a.input, args...)
	if err != nil {
		return err
	}

	a.output = output
	return nil
}

type buildHandler struct {
	mutex     sync.Mutex
	artifacts map[string]*artifact
	args      []string
}

func (b *buildHandler) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	if req.Method != http.MethodGet {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	input := strings.TrimPrefix(req.URL.Path, "/")
	a := b.artifact(input)

	if err := a.build(b.args...); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprintf(w, "%s", err)
		return
	}

	w.WriteHeader(http.StatusOK)
	fmt.Fprintf(w, "%s", a.output)
}

func (b *buildHandler) artifact(input string) *artifact {
	b.mutex.Lock()
	defer b.mutex.Unlock()

	if b.artifacts == nil {
		b.artifacts = map[string]*artifact{}
	}

	a, ok := b.artifacts[input]
	if !ok {
		a = &artifact{input: input}
		b.artifacts[input] = a
	}

	return a
}
