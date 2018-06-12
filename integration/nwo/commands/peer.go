/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package commands

type NodeStart struct {
	PeerID string
	Dir    string
}

func (n NodeStart) SessionName() string {
	return n.PeerID
}

func (n NodeStart) WorkingDir() string {
	return n.Dir
}

func (n NodeStart) Args() []string {
	return []string{
		"node", "start",
	}
}

type ChannelCreate struct {
	ChannelID   string
	Orderer     string
	File        string
	OutputBlock string
}

func (c ChannelCreate) SessionName() string {
	return "peer-channel-create"
}

func (c ChannelCreate) Args() []string {
	return []string{
		"channel", "create",
		"--channelID", c.ChannelID,
		"--orderer", c.Orderer,
		"--file", c.File,
		"--outputBlock", c.OutputBlock,
	}
}

type ChannelJoin struct {
	BlockPath string
}

func (c ChannelJoin) SessionName() string {
	return "peer-channel-join"
}

func (c ChannelJoin) Args() []string {
	return []string{
		"channel", "join",
		"-b", c.BlockPath,
	}
}

type ChannelFetch struct {
	ChannelID  string
	Block      string
	Orderer    string
	OutputFile string
}

func (c ChannelFetch) SessionName() string {
	return "peer-channel-fetch"
}

func (c ChannelFetch) Args() []string {
	args := []string{
		"channel", "fetch", c.Block,
	}
	if c.ChannelID != "" {
		args = append(args, "--channelID", c.ChannelID)
	}
	if c.Orderer != "" {
		args = append(args, "--orderer", c.Orderer)
	}
	if c.OutputFile != "" {
		args = append(args, c.OutputFile)
	}
	return args
}

type ChaincodeInstall struct {
	Name    string
	Version string
	Path    string
}

func (c ChaincodeInstall) SessionName() string {
	return "peer-chaincode-install"
}

func (c ChaincodeInstall) Args() []string {
	return []string{
		"chaincode", "install",
		"--name", c.Name,
		"--version", c.Version,
		"--path", c.Path,
	}
}

type ChaincodeInstantiate struct {
	ChannelID string
	Orderer   string
	Name      string
	Version   string
	Ctor      string
	Policy    string
}

func (c ChaincodeInstantiate) SessionName() string {
	return "peer-chaincode-instantiate"
}

func (c ChaincodeInstantiate) Args() []string {
	return []string{
		"chaincode", "instantiate",
		"--channelID", c.ChannelID,
		"--orderer", c.Orderer,
		"--name", c.Name,
		"--version", c.Version,
		"--ctor", c.Ctor,
		"--policy", c.Policy,
	}
}

type ChaincodeListInstalled struct{}

func (c ChaincodeListInstalled) SessionName() string {
	return "peer-chaincode-list-installed"
}

func (c ChaincodeListInstalled) Args() []string {
	return []string{
		"chaincode", "list", "--installed",
	}
}

type ChaincodeListInstantiated struct {
	ChannelID string
}

func (c ChaincodeListInstantiated) SessionName() string {
	return "peer-chaincode-list-instantiated"
}

func (c ChaincodeListInstantiated) Args() []string {
	return []string{
		"chaincode", "list", "--instantiated",
		"--channelID", c.ChannelID,
	}
}

type ChaincodeQuery struct {
	ChannelID string
	Name      string
	Ctor      string
}

func (c ChaincodeQuery) SessionName() string {
	return "peer-chaincode-query"
}

func (c ChaincodeQuery) Args() []string {
	return []string{
		"chaincode", "query",
		"--channelID", c.ChannelID,
		"--name", c.Name,
		"--ctor", c.Ctor,
	}
}

type ChaincodeInvoke struct {
	ChannelID     string
	Orderer       string
	Name          string
	Ctor          string
	PeerAddresses []string
	WaitForEvent  bool
}

func (c ChaincodeInvoke) SessionName() string {
	return "peer-chaincode-invoke"
}

func (c ChaincodeInvoke) Args() []string {
	args := []string{
		"chaincode", "invoke",
		"--channelID", c.ChannelID,
		"--orderer", c.Orderer,
		"--name", c.Name,
		"--ctor", c.Ctor,
	}
	for _, p := range c.PeerAddresses {
		args = append(args, "--peerAddresses", p)
	}
	if c.WaitForEvent {
		args = append(args, "--waitForEvent")
	}
	return args
}

type SignConfigTx struct {
	File string
}

func (s SignConfigTx) SessionName() string {
	return "peer-channel-signconfigtx"
}

func (s SignConfigTx) Args() []string {
	return []string{
		"channel", "signconfigtx",
		"--file", s.File,
	}
}

type ChannelUpdate struct {
	ChannelID string
	Orderer   string
	File      string
}

func (c ChannelUpdate) SessionName() string {
	return "peer-channel-update"
}

func (c ChannelUpdate) Args() []string {
	return []string{
		"channel", "update",
		"--channelID", c.ChannelID,
		"--orderer", c.Orderer,
		"--file", c.File,
	}
}

type LoggingSetLevel struct {
	ModuleRegexp string
	Level        string
}

func (l LoggingSetLevel) SessionName() string {
	return "peer-logging-setlevel"
}

func (l LoggingSetLevel) Args() []string {
	return []string{
		"logging", "setlevel", l.ModuleRegexp, l.Level,
	}
}
