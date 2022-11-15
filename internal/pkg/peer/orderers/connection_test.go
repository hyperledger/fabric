/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package orderers_test

import (
	"bytes"
	"io/ioutil"
	"sort"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/hyperledger/fabric/common/flogging"
	"github.com/hyperledger/fabric/internal/pkg/peer/orderers"
)

type comparableEndpoint struct {
	Address   string
	RootCerts [][]byte
}

// stripEndpoints makes a comparable version of the endpoints specified.  This
// is necessary because the endpoint contains a channel which is not
// comparable.
func stripEndpoints(endpoints []*orderers.Endpoint) []comparableEndpoint {
	endpointsWithChannelStripped := make([]comparableEndpoint, len(endpoints))
	for i, endpoint := range endpoints {
		certs := endpoint.RootCerts
		sort.Slice(certs, func(i, j int) bool {
			return bytes.Compare(certs[i], certs[j]) >= 0
		})
		endpointsWithChannelStripped[i].Address = endpoint.Address
		endpointsWithChannelStripped[i].RootCerts = certs
	}
	return endpointsWithChannelStripped
}

var _ = Describe("Connection", func() {
	var (
		cert1 []byte
		cert2 []byte
		cert3 []byte

		org1 orderers.OrdererOrg
		org2 orderers.OrdererOrg

		org1Certs     [][]byte
		org2Certs     [][]byte
		overrideCerts [][]byte

		cs *orderers.ConnectionSource

		endpoints []*orderers.Endpoint
	)

	BeforeEach(func() {
		var err error
		cert1, err = ioutil.ReadFile("testdata/tlsca.example.com-cert.pem")
		Expect(err).NotTo(HaveOccurred())

		cert2, err = ioutil.ReadFile("testdata/tlsca.org1.example.com-cert.pem")
		Expect(err).NotTo(HaveOccurred())

		cert3, err = ioutil.ReadFile("testdata/tlsca.org2.example.com-cert.pem")
		Expect(err).NotTo(HaveOccurred())

		org1 = orderers.OrdererOrg{
			Addresses: []string{"org1-address1", "org1-address2"},
			RootCerts: [][]byte{cert1, cert2},
		}

		org2 = orderers.OrdererOrg{
			Addresses: []string{"org2-address1", "org2-address2"},
			RootCerts: [][]byte{cert3},
		}

		org1Certs = [][]byte{cert1, cert2}
		org2Certs = [][]byte{cert3}
		overrideCerts = [][]byte{cert2}

		cs = orderers.NewConnectionSource(flogging.MustGetLogger("peer.orderers"), map[string]*orderers.Endpoint{
			"override-address": {
				Address:   "re-mapped-address",
				RootCerts: overrideCerts,
			},
		})
		cs.Update(nil, map[string]orderers.OrdererOrg{
			"org1": org1,
			"org2": org2,
		})

		endpoints = cs.Endpoints()
	})

	It("holds endpoints for all of the defined orderers", func() {
		Expect(stripEndpoints(endpoints)).To(ConsistOf(
			stripEndpoints([]*orderers.Endpoint{
				{
					Address:   "org1-address1",
					RootCerts: org1Certs,
				},
				{
					Address:   "org1-address2",
					RootCerts: org1Certs,
				},
				{
					Address:   "org2-address1",
					RootCerts: org2Certs,
				},
				{
					Address:   "org2-address2",
					RootCerts: org2Certs,
				},
			}),
		))
	})

	It("does not mark any of the endpoints as refreshed", func() {
		for _, endpoint := range endpoints {
			Expect(endpoint.Refreshed).NotTo(BeClosed())
		}
	})

	When("an update does not modify the endpoint set", func() {
		BeforeEach(func() {
			cs.Update(nil, map[string]orderers.OrdererOrg{
				"org1": org1,
				"org2": org2,
			})
		})

		It("does not update the endpoints", func() {
			newEndpoints := cs.Endpoints()
			Expect(newEndpoints).To(Equal(endpoints))
		})

		It("does not close any of the refresh channels", func() {
			for _, endpoint := range endpoints {
				Expect(endpoint.Refreshed).NotTo(BeClosed())
			}
		})
	})

	When("an update change's an org's TLS CA", func() {
		BeforeEach(func() {
			org1.RootCerts = [][]byte{cert1}

			cs.Update(nil, map[string]orderers.OrdererOrg{
				"org1": org1,
				"org2": org2,
			})
		})

		It("creates a new set of orderer endpoints", func() {
			newOrg1Certs := [][]byte{cert1}

			newEndpoints := cs.Endpoints()
			Expect(stripEndpoints(newEndpoints)).To(ConsistOf(
				stripEndpoints([]*orderers.Endpoint{
					{
						Address:   "org1-address1",
						RootCerts: newOrg1Certs,
					},
					{
						Address:   "org1-address2",
						RootCerts: newOrg1Certs,
					},
					{
						Address:   "org2-address1",
						RootCerts: org2Certs,
					},
					{
						Address:   "org2-address2",
						RootCerts: org2Certs,
					},
				}),
			))
		})

		It("closes the refresh channel for all of the old endpoints", func() {
			for _, endpoint := range endpoints {
				Expect(endpoint.Refreshed).To(BeClosed())
			}
		})
	})

	When("an update change's an org's endpoint addresses", func() {
		BeforeEach(func() {
			org1.Addresses = []string{"org1-address1", "org1-address3"}
			cs.Update(nil, map[string]orderers.OrdererOrg{
				"org1": org1,
				"org2": org2,
			})
		})

		It("creates a new set of orderer endpoints", func() {
			newEndpoints := cs.Endpoints()
			Expect(stripEndpoints(newEndpoints)).To(ConsistOf(
				stripEndpoints([]*orderers.Endpoint{
					{
						Address:   "org1-address1",
						RootCerts: org1Certs,
					},
					{
						Address:   "org1-address3",
						RootCerts: org1Certs,
					},
					{
						Address:   "org2-address1",
						RootCerts: org2Certs,
					},
					{
						Address:   "org2-address2",
						RootCerts: org2Certs,
					},
				}),
			))
		})

		It("closes the refresh channel for all of the old endpoints", func() {
			for _, endpoint := range endpoints {
				Expect(endpoint.Refreshed).To(BeClosed())
			}
		})
	})

	When("an update references an overridden org endpoint address", func() {
		BeforeEach(func() {
			cs.Update(nil, map[string]orderers.OrdererOrg{
				"org1": {
					Addresses: []string{"org1-address1", "override-address"},
					RootCerts: [][]byte{cert1, cert2},
				},
			})
		})

		It("creates a new set of orderer endpoints with overrides", func() {
			newEndpoints := cs.Endpoints()
			Expect(stripEndpoints(newEndpoints)).To(ConsistOf(
				stripEndpoints([]*orderers.Endpoint{
					{
						Address:   "org1-address1",
						RootCerts: org1Certs,
					},
					{
						Address:   "re-mapped-address",
						RootCerts: overrideCerts,
					},
				}),
			))
		})
	})

	When("an update removes an ordering organization", func() {
		BeforeEach(func() {
			cs.Update(nil, map[string]orderers.OrdererOrg{
				"org2": org2,
			})
		})

		It("creates a new set of orderer endpoints", func() {
			newEndpoints := cs.Endpoints()
			Expect(stripEndpoints(newEndpoints)).To(ConsistOf(
				stripEndpoints([]*orderers.Endpoint{
					{
						Address:   "org2-address1",
						RootCerts: org2Certs,
					},
					{
						Address:   "org2-address2",
						RootCerts: org2Certs,
					},
				}),
			))
		})

		It("closes the refresh channel for all of the old endpoints", func() {
			for _, endpoint := range endpoints {
				Expect(endpoint.Refreshed).To(BeClosed())
			}
		})

		When("the org is added back", func() {
			BeforeEach(func() {
				cs.Update(nil, map[string]orderers.OrdererOrg{
					"org1": org1,
					"org2": org2,
				})
			})

			It("returns to the set of orderer endpoints", func() {
				newEndpoints := cs.Endpoints()
				Expect(stripEndpoints(newEndpoints)).To(ConsistOf(
					stripEndpoints([]*orderers.Endpoint{
						{
							Address:   "org1-address1",
							RootCerts: org1Certs,
						},
						{
							Address:   "org1-address2",
							RootCerts: org1Certs,
						},
						{
							Address:   "org2-address1",
							RootCerts: org2Certs,
						},
						{
							Address:   "org2-address2",
							RootCerts: org2Certs,
						},
					}),
				))
			})
		})
	})

	When("an update modifies the global endpoints but does not affect the org endpoints", func() {
		BeforeEach(func() {
			cs.Update(nil, map[string]orderers.OrdererOrg{
				"org1": org1,
				"org2": org2,
			})
		})

		It("does not update the endpoints", func() {
			newEndpoints := cs.Endpoints()
			Expect(newEndpoints).To(Equal(endpoints))
		})

		It("does not close any of the refresh channels", func() {
			for _, endpoint := range endpoints {
				Expect(endpoint.Refreshed).NotTo(BeClosed())
			}
		})
	})

	When("the configuration does not contain orderer org endpoints", func() {
		var globalCerts [][]byte

		BeforeEach(func() {
			org1.Addresses = nil
			org2.Addresses = nil

			globalCerts = [][]byte{cert1, cert2, cert3}

			cs.Update([]string{"global-addr1", "global-addr2"}, map[string]orderers.OrdererOrg{
				"org1": org1,
				"org2": org2,
			})
		})

		It("creates endpoints for the global addrs", func() {
			newEndpoints := cs.Endpoints()
			Expect(stripEndpoints(newEndpoints)).To(ConsistOf(
				stripEndpoints([]*orderers.Endpoint{
					{
						Address:   "global-addr1",
						RootCerts: globalCerts,
					},
					{
						Address:   "global-addr2",
						RootCerts: globalCerts,
					},
				}),
			))
		})

		It("closes the refresh channel for all of the old endpoints", func() {
			for _, endpoint := range endpoints {
				Expect(endpoint.Refreshed).To(BeClosed())
			}
		})

		When("the global list of addresses grows", func() {
			BeforeEach(func() {
				cs.Update([]string{"global-addr1", "global-addr2", "global-addr3"}, map[string]orderers.OrdererOrg{
					"org1": org1,
					"org2": org2,
				})
			})

			It("creates endpoints for the global addrs", func() {
				newEndpoints := cs.Endpoints()
				Expect(stripEndpoints(newEndpoints)).To(ConsistOf(
					stripEndpoints([]*orderers.Endpoint{
						{
							Address:   "global-addr1",
							RootCerts: globalCerts,
						},
						{
							Address:   "global-addr2",
							RootCerts: globalCerts,
						},
						{
							Address:   "global-addr3",
							RootCerts: globalCerts,
						},
					}),
				))
			})

			It("closes the refresh channel for all of the old endpoints", func() {
				for _, endpoint := range endpoints {
					Expect(endpoint.Refreshed).To(BeClosed())
				}
			})
		})

		When("the global set of addresses shrinks", func() {
			BeforeEach(func() {
				cs.Update([]string{"global-addr1"}, map[string]orderers.OrdererOrg{
					"org1": org1,
					"org2": org2,
				})
			})

			It("creates endpoints for the global addrs", func() {
				newEndpoints := cs.Endpoints()
				Expect(stripEndpoints(newEndpoints)).To(ConsistOf(
					stripEndpoints([]*orderers.Endpoint{
						{
							Address:   "global-addr1",
							RootCerts: globalCerts,
						},
					}),
				))
			})

			It("closes the refresh channel for all of the old endpoints", func() {
				for _, endpoint := range endpoints {
					Expect(endpoint.Refreshed).To(BeClosed())
				}
			})
		})

		When("the global set of addresses is modified", func() {
			BeforeEach(func() {
				cs.Update([]string{"global-addr1", "global-addr3"}, map[string]orderers.OrdererOrg{
					"org1": org1,
					"org2": org2,
				})
			})

			It("creates endpoints for the global addrs", func() {
				newEndpoints := cs.Endpoints()
				Expect(stripEndpoints(newEndpoints)).To(ConsistOf(
					stripEndpoints([]*orderers.Endpoint{
						{
							Address:   "global-addr1",
							RootCerts: globalCerts,
						},
						{
							Address:   "global-addr3",
							RootCerts: globalCerts,
						},
					}),
				))
			})

			It("closes the refresh channel for all of the old endpoints", func() {
				for _, endpoint := range endpoints {
					Expect(endpoint.Refreshed).To(BeClosed())
				}
			})
		})

		When("an update to the global addrs references an overridden org endpoint address", func() {
			BeforeEach(func() {
				cs.Update([]string{"global-addr1", "override-address"}, map[string]orderers.OrdererOrg{
					"org1": org1,
					"org2": org2,
				},
				)
			})

			It("creates a new set of orderer endpoints with overrides", func() {
				newEndpoints := cs.Endpoints()
				Expect(stripEndpoints(newEndpoints)).To(ConsistOf(
					stripEndpoints([]*orderers.Endpoint{
						{
							Address:   "global-addr1",
							RootCerts: globalCerts,
						},
						{
							Address:   "re-mapped-address",
							RootCerts: overrideCerts,
						},
					}),
				))
			})
		})

		When("an orderer org adds an endpoint", func() {
			BeforeEach(func() {
				org1.Addresses = []string{"new-org1-address"}
				cs.Update([]string{"global-addr1", "global-addr2"}, map[string]orderers.OrdererOrg{
					"org1": org1,
					"org2": org2,
				})
			})

			It("removes the global endpoints and uses only the org level ones", func() {
				newEndpoints := cs.Endpoints()
				Expect(stripEndpoints(newEndpoints)).To(ConsistOf(
					stripEndpoints([]*orderers.Endpoint{
						{
							Address:   "new-org1-address",
							RootCerts: org1Certs,
						},
					}),
				))
			})

			It("closes the refresh channel for all of the old endpoints", func() {
				for _, endpoint := range endpoints {
					Expect(endpoint.Refreshed).To(BeClosed())
				}
			})
		})
	})
})
