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

	"github.com/hyperledger/fabric/protos/peer"
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
	ap := &peer.AnchorPeer{
		Host: hostname,
		Port: int32(port),
		Cert: block.Bytes,
	}
	return ap, nil
}

const defaultAnchorPeerFile = `anchorpeer
7051
-----BEGIN CERTIFICATE-----
MIIDZzCCAk+gAwIBAgIJAKSntkwsYDPhMA0GCSqGSIb3DQEBCwUAMEoxCzAJBgNV
BAYTAklMMQ8wDQYDVQQIDAZJc3JhZWwxDjAMBgNVBAcMBUhhaWZhMQwwCgYDVQQK
DANJQk0xDDAKBgNVBAsMA0lCTTAeFw0xNzAxMjcwMzUzMjdaFw0xODAxMjcwMzUz
MjdaMEoxCzAJBgNVBAYTAklMMQ8wDQYDVQQIDAZJc3JhZWwxDjAMBgNVBAcMBUhh
aWZhMQwwCgYDVQQKDANJQk0xDDAKBgNVBAsMA0lCTTCCASIwDQYJKoZIhvcNAQEB
BQADggEPADCCAQoCggEBAL0KUiuuZFfs+BN7+FnDKoeiVGCkayQVrPrdeO8Mwu/n
928T9lhmBI6wFnmkdeEjYTi1M5dks8hEal2AP8ykREc+LTmMH5JAJ8kktnoNQteO
rdFqnBpzA0IdiDnaLLLU3QD22VT47TPxWnfqZ3Z+fEJkmxc+tNmJJ5/0eCxXC4v4
875wQZP8CEeI1EpkljL6AILLNCUN4qpug2R2CCBRvGaqA81TM8NKxvWgN90iSAiv
vrQIc3/aelIpaJN457JEqLWgAcWw982rFUn5+D3u63pUq99lWH16VU4vRdUFzqi1
E3mBbGNTcNBzrBYswj5KhMFHLBpzIwQQX+Tvjh70cwkCAwEAAaNQME4wHQYDVR0O
BBYEFNHpTtXPDggAIavkdxLh+ttFH+HCMB8GA1UdIwQYMBaAFNHpTtXPDggAIavk
dxLh+ttFH+HCMAwGA1UdEwQFMAMBAf8wDQYJKoZIhvcNAQELBQADggEBADuYrb0h
eXepdpkUSZ6t5mk6R4vyaGDQAHCUltL5Q2qhL8f5sWxMBqke/JhbB02+pQCsvj8P
SIVSXuCXgFbzP0O3gNWqGhGn9atgN/j81hGyXtpAl5U5hyqcaFATX++Rdv58TKty
WnjzYUtnrG2W6c5uK/XPmoUHoNHxkgj1HrlmuahdrxzFXkdcND7UIfW8U2K0Cz4V
gJyAC5yIOs+kakE2gwjJI8SqREgegfO1JIbBfnUCkDJj1TLu2eUkBgnVLeJrcbXq
AbiMV4MFfj5KFA51Tp8QltKbsPPm1Vx3+CRVWNnMgqVWygIQF+8h4H/CcETU4XCV
4LqJvYfKwy27YUA=
-----END CERTIFICATE-----`
