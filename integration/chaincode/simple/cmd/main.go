/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package main

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/hyperledger/fabric-chaincode-go/shim"
	"github.com/hyperledger/fabric/integration/chaincode/simple"
)

func handleSignals(handlers map[os.Signal]func()) {
	var signals []os.Signal
	for sig := range handlers {
		signals = append(signals, sig)
	}

	signalChan := make(chan os.Signal, 1)
	signal.Notify(signalChan, signals...)

	for sig := range signalChan {
		fmt.Fprintf(os.Stderr, "Received signal: %d (%s)", sig, sig)
		handlers[sig]()
	}
}

func handleSigTerm() {
	termFile := os.Getenv("ORG1_TERM_FILE")
	if termFile == "" {
		termFile = os.Getenv("ORG2_TERM_FILE")
	}
	if termFile == "" {
		return
	}

	err := os.WriteFile(termFile, []byte("term-file"), 0o644)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to write term file to %s: %s", termFile, err)
	}
}

func main() {
	startErrChan := make(chan error)
	sigtermChan := make(chan error)

	go handleSignals(map[os.Signal]func(){
		syscall.SIGTERM: func() { sigtermChan <- nil },
	})

	go func() {
		err := shim.Start(&simple.SimpleChaincode{})
		if err != nil {
			startErrChan <- err
		}
	}()

	if os.Getenv("DEVMODE_ENABLED") != "" {
		fmt.Println("starting up in devmode...")
	}

	var err error
	select {
	case err = <-startErrChan:
		if err != nil {
			fmt.Fprintf(os.Stderr, "Exiting Simple chaincode: %s", err)
			os.Exit(2)
		}
		os.Exit(0)
	case <-sigtermChan:
		handleSigTerm()
	}
}
