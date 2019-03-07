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

type ChaincodePackageLifecycle struct {
	Path       string
	Lang       string
	Label      string
	OutputFile string
}

func (c ChaincodePackageLifecycle) SessionName() string {
	return "peer-lifecycle-chaincode-package"
}

func (c ChaincodePackageLifecycle) Args() []string {
	args := []string{
		"lifecycle", "chaincode", "package",
		"--path", c.Path,
		"--lang", c.Lang,
		"--label", c.Label,
		c.OutputFile,
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

type ChaincodeInstallLifecycle struct {
	PackageFile string
}

func (c ChaincodeInstallLifecycle) SessionName() string {
	return "peer-lifecycle-chaincode-install"
}

func (c ChaincodeInstallLifecycle) Args() []string {
	args := []string{
		"lifecycle", "chaincode", "install",
		c.PackageFile,
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

type ChaincodeApproveForMyOrgLifecycle struct {
	ChannelID         string
	Orderer           string
	Name              string
	Version           string
	PackageID         string
	Sequence          string
	EndorsementPlugin string
	ValidationPlugin  string
	Policy            string
	InitRequired      bool
	CollectionsConfig string
	PeerAddresses     []string
	WaitForEvent      bool
}

func (c ChaincodeApproveForMyOrgLifecycle) SessionName() string {
	return "peer-lifecycle-chaincode-approveformyorg"
}

func (c ChaincodeApproveForMyOrgLifecycle) Args() []string {
	args := []string{
		"lifecycle", "chaincode", "approveformyorg",
		"--channelID", c.ChannelID,
		"--orderer", c.Orderer,
		"--name", c.Name,
		"--version", c.Version,
		"--package-id", c.PackageID,
		"--sequence", c.Sequence,
		"--escc", c.EndorsementPlugin,
		"--vscc", c.ValidationPlugin,
		"--policy", c.Policy,
	}

	if c.InitRequired {
		args = append(args, "--init-required")
	}

	if c.CollectionsConfig != "" {
		args = append(args, "--collections-config", c.CollectionsConfig)
	}

	for _, p := range c.PeerAddresses {
		args = append(args, "--peerAddresses", p)
	}

	if c.WaitForEvent {
		args = append(args, "--waitForEvent")
	}

	return args
}

type ChaincodeCommitLifecycle struct {
	ChannelID         string
	Orderer           string
	Name              string
	Version           string
	Sequence          string
	EndorsementPlugin string
	ValidationPlugin  string
	Policy            string
	InitRequired      bool
	CollectionsConfig string
	PeerAddresses     []string
	WaitForEvent      bool
}

func (c ChaincodeCommitLifecycle) SessionName() string {
	return "peer-lifecycle-chaincode-commit"
}

func (c ChaincodeCommitLifecycle) Args() []string {
	args := []string{
		"lifecycle", "chaincode", "commit",
		"--channelID", c.ChannelID,
		"--orderer", c.Orderer,
		"--name", c.Name,
		"--version", c.Version,
		"--sequence", c.Sequence,
		"--escc", c.EndorsementPlugin,
		"--vscc", c.ValidationPlugin,
		"--policy", c.Policy,
	}
	if c.InitRequired {
		args = append(args, "--init-required")
	}
	for _, p := range c.PeerAddresses {
		args = append(args, "--peerAddresses", p)
	}
	if c.WaitForEvent {
		args = append(args, "--waitForEvent")
	}
	if c.CollectionsConfig != "" {
		args = append(args, "--collections-config", c.CollectionsConfig)
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

type ChaincodeQueryInstalledLifecycle struct{}

func (c ChaincodeQueryInstalledLifecycle) SessionName() string {
	return "peer-lifecycle-chaincode-queryinstalled"
}

func (c ChaincodeQueryInstalledLifecycle) Args() []string {
	return []string{
		"lifecycle", "chaincode", "queryinstalled",
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

type ChaincodeListCommittedLifecycle struct {
	ChannelID string
	Name      string
}

func (c ChaincodeListCommittedLifecycle) SessionName() string {
	return "peer-lifecycle-chaincode-querycommitted"
}

func (c ChaincodeListCommittedLifecycle) Args() []string {
	return []string{
		"lifecycle", "chaincode", "querycommitted",
		"--channelID", c.ChannelID,
		"--name", c.Name,
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
	IsInit        bool
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
	if c.IsInit {
		args = append(args, "--isInit")
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
