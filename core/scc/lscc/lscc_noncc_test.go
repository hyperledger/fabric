/*
	"github.com/golang/protobuf/proto"
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package lscc_test

import (
	"errors"

	"github.com/hyperledger/fabric/core/common/ccprovider"
	"github.com/hyperledger/fabric/core/scc/lscc"
	"github.com/hyperledger/fabric/core/scc/lscc/mock"
	pb "github.com/hyperledger/fabric/protos/peer"

	"github.com/golang/protobuf/proto"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("LSCC", func() {

	var (
		l               *lscc.LifeCycleSysCC
		fakeSupport     *mock.FileSystemSupport
		fakeSCCProvider *mock.SystemChaincodeProvider
	)

	BeforeEach(func() {
		fakeSupport = &mock.FileSystemSupport{}
		fakeSCCProvider = &mock.SystemChaincodeProvider{}

		l = &lscc.LifeCycleSysCC{
			Support:     fakeSupport,
			SCCProvider: fakeSCCProvider,
		}
	})

	Describe("GetChaincodeDeploymentSpec", func() {
		var (
			fakeQueryExecutor *mock.QueryExecutor
			ccData            *ccprovider.ChaincodeData
			ccDataBytes       []byte
			fakeCCPackage     *mock.CCPackage
			err               error
			deploymentSpec    *pb.ChaincodeDeploymentSpec
		)

		BeforeEach(func() {
			ccData = &ccprovider.ChaincodeData{
				Name: "chaincode-data-name",
			}
			ccDataBytes, err = proto.Marshal(ccData)
			Expect(err).NotTo(HaveOccurred())

			fakeQueryExecutor = &mock.QueryExecutor{}
			fakeQueryExecutor.GetStateReturns(ccDataBytes, nil)

			fakeSCCProvider.GetQueryExecutorForLedgerReturns(fakeQueryExecutor, nil)

			deploymentSpec = &pb.ChaincodeDeploymentSpec{
				ChaincodeSpec: &pb.ChaincodeSpec{
					ChaincodeId: &pb.ChaincodeID{
						Name: "chaincode-name",
					},
				},
			}

			fakeCCPackage = &mock.CCPackage{}
			fakeCCPackage.GetDepSpecReturns(deploymentSpec)

			fakeSupport.GetChaincodeFromLocalStorageReturns(fakeCCPackage, nil)
		})

		It("returns the chaincode deployment spec for a valid chaincode", func() {
			cds, err := l.ChaincodeDeploymentSpec("channel-foo", "chaincode-data-name")
			Expect(err).NotTo(HaveOccurred())
			Expect(cds).To(Equal(deploymentSpec))

			Expect(fakeSCCProvider.GetQueryExecutorForLedgerCallCount()).To(Equal(1))
			Expect(fakeSCCProvider.GetQueryExecutorForLedgerArgsForCall(0)).To(Equal("channel-foo"))

			Expect(fakeQueryExecutor.GetStateCallCount()).To(Equal(1))
			getStateNamespace, getStateCCName := fakeQueryExecutor.GetStateArgsForCall(0)
			Expect(getStateNamespace).To(Equal("lscc"))
			Expect(getStateCCName).To(Equal("chaincode-data-name"))
			Expect(fakeQueryExecutor.DoneCallCount()).To(Equal(1))
		})

		Context("when the query executor cannot be retrieved", func() {
			BeforeEach(func() {
				fakeSCCProvider.GetQueryExecutorForLedgerReturns(nil, errors.New("fake-error"))
			})

			It("wraps and returns the error", func() {
				_, err := l.ChaincodeDeploymentSpec("channel-foo", "chaincode-data-name")
				Expect(err).To(MatchError("could not retrieve QueryExecutor for channel channel-foo: fake-error"))
			})
		})

		Context("when the get state query fails", func() {
			BeforeEach(func() {
				fakeQueryExecutor.GetStateReturns(nil, errors.New("fake-error"))
			})

			It("wraps and returns the error", func() {
				_, err := l.ChaincodeDeploymentSpec("channel-foo", "chaincode-data-name")
				Expect(err).To(MatchError("could not retrieve state for chaincode chaincode-data-name on channel channel-foo: fake-error"))
			})
		})

		Context("when the chaincode is not found in the table", func() {
			BeforeEach(func() {
				fakeQueryExecutor.GetStateReturns(nil, nil)
			})

			It("returns an error", func() {
				_, err := l.ChaincodeDeploymentSpec("channel-foo", "chaincode-data-name")
				Expect(err).To(MatchError("chaincode chaincode-data-name not found on channel channel-foo"))
			})
		})
	})
})
