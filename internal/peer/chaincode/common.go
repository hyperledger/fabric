/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package chaincode

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"strings"
	"sync"

	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric-chaincode-go/shim"
	pcommon "github.com/hyperledger/fabric-protos-go/common"
	ab "github.com/hyperledger/fabric-protos-go/orderer"
	pb "github.com/hyperledger/fabric-protos-go/peer"
	"github.com/hyperledger/fabric/bccsp"
	"github.com/hyperledger/fabric/common/policydsl"
	"github.com/hyperledger/fabric/common/util"
	"github.com/hyperledger/fabric/internal/peer/common"
	"github.com/hyperledger/fabric/internal/pkg/identity"
	"github.com/hyperledger/fabric/protoutil"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

// checkSpec to see if chaincode resides within current package capture for language.
func checkSpec(spec *pb.ChaincodeSpec) error {
	// Don't allow nil value
	if spec == nil {
		return errors.New("expected chaincode specification, nil received")
	}
	if spec.ChaincodeId == nil {
		return errors.New("expected chaincode ID, nil received")
	}

	return platformRegistry.ValidateSpec(spec.Type.String(), spec.ChaincodeId.Path)
}

// getChaincodeDeploymentSpec get chaincode deployment spec given the chaincode spec
func getChaincodeDeploymentSpec(spec *pb.ChaincodeSpec, crtPkg bool) (*pb.ChaincodeDeploymentSpec, error) {
	var codePackageBytes []byte
	if crtPkg {
		var err error
		if err = checkSpec(spec); err != nil {
			return nil, err
		}

		codePackageBytes, err = platformRegistry.GetDeploymentPayload(spec.Type.String(), spec.ChaincodeId.Path)
		if err != nil {
			return nil, errors.WithMessage(err, "error getting chaincode package bytes")
		}
		chaincodePath, err := platformRegistry.NormalizePath(spec.Type.String(), spec.ChaincodeId.Path)
		if err != nil {
			return nil, errors.WithMessage(err, "failed to normalize chaincode path")
		}
		spec.ChaincodeId.Path = chaincodePath
	}

	return &pb.ChaincodeDeploymentSpec{ChaincodeSpec: spec, CodePackage: codePackageBytes}, nil
}

// getChaincodeSpec get chaincode spec from the cli cmd parameters
func getChaincodeSpec(cmd *cobra.Command) (*pb.ChaincodeSpec, error) {
	spec := &pb.ChaincodeSpec{}
	if err := checkChaincodeCmdParams(cmd); err != nil {
		// unset usage silence because it's a command line usage error
		cmd.SilenceUsage = false
		return spec, err
	}

	// Build the spec
	input := chaincodeInput{}
	if err := json.Unmarshal([]byte(chaincodeCtorJSON), &input); err != nil {
		return spec, errors.Wrap(err, "chaincode argument error")
	}
	input.IsInit = isInit

	chaincodeLang = strings.ToUpper(chaincodeLang)
	spec = &pb.ChaincodeSpec{
		Type:        pb.ChaincodeSpec_Type(pb.ChaincodeSpec_Type_value[chaincodeLang]),
		ChaincodeId: &pb.ChaincodeID{Path: chaincodePath, Name: chaincodeName, Version: chaincodeVersion},
		Input:       &input.ChaincodeInput,
	}
	return spec, nil
}

// chaincodeInput is wrapper around the proto defined ChaincodeInput message that
// is decorated with a custom JSON unmarshaller.
type chaincodeInput struct {
	pb.ChaincodeInput
}

// UnmarshalJSON converts the string-based REST/JSON input to
// the []byte-based current ChaincodeInput structure.
func (c *chaincodeInput) UnmarshalJSON(b []byte) error {
	sa := struct {
		Function string
		Args     []string
	}{}
	err := json.Unmarshal(b, &sa)
	if err != nil {
		return err
	}
	allArgs := sa.Args
	if sa.Function != "" {
		allArgs = append([]string{sa.Function}, sa.Args...)
	}
	c.Args = util.ToChaincodeArgs(allArgs...)
	return nil
}

func chaincodeInvokeOrQuery(cmd *cobra.Command, invoke bool, cf *ChaincodeCmdFactory) (err error) {
	spec, err := getChaincodeSpec(cmd)
	if err != nil {
		return err
	}

	// call with empty txid to ensure production code generates a txid.
	// otherwise, tests can explicitly set their own txid
	txID := ""

	proposalResp, err := ChaincodeInvokeOrQuery(
		spec,
		channelID,
		txID,
		invoke,
		cf.Signer,
		cf.Certificate,
		cf.EndorserClients,
		cf.DeliverClients,
		cf.BroadcastClient,
	)
	if err != nil {
		return errors.Errorf("%s - proposal response: %v", err, proposalResp)
	}

	if invoke {
		logger.Debugf("ESCC invoke result: %v", proposalResp)
		pRespPayload, err := protoutil.UnmarshalProposalResponsePayload(proposalResp.Payload)
		if err != nil {
			return errors.WithMessage(err, "error while unmarshalling proposal response payload")
		}
		ca, err := protoutil.UnmarshalChaincodeAction(pRespPayload.Extension)
		if err != nil {
			return errors.WithMessage(err, "error while unmarshalling chaincode action")
		}
		if proposalResp.Endorsement == nil {
			return errors.Errorf("endorsement failure during invoke. response: %v", proposalResp.Response)
		}
		logger.Infof("Chaincode invoke successful. result: %v", ca.Response)
	} else {
		if proposalResp == nil {
			return errors.New("error during query: received nil proposal response")
		}
		if proposalResp.Endorsement == nil {
			return errors.Errorf("endorsement failure during query. response: %v", proposalResp.Response)
		}

		if chaincodeQueryRaw && chaincodeQueryHex {
			return fmt.Errorf("options --raw (-r) and --hex (-x) are not compatible")
		}
		if chaincodeQueryRaw {
			fmt.Println(proposalResp.Response.Payload)
			return nil
		}
		if chaincodeQueryHex {
			fmt.Printf("%x\n", proposalResp.Response.Payload)
			return nil
		}
		fmt.Println(string(proposalResp.Response.Payload))
	}
	return nil
}

type endorsementPolicy struct {
	ChannelConfigPolicy string `json:"channelConfigPolicy,omitempty"`
	SignaturePolicy     string `json:"signaturePolicy,omitempty"`
}

type collectionConfigJson struct {
	Name              string             `json:"name"`
	Policy            string             `json:"policy"`
	RequiredPeerCount *int32             `json:"requiredPeerCount"`
	MaxPeerCount      *int32             `json:"maxPeerCount"`
	BlockToLive       uint64             `json:"blockToLive"`
	MemberOnlyRead    bool               `json:"memberOnlyRead"`
	MemberOnlyWrite   bool               `json:"memberOnlyWrite"`
	EndorsementPolicy *endorsementPolicy `json:"endorsementPolicy,omitempty"`
}

// GetCollectionConfigFromFile retrieves the collection configuration
// from the supplied file; the supplied file must contain a
// json-formatted array of collectionConfigJson elements
func GetCollectionConfigFromFile(ccFile string) (*pb.CollectionConfigPackage, []byte, error) {
	fileBytes, err := os.ReadFile(ccFile)
	if err != nil {
		return nil, nil, errors.Wrapf(err, "could not read file '%s'", ccFile)
	}

	return getCollectionConfigFromBytes(fileBytes)
}

// getCollectionConfig retrieves the collection configuration
// from the supplied byte array; the byte array must contain a
// json-formatted array of collectionConfigJson elements
func getCollectionConfigFromBytes(cconfBytes []byte) (*pb.CollectionConfigPackage, []byte, error) {
	cconf := &[]collectionConfigJson{}
	err := json.Unmarshal(cconfBytes, cconf)
	if err != nil {
		return nil, nil, errors.Wrap(err, "could not parse the collection configuration")
	}

	ccarray := make([]*pb.CollectionConfig, 0, len(*cconf))
	for _, cconfitem := range *cconf {
		p, err := policydsl.FromString(cconfitem.Policy)
		if err != nil {
			return nil, nil, errors.WithMessagef(err, "invalid policy %s", cconfitem.Policy)
		}

		cpc := &pb.CollectionPolicyConfig{
			Payload: &pb.CollectionPolicyConfig_SignaturePolicy{
				SignaturePolicy: p,
			},
		}

		var ep *pb.ApplicationPolicy
		if cconfitem.EndorsementPolicy != nil {
			signaturePolicy := cconfitem.EndorsementPolicy.SignaturePolicy
			channelConfigPolicy := cconfitem.EndorsementPolicy.ChannelConfigPolicy
			ep, err = getApplicationPolicy(signaturePolicy, channelConfigPolicy)
			if err != nil {
				return nil, nil, errors.WithMessagef(err, "invalid endorsement policy [%#v]", cconfitem.EndorsementPolicy)
			}
		}

		// Set default requiredPeerCount and MaxPeerCount if not specified in json
		requiredPeerCount := int32(0)
		maxPeerCount := int32(1)
		if cconfitem.RequiredPeerCount != nil {
			requiredPeerCount = *cconfitem.RequiredPeerCount
		}
		if cconfitem.MaxPeerCount != nil {
			maxPeerCount = *cconfitem.MaxPeerCount
		}

		cc := &pb.CollectionConfig{
			Payload: &pb.CollectionConfig_StaticCollectionConfig{
				StaticCollectionConfig: &pb.StaticCollectionConfig{
					Name:              cconfitem.Name,
					MemberOrgsPolicy:  cpc,
					RequiredPeerCount: requiredPeerCount,
					MaximumPeerCount:  maxPeerCount,
					BlockToLive:       cconfitem.BlockToLive,
					MemberOnlyRead:    cconfitem.MemberOnlyRead,
					MemberOnlyWrite:   cconfitem.MemberOnlyWrite,
					EndorsementPolicy: ep,
				},
			},
		}

		ccarray = append(ccarray, cc)
	}

	ccp := &pb.CollectionConfigPackage{Config: ccarray}
	ccpBytes, err := proto.Marshal(ccp)
	return ccp, ccpBytes, err
}

func getApplicationPolicy(signaturePolicy, channelConfigPolicy string) (*pb.ApplicationPolicy, error) {
	if signaturePolicy == "" && channelConfigPolicy == "" {
		// no policy, no problem
		return nil, nil
	}

	if signaturePolicy != "" && channelConfigPolicy != "" {
		// mo policies, mo problems
		return nil, errors.New(`cannot specify both "--signature-policy" and "--channel-config-policy"`)
	}

	var applicationPolicy *pb.ApplicationPolicy
	if signaturePolicy != "" {
		signaturePolicyEnvelope, err := policydsl.FromString(signaturePolicy)
		if err != nil {
			return nil, errors.Errorf("invalid signature policy: %s", signaturePolicy)
		}

		applicationPolicy = &pb.ApplicationPolicy{
			Type: &pb.ApplicationPolicy_SignaturePolicy{
				SignaturePolicy: signaturePolicyEnvelope,
			},
		}
	}

	if channelConfigPolicy != "" {
		applicationPolicy = &pb.ApplicationPolicy{
			Type: &pb.ApplicationPolicy_ChannelConfigPolicyReference{
				ChannelConfigPolicyReference: channelConfigPolicy,
			},
		}
	}

	return applicationPolicy, nil
}

func checkChaincodeCmdParams(cmd *cobra.Command) error {
	// we need chaincode name for everything, including deploy
	if chaincodeName == common.UndefinedParamValue {
		return errors.Errorf("must supply value for %s name parameter", chainFuncName)
	}

	if cmd.Name() == instantiateCmdName || cmd.Name() == installCmdName ||
		cmd.Name() == upgradeCmdName || cmd.Name() == packageCmdName {
		if chaincodeVersion == common.UndefinedParamValue {
			return errors.Errorf("chaincode version is not provided for %s", cmd.Name())
		}

		if escc != common.UndefinedParamValue {
			logger.Infof("Using escc %s", escc)
		} else {
			logger.Info("Using default escc")
			escc = "escc"
		}

		if vscc != common.UndefinedParamValue {
			logger.Infof("Using vscc %s", vscc)
		} else {
			logger.Info("Using default vscc")
			vscc = "vscc"
		}

		if policy != common.UndefinedParamValue {
			p, err := policydsl.FromString(policy)
			if err != nil {
				return errors.Errorf("invalid policy %s", policy)
			}
			policyMarshalled = protoutil.MarshalOrPanic(p)
		}

		if collectionsConfigFile != common.UndefinedParamValue {
			var err error
			_, collectionConfigBytes, err = GetCollectionConfigFromFile(collectionsConfigFile)
			if err != nil {
				return errors.WithMessagef(err, "invalid collection configuration in file %s", collectionsConfigFile)
			}
		}
	}

	// Check that non-empty chaincode parameters contain only Args as a key.
	// Type checking is done later when the JSON is actually unmarshaled
	// into a pb.ChaincodeInput. To better understand what's going
	// on here with JSON parsing see http://blog.golang.org/json-and-go -
	// Generic JSON with interface{}
	if chaincodeCtorJSON != "{}" {
		var f interface{}
		err := json.Unmarshal([]byte(chaincodeCtorJSON), &f)
		if err != nil {
			return errors.Wrap(err, "chaincode argument error")
		}
		m := f.(map[string]interface{})
		sm := make(map[string]interface{})
		for k := range m {
			sm[strings.ToLower(k)] = m[k]
		}
		_, argsPresent := sm["args"]
		_, funcPresent := sm["function"]
		if !argsPresent || (len(m) == 2 && !funcPresent) || len(m) > 2 {
			return errors.New("non-empty JSON chaincode parameters must contain the following keys: 'Args' or 'Function' and 'Args'")
		}
	} else {
		if cmd == nil || (cmd != chaincodeInstallCmd && cmd != chaincodePackageCmd) {
			return errors.New("empty JSON chaincode parameters must contain the following keys: 'Args' or 'Function' and 'Args'")
		}
	}

	return nil
}

func validatePeerConnectionParameters(cmdName string) error {
	if connectionProfile != common.UndefinedParamValue {
		networkConfig, err := common.GetConfig(connectionProfile)
		if err != nil {
			return err
		}
		if len(networkConfig.Channels[channelID].Peers) != 0 {
			peerAddresses = []string{}
			tlsRootCertFiles = []string{}
			for peer, peerChannelConfig := range networkConfig.Channels[channelID].Peers {
				if peerChannelConfig.EndorsingPeer {
					peerConfig, ok := networkConfig.Peers[peer]
					if !ok {
						return errors.Errorf("peer '%s' is defined in the channel config but doesn't have associated peer config", peer)
					}
					peerAddresses = append(peerAddresses, peerConfig.URL)
					tlsRootCertFiles = append(tlsRootCertFiles, peerConfig.TLSCACerts.Path)
				}
			}
		}
	}

	// currently only support multiple peer addresses for invoke
	multiplePeersAllowed := map[string]bool{
		"invoke": true,
	}
	_, ok := multiplePeersAllowed[cmdName]
	if !ok && len(peerAddresses) > 1 {
		return errors.Errorf("'%s' command can only be executed against one peer. received %d", cmdName, len(peerAddresses))
	}

	if len(tlsRootCertFiles) > len(peerAddresses) {
		logger.Warningf("received more TLS root cert files (%d) than peer addresses (%d)", len(tlsRootCertFiles), len(peerAddresses))
	}

	if viper.GetBool("peer.tls.enabled") {
		if len(tlsRootCertFiles) != len(peerAddresses) {
			return errors.Errorf("number of peer addresses (%d) does not match the number of TLS root cert files (%d)", len(peerAddresses), len(tlsRootCertFiles))
		}
	} else {
		tlsRootCertFiles = nil
	}

	return nil
}

// ChaincodeCmdFactory holds the clients used by ChaincodeCmd
type ChaincodeCmdFactory struct {
	EndorserClients []pb.EndorserClient
	DeliverClients  []pb.DeliverClient
	Certificate     tls.Certificate
	Signer          identity.SignerSerializer
	BroadcastClient common.BroadcastClient
}

// InitCmdFactory init the ChaincodeCmdFactory with default clients
func InitCmdFactory(cmdName string, isEndorserRequired, isOrdererRequired bool, cryptoProvider bccsp.BCCSP) (*ChaincodeCmdFactory, error) {
	var err error
	var endorserClients []pb.EndorserClient
	var deliverClients []pb.DeliverClient
	if isEndorserRequired {
		if err = validatePeerConnectionParameters(cmdName); err != nil {
			return nil, errors.WithMessage(err, "error validating peer connection parameters")
		}
		for i, address := range peerAddresses {
			var tlsRootCertFile string
			if tlsRootCertFiles != nil {
				tlsRootCertFile = tlsRootCertFiles[i]
			}
			endorserClient, err := common.GetEndorserClientFnc(address, tlsRootCertFile)
			if err != nil {
				return nil, errors.WithMessagef(err, "error getting endorser client for %s", cmdName)
			}
			endorserClients = append(endorserClients, endorserClient)
			deliverClient, err := common.GetPeerDeliverClientFnc(address, tlsRootCertFile)
			if err != nil {
				return nil, errors.WithMessagef(err, "error getting deliver client for %s", cmdName)
			}
			deliverClients = append(deliverClients, deliverClient)
		}
		if len(endorserClients) == 0 {
			return nil, errors.New("no endorser clients retrieved - this might indicate a bug")
		}
	}
	certificate, err := common.GetClientCertificateFnc()
	if err != nil {
		return nil, errors.WithMessage(err, "error getting client certificate")
	}

	signer, err := common.GetDefaultSignerFnc()
	if err != nil {
		return nil, errors.WithMessage(err, "error getting default signer")
	}

	var broadcastClient common.BroadcastClient
	if isOrdererRequired {
		if len(common.OrderingEndpoint) == 0 {
			if len(endorserClients) == 0 {
				return nil, errors.New("orderer is required, but no ordering endpoint or endorser client supplied")
			}
			endorserClient := endorserClients[0]

			orderingEndpoints, err := common.GetOrdererEndpointOfChainFnc(channelID, signer, endorserClient, cryptoProvider)
			if err != nil {
				return nil, errors.WithMessagef(err, "error getting channel (%s) orderer endpoint", channelID)
			}
			if len(orderingEndpoints) == 0 {
				return nil, errors.Errorf("no orderer endpoints retrieved for channel %s, pass orderer endpoint with -o flag instead", channelID)
			}
			logger.Infof("Retrieved channel (%s) orderer endpoint: %s", channelID, orderingEndpoints[0])
			// override viper env
			viper.Set("orderer.address", orderingEndpoints[0])
		}

		broadcastClient, err = common.GetBroadcastClientFnc()
		if err != nil {
			return nil, errors.WithMessage(err, "error getting broadcast client")
		}
	}
	return &ChaincodeCmdFactory{
		EndorserClients: endorserClients,
		DeliverClients:  deliverClients,
		Signer:          signer,
		BroadcastClient: broadcastClient,
		Certificate:     certificate,
	}, nil
}

// processProposals sends a signed proposal to a set of peers, and gathers all the responses.
func processProposals(endorserClients []pb.EndorserClient, signedProposal *pb.SignedProposal) ([]*pb.ProposalResponse, error) {
	responsesCh := make(chan *pb.ProposalResponse, len(endorserClients))
	errorCh := make(chan error, len(endorserClients))
	wg := sync.WaitGroup{}
	for _, endorser := range endorserClients {
		wg.Add(1)
		go func(endorser pb.EndorserClient) {
			defer wg.Done()
			proposalResp, err := endorser.ProcessProposal(context.Background(), signedProposal)
			if err != nil {
				errorCh <- err
				return
			}
			responsesCh <- proposalResp
		}(endorser)
	}
	wg.Wait()
	close(responsesCh)
	close(errorCh)
	for err := range errorCh {
		return nil, err
	}
	var responses []*pb.ProposalResponse
	for response := range responsesCh {
		responses = append(responses, response)
	}
	return responses, nil
}

// ChaincodeInvokeOrQuery invokes or queries the chaincode. If successful, the
// INVOKE form prints the ProposalResponse to STDOUT, and the QUERY form prints
// the query result on STDOUT. A command-line flag (-r, --raw) determines
// whether the query result is output as raw bytes, or as a printable string.
// The printable form is optionally (-x, --hex) a hexadecimal representation
// of the query response. If the query response is NIL, nothing is output.
//
// NOTE - Query will likely go away as all interactions with the endorser are
// Proposal and ProposalResponses
func ChaincodeInvokeOrQuery(
	spec *pb.ChaincodeSpec,
	cID string,
	txID string,
	invoke bool,
	signer identity.SignerSerializer,
	certificate tls.Certificate,
	endorserClients []pb.EndorserClient,
	deliverClients []pb.DeliverClient,
	bc common.BroadcastClient,
) (*pb.ProposalResponse, error) {
	// Build the ChaincodeInvocationSpec message
	invocation := &pb.ChaincodeInvocationSpec{ChaincodeSpec: spec}

	creator, err := signer.Serialize()
	if err != nil {
		return nil, errors.WithMessage(err, "error serializing identity")
	}

	funcName := "invoke"
	if !invoke {
		funcName = "query"
	}

	// extract the transient field if it exists
	var tMap map[string][]byte
	if transient != "" {
		if err := json.Unmarshal([]byte(transient), &tMap); err != nil {
			return nil, errors.Wrap(err, "error parsing transient string")
		}
	}

	prop, txid, err := protoutil.CreateChaincodeProposalWithTxIDAndTransient(pcommon.HeaderType_ENDORSER_TRANSACTION, cID, invocation, creator, txID, tMap)
	if err != nil {
		return nil, errors.WithMessagef(err, "error creating proposal for %s", funcName)
	}

	signedProp, err := protoutil.GetSignedProposal(prop, signer)
	if err != nil {
		return nil, errors.WithMessagef(err, "error creating signed proposal for %s", funcName)
	}

	responses, err := processProposals(endorserClients, signedProp)
	if err != nil {
		return nil, errors.WithMessagef(err, "error endorsing %s", funcName)
	}

	if len(responses) == 0 {
		// this should only happen if some new code has introduced a bug
		return nil, errors.New("no proposal responses received - this might indicate a bug")
	}
	// all responses will be checked when the signed transaction is created.
	// for now, just set this so we check the first response's status
	proposalResp := responses[0]

	if invoke {
		if proposalResp != nil {
			if proposalResp.Response.Status >= shim.ERRORTHRESHOLD {
				return proposalResp, nil
			}
			// assemble a signed transaction (it's an Envelope message)
			env, err := protoutil.CreateSignedTx(prop, signer, responses...)
			if err != nil {
				return proposalResp, errors.WithMessage(err, "could not assemble transaction")
			}
			var dg *DeliverGroup
			var ctx context.Context
			if waitForEvent {
				var cancelFunc context.CancelFunc
				ctx, cancelFunc = context.WithTimeout(context.Background(), waitForEventTimeout)
				defer cancelFunc()

				dg = NewDeliverGroup(
					deliverClients,
					peerAddresses,
					signer,
					certificate,
					channelID,
					txid,
				)
				// connect to deliver service on all peers
				err := dg.Connect(ctx)
				if err != nil {
					return nil, err
				}
			}

			// send the envelope for ordering
			if err = bc.Send(env); err != nil {
				return proposalResp, errors.WithMessagef(err, "error sending transaction for %s", funcName)
			}

			if dg != nil && ctx != nil {
				// wait for event that contains the txid from all peers
				err = dg.Wait(ctx)
				if err != nil {
					return nil, err
				}
			}
		}
	}

	return proposalResp, nil
}

// DeliverGroup holds all of the information needed to connect
// to a set of peers to wait for the interested txid to be
// committed to the ledgers of all peers. This functionality
// is currently implemented via the peer's DeliverFiltered service.
// An error from any of the peers/deliver clients will result in
// the invoke command returning an error. Only the first error that
// occurs will be set
type DeliverGroup struct {
	Clients     []*DeliverClient
	Certificate tls.Certificate
	ChannelID   string
	TxID        string
	Signer      identity.SignerSerializer
	mutex       sync.Mutex
	Error       error
	wg          sync.WaitGroup
}

// DeliverClient holds the client/connection related to a specific
// peer. The address is included for logging purposes
type DeliverClient struct {
	Client     pb.DeliverClient
	Connection pb.Deliver_DeliverClient
	Address    string
}

func NewDeliverGroup(
	deliverClients []pb.DeliverClient,
	peerAddresses []string,
	signer identity.SignerSerializer,
	certificate tls.Certificate,
	channelID string,
	txid string,
) *DeliverGroup {
	clients := make([]*DeliverClient, len(deliverClients))
	for i, client := range deliverClients {
		address := peerAddresses[i]
		if address == "" {
			address = viper.GetString("peer.address")
		}
		dc := &DeliverClient{
			Client:  client,
			Address: address,
		}
		clients[i] = dc
	}

	dg := &DeliverGroup{
		Clients:     clients,
		Certificate: certificate,
		ChannelID:   channelID,
		TxID:        txid,
		Signer:      signer,
	}

	return dg
}

// Connect waits for all deliver clients in the group to connect to
// the peer's deliver service, receive an error, or for the context
// to timeout. An error will be returned whenever even a single
// deliver client fails to connect to its peer
func (dg *DeliverGroup) Connect(ctx context.Context) error {
	dg.wg.Add(len(dg.Clients))
	for _, client := range dg.Clients {
		go dg.ClientConnect(ctx, client)
	}
	readyCh := make(chan struct{})
	go dg.WaitForWG(readyCh)

	select {
	case <-readyCh:
		if dg.Error != nil {
			err := errors.WithMessage(dg.Error, "failed to connect to deliver on all peers")
			return err
		}
	case <-ctx.Done():
		err := errors.New("timed out waiting for connection to deliver on all peers")
		return err
	}

	return nil
}

// ClientConnect sends a deliver seek info envelope using the
// provided deliver client, setting the deliverGroup's Error
// field upon any error
func (dg *DeliverGroup) ClientConnect(ctx context.Context, dc *DeliverClient) {
	defer dg.wg.Done()
	df, err := dc.Client.DeliverFiltered(ctx)
	if err != nil {
		err = errors.WithMessagef(err, "error connecting to deliver filtered at %s", dc.Address)
		dg.setError(err)
		return
	}
	defer df.CloseSend()
	dc.Connection = df

	envelope := createDeliverEnvelope(dg.ChannelID, dg.Certificate, dg.Signer)
	err = df.Send(envelope)
	if err != nil {
		err = errors.WithMessagef(err, "error sending deliver seek info envelope to %s", dc.Address)
		dg.setError(err)
		return
	}
}

// Wait waits for all deliver client connections in the group to
// either receive a block with the txid, an error, or for the
// context to timeout
func (dg *DeliverGroup) Wait(ctx context.Context) error {
	if len(dg.Clients) == 0 {
		return nil
	}

	dg.wg.Add(len(dg.Clients))
	for _, client := range dg.Clients {
		go dg.ClientWait(client)
	}
	readyCh := make(chan struct{})
	go dg.WaitForWG(readyCh)

	select {
	case <-readyCh:
		if dg.Error != nil {
			return dg.Error
		}
	case <-ctx.Done():
		err := errors.New("timed out waiting for txid on all peers")
		return err
	}

	return nil
}

// ClientWait waits for the specified deliver client to receive
// a block event with the requested txid
func (dg *DeliverGroup) ClientWait(dc *DeliverClient) {
	defer dg.wg.Done()
	for {
		resp, err := dc.Connection.Recv()
		if err != nil {
			err = errors.WithMessagef(err, "error receiving from deliver filtered at %s", dc.Address)
			dg.setError(err)
			return
		}
		switch r := resp.Type.(type) {
		case *pb.DeliverResponse_FilteredBlock:
			filteredTransactions := r.FilteredBlock.FilteredTransactions
			for _, tx := range filteredTransactions {
				if tx.Txid == dg.TxID {
					logger.Infof("txid [%s] committed with status (%s) at %s", dg.TxID, tx.TxValidationCode, dc.Address)
					if tx.TxValidationCode != pb.TxValidationCode_VALID {
						err = errors.Errorf("transaction invalidated with status (%s)", tx.TxValidationCode)
						dg.setError(err)
					}
					return
				}
			}
		case *pb.DeliverResponse_Status:
			err = errors.Errorf("deliver completed with status (%s) before txid received", r.Status)
			dg.setError(err)
			return
		default:
			err = errors.Errorf("received unexpected response type (%T) from %s", r, dc.Address)
			dg.setError(err)
			return
		}
	}
}

// WaitForWG waits for the deliverGroup's wait group and closes
// the channel when ready
func (dg *DeliverGroup) WaitForWG(readyCh chan struct{}) {
	dg.wg.Wait()
	close(readyCh)
}

// setError serializes an error for the deliverGroup
func (dg *DeliverGroup) setError(err error) {
	dg.mutex.Lock()
	dg.Error = err
	dg.mutex.Unlock()
}

func createDeliverEnvelope(
	channelID string,
	certificate tls.Certificate,
	signer identity.SignerSerializer,
) *pcommon.Envelope {
	var tlsCertHash []byte
	// check for client certificate and create hash if present
	if len(certificate.Certificate) > 0 {
		tlsCertHash = util.ComputeSHA256(certificate.Certificate[0])
	}

	start := &ab.SeekPosition{
		Type: &ab.SeekPosition_Newest{
			Newest: &ab.SeekNewest{},
		},
	}

	stop := &ab.SeekPosition{
		Type: &ab.SeekPosition_Specified{
			Specified: &ab.SeekSpecified{
				Number: math.MaxUint64,
			},
		},
	}

	seekInfo := &ab.SeekInfo{
		Start:    start,
		Stop:     stop,
		Behavior: ab.SeekInfo_BLOCK_UNTIL_READY,
	}

	env, err := protoutil.CreateSignedEnvelopeWithTLSBinding(
		pcommon.HeaderType_DELIVER_SEEK_INFO,
		channelID,
		signer,
		seekInfo,
		int32(0),
		uint64(0),
		tlsCertHash,
	)
	if err != nil {
		logger.Errorf("Error signing envelope: %s", err)
		return nil
	}

	return env
}
