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
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"

	cfsslClient "github.com/cloudflare/cfssl/api/client"
	"github.com/cloudflare/cfssl/cli"
	"github.com/cloudflare/cfssl/cli/sign"
	"github.com/cloudflare/cfssl/csr"
	"github.com/cloudflare/cfssl/log"
	"github.com/cloudflare/cfssl/signer"
	cop "github.com/hyperledger/fabric/cop/api"
)

// Client is the default implementation of COP client
type Client struct {
}

// NewClient creates a new client
func NewClient() *Client {
	return new(Client)
}

var c cli.Config

// SetConfig initializes by JSON config
func (c *Client) SetConfig(json string) cop.Error {
	// TODO: implement
	return nil
}

// Register a new identity
func (c *Client) Register(registration *cop.Registration) cop.Error {
	// TODO: implement
	return nil
}

// Enroll a registered identity
func (c *Client) Enroll(id string, secret string, remoteHost string, csrJSON string) ([]byte, cop.Error) {
	log.Debug(fmt.Sprintf("enrolling %s to %s", id, remoteHost))

	bam := newBasicAuthModifier(id, secret)
	cfsslClient.SetRequestModifier(bam.modify)
	csrPEM, _ := genKey(csrJSON)
	cert, _ := signKey(csrPEM, remoteHost)

	// fmt.Println("cert: ", cert)
	ioutil.WriteFile("/var/hyperledger/production/.cop/client/ecert.pem", cert, 0755)

	return cert, nil
}

// RegisterAndEnroll registers and enrolls a new identity
func (c *Client) RegisterAndEnroll(registration *cop.Registration) (cop.Identity, cop.Error) {
	// TODO: implement
	return nil, nil
}

// SetMyIdentity sets the identity information associated with this client
func (c *Client) SetMyIdentity(identity cop.Identity) cop.Error {
	// TODO: implement
	return nil
}

// SubmitJoinRequest submits a join request, implicitly approving by the caller
// Returns the join request ID
func (c *Client) SubmitJoinRequest(participantFile string) (*cop.JoinRequest, cop.Error) {
	// TODO: implement
	return nil, nil
}

// ApproveJoinRequest approves the join request
func (c *Client) ApproveJoinRequest(joinRequestID string) cop.Error {
	// TODO: implement
	return nil
}

// DenyJoinRequest denies the join request
func (c *Client) DenyJoinRequest(joinRequestID string) cop.Error {
	// TODO: implement
	return nil
}

// ListJoinRequests lists the currently outstanding join requests for the blockchain network
func (c *Client) ListJoinRequests() ([]cop.JoinRequest, cop.Error) {
	// TODO: implement
	return nil, nil
}

// ListParticipants lists the current participants in the blockchain network
func (c *Client) ListParticipants() ([]string, cop.Error) {
	// TODO: implement
	return nil, nil
}

// SetJoinRequestListener sets the listener to be called when a JoinRequestEvent is emitted
func (c *Client) SetJoinRequestListener(listener cop.JoinRequestListener) {
	// TODO: implement
}

func genKey(csrFile string) ([]byte, error) {
	csrFileBytes, _ := ioutil.ReadFile(csrFile)

	req := csr.CertificateRequest{
		KeyRequest: csr.NewBasicKeyRequest(),
	}

	err := json.Unmarshal(csrFileBytes, &req)
	if err != nil {
		return nil, err
	}

	var key, csrPEM []byte
	g := &csr.Generator{Validator: Validator}
	csrPEM, key, err = g.ProcessRequest(&req)
	if err != nil {
		key = nil
		return nil, err
	}

	// TODO: Put in HOME directory for end user
	os.MkdirAll("/var/hyperledger/production/.cop/client", 0755)
	ioutil.WriteFile("/var/hyperledger/production/.cop/client/key.pem", key, 0755)
	// fmt.Println("key: ", key)

	return csrPEM, nil

}

// Validator does nothing and will never return an error. It exists because creating a
// csr.Generator requires a Validator.
func Validator(req *csr.CertificateRequest) error {
	return nil
}

// func signerMain(args []string, c cli.Config) ([]byte, error) {
func signKey(csrPEM []byte, remoteHost string) ([]byte, error) {
	c.Remote = remoteHost
	s, err := sign.SignerFromConfig(c)
	if err != nil {
		return nil, nil
	}

	req := signer.SignRequest{
		// Hosts:   signer.SplitHosts(c.Hostname),
		Request: string(csrPEM),
		// Profile: c.Profile,
		// Label:   c.Label,
	}
	cert, err := s.Sign(req)
	if err != nil {
		return nil, nil
	}

	return cert, nil
}

type basicAuthModifier struct {
	user, pass string
}

func newBasicAuthModifier(user, pass string) *basicAuthModifier {
	return &basicAuthModifier{
		user: user,
		pass: pass}
}

func (bac *basicAuthModifier) modify(req *http.Request) {
	req.SetBasicAuth(bac.user, bac.pass)
}
