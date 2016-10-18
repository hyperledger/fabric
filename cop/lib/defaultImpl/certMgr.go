/*
Copyright IBM Corp. 2016 All Rights Reserved.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

		 http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package defaultImpl

import (
	"bytes"
	"fmt"
	"io"
	"os/exec"
	"path/filepath"

	"github.com/cloudflare/cfssl/log"
	cop "github.com/hyperledger/fabric/cop/api"

	"encoding/json"
	"io/ioutil"
	"os"

	"github.com/cloudflare/cfssl/cli"
	"github.com/cloudflare/cfssl/cli/bundle"
	"github.com/cloudflare/cfssl/cli/certinfo"
	"github.com/cloudflare/cfssl/cli/gencert"
	"github.com/cloudflare/cfssl/cli/gencrl"
	"github.com/cloudflare/cfssl/cli/genkey"
	"github.com/cloudflare/cfssl/cli/info"
	"github.com/cloudflare/cfssl/cli/ocspdump"
	"github.com/cloudflare/cfssl/cli/ocsprefresh"
	"github.com/cloudflare/cfssl/cli/ocspserve"
	"github.com/cloudflare/cfssl/cli/ocspsign"
	"github.com/cloudflare/cfssl/cli/printdefault"
	"github.com/cloudflare/cfssl/cli/revoke"
	"github.com/cloudflare/cfssl/cli/scan"
	"github.com/cloudflare/cfssl/cli/selfsign"
	"github.com/cloudflare/cfssl/cli/serve"
	"github.com/cloudflare/cfssl/cli/sign"
	"github.com/cloudflare/cfssl/cli/version"
	//	err "github.com/hyperledger/fabric/cop/errors"

	"google.golang.org/grpc"
)

var cfsslCmds = map[string]*cli.Command{
	"bundle":         bundle.Command,
	"certinfo":       certinfo.Command,
	"sign":           sign.Command,
	"serve":          serve.Command,
	"version":        version.Command,
	"genkey":         genkey.Command,
	"gencert":        gencert.Command,
	"gencrl":         gencrl.Command,
	"ocspdump":       ocspdump.Command,
	"ocsprefresh":    ocsprefresh.Command,
	"ocspsign":       ocspsign.Command,
	"ocspserve":      ocspserve.Command,
	"selfsign":       selfsign.Command,
	"scan":           scan.Command,
	"info":           info.Command,
	"print-defaults": printdefaults.Command,
	"revoke":         revoke.Command,
}

// CertMgr is the default certificate manager
type CertMgr struct {
	rootPath            string
	participantFilePath string
	cert                []byte
	grpcServer          *grpc.Server
}

type output struct {
	Cert string
}

type gencertOutput struct {
	cert []byte
	csr  []byte
	key  []byte
}

func NewCertMgr() *CertMgr {
	// TODO: parse JSON
	srv := new(CertMgr)
	srv.rootPath = "/var/hyperledger/production/.cop/"
	srv.participantFilePath = filepath.Join(srv.rootPath, "participant.json")
	_ = os.MkdirAll(srv.rootPath, 0777)
	return srv
}

// InitSelfSign generates self-signed certs and updates the participant file
func (cm *CertMgr) InitSelfSign(hostname string, path string) cop.Error {
	return cop.NewError(cop.NotImplemented, "TODO: implement Server.InitSelfSign")
}

// GenCert generates certs and updates the participant file
func (cm *CertMgr) GenCert(csr string, prefix string, participantFile string) cop.Error {
	//log.Debug("DEBUG_ - server.go - GenCert() - Entered")
	var args []string

	gencertCmd := cfsslCmds["gencert"]
	var c cli.Config
	c.IsCA = true

	if csr == "" {
		return cop.NewError(cop.Input, "No CSR json file specified", "")
	}
	args = append(args, csr)

	if prefix == "" {
		return cop.NewError(cop.Input, "No prefix specified", "")
	}
	out := executeCommand(args, gencertCmd, c, prefix)

	var gencertOut map[string]interface{}
	json.Unmarshal([]byte(out), &gencertOut)

	var writeJSON output
	writeJSON.Cert = gencertOut["cert"].(string)
	jsonOut, _ := json.Marshal(writeJSON)

	// Write to participant.json
	if participantFile == "" {
		return cop.NewError(cop.Input, "No participant file specified", "")
	}
	ioutil.WriteFile(participantFile, jsonOut, 0644)

	//log.Debug("DEBUG_ - server.go - GenCert() - Exit")
	return nil

}

// InitLego gets certificates from Let's Encrypt and updates the participant file
func (cm *CertMgr) InitLego(host string) cop.Error {
	return cop.NewError(cop.NotImplemented, "TODO: implement Server.InitLego")
}

// SetECAKey sets the ECA key
func (cm *CertMgr) SetECAKey(key []byte) cop.Error {
	return cop.NewError(cop.NotImplemented, "TODO: implement Server.SetECAKey")
}

// SetTCAKey sets the TCA key
func (cm *CertMgr) SetTCAKey(key []byte) cop.Error {
	return cop.NewError(cop.NotImplemented, "TODO: implement Server.SetTCAKey")
}

// SetTLSKey sets the TLS key
func (cm *CertMgr) SetTLSKey(key []byte) cop.Error {
	return cop.NewError(cop.NotImplemented, "TODO: implement Server.SetLSKey")
}

// SetParticipantFilePath sets the path for the participant file
func (cm *CertMgr) SetParticipantFilePath(path string) cop.Error {
	cm.participantFilePath = path
	return nil
}

// UpdateParticipantFile updates the participant file
func (cm *CertMgr) UpdateParticipantFile() cop.Error {
	return cop.NewError(cop.NotImplemented, "TODO: implement UpdateParticipantFile")
}

// NewCertHandler creates a certificate handler
func (cm *CertMgr) NewCertHandler(cert []byte) (cop.CertHandler, cop.Error) {
	return newCOPCertHandler(cert)
}

// NewKeyHandler creates a key handler
func (cm *CertMgr) NewKeyHandler(key []byte) (cop.KeyHandler, cop.Error) {
	return nil, cop.NewError(cop.NotImplemented, "TODO: implement NewKeyHandler")
}

func executeCommand(args []string, command *cli.Command, c cli.Config, prefix string) string {
	cfsslJSONCmd := exec.Command("cfssljson", "-bare", prefix)

	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err := command.Main(args, c) // Execute command
	if err != nil {
		log.Error(err)
	}

	outC := make(chan string)
	// copy the output in a separate goroutine so printing can't block indefinitely
	var buf bytes.Buffer
	go func() {
		io.Copy(&buf, r)
		cfsslJSONCmd.Stdin = &buf
		outC <- buf.String()
	}()

	w.Close()

	out := <-outC
	outByte := []byte(out)

	tmpFile, _ := ioutil.TempFile("", "tmp")
	defer os.Remove(tmpFile.Name())
	if _, err = tmpFile.Write(outByte); err != nil {
		fmt.Println("err: ", err)
	}

	os.Stdin = tmpFile
	os.Stdout = old // restoring the real stdout

	err = cfsslJSONCmd.Run() // Execute cfssljson -bare <prefix>
	if err != nil {
		log.Error(err)
	}

	return out // To be used to store in participant file
}
