/*
Copyright IBM Corp All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package e2e

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/hyperledger/fabric/integration/world"

	. "github.com/onsi/gomega"
)

func setupWorld(w *world.World) {
	w.BootstrapNetwork()
	copyFile(filepath.Join("testdata", "orderer.yaml"), filepath.Join(w.Rootpath, "orderer.yaml"))
	copyPeerConfigs(w.PeerOrgs, w.Rootpath)
	w.BuildNetwork()
	err := w.SetupChannel()
	Expect(err).NotTo(HaveOccurred())
}

func copyFile(src, dest string) {
	data, err := ioutil.ReadFile(src)
	Expect(err).NotTo(HaveOccurred())
	err = ioutil.WriteFile(dest, data, 0775)
	Expect(err).NotTo(HaveOccurred())
}

func copyPeerConfigs(peerOrgs []world.PeerOrgConfig, rootPath string) {
	for _, peerOrg := range peerOrgs {
		for peer := 0; peer < peerOrg.PeerCount; peer++ {
			peerDir := fmt.Sprintf("%s_%d", peerOrg.Domain, peer)
			if _, err := os.Stat(filepath.Join(rootPath, peerDir)); os.IsNotExist(err) {
				err := os.Mkdir(filepath.Join(rootPath, peerDir), 0755)
				Expect(err).NotTo(HaveOccurred())
			}
			copyFile(filepath.Join("testdata", fmt.Sprintf("%s-core.yaml", peerDir)),
				filepath.Join(rootPath, peerDir, "core.yaml"))
		}
	}
}
