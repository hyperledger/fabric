/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package chaincode

import (
	"strconv"
	"time"

	"github.com/hyperledger/fabric/core/chaincode/accesscontrol"
	"github.com/hyperledger/fabric/core/container/ccintf"
	"github.com/pkg/errors"
)

// LaunchRegistry tracks launching chaincode instances.
type LaunchRegistry interface {
	Launching(ccid string) (launchState *LaunchState, started bool)
	Deregister(ccid string) error
}

// RuntimeLauncher is responsible for launching chaincode runtimes.
type RuntimeLauncher struct {
	Runtime        Runtime
	Registry       LaunchRegistry
	StartupTimeout time.Duration
	Metrics        *LaunchMetrics
	PeerAddress    string
	CACert         []byte
	CertGenerator  CertGenerator
}

// CertGenerator generates client certificates for chaincode.
type CertGenerator interface {
	// Generate returns a certificate and private key and associates
	// the hash of the certificates with the given chaincode name
	Generate(ccName string) (*accesscontrol.CertAndPrivKeyPair, error)
}

func (r *RuntimeLauncher) ChaincodeClientInfo(ccid string) (*ccintf.PeerConnection, error) {
	var tlsConfig *ccintf.TLSConfig
	if r.CertGenerator != nil {
		certKeyPair, err := r.CertGenerator.Generate(string(ccid))
		if err != nil {
			return nil, errors.WithMessagef(err, "failed to generate TLS certificates for %s", ccid)
		}

		tlsConfig = &ccintf.TLSConfig{
			ClientCert: certKeyPair.Cert,
			ClientKey:  certKeyPair.Key,
			RootCert:   r.CACert,
		}
	}

	return &ccintf.PeerConnection{
		Address:   r.PeerAddress,
		TLSConfig: tlsConfig,
	}, nil
}

func (r *RuntimeLauncher) Launch(ccid string) error {
	var startFailCh chan error
	var timeoutCh <-chan time.Time

	startTime := time.Now()
	launchState, alreadyStarted := r.Registry.Launching(ccid)
	if !alreadyStarted {
		startFailCh = make(chan error, 1)
		timeoutCh = time.NewTimer(r.StartupTimeout).C

		go func() {
			var ccservinfo *ccintf.ChaincodeServerInfo
			ccservinfo, err := r.Runtime.Build(ccid)
			if err != nil {
				startFailCh <- errors.WithMessage(err, "error building chaincode")
				return
			}
			if ccservinfo != nil {
				startFailCh <- errors.New("peer as client to be implemented")
				return
			}
			ccinfo, err := r.ChaincodeClientInfo(ccid)
			if err != nil {
				startFailCh <- errors.WithMessage(err, "could not get connection info")
				return
			}
			if ccinfo == nil {
				startFailCh <- errors.New("could not get connection info")
				return
			}
			if err = r.Runtime.Start(ccid, ccinfo); err != nil {
				startFailCh <- errors.WithMessage(err, "error starting container")
				return
			}
			exitCode, err := r.Runtime.Wait(ccid)
			if err != nil {
				launchState.Notify(errors.Wrap(err, "failed to wait on container exit"))
			}
			launchState.Notify(errors.Errorf("container exited with %d", exitCode))
		}()
	}

	var err error
	select {
	case <-launchState.Done():
		err = errors.WithMessage(launchState.Err(), "chaincode registration failed")
	case err = <-startFailCh:
		launchState.Notify(err)
		r.Metrics.LaunchFailures.With("chaincode", ccid).Add(1)
	case <-timeoutCh:
		err = errors.Errorf("timeout expired while starting chaincode %s for transaction", ccid)
		launchState.Notify(err)
		r.Metrics.LaunchTimeouts.With("chaincode", ccid).Add(1)
	}

	success := true
	if err != nil && !alreadyStarted {
		success = false
		chaincodeLogger.Debugf("stopping due to error while launching: %+v", err)
		defer r.Registry.Deregister(ccid)
		if err := r.Runtime.Stop(ccid); err != nil {
			chaincodeLogger.Debugf("stop failed: %+v", err)
		}
	}

	r.Metrics.LaunchDuration.With(
		"chaincode", ccid,
		"success", strconv.FormatBool(success),
	).Observe(time.Since(startTime).Seconds())

	chaincodeLogger.Debug("launch complete")
	return err
}
