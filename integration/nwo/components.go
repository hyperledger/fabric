/*
Copyright IBM Corp All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package nwo

import (
	"os"
	"sync"

	"github.com/hyperledger/fabric/integration/helpers"
	"github.com/hyperledger/fabric/integration/runner"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gexec"
)

const CCEnvDefaultImage = "hyperledger/fabric-ccenv:latest"

type Components struct {
	Paths map[string]string
	Args  []string

	cryptoOnce   sync.Once
	idemixOnce   sync.Once
	configOnce   sync.Once
	ordererOnce  sync.Once
	peerOnce     sync.Once
	discoverOnce sync.Once
	tokenOnce    sync.Once
	verifyOnce   sync.Once
}

var RequiredImages = []string{
	CCEnvDefaultImage,
	runner.CouchDBDefaultImage,
	runner.KafkaDefaultImage,
	runner.ZooKeeperDefaultImage,
}

func (c *Components) Cleanup() {
	for _, path := range c.Paths {
		err := os.Remove(path)
		Expect(err).NotTo(HaveOccurred())
	}
	gexec.CleanupBuildArtifacts()
}

func (c *Components) Cryptogen() string {
	c.cryptoOnce.Do(func() {
		c.Paths["cryptogen"] = c.build("cryptogen", "github.com/hyperledger/fabric/cmd/cryptogen")
	})
	return c.Paths["cryptogen"]
}

func (c *Components) Idemixgen() string {
	c.idemixOnce.Do(func() {
		c.Paths["idemix"] = c.build("idemix", "github.com/hyperledger/fabric/common/tools/idemixgen")
	})
	return c.Paths["idemix"]
}

func (c *Components) ConfigTxGen() string {
	c.configOnce.Do(func() {
		c.Paths["configtxgen"] = c.build("configtxgen", "github.com/hyperledger/fabric/cmd/configtxgen")
	})
	return c.Paths["configtxgen"]
}

func (c *Components) Orderer() string {
	c.ordererOnce.Do(func() {
		c.Paths["orderer"] = c.build("orderer", "github.com/hyperledger/fabric/orderer")
	})
	return c.Paths["orderer"]
}

func (c *Components) Peer() string {
	c.peerOnce.Do(func() {
		c.Paths["peer"] = c.build("peer", "github.com/hyperledger/fabric/cmd/peer")
	})
	return c.Paths["peer"]
}

func (c *Components) Discover() string {
	c.discoverOnce.Do(func() {
		c.Paths["discover"] = c.build("discover", "github.com/hyperledger/fabric/cmd/discover")
	})
	return c.Paths["discover"]
}

func (c *Components) Token() string {
	c.tokenOnce.Do(func() {
		c.Paths["token"] = c.build("token", "github.com/hyperledger/fabric/cmd/token")
	})
	return c.Paths["token"]
}

func (c *Components) build(binaryName string, path string) string {
	c.verifyOnce.Do(func() {
		if c.Paths == nil {
			c.Paths = make(map[string]string)
		}
		helpers.AssertImagesExist(RequiredImages...)
	})
	build, err := gexec.Build(path, c.Args...)
	Expect(err).NotTo(HaveOccurred())
	c.Paths[binaryName] = build
	return c.Paths[binaryName]
}
