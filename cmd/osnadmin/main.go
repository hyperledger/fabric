/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package main

import (
	"bytes"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"

	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric-protos-go/common"
	"github.com/hyperledger/fabric/internal/osnadmin"
	"github.com/hyperledger/fabric/protoutil"
	"gopkg.in/alecthomas/kingpin.v2"
)

func main() {
	kingpin.Version("0.0.1")

	output, exit, err := executeForArgs(os.Args[1:])
	if err != nil {
		kingpin.Fatalf("parsing arguments: %s. Try --help", err)
	}
	fmt.Println(output)
	os.Exit(exit)
}

func executeForArgs(args []string) (output string, exit int, err error) {
	//
	// command line flags
	//
	app := kingpin.New("osnadmin", "Orderer Service Node (OSN) administration")
	orderer := app.Flag("orderer-address", "Admin endpoint of the OSN").Short('o').Required().String()
	caFile := app.Flag("ca-file", "Path to file containing PEM-encoded TLS CA certificate(s) for the OSN").String()
	clientCert := app.Flag("client-cert", "Path to file containing PEM-encoded X509 public key to use for mutual TLS communication with the OSN").String()
	clientKey := app.Flag("client-key", "Path to file containing PEM-encoded private key to use for mutual TLS communication with the OSN").String()
	noStatus := app.Flag("no-status", "Remove the HTTP status message from the command output").Default("false").Bool()

	channel := app.Command("channel", "Channel actions")

	join := channel.Command("join", "Join an Ordering Service Node (OSN) to a channel. If the channel does not yet exist, it will be created.")
	joinChannelID := join.Flag("channelID", "Channel ID").Short('c').Required().String()
	configBlockPath := join.Flag("config-block", "Path to the file containing an up-to-date config block for the channel").Short('b').Required().String()

	list := channel.Command("list", "List channel information for an Ordering Service Node (OSN). If the channelID flag is set, more detailed information will be provided for that channel.")
	listChannelID := list.Flag("channelID", "Channel ID").Short('c').String()

	remove := channel.Command("remove", "Remove an Ordering Service Node (OSN) from a channel.")
	removeChannelID := remove.Flag("channelID", "Channel ID").Short('c').Required().String()

	command, err := app.Parse(args)
	if err != nil {
		return "", 1, err
	}

	//
	// flag validation
	//
	var (
		osnURL        string
		caCertPool    *x509.CertPool
		tlsClientCert tls.Certificate
	)
	// TLS enabled
	if *caFile != "" {
		osnURL = fmt.Sprintf("https://%s", *orderer)
		var err error
		caCertPool = x509.NewCertPool()
		caFilePEM, err := os.ReadFile(*caFile)
		if err != nil {
			return "", 1, fmt.Errorf("reading orderer CA certificate: %s", err)
		}
		if !caCertPool.AppendCertsFromPEM(caFilePEM) {
			return "", 1, fmt.Errorf("failed to add ca-file PEM to cert pool")
		}

		tlsClientCert, err = tls.LoadX509KeyPair(*clientCert, *clientKey)
		if err != nil {
			return "", 1, fmt.Errorf("loading client cert/key pair: %s", err)
		}
	} else { // TLS disabled
		osnURL = fmt.Sprintf("http://%s", *orderer)
	}

	var marshaledConfigBlock []byte
	if *configBlockPath != "" {
		marshaledConfigBlock, err = os.ReadFile(*configBlockPath)
		if err != nil {
			return "", 1, fmt.Errorf("reading config block: %s", err)
		}

		err = validateBlockChannelID(marshaledConfigBlock, *joinChannelID)
		if err != nil {
			return "", 1, err
		}
	}

	//
	// call the underlying implementations
	//
	var resp *http.Response

	switch command {
	case join.FullCommand():
		resp, err = osnadmin.Join(osnURL, marshaledConfigBlock, caCertPool, tlsClientCert)
	case list.FullCommand():
		if *listChannelID != "" {
			resp, err = osnadmin.ListSingleChannel(osnURL, *listChannelID, caCertPool, tlsClientCert)
			break
		}
		resp, err = osnadmin.ListAllChannels(osnURL, caCertPool, tlsClientCert)
	case remove.FullCommand():
		resp, err = osnadmin.Remove(osnURL, *removeChannelID, caCertPool, tlsClientCert)
	}
	if err != nil {
		return errorOutput(err), 1, nil
	}

	bodyBytes, err := readBodyBytes(resp.Body)
	if err != nil {
		return errorOutput(err), 1, nil
	}

	output, err = responseOutput(!*noStatus, resp.StatusCode, bodyBytes)
	if err != nil {
		return errorOutput(err), 1, nil
	}

	return output, 0, nil
}

func responseOutput(showStatus bool, statusCode int, responseBody []byte) (string, error) {
	var buffer bytes.Buffer
	if showStatus {
		fmt.Fprintf(&buffer, "Status: %d\n", statusCode)
	}
	if len(responseBody) != 0 {
		if err := json.Indent(&buffer, responseBody, "", "\t"); err != nil {
			return "", err
		}
	}
	return buffer.String(), nil
}

func readBodyBytes(body io.ReadCloser) ([]byte, error) {
	bodyBytes, err := io.ReadAll(body)
	if err != nil {
		return nil, fmt.Errorf("reading http response body: %s", err)
	}
	body.Close()

	return bodyBytes, nil
}

func errorOutput(err error) string {
	return fmt.Sprintf("Error: %s\n", err)
}

func validateBlockChannelID(blockBytes []byte, channelID string) error {
	block := &common.Block{}
	err := proto.Unmarshal(blockBytes, block)
	if err != nil {
		return fmt.Errorf("unmarshalling block: %s", err)
	}

	blockChannelID, err := protoutil.GetChannelIDFromBlock(block)
	if err != nil {
		return err
	}

	// quick sanity check that the orderer admin is joining
	// the channel they think they're joining.
	if channelID != blockChannelID {
		return fmt.Errorf("specified --channelID %s does not match channel ID %s in config block", channelID, blockChannelID)
	}

	return nil
}
