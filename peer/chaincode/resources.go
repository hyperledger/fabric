/*
Copyright IBM Corp. 2016-2017 All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package chaincode

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"os"
	"strings"
	"time"

	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric/common/cauthdsl"
	"github.com/hyperledger/fabric/common/resourcesconfig"
	update2 "github.com/hyperledger/fabric/common/tools/configtxlator/update"
	"github.com/hyperledger/fabric/common/util"
	"github.com/hyperledger/fabric/core/chaincode/shim"
	"github.com/hyperledger/fabric/core/scc/lscc"
	"github.com/hyperledger/fabric/msp"
	"github.com/hyperledger/fabric/protos/common"
	"github.com/hyperledger/fabric/protos/peer"
	"github.com/hyperledger/fabric/protos/utils"
	"github.com/pkg/errors"
	"golang.org/x/net/context"
)

type channelVersion uint8

const (
	v1 = iota
	v11
)

const defaultEndorsementPolicy = "/Channel/Application/Writers"

// SignatureSupport creates signature headers, signs messages,
// and also serializes its identity to bytes
type SignatureSupport interface {
	// Sign the message
	Sign(msg []byte) ([]byte, error)

	// Serialize converts an identity to bytes
	Serialize() ([]byte, error)

	// NewSignatureHeader creates a new signature header
	NewSignatureHeader() (*common.SignatureHeader, error)
}

// deltaComputer computes the delta from a starting config to a target config
type deltaComputer func(original, updated *common.Config) (*common.ConfigUpdate, error)

type sendInitTransaction func() error

type ccUpdate struct {
	policy       *common.SignaturePolicyEnvelope
	computeDelta deltaComputer
	SignatureSupport
	ccName      string
	oldConfig   *common.Config
	newConfig   *common.Config
	chainID     string
	hash        []byte
	validation  string
	endorsement string
	version     string
}

// assembleProposal assembles a SignedProposal given parameters
func assembleProposal(ss SignatureSupport, channel string, targetCC string, function string, args ...string) (*peer.SignedProposal, error) {
	var invocation *peer.ChaincodeInvocationSpec
	if len(args) == 0 {
		invocation = &peer.ChaincodeInvocationSpec{
			ChaincodeSpec: &peer.ChaincodeSpec{
				Type:        peer.ChaincodeSpec_Type(peer.ChaincodeSpec_Type_value["GOLANG"]),
				ChaincodeId: &peer.ChaincodeID{Name: targetCC},
				Input:       &peer.ChaincodeInput{Args: [][]byte{[]byte(function)}},
			},
		}
	} else {
		invocation = &peer.ChaincodeInvocationSpec{
			ChaincodeSpec: &peer.ChaincodeSpec{
				Type:        peer.ChaincodeSpec_Type(peer.ChaincodeSpec_Type_value["GOLANG"]),
				ChaincodeId: &peer.ChaincodeID{Name: targetCC},
				Input:       &peer.ChaincodeInput{Args: [][]byte{[]byte(function), []byte(args[0])}},
			},
		}
	}

	var prop *peer.Proposal
	c, err := ss.Serialize()
	if err != nil {
		return nil, err
	}
	prop, _, err = utils.CreateProposalFromCIS(common.HeaderType_ENDORSER_TRANSACTION, channel, invocation, c)
	if err != nil {
		return nil, errors.New(fmt.Sprintf("Cannot create proposal, due to %s", err))
	}
	return utils.GetSignedProposal(prop, ss)
}

// fetchCCID fetches the ID of the chaincode from LSCC
func fetchCCID(ss SignatureSupport, ec peer.EndorserClient, name, version string) ([]byte, error) {
	sp, err := assembleProposal(ss, "", "lscc", lscc.GETINSTALLEDCHAINCODES)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*5)
	defer cancel()
	resp, err := ec.ProcessProposal(ctx, sp)
	if err != nil {
		return nil, err
	}
	if resp.Response.Status != shim.OK {
		return nil, errors.New(resp.Response.Message)
	}
	qr := &peer.ChaincodeQueryResponse{}
	if err := proto.Unmarshal(resp.Response.Payload, qr); err != nil {
		return nil, err
	}
	for _, cc := range qr.Chaincodes {
		if cc.Name == name && cc.Version == version {
			return cc.Id, nil
		}
	}
	return nil, errors.Errorf("chaincode with name %s and version %s wasn't found", name, version)
}

// fetchResourceConfig fetches the resource config from the peer, if applicable.
// else, it returns the channel version to be v1.0, or an error upon failure
func fetchResourceConfig(ec peer.EndorserClient, ss SignatureSupport, channel string) (channelVersion, *common.Config, error) {
	sp, err := assembleProposal(ss, channel, "cscc", "GetConfigTree", channel)
	if err != nil {
		logger.Warning("Cannot determine peer version, Proposal assembly failed:", err)
		return 0, nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*5)
	defer cancel()
	resp, err := ec.ProcessProposal(ctx, sp)

	if err != nil {
		// ProcessProposal may return:
		// Case 1: An error, that isn't associated with CSCC chaincode execution (i.e RPC error)
		if !strings.Contains(err.Error(), "chaincode") {
			logger.Warning("Cannot determine peer version, CSCC returned:", err)
			return 0, nil, err
		}
		// Case 2: An error, that is associated with CSCC chaincode execution
		logger.Debug("Peer returned:", err.Error(), "assuming it to be an lscc channel")
		return v1, nil, nil
	}

	if resp == nil || resp.Response == nil {
		return 0, nil, errors.Errorf("empty response")
	}

	// Case 3: A response, with response of a non OK status
	if resp.Response.Status != shim.OK {
		logger.Debug("Peer returned:", resp.Response, "assuming it to be an lscc channel")
		return v1, nil, nil
	}
	conf := &peer.ConfigTree{}

	// Case 4: A response, with an OK status but with an invalid Config payload
	if err := proto.Unmarshal(resp.Response.Payload, conf); err != nil || conf.ResourcesConfig.ChannelGroup == nil {
		logger.Warning("Peer returned malformed config", conf)
		return v1, nil, nil
	}

	// case 5: A response with an OK status but with a nil groups inside the channel group,
	// or with a missing chaincodeGroupKey
	if g := conf.ResourcesConfig.ChannelGroup.Groups; g == nil || g[resourcesconfig.ChaincodesGroupKey] == nil {
		return v1, nil, nil
	}

	// Case 6: A response, with an OK status and a valid config
	logger.Debug("Peer returned config, assuming it is a config based lifecycle channel")
	return v11, conf.ResourcesConfig, nil
}

func ccGroup(ccName string, version string, validation string, endorsement string, hash []byte, modpolicies map[string]string, policy *common.SignaturePolicyEnvelope) *common.ConfigGroup {
	logger.Infof("creating ccGroup using %s and %s", validation, endorsement)
	var vsccArg []byte
	appendConfigPolicy := true
	if validation == "" {
		validation = "vscc"
	}
	if endorsement == "" {
		endorsement = "escc"
	}
	if validation == "static-endorsement-policy" {
		if policy == nil {
			appendConfigPolicy = false
			logger.Info("Policy not specified, defaulting to", defaultEndorsementPolicy)
			vsccArg = utils.MarshalOrPanic(&peer.VSCCArgs{
				EndorsementPolicyRef: defaultEndorsementPolicy,
			})
		} else {
			vsccArg = utils.MarshalOrPanic(&peer.VSCCArgs{
				EndorsementPolicyRef: fmt.Sprintf("/Resources/Chaincodes/%s/Endorsement", ccName),
			})
		}
	}
	if validation == "vscc" {
		logger.Infof("Setting VSCC arg to simple policy, using %s and %s", validation, endorsement)
		vsccArg = utils.MarshalOrPanic(policy)
	}
	cfgGrp := &common.ConfigGroup{
		ModPolicy: modpolicies["Base"],
		Values: map[string]*common.ConfigValue{
			// TODO: make a constant in some other package
			"ChaincodeIdentifier": {
				ModPolicy: modpolicies["ChaincodeIdentifier"],
				Value: utils.MarshalOrPanic(&peer.ChaincodeIdentifier{
					Version: version,
					Hash:    hash,
				}),
			},
			// TODO: make a constant in some other package
			"ChaincodeValidation": {
				ModPolicy: modpolicies["ChaincodeValidation"],
				Value: utils.MarshalOrPanic(&peer.ChaincodeValidation{
					Name:     validation,
					Argument: vsccArg,
				}),
			},
			// TODO: make a constant in some other package
			"ChaincodeEndorsement": {
				ModPolicy: modpolicies["ChaincodeEndorsement"],
				Value: utils.MarshalOrPanic(&peer.ChaincodeEndorsement{
					Name: endorsement,
				}),
			},
		},
	}
	if appendConfigPolicy {
		cfgGrp.Policies = map[string]*common.ConfigPolicy{
			// TODO: make a constant in some other package
			"Endorsement": {
				ModPolicy: modpolicies["[Policy] Endorsement"],
				Policy: &common.Policy{
					Type:  int32(common.Policy_SIGNATURE),
					Value: utils.MarshalOrPanic(policy),
				},
			},
		}
	}
	return cfgGrp
}

func (update ccUpdate) addChaincode() {
	ccGrp, exists := update.newConfig.ChannelGroup.Groups[resourcesconfig.ChaincodesGroupKey]
	if !exists {
		// We shouldn't reach here, because we classify such a resource config as a v1 channel,
		// thus we shouldn't even attempt to modify the chaincode definitions, hence not reach this function.
		logger.Panic("Programming error: chaincodes group doesn't exist")
	}
	// In case no Groups, make our own
	if ccGrp.Groups == nil {
		logger.Debug("Creating a Groups key for", resourcesconfig.ChaincodesGroupKey)
		ccGrp.Groups = make(map[string]*common.ConfigGroup)
	}
	modPolicies := getModPolicies(ccGrp, update)
	ccg := ccGroup(update.ccName, update.version, update.validation, update.endorsement, update.hash, modPolicies, update.policy)
	ccGrp.Groups[update.ccName] = ccg
}

func (update ccUpdate) newConfigUpdate(cfgUpdate *common.ConfigUpdate) *common.ConfigUpdateEnvelope {
	newConfigUpdateEnv := &common.ConfigUpdateEnvelope{
		ConfigUpdate: utils.MarshalOrPanic(cfgUpdate),
	}
	update.appendSignature(newConfigUpdateEnv)
	return newConfigUpdateEnv
}

func (update ccUpdate) appendSignature(env *common.ConfigUpdateEnvelope) *common.ConfigUpdateEnvelope {
	sigHdr, err := update.NewSignatureHeader()
	if err != nil {
		logger.Panic(err)
	}
	// First iterate over the signatures and see if we already signed this envelope
	for _, sig := range env.Signatures {
		sh, err := utils.GetSignatureHeader(sig.SignatureHeader)
		if err != nil {
			logger.Panicf("signature header invalid: %v", err)
		}
		logger.Info("Found our own signature header, skipping appending our signature...")
		if bytes.Equal(sigHdr.Creator, sh.Creator) {
			return env
		}
	}
	configSig := &common.ConfigSignature{
		SignatureHeader: utils.MarshalOrPanic(sigHdr),
	}
	configSig.Signature, err = update.Sign(util.ConcatenateBytes(configSig.SignatureHeader, env.ConfigUpdate))
	if err != nil {
		logger.Panicf("failed signing config update: %v", err)
	}
	env.Signatures = append(env.Signatures, configSig)
	return env
}

func (update ccUpdate) updateIntoEnvelope(updateEnv *common.ConfigUpdateEnvelope) *common.Envelope {
	env, err := utils.CreateSignedEnvelope(common.HeaderType_PEER_RESOURCE_UPDATE, update.chainID, update, updateEnv, 0, 0)
	if err != nil {
		logger.Panic(err)
	}
	return env
}

func (update ccUpdate) buildCCUpdateEnvelope() *common.Envelope {
	update.newConfig = proto.Clone(update.oldConfig).(*common.Config)
	update.addChaincode()
	cfgUpdate, err := update.computeDelta(update.oldConfig, update.newConfig)
	if err != nil {
		logger.Panic(err)
	}
	cfgUpdate.ChannelId = update.chainID
	newConfigUpdateEnv := update.newConfigUpdate(cfgUpdate)
	return update.updateIntoEnvelope(newConfigUpdateEnv)
}

func getModPolicies(ccGrp *common.ConfigGroup, update ccUpdate) map[string]string {
	// By default, it's the channel admins
	modPolicies := map[string]string{
		"Base":                 "/Channel/Application/Admins",
		"ChaincodeIdentifier":  "/Channel/Application/Admins",
		"ChaincodeValidation":  "/Channel/Application/Admins",
		"ChaincodeEndorsement": "/Channel/Application/Admins",
		"[Policy] Endorsement": "/Channel/Application/Admins",
	}
	// If the chaincode has already been configured before,
	// we would want (by default) to preserve its modification policy
	if oldChaincodeGroup := ccGrp.Groups[update.ccName]; oldChaincodeGroup != nil {
		modPolicies["Base"] = oldChaincodeGroup.ModPolicy
		modPolicies["ChaincodeIdentifier"] = oldChaincodeGroup.Values["ChaincodeIdentifier"].ModPolicy
		modPolicies["ChaincodeValidation"] = oldChaincodeGroup.Values["ChaincodeValidation"].ModPolicy
		modPolicies["ChaincodeEndorsement"] = oldChaincodeGroup.Values["ChaincodeEndorsement"].ModPolicy
		modPolicies["[Policy] Endorsement"] = oldChaincodeGroup.Policies["Endorsement"].ModPolicy
	}
	return modPolicies
}

func configBasedLifecycleUpdate(ss *sigSupport, cf *ChaincodeCmdFactory, config *common.Config, sendInit sendInitTransaction) error {
	var env *common.Envelope
	hash, err := fetchCCID(ss, cf.EndorserClient, chaincodeName, chaincodeVersion)
	if err != nil {
		return err
	}
	var pol *common.SignaturePolicyEnvelope
	if policy != "" {
		pol, err = cauthdsl.FromString(policy)
		if err != nil {
			return err
		}
	}

	if policy == "" && (vscc == "vscc" || vscc == "") {
		return errors.New("policy must be specified when vscc flag is set to 'vscc' or missing")
	}

	update := ccUpdate{
		policy:           pol,
		computeDelta:     update2.Compute,
		ccName:           chaincodeName,
		oldConfig:        config,
		version:          chaincodeVersion,
		endorsement:      escc,
		validation:       vscc,
		hash:             hash,
		chainID:          channelID,
		SignatureSupport: &sigSupport{cf.Signer},
	}
	if resourceEnvelopeLoadPath == "" {
		// We're making a new config update
		env = update.buildCCUpdateEnvelope()
	} else {
		// We're loading the config update from disk
		updateEnv, err := loadEnvelope(resourceEnvelopeLoadPath)
		if err != nil {
			return err
		}
		// and appending our signature to it
		updateEnv = update.appendSignature(updateEnv)
		// and putting it back into an envelope
		env = update.updateIntoEnvelope(updateEnv)
	}
	if resourceEnvelopeSavePath != "" {
		return saveEnvelope(env)
	}
	if err := cf.BroadcastClient.Send(env); err != nil {
		return err
	}
	return sendInit()
}

func loadEnvelope(file string) (*common.ConfigUpdateEnvelope, error) {
	data, err := ioutil.ReadFile(file)
	if err != nil {
		return nil, err
	}
	env := &common.Envelope{}
	if err := proto.Unmarshal(data, env); err != nil {
		return nil, err
	}
	payload := &common.Payload{}
	if err := proto.Unmarshal(env.Payload, payload); err != nil {
		return nil, err
	}
	update := &common.ConfigUpdateEnvelope{}
	if err := proto.Unmarshal(payload.Data, update); err != nil {
		return nil, err
	}
	return update, nil
}

func saveEnvelope(env *common.Envelope) error {
	f, err := os.Create(resourceEnvelopeSavePath)
	if err != nil {
		return errors.Errorf("failed saving resource envelope to file %s: %v", resourceEnvelopeSavePath, err)
	}
	if _, err := f.Write(utils.MarshalOrPanic(env)); err != nil {
		return errors.Errorf("failed saving resource envelope to file %s: %v", resourceEnvelopeSavePath, err)
	}
	fmt.Printf(`Saved config update envelope to %s.
			You can now either:
			1) Append your signature using --resourceEnvelopeSave along with --resourceEnvelopeLoad
			2) Submit a transaction using only --resourceEnvelopeLoad
			`, f.Name())
	return nil
}

type sigSupport struct {
	msp.SigningIdentity
}

// NewSignatureHeader creates a new signature header
func (s *sigSupport) NewSignatureHeader() (*common.SignatureHeader, error) {
	sID, err := s.Serialize()
	if err != nil {
		return nil, err
	}
	return utils.MakeSignatureHeader(sID, utils.CreateNonceOrPanic()), nil
}
