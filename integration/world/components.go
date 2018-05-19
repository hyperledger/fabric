/*
Copyright IBM Corp All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package world

import (
	"os"
	"time"

	docker "github.com/fsouza/go-dockerclient"
	"github.com/hyperledger/fabric/integration/runner"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gexec"
	"github.com/tedsuo/ifrit"
)

type Components struct {
	Paths map[string]string
}

func (c *Components) Build(args ...string) {
	if c.Paths == nil {
		c.Paths = map[string]string{}
	}
	cryptogen, err := gexec.Build("github.com/hyperledger/fabric/common/tools/cryptogen", args...)
	Expect(err).NotTo(HaveOccurred())
	c.Paths["cryptogen"] = cryptogen

	idemixgen, err := gexec.Build("github.com/hyperledger/fabric/common/tools/idemixgen", args...)
	Expect(err).NotTo(HaveOccurred())
	c.Paths["idemixgen"] = idemixgen

	configtxgen, err := gexec.Build("github.com/hyperledger/fabric/common/tools/configtxgen", args...)
	Expect(err).NotTo(HaveOccurred())
	c.Paths["configtxgen"] = configtxgen

	orderer, err := gexec.Build("github.com/hyperledger/fabric/orderer", args...)
	Expect(err).NotTo(HaveOccurred())
	c.Paths["orderer"] = orderer

	peer, err := gexec.Build("github.com/hyperledger/fabric/peer", args...)
	Expect(err).NotTo(HaveOccurred())
	c.Paths["peer"] = peer
}

func (c *Components) Cleanup() {
	for _, path := range c.Paths {
		err := os.Remove(path)
		Expect(err).NotTo(HaveOccurred())
	}
}

func (c *Components) Cryptogen() *runner.Cryptogen {
	return &runner.Cryptogen{
		Path: c.Paths["cryptogen"],
	}
}

func (c *Components) Idemixgen() *runner.Idemixgen {
	return &runner.Idemixgen{
		Path: c.Paths["idemixgen"],
	}
}

func (c *Components) Configtxgen() *runner.Configtxgen {
	return &runner.Configtxgen{
		Path: c.Paths["configtxgen"],
	}
}

func (c *Components) Orderer() *runner.Orderer {
	return &runner.Orderer{
		Path: c.Paths["orderer"],
	}
}

func (c *Components) Peer() *runner.Peer {
	return &runner.Peer{
		Path: c.Paths["peer"],
	}
}

func (c *Components) Zookeeper(id int, network *docker.Network) *runner.Zookeeper {
	return &runner.Zookeeper{
		ZooMyID:     id,
		NetworkID:   network.ID,
		NetworkName: network.Name,
	}
}

func (c *Components) Kafka(id int, network *docker.Network) *runner.Kafka {
	return &runner.Kafka{
		BrokerID:    id,
		NetworkID:   network.ID,
		NetworkName: network.Name,
	}
}

func execute(r ifrit.Runner) {
	p := ifrit.Invoke(r)
	EventuallyWithOffset(1, p.Ready(), 2*time.Second).Should(BeClosed())
	EventuallyWithOffset(1, p.Wait(), 45*time.Second).Should(Receive(BeNil()))
}
