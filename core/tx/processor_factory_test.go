/*
Copyright State Street Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package tx_test

import (
	"testing"

	"github.com/hyperledger/fabric-protos-go/common"
	"github.com/hyperledger/fabric-protos-go/peer"
	"github.com/hyperledger/fabric/core/tx"
	pkgtx "github.com/hyperledger/fabric/pkg/tx"
	"github.com/hyperledger/fabric/protoutil"

	"github.com/pkg/errors"
	"github.com/stretchr/testify/require"
)

func TestCreateProcessor(t *testing.T) {
	f := &tx.ProcessorFactory{}

	invalidType := int32(-1)

	env := protoutil.MarshalOrPanic(&common.Envelope{Payload: protoutil.MarshalOrPanic(&common.Payload{Header: &common.Header{ChannelHeader: protoutil.MarshalOrPanic(&common.ChannelHeader{ChannelId: "myc", TxId: "tid", Type: invalidType}), SignatureHeader: protoutil.MarshalOrPanic(&common.SignatureHeader{Creator: []byte("creator"), Nonce: []byte("nonce")})}}), Signature: []byte("signature")})

	_, _, err := f.CreateProcessor(env)
	require.Equal(t, err.Error(), "ValidationCode = UNKNOWN_TX_TYPE, ActualErr = invalid transaction type -1")
}

func TestBasicTxValidity(t *testing.T) {
	f := &tx.ProcessorFactory{}

	invalidConfigs := []struct {
		env         []byte
		expectedErr *pkgtx.InvalidErr
	}{
		{
			nil, &pkgtx.InvalidErr{
				ActualErr:      errors.New("nil envelope payload"),
				ValidationCode: peer.TxValidationCode_BAD_PAYLOAD,
			},
		},
		{
			[]byte("bad env"), &pkgtx.InvalidErr{
				ActualErr:      errors.New("error unmarshalling Envelope"),
				ValidationCode: peer.TxValidationCode_INVALID_OTHER_REASON,
			},
		},
		{
			protoutil.MarshalOrPanic(&common.Envelope{Payload: []byte("bad payload"), Signature: []byte("signature")}), &pkgtx.InvalidErr{
				ActualErr:      errors.New("error unmarshalling Payload"),
				ValidationCode: peer.TxValidationCode_BAD_PAYLOAD,
			},
		},
		{
			protoutil.MarshalOrPanic(&common.Envelope{Signature: []byte("signature")}), &pkgtx.InvalidErr{
				ActualErr:      errors.New("nil envelope payload"),
				ValidationCode: peer.TxValidationCode_BAD_PAYLOAD,
			},
		},
		{
			protoutil.MarshalOrPanic(&common.Envelope{Payload: protoutil.MarshalOrPanic(&common.Payload{Data: []byte("data")}), Signature: []byte("signature")}), &pkgtx.InvalidErr{
				ActualErr:      errors.New("nil payload header"),
				ValidationCode: peer.TxValidationCode_BAD_PAYLOAD,
			},
		},
		{
			protoutil.MarshalOrPanic(&common.Envelope{Payload: protoutil.MarshalOrPanic(&common.Payload{Header: &common.Header{}}), Signature: []byte("signature")}), &pkgtx.InvalidErr{
				ActualErr:      errors.New("nil payload channel header"),
				ValidationCode: peer.TxValidationCode_BAD_PAYLOAD,
			},
		},
		{
			protoutil.MarshalOrPanic(&common.Envelope{Payload: protoutil.MarshalOrPanic(&common.Payload{Header: &common.Header{ChannelHeader: protoutil.MarshalOrPanic(&common.ChannelHeader{ChannelId: "myc", TxId: "tid"})}}), Signature: []byte("signature")}), &pkgtx.InvalidErr{
				ActualErr:      errors.New("nil payload signature header"),
				ValidationCode: peer.TxValidationCode_BAD_PAYLOAD,
			},
		},
		{
			protoutil.MarshalOrPanic(&common.Envelope{Payload: protoutil.MarshalOrPanic(&common.Payload{Header: &common.Header{ChannelHeader: []byte("bad channel header"), SignatureHeader: protoutil.MarshalOrPanic(&common.SignatureHeader{Creator: []byte("creator"), Nonce: []byte("nonce")})}}), Signature: []byte("signature")}), &pkgtx.InvalidErr{
				ActualErr:      errors.New("error unmarshalling ChannelHeader"),
				ValidationCode: peer.TxValidationCode_BAD_PAYLOAD,
			},
		},
		{
			protoutil.MarshalOrPanic(&common.Envelope{Payload: protoutil.MarshalOrPanic(&common.Payload{Header: &common.Header{ChannelHeader: protoutil.MarshalOrPanic(&common.ChannelHeader{ChannelId: "myc", TxId: "tid"}), SignatureHeader: []byte("bad sig hdr")}}), Signature: []byte("signature")}), &pkgtx.InvalidErr{
				ActualErr:      errors.New("error unmarshalling SignatureHeader"),
				ValidationCode: peer.TxValidationCode_BAD_PAYLOAD,
			},
		},
	}

	for _, ic := range invalidConfigs {
		_, _, err := f.CreateProcessor(ic.env)
		require.ErrorContains(t, err, ic.expectedErr.Error())
	}

	// NOTE: common.HeaderType_CONFIG is a valid type and this should succeed when we populate ProcessorFactory (and signifies successful validation). Till then, a negative test
	env := protoutil.MarshalOrPanic(&common.Envelope{Payload: protoutil.MarshalOrPanic(&common.Payload{Header: &common.Header{ChannelHeader: protoutil.MarshalOrPanic(&common.ChannelHeader{ChannelId: "myc", TxId: "tid", Type: int32(common.HeaderType_CONFIG)}), SignatureHeader: protoutil.MarshalOrPanic(&common.SignatureHeader{Creator: []byte("creator"), Nonce: []byte("nonce")})}}), Signature: []byte("signature")})

	_, _, err := f.CreateProcessor(env)
	require.Equal(t, err.Error(), "ValidationCode = UNKNOWN_TX_TYPE, ActualErr = invalid transaction type 1")
}
