/*
Copyright IBM Corp All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package token

import (
	"context"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"
	"unicode/utf8"

	docker "github.com/fsouza/go-dockerclient"
	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric/bccsp/sw"
	"github.com/hyperledger/fabric/integration/nwo"
	"github.com/hyperledger/fabric/integration/nwo/commands"
	"github.com/hyperledger/fabric/msp"
	"github.com/hyperledger/fabric/protos/common"
	"github.com/hyperledger/fabric/protos/token"
	tk "github.com/hyperledger/fabric/token"
	"github.com/hyperledger/fabric/token/client"
	tokenclient "github.com/hyperledger/fabric/token/client"
	token2 "github.com/hyperledger/fabric/token/cmd"
	"github.com/hyperledger/fabric/token/tms/plain"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gexec"
	"github.com/pkg/errors"
	"github.com/tedsuo/ifrit"
)

var _ bool = Describe("Token EndToEnd", func() {
	var (
		testDir string
		client  *docker.Client
		network *nwo.Network
		process ifrit.Process

		tokensToIssue               []*token.Token
		expectedTokenTransaction    *token.TokenTransaction
		expectedUnspentTokens       *token.UnspentTokens
		issuedTokens                []*token.UnspentToken
		expectedTransferTransaction *token.TokenTransaction
		expectedRedeemTransaction   *token.TokenTransaction
		recipientUser1              *token.TokenOwner
		recipientUser2              *token.TokenOwner
	)

	BeforeEach(func() {
		// prepare data for issue tokens
		tokensToIssue = []*token.Token{
			{Owner: &token.TokenOwner{Raw: []byte("test-owner")}, Type: "ABC123", Quantity: ToHex(119)},
		}

		// expected token transaction returned by prover peer
		expectedTokenTransaction = &token.TokenTransaction{
			Action: &token.TokenTransaction_TokenAction{
				TokenAction: &token.TokenAction{
					Data: &token.TokenAction_Issue{
						Issue: &token.Issue{
							Outputs: []*token.Token{{
								Owner:    &token.TokenOwner{Raw: []byte("test-owner")},
								Type:     "ABC123",
								Quantity: ToHex(119),
							}}}}}}}
		expectedTransferTransaction = &token.TokenTransaction{
			Action: &token.TokenTransaction_TokenAction{
				TokenAction: &token.TokenAction{
					Data: &token.TokenAction_Transfer{
						Transfer: &token.Transfer{
							Inputs: nil,
							Outputs: []*token.Token{{
								Owner:    nil,
								Type:     "ABC123",
								Quantity: ToHex(119),
							}},
						},
					},
				},
			},
		}
		expectedRedeemTransaction = &token.TokenTransaction{
			Action: &token.TokenTransaction_TokenAction{
				TokenAction: &token.TokenAction{
					Data: &token.TokenAction_Redeem{
						Redeem: &token.Transfer{
							Inputs: nil,
							Outputs: []*token.Token{
								{
									Owner:    nil,
									Type:     "ABC123",
									Quantity: ToHex(50),
								},
								{
									Owner:    nil,
									Type:     "ABC123",
									Quantity: ToHex(119 - 50),
								},
							},
						},
					},
				},
			},
		}

		expectedUnspentTokens = &token.UnspentTokens{
			Tokens: []*token.UnspentToken{
				{
					Quantity: ToDecimal(119),
					Type:     "ABC123",
					Id:       &token.TokenId{TxId: "ledger-id", Index: 1},
				},
			},
		}

		var err error
		testDir, err = ioutil.TempDir("", "token-e2e")
		Expect(err).NotTo(HaveOccurred())

		client, err = docker.NewClientFromEnv()
		Expect(err).NotTo(HaveOccurred())
	})

	AfterEach(func() {
		if process != nil {
			process.Signal(syscall.SIGTERM)
			Eventually(process.Wait(), time.Minute).Should(Receive())
		}
		if network != nil {
			network.Cleanup()
		}
		os.RemoveAll(testDir)
	})

	Describe("basic solo network for token transaction e2e using Token CLI", func() {
		BeforeEach(func() {
			var err error

			network = nwo.New(BasicSoloV20(), testDir, client, StartPort(), components)
			network.GenerateConfigTree()

			network.Bootstrap()

			client, err = docker.NewClientFromEnv()
			Expect(err).NotTo(HaveOccurred())

			networkRunner := network.NetworkGroupRunner()
			process = ifrit.Invoke(networkRunner)
			Eventually(process.Ready()).Should(BeClosed())

			orderer := network.Orderer("orderer")
			network.CreateAndJoinChannel(orderer, "testchannel")
		})

		It("executes a basic solo network and submits token transaction", func() {
			By("issuing tokens to user2")

			user1 := getShellIdentity(network, network.Peer("Org1", "peer1"), "User1", "Org1MSP")
			user2 := getShellIdentity(network, network.Peer("Org1", "peer1"), "User2", "Org1MSP")

			// User1 issues 100 ABC123 tokens to User2
			IssueToken(network, network.Peer("Org1", "peer1"), network.Orderer("orderer"),
				"testchannel", "User1", "Org1MSP", "ABC123", "100", string(user2))

			// User2 lists her tokens and verify
			outputs := ListTokens(network, network.Peer("Org1", "peer1"), network.Orderer("orderer"),
				"testchannel", "User2", "Org1MSP")
			Expect(len(outputs)).To(BeEquivalentTo(1))
			fmt.Println(outputs[0].Quantity)
			fmt.Println(ToHex(100))
			Expect(outputs[0].Quantity).To(BeEquivalentTo(ToDecimal(100)))
			Expect(outputs[0].Type).To(BeEquivalentTo("ABC123"))

			// User2 transfers back the tokens to User1
			TransferTokens(network, network.Peer("Org1", "peer1"), network.Orderer("orderer"),
				"testchannel", "User2", "Org1MSP",
				[]*token.TokenId{outputs[0].Id},
				[]*token2.ShellRecipientShare{
					{
						Quantity:  ToHex(50),
						Recipient: user2,
					},
					{
						Quantity:  ToHex(50),
						Recipient: user1,
					},
				},
			)

			// User2 lists her tokens and verify
			outputs = ListTokens(network, network.Peer("Org1", "peer1"), network.Orderer("orderer"),
				"testchannel", "User2", "Org1MSP")
			Expect(len(outputs)).To(BeEquivalentTo(1))
			Expect(outputs[0].Quantity).To(BeEquivalentTo(ToDecimal(50)))
			Expect(outputs[0].Type).To(BeEquivalentTo("ABC123"))

			// User1 lists her tokens and verify
			outputs = ListTokens(network, network.Peer("Org1", "peer1"), network.Orderer("orderer"),
				"testchannel", "User2", "Org1MSP")
			Expect(len(outputs)).To(BeEquivalentTo(1))
			Expect(outputs[0].Quantity).To(BeEquivalentTo(ToDecimal(50)))
			Expect(outputs[0].Type).To(BeEquivalentTo("ABC123"))

			// User1 redeems 25 of her tokens
			RedeemTokens(network, network.Peer("Org1", "peer1"), network.Orderer("orderer"),
				"testchannel", "User2", "Org1MSP",
				[]*token.TokenId{outputs[0].Id}, 25)

			// User1 lists her tokens and verify again
			outputs = ListTokens(network, network.Peer("Org1", "peer1"), network.Orderer("orderer"),
				"testchannel", "User2", "Org1MSP")
			Expect(len(outputs)).To(BeEquivalentTo(1))
			Expect(outputs[0].Quantity).To(BeEquivalentTo(ToDecimal(25)))
			Expect(outputs[0].Type).To(BeEquivalentTo("ABC123"))
		})

	})

	Describe("basic solo network for token transaction e2e", func() {
		BeforeEach(func() {
			var err error

			network = nwo.New(BasicSoloV20(), testDir, client, StartPort(), components)
			network.GenerateConfigTree()

			network.Bootstrap()

			client, err = docker.NewClientFromEnv()
			Expect(err).NotTo(HaveOccurred())

			peer := network.Peer("Org1", "peer1")
			// Get recipients' identity
			recipientUser2Bytes, err := getIdentity(network, peer, "User2", "Org1MSP")
			Expect(err).NotTo(HaveOccurred())
			recipientUser2 = &token.TokenOwner{Raw: recipientUser2Bytes}
			tokensToIssue[0].Owner = recipientUser2
			expectedTokenTransaction.GetTokenAction().GetIssue().Outputs[0].Owner = recipientUser2
			recipientUser1Bytes, err := getIdentity(network, peer, "User1", "Org1MSP")
			Expect(err).NotTo(HaveOccurred())
			recipientUser1 = &token.TokenOwner{Raw: recipientUser1Bytes}
			expectedTransferTransaction.GetTokenAction().GetTransfer().Outputs[0].Owner = recipientUser1
			expectedRedeemTransaction.GetTokenAction().GetRedeem().Outputs[1].Owner = recipientUser1

			networkRunner := network.NetworkGroupRunner()
			process = ifrit.Invoke(networkRunner)
			Eventually(process.Ready()).Should(BeClosed())
		})

		It("executes a basic solo network and submits token transaction", func() {
			By("getting the orderer by name")
			orderer := network.Orderer("orderer")

			By("setting up the channel")
			network.CreateAndJoinChannel(orderer, "testchannel")

			By("getting the client peer by name")
			peer := network.Peer("Org1", "peer1")

			By("creating a new client")
			config := getClientConfig(network, peer, orderer, "testchannel", "User1", "Org1MSP")
			signingIdentity, err := getSigningIdentity(config.MSPInfo.MSPConfigPath, config.MSPInfo.MSPID, config.MSPInfo.MSPType)
			Expect(err).NotTo(HaveOccurred())
			tClient, err := tokenclient.NewClient(*config, signingIdentity)
			Expect(err).NotTo(HaveOccurred())

			By("issuing tokens to user2")
			txID := RunIssueRequest(tClient, tokensToIssue, expectedTokenTransaction)

			By("list tokens")
			config = getClientConfig(network, peer, orderer, "testchannel", "User2", "Org1MSP")
			signingIdentity, err = getSigningIdentity(config.MSPInfo.MSPConfigPath, config.MSPInfo.MSPID, config.MSPInfo.MSPType)
			Expect(err).NotTo(HaveOccurred())
			tClient, err = tokenclient.NewClient(*config, signingIdentity)
			Expect(err).NotTo(HaveOccurred())
			issuedTokens = RunListTokens(tClient, expectedUnspentTokens)
			Expect(issuedTokens).ToNot(BeNil())
			Expect(len(issuedTokens)).To(Equal(1))

			By("transferring tokens to user1")
			tokenIDs := []*token.TokenId{{TxId: txID, Index: 0}}
			expectedTransferTransaction.GetTokenAction().GetTransfer().Inputs = tokenIDs
			txID = RunTransferRequest(tClient, issuedTokens, recipientUser1, expectedTransferTransaction)

			By("list tokens user 2")
			config = getClientConfig(network, peer, orderer, "testchannel", "User2", "Org1MSP")
			signingIdentity, err = getSigningIdentity(config.MSPInfo.MSPConfigPath, config.MSPInfo.MSPID, config.MSPInfo.MSPType)
			tClient, err = tokenclient.NewClient(*config, signingIdentity)
			Expect(err).NotTo(HaveOccurred())
			issuedTokens = RunListTokens(tClient, nil)

			By("list tokens user 1")
			config = getClientConfig(network, peer, orderer, "testchannel", "User1", "Org1MSP")
			signingIdentity, err = getSigningIdentity(config.MSPInfo.MSPConfigPath, config.MSPInfo.MSPID, config.MSPInfo.MSPType)
			tClient, err = tokenclient.NewClient(*config, signingIdentity)
			Expect(err).NotTo(HaveOccurred())
			issuedTokens = RunListTokens(tClient, expectedUnspentTokens)

			By("redeeming tokens user1")
			tokenIDs = []*token.TokenId{{TxId: txID, Index: 0}}
			expectedRedeemTransaction.GetTokenAction().GetRedeem().Inputs = tokenIDs
			var quantityToRedeem uint64 = 50 // redeem 50 out of 119
			RunRedeemRequest(tClient, issuedTokens, ToHex(quantityToRedeem), expectedRedeemTransaction)

			By("listing tokens user1")
			expectedUnspentTokens = &token.UnspentTokens{
				Tokens: []*token.UnspentToken{
					{
						Quantity: ToDecimal(119 - 50),
						Type:     "ABC123",
						Id:       &token.TokenId{TxId: "ledger-id", Index: 0},
					},
				},
			}
			issuedTokens = RunListTokens(tClient, expectedUnspentTokens)
		})

	})

	Describe("basic solo network for token transaction bad path e2e ", func() {
		var (
			orderer *nwo.Orderer
			peer    *nwo.Peer
		)

		BeforeEach(func() {
			var err error

			network = nwo.New(BasicSoloV20(), testDir, client, StartPort(), components)
			network.GenerateConfigTree()

			network.Bootstrap()

			client, err = docker.NewClientFromEnv()
			Expect(err).NotTo(HaveOccurred())

			peer = network.Peer("Org1", "peer1")

			// Get recipients' identity
			recipientUser2Bytes, err := getIdentity(network, peer, "User2", "Org1MSP")
			Expect(err).NotTo(HaveOccurred())
			recipientUser2 = &token.TokenOwner{Raw: recipientUser2Bytes}
			tokensToIssue[0].Owner = &token.TokenOwner{Raw: recipientUser2Bytes}
			expectedTokenTransaction.GetTokenAction().GetIssue().Outputs[0].Owner = recipientUser2
			recipientUser1Bytes, err := getIdentity(network, peer, "User1", "Org1MSP")
			Expect(err).NotTo(HaveOccurred())
			recipientUser1 = &token.TokenOwner{Raw: recipientUser1Bytes}
			expectedTransferTransaction.GetTokenAction().GetTransfer().Outputs[0].Owner = &token.TokenOwner{Raw: recipientUser1Bytes}

			networkRunner := network.NetworkGroupRunner()
			process = ifrit.Invoke(networkRunner)
			Eventually(process.Ready()).Should(BeClosed())

			By("getting the orderer by name")
			orderer = network.Orderer("orderer")

			By("setting up the channel")
			network.CreateAndJoinChannel(orderer, "testchannel")
		})

		It("e2e token transfer double spending fails", func() {
			By("User1 issuing tokens to User2")
			tClient := GetTokenClient(network, peer, orderer, "User1", "Org1MSP")
			txID := RunIssueRequest(tClient, tokensToIssue, expectedTokenTransaction)

			By("User2 lists tokens as sanity check")
			tClient = GetTokenClient(network, peer, orderer, "User2", "Org1MSP")
			issuedTokens = RunListTokens(tClient, expectedUnspentTokens)
			Expect(issuedTokens).ToNot(BeNil())
			Expect(len(issuedTokens)).To(Equal(1))

			By("User2 transfers his token to User1")
			inputIDs := []*token.TokenId{{TxId: txID, Index: 0}}
			expectedTransferTransaction.GetTokenAction().GetTransfer().Inputs = inputIDs
			RunTransferRequest(tClient, issuedTokens, recipientUser1, expectedTransferTransaction)

			By("User2 try to transfer again the same token already transferred before")
			_, ordererStatus, committed, err := RunTransferRequestWithFailure(tClient, issuedTokens, recipientUser1)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("does not exist or not owned by the user"))
			Expect(ordererStatus).To(BeNil())
			Expect(committed).To(BeFalse())

			By("User2 try to transfer again the same token already transferred before but bypassing prover peer")
			expectedTokenTransaction = &token.TokenTransaction{
				Action: &token.TokenTransaction_TokenAction{
					TokenAction: &token.TokenAction{
						Data: &token.TokenAction_Transfer{
							Transfer: &token.Transfer{
								Inputs: []*token.TokenId{{TxId: txID, Index: 0}},
								Outputs: []*token.Token{{
									Owner:    &token.TokenOwner{Raw: []byte("test-owner")},
									Type:     "ABC123",
									Quantity: ToHex(119),
								}}}}}}}
			tempTxID, ordererStatus, committed, err := SubmitTokenTx(tClient, expectedTokenTransaction)
			Expect(err).To(HaveOccurred())
			Expect(err).To(MatchError(fmt.Sprintf("transaction [%s] status is not valid: INVALID_OTHER_REASON", tempTxID)))
			Expect(*ordererStatus).To(Equal(common.Status_SUCCESS))
			Expect(committed).To(BeFalse())
		})

		It("User1's spending User2's token", func() {
			By("User1 issuing tokens to User2")
			tClient := GetTokenClient(network, peer, orderer, "User1", "Org1MSP")
			txID := RunIssueRequest(tClient, tokensToIssue, expectedTokenTransaction)

			By("User2 lists tokens as sanity check")
			tClient = GetTokenClient(network, peer, orderer, "User2", "Org1MSP")
			issuedTokens = RunListTokens(tClient, expectedUnspentTokens)
			Expect(issuedTokens).ToNot(BeNil())
			Expect(len(issuedTokens)).To(Equal(1))

			By("User1 attempts to spend User2's tokens using the prover peer")
			tClient = GetTokenClient(network, peer, orderer, "User1", "Org1MSP")
			_, ordererStatus, committed, err := RunTransferRequestWithFailure(tClient, issuedTokens, recipientUser1)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("does not exist or not owned by the user"))
			Expect(ordererStatus).To(BeNil())
			Expect(committed).To(BeFalse())

			By("User1 attempts to spend User2's tokens by submitting a token transaction directly")
			expectedTokenTransaction = &token.TokenTransaction{
				Action: &token.TokenTransaction_TokenAction{
					TokenAction: &token.TokenAction{
						Data: &token.TokenAction_Transfer{
							Transfer: &token.Transfer{
								Inputs: []*token.TokenId{{TxId: txID, Index: 0}},
								Outputs: []*token.Token{{
									Owner:    &token.TokenOwner{Raw: []byte("test-owner")},
									Type:     "ABC123",
									Quantity: ToHex(119),
								}}}}}}}
			tempTxID, ordererStatus, committed, err := SubmitTokenTx(tClient, expectedTokenTransaction)
			Expect(err).To(HaveOccurred())
			Expect(err).To(MatchError(fmt.Sprintf("transaction [%s] status is not valid: INVALID_OTHER_REASON", tempTxID)))
			Expect(*ordererStatus).To(Equal(common.Status_SUCCESS))
			Expect(committed).To(BeFalse())
		})

		It("User1's spending its token to a nil output, to zero value", func() {
			By("User1 issuing tokens to User2")
			tClient := GetTokenClient(network, peer, orderer, "User1", "Org1MSP")
			txID := RunIssueRequest(tClient, tokensToIssue, expectedTokenTransaction)

			By("User2 lists tokens as sanity check")
			tClient = GetTokenClient(network, peer, orderer, "User2", "Org1MSP")
			issuedTokens = RunListTokens(tClient, expectedUnspentTokens)
			Expect(issuedTokens).ToNot(BeNil())
			Expect(len(issuedTokens)).To(Equal(1))

			By("User2 attempts to spend to a nil recipient by using the prover peer")
			tempTxID, ordererStatus, committed, err := RunTransferRequestWithFailure(tClient, issuedTokens, nil)
			Expect(err).To(HaveOccurred())
			Expect(err).To(MatchError("error from prover: invalid recipient in transfer request 'identity cannot be nil'"))
			Expect(ordererStatus).To(BeNil())
			Expect(committed).To(BeFalse())

			By("Check that nothing changed")
			tClient = GetTokenClient(network, peer, orderer, "User2", "Org1MSP")
			issuedTokens = RunListTokens(tClient, expectedUnspentTokens)
			Expect(issuedTokens).ToNot(BeNil())
			Expect(len(issuedTokens)).To(Equal(1))

			By("User2 attempts to spend to a nil recipient by assembling a faulty token transaction")
			expectedTokenTransaction = &token.TokenTransaction{
				Action: &token.TokenTransaction_TokenAction{
					TokenAction: &token.TokenAction{
						Data: &token.TokenAction_Transfer{
							Transfer: &token.Transfer{
								Inputs: []*token.TokenId{{TxId: txID, Index: 0}},
								Outputs: []*token.Token{{
									Owner:    nil,
									Type:     "ABC123",
									Quantity: ToHex(119),
								}}}}}}}
			tempTxID, ordererStatus, committed, err = SubmitTokenTx(tClient, expectedTokenTransaction)
			Expect(err).To(HaveOccurred())
			Expect(err).To(MatchError(fmt.Sprintf("transaction [%s] status is not valid: INVALID_OTHER_REASON", tempTxID)))
			Expect(*ordererStatus).To(Equal(common.Status_SUCCESS))
			Expect(committed).To(BeFalse())

			By("Check that nothing changed")
			tClient = GetTokenClient(network, peer, orderer, "User2", "Org1MSP")
			issuedTokens = RunListTokens(tClient, expectedUnspentTokens)
			Expect(issuedTokens).ToNot(BeNil())
			Expect(len(issuedTokens)).To(Equal(1))

			By("User2 attempts to spend to an invalid recipient by using the prover peer")
			tempTxID, ordererStatus, committed, err = RunTransferRequestWithFailure(tClient, issuedTokens, &token.TokenOwner{Raw: []byte{1, 2, 3, 4}})
			Expect(err).To(HaveOccurred())
			Expect(err).To(MatchError("error from prover: invalid recipient in transfer request 'identity [0x7261773a225c3030315c3030325c3030335c3030342220] cannot be deserialised: could not deserialize a SerializedIdentity: proto: msp.SerializedIdentity: illegal tag 0 (wire type 1)'"))
			Expect(ordererStatus).To(BeNil())
			Expect(committed).To(BeFalse())

			By("Check that nothing changed")
			tClient = GetTokenClient(network, peer, orderer, "User2", "Org1MSP")
			issuedTokens = RunListTokens(tClient, expectedUnspentTokens)
			Expect(issuedTokens).ToNot(BeNil())
			Expect(len(issuedTokens)).To(Equal(1))

			By("User2 attempts to spend to an invalid recipient by assembling a faulty token transaction")
			expectedTokenTransaction = &token.TokenTransaction{
				Action: &token.TokenTransaction_TokenAction{
					TokenAction: &token.TokenAction{
						Data: &token.TokenAction_Transfer{
							Transfer: &token.Transfer{
								Inputs: []*token.TokenId{{TxId: txID, Index: 0}},
								Outputs: []*token.Token{{
									Owner:    &token.TokenOwner{Raw: []byte{1, 2, 3, 4}},
									Type:     "ABC123",
									Quantity: ToHex(119),
								}}}}}}}
			tempTxID, ordererStatus, committed, err = SubmitTokenTx(tClient, expectedTokenTransaction)
			Expect(err).To(HaveOccurred())
			Expect(err).To(MatchError(fmt.Sprintf("transaction [%s] status is not valid: INVALID_OTHER_REASON", tempTxID)))
			Expect(*ordererStatus).To(Equal(common.Status_SUCCESS))
			Expect(committed).To(BeFalse())

			By("Check that nothing changed")
			tClient = GetTokenClient(network, peer, orderer, "User2", "Org1MSP")
			issuedTokens = RunListTokens(tClient, expectedUnspentTokens)
			Expect(issuedTokens).ToNot(BeNil())
			Expect(len(issuedTokens)).To(Equal(1))

			By("User2 attempts to transfer zero amount by assembling a faulty token transaction by using the prover peer")
			tempTxID, ordererStatus, committed, err = RunTransferRequestWithSharesAddFailure(tClient, issuedTokens,
				[]*token.RecipientShare{{Recipient: recipientUser1, Quantity: ToHex(0)}})
			Expect(err).To(HaveOccurred())
			Expect(err).To(MatchError("error from prover: invalid quantity in transfer request 'quantity must be larger than 0'"))
			Expect(ordererStatus).To(BeNil())
			Expect(committed).To(BeFalse())

			By("Check that nothing changed")
			tClient = GetTokenClient(network, peer, orderer, "User2", "Org1MSP")
			issuedTokens = RunListTokens(tClient, expectedUnspentTokens)
			Expect(issuedTokens).ToNot(BeNil())
			Expect(len(issuedTokens)).To(Equal(1))

			By("User2 attempts to transfer zero amount by assembling a faulty token transaction")
			expectedTokenTransaction = &token.TokenTransaction{
				Action: &token.TokenTransaction_TokenAction{
					TokenAction: &token.TokenAction{
						Data: &token.TokenAction_Transfer{
							Transfer: &token.Transfer{
								Inputs: []*token.TokenId{{TxId: txID, Index: 0}},
								Outputs: []*token.Token{{
									Owner:    recipientUser1,
									Type:     "ABC123",
									Quantity: ToHex(0),
								}}}}}}}
			tempTxID, ordererStatus, committed, err = SubmitTokenTx(tClient, expectedTokenTransaction)
			Expect(err).To(HaveOccurred())
			Expect(err).To(MatchError(fmt.Sprintf("transaction [%s] status is not valid: INVALID_OTHER_REASON", tempTxID)))
			Expect(*ordererStatus).To(Equal(common.Status_SUCCESS))
			Expect(committed).To(BeFalse())

			By("Check that nothing changed")
			tClient = GetTokenClient(network, peer, orderer, "User2", "Org1MSP")
			issuedTokens = RunListTokens(tClient, expectedUnspentTokens)
			Expect(issuedTokens).ToNot(BeNil())
			Expect(len(issuedTokens)).To(Equal(1))

			By("User2 attempts to transfer an over supported precision amount by assembling a faulty token transaction by using the prover peer")
			tempTxID, ordererStatus, committed, err = RunTransferRequestWithSharesAddFailure(tClient, issuedTokens,
				[]*token.RecipientShare{{Recipient: recipientUser1, Quantity: "99999999999999999999"}})
			Expect(err).To(HaveOccurred())
			Expect(err).To(MatchError("error from prover: invalid quantity in transfer request '99999999999999999999 has precision 67 > 64'"))
			Expect(ordererStatus).To(BeNil())
			Expect(committed).To(BeFalse())

			By("Check that nothing changed")
			tClient = GetTokenClient(network, peer, orderer, "User2", "Org1MSP")
			issuedTokens = RunListTokens(tClient, expectedUnspentTokens)
			Expect(issuedTokens).ToNot(BeNil())
			Expect(len(issuedTokens)).To(Equal(1))

			By("User2 attempts to transfer an over supported precision amount by assembling a faulty token transaction")
			expectedTokenTransaction = &token.TokenTransaction{
				Action: &token.TokenTransaction_TokenAction{
					TokenAction: &token.TokenAction{
						Data: &token.TokenAction_Transfer{
							Transfer: &token.Transfer{
								Inputs: []*token.TokenId{{TxId: txID, Index: 0}},
								Outputs: []*token.Token{{
									Owner:    recipientUser1,
									Type:     "ABC123",
									Quantity: "99999999999999999999",
								}}}}}}}
			tempTxID, ordererStatus, committed, err = SubmitTokenTx(tClient, expectedTokenTransaction)
			Expect(err).To(HaveOccurred())
			Expect(err).To(MatchError(fmt.Sprintf("transaction [%s] status is not valid: INVALID_OTHER_REASON", tempTxID)))
			Expect(*ordererStatus).To(Equal(common.Status_SUCCESS))
			Expect(committed).To(BeFalse())

			By("Check that nothing changed")
			tClient = GetTokenClient(network, peer, orderer, "User2", "Org1MSP")
			issuedTokens = RunListTokens(tClient, expectedUnspentTokens)
			Expect(issuedTokens).ToNot(BeNil())
			Expect(len(issuedTokens)).To(Equal(1))

			By("User2 attempts to transfer a negative amount by assembling a faulty token transaction by using the prover peer")
			tempTxID, ordererStatus, committed, err = RunTransferRequestWithSharesAddFailure(tClient, issuedTokens,
				[]*token.RecipientShare{{Recipient: recipientUser1, Quantity: "-99999999999999999999"}})
			Expect(err).To(HaveOccurred())
			Expect(err).To(MatchError("error from prover: invalid quantity in transfer request 'quantity must be larger than 0'"))
			Expect(ordererStatus).To(BeNil())
			Expect(committed).To(BeFalse())

			By("Check that nothing changed")
			tClient = GetTokenClient(network, peer, orderer, "User2", "Org1MSP")
			issuedTokens = RunListTokens(tClient, expectedUnspentTokens)
			Expect(issuedTokens).ToNot(BeNil())
			Expect(len(issuedTokens)).To(Equal(1))

			By("User2 attempts to transfer a negative amount by assembling a faulty token transaction")
			expectedTokenTransaction = &token.TokenTransaction{
				Action: &token.TokenTransaction_TokenAction{
					TokenAction: &token.TokenAction{
						Data: &token.TokenAction_Transfer{
							Transfer: &token.Transfer{
								Inputs: []*token.TokenId{{TxId: txID, Index: 0}},
								Outputs: []*token.Token{{
									Owner:    recipientUser1,
									Type:     "ABC123",
									Quantity: "-99999999999999999999",
								}}}}}}}
			tempTxID, ordererStatus, committed, err = SubmitTokenTx(tClient, expectedTokenTransaction)
			Expect(err).To(HaveOccurred())
			Expect(err).To(MatchError(fmt.Sprintf("transaction [%s] status is not valid: INVALID_OTHER_REASON", tempTxID)))
			Expect(*ordererStatus).To(Equal(common.Status_SUCCESS))
			Expect(committed).To(BeFalse())

			By("Check that nothing changed")
			tClient = GetTokenClient(network, peer, orderer, "User2", "Org1MSP")
			issuedTokens = RunListTokens(tClient, expectedUnspentTokens)
			Expect(issuedTokens).ToNot(BeNil())
			Expect(len(issuedTokens)).To(Equal(1))
		})

		Context("user1 sends malformed TokenTransaction", func() {

			var tClient *tokenclient.Client

			JustBeforeEach(func() {
				tClient = GetTokenClient(network, peer, orderer, "User1", "Org1MSP")
			})

			Context("when serialized token transaction is empty", func() {
				It("should fail with BAD_PAYLOAD", func() {
					var serializedTokenTx []byte
					txEnvelope, txId, err := tClient.TxSubmitter.CreateTxEnvelope(serializedTokenTx)
					Expect(err).ToNot(HaveOccurred())

					ordererStatus, committed, err := tClient.TxSubmitter.Submit(txEnvelope, 30*time.Second)

					Expect(err).To(HaveOccurred())
					Expect(err).To(MatchError(fmt.Sprintf("transaction [%s] status is not valid: BAD_PAYLOAD", txId)))
					Expect(*ordererStatus).To(Equal(common.Status_SUCCESS))
					Expect(committed).To(BeFalse())
				})
			})

			Context("when serialized token transaction is not a token transaction", func() {
				It("should fail with  BAD_PAYLOAD", func() {
					serializedTokenTx := []byte("I am not a token transaction")
					txEnvelope, txId, err := tClient.TxSubmitter.CreateTxEnvelope(serializedTokenTx)
					Expect(err).ToNot(HaveOccurred())

					ordererStatus, committed, err := tClient.TxSubmitter.Submit(txEnvelope, 30*time.Second)

					Expect(err).To(HaveOccurred())
					Expect(err).To(MatchError(fmt.Sprintf("transaction [%s] status is not valid: BAD_PAYLOAD", txId)))
					Expect(*ordererStatus).To(Equal(common.Status_SUCCESS))
					Expect(committed).To(BeFalse())
				})
			})

			Context("when token transaction has no action", func() {
				It("Should fail with BAD_PAYLOAD", func() {
					tokenTx := &token.TokenTransaction{}
					serializedTokenTx, err := proto.Marshal(tokenTx)
					Expect(err).ToNot(HaveOccurred())
					txEnvelope, txId, err := tClient.TxSubmitter.CreateTxEnvelope(serializedTokenTx)
					Expect(err).ToNot(HaveOccurred())

					ordererStatus, committed, err := tClient.TxSubmitter.Submit(txEnvelope, 30*time.Second)

					Expect(err).To(HaveOccurred())
					Expect(err).To(MatchError(fmt.Sprintf("transaction [%s] status is not valid: BAD_PAYLOAD", txId)))
					Expect(*ordererStatus).To(Equal(common.Status_SUCCESS))
					Expect(committed).To(BeFalse())
				})
			})

			Context("when token transaction action has no data", func() {
				It("Should fail with INVALID_OTHER_REASON", func() {
					tokenTx := &token.TokenTransaction{Action: &token.TokenTransaction_TokenAction{TokenAction: &token.TokenAction{}}}
					serializedTokenTx, err := proto.Marshal(tokenTx)
					Expect(err).ToNot(HaveOccurred())
					txEnvelope, txId, err := tClient.TxSubmitter.CreateTxEnvelope(serializedTokenTx)
					Expect(err).ToNot(HaveOccurred())

					ordererStatus, committed, err := tClient.TxSubmitter.Submit(txEnvelope, 30*time.Second)

					Expect(err).To(HaveOccurred())
					Expect(err).To(MatchError(fmt.Sprintf("transaction [%s] status is not valid: INVALID_OTHER_REASON", txId)))
					Expect(*ordererStatus).To(Equal(common.Status_SUCCESS))
					Expect(committed).To(BeFalse())
				})
			})

			Context("when token action is issue", func() {
				Context("and issue has no token", func() {
					It("Should fail with INVALID_OTHER_REASON", func() {
						tokenTx := &token.TokenTransaction{
							Action: &token.TokenTransaction_TokenAction{
								TokenAction: &token.TokenAction{
									Data: &token.TokenAction_Issue{
										Issue: &token.Issue{
											Outputs: nil,
										}}}}}

						serializedTokenTx, err := proto.Marshal(tokenTx)
						Expect(err).ToNot(HaveOccurred())
						txEnvelope, txId, err := tClient.TxSubmitter.CreateTxEnvelope(serializedTokenTx)
						Expect(err).ToNot(HaveOccurred())

						ordererStatus, committed, err := tClient.TxSubmitter.Submit(txEnvelope, 30*time.Second)

						Expect(err).To(HaveOccurred())
						Expect(err).To(MatchError(fmt.Sprintf("transaction [%s] status is not valid: INVALID_OTHER_REASON", txId)))
						Expect(*ordererStatus).To(Equal(common.Status_SUCCESS))
						Expect(committed).To(BeFalse())
					})
				})

				Context("and issue has invalid token", func() {
					It("Should fail with INVALID_OTHER_REASON", func() {
						tokenTx := &token.TokenTransaction{
							Action: &token.TokenTransaction_TokenAction{
								TokenAction: &token.TokenAction{
									Data: &token.TokenAction_Issue{
										Issue: &token.Issue{
											Outputs: []*token.Token{{}},
										}}}}}

						serializedTokenTx, err := proto.Marshal(tokenTx)
						Expect(err).ToNot(HaveOccurred())
						txEnvelope, txId, err := tClient.TxSubmitter.CreateTxEnvelope(serializedTokenTx)
						Expect(err).ToNot(HaveOccurred())

						ordererStatus, committed, err := tClient.TxSubmitter.Submit(txEnvelope, 30*time.Second)

						Expect(err).To(HaveOccurred())
						Expect(err).To(MatchError(fmt.Sprintf("transaction [%s] status is not valid: INVALID_OTHER_REASON", txId)))
						Expect(*ordererStatus).To(Equal(common.Status_SUCCESS))
						Expect(committed).To(BeFalse())
					})
				})
			})

			Context("when token action is transfer", func() {
				Context("and transfer has no inputs ", func() {
					It("Should fail with INVALID_OTHER_REASON", func() {
						RunIssueRequest(tClient, tokensToIssue, expectedTokenTransaction)

						tokenTx := &token.TokenTransaction{
							Action: &token.TokenTransaction_TokenAction{
								TokenAction: &token.TokenAction{
									Data: &token.TokenAction_Transfer{
										Transfer: &token.Transfer{
											Inputs: nil,
											Outputs: []*token.Token{{
												Owner:    recipientUser2,
												Type:     "ABC123",
												Quantity: ToHex(119),
											}}}}}}}

						serializedTokenTx, err := proto.Marshal(tokenTx)
						Expect(err).ToNot(HaveOccurred())
						txEnvelope, txId, err := tClient.TxSubmitter.CreateTxEnvelope(serializedTokenTx)
						Expect(err).ToNot(HaveOccurred())

						ordererStatus, committed, err := tClient.TxSubmitter.Submit(txEnvelope, 30*time.Second)

						Expect(err).To(HaveOccurred())
						Expect(err).To(MatchError(fmt.Sprintf("transaction [%s] status is not valid: INVALID_OTHER_REASON", txId)))
						Expect(*ordererStatus).To(Equal(common.Status_SUCCESS))
						Expect(committed).To(BeFalse())
					})
				})

				Context("and transfer has invalid input ", func() {
					It("Should fail with INVALID_OTHER_REASON", func() {
						RunIssueRequest(tClient, tokensToIssue, expectedTokenTransaction)

						tokenTx := &token.TokenTransaction{
							Action: &token.TokenTransaction_TokenAction{
								TokenAction: &token.TokenAction{
									Data: &token.TokenAction_Transfer{
										Transfer: &token.Transfer{
											Inputs: []*token.TokenId{{TxId: "xxx", Index: 0}},
											Outputs: []*token.Token{{
												Owner:    recipientUser2,
												Type:     "ABC123",
												Quantity: ToHex(119),
											}}}}}}}

						serializedTokenTx, err := proto.Marshal(tokenTx)
						Expect(err).ToNot(HaveOccurred())
						txEnvelope, txId, err := tClient.TxSubmitter.CreateTxEnvelope(serializedTokenTx)
						Expect(err).ToNot(HaveOccurred())

						ordererStatus, committed, err := tClient.TxSubmitter.Submit(txEnvelope, 30*time.Second)

						Expect(err).To(HaveOccurred())
						Expect(err).To(MatchError(fmt.Sprintf("transaction [%s] status is not valid: INVALID_OTHER_REASON", txId)))
						Expect(*ordererStatus).To(Equal(common.Status_SUCCESS))
						Expect(committed).To(BeFalse())
					})
				})

				Context("and transfer has no outputs", func() {
					It("Should fail with INVALID_OTHER_REASON", func() {
						TxId := RunIssueRequest(tClient, tokensToIssue, expectedTokenTransaction)

						tokenTx := &token.TokenTransaction{
							Action: &token.TokenTransaction_TokenAction{
								TokenAction: &token.TokenAction{
									Data: &token.TokenAction_Transfer{
										Transfer: &token.Transfer{
											Inputs:  []*token.TokenId{{TxId: TxId, Index: 0}},
											Outputs: nil,
										}}}}}

						serializedTokenTx, err := proto.Marshal(tokenTx)
						Expect(err).ToNot(HaveOccurred())
						txEnvelope, txId, err := tClient.TxSubmitter.CreateTxEnvelope(serializedTokenTx)
						Expect(err).ToNot(HaveOccurred())

						ordererStatus, committed, err := tClient.TxSubmitter.Submit(txEnvelope, 30*time.Second)

						Expect(err).To(HaveOccurred())
						Expect(err).To(MatchError(fmt.Sprintf("transaction [%s] status is not valid: INVALID_OTHER_REASON", txId)))
						Expect(*ordererStatus).To(Equal(common.Status_SUCCESS))
						Expect(committed).To(BeFalse())
					})
				})

				Context("and transfer has invalid output", func() {
					It("Should fail with INVALID_OTHER_REASON", func() {
						TxId := RunIssueRequest(tClient, tokensToIssue, expectedTokenTransaction)

						tokenTx := &token.TokenTransaction{
							Action: &token.TokenTransaction_TokenAction{
								TokenAction: &token.TokenAction{
									Data: &token.TokenAction_Transfer{
										Transfer: &token.Transfer{
											Inputs:  []*token.TokenId{{TxId: TxId, Index: 0}},
											Outputs: []*token.Token{{}},
										}}}}}

						serializedTokenTx, err := proto.Marshal(tokenTx)
						Expect(err).ToNot(HaveOccurred())
						txEnvelope, txId, err := tClient.TxSubmitter.CreateTxEnvelope(serializedTokenTx)
						Expect(err).ToNot(HaveOccurred())

						ordererStatus, committed, err := tClient.TxSubmitter.Submit(txEnvelope, 30*time.Second)

						Expect(err).To(HaveOccurred())
						Expect(err).To(MatchError(fmt.Sprintf("transaction [%s] status is not valid: INVALID_OTHER_REASON", txId)))
						Expect(*ordererStatus).To(Equal(common.Status_SUCCESS))
						Expect(committed).To(BeFalse())
					})
				})
			})

			Context("when token action is redeem", func() {
				Context("and redeem has no input", func() {
					It("Should fail with INVALID_OTHER_REASON", func() {
						RunIssueRequest(tClient, tokensToIssue, expectedTokenTransaction)

						tokenTx := &token.TokenTransaction{
							Action: &token.TokenTransaction_TokenAction{
								TokenAction: &token.TokenAction{
									Data: &token.TokenAction_Redeem{
										Redeem: &token.Transfer{
											Inputs: nil,
											Outputs: []*token.Token{{
												Type:     "ABC123",
												Quantity: ToHex(119),
											}}}}}}}

						serializedTokenTx, err := proto.Marshal(tokenTx)
						Expect(err).ToNot(HaveOccurred())
						txEnvelope, txId, err := tClient.TxSubmitter.CreateTxEnvelope(serializedTokenTx)
						Expect(err).ToNot(HaveOccurred())

						ordererStatus, committed, err := tClient.TxSubmitter.Submit(txEnvelope, 30*time.Second)

						Expect(err).To(HaveOccurred())
						Expect(err).To(MatchError(fmt.Sprintf("transaction [%s] status is not valid: INVALID_OTHER_REASON", txId)))
						Expect(*ordererStatus).To(Equal(common.Status_SUCCESS))
						Expect(committed).To(BeFalse())
					})
				})

				Context("and redeem has no output", func() {
					It("Should fail with INVALID_OTHER_REASON", func() {
						TxId := RunIssueRequest(tClient, tokensToIssue, expectedTokenTransaction)

						tokenTx := &token.TokenTransaction{
							Action: &token.TokenTransaction_TokenAction{
								TokenAction: &token.TokenAction{
									Data: &token.TokenAction_Redeem{
										Redeem: &token.Transfer{
											Inputs:  []*token.TokenId{{TxId: TxId, Index: 0}},
											Outputs: nil,
										}}}}}

						serializedTokenTx, err := proto.Marshal(tokenTx)
						Expect(err).ToNot(HaveOccurred())
						txEnvelope, txId, err := tClient.TxSubmitter.CreateTxEnvelope(serializedTokenTx)
						Expect(err).ToNot(HaveOccurred())

						ordererStatus, committed, err := tClient.TxSubmitter.Submit(txEnvelope, 30*time.Second)

						Expect(err).To(HaveOccurred())
						Expect(err).To(MatchError(fmt.Sprintf("transaction [%s] status is not valid: INVALID_OTHER_REASON", txId)))
						Expect(*ordererStatus).To(Equal(common.Status_SUCCESS))
						Expect(committed).To(BeFalse())
					})
				})

				Context("and redeem has more than two outputs", func() {
					It("Should fail with INVALID_OTHER_REASON", func() {
						TxId := RunIssueRequest(tClient, tokensToIssue, expectedTokenTransaction)

						tokenTx := &token.TokenTransaction{
							Action: &token.TokenTransaction_TokenAction{
								TokenAction: &token.TokenAction{
									Data: &token.TokenAction_Redeem{
										Redeem: &token.Transfer{
											Inputs: []*token.TokenId{{TxId: TxId, Index: 0}},
											Outputs: []*token.Token{
												{Type: "ABC123", Quantity: ToHex(111)},
												{Owner: recipientUser1, Type: "ABC123", Quantity: ToHex(8)},
												{Owner: recipientUser2, Type: "ABC123", Quantity: ToHex(8)},
											}}}}}}

						serializedTokenTx, err := proto.Marshal(tokenTx)
						Expect(err).ToNot(HaveOccurred())
						txEnvelope, txId, err := tClient.TxSubmitter.CreateTxEnvelope(serializedTokenTx)
						Expect(err).ToNot(HaveOccurred())

						ordererStatus, committed, err := tClient.TxSubmitter.Submit(txEnvelope, 30*time.Second)

						Expect(err).To(HaveOccurred())
						Expect(err).To(MatchError(fmt.Sprintf("transaction [%s] status is not valid: INVALID_OTHER_REASON", txId)))
						Expect(*ordererStatus).To(Equal(common.Status_SUCCESS))
						Expect(committed).To(BeFalse())
					})
				})
			})

		})
	})

	Describe("custom solo network for token double spending", func() {
		var (
			orderer *nwo.Orderer
			peer    *nwo.Peer
			tClient *tokenclient.Client
			txId    string
		)

		BeforeEach(func() {
			var err error

			config := BasicSoloV20()

			// override default block size
			customTxTemplate := nwo.DefaultConfigTxTemplate
			customTxTemplate = strings.Replace(customTxTemplate, "MaxMessageCount: 1", "MaxMessageCount: 2", 1)
			customTxTemplate = strings.Replace(customTxTemplate, "BatchTimeout: 1s", "BatchTimeout: 10s", 1)

			config.Templates = &nwo.Templates{}
			config.Templates.ConfigTx = customTxTemplate

			network = nwo.New(config, testDir, client, StartPort(), components)
			network.GenerateConfigTree()

			network.Bootstrap()

			client, err = docker.NewClientFromEnv()
			Expect(err).NotTo(HaveOccurred())

			networkRunner := network.NetworkGroupRunner()
			process = ifrit.Invoke(networkRunner)
			Eventually(process.Ready()).Should(BeClosed())

			orderer = network.Orderer("orderer")
			network.CreateAndJoinChannel(orderer, "testchannel")

			peer = network.Peer("Org1", "peer1")

			recipientUser2Bytes, err := getIdentity(network, peer, "User2", "Org1MSP")
			Expect(err).NotTo(HaveOccurred())
			recipientUser2 = &token.TokenOwner{Raw: recipientUser2Bytes}
			tokensToIssue[0].Owner = recipientUser2
			expectedTokenTransaction.GetTokenAction().GetIssue().Outputs[0].Owner = recipientUser2
			recipientUser1Bytes, err := getIdentity(network, peer, "User1", "Org1MSP")
			Expect(err).NotTo(HaveOccurred())
			recipientUser1 = &token.TokenOwner{Raw: recipientUser1Bytes}
			expectedTransferTransaction.GetTokenAction().GetTransfer().Outputs[0].Owner = recipientUser1
			expectedRedeemTransaction.GetTokenAction().GetRedeem().Outputs[1].Owner = recipientUser1

			tClient := GetTokenClient(network, peer, orderer, "User1", "Org1MSP")

			By("User1 issuing tokens to User2")
			txId = RunIssueRequest(tClient, tokensToIssue, expectedTokenTransaction)

			By("User2 lists tokens as sanity check")
			tClient = GetTokenClient(network, peer, orderer, "User2", "Org1MSP")
			issuedTokens = RunListTokens(tClient, expectedUnspentTokens)
			Expect(issuedTokens).ToNot(BeNil())
			Expect(len(issuedTokens)).To(Equal(1))

			waitForBlockReception(orderer, peer, network, "testchannel", 1)
		})

		It("double spending with resend transfer results in INVALID_OTHER_REASON", func() {
			tClient = GetTokenClient(network, peer, orderer, "User2", "Org1MSP")

			By("Prepare token transfer")
			expectedTransferTransaction.GetTokenAction().GetTransfer().Inputs = []*token.TokenId{{TxId: txId, Index: 0}}
			inputTokenIDs := make([]*token.TokenId, len(issuedTokens))
			for i, inToken := range issuedTokens {
				inputTokenIDs[i] = &token.TokenId{TxId: inToken.GetId().TxId, Index: inToken.GetId().Index}
			}
			shares1 := []*token.RecipientShare{{Recipient: recipientUser1, Quantity: ToHex(20)}}

			By("create transfer transaction using prover peer")
			serializedTokenTx1, err := tClient.Prover.RequestTransfer(inputTokenIDs, shares1, tClient.SigningIdentity)
			Expect(err).NotTo(HaveOccurred())
			txEnvelope1, txId1, err := tClient.TxSubmitter.CreateTxEnvelope(serializedTokenTx1)
			Expect(err).NotTo(HaveOccurred())

			By("register for transaction commit events")
			ctx1, cancelFunc1 := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancelFunc1()
			done1, cErr1, err := RegisterForCommitEvent(txId1, ctx1, tClient)
			Expect(err).NotTo(HaveOccurred())

			By("User2 transfers his token to User1")
			ordererStatus, committed, err := tClient.TxSubmitter.Submit(txEnvelope1, 0)
			Expect(err).NotTo(HaveOccurred())
			Expect(*ordererStatus).To(Equal(common.Status_SUCCESS))
			Expect(committed).To(BeFalse())

			By("User2 submits double spending attack")
			ordererStatus, _, err = tClient.TxSubmitter.Submit(txEnvelope1, 0)
			Expect(err).NotTo(HaveOccurred())
			Expect(*ordererStatus).To(Equal(common.Status_SUCCESS))

			By("Waiting for committing")
			Eventually(<-done1, 30*time.Second, time.Second).Should(BeTrue())
			Expect(<-cErr1).To(MatchError(fmt.Sprintf("transaction [%s] status is not valid: INVALID_OTHER_REASON", txId1)))

			By("User2 lists tokens as sanity check to make sure no tx has been performed")
			expectedTokens := &token.UnspentTokens{
				Tokens: []*token.UnspentToken{
					{
						Quantity: ToDecimal(119),
						Type:     "ABC123",
						Id:       &token.TokenId{TxId: txId1, Index: 1},
					},
				},
			}
			tClient = GetTokenClient(network, peer, orderer, "User2", "Org1MSP")
			RunListTokens(tClient, expectedTokens)

			By("check that transactions have been processed in the same block")
			waitForBlockReception(orderer, peer, network, "testchannel", 2)
		})

		It("double spending with transfer results in MVCC_READ_CONFLICT", func() {
			tClient = GetTokenClient(network, peer, orderer, "User2", "Org1MSP")

			By("Prepare token transfer")
			expectedTransferTransaction.GetTokenAction().GetTransfer().Inputs = []*token.TokenId{{TxId: txId, Index: 0}}
			inputTokenIDs := make([]*token.TokenId, len(issuedTokens))
			for i, inToken := range issuedTokens {
				inputTokenIDs[i] = &token.TokenId{TxId: inToken.GetId().TxId, Index: inToken.GetId().Index}
			}
			shares1 := []*token.RecipientShare{{Recipient: recipientUser1, Quantity: ToHex(119)}}

			By("create transfer transaction using prover peer")
			serializedTokenTx1, err := tClient.Prover.RequestTransfer(inputTokenIDs, shares1, tClient.SigningIdentity)
			Expect(err).NotTo(HaveOccurred())
			txEnvelope1, txId1, err := tClient.TxSubmitter.CreateTxEnvelope(serializedTokenTx1)
			Expect(err).NotTo(HaveOccurred())

			By("create another transfer transaction with same input using prover peer")
			serializedTokenTx2, err := tClient.Prover.RequestTransfer(inputTokenIDs, shares1, tClient.SigningIdentity)
			Expect(err).NotTo(HaveOccurred())
			txEnvelope2, txId2, err := tClient.TxSubmitter.CreateTxEnvelope(serializedTokenTx2)
			Expect(err).NotTo(HaveOccurred())

			By("register for transaction commit events")
			ctx1, cancelFunc1 := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancelFunc1()
			done1, cErr1, err := RegisterForCommitEvent(txId1, ctx1, tClient)
			Expect(err).NotTo(HaveOccurred())
			ctx2, cancelFunc2 := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancelFunc2()
			done2, cErr2, err := RegisterForCommitEvent(txId2, ctx2, tClient)
			Expect(err).NotTo(HaveOccurred())

			By("User2 transfers his token to User1")
			ordererStatus, committed, err := tClient.TxSubmitter.Submit(txEnvelope1, 0)
			Expect(err).NotTo(HaveOccurred())
			Expect(*ordererStatus).To(Equal(common.Status_SUCCESS))
			Expect(committed).To(BeFalse())

			By("User2 submits double spending attack")
			ordererStatus, _, err = tClient.TxSubmitter.Submit(txEnvelope2, 0)
			Expect(err).NotTo(HaveOccurred())
			Expect(*ordererStatus).To(Equal(common.Status_SUCCESS))

			By("Waiting for committing")
			Eventually(<-done1, 30*time.Second, time.Second).Should(BeTrue())
			Eventually(<-done2, 30*time.Second, time.Second).Should(BeTrue())
			Expect(<-cErr1).ToNot(HaveOccurred())
			Expect(<-cErr2).To(MatchError(fmt.Sprintf("transaction [%s] status is not valid: MVCC_READ_CONFLICT", txId2)))

			By("User2 lists tokens as sanity check")
			tClient = GetTokenClient(network, peer, orderer, "User2", "Org1MSP")
			RunListTokens(tClient, nil)

			By("check that transactions have been processed in the same block")
			waitForBlockReception(orderer, peer, network, "testchannel", 2)
		})

		It("double spending with redeem results in MVCC_READ_CONFLICT", func() {
			tClient = GetTokenClient(network, peer, orderer, "User2", "Org1MSP")

			By("Prepare token transfer")
			expectedTransferTransaction.GetTokenAction().GetTransfer().Inputs = []*token.TokenId{{TxId: txId, Index: 0}}
			inputTokenIDs := make([]*token.TokenId, len(issuedTokens))
			for i, inToken := range issuedTokens {
				inputTokenIDs[i] = &token.TokenId{TxId: inToken.GetId().TxId, Index: inToken.GetId().Index}
			}
			shares := []*token.RecipientShare{{Recipient: recipientUser1, Quantity: ToHex(119)}}

			By("create transfer transaction using prover peer")
			serializedTokenTx1, err := tClient.Prover.RequestTransfer(inputTokenIDs, shares, tClient.SigningIdentity)
			Expect(err).NotTo(HaveOccurred())
			txEnvelope1, txId1, err := tClient.TxSubmitter.CreateTxEnvelope(serializedTokenTx1)
			Expect(err).NotTo(HaveOccurred())

			By("create redeem transaction with same input using prover peer")
			serializedTokenTx2, err := tClient.Prover.RequestRedeem(inputTokenIDs, ToHex(119), tClient.SigningIdentity)
			Expect(err).NotTo(HaveOccurred())
			txEnvelope2, txId2, err := tClient.TxSubmitter.CreateTxEnvelope(serializedTokenTx2)
			Expect(err).NotTo(HaveOccurred())

			By("register for transaction commit events")
			ctx1, cancelFunc1 := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancelFunc1()
			done1, cErr1, err := RegisterForCommitEvent(txId1, ctx1, tClient)
			Expect(err).NotTo(HaveOccurred())
			ctx2, cancelFunc2 := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancelFunc2()
			done2, cErr2, err := RegisterForCommitEvent(txId2, ctx2, tClient)
			Expect(err).NotTo(HaveOccurred())

			By("User2 transfers his token to User1")
			ordererStatus, committed, err := tClient.TxSubmitter.Submit(txEnvelope1, 0)
			Expect(err).NotTo(HaveOccurred())
			Expect(*ordererStatus).To(Equal(common.Status_SUCCESS))
			Expect(committed).To(BeFalse())

			By("User2 submits double spending attack")
			ordererStatus, _, err = tClient.TxSubmitter.Submit(txEnvelope2, 0)
			Expect(err).NotTo(HaveOccurred())
			Expect(*ordererStatus).To(Equal(common.Status_SUCCESS))

			By("Waiting for committing")
			Eventually(<-done1, 30*time.Second, time.Second).Should(BeTrue())
			Eventually(<-done2, 30*time.Second, time.Second).Should(BeTrue())
			Expect(<-cErr1).NotTo(HaveOccurred())
			Expect(<-cErr2).To(MatchError(fmt.Sprintf("transaction [%s] status is not valid: MVCC_READ_CONFLICT", txId2)))

			By("User2 lists tokens as sanity check")
			tClient = GetTokenClient(network, peer, orderer, "User2", "Org1MSP")
			RunListTokens(tClient, nil)

			By("User1 lists tokens as sanity check")
			expectedTokens := &token.UnspentTokens{
				Tokens: []*token.UnspentToken{
					{
						Quantity: ToDecimal(119),
						Type:     "ABC123",
						Id:       &token.TokenId{TxId: txId1, Index: 1},
					},
				},
			}
			tClient = GetTokenClient(network, peer, orderer, "User1", "Org1MSP")
			RunListTokens(tClient, expectedTokens)

			By("check that transactions have been processed in the same block")
			waitForBlockReception(orderer, peer, network, "testchannel", 2)
		})
	})

})

func waitForBlockReception(o *nwo.Orderer, submitter *nwo.Peer, network *nwo.Network, channelName string, blockSeq int) {
	c := commands.ChannelFetch{
		ChannelID:  channelName,
		Block:      "newest",
		OutputFile: "/dev/null",
		Orderer:    network.OrdererAddress(o, nwo.ListenPort),
	}
	Eventually(func() string {
		sess, err := network.OrdererAdminSession(o, submitter, c)
		Expect(err).NotTo(HaveOccurred())
		Eventually(sess, network.EventuallyTimeout).Should(gexec.Exit())
		if sess.ExitCode() != 0 {
			return fmt.Sprintf("exit code is %d: %s", sess.ExitCode(), string(sess.Err.Contents()))
		}
		sessErr := string(sess.Err.Contents())
		expected := fmt.Sprintf("Received block: %d", blockSeq)
		if strings.Contains(sessErr, expected) {
			return ""
		}
		return sessErr
	}, network.EventuallyTimeout, time.Second).Should(BeEmpty())
}

func GetTokenClient(network *nwo.Network, peer *nwo.Peer, orderer *nwo.Orderer, user, mspID string) *tokenclient.Client {
	config := getClientConfig(network, peer, orderer, "testchannel", user, mspID)
	signingIdentity, err := getSigningIdentity(config.MSPInfo.MSPConfigPath, config.MSPInfo.MSPID, config.MSPInfo.MSPType)
	Expect(err).NotTo(HaveOccurred())
	tClient, err := tokenclient.NewClient(*config, signingIdentity)
	Expect(err).NotTo(HaveOccurred())

	return tClient
}

func RegisterForCommitEvent(txid string, ctx context.Context, s *tokenclient.Client) (<-chan bool, <-chan error, error) {

	dc, err := client.NewDeliverClient(&s.Config.CommitterPeer)
	if err != nil {
		return nil, nil, err
	}

	deliverFiltered, err := dc.NewDeliverFiltered(ctx)
	if err != nil {
		return nil, nil, err
	}

	creator, _ := s.SigningIdentity.Serialize()

	blockEnvelope, err := client.CreateDeliverEnvelope(s.Config.ChannelID, creator, s.SigningIdentity, dc.Certificate())
	if err != nil {
		return nil, nil, err
	}
	err = client.DeliverSend(deliverFiltered, s.Config.CommitterPeer.Address, blockEnvelope)
	if err != nil {
		return nil, nil, err
	}

	eventCh := make(chan client.TxEvent, 1)
	go client.DeliverReceive(deliverFiltered, s.Config.CommitterPeer.Address, txid, eventCh)

	done := make(chan bool)
	errorCh := make(chan error)
	go func() {
		select {
		case event, _ := <-eventCh:
			if txid == event.Txid {
				done <- true
				errorCh <- event.Err
			} else {
				Fail("Received wrong event")
			}
		case <-ctx.Done():
			Fail("CTX timeout must not happen")
		}
	}()

	return done, errorCh, nil
}

func RunIssueRequest(c *tokenclient.Client, tokens []*token.Token, expectedTokenTx *token.TokenTransaction) string {
	envelope, txid, ordererStatus, committed, err := c.Issue(tokens, 30*time.Second)
	Expect(err).NotTo(HaveOccurred())
	Expect(*ordererStatus).To(Equal(common.Status_SUCCESS))
	Expect(committed).To(Equal(true))

	payload := common.Payload{}
	err = proto.Unmarshal(envelope.Payload, &payload)

	tokenTx := &token.TokenTransaction{}
	err = proto.Unmarshal(payload.Data, tokenTx)
	Expect(proto.Equal(tokenTx, expectedTokenTx)).To(BeTrue())

	tokenTxid, err := tokenclient.GetTransactionID(envelope)
	Expect(err).NotTo(HaveOccurred())
	Expect(tokenTxid).To(Equal(txid))
	return txid
}

func RunListTokens(c *tokenclient.Client, expectedUnspentTokens *token.UnspentTokens) []*token.UnspentToken {
	tokenOutputs, err := c.ListTokens()
	Expect(err).NotTo(HaveOccurred())

	// unmarshall CommandResponse and verify the result
	if expectedUnspentTokens == nil {
		Expect(len(tokenOutputs)).To(Equal(0))
	} else {
		Expect(len(tokenOutputs)).To(Equal(len(expectedUnspentTokens.Tokens)))
		for i, token := range expectedUnspentTokens.Tokens {
			Expect(tokenOutputs[i].Type).To(Equal(token.Type))
			Expect(tokenOutputs[i].Quantity).To(Equal(token.Quantity))
		}
	}
	return tokenOutputs
}

func RunTransferRequest(c *tokenclient.Client, inputTokens []*token.UnspentToken, recipient *token.TokenOwner, expectedTokenTx *token.TokenTransaction) string {
	inputTokenIDs := make([]*token.TokenId, len(inputTokens))
	for i, inToken := range inputTokens {
		inputTokenIDs[i] = &token.TokenId{TxId: inToken.GetId().TxId, Index: inToken.GetId().Index}
	}
	shares := []*token.RecipientShare{
		{Recipient: recipient, Quantity: ToHex(119)},
	}

	envelope, txid, ordererStatus, committed, err := c.Transfer(inputTokenIDs, shares, 30*time.Second)
	Expect(err).NotTo(HaveOccurred())
	Expect(*ordererStatus).To(Equal(common.Status_SUCCESS))
	Expect(committed).To(Equal(true))

	payload := common.Payload{}
	err = proto.Unmarshal(envelope.Payload, &payload)

	tokenTx := &token.TokenTransaction{}
	err = proto.Unmarshal(payload.Data, tokenTx)
	By(tokenTx.String())
	By(expectedTokenTx.String())

	Expect(proto.Equal(tokenTx, expectedTokenTx)).To(BeTrue())

	tokenTxid, err := tokenclient.GetTransactionID(envelope)
	Expect(err).NotTo(HaveOccurred())
	Expect(tokenTxid).To(Equal(txid))

	// Verify the result
	Expect(tokenTx.GetTokenAction().GetTransfer().GetInputs()).ToNot(BeNil())
	Expect(len(tokenTx.GetTokenAction().GetTransfer().GetInputs())).To(Equal(1))
	Expect(tokenTx.GetTokenAction().GetTransfer().GetOutputs()).ToNot(BeNil())

	return txid
}

func RunRedeemRequest(c *tokenclient.Client, inputTokens []*token.UnspentToken, quantity string, expectedTokenTx *token.TokenTransaction) {
	inputTokenIDs := make([]*token.TokenId, len(inputTokens))
	for i, inToken := range inputTokens {
		inputTokenIDs[i] = &token.TokenId{TxId: inToken.GetId().TxId, Index: inToken.GetId().Index}
	}

	envelope, txid, ordererStatus, committed, err := c.Redeem(inputTokenIDs, quantity, 30*time.Second)
	Expect(err).NotTo(HaveOccurred())
	Expect(*ordererStatus).To(Equal(common.Status_SUCCESS))
	Expect(committed).To(Equal(true))

	payload := common.Payload{}
	err = proto.Unmarshal(envelope.Payload, &payload)

	tokenTx := &token.TokenTransaction{}
	err = proto.Unmarshal(payload.Data, tokenTx)
	Expect(proto.Equal(tokenTx, expectedTokenTx)).To(BeTrue())

	tokenTxid, err := tokenclient.GetTransactionID(envelope)
	Expect(err).NotTo(HaveOccurred())
	Expect(tokenTxid).To(Equal(txid))
}

func RunTransferRequestWithFailure(c *tokenclient.Client, inputTokens []*token.UnspentToken, recipient *token.TokenOwner) (string, *common.Status, bool, error) {
	inputTokenIDs := make([]*token.TokenId, len(inputTokens))
	sum := plain.NewZeroQuantity(64)
	var err error
	for i, inToken := range inputTokens {
		inputTokenIDs[i] = &token.TokenId{TxId: inToken.GetId().TxId, Index: inToken.GetId().Index}

		v, err := plain.ToQuantity(inToken.GetQuantity(), 64)
		Expect(err).NotTo(HaveOccurred())
		sum, err = sum.Add(v)
		Expect(err).NotTo(HaveOccurred())
	}
	shares := []*token.RecipientShare{
		{Recipient: recipient, Quantity: sum.Hex()},
	}

	_, txid, ordererStatus, committed, err := c.Transfer(inputTokenIDs, shares, 30*time.Second)
	return txid, ordererStatus, committed, err
}

func RunTransferRequestWithSharesAddFailure(c *tokenclient.Client, inputTokens []*token.UnspentToken, shares []*token.RecipientShare) (string, *common.Status, bool, error) {
	inputTokenIDs := make([]*token.TokenId, len(inputTokens))
	sum := plain.NewZeroQuantity(64)
	var err error
	for i, inToken := range inputTokens {
		inputTokenIDs[i] = &token.TokenId{TxId: inToken.GetId().TxId, Index: inToken.GetId().Index}

		v, err := plain.ToQuantity(inToken.GetQuantity(), 64)
		Expect(err).NotTo(HaveOccurred())
		sum, err = sum.Add(v)
		Expect(err).NotTo(HaveOccurred())
	}
	_, txid, ordererStatus, committed, err := c.Transfer(inputTokenIDs, shares, 30*time.Second)
	return txid, ordererStatus, committed, err
}

func SubmitTokenTx(c *tokenclient.Client, tokenTx *token.TokenTransaction) (string, *common.Status, bool, error) {
	serializedTokenTx, err := proto.Marshal(tokenTx)
	Expect(err).ToNot(HaveOccurred())

	txEnvelope, txid, err := c.TxSubmitter.CreateTxEnvelope(serializedTokenTx)
	Expect(err).ToNot(HaveOccurred())

	ordererStatus, committed, err := c.TxSubmitter.Submit(txEnvelope, 30*time.Second)
	return txid, ordererStatus, committed, err
}

func getShellIdentity(n *nwo.Network, peer *nwo.Peer, user, mspId string) string {
	orderer := n.Orderer("orderer")
	config := getClientConfig(n, peer, orderer, "testchannel", user, mspId)

	return mspId + ":" + config.MSPInfo.MSPConfigPath
}

func getIdentity(n *nwo.Network, peer *nwo.Peer, user, mspId string) ([]byte, error) {
	orderer := n.Orderer("orderer")
	config := getClientConfig(n, peer, orderer, "testchannel", user, mspId)

	localMSP, err := LoadLocalMSPAt(config.MSPInfo.MSPConfigPath, config.MSPInfo.MSPID, config.MSPInfo.MSPType)
	if err != nil {
		return nil, err
	}

	signer, err := localMSP.GetDefaultSigningIdentity()
	if err != nil {
		return nil, err
	}

	creator, err := signer.Serialize()
	if err != nil {
		return nil, err
	}

	return creator, nil
}

func getSigningIdentity(mspConfigPath, mspID, mspType string) (tk.SigningIdentity, error) {
	mspInstance, err := LoadLocalMSPAt(mspConfigPath, mspID, mspType)
	if err != nil {
		return nil, err
	}

	signingIdentity, err := mspInstance.GetDefaultSigningIdentity()
	if err != nil {
		return nil, err
	}

	return signingIdentity, nil
}

// LoadLocalMSPAt loads an MSP whose configuration is stored at 'dir', and whose
// id and type are the passed as arguments.
func LoadLocalMSPAt(dir, id, mspType string) (msp.MSP, error) {
	if mspType != "bccsp" {
		return nil, errors.Errorf("invalid msp type, expected 'bccsp', got %s", mspType)
	}
	conf, err := msp.GetLocalMspConfig(dir, nil, id)
	if err != nil {
		return nil, err
	}
	ks, err := sw.NewFileBasedKeyStore(nil, filepath.Join(dir, "keystore"), true)
	if err != nil {
		return nil, err
	}
	thisMSP, err := msp.NewBccspMspWithKeyStore(msp.MSPv1_0, ks)
	if err != nil {
		return nil, err
	}
	err = thisMSP.Setup(conf)
	if err != nil {
		return nil, err
	}
	return thisMSP, nil
}

// createCompositeKey and its related functions and consts copied from core/chaincode/shim/chaincode.go
func createCompositeKey(objectType string, attributes []string) (string, error) {
	if err := validateCompositeKeyAttribute(objectType); err != nil {
		return "", err
	}
	ck := compositeKeyNamespace + objectType + string(minUnicodeRuneValue)
	for _, att := range attributes {
		if err := validateCompositeKeyAttribute(att); err != nil {
			return "", err
		}
		ck += att + string(minUnicodeRuneValue)
	}
	return ck, nil
}

func validateCompositeKeyAttribute(str string) error {
	if !utf8.ValidString(str) {
		return errors.Errorf("not a valid utf8 string: [%x]", str)
	}
	for index, runeValue := range str {
		if runeValue == minUnicodeRuneValue || runeValue == maxUnicodeRuneValue {
			return errors.Errorf(`input contain unicode %#U starting at position [%d]. %#U and %#U are not allowed in the input attribute of a composite key`,
				runeValue, index, minUnicodeRuneValue, maxUnicodeRuneValue)
		}
	}
	return nil
}

const (
	minUnicodeRuneValue   = 0            //U+0000
	maxUnicodeRuneValue   = utf8.MaxRune //U+10FFFF - maximum (and unallocated) code point
	compositeKeyNamespace = "\x00"
)
