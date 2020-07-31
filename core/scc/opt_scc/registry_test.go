/*
Copyright 2020 Intel Corporation

SPDX-License-Identifier: Apache-2.0
*/

package opt_scc_test

import (
	"sort"
	"testing"

	. "github.com/onsi/gomega"

	"github.com/hyperledger/fabric-chaincode-go/shim"
	"github.com/hyperledger/fabric/core/aclmgmt"
	"github.com/hyperledger/fabric/core/peer"
	"github.com/hyperledger/fabric/core/scc"
	"github.com/hyperledger/fabric/core/scc/opt_scc"
)

type mock_scc struct {
	name string
}

func (s *mock_scc) Name() string {
	return s.name
}
func (s *mock_scc) Chaincode() shim.Chaincode {
	return nil
}

func TestRegistry(t *testing.T) {
	g := NewGomegaWithT(t)

	// 1. initially we should get an empty list
	ffs := opt_scc.ListFactoryFuncs()
	g.Expect(ffs).NotTo(BeNil(), "factory function list should not be nil")
	g.Expect(len(ffs)).To(Equal(0), "factory function list should have zero elements")

	// 2. after adding several handlers, make sure it returns exactly and only these handlers
	names := []string{"ff1", "ff2", "ff3"}
	for _, n := range names {
		nLocal := n // to get separate version for below closure ...
		opt_scc.AddFactoryFunc(
			func(aclProvider aclmgmt.ACLProvider, p *peer.Peer) scc.SelfDescribingSysCC {
				return &mock_scc{nLocal}
			})
	}
	ffs = opt_scc.ListFactoryFuncs()
	g.Expect(ffs).NotTo(BeNil(), "factory function list should not be nil")
	g.Expect(len(ffs)).To(Equal(len(names)), "factory function list should have %v elements", len(names))
	got_names := []string{}
	for i, ff := range ffs {
		s := ff(nil, nil)
		g.Expect(s).NotTo(BeNil(), "received unexpected factory function (element %v in list) returns nil", i)
		got_names = append(got_names, s.Name())
	}
	// Note: we do not force any order, so compare sorted lists
	sort.Strings(names)
	sort.Strings(got_names)
	g.Expect(got_names).To(Equal(names), "Unexpected factory function in list")

	// 3. Test that error on nil factory functions
	g.Expect(func() {
		opt_scc.AddFactoryFunc(nil)
	}).To(Panic(), "Adding a nil factory function should panic")

}
