/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package aries

import (
	"github.com/IBM/idemix/bccsp/types"
	math "github.com/IBM/mathlib"
	"github.com/ale-linux/aries-framework-go/component/kmscrypto/crypto/primitive/bbs12381g2pub"
)

func attributesToSignatureMessage(sk *math.Zr, attributes []types.IdemixAttribute, curve *math.Curve) []*bbs12381g2pub.SignatureMessage {
	var msgsZr []*bbs12381g2pub.SignatureMessage

	if sk == nil {
		msgsZr = make([]*bbs12381g2pub.SignatureMessage, 0, len(attributes))
	} else {
		msgsZr = make([]*bbs12381g2pub.SignatureMessage, 1, len(attributes)+1)
		msgsZr[UserSecretKeyIndex] = &bbs12381g2pub.SignatureMessage{
			FR:  sk,
			Idx: UserSecretKeyIndex,
		}
	}

	for i, msg := range attributes {
		switch msg.Type {
		case types.IdemixBytesAttribute:
			msgsZr = append(msgsZr, &bbs12381g2pub.SignatureMessage{
				FR:  bbs12381g2pub.FrFromOKM(msg.Value.([]byte)),
				Idx: i + 1,
			})
		case types.IdemixIntAttribute:
			msgsZr = append(msgsZr, &bbs12381g2pub.SignatureMessage{
				FR:  curve.NewZrFromInt(int64(msg.Value.(int))),
				Idx: i + 1,
			})
		case types.IdemixHiddenAttribute:
			continue
		}
	}

	return msgsZr
}

func revealedAttributesIndex(attributes []types.IdemixAttribute) []int {
	revealed := make([]int, 0, len(attributes))

	for i, msg := range attributes {
		if msg.Type != types.IdemixHiddenAttribute {
			revealed = append(revealed, i+1)
		}
	}

	return revealed
}

func (c *Credential) toSignatureMessage(sk *math.Zr, curve *math.Curve) []*bbs12381g2pub.SignatureMessage {
	var msgsZr []*bbs12381g2pub.SignatureMessage

	if sk == nil {
		msgsZr = make([]*bbs12381g2pub.SignatureMessage, 0, len(c.Attrs))
	} else {
		msgsZr = make([]*bbs12381g2pub.SignatureMessage, 1, len(c.Attrs)+1)
		msgsZr[UserSecretKeyIndex] = &bbs12381g2pub.SignatureMessage{
			FR:  sk,
			Idx: UserSecretKeyIndex,
		}
	}

	for i, msg := range c.Attrs {
		msgsZr = append(msgsZr, &bbs12381g2pub.SignatureMessage{
			FR:  curve.NewZrFromBytes(msg),
			Idx: i + 1,
		})
	}

	return msgsZr
}
