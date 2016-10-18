package client

import (
	"bytes"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"time"

	backoff "github.com/cloudflare/cfssl/transport/core"
	"github.com/cloudflare/redoctober/core"
)

// RemoteServer represents a remote RedOctober server.
type RemoteServer struct {
	client        *http.Client
	serverAddress string
}

// NewRemoteServer generates a RemoteServer with the server address and
// the root CA the server uses to authenticate itself.
func NewRemoteServer(serverAddress, CAFile string) (*RemoteServer, error) {

	var rootCAs *x509.CertPool
	// populate a root CA pool from input CAfile
	// otherwise, use the system's default root CA set
	if CAFile != "" {
		rootCAs = x509.NewCertPool()
		pemBytes, err := ioutil.ReadFile(CAFile)
		if err != nil {
			return nil, errors.New("fail to read CA file: " + err.Error())
		}
		ok := rootCAs.AppendCertsFromPEM(pemBytes)
		if !ok {
			return nil, errors.New("fail to populate CA root pool.")
		}
	}

	tr := &http.Transport{
		TLSClientConfig:    &tls.Config{RootCAs: rootCAs},
		DisableCompression: true,
	}
	server := &RemoteServer{
		client:        &http.Client{Transport: tr},
		serverAddress: serverAddress,
	}
	return server, nil
}

// getURL creates URL for a specific path of the RemoteServer
func (c *RemoteServer) getURL(path string) string {
	return fmt.Sprintf("https://%s%s", c.serverAddress, path)

}

// doAction sends req to the remote server and returns the response
func (c *RemoteServer) doAction(action string, req []byte) ([]byte, error) {
	buf := bytes.NewBuffer(req)
	url := c.getURL("/" + action)
	b := backoff.Backoff{}
	var resp *http.Response
	var err error
	for {
		resp, err = c.client.Post(url, "application/json", buf)
		if err == nil {
			break
		}
		delay := b.Duration()
		log.Printf("Request to server failed. Will try again in %s", delay)
		<-time.After(delay)
	}

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	resp.Body.Close()

	// Return response body as error if status code != 200
	if resp.StatusCode != http.StatusOK {
		return nil, errors.New(string(body))
	}

	return body, nil

}

// unmarshalResponseData is a helper function that unmarshal response bytes
// into ResponseData object.
func unmarshalResponseData(respBytes []byte) (*core.ResponseData, error) {
	response := new(core.ResponseData)
	err := json.Unmarshal(respBytes, response)
	if err != nil {
		return nil, err
	}

	if response.Status != "ok" {
		return nil, errors.New(response.Status)
	}

	return response, nil
}

// Create creates an admin account at the remote server
func (c *RemoteServer) Create(req core.CreateRequest) (*core.ResponseData, error) {
	reqBytes, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}

	respBytes, err := c.doAction("create", reqBytes)
	if err != nil {
		return nil, err
	}

	return unmarshalResponseData(respBytes)
}

// Summary returns the summary reported by the remote server
func (c *RemoteServer) Summary(req core.SummaryRequest) (*core.SummaryData, error) {
	reqBytes, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}

	respBytes, err := c.doAction("summary", reqBytes)
	if err != nil {
		return nil, err
	}

	response := new(core.SummaryData)
	err = json.Unmarshal(respBytes, response)
	if err != nil {
		return nil, err
	}

	if response.Status != "ok" {
		return nil, errors.New(response.Status)
	}
	return response, nil
}

// Delegate issues a delegate request to the remote server
func (c *RemoteServer) Delegate(req core.DelegateRequest) (*core.ResponseData, error) {
	reqBytes, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}

	respBytes, err := c.doAction("delegate", reqBytes)
	if err != nil {
		return nil, err
	}

	return unmarshalResponseData(respBytes)
}

// CreateUser issues a create-user request to the remote server
func (c *RemoteServer) CreateUser(req core.CreateUserRequest) (*core.ResponseData, error) {
	reqBytes, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}

	respBytes, err := c.doAction("create-user", reqBytes)
	if err != nil {
		return nil, err
	}

	return unmarshalResponseData(respBytes)
}

// Purge issues a purge request to the remote server
func (c *RemoteServer) Purge(req core.DelegateRequest) (*core.ResponseData, error) {
	reqBytes, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}

	respBytes, err := c.doAction("purge", reqBytes)
	if err != nil {
		return nil, err
	}

	return unmarshalResponseData(respBytes)
}

// Modify issues a modify request to the remote server
func (c *RemoteServer) Modify(req core.ModifyRequest) (*core.ResponseData, error) {
	reqBytes, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}

	respBytes, err := c.doAction("modify", reqBytes)
	if err != nil {
		return nil, err
	}

	return unmarshalResponseData(respBytes)
}

// Encrypt issues an encrypt request to the remote server
func (c *RemoteServer) Encrypt(req core.EncryptRequest) (*core.ResponseData, error) {
	reqBytes, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}

	respBytes, err := c.doAction("encrypt", reqBytes)
	if err != nil {
		return nil, err
	}

	return unmarshalResponseData(respBytes)
}

// ReEncrypt issues an re-encrypt request to the remote server
func (c *RemoteServer) ReEncrypt(req core.ReEncryptRequest) (*core.ResponseData, error) {
	reqBytes, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}

	respBytes, err := c.doAction("re-encrypt", reqBytes)
	if err != nil {
		return nil, err
	}

	return unmarshalResponseData(respBytes)
}

// Decrypt issues an decrypt request to the remote server
func (c *RemoteServer) Decrypt(req core.DecryptRequest) (*core.ResponseData, error) {
	reqBytes, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}

	respBytes, err := c.doAction("decrypt", reqBytes)
	if err != nil {
		return nil, err
	}

	return unmarshalResponseData(respBytes)

}

// DecryptIntoData issues an decrypt request to the remote server and extract
// the decrypted data from the response
func (c *RemoteServer) DecryptIntoData(req core.DecryptRequest) ([]byte, error) {
	responseData, err := c.Decrypt(req)
	if err != nil {
		return nil, err
	}

	d := new(core.DecryptWithDelegates)
	err = json.Unmarshal(responseData.Response, d)
	if err != nil {
		return nil, err
	}

	return d.Data, nil

}

// Password issues an password request to the remote server
func (c *RemoteServer) Password(req []byte) (*core.ResponseData, error) {
	reqBytes, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}

	respBytes, err := c.doAction("delegate", reqBytes)
	if err != nil {
		return nil, err
	}

	return unmarshalResponseData(respBytes)
}

// Order issues an order request to the remote server
func (c *RemoteServer) Order(req core.OrderRequest) (*core.ResponseData, error) {
	reqBytes, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}

	respBytes, err := c.doAction("order", reqBytes)
	if err != nil {
		return nil, err
	}

	return unmarshalResponseData(respBytes)
}

// OrderOutstanding issues an order outstanding request to the remote server
func (c *RemoteServer) OrderOutstanding(req core.OrderOutstandingRequest) (*core.ResponseData, error) {
	reqBytes, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}

	respBytes, err := c.doAction("orderout", reqBytes)
	if err != nil {
		return nil, err
	}

	return unmarshalResponseData(respBytes)
}

// OrderInfo issues an order info request to the remote server
func (c *RemoteServer) OrderInfo(req core.OrderInfoRequest) (*core.ResponseData, error) {
	reqBytes, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}

	respBytes, err := c.doAction("orderinfo", reqBytes)
	if err != nil {
		return nil, err
	}

	return unmarshalResponseData(respBytes)
}

// OrderCancel issues an order cancel request to the remote server
func (c *RemoteServer) OrderCancel(req core.OrderInfoRequest) (*core.ResponseData, error) {
	reqBytes, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}

	respBytes, err := c.doAction("ordercancel", reqBytes)
	if err != nil {
		return nil, err
	}

	return unmarshalResponseData(respBytes)
}
