/*
Copyright IBM Corp. 2017 All Rights Reserved.

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

package cauthdsl

import (
	"reflect"
	"testing"

	"github.com/hyperledger/fabric/protos/common"
	"github.com/hyperledger/fabric/protos/utils"
	"github.com/stretchr/testify/assert"
)

func TestAnd(t *testing.T) {
	p1, err := FromString("AND('A.member', 'B.member')")
	assert.NoError(t, err)

	principals := make([]*common.MSPPrincipal, 0)

	principals = append(principals, &common.MSPPrincipal{
		PrincipalClassification: common.MSPPrincipal_ByMSPRole,
		Principal:               utils.MarshalOrPanic(&common.MSPRole{Role: common.MSPRole_Member, MSPIdentifier: "A"})})

	principals = append(principals, &common.MSPPrincipal{
		PrincipalClassification: common.MSPPrincipal_ByMSPRole,
		Principal:               utils.MarshalOrPanic(&common.MSPRole{Role: common.MSPRole_Member, MSPIdentifier: "B"})})

	p2 := &common.SignaturePolicyEnvelope{
		Version:    0,
		Policy:     And(SignedBy(0), SignedBy(1)),
		Identities: principals,
	}

	assert.True(t, reflect.DeepEqual(p1, p2))
}

func TestOr(t *testing.T) {
	p1, err := FromString("OR('A.member', 'B.member')")
	assert.NoError(t, err)

	principals := make([]*common.MSPPrincipal, 0)

	principals = append(principals, &common.MSPPrincipal{
		PrincipalClassification: common.MSPPrincipal_ByMSPRole,
		Principal:               utils.MarshalOrPanic(&common.MSPRole{Role: common.MSPRole_Member, MSPIdentifier: "A"})})

	principals = append(principals, &common.MSPPrincipal{
		PrincipalClassification: common.MSPPrincipal_ByMSPRole,
		Principal:               utils.MarshalOrPanic(&common.MSPRole{Role: common.MSPRole_Member, MSPIdentifier: "B"})})

	p2 := &common.SignaturePolicyEnvelope{
		Version:    0,
		Policy:     Or(SignedBy(0), SignedBy(1)),
		Identities: principals,
	}

	assert.True(t, reflect.DeepEqual(p1, p2))
}

func TestComplex1(t *testing.T) {
	p1, err := FromString("OR('A.member', AND('B.member', 'C.member'))")
	assert.NoError(t, err)

	principals := make([]*common.MSPPrincipal, 0)

	principals = append(principals, &common.MSPPrincipal{
		PrincipalClassification: common.MSPPrincipal_ByMSPRole,
		Principal:               utils.MarshalOrPanic(&common.MSPRole{Role: common.MSPRole_Member, MSPIdentifier: "B"})})

	principals = append(principals, &common.MSPPrincipal{
		PrincipalClassification: common.MSPPrincipal_ByMSPRole,
		Principal:               utils.MarshalOrPanic(&common.MSPRole{Role: common.MSPRole_Member, MSPIdentifier: "C"})})

	principals = append(principals, &common.MSPPrincipal{
		PrincipalClassification: common.MSPPrincipal_ByMSPRole,
		Principal:               utils.MarshalOrPanic(&common.MSPRole{Role: common.MSPRole_Member, MSPIdentifier: "A"})})

	p2 := &common.SignaturePolicyEnvelope{
		Version:    0,
		Policy:     Or(SignedBy(2), And(SignedBy(0), SignedBy(1))),
		Identities: principals,
	}

	assert.True(t, reflect.DeepEqual(p1, p2))
}

func TestComplex2(t *testing.T) {
	p1, err := FromString("OR(AND('A.member', 'B.member'), OR('C.admin', 'D.member'))")
	assert.NoError(t, err)

	principals := make([]*common.MSPPrincipal, 0)

	principals = append(principals, &common.MSPPrincipal{
		PrincipalClassification: common.MSPPrincipal_ByMSPRole,
		Principal:               utils.MarshalOrPanic(&common.MSPRole{Role: common.MSPRole_Member, MSPIdentifier: "A"})})

	principals = append(principals, &common.MSPPrincipal{
		PrincipalClassification: common.MSPPrincipal_ByMSPRole,
		Principal:               utils.MarshalOrPanic(&common.MSPRole{Role: common.MSPRole_Member, MSPIdentifier: "B"})})

	principals = append(principals, &common.MSPPrincipal{
		PrincipalClassification: common.MSPPrincipal_ByMSPRole,
		Principal:               utils.MarshalOrPanic(&common.MSPRole{Role: common.MSPRole_Admin, MSPIdentifier: "C"})})

	principals = append(principals, &common.MSPPrincipal{
		PrincipalClassification: common.MSPPrincipal_ByMSPRole,
		Principal:               utils.MarshalOrPanic(&common.MSPRole{Role: common.MSPRole_Member, MSPIdentifier: "D"})})

	p2 := &common.SignaturePolicyEnvelope{
		Version:    0,
		Policy:     Or(And(SignedBy(0), SignedBy(1)), Or(SignedBy(2), SignedBy(3))),
		Identities: principals,
	}

	assert.True(t, reflect.DeepEqual(p1, p2))
}
