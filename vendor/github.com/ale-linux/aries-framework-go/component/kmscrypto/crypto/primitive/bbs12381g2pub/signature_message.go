/*
Copyright SecureKey Technologies Inc. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package bbs12381g2pub

import (
	ml "github.com/IBM/mathlib"
)

// SignatureMessage defines a message to be used for a signature check.
type SignatureMessage struct {
	FR  *ml.Zr
	Idx int
}

// ParseSignatureMessage parses SignatureMessage from bytes.
func ParseSignatureMessage(message []byte, idx int) *SignatureMessage {
	elm := FrFromOKM(message)

	return &SignatureMessage{
		FR:  elm,
		Idx: idx,
	}
}
