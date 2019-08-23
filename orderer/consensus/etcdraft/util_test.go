/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package etcdraft

import (
	"encoding/base64"
	"io/ioutil"
	"path/filepath"
	"testing"

	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric-protos-go/common"
	etcdraftproto "github.com/hyperledger/fabric-protos-go/orderer/etcdraft"
	"github.com/hyperledger/fabric/bccsp/sw"
	"github.com/hyperledger/fabric/common/crypto/tlsgen"
	"github.com/hyperledger/fabric/orderer/common/cluster"
	"github.com/hyperledger/fabric/protoutil"
	"github.com/stretchr/testify/assert"
)

func TestIsConsenterOfChannel(t *testing.T) {
	certInsideConfigBlock, err := base64.StdEncoding.DecodeString("LS0tLS1CRUdJTiBDRVJUSUZJQ0FURS0tLS0tCk1JSUNmekNDQWlhZ0F3SUJBZ0l" +
		"SQUo4bjFLYTVzS1ZaTXRMTHJ1dldERDB3Q2dZSUtvWkl6ajBFQXdJd2JERUwKTUFrR0ExVUVCaE1DVlZNeEV6QVJCZ05WQkFnVENrTmhiR" +
		"2xtYjNKdWFXRXhGakFVQmdOVkJBY1REVk5oYmlCRwpjbUZ1WTJselkyOHhGREFTQmdOVkJBb1RDMlY0WVcxd2JHVXVZMjl0TVJvd0dBWUR" +
		"WUVFERXhGMGJITmpZUzVsCmVHRnRjR3hsTG1OdmJUQWVGdzB4T0RFeE1EWXdPVFE1TURCYUZ3MHlPREV4TURNd09UUTVNREJhTUZreEN6QU" +
		"oKQmdOVkJBWVRBbFZUTVJNd0VRWURWUVFJRXdwRFlXeHBabTl5Ym1saE1SWXdGQVlEVlFRSEV3MVRZVzRnUm5KaApibU5wYzJOdk1SMHdH" +
		"d1lEVlFRREV4UnZjbVJsY21WeU1TNWxlR0Z0Y0d4bExtTnZiVEJaTUJNR0J5cUdTTTQ5CkFnRUdDQ3FHU000OUF3RUhBMElBQkRUVlFZc0" +
		"ZKZWxUcFZDMDFsek5DSkx6OENRMFFGVDBvN1BmSnBwSkl2SXgKUCtRVjQvRGRCSnRqQ0cvcGsvMGFxZXRpSjhZRUFMYmMrOUhmWnExN2tJ" +
		"Q2pnYnN3Z2Jnd0RnWURWUjBQQVFILwpCQVFEQWdXZ01CMEdBMVVkSlFRV01CUUdDQ3NHQVFVRkJ3TUJCZ2dyQmdFRkJRY0RBakFNQmdOV" +
		"khSTUJBZjhFCkFqQUFNQ3NHQTFVZEl3UWtNQ0tBSUVBOHFrSVJRTVBuWkxBR2g0TXZla2gzZFpHTmNxcEhZZWlXdzE3Rmw0ZlMKTUV3R0" +
		"ExVWRFUVJGTUVPQ0ZHOXlaR1Z5WlhJeExtVjRZVzF3YkdVdVkyOXRnZ2h2Y21SbGNtVnlNWUlKYkc5agpZV3hvYjNOMGh3Ui9BQUFCaHh" +
		"BQUFBQUFBQUFBQUFBQUFBQUFBQUFCTUFvR0NDcUdTTTQ5QkFNQ0EwY0FNRVFDCklFckJZRFVzV0JwOHB0ZVFSaTZyNjNVelhJQi81Sn" +
		"YxK0RlTkRIUHc3aDljQWlCakYrM3V5TzBvMEdRclB4MEUKUWptYlI5T3BVREN2LzlEUkNXWU9GZitkVlE9PQotLS0tLUVORCBDRVJUSU" +
		"ZJQ0FURS0tLS0tCg==")
	assert.NoError(t, err)

	validBlock := func() *common.Block {
		b, err := ioutil.ReadFile(filepath.Join("testdata", "etcdraftgenesis.block"))
		assert.NoError(t, err)
		block := &common.Block{}
		err = proto.Unmarshal(b, block)
		assert.NoError(t, err)
		return block
	}
	for _, testCase := range []struct {
		name          string
		expectedError string
		configBlock   *common.Block
		certificate   []byte
	}{
		{
			name:          "nil block",
			expectedError: "nil block",
		},
		{
			name:          "no block data",
			expectedError: "block data is nil",
			configBlock:   &common.Block{},
		},
		{
			name: "invalid envelope inside block",
			expectedError: "failed to unmarshal payload from envelope:" +
				" error unmarshaling Payload: proto: common.Payload: illegal tag 0 (wire type 1)",
			configBlock: &common.Block{
				Data: &common.BlockData{
					Data: [][]byte{protoutil.MarshalOrPanic(&common.Envelope{
						Payload: []byte{1, 2, 3},
					})},
				},
			},
		},
		{
			name:          "valid config block with cert mismatch",
			configBlock:   validBlock(),
			certificate:   certInsideConfigBlock[2:],
			expectedError: cluster.ErrNotInChannel.Error(),
		},
		{
			name:        "valid config block with matching cert",
			configBlock: validBlock(),
			certificate: certInsideConfigBlock,
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			cryptoProvider, err := sw.NewDefaultSecurityLevelWithKeystore(sw.NewDummyKeyStore())
			assert.NoError(t, err)

			consenterCertificate := &ConsenterCertificate{
				ConsenterCertificate: testCase.certificate,
				CryptoProvider:       cryptoProvider,
			}
			err = consenterCertificate.IsConsenterOfChannel(testCase.configBlock)
			if testCase.expectedError != "" {
				assert.EqualError(t, err, testCase.expectedError)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestCheckConfigMetadata(t *testing.T) {
	tlsCA, err := tlsgen.NewCA()
	if err != nil {
		panic(err)
	}
	serverPair, err := tlsCA.NewServerCertKeyPair("localhost")
	serverCert := serverPair.Cert
	if err != nil {
		panic(err)
	}
	clientPair, err := tlsCA.NewClientCertKeyPair()
	clientCert := clientPair.Cert
	if err != nil {
		panic(err)
	}
	validOptions := &etcdraftproto.Options{
		TickInterval:         "500ms",
		ElectionTick:         10,
		HeartbeatTick:        1,
		MaxInflightBlocks:    5,
		SnapshotIntervalSize: 20 * 1024 * 1024, // 20 MB
	}
	singleConsenter := &etcdraftproto.Consenter{
		Host:          "host1",
		Port:          10001,
		ClientTlsCert: clientCert,
		ServerTlsCert: serverCert,
	}

	// valid metadata should give nil error
	goodMetadata := &etcdraftproto.ConfigMetadata{
		Options: validOptions,
		Consenters: []*etcdraftproto.Consenter{
			singleConsenter,
		},
	}
	assert.Nil(t, CheckConfigMetadata(goodMetadata))

	// test variety of bad metadata
	for _, testCase := range []struct {
		description string
		metadata    *etcdraftproto.ConfigMetadata
		errRegex    string
	}{
		{
			description: "nil metadata",
			metadata:    nil,
			errRegex:    "nil Raft config metadata",
		},
		{
			description: "HeartbeatTick is 0",
			metadata: &etcdraftproto.ConfigMetadata{
				Options: &etcdraftproto.Options{
					HeartbeatTick: 0,
				},
			},
			errRegex: "none of HeartbeatTick .* can be zero",
		},
		{
			description: "ElectionTick is 0",
			metadata: &etcdraftproto.ConfigMetadata{
				Options: &etcdraftproto.Options{
					HeartbeatTick: validOptions.HeartbeatTick,
					ElectionTick:  0,
				},
			},
			errRegex: "none of .* ElectionTick .* can be zero",
		},
		{
			description: "MaxInflightBlocks is 0",
			metadata: &etcdraftproto.ConfigMetadata{
				Options: &etcdraftproto.Options{
					HeartbeatTick:     validOptions.HeartbeatTick,
					ElectionTick:      validOptions.ElectionTick,
					MaxInflightBlocks: 0,
				},
			},
			errRegex: "none of .* MaxInflightBlocks .* can be zero",
		},
		{
			description: "ElectionTick is less than HeartbeatTick",
			metadata: &etcdraftproto.ConfigMetadata{
				Options: &etcdraftproto.Options{
					HeartbeatTick:     10,
					ElectionTick:      1,
					MaxInflightBlocks: validOptions.MaxInflightBlocks,
				},
			},
			errRegex: "ElectionTick .* must be greater than HeartbeatTick",
		},
		{
			description: "TickInterval is not parsable",
			metadata: &etcdraftproto.ConfigMetadata{
				Options: &etcdraftproto.Options{
					HeartbeatTick:     validOptions.HeartbeatTick,
					ElectionTick:      validOptions.ElectionTick,
					MaxInflightBlocks: validOptions.MaxInflightBlocks,
					TickInterval:      "abcd",
				},
			},
			errRegex: "failed to parse TickInterval .* to time duration",
		},
		{
			description: "TickInterval is 0",
			metadata: &etcdraftproto.ConfigMetadata{
				Options: &etcdraftproto.Options{
					HeartbeatTick:     validOptions.HeartbeatTick,
					ElectionTick:      validOptions.ElectionTick,
					MaxInflightBlocks: validOptions.MaxInflightBlocks,
					TickInterval:      "0s",
				},
			},
			errRegex: "TickInterval cannot be zero",
		},
		{
			description: "consenter set is empty",
			metadata: &etcdraftproto.ConfigMetadata{
				Options:    validOptions,
				Consenters: []*etcdraftproto.Consenter{},
			},
			errRegex: "empty consenter set",
		},
		{
			description: "metadata has nil consenter",
			metadata: &etcdraftproto.ConfigMetadata{
				Options: validOptions,
				Consenters: []*etcdraftproto.Consenter{
					nil,
				},
			},
			errRegex: "metadata has nil consenter",
		},
		{
			description: "consenter has invalid server cert",
			metadata: &etcdraftproto.ConfigMetadata{
				Options: validOptions,
				Consenters: []*etcdraftproto.Consenter{
					{
						ServerTlsCert: []byte("invalid"),
						ClientTlsCert: clientCert,
					},
				},
			},
			errRegex: "server TLS certificate is not PEM encoded",
		},
		{
			description: "consenter has invalid client cert",
			metadata: &etcdraftproto.ConfigMetadata{
				Options: validOptions,
				Consenters: []*etcdraftproto.Consenter{
					{
						ServerTlsCert: serverCert,
						ClientTlsCert: []byte("invalid"),
					},
				},
			},
			errRegex: "client TLS certificate is not PEM encoded",
		},
		{
			description: "metadata has duplicate consenters",
			metadata: &etcdraftproto.ConfigMetadata{
				Options: validOptions,
				Consenters: []*etcdraftproto.Consenter{
					singleConsenter,
					singleConsenter,
				},
			},
			errRegex: "duplicate consenter",
		},
	} {
		err := CheckConfigMetadata(testCase.metadata)
		assert.NotNil(t, err, testCase.description)
		assert.Regexp(t, testCase.errRegex, err)
	}
}
