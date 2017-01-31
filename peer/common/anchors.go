/*
Copyright IBM Corp. 2017 All Rights Reserved.

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

package common

import (
	"bufio"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/hyperledger/fabric/msp"
	"github.com/hyperledger/fabric/protos/peer"
	"github.com/spf13/viper"
)

type AnchorPeerParser struct {
	anchorPeerParam    string
	defaultAnchorPeers []*peer.AnchorPeer
}

func (app *AnchorPeerParser) Parse() ([]*peer.AnchorPeer, error) {
	if app.defaultAnchorPeers != nil {
		return app.defaultAnchorPeers, nil
	}
	return loadAnchorPeers(app.anchorPeerParam)
}

func GetDefaultAnchorPeerParser() *AnchorPeerParser {
	anchorPeerFile := "/tmp/anchorPeer.txt"
	f, err := os.Create(anchorPeerFile)
	if err != nil {
		panic(err)
	}
	defer f.Close()
	defer os.Remove(anchorPeerFile)
	f.Write([]byte(defaultAnchorPeerFile))
	f.Sync()
	aps, err := loadAnchorPeers(anchorPeerFile)
	if err != nil {
		panic(err)
	}
	return &AnchorPeerParser{defaultAnchorPeers: aps}
}

func loadAnchorPeers(anchorPeerParam string) ([]*peer.AnchorPeer, error) {
	anchorPeerFileList := strings.Split(anchorPeerParam, ",")
	for _, f := range anchorPeerFileList {
		if _, err := os.Stat(f); os.IsNotExist(err) {
			return nil, fmt.Errorf("File %s doesn't exist", f)
		}
	}

	var anchorPeers []*peer.AnchorPeer

	for _, f := range anchorPeerFileList {
		if ap, err := anchorPeerFromFile(f); err != nil {
			return nil, err
		} else {
			anchorPeers = append(anchorPeers, ap)
		}
	}

	return anchorPeers, nil
}

func anchorPeerFromFile(filename string) (*peer.AnchorPeer, error) {
	f, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	invalidFormatErr := fmt.Errorf("%s has invalid format", filename)
	if !sc.Scan() {
		return nil, invalidFormatErr
	}
	hostname := sc.Text()
	if !sc.Scan() || len(hostname) == 0 {
		return nil, invalidFormatErr
	}
	port, err := strconv.ParseInt(sc.Text(), 10, 64)
	if err != nil {
		return nil, fmt.Errorf("Second line must be a number: %d", port)
	}

	var rawPEM []byte
	for sc.Scan() {
		line := sc.Text()
		line = line + "\n"
		rawPEM = append(rawPEM, []byte(line)...)
	}
	block, _ := pem.Decode(rawPEM)
	if block == nil {
		return nil, fmt.Errorf("Anchor peer certificate is not a valid PEM certificate")
	}
	_, err = x509.ParseCertificate(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("Anchor peer certificate is not a valid PEM certificate: %v", err)
	}

	// TODO: this MSP-ID isn't going to stay like that in the future,
	// but right now, it's the same for everyone.
	identity, err := msp.NewSerializedIdentity("DEFAULT", rawPEM)
	if err != nil {
		return nil, err
	}

	ap := &peer.AnchorPeer{
		Host: hostname,
		Port: int32(port),
		Cert: identity,
	}

	if viper.GetBool("peer.gossip.ignoresecurity") {
		ap.Cert = []byte(fmt.Sprintf("%s:%d", ap.Host, ap.Port))
	}

	return ap, nil
}

const defaultAnchorPeerFile = `peer0
7051
-----BEGIN CERTIFICATE-----
MIICjDCCAjKgAwIBAgIUBEVwsSx0TmqdbzNwleNBBzoIT0wwCgYIKoZIzj0EAwIw
fzELMAkGA1UEBhMCVVMxEzARBgNVBAgTCkNhbGlmb3JuaWExFjAUBgNVBAcTDVNh
biBGcmFuY2lzY28xHzAdBgNVBAoTFkludGVybmV0IFdpZGdldHMsIEluYy4xDDAK
BgNVBAsTA1dXVzEUMBIGA1UEAxMLZXhhbXBsZS5jb20wHhcNMTYxMTExMTcwNzAw
WhcNMTcxMTExMTcwNzAwWjBjMQswCQYDVQQGEwJVUzEXMBUGA1UECBMOTm9ydGgg
Q2Fyb2xpbmExEDAOBgNVBAcTB1JhbGVpZ2gxGzAZBgNVBAoTEkh5cGVybGVkZ2Vy
IEZhYnJpYzEMMAoGA1UECxMDQ09QMFkwEwYHKoZIzj0CAQYIKoZIzj0DAQcDQgAE
HBuKsAO43hs4JGpFfiGMkB/xsILTsOvmN2WmwpsPHZNL6w8HWe3xCPQtdG/XJJvZ
+C756KEsUBM3yw5PTfku8qOBpzCBpDAOBgNVHQ8BAf8EBAMCBaAwHQYDVR0lBBYw
FAYIKwYBBQUHAwEGCCsGAQUFBwMCMAwGA1UdEwEB/wQCMAAwHQYDVR0OBBYEFOFC
dcUZ4es3ltiCgAVDoyLfVpPIMB8GA1UdIwQYMBaAFBdnQj2qnoI/xMUdn1vDmdG1
nEgQMCUGA1UdEQQeMByCCm15aG9zdC5jb22CDnd3dy5teWhvc3QuY29tMAoGCCqG
SM49BAMCA0gAMEUCIDf9Hbl4xn3z4EwNKmilM9lX2Fq4jWpAaRVB97OmVEeyAiEA
25aDPQHGGq2AvhKT0wvt08cX1GTGCIbfmuLpMwKQj38=
-----END CERTIFICATE-----`
