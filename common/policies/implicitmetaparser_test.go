/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package policies

import (
	"fmt"
	"testing"

	"github.com/golang/protobuf/proto"
	cb "github.com/hyperledger/fabric-protos-go/common"
	"github.com/stretchr/testify/require"
)

func TestImplicitMetaParserWrongTokenCount(t *testing.T) {
	errorMatch := "expected two space separated tokens, but got"

	t.Run("NoArgs", func(t *testing.T) {
		res, err := ImplicitMetaFromString("")
		require.Nil(t, res)
		require.Error(t, err)
		require.Regexp(t, errorMatch, err.Error())
	})

	t.Run("OneArg", func(t *testing.T) {
		res, err := ImplicitMetaFromString("ANY")
		require.Nil(t, res)
		require.Error(t, err)
		require.Regexp(t, errorMatch, err.Error())
	})

	t.Run("ThreeArgs", func(t *testing.T) {
		res, err := ImplicitMetaFromString("ANY of these")
		require.Nil(t, res)
		require.Error(t, err)
		require.Regexp(t, errorMatch, err.Error())
	})
}

func TestImplicitMetaParserBadRule(t *testing.T) {
	res, err := ImplicitMetaFromString("BAD Rule")
	require.Nil(t, res)
	require.Error(t, err)
	require.Regexp(t, "unknown rule type 'BAD'", err.Error())
}

func TestImplicitMetaParserGreenPath(t *testing.T) {
	for _, rule := range []cb.ImplicitMetaPolicy_Rule{cb.ImplicitMetaPolicy_ANY, cb.ImplicitMetaPolicy_ALL, cb.ImplicitMetaPolicy_MAJORITY} {
		t.Run(rule.String(), func(t *testing.T) {
			subPolicy := "foo"
			res, err := ImplicitMetaFromString(fmt.Sprintf("%v %s", rule, subPolicy))
			require.NoError(t, err)
			require.True(t, proto.Equal(res, &cb.ImplicitMetaPolicy{
				SubPolicy: subPolicy,
				Rule:      rule,
			}))
		})
	}
}
