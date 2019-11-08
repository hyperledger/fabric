/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package crypto

import (
	"crypto/x509"
	"encoding/pem"
	"time"

	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric/protos/msp"
)

// ExpiresAt returns when the given identity expires, or a zero time.Time
// in case we cannot determine that
func ExpiresAt(identityBytes []byte) time.Time {
	sId := &msp.SerializedIdentity{}
	// If protobuf parsing failed, we make no decisions about the expiration time
	if err := proto.Unmarshal(identityBytes, sId); err != nil {
		return time.Time{}
	}
	return certExpirationTime(sId.IdBytes)
}

func certExpirationTime(pemBytes []byte) time.Time {
	bl, _ := pem.Decode(pemBytes)
	if bl == nil {
		// If the identity isn't a PEM block, we make no decisions about the expiration time
		return time.Time{}
	}
	cert, err := x509.ParseCertificate(bl.Bytes)
	if err != nil {
		return time.Time{}
	}
	return cert.NotAfter
}

// WarnFunc notifies a warning happened with the given format, and can be replaced with Warnf of a logger.
type WarnFunc func(format string, args ...interface{})

// Scheduler invokes f after d time, and can be replaced with time.AfterFunc.
type Scheduler func(d time.Duration, f func()) *time.Timer

// TrackExpiration warns a week before one of the certificates expires
func TrackExpiration(tls bool, serverCert []byte, clientCertChain [][]byte, sIDBytes []byte, warn WarnFunc, now time.Time, s Scheduler) {
	sID := &msp.SerializedIdentity{}
	if err := proto.Unmarshal(sIDBytes, sID); err != nil {
		return
	}

	trackCertExpiration(sID.IdBytes, "enrollment", warn, now, s)

	if !tls {
		return
	}

	trackCertExpiration(serverCert, "server TLS", warn, now, s)

	if len(clientCertChain) == 0 || len(clientCertChain[0]) == 0 {
		return
	}

	trackCertExpiration(clientCertChain[0], "client TLS", warn, now, s)
}

func trackCertExpiration(rawCert []byte, certRole string, warn WarnFunc, now time.Time, sched Scheduler) {
	expirationTime := certExpirationTime(rawCert)
	if expirationTime.IsZero() {
		// If the certificate expiration time cannot be classified, return.
		return
	}

	timeLeftUntilExpiration := expirationTime.Sub(now)
	oneWeek := time.Hour * 24 * 7

	if timeLeftUntilExpiration < 0 {
		warn("The %s certificate has expired", certRole)
		return
	}

	if timeLeftUntilExpiration < oneWeek {
		days := timeLeftUntilExpiration / (time.Hour * 24)
		hours := (timeLeftUntilExpiration - (days * time.Hour * 24)) / time.Hour
		warn("The %s certificate expires within %d days and %d hours", certRole, days, hours)
		return
	}

	timeLeftUntilOneWeekBeforeExpiration := timeLeftUntilExpiration - oneWeek

	sched(timeLeftUntilOneWeekBeforeExpiration, func() {
		warn("The %s certificate will expire within one week", certRole)
	})
}
