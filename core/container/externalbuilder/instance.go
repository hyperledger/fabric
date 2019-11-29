/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package externalbuilder

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"time"

	"github.com/hyperledger/fabric/core/comm"
	"github.com/hyperledger/fabric/core/container/ccintf"
	"github.com/pkg/errors"
)

const (
	DialTimeout        = 3 * time.Second
	CCServerReleaseDir = "chaincode/server"
)

type Instance struct {
	PackageID   string
	BldDir      string
	ReleaseDir  string
	Builder     *Builder
	Session     *Session
	TermTimeout time.Duration
}

// Duration used for the DialTimeout property
type Duration struct {
	time.Duration
}

func (d Duration) MarshalJSON() ([]byte, error) {
	return json.Marshal(d.Seconds())
}

func (d *Duration) UnmarshalJSON(b []byte) error {
	var v interface{}
	if err := json.Unmarshal(b, &v); err != nil {
		return err
	}

	switch value := v.(type) {
	case float64:
		d.Duration = time.Duration(value)
		return nil
	case string:
		var err error
		if d.Duration, err = time.ParseDuration(value); err != nil {
			return err
		}
		return nil
	default:
		return errors.New("invalid duration")
	}
}

// ChaincodeServerUserData holds "connection.json" information
type ChaincodeServerUserData struct {
	Address            string   `json:"address"`
	DialTimeout        Duration `json:"dial_timeout"`
	TlsRequired        bool     `json:"tls_required"`
	ClientAuthRequired bool     `json:"client_auth_required"`
	KeyPath            string   `json:"key_path"`
	CertPath           string   `json:"cert_path"`
	RootCertPath       string   `json:"root_cert_path"`
}

func (ccdata *ChaincodeServerUserData) ChaincodeServerInfo(cryptoDir string) (*ccintf.ChaincodeServerInfo, error) {
	if ccdata.Address == "" {
		return nil, errors.New("chaincode address not provided")
	}
	connInfo := &ccintf.ChaincodeServerInfo{Address: ccdata.Address}

	if ccdata.DialTimeout == (Duration{}) {
		connInfo.ClientConfig.Timeout = DialTimeout
	} else {
		connInfo.ClientConfig.Timeout = ccdata.DialTimeout.Duration
	}

	// we can expose this if necessary
	connInfo.ClientConfig.KaOpts = comm.DefaultKeepaliveOptions

	if !ccdata.TlsRequired {
		return connInfo, nil
	}
	if ccdata.ClientAuthRequired && ccdata.KeyPath == "" {
		return nil, errors.New("chaincode tls key not provided")
	}
	if ccdata.ClientAuthRequired && ccdata.CertPath == "" {
		return nil, errors.New("chaincode tls cert not provided")
	}
	if ccdata.RootCertPath == "" {
		return nil, errors.New("chaincode tls root cert not provided")
	}

	connInfo.ClientConfig.SecOpts.UseTLS = true

	if ccdata.ClientAuthRequired {
		connInfo.ClientConfig.SecOpts.RequireClientCert = true
		b, err := ioutil.ReadFile(filepath.Join(cryptoDir, ccdata.CertPath))
		if err != nil {
			return nil, errors.WithMessage(err, fmt.Sprintf("error reading cert file %s", ccdata.CertPath))
		}
		connInfo.ClientConfig.SecOpts.Certificate = b

		b, err = ioutil.ReadFile(filepath.Join(cryptoDir, ccdata.KeyPath))
		if err != nil {
			return nil, errors.WithMessage(err, fmt.Sprintf("error reading key file %s", ccdata.KeyPath))
		}
		connInfo.ClientConfig.SecOpts.Key = b
	}

	b, err := ioutil.ReadFile(filepath.Join(cryptoDir, ccdata.RootCertPath))
	if err != nil {
		return nil, errors.WithMessage(err, fmt.Sprintf("error reading root cert file %s", ccdata.RootCertPath))
	}
	connInfo.ClientConfig.SecOpts.ServerRootCAs = [][]byte{b}

	return connInfo, nil
}

func (i *Instance) ChaincodeServerReleaseDir() string {
	return filepath.Join(i.ReleaseDir, CCServerReleaseDir)
}

func (i *Instance) ChaincodeServerInfo() (*ccintf.ChaincodeServerInfo, error) {
	ccinfoPath := filepath.Join(i.ChaincodeServerReleaseDir(), "connection.json")

	_, err := os.Stat(ccinfoPath)

	if os.IsNotExist(err) {
		return nil, nil
	}

	if err != nil {
		return nil, errors.WithMessage(err, "connection information not provided")
	}
	b, err := ioutil.ReadFile(ccinfoPath)
	if err != nil {
		return nil, errors.WithMessagef(err, "could not read '%s' for chaincode info", ccinfoPath)
	}
	ccdata := &ChaincodeServerUserData{}
	err = json.Unmarshal(b, &ccdata)
	if err != nil {
		return nil, errors.WithMessagef(err, "malformed chaincode info at '%s'", ccinfoPath)
	}

	return ccdata.ChaincodeServerInfo(i.ChaincodeServerReleaseDir())
}

func (i *Instance) Start(peerConnection *ccintf.PeerConnection) error {
	sess, err := i.Builder.Run(i.PackageID, i.BldDir, peerConnection)
	if err != nil {
		return errors.WithMessage(err, "could not execute run")
	}
	i.Session = sess
	return nil
}

// Stop signals the process to terminate with SIGTERM. If the process doesn't
// terminate within TermTimeout, the process is killed with SIGKILL.
func (i *Instance) Stop() error {
	if i.Session == nil {
		return errors.Errorf("instance has not been started")
	}

	done := make(chan struct{})
	go func() { i.Wait(); close(done) }()

	i.Session.Signal(syscall.SIGTERM)
	select {
	case <-time.After(i.TermTimeout):
		i.Session.Signal(syscall.SIGKILL)
	case <-done:
		return nil
	}

	select {
	case <-time.After(5 * time.Second):
		return errors.Errorf("failed to stop instance '%s'", i.PackageID)
	case <-done:
		return nil
	}
}

func (i *Instance) Wait() (int, error) {
	if i.Session == nil {
		return -1, errors.Errorf("instance was not successfully started")
	}

	err := i.Session.Wait()
	err = errors.Wrapf(err, "builder '%s' run failed", i.Builder.Name)
	if exitErr, ok := errors.Cause(err).(*exec.ExitError); ok {
		return exitErr.ExitCode(), err
	}
	return 0, err
}
