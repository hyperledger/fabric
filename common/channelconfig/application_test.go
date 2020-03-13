/*
Copyright IBM Corp. 2017 All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package channelconfig

import (
	"testing"

	"github.com/golang/protobuf/proto"
	cb "github.com/hyperledger/fabric-protos-go/common"
	"github.com/hyperledger/fabric/common/capabilities"
	"github.com/hyperledger/fabric/protoutil"
	. "github.com/onsi/gomega"
)

func TestApplicationInterface(t *testing.T) {
	_ = Application((*ApplicationConfig)(nil))
}

func TestACL(t *testing.T) {
	g := NewGomegaWithT(t)
	cgt := &cb.ConfigGroup{
		Values: map[string]*cb.ConfigValue{
			ACLsKey: {
				Value: protoutil.MarshalOrPanic(
					ACLValues(map[string]string{}).Value(),
				),
			},
			CapabilitiesKey: {
				Value: protoutil.MarshalOrPanic(
					CapabilitiesValue(map[string]bool{
						capabilities.ApplicationV1_2: true,
					}).Value(),
				),
			},
		},
	}

	t.Run("Success", func(t *testing.T) {
		cg := proto.Clone(cgt).(*cb.ConfigGroup)
		_, err := NewApplicationConfig(proto.Clone(cg).(*cb.ConfigGroup), nil)
		g.Expect(err).NotTo(HaveOccurred())
	})

	t.Run("MissingCapability", func(t *testing.T) {
		cg := proto.Clone(cgt).(*cb.ConfigGroup)
		delete(cg.Values, CapabilitiesKey)
		_, err := NewApplicationConfig(cg, nil)
		g.Expect(err).To(MatchError("ACLs may not be specified without the required capability"))
	})
}
