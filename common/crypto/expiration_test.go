/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package crypto_test

import (
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"io/ioutil"
	"path/filepath"
	"testing"
	"time"

	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric/common/crypto"
	"github.com/hyperledger/fabric/common/crypto/tlsgen"
	"github.com/hyperledger/fabric/protos/msp"
	"github.com/hyperledger/fabric/protos/utils"
	"github.com/stretchr/testify/assert"
)

func TestX509CertExpiresAt(t *testing.T) {
	certBytes, err := ioutil.ReadFile(filepath.Join("testdata", "cert.pem"))
	assert.NoError(t, err)
	sId := &msp.SerializedIdentity{
		IdBytes: certBytes,
	}
	serializedIdentity, err := proto.Marshal(sId)
	assert.NoError(t, err)
	expirationTime := crypto.ExpiresAt(serializedIdentity)
	assert.Equal(t, time.Date(2027, 8, 17, 12, 19, 48, 0, time.UTC), expirationTime)
}

func TestX509InvalidCertExpiresAt(t *testing.T) {
	certBytes, err := ioutil.ReadFile(filepath.Join("testdata", "badCert.pem"))
	assert.NoError(t, err)
	sId := &msp.SerializedIdentity{
		IdBytes: certBytes,
	}
	serializedIdentity, err := proto.Marshal(sId)
	assert.NoError(t, err)
	expirationTime := crypto.ExpiresAt(serializedIdentity)
	assert.True(t, expirationTime.IsZero())
}

func TestIdemixIdentityExpiresAt(t *testing.T) {
	idemixId := &msp.SerializedIdemixIdentity{
		NymX: []byte{1, 2, 3},
		NymY: []byte{1, 2, 3},
		Ou:   []byte("OU1"),
	}
	idemixBytes, err := proto.Marshal(idemixId)
	assert.NoError(t, err)
	sId := &msp.SerializedIdentity{
		IdBytes: idemixBytes,
	}
	serializedIdentity, err := proto.Marshal(sId)
	assert.NoError(t, err)
	expirationTime := crypto.ExpiresAt(serializedIdentity)
	assert.True(t, expirationTime.IsZero())
}

func TestInvalidIdentityExpiresAt(t *testing.T) {
	expirationTime := crypto.ExpiresAt([]byte{1, 2, 3})
	assert.True(t, expirationTime.IsZero())
}

func TestTrackExpiration(t *testing.T) {
	ca, err := tlsgen.NewCA()
	assert.NoError(t, err)

	now := time.Now()
	bl, _ := pem.Decode(ca.CertBytes())
	caCert, err := x509.ParseCertificate(bl.Bytes)
	assert.NoError(t, err)
	expirationTime := caCert.NotAfter

	timeUntilExpiration := expirationTime.Sub(now)
	timeUntilOneMonthBeforeExpiration := timeUntilExpiration - 28*24*time.Hour
	timeUntil2DaysBeforeExpiration := timeUntilExpiration - 2*24*time.Hour - time.Hour*12

	monthBeforeExpiration := now.Add(timeUntilOneMonthBeforeExpiration)
	twoDaysBeforeExpiration := now.Add(timeUntil2DaysBeforeExpiration)

	tlsCert, err := ca.NewServerCertKeyPair("127.0.0.1")
	assert.NoError(t, err)

	signingIdentity := utils.MarshalOrPanic(&msp.SerializedIdentity{
		IdBytes: tlsCert.Cert,
	})

	shouldNotBeInvoked := func(format string, args ...interface{}) {
		t.Fatalf(format, args...)
	}

	var formattedWarning string
	shouldBeInvoked := func(format string, args ...interface{}) {
		formattedWarning = fmt.Sprintf(format, args...)
	}

	for _, testCase := range []struct {
		description     string
		tls             bool
		serverCert      []byte
		clientCertChain [][]byte
		sIDBytes        []byte
		warn            crypto.WarnFunc
		now             time.Time
		expectedWarn    string
	}{
		{
			description: "No TLS, enrollment cert isn't valid logs a warning",
			warn:        shouldNotBeInvoked,
			sIDBytes:    []byte{1, 2, 3},
		},
		{
			description:  "No TLS, enrollment cert expires soon",
			sIDBytes:     signingIdentity,
			warn:         shouldBeInvoked,
			now:          monthBeforeExpiration,
			expectedWarn: "The enrollment certificate will expire within one week",
		},
		{
			description:  "TLS, server cert expires soon",
			warn:         shouldBeInvoked,
			now:          monthBeforeExpiration,
			tls:          true,
			serverCert:   tlsCert.Cert,
			expectedWarn: "The server TLS certificate will expire within one week",
		},
		{
			description:  "TLS, server cert expires really soon",
			warn:         shouldBeInvoked,
			now:          twoDaysBeforeExpiration,
			tls:          true,
			serverCert:   tlsCert.Cert,
			expectedWarn: "The server TLS certificate expires within 2 days and 12 hours",
		},
		{
			description:  "TLS, server cert has expired",
			warn:         shouldBeInvoked,
			now:          expirationTime.Add(time.Hour),
			tls:          true,
			serverCert:   tlsCert.Cert,
			expectedWarn: "The server TLS certificate has expired",
		},
		{
			description:     "TLS, client cert expires soon",
			warn:            shouldBeInvoked,
			now:             monthBeforeExpiration,
			tls:             true,
			clientCertChain: [][]byte{tlsCert.Cert},
			expectedWarn:    "The client TLS certificate will expire within one week",
		},
	} {
		t.Run(testCase.description, func(t *testing.T) {
			defer func() {
				formattedWarning = ""
			}()

			fakeTimeAfter := func(duration time.Duration, f func()) *time.Timer {
				assert.NotEmpty(t, testCase.expectedWarn)
				threeWeeks := 3 * 7 * 24 * time.Hour
				assert.Equal(t, threeWeeks, duration)
				f()
				return nil
			}

			crypto.TrackExpiration(testCase.tls,
				testCase.serverCert,
				testCase.clientCertChain,
				testCase.sIDBytes,
				testCase.warn,
				testCase.now,
				fakeTimeAfter)

			if testCase.expectedWarn != "" {
				assert.Equal(t, testCase.expectedWarn, formattedWarning)
			} else {
				assert.Empty(t, formattedWarning)
			}
		})
	}
}
