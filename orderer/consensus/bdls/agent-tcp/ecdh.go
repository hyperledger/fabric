
package agent

import (
	"crypto/ecdsa"
	"math/big"
)
 
func ECDH(publicKey *ecdsa.PublicKey, key *ecdsa.PrivateKey) *big.Int {
	secret, _ := key.Curve.ScalarMult(publicKey.X, publicKey.Y, key.D.Bytes())
	return secret
}
