/*
Copyright IBM Corp All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package runner

import (
	"fmt"
	"os"
	"os/exec"
	"time"

	"github.com/hyperledger/fabric/integration/pvtdata/helpers"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gbytes"
	"github.com/onsi/gomega/gexec"
	"github.com/tedsuo/ifrit/ginkgomon"
)

type Peer struct {
	Path          string
	GoPath        string
	ExecPath      string
	ConfigDir     string
	MSPConfigPath string
	LogLevel      string
}

func (p *Peer) setupEnvironment(cmd *exec.Cmd) {
	for _, env := range os.Environ() {
		cmd.Env = append(cmd.Env, env)
	}
	if p.ConfigDir != "" {
		cmd.Env = append(cmd.Env, fmt.Sprintf("FABRIC_CFG_PATH=%s", p.ConfigDir))
	}
	if p.MSPConfigPath != "" {
		cmd.Env = append(cmd.Env, fmt.Sprintf("CORE_PEER_MSPCONFIGPATH=%s", p.MSPConfigPath))
	}
	if p.GoPath != "" {
		cmd.Env = append(cmd.Env, fmt.Sprintf("GOPATH=%s", p.GoPath))
	}
	if p.ExecPath != "" {
		cmd.Env = append(cmd.Env, fmt.Sprintf("PATH=%s", p.ExecPath))
	}
	if p.LogLevel != "" {
		cmd.Env = append(cmd.Env, fmt.Sprintf("CORE_LOGGING_LEVEL=%s", p.LogLevel))
	}
}

func (p *Peer) NodeStart(index int) *ginkgomon.Runner {
	cmd := exec.Command(p.Path, "node", "start")
	p.setupEnvironment(cmd)

	r := ginkgomon.New(ginkgomon.Config{
		Name:          fmt.Sprintf("peer-%d", index),
		AnsiColorCode: fmt.Sprintf("%dm", 92+index%6),
		Command:       cmd,
	})

	return r
}

func (p *Peer) ChaincodeListInstalled() *ginkgomon.Runner {
	cmd := exec.Command(p.Path, "chaincode", "list", "--installed")
	p.setupEnvironment(cmd)

	r := ginkgomon.New(ginkgomon.Config{
		Name:          "list installed",
		AnsiColorCode: "4;33m",
		Command:       cmd,
	})

	return r
}

func (p *Peer) ChaincodeListInstantiated(channel string) *ginkgomon.Runner {
	cmd := exec.Command(p.Path, "chaincode", "list", "--instantiated", "-C", channel)
	p.setupEnvironment(cmd)

	r := ginkgomon.New(ginkgomon.Config{
		Name:          "list instantiated",
		AnsiColorCode: "4;34m",
		Command:       cmd,
	})

	return r
}

func (p *Peer) CreateChannel(channel string, filename string, orderer string) *ginkgomon.Runner {
	cmd := exec.Command(
		p.Path, "channel", "create",
		"-c", channel,
		"-o", orderer,
		"-f", filename,
		"--outputBlock", "/dev/null",
	)
	p.setupEnvironment(cmd)

	r := ginkgomon.New(ginkgomon.Config{
		Name:          "channel create",
		AnsiColorCode: "4;35m",
		Command:       cmd,
	})

	return r
}

func (p *Peer) GetChannelInfo(channel string) *ginkgomon.Runner {
	cmd := exec.Command(p.Path, "channel", "getinfo", "-c", channel)
	p.setupEnvironment(cmd)

	r := ginkgomon.New(ginkgomon.Config{
		Name:          "channel get info",
		AnsiColorCode: "4;35m",
		Command:       cmd,
	})

	return r
}

func (p *Peer) FetchChannel(channel string, filename string, block string, orderer string) *ginkgomon.Runner {
	cmd := exec.Command(p.Path, "channel", "fetch", block, "-c", channel, "-o", orderer, filename)
	p.setupEnvironment(cmd)

	r := ginkgomon.New(ginkgomon.Config{
		Name:          "channel fetch",
		AnsiColorCode: "4;36m",
		Command:       cmd,
	})

	return r
}

func (p *Peer) JoinChannel(transactionFile string) *ginkgomon.Runner {
	cmd := exec.Command(p.Path, "channel", "join", "-b", transactionFile)
	p.setupEnvironment(cmd)

	r := ginkgomon.New(ginkgomon.Config{
		Name:          "channel join",
		AnsiColorCode: "4;37m",
		Command:       cmd,
	})

	return r
}

func (p *Peer) SignConfigTx(transactionFile string) *ginkgomon.Runner {
	cmd := exec.Command(p.Path, "channel", "signconfigtx", "-f", transactionFile)
	p.setupEnvironment(cmd)

	r := ginkgomon.New(ginkgomon.Config{
		Name:          "channel signconfigtx",
		AnsiColorCode: "4;38m",
		Command:       cmd,
	})

	return r
}

func (p *Peer) UpdateChannel(transactionFile string, channel string, orderer string) *ginkgomon.Runner {
	cmd := exec.Command(p.Path, "channel", "update", "-c", channel, "-o", orderer, "-f", transactionFile)
	p.setupEnvironment(cmd)

	r := ginkgomon.New(ginkgomon.Config{
		Name:          "channel update",
		AnsiColorCode: "4;33m",
		Command:       cmd,
	})

	return r
}

func (p *Peer) InstallChaincode(name, version, path string) {
	cmd := exec.Command(p.Path, "chaincode", "install", "-n", name, "-v", version, "-p", path)
	p.setupEnvironment(cmd)

	sess, err := helpers.StartSession(cmd, "install", "4;34m")
	ExpectWithOffset(1, err).NotTo(HaveOccurred())
	EventuallyWithOffset(1, sess, time.Minute).Should(gexec.Exit(0))

	cmd = exec.Command(p.Path, "chaincode", "list", "--installed")
	p.setupEnvironment(cmd)

	sess, err = helpers.StartSession(cmd, "list", "4;33m")
	ExpectWithOffset(1, err).NotTo(HaveOccurred())
	EventuallyWithOffset(1, sess, time.Minute).Should(gexec.Exit(0))
	Expect(sess).To(gbytes.Say(fmt.Sprintf("Name: %s, Version: %s,", name, version)))
}

func (p *Peer) InstantiateChaincode(name, version, orderer, channel, args, policy string, collectionsConfigPath string) {
	cmd := exec.Command(p.Path, "chaincode", "instantiate", "-n", name, "-v", version, "-o", orderer, "-C", channel, "-c", args, "-P", policy, "--collections-config", collectionsConfigPath)
	p.setupEnvironment(cmd)

	sess, err := helpers.StartSession(cmd, "instantiate", "4;35m")
	ExpectWithOffset(1, err).NotTo(HaveOccurred())
	EventuallyWithOffset(1, sess, time.Minute).Should(gexec.Exit(0))

	p.VerifyChaincodeIsInstantiated(name, version, channel, time.Minute)
}

func (p *Peer) UpgradeChaincode(name string, version string, orderer string, channel string, args string, policy string, collectionsConfigPath string) {
	cmd := exec.Command(p.Path, "chaincode", "upgrade", "-n", name, "-v", version, "-o", orderer, "-C", channel, "-c", args, "-P", policy, "--collections-config", collectionsConfigPath)
	p.setupEnvironment(cmd)

	sess, err := helpers.StartSession(cmd, "upgrade", "4;35m")
	ExpectWithOffset(1, err).NotTo(HaveOccurred())
	EventuallyWithOffset(1, sess, time.Minute).Should(gexec.Exit(0))

	p.VerifyChaincodeIsInstantiated(name, version, channel, time.Minute)
}

func (p *Peer) VerifyChaincodeIsInstantiated(chaincodeName string, version string, channel string, timeout time.Duration) {
	listInstantiated := func() *gbytes.Buffer {
		cmd := exec.Command(p.Path, "chaincode", "list", "--instantiated", "-C", channel)
		p.setupEnvironment(cmd)

		sess, err := helpers.StartSession(cmd, "list instantiated", "4;34m")
		ExpectWithOffset(1, err).NotTo(HaveOccurred())
		EventuallyWithOffset(1, sess, 10*time.Second).Should(gexec.Exit(0))
		return sess.Buffer()
	}
	EventuallyWithOffset(1, listInstantiated, timeout).Should(gbytes.Say(fmt.Sprintf("Name: %s, Version: %s,", chaincodeName, version)))
}

func (p *Peer) QueryChaincode(name string, channel string, args string) *ginkgomon.Runner {
	cmd := exec.Command(p.Path, "chaincode", "query", "-n", name, "-C", channel, "-c", args)
	p.setupEnvironment(cmd)

	r := ginkgomon.New(ginkgomon.Config{
		Name:          "query",
		AnsiColorCode: "4;36m",
		Command:       cmd,
	})

	return r
}

func (p *Peer) InvokeChaincode(name string, channel string, args string, orderer string, extraArgs ...string) *ginkgomon.Runner {
	cmd := exec.Command(p.Path, "chaincode", "invoke", "-n", name, "-C", channel, "-c", args, "-o", orderer)
	for _, arg := range extraArgs {
		cmd.Args = append(cmd.Args, arg)
	}

	p.setupEnvironment(cmd)

	r := ginkgomon.New(ginkgomon.Config{
		Name:          "invoke",
		AnsiColorCode: "4;37m",
		Command:       cmd,
	})

	return r
}

func (p *Peer) SetLogLevel(moduleRegExp string, level string) *ginkgomon.Runner {
	cmd := exec.Command(p.Path, "logging", "setlevel", moduleRegExp, level)
	p.setupEnvironment(cmd)

	r := ginkgomon.New(ginkgomon.Config{
		Name:          "logging",
		AnsiColorCode: "4;38m",
		Command:       cmd,
	})

	return r
}
