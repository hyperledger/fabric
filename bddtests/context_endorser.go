/*
Copyright IBM Corp. 2016 All Rights Reserved.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

		 http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package bddtests

import (
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/DATA-DOG/godog"
	"github.com/DATA-DOG/godog/gherkin"
	"github.com/hyperledger/fabric/common/util"
	"github.com/hyperledger/fabric/core/chaincode/platforms"
	pb "github.com/hyperledger/fabric/protos/peer"
	"github.com/hyperledger/fabric/protos/utils"
	"golang.org/x/net/context"
	"google.golang.org/grpc"
)

// checkSpec to see if chaincode resides within current package capture for language.
func (*BDDContext) checkSpec(spec *pb.ChaincodeSpec) error {
	// Don't allow nil value
	if spec == nil {
		return errors.New("Expected chaincode specification, nil received")
	}

	platform, err := platforms.Find(spec.Type)
	if err != nil {
		return fmt.Errorf("Failed to determine platform type: %s", err)
	}

	return platform.ValidateSpec(spec)
}

// Build builds the supplied chaincode image
func (b *BDDContext) build(spec *pb.ChaincodeSpec) (*pb.ChaincodeDeploymentSpec, error) {
	var codePackageBytes []byte
	if err := b.checkSpec(spec); err != nil {
		return nil, err
	}

	// FIXME: This only returns a deployment spec...the chaincode is not compiled.
	// Is compilation needed?
	codePackageBytes, err := platforms.GetDeploymentPayload(spec)
	if err != nil {
		return nil, err
	}
	chaincodeDeploymentSpec := &pb.ChaincodeDeploymentSpec{ChaincodeSpec: spec, CodePackage: codePackageBytes}
	return chaincodeDeploymentSpec, nil
}

func (b *BDDContext) requestingFrom(arg1, arg2 string) error {
	return godog.ErrPending
}

func (b *BDDContext) iShouldGetAJSONResponseWithArrayContainsElements(arg1, arg2 string) error {
	return godog.ErrPending
}

func (b *BDDContext) iWaitSeconds(seconds string) error {
	waitInSecs, err := strconv.Atoi(seconds)
	if err != nil {
		return err
	}
	time.Sleep(time.Duration(waitInSecs) * time.Second)
	return nil
}

func (b *BDDContext) iRegisterWithCASupplyingUsernameAndSecretOnPeers(enrollID, secret string, tableOfPeers *gherkin.DataTable) error {
	return b.registerUser(enrollID, secret)
}

func (b *BDDContext) userCreatesAChaincodeSpecOfTypeForChaincodeWithArgs(enrollID, ccSpecAlias, ccType, chaincodePath string, argsTable *gherkin.DataTable) error {
	userRegistration, err := b.GetUserRegistration(enrollID)
	if err != nil {
		return err
	}

	args, err := b.GetArgsForUser(argsTable.Rows[1].Cells, userRegistration)
	//fmt.Printf("Args for user: %v, with err = %s", args, err)
	ccSpec := createChaincodeSpec(ccType, chaincodePath, util.ToChaincodeArgs(args...))
	userRegistration.lastResult = ccSpec
	userRegistration.tags[ccSpecAlias] = userRegistration.lastResult

	return nil
}

func (b *BDDContext) userCreatesADeploymentProposalUsingChaincodeDeploymentSpec(enrollID, proposalAlias, ccDeploymentSpecAlias string) (err error) {
	var userRegistration *UserRegistration
	var ccDeploymentSpec *pb.ChaincodeDeploymentSpec
	errRetFunc := func() error {
		return fmt.Errorf("Error creating chaincode proposal for user '%s' from chaincodeDeploymentSpec '%s':  %s", enrollID, ccDeploymentSpecAlias, err)
	}
	if userRegistration, err = b.GetUserRegistration(enrollID); err != nil {
		return errRetFunc()
	}
	if ccDeploymentSpec, err = userRegistration.GetChaincodeDeploymentSpec(ccDeploymentSpecAlias); err != nil {
		return errRetFunc()
	}
	var proposal *pb.Proposal
	// TODO: how should we get a cert from the command line?
	if proposal, err = createProposalForChaincode(ccDeploymentSpec, []byte("cert")); err != nil {

	}
	if _, err = userRegistration.SetTagValue(proposalAlias, proposal); err != nil {
		return errRetFunc()
	}
	return nil
}

func (b *BDDContext) userCreatesADeploymentSpecUsingChaincodeSpecAndDevopsOnPeer(enrollID, ccDeploymentSpecAlias, ccSpecAlias, devopsPeerComposeService string) (err error) {
	var ccSpec *pb.ChaincodeSpec
	var userRegistration *UserRegistration
	errRetFunc := func() error {
		return fmt.Errorf("Error creating deployment spec '%s' for user '%s' from chaincode spec '%s':  %s", ccDeploymentSpecAlias, enrollID, ccSpecAlias, err)
	}
	if userRegistration, err = b.GetUserRegistration(enrollID); err != nil {
		return errRetFunc()
	}
	if ccSpec, err = userRegistration.GetChaincodeSpec(ccSpecAlias); err != nil {
		return errRetFunc()
	}

	/****** Let us get the deployment spec directly from this machine like SDK would
	// Now use the devops client to create the deployment spec
	var grpcClient *grpc.ClientConn
	if grpcClient, err = b.getGrpcClientForComposeService(devopsPeerComposeService); err != nil {
		return errRetFunc()
	}
	defer grpcClient.Close()
	devopsClient := pb.NewDevopsClient(grpcClient)
	var ccDeploymentSpec *pb.ChaincodeDeploymentSpec
	if ccDeploymentSpec, err = devopsClient.Build(context.Background(), ccSpec); err != nil {
		return errRetFunc()
	}
	********/
	var ccDeploymentSpec *pb.ChaincodeDeploymentSpec
	if ccDeploymentSpec, err = b.build(ccSpec); err != nil {
		return errRetFunc()
	}
	// Now store the chaincode deployment spec
	if _, err = userRegistration.SetTagValue(ccDeploymentSpecAlias, ccDeploymentSpec); err != nil {
		return errRetFunc()
	}
	return err
}

func getContextAndCancelForTimeoutInSecs(parentCtx context.Context, timeoutInSecs string) (context.Context, context.CancelFunc, error) {
	var err error
	errRetFunc := func() error {
		return fmt.Errorf("Error building context and cancel func with timeout '%s':  %s", timeoutInSecs, err)
	}
	var (
		durationToWait time.Duration
		ctx            context.Context
		cancel         context.CancelFunc
	)
	if durationToWait, err = time.ParseDuration(fmt.Sprintf("%ss", timeoutInSecs)); err != nil {
		return nil, nil, errRetFunc()
	}
	ctx, cancel = context.WithTimeout(parentCtx, durationToWait)
	return ctx, cancel, nil
}

func (b *BDDContext) invokeOnWithTimeout(composeServices []string, timeoutInSecs string, callBack func(context.Context, pb.EndorserClient) (proposalResponse *pb.ProposalResponse, err error)) (map[string]*pb.ProposalResponse, error) {
	var err error
	resultsMap := make(map[string]*pb.ProposalResponse)
	errRetFunc := func() error {
		return fmt.Errorf("Error when invoking endorser(s) on '%s':  %s", composeServices, err)
	}
	var (
		durationToWait time.Duration
		ctx            context.Context
		cancel         context.CancelFunc
	)
	if durationToWait, err = time.ParseDuration(fmt.Sprintf("%ss", timeoutInSecs)); err != nil {
		return nil, errRetFunc()
	}
	ctx, cancel = context.WithTimeout(context.Background(), durationToWait)
	defer cancel()
	cancel()
	for _, cs := range composeServices {
		go func(composeService string) {
			var proposalResponse *pb.ProposalResponse
			// Now use the endorser client to create the send the proposal
			println("Calling endorser for compose service:", composeService)
			var grpcClient *grpc.ClientConn
			if grpcClient, err = NewGrpcClient("172.17.0.4:7051"); err != nil {
				return
			}
			defer grpcClient.Close()
			endorserClient := pb.NewEndorserClient(grpcClient)
			if proposalResponse, err = callBack(ctx, endorserClient); err != nil {
				return
			}
			resultsMap[composeService] = proposalResponse
		}(cs)
	}
	return resultsMap, err
}

func (b *BDDContext) getGrpcClientForComposeService(composeService string) (grpcClient *grpc.ClientConn, err error) {
	var ipAddress string
	errRetFunc := func() error {
		return fmt.Errorf("Error getting grpc client conn for compose service '%s':  %s", composeService, err)
	}
	if ipAddress, err = b.composition.GetIPAddressForComposeService(composeService); err != nil {
		return nil, errRetFunc()
	}
	if grpcClient, err = NewGrpcClient(fmt.Sprintf("%s:%d", ipAddress, b.grpcClientPort)); err != nil {
		return nil, errRetFunc()
	}
	return grpcClient, err
}

func (b *BDDContext) userSendsProposalToEndorsersWithTimeoutOfSeconds(enrollID, proposalAlias, timeoutInSecs string, endorsersTable *gherkin.DataTable) (err error) {
	var proposal *pb.Proposal
	var keyedProposalResponsesMap KeyedProposalResponseMap
	keyedProposalResponsesMap = make(KeyedProposalResponseMap)
	errRetFunc := func() error {
		return fmt.Errorf("Error sending proposal '%s' for user '%s':  %s", proposalAlias, enrollID, err)
	}
	var userRegistration *UserRegistration
	if userRegistration, err = b.GetUserRegistration(enrollID); err != nil {
		return errRetFunc()
	}
	// Get the proposal from the user
	if proposal, err = userRegistration.GetProposal(proposalAlias); err != nil {
		return errRetFunc()
	}

	var ctx context.Context
	var cancel context.CancelFunc
	if ctx, cancel, err = getContextAndCancelForTimeoutInSecs(context.Background(), timeoutInSecs); err != nil {
		return errRetFunc()
	}
	defer cancel()
	// Loop through endorsers and send proposals
	var endorsers []string
	if endorsers, err = b.GetArgsForUser(endorsersTable.Rows[0].Cells, userRegistration); err != nil {
		return errRetFunc()
	}
	respQueue := make(chan *KeyedProposalResponse)
	for _, e := range endorsers {
		go func(endorser string) {
			var localErr error
			var proposalResponse *pb.ProposalResponse
			// Now use the endorser client to send the proposal
			var grpcClient *grpc.ClientConn
			if grpcClient, localErr = b.getGrpcClientForComposeService(endorser); localErr != nil {
				respQueue <- &KeyedProposalResponse{endorser, nil, fmt.Errorf("Error calling endorser '%s': %s", endorser, localErr)}
				return
			}
			defer grpcClient.Close()

			proposalBytes, err := utils.GetBytesProposal(proposal)
			if err != nil {
				respQueue <- &KeyedProposalResponse{endorser, nil, fmt.Errorf("Error serializing proposal bytes")}
				return
			}
			// FIXME: the endorser needs to be given a signed proposal - who should sign?
			signedProposal := &pb.SignedProposal{ProposalBytes: proposalBytes, Signature: []byte("signature")}

			endorserClient := pb.NewEndorserClient(grpcClient)
			if proposalResponse, localErr = endorserClient.ProcessProposal(ctx, signedProposal); localErr != nil {
				respQueue <- &KeyedProposalResponse{endorser, nil, fmt.Errorf("Error calling endorser '%s':  %s", endorser, localErr)}
				return
			}
			respQueue <- &KeyedProposalResponse{endorser, proposalResponse, nil}
		}(e)
	}
	go func() {
		for i := 0; i < len(endorsers); i++ {
			result := <-respQueue
			keyedProposalResponsesMap[result.endorser] = result
			if result.err != nil {
				// TODO: think about whether to break on first failure, or allow to collect
			}
		}
		cancel()
	}()
	<-ctx.Done()
	if ctx.Err() != context.Canceled {
		err = ctx.Err()
		return errRetFunc()
	}
	userRegistration.lastResult = keyedProposalResponsesMap
	return nil
}

func (b *BDDContext) userStoresTheirLastResultAs(enrollID, tagName string) error {
	userRegistration, err := b.GetUserRegistration(enrollID)
	if err != nil {
		return err
	}
	userRegistration.tags[tagName] = userRegistration.lastResult
	return nil
}

func (b *BDDContext) userExpectsProposalResponsesWithStatusFromEndorsers(enrollID, proposalResponseAlias, respStatusCode string, endorsersTable *gherkin.DataTable) (err error) {
	var userRegistration *UserRegistration
	var keyedProposalResponseMap KeyedProposalResponseMap
	errRetFunc := func() error {
		return fmt.Errorf("Error verifying proposal response '%s' for user '%s' with expected response code of '%s':  %s", proposalResponseAlias, enrollID, respStatusCode, err)
	}
	if userRegistration, err = b.GetUserRegistration(enrollID); err != nil {
		return errRetFunc()
	}
	if keyedProposalResponseMap, err = userRegistration.GetKeyedProposalResponseDict(proposalResponseAlias); err != nil {
		return errRetFunc()
	}
	for endorserComposeService, keyedProposalResponse := range keyedProposalResponseMap {
		// If their is an err in getting the proposal response, fail
		if keyedProposalResponse.err != nil {
			err = fmt.Errorf("Received error in keyedProposalResponse for endorser '%s':  %s", endorserComposeService, keyedProposalResponse.err)
			return errRetFunc()
		}
		if keyedProposalResponse.proposal == nil {
			err = fmt.Errorf("keyedProposalResponse.proposal value was nil for endorser '%s':  %s", endorserComposeService, keyedProposalResponse.err)
			return errRetFunc()
		}
		if fmt.Sprintf("%d", keyedProposalResponse.proposal.Response.Status) != respStatusCode {
			err = fmt.Errorf("Expected ProposalResponse.Response.Status to be '%s', received '%d'", respStatusCode, keyedProposalResponse.proposal.Response.Status)
			return errRetFunc()
		}
	}
	return nil
}

func (b *BDDContext) userSetsESCCToForChaincodeSpec(arg1, arg2, arg3 string) error {
	return godog.ErrPending
}

func (b *BDDContext) userSetsVSCCToForChaincodeSpec(arg1, arg2, arg3 string) error {
	return godog.ErrPending
}
