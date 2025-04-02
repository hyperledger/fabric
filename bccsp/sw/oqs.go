package sw

import (
	"github.com/hyperledger/fabric/bccsp"

	oqs "github.com/hyperledger/fabric/pq-crypto"
)

func signOQS(k *oqs.SecretKey, digest []byte, opts bccsp.SignerOpts) ([]byte, error) {
	return oqs.Sign(*k, digest)
}

func verifyOQS(k *oqs.PublicKey, signature, digest []byte, opts bccsp.SignerOpts) (bool, error) {
	return oqs.Verify(*k, signature, digest)
}

type oqsSigner struct{}

func (s *oqsSigner) Sign(k bccsp.Key, digest []byte, opts bccsp.SignerOpts) ([]byte, error) {
	return signOQS(k.(*oqsPrivateKey).privKey, digest, opts)
}

type oqsPrivateKeyVerifier struct{}

func (v *oqsPrivateKeyVerifier) Verify(k bccsp.Key, signature, digest []byte, opts bccsp.SignerOpts) (bool, error) {
	return verifyOQS(&(k.(*oqsPrivateKey).privKey.PublicKey), signature, digest, opts)
}

type oqsPublicKeyKeyVerifier struct{}

func (v *oqsPublicKeyKeyVerifier) Verify(k bccsp.Key, signature, digest []byte, opts bccsp.SignerOpts) (bool, error) {
	return verifyOQS(k.(*oqsPublicKey).pubKey, signature, digest, opts)
}
