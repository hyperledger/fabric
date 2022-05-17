/*
Copyright IBM Corp All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package nwo

import (
	"fmt"
	"io/ioutil"
	"net/http"

	"github.com/hyperledger/fabric/integration/nwo/runner"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gexec"
)

type Components struct {
	ServerAddress string `json:"server_address"`
}

func (c *Components) ConfigTxGen() string {
	return c.Build("github.com/hyperledger/fabric/cmd/configtxgen")
}

func (c *Components) Cryptogen() string {
	return c.Build("github.com/hyperledger/fabric/cmd/cryptogen")
}

func (c *Components) Discover() string {
	return c.Build("github.com/hyperledger/fabric/cmd/discover")
}

func (c *Components) Idemixgen() string {
	idemixgen, err := gexec.Build("github.com/IBM/idemix/tools/idemixgen", "-mod=mod")
	Expect(err).NotTo(HaveOccurred())
	return idemixgen
}

func (c *Components) Orderer() string {
	return c.Build("github.com/hyperledger/fabric/cmd/orderer")
}

func (c *Components) Osnadmin() string {
	return c.Build("github.com/hyperledger/fabric/cmd/osnadmin")
}

func (c *Components) Peer() string {
	return c.Build("github.com/hyperledger/fabric/cmd/peer")
}

func (c *Components) Cleanup() {}

func (c *Components) Build(path string) string {
	Expect(c.ServerAddress).NotTo(BeEmpty(), "build server address is empty")

	resp, err := http.Get(fmt.Sprintf("http://%s/%s", c.ServerAddress, path))
	Expect(err).NotTo(HaveOccurred())

	body, err := ioutil.ReadAll(resp.Body)
	Expect(err).NotTo(HaveOccurred())

	if resp.StatusCode != http.StatusOK {
		Expect(resp.StatusCode).To(Equal(http.StatusOK), string(body))
	}

	return string(body)
}

const CCEnvDefaultImage = "hyperledger/fabric-ccenv:latest"

var RequiredImages = []string{
	CCEnvDefaultImage,
	runner.CouchDBDefaultImage,
}
