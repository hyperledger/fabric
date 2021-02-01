/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package etcdraft

import (
	"crypto/x509"
	"encoding/base64"
	"fmt"
	"io/ioutil"
	"path/filepath"
	"testing"

	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric-protos-go/common"
	etcdraftproto "github.com/hyperledger/fabric-protos-go/orderer/etcdraft"
	"github.com/hyperledger/fabric/bccsp/sw"
	"github.com/hyperledger/fabric/common/crypto/tlsgen"
	"github.com/hyperledger/fabric/common/flogging"
	"github.com/hyperledger/fabric/orderer/common/cluster"
	"github.com/hyperledger/fabric/protoutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	consentersTestDataDir = "testdata/consenters_certs/"
	ca1Dir                = consentersTestDataDir + "ca1"
	ca2Dir                = consentersTestDataDir + "ca2"
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
	require.NoError(t, err)

	ca, err := tlsgen.NewCA()
	require.NoError(t, err)

	kp, err := ca.NewClientCertKeyPair()
	require.NoError(t, err)

	validBlock := func() *common.Block {
		b, err := ioutil.ReadFile(filepath.Join("testdata", "etcdraftgenesis.block"))
		require.NoError(t, err)
		block := &common.Block{}
		err = proto.Unmarshal(b, block)
		require.NoError(t, err)
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
			expectedError: "nil block or nil header",
		},
		{
			name:          "nil header",
			expectedError: "nil block or nil header",
			configBlock:   &common.Block{},
		},
		{
			name:          "no block data",
			expectedError: "block data is nil",
			configBlock:   &common.Block{Header: &common.BlockHeader{}},
		},
		{
			name: "invalid envelope inside block",
			expectedError: "failed to unmarshal payload from envelope:" +
				" error unmarshaling Payload: proto: common.Payload: illegal tag 0 (wire type 1)",
			configBlock: &common.Block{
				Header: &common.BlockHeader{},
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
			certificate:   kp.Cert,
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
			require.NoError(t, err)

			consenterCertificate := &ConsenterCertificate{
				Logger:               flogging.MustGetLogger("test"),
				ConsenterCertificate: testCase.certificate,
				CryptoProvider:       cryptoProvider,
			}
			err = consenterCertificate.IsConsenterOfChannel(testCase.configBlock)
			if testCase.expectedError != "" {
				require.EqualError(t, err, testCase.expectedError)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestVerifyConfigMetadata(t *testing.T) {
	tlsCA, err := tlsgen.NewCA()
	if err != nil {
		panic(err)
	}

	caRootCert, err := parseCertificateFromBytes(tlsCA.CertBytes())
	if err != nil {
		panic(err)
	}

	serverPair, err := tlsCA.NewServerCertKeyPair("localhost")
	if err != nil {
		panic(err)
	}

	clientPair, err := tlsCA.NewClientCertKeyPair()
	if err != nil {
		panic(err)
	}

	unknownTlsCA, err := tlsgen.NewCA()
	if err != nil {
		panic(err)
	}

	unknownServerPair, err := unknownTlsCA.NewServerCertKeyPair("unknownhost")
	if err != nil {
		panic(err)
	}

	unknownServerCert, err := parseCertificateFromBytes(unknownServerPair.Cert)
	if err != nil {
		panic(err)
	}

	unknownClientPair, err := unknownTlsCA.NewClientCertKeyPair()
	if err != nil {
		panic(err)
	}

	unknownClientCert, err := parseCertificateFromBytes(unknownClientPair.Cert)
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
		ClientTlsCert: clientPair.Cert,
		ServerTlsCert: serverPair.Cert,
	}

	rootCertPool := x509.NewCertPool()
	rootCertPool.AddCert(caRootCert)
	goodVerifyingOpts := x509.VerifyOptions{
		Roots: rootCertPool,
		KeyUsages: []x509.ExtKeyUsage{
			x509.ExtKeyUsageClientAuth,
			x509.ExtKeyUsageServerAuth,
		},
	}

	// valid metadata should give nil error
	goodMetadata := &etcdraftproto.ConfigMetadata{
		Options: validOptions,
		Consenters: []*etcdraftproto.Consenter{
			singleConsenter,
		},
	}
	assert.Nil(t, VerifyConfigMetadata(goodMetadata, goodVerifyingOpts))

	// test variety of bad metadata
	for _, testCase := range []struct {
		description string
		metadata    *etcdraftproto.ConfigMetadata
		verifyOpts  x509.VerifyOptions
		errRegex    string
	}{
		{
			description: "nil metadata",
			metadata:    nil,
			errRegex:    "nil Raft config metadata",
			verifyOpts:  goodVerifyingOpts,
		},
		{
			description: "nil options",
			metadata:    &etcdraftproto.ConfigMetadata{},
			verifyOpts:  goodVerifyingOpts,
			errRegex:    "nil Raft config metadata options",
		},
		{
			description: "HeartbeatTick is 0",
			metadata: &etcdraftproto.ConfigMetadata{
				Options: &etcdraftproto.Options{
					HeartbeatTick: 0,
				},
			},
			verifyOpts: goodVerifyingOpts,
			errRegex:   "none of HeartbeatTick .* can be zero",
		},
		{
			description: "ElectionTick is 0",
			metadata: &etcdraftproto.ConfigMetadata{
				Options: &etcdraftproto.Options{
					HeartbeatTick: validOptions.HeartbeatTick,
					ElectionTick:  0,
				},
			},
			verifyOpts: goodVerifyingOpts,
			errRegex:   "none of .* ElectionTick .* can be zero",
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
			verifyOpts: goodVerifyingOpts,
			errRegex:   "none of .* MaxInflightBlocks .* can be zero",
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
			verifyOpts: goodVerifyingOpts,
			errRegex:   "ElectionTick .* must be greater than HeartbeatTick",
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
			verifyOpts: goodVerifyingOpts,
			errRegex:   "failed to parse TickInterval .* to time duration",
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
			verifyOpts: goodVerifyingOpts,
			errRegex:   "TickInterval cannot be zero",
		},
		{
			description: "consenter set is empty",
			metadata: &etcdraftproto.ConfigMetadata{
				Options:    validOptions,
				Consenters: []*etcdraftproto.Consenter{},
			},
			verifyOpts: goodVerifyingOpts,
			errRegex:   "empty consenter set",
		},
		{
			description: "metadata has nil consenter",
			metadata: &etcdraftproto.ConfigMetadata{
				Options: validOptions,
				Consenters: []*etcdraftproto.Consenter{
					nil,
				},
			},
			verifyOpts: goodVerifyingOpts,
			errRegex:   "metadata has nil consenter",
		},
		{
			description: "consenter has invalid server cert",
			metadata: &etcdraftproto.ConfigMetadata{
				Options: validOptions,
				Consenters: []*etcdraftproto.Consenter{
					{
						ServerTlsCert: []byte("invalid"),
						ClientTlsCert: clientPair.Cert,
					},
				},
			},
			verifyOpts: goodVerifyingOpts,
			errRegex:   "no PEM data found in cert",
		},
		{
			description: "consenter has invalid client cert",
			metadata: &etcdraftproto.ConfigMetadata{
				Options: validOptions,
				Consenters: []*etcdraftproto.Consenter{
					{
						ServerTlsCert: serverPair.Cert,
						ClientTlsCert: []byte("invalid"),
					},
				},
			},
			verifyOpts: goodVerifyingOpts,
			errRegex:   "no PEM data found in cert",
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
			verifyOpts: goodVerifyingOpts,
			errRegex:   "duplicate consenter",
		},
		{
			description: "consenter has client cert signed by unknown authority",
			metadata: &etcdraftproto.ConfigMetadata{
				Options: validOptions,
				Consenters: []*etcdraftproto.Consenter{
					{
						ClientTlsCert: unknownClientPair.Cert,
						ServerTlsCert: serverPair.Cert,
					},
				},
			},
			verifyOpts: goodVerifyingOpts,
			errRegex:   fmt.Sprintf("verifying tls client cert with serial number %d: x509: certificate signed by unknown authority", unknownClientCert.SerialNumber),
		},
		{
			description: "consenter has server cert signed by unknown authority",
			metadata: &etcdraftproto.ConfigMetadata{
				Options: validOptions,
				Consenters: []*etcdraftproto.Consenter{
					{
						ServerTlsCert: unknownServerPair.Cert,
						ClientTlsCert: clientPair.Cert,
					},
				},
			},
			verifyOpts: goodVerifyingOpts,
			errRegex:   fmt.Sprintf("verifying tls server cert with serial number %d: x509: certificate signed by unknown authority", unknownServerCert.SerialNumber),
		},
	} {
		t.Run(testCase.description, func(t *testing.T) {
			err := VerifyConfigMetadata(testCase.metadata, testCase.verifyOpts)
			require.NotNil(t, err, testCase.description)
			require.Regexp(t, testCase.errRegex, err)
		})
	}

	// test use case when consenter has expired certificates
	tlsCaCertBytes, err := ioutil.ReadFile(filepath.Join(ca1Dir, "ca.pem"))
	require.Nil(t, err)
	tlsCaCert, err := parseCertificateFromBytes(tlsCaCertBytes)
	require.Nil(t, err)

	tlsClientCert, err := ioutil.ReadFile(filepath.Join(ca1Dir, "client3.pem"))
	require.Nil(t, err)

	expiredCertVerifyOpts := goodVerifyingOpts
	expiredCertVerifyOpts.Roots.AddCert(tlsCaCert)
	consenterWithExpiredCerts := &etcdraftproto.Consenter{
		Host:          "host1",
		Port:          10001,
		ClientTlsCert: tlsClientCert,
		ServerTlsCert: tlsClientCert,
	}

	metadataWithExpiredConsenter := &etcdraftproto.ConfigMetadata{
		Options: validOptions,
		Consenters: []*etcdraftproto.Consenter{
			consenterWithExpiredCerts,
		},
	}

	require.Nil(t, VerifyConfigMetadata(metadataWithExpiredConsenter, expiredCertVerifyOpts))
}
