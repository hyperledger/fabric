/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package commands

import (
	"strconv"
	"time"
)

type NodeStart struct {
	PeerID  string
	DevMode bool
}

func (n NodeStart) SessionName() string {
	return n.PeerID
}

func (n NodeStart) Args() []string {
	args := []string{"node", "start"}
	if n.DevMode {
		args = append(args, "--peer-chaincodedev")
	}
	return args
}

type NodeReset struct{}

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

type NodePause struct {
	ChannelID string
}

func (n NodePause) SessionName() string {
	return "peer-node-pause"
}

func (n NodePause) Args() []string {
	return []string{
		"node", "pause",
		"--channelID", n.ChannelID,
	}
}

type NodeResume struct {
	ChannelID string
}

func (n NodeResume) SessionName() string {
	return "peer-node-resume"
}

func (n NodeResume) Args() []string {
	return []string{
		"node", "resume",
		"--channelID", n.ChannelID,
	}
}

type NodeUnjoin struct {
	ChannelID string
}

func (n NodeUnjoin) SessionName() string {
	return "peer-node-unjoin"
}

func (n NodeUnjoin) Args() []string {
	return []string{
		"node", "unjoin",
		"--channelID", n.ChannelID,
	}
}

type ChannelCreate struct {
	ChannelID   string
	Orderer     string
	File        string
	OutputBlock string
	ClientAuth  bool
}

func (c ChannelCreate) SessionName() string {
	return "peer-channel-create"
}

func (c ChannelCreate) Args() []string {
	args := []string{
		"channel", "create",
		"--channelID", c.ChannelID,
		"--orderer", c.Orderer,
		"--file", c.File,
		"--outputBlock", c.OutputBlock,
		"--timeout", "15s",
	}
	if c.ClientAuth {
		args = append(args, "--clientauth")
	}
	return args
}

type ChannelJoin struct {
	BlockPath  string
	ClientAuth bool
}

func (c ChannelJoin) SessionName() string {
	return "peer-channel-join"
}

func (c ChannelJoin) Args() []string {
	args := []string{
		"channel", "join",
		"-b", c.BlockPath,
	}
	if c.ClientAuth {
		args = append(args, "--clientauth")
	}
	return args
}

type ChannelJoinBySnapshot struct {
	SnapshotPath string
	ClientAuth   bool
}

func (c ChannelJoinBySnapshot) SessionName() string {
	return "peer-channel-joinbysnapshot"
}

func (c ChannelJoinBySnapshot) Args() []string {
	args := []string{
		"channel", "joinbysnapshot",
		"--snapshotpath", c.SnapshotPath,
	}
	if c.ClientAuth {
		args = append(args, "--clientauth")
	}
	return args
}

type ChannelJoinBySnapshotStatus struct {
	ClientAuth bool
}

func (c ChannelJoinBySnapshotStatus) SessionName() string {
	return "peer-channel-joinbysnapshotstatus"
}

func (c ChannelJoinBySnapshotStatus) Args() []string {
	args := []string{
		"channel", "joinbysnapshotstatus",
	}
	if c.ClientAuth {
		args = append(args, "--clientauth")
	}
	return args
}

type ChannelFetch struct {
	ChannelID             string
	Block                 string
	Orderer               string
	OutputFile            string
	ClientAuth            bool
	TLSHandshakeTimeShift time.Duration
}

func (c ChannelFetch) SessionName() string {
	return "peer-channel-fetch"
}

func (c ChannelFetch) Args() []string {
	args := []string{
		"channel", "fetch", c.Block,
		"--channelID", c.ChannelID,
		"--orderer", c.Orderer,
		"--tlsHandshakeTimeShift", c.TLSHandshakeTimeShift.String(),
		c.OutputFile,
	}
	if c.ClientAuth {
		args = append(args, "--clientauth")
	}

	return args
}

type ChaincodePackage struct {
	Path       string
	Lang       string
	Label      string
	OutputFile string
	ClientAuth bool
}

func (c ChaincodePackage) SessionName() string {
	return "peer-lifecycle-chaincode-package"
}

func (c ChaincodePackage) Args() []string {
	args := []string{
		"lifecycle", "chaincode", "package",
		"--path", c.Path,
		"--lang", c.Lang,
		"--label", c.Label,
		c.OutputFile,
	}

	if c.ClientAuth {
		args = append(args, "--clientauth")
	}

	return args
}

type ChaincodePackageLegacy struct {
	Name       string
	Version    string
	Path       string
	Lang       string
	OutputFile string
	ClientAuth bool
}

func (c ChaincodePackageLegacy) SessionName() string {
	return "peer-chaincode-package"
}

func (c ChaincodePackageLegacy) Args() []string {
	args := []string{
		"chaincode", "package",
		"--name", c.Name,
		"--version", c.Version,
		"--path", c.Path,
		c.OutputFile,
	}

	if c.ClientAuth {
		args = append(args, "--clientauth")
	}
	if c.Lang != "" {
		args = append(args, "--lang", c.Lang)
	}

	return args
}

type ChaincodeCalculatePackageID struct {
	PackageFile string
	ClientAuth  bool
}

func (c ChaincodeCalculatePackageID) SessionName() string {
	return "peer-lifecycle-chaincode-calculatepackageid"
}

func (c ChaincodeCalculatePackageID) Args() []string {
	args := []string{
		"lifecycle", "chaincode", "calculatepackageid",
		c.PackageFile,
	}

	if c.ClientAuth {
		args = append(args, "--clientauth")
	}

	return args
}

type ChaincodeInstall struct {
	PackageFile   string
	PeerAddresses []string
	ClientAuth    bool
}

func (c ChaincodeInstall) SessionName() string {
	return "peer-lifecycle-chaincode-install"
}

func (c ChaincodeInstall) Args() []string {
	args := []string{
		"lifecycle", "chaincode", "install",
		c.PackageFile,
	}

	for _, p := range c.PeerAddresses {
		args = append(args, "--peerAddresses", p)
	}
	if c.ClientAuth {
		args = append(args, "--clientauth")
	}

	return args
}

type ChaincodeGetInstalledPackage struct {
	PackageID       string
	OutputDirectory string
	ClientAuth      bool
}

func (c ChaincodeGetInstalledPackage) SessionName() string {
	return "peer-lifecycle-chaincode-getinstalledpackage"
}

func (c ChaincodeGetInstalledPackage) Args() []string {
	args := []string{
		"lifecycle", "chaincode", "getinstalledpackage",
		"--package-id", c.PackageID,
		"--output-directory", c.OutputDirectory,
	}
	if c.ClientAuth {
		args = append(args, "--clientauth")
	}

	return args
}

type ChaincodeInstallLegacy struct {
	Name        string
	Version     string
	Path        string
	Lang        string
	PackageFile string
	ClientAuth  bool
}

func (c ChaincodeInstallLegacy) SessionName() string {
	return "peer-chaincode-install"
}

func (c ChaincodeInstallLegacy) Args() []string {
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

	if c.ClientAuth {
		args = append(args, "--clientauth")
	}
	return args
}

type ChaincodeApproveForMyOrg struct {
	ChannelID           string
	Orderer             string
	Name                string
	Version             string
	PackageID           string
	Sequence            string
	EndorsementPlugin   string
	ValidationPlugin    string
	SignaturePolicy     string
	ChannelConfigPolicy string
	InitRequired        bool
	CollectionsConfig   string
	PeerAddresses       []string
	WaitForEvent        bool
	ClientAuth          bool
}

func (c ChaincodeApproveForMyOrg) SessionName() string {
	return "peer-lifecycle-chaincode-approveformyorg"
}

func (c ChaincodeApproveForMyOrg) Args() []string {
	args := []string{
		"lifecycle", "chaincode", "approveformyorg",
		"--channelID", c.ChannelID,
		"--orderer", c.Orderer,
		"--name", c.Name,
		"--version", c.Version,
		"--package-id", c.PackageID,
		"--sequence", c.Sequence,
		"--endorsement-plugin", c.EndorsementPlugin,
		"--validation-plugin", c.ValidationPlugin,
		"--signature-policy", c.SignaturePolicy,
		"--channel-config-policy", c.ChannelConfigPolicy,
	}

	if c.InitRequired {
		args = append(args, "--init-required")
	}

	if c.CollectionsConfig != "" {
		args = append(args, "--collections-config", c.CollectionsConfig)
	}

	if c.ClientAuth {
		args = append(args, "--clientauth")
	}

	for _, p := range c.PeerAddresses {
		args = append(args, "--peerAddresses", p)
	}

	return args
}

type ChaincodeQueryApproved struct {
	ChannelID     string
	Name          string
	Sequence      string
	PeerAddresses []string
	ClientAuth    bool
}

func (c ChaincodeQueryApproved) SessionName() string {
	return "peer-lifecycle-chaincode-queryapproved"
}

func (c ChaincodeQueryApproved) Args() []string {
	args := []string{
		"lifecycle", "chaincode", "queryapproved",
		"--channelID", c.ChannelID,
		"--name", c.Name,
		"--sequence", c.Sequence,
		"--output", "json",
	}
	for _, p := range c.PeerAddresses {
		args = append(args, "--peerAddresses", p)
	}
	if c.ClientAuth {
		args = append(args, "--clientauth")
	}

	return args
}

type ChaincodeCheckCommitReadiness struct {
	ChannelID           string
	Name                string
	Version             string
	Sequence            string
	EndorsementPlugin   string
	ValidationPlugin    string
	SignaturePolicy     string
	ChannelConfigPolicy string
	InitRequired        bool
	CollectionsConfig   string
	PeerAddresses       []string
	ClientAuth          bool
}

func (c ChaincodeCheckCommitReadiness) SessionName() string {
	return "peer-lifecycle-chaincode-checkcommitreadiness"
}

func (c ChaincodeCheckCommitReadiness) Args() []string {
	args := []string{
		"lifecycle", "chaincode", "checkcommitreadiness",
		"--channelID", c.ChannelID,
		"--name", c.Name,
		"--version", c.Version,
		"--sequence", c.Sequence,
		"--endorsement-plugin", c.EndorsementPlugin,
		"--validation-plugin", c.ValidationPlugin,
		"--signature-policy", c.SignaturePolicy,
		"--channel-config-policy", c.ChannelConfigPolicy,
		"--output", "json",
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

	if c.ClientAuth {
		args = append(args, "--clientauth")
	}
	return args
}

type ChaincodeCommit struct {
	ChannelID           string
	Orderer             string
	Name                string
	Version             string
	Sequence            string
	EndorsementPlugin   string
	ValidationPlugin    string
	SignaturePolicy     string
	ChannelConfigPolicy string
	InitRequired        bool
	CollectionsConfig   string
	PeerAddresses       []string
	WaitForEvent        bool
	ClientAuth          bool
}

func (c ChaincodeCommit) SessionName() string {
	return "peer-lifecycle-chaincode-commit"
}

func (c ChaincodeCommit) Args() []string {
	args := []string{
		"lifecycle", "chaincode", "commit",
		"--channelID", c.ChannelID,
		"--orderer", c.Orderer,
		"--name", c.Name,
		"--version", c.Version,
		"--sequence", c.Sequence,
		"--endorsement-plugin", c.EndorsementPlugin,
		"--validation-plugin", c.ValidationPlugin,
		"--signature-policy", c.SignaturePolicy,
		"--channel-config-policy", c.ChannelConfigPolicy,
	}
	if c.InitRequired {
		args = append(args, "--init-required")
	}
	for _, p := range c.PeerAddresses {
		args = append(args, "--peerAddresses", p)
	}
	if c.CollectionsConfig != "" {
		args = append(args, "--collections-config", c.CollectionsConfig)
	}
	if c.ClientAuth {
		args = append(args, "--clientauth")
	}
	return args
}

type ChaincodeInstantiateLegacy struct {
	ChannelID         string
	Orderer           string
	Name              string
	Version           string
	Ctor              string
	Policy            string
	Lang              string
	CollectionsConfig string
	ClientAuth        bool
}

func (c ChaincodeInstantiateLegacy) SessionName() string {
	return "peer-chaincode-instantiate"
}

func (c ChaincodeInstantiateLegacy) Args() []string {
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

	if c.ClientAuth {
		args = append(args, "--clientauth")
	}
	return args
}

type ChaincodeQueryInstalled struct {
	ClientAuth bool
}

func (c ChaincodeQueryInstalled) SessionName() string {
	return "peer-lifecycle-chaincode-queryinstalled"
}

func (c ChaincodeQueryInstalled) Args() []string {
	args := []string{
		"lifecycle", "chaincode", "queryinstalled",
		"--output", "json",
	}
	if c.ClientAuth {
		args = append(args, "--clientauth")
	}
	return args
}

type ChaincodeListInstalledLegacy struct {
	ClientAuth bool
}

func (c ChaincodeListInstalledLegacy) SessionName() string {
	return "peer-chaincode-list-installed"
}

func (c ChaincodeListInstalledLegacy) Args() []string {
	args := []string{
		"chaincode", "list", "--installed",
	}
	if c.ClientAuth {
		args = append(args, "--clientauth")
	}
	return args
}

type ChaincodeListCommitted struct {
	ChannelID  string
	Name       string
	ClientAuth bool
}

func (c ChaincodeListCommitted) SessionName() string {
	return "peer-lifecycle-chaincode-querycommitted"
}

func (c ChaincodeListCommitted) Args() []string {
	args := []string{
		"lifecycle", "chaincode", "querycommitted",
		"--channelID", c.ChannelID,
		"--name", c.Name,
		"--output", "json",
	}
	if c.ClientAuth {
		args = append(args, "--clientauth")
	}
	return args
}

type ChaincodeListInstantiatedLegacy struct {
	ChannelID  string
	ClientAuth bool
}

func (c ChaincodeListInstantiatedLegacy) SessionName() string {
	return "peer-chaincode-list-instantiated"
}

func (c ChaincodeListInstantiatedLegacy) Args() []string {
	args := []string{
		"chaincode", "list", "--instantiated",
		"--channelID", c.ChannelID,
	}
	if c.ClientAuth {
		args = append(args, "--clientauth")
	}
	return args
}

type ChaincodeQuery struct {
	ChannelID  string
	Name       string
	Ctor       string
	ClientAuth bool
}

func (c ChaincodeQuery) SessionName() string {
	return "peer-chaincode-query"
}

func (c ChaincodeQuery) Args() []string {
	args := []string{
		"chaincode", "query",
		"--channelID", c.ChannelID,
		"--name", c.Name,
		"--ctor", c.Ctor,
	}
	if c.ClientAuth {
		args = append(args, "--clientauth")
	}
	return args
}

type ChaincodeInvoke struct {
	ChannelID     string
	Orderer       string
	Name          string
	Ctor          string
	Transient     string
	PeerAddresses []string
	WaitForEvent  bool
	IsInit        bool
	ClientAuth    bool
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

	if c.Transient != "" {
		args = append(args, "--transient", c.Transient)
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
	if c.ClientAuth {
		args = append(args, "--clientauth")
	}
	return args
}

type ChaincodeUpgradeLegacy struct {
	Name              string
	Version           string
	Path              string // optional
	ChannelID         string
	Orderer           string
	Ctor              string
	Policy            string
	CollectionsConfig string // optional
	ClientAuth        bool
}

func (c ChaincodeUpgradeLegacy) SessionName() string {
	return "peer-chaincode-upgrade"
}

func (c ChaincodeUpgradeLegacy) Args() []string {
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
	if c.ClientAuth {
		args = append(args, "--clientauth")
	}
	return args
}

type SignConfigTx struct {
	File       string
	ClientAuth bool
}

func (s SignConfigTx) SessionName() string {
	return "peer-channel-signconfigtx"
}

func (s SignConfigTx) Args() []string {
	args := []string{
		"channel", "signconfigtx",
		"--file", s.File,
	}
	if s.ClientAuth {
		args = append(args, "--clientauth")
	}
	return args
}

type ChannelUpdate struct {
	ChannelID             string
	Orderer               string
	File                  string
	ClientAuth            bool
	TLSHandshakeTimeShift time.Duration
}

func (c ChannelUpdate) SessionName() string {
	return "peer-channel-update"
}

func (c ChannelUpdate) Args() []string {
	args := []string{
		"channel", "update",
		"--channelID", c.ChannelID,
		"--orderer", c.Orderer,
		"--file", c.File,
		"--tlsHandshakeTimeShift", c.TLSHandshakeTimeShift.String(),
	}
	if c.ClientAuth {
		args = append(args, "--clientauth")
	}
	return args
}

type ChannelInfo struct {
	ChannelID  string
	ClientAuth bool
}

func (c ChannelInfo) SessionName() string {
	return "peer-channel-info"
}

func (c ChannelInfo) Args() []string {
	args := []string{
		"channel", "getinfo",
		"-c", c.ChannelID,
	}
	if c.ClientAuth {
		args = append(args, "--clientauth")
	}
	return args
}

type SnapshotSubmitRequest struct {
	ChannelID   string
	BlockNumber string
	ClientAuth  bool
	PeerAddress string
}

func (s SnapshotSubmitRequest) SessionName() string {
	return "peer-snapshot-submit"
}

func (s SnapshotSubmitRequest) Args() []string {
	args := []string{
		"snapshot", "submitrequest",
		"--channelID", s.ChannelID,
		"--blockNumber", s.BlockNumber,
		"--peerAddress", s.PeerAddress,
	}
	if s.ClientAuth {
		args = append(args, "--clientauth")
	}
	return args
}

type SnapshotCancelRequest struct {
	ChannelID   string
	BlockNumber string
	ClientAuth  bool
	PeerAddress string
}

func (s SnapshotCancelRequest) SessionName() string {
	return "peer-snapshot-submit"
}

func (s SnapshotCancelRequest) Args() []string {
	args := []string{
		"snapshot", "cancelrequest",
		"--channelID", s.ChannelID,
		"--blockNumber", s.BlockNumber,
		"--peerAddress", s.PeerAddress,
	}
	if s.ClientAuth {
		args = append(args, "--clientauth")
	}
	return args
}

type SnapshotListPending struct {
	ChannelID   string
	ClientAuth  bool
	PeerAddress string
}

func (s SnapshotListPending) SessionName() string {
	return "peer-snapshot-submit"
}

func (s SnapshotListPending) Args() []string {
	args := []string{
		"snapshot", "listpending",
		"--channelID", s.ChannelID,
		"--peerAddress", s.PeerAddress,
	}
	if s.ClientAuth {
		args = append(args, "--clientauth")
	}
	return args
}
