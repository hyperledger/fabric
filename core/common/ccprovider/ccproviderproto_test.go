/*
Copyright IBM Corp. All Rights Reserved.
SPDX-License-Identifier: Apache-2.0
*/

package ccprovider_test

import (
	"testing"

	"github.com/hyperledger/fabric/core/common/ccprovider"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/encoding/prototext"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/protoadapt"
)

// -------- ChaincodeDataOld is stored on the LSCC -------

// ChaincodeDataOld defines the datastructure for chaincodes to be serialized by proto
// Type provides an additional check by directing to use a specific package after instantiation
// Data is Type specific (see CDSPackage and SignedCDSPackage)
type ChaincodeData struct {
	// Name of the chaincode
	Name string `protobuf:"bytes,1,opt,name=name"`
	// Version of the chaincode
	Version string `protobuf:"bytes,2,opt,name=version"`
	// Escc for the chaincode instance
	Escc string `protobuf:"bytes,3,opt,name=escc"`
	// Vscc for the chaincode instance
	Vscc string `protobuf:"bytes,4,opt,name=vscc"`
	// Policy endorsement policy for the chaincode instance
	Policy []byte `protobuf:"bytes,5,opt,name=policy,proto3"`
	// Data data specific to the package
	Data []byte `protobuf:"bytes,6,opt,name=data,proto3"`
	// Id of the chaincode that's the unique fingerprint for the CC This is not
	// currently used anywhere but serves as a good eyecatcher
	Id []byte `protobuf:"bytes,7,opt,name=id,proto3"`
	// InstantiationPolicy for the chaincode
	InstantiationPolicy []byte `protobuf:"bytes,8,opt,name=instantiation_policy,proto3"`
}

// implement functions needed from proto.Message for proto's mar/unmarshal functions

// Reset resets
func (cd *ChaincodeData) Reset() { *cd = ChaincodeData{} }

// String converts to string
func (cd *ChaincodeData) String() string {
	b, _ := prototext.MarshalOptions{Indent: ""}.Marshal(protoadapt.MessageV2Of(cd))
	return string(b)
}

// ProtoMessage just exists to make proto happy
func (*ChaincodeData) ProtoMessage() {}

// ----- SignedCDSData ------
// SignedCDSData is data stored in the LSCC on instantiation of a CC
// for SignedCDSPackage. This needs to be serialized for ChaincodeData
// hence the protobuf format
type SignedCDSData struct {
	CodeHash      []byte `protobuf:"bytes,1,opt,name=hash"`
	MetaDataHash  []byte `protobuf:"bytes,2,opt,name=metadatahash"`
	SignatureHash []byte `protobuf:"bytes,3,opt,name=signaturehash"`
}

// ----implement functions needed from proto.Message for proto's mar/unmarshal functions

// Reset resets
func (data *SignedCDSData) Reset() { *data = SignedCDSData{} }

// String converts to string
func (data *SignedCDSData) String() string {
	b, _ := prototext.MarshalOptions{Indent: ""}.Marshal(protoadapt.MessageV2Of(data))
	return string(b)
}

// ProtoMessage just exists to make proto happy
func (*SignedCDSData) ProtoMessage() {}

func TestChaincodeDataOldToNew(t *testing.T) {
	ccData := &ccprovider.ChaincodeData{
		Name:                "chaincode-data-name",
		Version:             "version",
		Escc:                "escc",
		Vscc:                "vscc",
		Policy:              []byte("policy"),
		Data:                []byte("data"),
		Id:                  []byte("id"),
		InstantiationPolicy: []byte("instantiation-policy"),
	}
	bNew, err := proto.Marshal(ccData)
	require.NoError(t, err)

	ccDataOld := &ChaincodeData{
		Name:                "chaincode-data-name",
		Version:             "version",
		Escc:                "escc",
		Vscc:                "vscc",
		Policy:              []byte("policy"),
		Data:                []byte("data"),
		Id:                  []byte("id"),
		InstantiationPolicy: []byte("instantiation-policy"),
	}
	bOld, err := proto.Marshal(protoadapt.MessageV2Of(ccDataOld))
	require.NoError(t, err)

	ccDataOld2 := &ChaincodeData{}
	ccData2 := &ccprovider.ChaincodeData{}
	err = proto.Unmarshal(bNew, protoadapt.MessageV2Of(ccDataOld2))
	require.NoError(t, err)
	err = proto.Unmarshal(bOld, ccData2)
	require.NoError(t, err)

	require.True(t, proto.Equal(protoadapt.MessageV2Of(ccDataOld), protoadapt.MessageV2Of(ccDataOld2)))
	require.True(t, proto.Equal(ccData, ccData2))
}

func TestSignedCDSDataOldToNew(t *testing.T) {
	ccData := &ccprovider.SignedCDSData{
		CodeHash:      []byte("policy"),
		MetaDataHash:  []byte("data"),
		SignatureHash: []byte("id"),
	}
	bNew, err := proto.Marshal(ccData)
	require.NoError(t, err)

	ccDataOld := &SignedCDSData{
		CodeHash:      []byte("policy"),
		MetaDataHash:  []byte("data"),
		SignatureHash: []byte("id"),
	}
	bOld, err := proto.Marshal(protoadapt.MessageV2Of(ccDataOld))
	require.NoError(t, err)

	ccDataOld2 := &SignedCDSData{}
	ccData2 := &ccprovider.SignedCDSData{}
	err = proto.Unmarshal(bNew, protoadapt.MessageV2Of(ccDataOld2))
	require.NoError(t, err)
	err = proto.Unmarshal(bOld, ccData2)
	require.NoError(t, err)

	require.True(t, proto.Equal(protoadapt.MessageV2Of(ccDataOld), protoadapt.MessageV2Of(ccDataOld2)))
	require.True(t, proto.Equal(ccData, ccData2))
}
