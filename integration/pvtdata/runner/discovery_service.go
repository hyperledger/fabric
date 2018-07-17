/*
Copyright IBM Corp All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package runner

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/hyperledger/fabric/integration/pvtdata/helpers"
	"github.com/hyperledger/fabric/protos/discovery"
	. "github.com/onsi/gomega"

	"github.com/tedsuo/ifrit/ginkgomon"
)

// DiscoveryService wraps sd cli and enables discovering peers, config and endorsement descriptors
type DiscoveryService struct {
	// The location of the discovery service executable
	Path string
	// Path to the config file that will be generated in GenerateConfig function and will be used later in all other functions
	ConfigFilePath string
	// Log level that will be used in discovery service executable
	LogLevel string
}

func (sd *DiscoveryService) setupEnvironment(cmd *exec.Cmd) {
	for _, env := range os.Environ() {
		cmd.Env = append(cmd.Env, env)
	}
	if sd.LogLevel != "" {
		cmd.Env = append(cmd.Env, fmt.Sprintf("CORE_LOGGING_LEVEL=%s", sd.LogLevel))
	}
}

func (sd *DiscoveryService) GenerateConfig(userCert string, userKey string, msp string, extraArgs ...string) *ginkgomon.Runner {
	cmd := exec.Command(sd.Path, "saveConfig", "--configFile", sd.ConfigFilePath, "--userCert", userCert, "--userKey", userKey, "--MSP", msp)

	cmd.Args = append(cmd.Args, extraArgs...)
	sd.setupEnvironment(cmd)
	return ginkgomon.New(ginkgomon.Config{
		Name:          "discover saveConfig",
		AnsiColorCode: "92m",
		Command:       cmd,
	})
}

func (sd *DiscoveryService) DiscoverPeers(channel string, server string, extraArgs ...string) *ginkgomon.Runner {
	cmd := exec.Command(sd.Path, "peers", "--configFile", sd.ConfigFilePath, "--channel", channel, "--server", server)

	cmd.Args = append(cmd.Args, extraArgs...)
	sd.setupEnvironment(cmd)
	return ginkgomon.New(ginkgomon.Config{
		Name:          "discover peers",
		AnsiColorCode: "95m",
		Command:       cmd,
	})
}

func (sd *DiscoveryService) DiscoverConfig(channel string, server string, extraArgs ...string) *ginkgomon.Runner {
	cmd := exec.Command(sd.Path, "config", "--configFile", sd.ConfigFilePath, "--channel", channel, "--server", server)

	cmd.Args = append(cmd.Args, extraArgs...)
	sd.setupEnvironment(cmd)
	return ginkgomon.New(ginkgomon.Config{
		Name:          "discover config",
		AnsiColorCode: "96m",
		Command:       cmd,
	})
}

func (sd *DiscoveryService) DiscoverEndorsers(channel string, server string, chaincodeName string, collectionName string, extraArgs ...string) *ginkgomon.Runner {
	var args []string
	if collectionName == "" {
		args = []string{"endorsers", "--configFile", sd.ConfigFilePath, "--channel", channel, "--server", server, "--chaincode", chaincodeName}
	} else {
		args = []string{"endorsers", "--configFile", sd.ConfigFilePath, "--channel", channel, "--server", server, "--chaincode", chaincodeName, fmt.Sprintf("--collection=%s:%s", chaincodeName, collectionName)}
	}
	cmd := exec.Command(sd.Path, args...)
	cmd.Args = append(cmd.Args, extraArgs...)
	sd.setupEnvironment(cmd)
	return ginkgomon.New(ginkgomon.Config{
		Name:          "discover endorsers",
		AnsiColorCode: "97m",
		Command:       cmd,
	})
}

func SetupDiscoveryService(sd *DiscoveryService, org int, configFilePath string, userCert string, userKeyDir string) *DiscoveryService {
	sd.ConfigFilePath = configFilePath
	sd.LogLevel = "debug"

	files, err := ioutil.ReadDir(userKeyDir)
	Expect(err).NotTo(HaveOccurred())
	userKey := ""
	Expect(len(files)).To(Equal(1))
	for _, f := range files {
		userKey = filepath.Join(userKeyDir, f.Name())
	}
	msp := fmt.Sprintf("Org%dMSP", org)

	sdRunner := sd.GenerateConfig(userCert, userKey, msp)
	err = helpers.Execute(sdRunner)
	Expect(err).NotTo(HaveOccurred())
	return sd
}

func VerifyAllPeersDiscovered(sd *DiscoveryService, expectedPeers []helpers.DiscoveredPeer, channel string, server string) ([]helpers.DiscoveredPeer, bool) {
	sdRunner := sd.DiscoverPeers(channel, server)
	err := helpers.Execute(sdRunner)
	Expect(err).NotTo(HaveOccurred())

	var discoveredPeers []helpers.DiscoveredPeer
	json.Unmarshal(sdRunner.Buffer().Contents(), &discoveredPeers)

	if helpers.CheckPeersContainsExpectedPeers(expectedPeers, discoveredPeers) &&
		helpers.CheckPeersContainsExpectedPeers(discoveredPeers, expectedPeers) {
		return discoveredPeers, true
	}
	return discoveredPeers, false
}

func VerifyConfigDiscovered(sd *DiscoveryService, expectedConfig discovery.ConfigResult, channel string, server string) bool {
	sdRunner := sd.DiscoverConfig(channel, server)
	err := helpers.Execute(sdRunner)
	Expect(err).NotTo(HaveOccurred())

	var config discovery.ConfigResult
	json.Unmarshal(sdRunner.Buffer().Contents(), &config)

	return helpers.CheckConfigContainsExpectedConfig(expectedConfig, config) &&
		helpers.CheckConfigContainsExpectedConfig(config, expectedConfig)
}

func VerifyEndorsersDiscovered(sd *DiscoveryService, expectedEndorsementDescriptor helpers.EndorsementDescriptor, channel string, server string, chaincodeName string, collectionName string) bool {
	sdRunner := sd.DiscoverEndorsers(channel, server, chaincodeName, collectionName)
	if err := helpers.Execute(sdRunner); err != nil {
		return false
	}
	var endorsers []helpers.EndorsementDescriptor
	if err := json.Unmarshal(sdRunner.Buffer().Contents(), &endorsers); err != nil {
		return false
	}

	return helpers.CheckEndorsementContainsExpectedEndorsement(expectedEndorsementDescriptor, endorsers[0]) &&
		helpers.CheckEndorsementContainsExpectedEndorsement(endorsers[0], expectedEndorsementDescriptor)
}
