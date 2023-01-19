/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package encoder_test

import (
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/golang/protobuf/proto"
	cb "github.com/hyperledger/fabric-protos-go/common"
	ab "github.com/hyperledger/fabric-protos-go/orderer"
	"github.com/hyperledger/fabric-protos-go/orderer/etcdraft"
	"github.com/hyperledger/fabric-protos-go/orderer/smartbft"
	"github.com/hyperledger/fabric/common/util"
	"github.com/hyperledger/fabric/internal/configtxgen/encoder"
	"github.com/hyperledger/fabric/internal/configtxgen/encoder/fakes"
	"github.com/hyperledger/fabric/internal/configtxgen/genesisconfig"
	"github.com/hyperledger/fabric/internal/pkg/identity"
	"github.com/hyperledger/fabric/protoutil"
)

//go:generate counterfeiter -o fakes/signer_serializer.go --fake-name SignerSerializer . signerSerializer
type signerSerializer interface {
	identity.SignerSerializer
}

func CreateStandardPolicies() map[string]*genesisconfig.Policy {
	return map[string]*genesisconfig.Policy{
		"Admins": {
			Type: "ImplicitMeta",
			Rule: "ANY Admins",
		},
		"Readers": {
			Type: "ImplicitMeta",
			Rule: "ANY Readers",
		},
		"Writers": {
			Type: "ImplicitMeta",
			Rule: "ANY Writers",
		},
	}
}

func CreateStandardOrdererPolicies() map[string]*genesisconfig.Policy {
	policies := CreateStandardPolicies()

	policies["BlockValidation"] = &genesisconfig.Policy{
		Type: "ImplicitMeta",
		Rule: "ANY Admins",
	}

	return policies
}

var _ = Describe("Encoder", func() {
	Describe("AddOrdererPolicies", func() {
		var (
			cg       *cb.ConfigGroup
			policies map[string]*genesisconfig.Policy
		)

		BeforeEach(func() {
			cg = protoutil.NewConfigGroup()
			policies = CreateStandardOrdererPolicies()
		})

		It("adds the block validation policy to the group", func() {
			err := encoder.AddOrdererPolicies(cg, policies, "Admins")
			Expect(err).NotTo(HaveOccurred())
			Expect(len(cg.Policies)).To(Equal(4))

			Expect(cg.Policies["BlockValidation"].Policy).To(Equal(&cb.Policy{
				Type: int32(cb.Policy_IMPLICIT_META),
				Value: protoutil.MarshalOrPanic(&cb.ImplicitMetaPolicy{
					SubPolicy: "Admins",
					Rule:      cb.ImplicitMetaPolicy_ANY,
				}),
			}))
		})

		Context("when the policy map is nil", func() {
			BeforeEach(func() {
				policies = nil
			})

			It("returns an error", func() {
				err := encoder.AddOrdererPolicies(cg, policies, "Admins")
				Expect(err).To(MatchError("no policies defined"))
			})
		})

		Context("when the policy map is missing 'BlockValidation'", func() {
			BeforeEach(func() {
				delete(policies, "BlockValidation")
			})

			It("returns an error", func() {
				err := encoder.AddOrdererPolicies(cg, policies, "Admins")
				Expect(err).To(MatchError("no BlockValidation policy defined"))
			})
		})
	})

	Describe("AddPolicies", func() {
		var (
			cg       *cb.ConfigGroup
			policies map[string]*genesisconfig.Policy
		)

		BeforeEach(func() {
			cg = protoutil.NewConfigGroup()
			policies = CreateStandardPolicies()
		})

		It("adds the standard policies to the group", func() {
			err := encoder.AddPolicies(cg, policies, "Admins")
			Expect(err).NotTo(HaveOccurred())
			Expect(len(cg.Policies)).To(Equal(3))

			Expect(cg.Policies["Admins"].Policy).To(Equal(&cb.Policy{
				Type: int32(cb.Policy_IMPLICIT_META),
				Value: protoutil.MarshalOrPanic(&cb.ImplicitMetaPolicy{
					SubPolicy: "Admins",
					Rule:      cb.ImplicitMetaPolicy_ANY,
				}),
			}))

			Expect(cg.Policies["Readers"].Policy).To(Equal(&cb.Policy{
				Type: int32(cb.Policy_IMPLICIT_META),
				Value: protoutil.MarshalOrPanic(&cb.ImplicitMetaPolicy{
					SubPolicy: "Readers",
					Rule:      cb.ImplicitMetaPolicy_ANY,
				}),
			}))

			Expect(cg.Policies["Writers"].Policy).To(Equal(&cb.Policy{
				Type: int32(cb.Policy_IMPLICIT_META),
				Value: protoutil.MarshalOrPanic(&cb.ImplicitMetaPolicy{
					SubPolicy: "Writers",
					Rule:      cb.ImplicitMetaPolicy_ANY,
				}),
			}))
		})

		Context("when the policy map is nil", func() {
			BeforeEach(func() {
				policies = nil
			})

			It("returns an error", func() {
				err := encoder.AddPolicies(cg, policies, "Admins")
				Expect(err).To(MatchError("no policies defined"))
			})
		})

		Context("when the policy map is missing 'Admins'", func() {
			BeforeEach(func() {
				delete(policies, "Admins")
			})

			It("returns an error", func() {
				err := encoder.AddPolicies(cg, policies, "Admins")
				Expect(err).To(MatchError("no Admins policy defined"))
			})
		})

		Context("when the policy map is missing 'Readers'", func() {
			BeforeEach(func() {
				delete(policies, "Readers")
			})

			It("returns an error", func() {
				err := encoder.AddPolicies(cg, policies, "Readers")
				Expect(err).To(MatchError("no Readers policy defined"))
			})
		})

		Context("when the policy map is missing 'Writers'", func() {
			BeforeEach(func() {
				delete(policies, "Writers")
			})

			It("returns an error", func() {
				err := encoder.AddPolicies(cg, policies, "Writers")
				Expect(err).To(MatchError("no Writers policy defined"))
			})
		})

		Context("when the signature policy definition is bad", func() {
			BeforeEach(func() {
				policies["Readers"].Type = "Signature"
				policies["Readers"].Rule = "garbage"
			})

			It("wraps and returns the error", func() {
				err := encoder.AddPolicies(cg, policies, "Readers")
				Expect(err).To(MatchError("invalid signature policy rule 'garbage': unrecognized token 'garbage' in policy string"))
			})
		})

		Context("when the implicit policy definition is bad", func() {
			BeforeEach(func() {
				policies["Readers"].Type = "ImplicitMeta"
				policies["Readers"].Rule = "garbage"
			})

			It("wraps and returns the error", func() {
				err := encoder.AddPolicies(cg, policies, "Readers")
				Expect(err).To(MatchError("invalid implicit meta policy rule 'garbage': expected two space separated tokens, but got 1"))
			})
		})

		Context("when the policy type is unknown", func() {
			BeforeEach(func() {
				policies["Readers"].Type = "garbage"
			})

			It("returns an error", func() {
				err := encoder.AddPolicies(cg, policies, "Readers")
				Expect(err).To(MatchError("unknown policy type: garbage"))
			})
		})
	})

	Describe("NewChannelGroup", func() {
		var conf *genesisconfig.Profile

		BeforeEach(func() {
			conf = &genesisconfig.Profile{
				Consortium: "MyConsortium",
				Policies:   CreateStandardPolicies(),
				Application: &genesisconfig.Application{
					Policies: CreateStandardPolicies(),
				},
				Orderer: &genesisconfig.Orderer{
					OrdererType: "solo",
					Policies:    CreateStandardOrdererPolicies(),
					Addresses:   []string{"foo.com:7050", "bar.com:8050"},
				},
				Consortiums: map[string]*genesisconfig.Consortium{
					"SampleConsortium": {},
				},
				Capabilities: map[string]bool{
					"FakeCapability": true,
				},
			}
		})

		It("translates the config into a config group", func() {
			cg, err := encoder.NewChannelGroup(conf)
			Expect(err).NotTo(HaveOccurred())
			Expect(len(cg.Values)).To(Equal(5))
			Expect(cg.Values["BlockDataHashingStructure"]).NotTo(BeNil())
			Expect(cg.Values["Consortium"]).NotTo(BeNil())
			Expect(cg.Values["Capabilities"]).NotTo(BeNil())
			Expect(cg.Values["HashingAlgorithm"]).NotTo(BeNil())
			Expect(cg.Values["OrdererAddresses"]).NotTo(BeNil())
		})

		Context("when the policy definition is bad", func() {
			BeforeEach(func() {
				conf.Policies["Admins"].Rule = "garbage"
			})

			It("wraps and returns the error", func() {
				_, err := encoder.NewChannelGroup(conf)
				Expect(err).To(MatchError("error adding policies to channel group: invalid implicit meta policy rule 'garbage': expected two space separated tokens, but got 1"))
			})
		})

		Context("when the orderer addresses are omitted", func() {
			BeforeEach(func() {
				conf.Orderer.Addresses = []string{}
			})

			It("does not create the config value", func() {
				cg, err := encoder.NewChannelGroup(conf)
				Expect(err).NotTo(HaveOccurred())
				Expect(cg.Values["OrdererAddresses"]).To(BeNil())
			})
		})

		Context("when the orderer config is bad", func() {
			BeforeEach(func() {
				conf.Orderer.OrdererType = "bad-type"
			})

			It("wraps and returns the error", func() {
				_, err := encoder.NewChannelGroup(conf)
				Expect(err).To(MatchError("could not create orderer group: unknown orderer type: bad-type"))
			})

			Context("when the orderer config is missing", func() {
				BeforeEach(func() {
					conf.Orderer = nil
				})

				It("handles it gracefully", func() {
					_, err := encoder.NewChannelGroup(conf)
					Expect(err).NotTo(HaveOccurred())
				})
			})
		})

		Context("when the application config is bad", func() {
			BeforeEach(func() {
				conf.Application.Policies["Admins"] = &genesisconfig.Policy{
					Type: "garbage",
				}
			})

			It("wraps and returns the error", func() {
				_, err := encoder.NewChannelGroup(conf)
				Expect(err).To(MatchError("could not create application group: error adding policies to application group: unknown policy type: garbage"))
			})
		})

		Context("when the consortium config is bad", func() {
			BeforeEach(func() {
				conf.Consortiums["SampleConsortium"].Organizations = []*genesisconfig.Organization{
					{
						Policies: map[string]*genesisconfig.Policy{
							"garbage-policy": {
								Type: "garbage",
							},
						},
					},
				}
			})

			It("wraps and returns the error", func() {
				_, err := encoder.NewChannelGroup(conf)
				Expect(err).To(MatchError("could not create consortiums group: failed to create consortium SampleConsortium: failed to create consortium org: 1 - Error loading MSP configuration for org: : unknown MSP type ''"))
			})
		})
	})

	Describe("NewOrdererGroup", func() {
		var conf *genesisconfig.Orderer

		BeforeEach(func() {
			conf = &genesisconfig.Orderer{
				OrdererType: "solo",
				Organizations: []*genesisconfig.Organization{
					{
						MSPDir:   "../../../sampleconfig/msp",
						ID:       "SampleMSP",
						MSPType:  "bccsp",
						Name:     "SampleOrg",
						Policies: CreateStandardPolicies(),
					},
				},
				Policies: CreateStandardOrdererPolicies(),
				Capabilities: map[string]bool{
					"FakeCapability": true,
				},
			}
		})

		It("translates the config into a config group", func() {
			cg, err := encoder.NewOrdererGroup(conf)
			Expect(err).NotTo(HaveOccurred())
			Expect(len(cg.Policies)).To(Equal(4)) // BlockValidation automatically added
			Expect(cg.Policies["Admins"]).NotTo(BeNil())
			Expect(cg.Policies["Readers"]).NotTo(BeNil())
			Expect(cg.Policies["Writers"]).NotTo(BeNil())
			Expect(cg.Policies["BlockValidation"]).NotTo(BeNil())
			Expect(len(cg.Groups)).To(Equal(1))
			Expect(cg.Groups["SampleOrg"]).NotTo(BeNil())
			Expect(len(cg.Values)).To(Equal(5))
			Expect(cg.Values["BatchSize"]).NotTo(BeNil())
			Expect(cg.Values["BatchTimeout"]).NotTo(BeNil())
			Expect(cg.Values["ChannelRestrictions"]).NotTo(BeNil())
			Expect(cg.Values["Capabilities"]).NotTo(BeNil())
		})

		Context("when the policy definition is bad", func() {
			BeforeEach(func() {
				conf.Policies["Admins"].Rule = "garbage"
			})

			It("wraps and returns the error", func() {
				_, err := encoder.NewOrdererGroup(conf)
				Expect(err).To(MatchError("error adding policies to orderer group: invalid implicit meta policy rule 'garbage': expected two space separated tokens, but got 1"))
			})
		})

		Context("when the consensus type is etcd/raft", func() {
			BeforeEach(func() {
				conf.OrdererType = "etcdraft"
				conf.EtcdRaft = &etcdraft.ConfigMetadata{
					Options: &etcdraft.Options{
						TickInterval: "500ms",
					},
				}
			})

			It("adds the raft metadata", func() {
				cg, err := encoder.NewOrdererGroup(conf)
				Expect(err).NotTo(HaveOccurred())
				Expect(len(cg.Values)).To(Equal(5))
				consensusType := &ab.ConsensusType{}
				err = proto.Unmarshal(cg.Values["ConsensusType"].Value, consensusType)
				Expect(err).NotTo(HaveOccurred())
				Expect(consensusType.Type).To(Equal("etcdraft"))
				metadata := &etcdraft.ConfigMetadata{}
				err = proto.Unmarshal(consensusType.Metadata, metadata)
				Expect(err).NotTo(HaveOccurred())
				Expect(metadata.Options.TickInterval).To(Equal("500ms"))
			})

			Context("when the raft configuration is bad", func() {
				BeforeEach(func() {
					conf.EtcdRaft = &etcdraft.ConfigMetadata{
						Consenters: []*etcdraft.Consenter{
							{},
						},
					}
				})

				It("wraps and returns the error", func() {
					_, err := encoder.NewOrdererGroup(conf)
					Expect(err).To(MatchError("cannot marshal metadata for orderer type etcdraft: cannot load client cert for consenter :0: open : no such file or directory"))
				})
			})
		})

		Context("when the consensus type is BFT", func() {
			BeforeEach(func() {
				conf.OrdererType = "BFT"
				conf.ConsenterMapping = []*genesisconfig.Consenter{
					{
						ID:    1,
						Host:  "host1",
						Port:  1001,
						MSPID: "MSP1",
					},
					{
						ID:            2,
						Host:          "host2",
						Port:          1002,
						MSPID:         "MSP2",
						ClientTLSCert: "../../../sampleconfig/msp/admincerts/admincert.pem",
						ServerTLSCert: "../../../sampleconfig/msp/admincerts/admincert.pem",
						Identity:      "../../../sampleconfig/msp/admincerts/admincert.pem",
					},
				}
				conf.SmartBFT = &smartbft.Options{}
			})

			It("adds the Orderers key", func() {
				cg, err := encoder.NewOrdererGroup(conf)
				Expect(err).NotTo(HaveOccurred())
				Expect(len(cg.Values)).To(Equal(6))
				Expect(cg.Values["Orderers"]).NotTo(BeNil())
				orderersType := &cb.Orderers{}
				err = proto.Unmarshal(cg.Values["Orderers"].Value, orderersType)
				Expect(err).NotTo(HaveOccurred())
				Expect(len(orderersType.ConsenterMapping)).To(Equal(2))
				consenter1 := orderersType.ConsenterMapping[0]
				Expect(consenter1.Id).To(Equal(uint32(1)))
				Expect(consenter1.ClientTlsCert).To(BeNil())
				consenter2 := orderersType.ConsenterMapping[1]
				Expect(consenter2.Id).To(Equal(uint32(2)))
				Expect(consenter2.ClientTlsCert).ToNot(BeNil())
			})
		})

		Context("when the consensus type is unknown", func() {
			BeforeEach(func() {
				conf.OrdererType = "bad-type"
			})

			It("returns an error", func() {
				_, err := encoder.NewOrdererGroup(conf)
				Expect(err).To(MatchError("unknown orderer type: bad-type"))
			})
		})

		Context("when the org definition is bad", func() {
			BeforeEach(func() {
				conf.Organizations[0].MSPType = "garbage"
			})

			It("wraps and returns the error", func() {
				_, err := encoder.NewOrdererGroup(conf)
				Expect(err).To(MatchError("failed to create orderer org: 1 - Error loading MSP configuration for org: SampleOrg: unknown MSP type 'garbage'"))
			})
		})
	})

	Describe("NewApplicationGroup", func() {
		var conf *genesisconfig.Application

		BeforeEach(func() {
			conf = &genesisconfig.Application{
				Organizations: []*genesisconfig.Organization{
					{
						MSPDir:   "../../../sampleconfig/msp",
						ID:       "SampleMSP",
						MSPType:  "bccsp",
						Name:     "SampleOrg",
						Policies: CreateStandardPolicies(),
					},
				},
				ACLs: map[string]string{
					"SomeACL": "SomePolicy",
				},
				Policies: CreateStandardPolicies(),
				Capabilities: map[string]bool{
					"FakeCapability": true,
				},
			}
		})

		It("translates the config into a config group", func() {
			cg, err := encoder.NewApplicationGroup(conf)
			Expect(err).NotTo(HaveOccurred())
			Expect(len(cg.Policies)).To(Equal(3))
			Expect(cg.Policies["Admins"]).NotTo(BeNil())
			Expect(cg.Policies["Readers"]).NotTo(BeNil())
			Expect(cg.Policies["Writers"]).NotTo(BeNil())
			Expect(len(cg.Groups)).To(Equal(1))
			Expect(cg.Groups["SampleOrg"]).NotTo(BeNil())
			Expect(len(cg.Values)).To(Equal(2))
			Expect(cg.Values["ACLs"]).NotTo(BeNil())
			Expect(cg.Values["Capabilities"]).NotTo(BeNil())
		})

		Context("when the policy definition is bad", func() {
			BeforeEach(func() {
				conf.Policies["Admins"].Rule = "garbage"
			})

			It("wraps and returns the error", func() {
				_, err := encoder.NewApplicationGroup(conf)
				Expect(err).To(MatchError("error adding policies to application group: invalid implicit meta policy rule 'garbage': expected two space separated tokens, but got 1"))
			})
		})

		Context("when the org definition is bad", func() {
			BeforeEach(func() {
				conf.Organizations[0].MSPType = "garbage"
			})

			It("wraps and returns the error", func() {
				_, err := encoder.NewApplicationGroup(conf)
				Expect(err).To(MatchError("failed to create application org: 1 - Error loading MSP configuration for org SampleOrg: unknown MSP type 'garbage'"))
			})
		})
	})

	Describe("NewConsortiumOrgGroup", func() {
		var conf *genesisconfig.Organization

		BeforeEach(func() {
			conf = &genesisconfig.Organization{
				MSPDir:   "../../../sampleconfig/msp",
				ID:       "SampleMSP",
				MSPType:  "bccsp",
				Name:     "SampleOrg",
				Policies: CreateStandardPolicies(),
			}
		})

		It("translates the config into a config group", func() {
			cg, err := encoder.NewConsortiumOrgGroup(conf)
			Expect(err).NotTo(HaveOccurred())
			Expect(len(cg.Values)).To(Equal(1))
			Expect(cg.Values["MSP"]).NotTo(BeNil())
			Expect(len(cg.Policies)).To(Equal(3))
			Expect(cg.Policies["Admins"]).NotTo(BeNil())
			Expect(cg.Policies["Readers"]).NotTo(BeNil())
			Expect(cg.Policies["Writers"]).NotTo(BeNil())
		})

		Context("when the org is marked to be skipped as foreign", func() {
			BeforeEach(func() {
				conf.SkipAsForeign = true
			})

			It("returns an empty org group with mod policy set", func() {
				cg, err := encoder.NewConsortiumOrgGroup(conf)
				Expect(err).NotTo(HaveOccurred())
				Expect(len(cg.Values)).To(Equal(0))
				Expect(len(cg.Policies)).To(Equal(0))
			})

			Context("even when the MSP dir is invalid/corrupt", func() {
				BeforeEach(func() {
					conf.MSPDir = "garbage"
				})

				It("returns without error", func() {
					_, err := encoder.NewConsortiumOrgGroup(conf)
					Expect(err).NotTo(HaveOccurred())
				})
			})
		})

		Context("when dev mode is enabled", func() {
			BeforeEach(func() {
				conf.AdminPrincipal = "Member"
			})

			It("does not produce an error", func() {
				_, err := encoder.NewConsortiumOrgGroup(conf)
				Expect(err).NotTo(HaveOccurred())
			})
		})

		Context("when the policy definition is bad", func() {
			BeforeEach(func() {
				conf.Policies["Admins"].Rule = "garbage"
			})

			It("wraps and returns the error", func() {
				_, err := encoder.NewConsortiumOrgGroup(conf)
				Expect(err).To(MatchError("error adding policies to consortiums org group 'SampleOrg': invalid implicit meta policy rule 'garbage': expected two space separated tokens, but got 1"))
			})
		})
	})

	Describe("NewOrdererOrgGroup", func() {
		var conf *genesisconfig.Organization

		BeforeEach(func() {
			conf = &genesisconfig.Organization{
				MSPDir:   "../../../sampleconfig/msp",
				ID:       "SampleMSP",
				MSPType:  "bccsp",
				Name:     "SampleOrg",
				Policies: CreateStandardPolicies(),
				OrdererEndpoints: []string{
					"foo:7050",
					"bar:8050",
				},
			}
		})

		It("translates the config into a config group", func() {
			cg, err := encoder.NewOrdererOrgGroup(conf)
			Expect(err).NotTo(HaveOccurred())
			Expect(len(cg.Values)).To(Equal(2))
			Expect(cg.Values["MSP"]).NotTo(BeNil())
			Expect(len(cg.Policies)).To(Equal(3))
			Expect(cg.Values["Endpoints"]).NotTo(BeNil())
			Expect(cg.Policies["Admins"]).NotTo(BeNil())
			Expect(cg.Policies["Readers"]).NotTo(BeNil())
			Expect(cg.Policies["Writers"]).NotTo(BeNil())
		})

		Context("when the org is marked to be skipped as foreign", func() {
			BeforeEach(func() {
				conf.SkipAsForeign = true
			})

			It("returns an empty org group with mod policy set", func() {
				cg, err := encoder.NewOrdererOrgGroup(conf)
				Expect(err).NotTo(HaveOccurred())
				Expect(len(cg.Values)).To(Equal(0))
				Expect(len(cg.Policies)).To(Equal(0))
			})

			Context("even when the MSP dir is invalid/corrupt", func() {
				BeforeEach(func() {
					conf.MSPDir = "garbage"
				})

				It("returns without error", func() {
					_, err := encoder.NewOrdererOrgGroup(conf)
					Expect(err).NotTo(HaveOccurred())
				})
			})
		})

		Context("when there are no ordering endpoints", func() {
			BeforeEach(func() {
				conf.OrdererEndpoints = []string{}
			})

			It("does not include the endpoints in the config group", func() {
				cg, err := encoder.NewOrdererOrgGroup(conf)
				Expect(err).NotTo(HaveOccurred())
				Expect(cg.Values["Endpoints"]).To(BeNil())
			})
		})

		Context("when dev mode is enabled", func() {
			BeforeEach(func() {
				conf.AdminPrincipal = "Member"
			})

			It("does not produce an error", func() {
				_, err := encoder.NewOrdererOrgGroup(conf)
				Expect(err).NotTo(HaveOccurred())
			})
		})

		Context("when the policy definition is bad", func() {
			BeforeEach(func() {
				conf.Policies["Admins"].Rule = "garbage"
			})

			It("wraps and returns the error", func() {
				_, err := encoder.NewOrdererOrgGroup(conf)
				Expect(err).To(MatchError("error adding policies to orderer org group 'SampleOrg': invalid implicit meta policy rule 'garbage': expected two space separated tokens, but got 1"))
			})
		})
	})

	Describe("NewApplicationOrgGroup", func() {
		var conf *genesisconfig.Organization

		BeforeEach(func() {
			conf = &genesisconfig.Organization{
				MSPDir:   "../../../sampleconfig/msp",
				ID:       "SampleMSP",
				MSPType:  "bccsp",
				Name:     "SampleOrg",
				Policies: CreateStandardPolicies(),
				AnchorPeers: []*genesisconfig.AnchorPeer{
					{
						Host: "hostname",
						Port: 5555,
					},
				},
			}
		})

		It("translates the config into a config group", func() {
			cg, err := encoder.NewApplicationOrgGroup(conf)
			Expect(err).NotTo(HaveOccurred())
			Expect(len(cg.Values)).To(Equal(2))
			Expect(cg.Values["MSP"]).NotTo(BeNil())
			Expect(cg.Values["AnchorPeers"]).NotTo(BeNil())
			Expect(len(cg.Policies)).To(Equal(3))
			Expect(cg.Policies["Admins"]).NotTo(BeNil())
			Expect(cg.Policies["Readers"]).NotTo(BeNil())
			Expect(cg.Policies["Writers"]).NotTo(BeNil())
			Expect(len(cg.Values)).To(Equal(2))
			Expect(cg.Values["MSP"]).NotTo(BeNil())
			Expect(cg.Values["AnchorPeers"]).NotTo(BeNil())
		})

		Context("when the org is marked to be skipped as foreign", func() {
			BeforeEach(func() {
				conf.SkipAsForeign = true
			})

			It("returns an empty org group with mod policy set", func() {
				cg, err := encoder.NewApplicationOrgGroup(conf)
				Expect(err).NotTo(HaveOccurred())
				Expect(len(cg.Values)).To(Equal(0))
				Expect(len(cg.Policies)).To(Equal(0))
			})

			Context("even when the MSP dir is invalid/corrupt", func() {
				BeforeEach(func() {
					conf.MSPDir = "garbage"
				})

				It("returns without error", func() {
					_, err := encoder.NewApplicationOrgGroup(conf)
					Expect(err).NotTo(HaveOccurred())
				})
			})
		})

		Context("when the policy definition is bad", func() {
			BeforeEach(func() {
				conf.Policies["Admins"].Type = "garbage"
			})

			It("wraps and returns the error", func() {
				_, err := encoder.NewApplicationOrgGroup(conf)
				Expect(err).To(MatchError("error adding policies to application org group SampleOrg: unknown policy type: garbage"))
			})
		})

		Context("when the MSP definition is bad", func() {
			BeforeEach(func() {
				conf.MSPDir = "garbage"
			})

			It("wraps and returns the error", func() {
				_, err := encoder.NewApplicationOrgGroup(conf)
				Expect(err).To(MatchError("1 - Error loading MSP configuration for org SampleOrg: could not load a valid ca certificate from directory garbage/cacerts: stat garbage/cacerts: no such file or directory"))
			})
		})

		Context("when there are no anchor peers defined", func() {
			BeforeEach(func() {
				conf.AnchorPeers = nil
			})

			It("does not encode the anchor peers", func() {
				cg, err := encoder.NewApplicationOrgGroup(conf)
				Expect(err).NotTo(HaveOccurred())
				Expect(len(cg.Values)).To(Equal(1))
				Expect(cg.Values["AnchorPeers"]).To(BeNil())
			})
		})
	})

	Describe("ChannelCreationOperations", func() {
		var (
			conf     *genesisconfig.Profile
			template *cb.ConfigGroup
		)

		BeforeEach(func() {
			conf = &genesisconfig.Profile{
				Consortium: "MyConsortium",
				Policies:   CreateStandardPolicies(),
				Application: &genesisconfig.Application{
					Organizations: []*genesisconfig.Organization{
						{
							Name:     "SampleOrg",
							MSPDir:   "../../../sampleconfig/msp",
							ID:       "SampleMSP",
							MSPType:  "bccsp",
							Policies: CreateStandardPolicies(),
							AnchorPeers: []*genesisconfig.AnchorPeer{
								{
									Host: "hostname",
									Port: 4444,
								},
							},
						},
					},
					Policies: CreateStandardPolicies(),
				},
			}

			var err error
			template, err = encoder.DefaultConfigTemplate(conf)
			Expect(err).NotTo(HaveOccurred())
		})

		Describe("NewChannelCreateConfigUpdate", func() {
			It("translates the config into a config group", func() {
				cg, err := encoder.NewChannelCreateConfigUpdate("channel-id", conf, template)
				Expect(err).NotTo(HaveOccurred())
				expected := &cb.ConfigUpdate{
					ChannelId: "channel-id",
					ReadSet: &cb.ConfigGroup{
						Groups: map[string]*cb.ConfigGroup{
							"Application": {
								Groups: map[string]*cb.ConfigGroup{
									"SampleOrg": {},
								},
							},
						},
						Values: map[string]*cb.ConfigValue{
							"Consortium": {},
						},
					},
					WriteSet: &cb.ConfigGroup{
						Groups: map[string]*cb.ConfigGroup{
							"Application": {
								Version:   1,
								ModPolicy: "Admins",
								Groups: map[string]*cb.ConfigGroup{
									"SampleOrg": {},
								},
								Policies: map[string]*cb.ConfigPolicy{
									"Admins": {
										Policy: &cb.Policy{
											Type: int32(cb.Policy_IMPLICIT_META),
											Value: protoutil.MarshalOrPanic(&cb.ImplicitMetaPolicy{
												SubPolicy: "Admins",
												Rule:      cb.ImplicitMetaPolicy_ANY,
											}),
										},
										ModPolicy: "Admins",
									},
									"Readers": {
										Policy: &cb.Policy{
											Type: int32(cb.Policy_IMPLICIT_META),
											Value: protoutil.MarshalOrPanic(&cb.ImplicitMetaPolicy{
												SubPolicy: "Readers",
												Rule:      cb.ImplicitMetaPolicy_ANY,
											}),
										},
										ModPolicy: "Admins",
									},
									"Writers": {
										Policy: &cb.Policy{
											Type: int32(cb.Policy_IMPLICIT_META),
											Value: protoutil.MarshalOrPanic(&cb.ImplicitMetaPolicy{
												SubPolicy: "Writers",
												Rule:      cb.ImplicitMetaPolicy_ANY,
											}),
										},
										ModPolicy: "Admins",
									},
								},
							},
						},
						Values: map[string]*cb.ConfigValue{
							"Consortium": {
								Value: protoutil.MarshalOrPanic(&cb.Consortium{
									Name: "MyConsortium",
								}),
							},
						},
					},
				}
				Expect(proto.Equal(expected, cg)).To(BeTrue())
			})

			Context("when the template configuration is not the default", func() {
				BeforeEach(func() {
					differentConf := &genesisconfig.Profile{
						Consortium: "MyConsortium",
						Policies:   CreateStandardPolicies(),
						Application: &genesisconfig.Application{
							Organizations: []*genesisconfig.Organization{
								{
									MSPDir:  "../../../sampleconfig/msp",
									ID:      "SampleMSP",
									MSPType: "bccsp",
									Name:    "SampleOrg",
									AnchorPeers: []*genesisconfig.AnchorPeer{
										{
											Host: "hostname",
											Port: 5555,
										},
									},
									Policies: CreateStandardPolicies(),
								},
							},
							Policies: CreateStandardPolicies(),
						},
					}

					var err error
					template, err = encoder.DefaultConfigTemplate(differentConf)
					Expect(err).NotTo(HaveOccurred())
				})

				It("reflects the additional modifications designated by the channel creation profile", func() {
					cg, err := encoder.NewChannelCreateConfigUpdate("channel-id", conf, template)
					Expect(err).NotTo(HaveOccurred())
					Expect(cg.WriteSet.Groups["Application"].Groups["SampleOrg"].Values["AnchorPeers"].Version).To(Equal(uint64(1)))
				})
			})

			Context("when the application config is bad", func() {
				BeforeEach(func() {
					conf.Application.Policies["Admins"].Type = "bad-type"
				})

				It("returns an error", func() {
					_, err := encoder.NewChannelCreateConfigUpdate("channel-id", conf, template)
					Expect(err).To(MatchError("could not turn parse profile into channel group: could not create application group: error adding policies to application group: unknown policy type: bad-type"))
				})

				Context("when the application config is missing", func() {
					BeforeEach(func() {
						conf.Application = nil
					})

					It("returns an error", func() {
						_, err := encoder.NewChannelCreateConfigUpdate("channel-id", conf, template)
						Expect(err).To(MatchError("cannot define a new channel with no Application section"))
					})
				})
			})

			Context("when the consortium is empty", func() {
				BeforeEach(func() {
					conf.Consortium = ""
				})

				It("returns an error", func() {
					_, err := encoder.NewChannelCreateConfigUpdate("channel-id", conf, template)
					Expect(err).To(MatchError("cannot define a new channel with no Consortium value"))
				})
			})

			Context("when an update cannot be computed", func() {
				It("returns an error", func() {
					_, err := encoder.NewChannelCreateConfigUpdate("channel-id", conf, nil)
					Expect(err).To(MatchError("could not compute update: no channel group included for original config"))
				})
			})
		})

		Describe("MakeChannelCreationTransaction", func() {
			var fakeSigner *fakes.SignerSerializer

			BeforeEach(func() {
				fakeSigner = &fakes.SignerSerializer{}
				fakeSigner.SerializeReturns([]byte("fake-creator"), nil)
			})

			It("returns an encoded and signed tx", func() {
				env, err := encoder.MakeChannelCreationTransaction("channel-id", fakeSigner, conf)
				Expect(err).NotTo(HaveOccurred())
				payload := &cb.Payload{}
				err = proto.Unmarshal(env.Payload, payload)
				Expect(err).NotTo(HaveOccurred())
				configUpdateEnv := &cb.ConfigUpdateEnvelope{}
				err = proto.Unmarshal(payload.Data, configUpdateEnv)
				Expect(err).NotTo(HaveOccurred())
				Expect(len(configUpdateEnv.Signatures)).To(Equal(1))
				Expect(fakeSigner.SerializeCallCount()).To(Equal(2))
				Expect(fakeSigner.SignCallCount()).To(Equal(2))
				Expect(fakeSigner.SignArgsForCall(0)).To(Equal(util.ConcatenateBytes(configUpdateEnv.Signatures[0].SignatureHeader, configUpdateEnv.ConfigUpdate)))
			})

			Context("when a default config cannot be generated", func() {
				BeforeEach(func() {
					conf.Application = nil
				})

				It("wraps and returns the error", func() {
					_, err := encoder.MakeChannelCreationTransaction("channel-id", fakeSigner, conf)
					Expect(err).To(MatchError("could not generate default config template: channel template configs must contain an application section"))
				})
			})

			Context("when the signer cannot create the signature header", func() {
				BeforeEach(func() {
					fakeSigner.SerializeReturns(nil, fmt.Errorf("serialize-error"))
				})

				It("wraps and returns the error", func() {
					_, err := encoder.MakeChannelCreationTransaction("channel-id", fakeSigner, conf)
					Expect(err).To(MatchError("creating signature header failed: serialize-error"))
				})
			})

			Context("when the signer cannot sign", func() {
				BeforeEach(func() {
					fakeSigner.SignReturns(nil, fmt.Errorf("sign-error"))
				})

				It("wraps and returns the error", func() {
					_, err := encoder.MakeChannelCreationTransaction("channel-id", fakeSigner, conf)
					Expect(err).To(MatchError("signature failure over config update: sign-error"))
				})
			})

			Context("when no signer is provided", func() {
				It("returns an encoded tx with no signature", func() {
					_, err := encoder.MakeChannelCreationTransaction("channel-id", nil, conf)
					Expect(err).NotTo(HaveOccurred())
				})
			})

			Context("when the config is bad", func() {
				BeforeEach(func() {
					conf.Consortium = ""
				})

				It("wraps and returns the error", func() {
					_, err := encoder.MakeChannelCreationTransaction("channel-id", nil, conf)
					Expect(err).To(MatchError("config update generation failure: cannot define a new channel with no Consortium value"))
				})
			})
		})

		Describe("MakeChannelCreationTransactionWithSystemChannelContext", func() {
			var (
				applicationConf *genesisconfig.Profile
				sysChannelConf  *genesisconfig.Profile
			)

			BeforeEach(func() {
				applicationConf = &genesisconfig.Profile{
					Consortium: "SampleConsortium",
					Policies:   CreateStandardPolicies(),
					Orderer: &genesisconfig.Orderer{
						OrdererType: "solo",
						Policies:    CreateStandardOrdererPolicies(),
					},
					Application: &genesisconfig.Application{
						Organizations: []*genesisconfig.Organization{
							{
								MSPDir:  "../../../sampleconfig/msp",
								ID:      "Org1MSP",
								MSPType: "bccsp",
								Name:    "Org1",
								AnchorPeers: []*genesisconfig.AnchorPeer{
									{
										Host: "my-peer",
										Port: 5555,
									},
								},
								Policies: CreateStandardPolicies(),
							},
							{
								MSPDir:   "../../../sampleconfig/msp",
								ID:       "Org2MSP",
								MSPType:  "bccsp",
								Name:     "Org2",
								Policies: CreateStandardPolicies(),
							},
						},
						Policies: CreateStandardPolicies(),
					},
				}

				sysChannelConf = &genesisconfig.Profile{
					Policies: CreateStandardPolicies(),
					Orderer: &genesisconfig.Orderer{
						OrdererType:  "solo",
						BatchTimeout: time.Hour,
						Policies:     CreateStandardOrdererPolicies(),
					},
					Consortiums: map[string]*genesisconfig.Consortium{
						"SampleConsortium": {
							Organizations: []*genesisconfig.Organization{
								{
									MSPDir:   "../../../sampleconfig/msp",
									ID:       "Org1MSP",
									MSPType:  "bccsp",
									Name:     "Org1",
									Policies: CreateStandardPolicies(),
								},
								{
									MSPDir:   "../../../sampleconfig/msp",
									ID:       "Org2MSP",
									MSPType:  "bccsp",
									Name:     "Org2",
									Policies: CreateStandardPolicies(),
								},
							},
						},
					},
				}
			})

			It("returns an encoded and signed tx including differences from the system channel", func() {
				env, err := encoder.MakeChannelCreationTransactionWithSystemChannelContext("channel-id", nil, applicationConf, sysChannelConf)
				Expect(err).NotTo(HaveOccurred())
				payload := &cb.Payload{}
				err = proto.Unmarshal(env.Payload, payload)
				Expect(err).NotTo(HaveOccurred())
				configUpdateEnv := &cb.ConfigUpdateEnvelope{}
				err = proto.Unmarshal(payload.Data, configUpdateEnv)
				Expect(err).NotTo(HaveOccurred())
				configUpdate := &cb.ConfigUpdate{}
				err = proto.Unmarshal(configUpdateEnv.ConfigUpdate, configUpdate)
				Expect(err).NotTo(HaveOccurred())
				Expect(configUpdate.WriteSet.Version).To(Equal(uint64(0)))
				Expect(configUpdate.WriteSet.Groups["Application"].Policies["Admins"].Version).To(Equal(uint64(1)))
				Expect(configUpdate.WriteSet.Groups["Application"].Groups["Org1"].Version).To(Equal(uint64(1)))
				Expect(configUpdate.WriteSet.Groups["Application"].Groups["Org1"].Values["AnchorPeers"]).NotTo(BeNil())
				Expect(configUpdate.WriteSet.Groups["Application"].Groups["Org2"].Version).To(Equal(uint64(0)))
				Expect(configUpdate.WriteSet.Groups["Orderer"].Values["BatchTimeout"].Version).To(Equal(uint64(1)))
			})

			Context("when the system channel config is bad", func() {
				BeforeEach(func() {
					sysChannelConf.Orderer.OrdererType = "garbage"
				})

				It("wraps and returns the error", func() {
					_, err := encoder.MakeChannelCreationTransactionWithSystemChannelContext("channel-id", nil, applicationConf, sysChannelConf)
					Expect(err).To(MatchError("could not parse system channel config: could not create orderer group: unknown orderer type: garbage"))
				})
			})

			Context("when the template cannot be computed", func() {
				BeforeEach(func() {
					applicationConf.Application = nil
				})

				It("wraps and returns the error", func() {
					_, err := encoder.MakeChannelCreationTransactionWithSystemChannelContext("channel-id", nil, applicationConf, sysChannelConf)
					Expect(err).To(MatchError("could not create config template: supplied channel creation profile does not contain an application section"))
				})
			})
		})

		Describe("DefaultConfigTemplate", func() {
			var conf *genesisconfig.Profile

			BeforeEach(func() {
				conf = &genesisconfig.Profile{
					Policies: CreateStandardPolicies(),
					Orderer: &genesisconfig.Orderer{
						OrdererType: "solo",
						Policies:    CreateStandardOrdererPolicies(),
					},
					Application: &genesisconfig.Application{
						Policies: CreateStandardPolicies(),
						Organizations: []*genesisconfig.Organization{
							{
								Name:          "Org1",
								SkipAsForeign: true,
							},
							{
								Name:          "Org2",
								SkipAsForeign: true,
							},
						},
					},
				}
			})

			It("returns the default config template", func() {
				cg, err := encoder.DefaultConfigTemplate(conf)
				Expect(err).NotTo(HaveOccurred())
				Expect(len(cg.Groups)).To(Equal(2))
				Expect(cg.Groups["Orderer"]).NotTo(BeNil())
				Expect(cg.Groups["Application"]).NotTo(BeNil())
				Expect(cg.Groups["Application"].Policies).To(BeEmpty())
				Expect(cg.Groups["Application"].Values).To(BeEmpty())
				Expect(len(cg.Groups["Application"].Groups)).To(Equal(2))
			})

			Context("when the config cannot be turned into a channel group", func() {
				BeforeEach(func() {
					conf.Orderer.OrdererType = "garbage"
				})

				It("wraps and returns the error", func() {
					_, err := encoder.DefaultConfigTemplate(conf)
					Expect(err).To(MatchError("error parsing configuration: could not create orderer group: unknown orderer type: garbage"))
				})
			})

			Context("when the application config is nil", func() {
				BeforeEach(func() {
					conf.Application = nil
				})

				It("returns an error", func() {
					_, err := encoder.DefaultConfigTemplate(conf)
					Expect(err).To(MatchError("channel template configs must contain an application section"))
				})
			})
		})

		Describe("ConfigTemplateFromGroup", func() {
			var (
				applicationConf *genesisconfig.Profile
				sysChannelGroup *cb.ConfigGroup
			)

			BeforeEach(func() {
				applicationConf = &genesisconfig.Profile{
					Policies:   CreateStandardPolicies(),
					Consortium: "SampleConsortium",
					Orderer: &genesisconfig.Orderer{
						OrdererType: "solo",
						Policies:    CreateStandardOrdererPolicies(),
					},
					Application: &genesisconfig.Application{
						Organizations: []*genesisconfig.Organization{
							{
								Name:          "Org1",
								SkipAsForeign: true,
							},
							{
								Name:          "Org2",
								SkipAsForeign: true,
							},
						},
					},
				}

				var err error
				sysChannelGroup, err = encoder.NewChannelGroup(&genesisconfig.Profile{
					Policies: CreateStandardPolicies(),
					Orderer: &genesisconfig.Orderer{
						OrdererType: "solo",
						Policies:    CreateStandardOrdererPolicies(),
					},
					Consortiums: map[string]*genesisconfig.Consortium{
						"SampleConsortium": {
							Organizations: []*genesisconfig.Organization{
								{
									Name:          "Org1",
									SkipAsForeign: true,
								},
								{
									Name:          "Org2",
									SkipAsForeign: true,
								},
							},
						},
					},
				})
				Expect(err).NotTo(HaveOccurred())
			})

			It("returns a config template", func() {
				cg, err := encoder.ConfigTemplateFromGroup(applicationConf, sysChannelGroup)
				Expect(err).NotTo(HaveOccurred())
				Expect(len(cg.Groups)).To(Equal(2))
				Expect(cg.Groups["Orderer"]).NotTo(BeNil())
				Expect(proto.Equal(cg.Groups["Orderer"], sysChannelGroup.Groups["Orderer"])).To(BeTrue())
				Expect(cg.Groups["Application"]).NotTo(BeNil())
				Expect(len(cg.Groups["Application"].Policies)).To(Equal(1))
				Expect(cg.Groups["Application"].Policies["Admins"]).NotTo(BeNil())
				Expect(cg.Groups["Application"].Values).To(BeEmpty())
				Expect(len(cg.Groups["Application"].Groups)).To(Equal(2))
			})

			Context("when the orderer system channel group has no sub-groups", func() {
				BeforeEach(func() {
					sysChannelGroup.Groups = nil
				})

				It("returns an error", func() {
					_, err := encoder.ConfigTemplateFromGroup(applicationConf, sysChannelGroup)
					Expect(err).To(MatchError("supplied system channel group has no sub-groups"))
				})
			})

			Context("when the orderer system channel group has no consortiums group", func() {
				BeforeEach(func() {
					delete(sysChannelGroup.Groups, "Consortiums")
				})

				It("returns an error", func() {
					_, err := encoder.ConfigTemplateFromGroup(applicationConf, sysChannelGroup)
					Expect(err).To(MatchError("supplied system channel group does not appear to be system channel (missing consortiums group)"))
				})
			})

			Context("when the orderer system channel group has no consortiums in the consortiums group", func() {
				BeforeEach(func() {
					sysChannelGroup.Groups["Consortiums"].Groups = nil
				})

				It("returns an error", func() {
					_, err := encoder.ConfigTemplateFromGroup(applicationConf, sysChannelGroup)
					Expect(err).To(MatchError("system channel consortiums group appears to have no consortiums defined"))
				})
			})

			Context("when the orderer system channel group does not have the requested consortium", func() {
				BeforeEach(func() {
					applicationConf.Consortium = "bad-consortium"
				})

				It("returns an error", func() {
					_, err := encoder.ConfigTemplateFromGroup(applicationConf, sysChannelGroup)
					Expect(err).To(MatchError("supplied system channel group is missing 'bad-consortium' consortium"))
				})
			})

			Context("when the channel creation profile has no application section", func() {
				BeforeEach(func() {
					applicationConf.Application = nil
				})

				It("returns an error", func() {
					_, err := encoder.ConfigTemplateFromGroup(applicationConf, sysChannelGroup)
					Expect(err).To(MatchError("supplied channel creation profile does not contain an application section"))
				})
			})

			Context("when the orderer system channel group does not have all the channel creation orgs", func() {
				BeforeEach(func() {
					delete(sysChannelGroup.Groups["Consortiums"].Groups["SampleConsortium"].Groups, "Org1")
				})

				It("returns an error", func() {
					_, err := encoder.ConfigTemplateFromGroup(applicationConf, sysChannelGroup)
					Expect(err).To(MatchError("consortium SampleConsortium does not contain member org Org1"))
				})
			})
		})

		Describe("HasSkippedForeignOrgs", func() {
			var conf *genesisconfig.Profile

			BeforeEach(func() {
				conf = &genesisconfig.Profile{
					Orderer: &genesisconfig.Orderer{
						Organizations: []*genesisconfig.Organization{
							{
								Name: "OrdererOrg1",
							},
							{
								Name: "OrdererOrg2",
							},
						},
					},
					Application: &genesisconfig.Application{
						Organizations: []*genesisconfig.Organization{
							{
								Name: "ApplicationOrg1",
							},
							{
								Name: "ApplicationOrg2",
							},
						},
					},
					Consortiums: map[string]*genesisconfig.Consortium{
						"SomeConsortium": {
							Organizations: []*genesisconfig.Organization{
								{
									Name: "ConsortiumOrg1",
								},
								{
									Name: "ConsortiumOrg2",
								},
							},
						},
					},
				}
			})

			It("returns no error if all orgs are not skipped as foreign", func() {
				err := encoder.HasSkippedForeignOrgs(conf)
				Expect(err).NotTo(HaveOccurred())
			})

			Context("when the orderer group has foreign orgs", func() {
				BeforeEach(func() {
					conf.Orderer.Organizations[1].SkipAsForeign = true
				})

				It("returns an error indicating the offending org", func() {
					err := encoder.HasSkippedForeignOrgs(conf)
					Expect(err).To(MatchError("organization 'OrdererOrg2' is marked to be skipped as foreign"))
				})
			})

			Context("when the application group has foreign orgs", func() {
				BeforeEach(func() {
					conf.Application.Organizations[1].SkipAsForeign = true
				})

				It("returns an error indicating the offending org", func() {
					err := encoder.HasSkippedForeignOrgs(conf)
					Expect(err).To(MatchError("organization 'ApplicationOrg2' is marked to be skipped as foreign"))
				})
			})

			Context("when the consortium group has foreign orgs", func() {
				BeforeEach(func() {
					conf.Consortiums["SomeConsortium"].Organizations[1].SkipAsForeign = true
				})

				It("returns an error indicating the offending org", func() {
					err := encoder.HasSkippedForeignOrgs(conf)
					Expect(err).To(MatchError("organization 'ConsortiumOrg2' is marked to be skipped as foreign"))
				})
			})
		})
	})

	Describe("Bootstrapper", func() {
		Describe("NewBootstrapper", func() {
			var conf *genesisconfig.Profile

			BeforeEach(func() {
				conf = &genesisconfig.Profile{
					Policies: CreateStandardPolicies(),
					Orderer: &genesisconfig.Orderer{
						OrdererType: "solo",
						Policies:    CreateStandardOrdererPolicies(),
					},
				}
			})

			It("creates a new bootstrapper for the given config", func() {
				bs, err := encoder.NewBootstrapper(conf)
				Expect(err).NotTo(HaveOccurred())
				Expect(bs).NotTo(BeNil())
			})

			Context("when the channel config is bad", func() {
				BeforeEach(func() {
					conf.Orderer.OrdererType = "bad-type"
				})

				It("wraps and returns the error", func() {
					_, err := encoder.NewBootstrapper(conf)
					Expect(err).To(MatchError("could not create channel group: could not create orderer group: unknown orderer type: bad-type"))
				})
			})

			Context("when the channel config contains a foreign org", func() {
				BeforeEach(func() {
					conf.Orderer.Organizations = []*genesisconfig.Organization{
						{
							Name:          "MyOrg",
							SkipAsForeign: true,
						},
					}
				})

				It("wraps and returns the error", func() {
					_, err := encoder.NewBootstrapper(conf)
					Expect(err).To(MatchError("all org definitions must be local during bootstrapping: organization 'MyOrg' is marked to be skipped as foreign"))
				})
			})
		})

		Describe("New", func() {
			var conf *genesisconfig.Profile

			BeforeEach(func() {
				conf = &genesisconfig.Profile{
					Policies: CreateStandardPolicies(),
					Orderer: &genesisconfig.Orderer{
						OrdererType: "solo",
						Policies:    CreateStandardOrdererPolicies(),
					},
				}
			})

			It("creates a new bootstrapper for the given config", func() {
				bs := encoder.New(conf)
				Expect(bs).NotTo(BeNil())
			})

			Context("when the channel config is bad", func() {
				BeforeEach(func() {
					conf.Orderer.OrdererType = "bad-type"
				})

				It("panics", func() {
					Expect(func() { encoder.New(conf) }).To(Panic())
				})
			})
		})

		Describe("Functions", func() {
			var bs *encoder.Bootstrapper

			BeforeEach(func() {
				bs = encoder.New(&genesisconfig.Profile{
					Policies: CreateStandardPolicies(),
					Orderer: &genesisconfig.Orderer{
						Policies:    CreateStandardOrdererPolicies(),
						OrdererType: "solo",
					},
				})
			})

			Describe("GenesisBlock", func() {
				It("produces a new genesis block with a default channel ID", func() {
					block := bs.GenesisBlock()
					Expect(block).NotTo(BeNil())
				})
			})

			Describe("GenesisBlockForChannel", func() {
				It("produces a new genesis block with a default channel ID", func() {
					block := bs.GenesisBlockForChannel("channel-id")
					Expect(block).NotTo(BeNil())
				})
			})
		})
	})
})
