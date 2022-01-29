/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package externalbuilder_test

import (
	"io"
	"os/exec"
	"syscall"

	"github.com/hyperledger/fabric/common/flogging"
	"github.com/hyperledger/fabric/core/container/externalbuilder"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gbytes"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

var _ = Describe("Session", func() {
	var (
		logbuf *gbytes.Buffer
		logger *flogging.FabricLogger
	)

	BeforeEach(func() {
		logbuf = gbytes.NewBuffer()
		writer := io.MultiWriter(logbuf, GinkgoWriter)
		enc := zapcore.NewConsoleEncoder(zapcore.EncoderConfig{MessageKey: "msg"})
		core := zapcore.NewCore(enc, zapcore.AddSync(writer), zap.NewAtomicLevel())
		logger = flogging.NewFabricLogger(zap.New(core).Named("logger"))
	})

	It("starts commands and returns a session handle to wait on", func() {
		cmd := exec.Command("true")
		sess, err := externalbuilder.Start(logger, cmd)
		Expect(err).NotTo(HaveOccurred())
		Expect(sess).NotTo(BeNil())

		err = sess.Wait()
		Expect(err).NotTo(HaveOccurred())
	})

	It("captures stderr to the provided logger", func() {
		cmd := exec.Command("sh", "-c", "echo 'this is a message to stderr' >&2")
		sess, err := externalbuilder.Start(logger, cmd)
		Expect(err).NotTo(HaveOccurred())
		err = sess.Wait()
		Expect(err).NotTo(HaveOccurred())

		Expect(logbuf).To(gbytes.Say("this is a message to stderr"))
	})

	It("delivers signals to started commands", func() {
		cmd := exec.Command("cat")
		stdin, err := cmd.StdinPipe()
		Expect(err).NotTo(HaveOccurred())
		defer stdin.Close()

		sess, err := externalbuilder.Start(logger, cmd)
		Expect(err).NotTo(HaveOccurred())

		exitCh := make(chan error)
		go func() { exitCh <- sess.Wait() }()

		Consistently(exitCh).ShouldNot(Receive())
		sess.Signal(syscall.SIGTERM)
		Eventually(exitCh).Should(Receive(MatchError("signal: terminated")))
	})

	It("calls exit functions", func() {
		var called1, called2 bool
		var exitErr1, exitErr2 error

		cmd := exec.Command("true")
		sess, err := externalbuilder.Start(logger, cmd, func(err error) {
			called1 = true
			exitErr1 = err
		}, func(err error) {
			called2 = true
			exitErr2 = err
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(sess).NotTo(BeNil())

		err = sess.Wait()
		Expect(err).NotTo(HaveOccurred())

		Expect(called1).To(BeTrue())
		Expect(exitErr1).NotTo(HaveOccurred())
		Expect(called2).To(BeTrue())
		Expect(exitErr2).NotTo(HaveOccurred())
	})

	When("start fails", func() {
		It("returns an error", func() {
			cmd := exec.Command("./this-is-not-a-command")
			_, err := externalbuilder.Start(logger, cmd)
			Expect(err).To(MatchError("fork/exec ./this-is-not-a-command: no such file or directory"))
		})
	})

	When("the command fails", func() {
		It("returns the exit error from the command", func() {
			cmd := exec.Command("false")
			sess, err := externalbuilder.Start(logger, cmd)
			Expect(err).NotTo(HaveOccurred())

			err = sess.Wait()
			Expect(err).To(MatchError("exit status 1"))
			Expect(err).To(BeAssignableToTypeOf(&exec.ExitError{}))
		})

		It("passes the error to the exit function", func() {
			var exitErr error
			cmd := exec.Command("false")
			sess, err := externalbuilder.Start(logger, cmd, func(err error) {
				exitErr = err
			})
			Expect(err).NotTo(HaveOccurred())

			err = sess.Wait()
			Expect(err).To(MatchError("exit status 1"))
			Expect(exitErr).To(Equal(err))
		})
	})
})
