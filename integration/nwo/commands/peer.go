/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package commands

import "strconv"

type NodeStart struct {
	PeerID string
}

func (n NodeStart) SessionName() string {
	return n.PeerID
}

func (n NodeStart) Args() []string {
	return []string{
		"node", "start",
	}
}

type NodeReset struct {
}

func (n NodeReset) SessionName() string {
	return "peer-node-reset"
}

func (n NodeReset) Args() []string {
	return []string{
		"node", "reset",
	}
}

type NodeRollback struct {
	ChannelID   string
	BlockNumber int
}

func (n NodeRollback) SessionName() string {
	return "peer-node-rollback"
}

func (n NodeRollback) Args() []string {
	return []string{
		"node", "rollback",
		"--channelID", n.ChannelID,
		"--blockNumber", strconv.Itoa(n.BlockNumber),
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
		"--timeout", "15s",
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

type ChaincodePackage struct {
	Name       string
	Version    string
	Path       string
	Lang       string
	OutputFile string
}

func (c ChaincodePackage) SessionName() string {
	return "peer-chaincode-package"
}

func (c ChaincodePackage) Args() []string {
	args := []string{
		"chaincode", "package",
		"--name", c.Name,
		"--version", c.Version,
		"--path", c.Path,
		c.OutputFile,
	}

	if c.Lang != "" {
		args = append(args, "--lang", c.Lang)
	}

	return args
}

type ChaincodeInstall struct {
	Name        string
	Version     string
	Path        string
	Lang        string
	PackageFile string
}

func (c ChaincodeInstall) SessionName() string {
	return "peer-chaincode-install"
}

func (c ChaincodeInstall) Args() []string {
	args := []string{
		"chaincode", "install",
	}

	if c.PackageFile != "" {
		args = append(args, c.PackageFile)
	}
	if c.Name != "" {
		args = append(args, "--name", c.Name)
	}
	if c.Version != "" {
		args = append(args, "--version", c.Version)
	}
	if c.Path != "" {
		args = append(args, "--path", c.Path)
	}
	if c.Lang != "" {
		args = append(args, "--lang", c.Lang)
	}

	return args
}

type ChaincodeInstantiate struct {
	ChannelID         string
	Orderer           string
	Name              string
	Version           string
	Ctor              string
	Policy            string
	Lang              string
	CollectionsConfig string
}

func (c ChaincodeInstantiate) SessionName() string {
	return "peer-chaincode-instantiate"
}

func (c ChaincodeInstantiate) Args() []string {
	args := []string{
		"chaincode", "instantiate",
		"--channelID", c.ChannelID,
		"--orderer", c.Orderer,
		"--name", c.Name,
		"--version", c.Version,
		"--ctor", c.Ctor,
		"--policy", c.Policy,
	}
	if c.CollectionsConfig != "" {
		args = append(args, "--collections-config", c.CollectionsConfig)
	}

	if c.Lang != "" {
		args = append(args, "--lang", c.Lang)
	}

	return args
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

type ChaincodeUpgrade struct {
	Name              string
	Version           string
	Path              string // optional
	ChannelID         string
	Orderer           string
	Ctor              string
	Policy            string
	CollectionsConfig string // optional
}

func (c ChaincodeUpgrade) SessionName() string {
	return "peer-chaincode-upgrade"
}

func (c ChaincodeUpgrade) Args() []string {
	args := []string{
		"chaincode", "upgrade",
		"--name", c.Name,
		"--version", c.Version,
		"--channelID", c.ChannelID,
		"--orderer", c.Orderer,
		"--ctor", c.Ctor,
		"--policy", c.Policy,
	}
	if c.Path != "" {
		args = append(args, "--path", c.Path)
	}
	if c.CollectionsConfig != "" {
		args = append(args, "--collections-config", c.CollectionsConfig)
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

type ChannelInfo struct {
	ChannelID string
}

func (c ChannelInfo) SessionName() string {
	return "peer-channel-info"
}

func (c ChannelInfo) Args() []string {
	return []string{
		"channel", "getinfo",
		"-c", c.ChannelID,
	}
}

type LoggingSetLevel struct {
	Logger string
	Level  string
}

func (l LoggingSetLevel) SessionName() string {
	return "peer-logging-setlevel"
}

func (l LoggingSetLevel) Args() []string {
	return []string{
		"logging", "setlevel", l.Logger, l.Level,
	}
}
