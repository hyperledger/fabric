/*
Copyright IBM Corp All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package world

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	docker "github.com/fsouza/go-dockerclient"
	"github.com/hyperledger/fabric/integration/helpers"
	"github.com/hyperledger/fabric/integration/runner"
	"github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gexec"
	"github.com/tedsuo/ifrit"
)

type Components struct {
	Paths map[string]string
}

var RequiredImages = []string{
	"hyperledger/fabric-ccenv:latest",
	runner.CouchDBDefaultImage,
	runner.KafkaDefaultImage,
	runner.ZooKeeperDefaultImage,
}

func (c *Components) Build(args ...string) {
	helpers.AssertImagesExist(RequiredImages...)

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

func (c *Components) ZooKeeper(id int, network *docker.Network) *runner.ZooKeeper {
	name := fmt.Sprintf("zookeeper-%d", id)
	colorCode := fmt.Sprintf("%dm", 31+id%6)
	return &runner.ZooKeeper{
		ZooMyID:     id,
		NetworkName: network.Name,
		OutputStream: gexec.NewPrefixedWriter(
			fmt.Sprintf("\x1b[32m[o]\x1b[%s[%s]\x1b[0m ", colorCode, name),
			ginkgo.GinkgoWriter,
		),
		ErrorStream: gexec.NewPrefixedWriter(
			fmt.Sprintf("\x1b[91m[e]\x1b[%s[%s]\x1b[0m ", colorCode, name),
			ginkgo.GinkgoWriter,
		),
	}
}

func (c *Components) Kafka(id int, network *docker.Network) *runner.Kafka {
	name := fmt.Sprintf("kafka-%d", id)
	colorCode := fmt.Sprintf("1;%dm", 31+id%6)
	return &runner.Kafka{
		BrokerID:    id,
		NetworkName: network.Name,
		OutputStream: gexec.NewPrefixedWriter(
			fmt.Sprintf("\x1b[32m[o]\x1b[%s[%s]\x1b[0m ", colorCode, name),
			ginkgo.GinkgoWriter,
		),
		ErrorStream: gexec.NewPrefixedWriter(
			fmt.Sprintf("\x1b[91m[e]\x1b[%s[%s]\x1b[0m ", colorCode, name),
			ginkgo.GinkgoWriter,
		),
	}
}

func execute(r ifrit.Runner) {
	p := ifrit.Invoke(r)
	EventuallyWithOffset(1, p.Ready(), 2*time.Second).Should(BeClosed())
	EventuallyWithOffset(1, p.Wait(), 45*time.Second).Should(Receive(BeNil()))
}

func (w *World) SetupWorld(d Deployment) {
	w.BootstrapNetwork(d.Channel)
	helpers.CopyFile(filepath.Join("testdata", "orderer.yaml"), filepath.Join(w.Rootpath, "orderer.yaml"))
	w.CopyPeerConfigs("testdata")
	w.BuildNetwork()

	peers := []string{}
	for _, peerOrg := range w.PeerOrgs {
		for peer := 0; peer < peerOrg.PeerCount; peer++ {
			peers = append(peers, fmt.Sprintf("peer%d.%s", peer, peerOrg.Domain))
		}
	}
	w.SetupChannel(d, peers)
}
