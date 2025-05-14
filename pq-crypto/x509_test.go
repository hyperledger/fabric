package oqs

import (
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"github.com/stretchr/testify/require"
	"io"
	"testing"
)

// implements crypto.Signer
type mockSigner struct {
	sk SecretKey
}

func (ms *mockSigner) Sign(rand io.Reader, digest []byte, opts crypto.SignerOpts) ([]byte, error) {
	return Sign(ms.sk, digest)
}

func (ms *mockSigner) Public() (crypto.PublicKey) {
	return ms.sk.Pk
}

func TestMarshalPKIXPublicKeySuccess(t *testing.T) {
	pk, _, err := KeyPair("DEFAULT")
	require.NoError(t, err)
	_, err = MarshalPKIXPublicKey(&pk)
	require.NoError(t, err)
}

func TestMarshalPKIXPublicKeyError(t *testing.T) {
	// An incorrect keytype passed should compile,
	// but return an error.
	ecdsaK := ecdsa.PublicKey{}
	_, err := MarshalPKIXPublicKey(&ecdsaK)
	require.Error(t, err)
	require.Contains(t, err.Error(), "not a known OQS key type")

	// A correct keytype with an unknown algorithm
	// should return an error.
	pk, _, err := KeyPair("DEFAULT")
	require.NoError(t, err)
	pk.Sig.Algorithm = "I am not a real OQS Algorithm"
	_, err = MarshalPKIXPublicKey(&pk)
	require.Error(t, err)
	require.Contains(t, err.Error(), "unknown OQS algorithm name")
}

func TestParsePKIXPublicKeySuccess(t *testing.T) {
	l, err := GetLib()
	require.NoError(t, err)
	for _, sigAlg := range(l.EnabledSigs()) {
		t.Run(string(sigAlg), func(t *testing.T) {
			if string(sigAlg) == "DEFAULT" {
				return
			}
			// Make up some nice key material
			pk, _, err := KeyPair(sigAlg)
			require.NoError(t, err)
			derBytes, err := MarshalPKIXPublicKey(&pk)
			require.NoError(t, err)
			key, err := ParsePKIXPublicKey(derBytes)
			require.NoError(t, err)
			oqsKey, ok := key.(*PublicKey)
			require.True(t, ok)
			require.Equal(t, oqsKey.Pk, pk.Pk)
			// require.Equal *can* compare SigType objects, but it gives much more useful error messages
			// for comparing string types.
			require.Equal(t, string(oqsKey.Sig.Algorithm), string(pk.Sig.Algorithm))
		})
	}
}

var pemRSAPublicKey = `-----BEGIN PUBLIC KEY-----
MIIBIjANBgkqhkiG9w0BAQEFAAOCAQ8AMIIBCgKCAQEA3VoPN9PKUjKFLMwOge6+
wnDi8sbETGIx2FKXGgqtAKpzmem53kRGEQg8WeqRmp12wgp74TGpkEXsGae7RS1k
enJCnma4fii+noGH7R0qKgHvPrI2Bwa9hzsH8tHxpyM3qrXslOmD45EH9SxIDUBJ
FehNdaPbLP1gFyahKMsdfxFJLUvbUycuZSJ2ZnIgeVxwm4qbSvZInL9Iu4FzuPtg
fINKcbbovy1qq4KvPIrXzhbY3PWDc6btxCf3SE0JdE1MCPThntB62/bLMSQ7xdDR
FF53oIpvxe/SCOymfWq/LW849Ytv3Xwod0+wzAP8STXG4HSELS4UedPYeHJJJYcZ
+QIDAQAB
-----END PUBLIC KEY-----
`
func TestParsePKIXPublicKeyError(t *testing.T) {
	block, _ := pem.Decode([]byte(pemRSAPublicKey))
	_, err := x509.ParsePKIXPublicKey(block.Bytes)
	require.NoError(t, err)
	// The non-OQS key should return an error from the oqs parse function:
	// use the normal x509 functions for classical crypto, which should have worked above.
	_, err = ParsePKIXPublicKey(block.Bytes)
	require.Error(t, err)
	require.Contains(t, err.Error(), "unknown OQS public key algorithm")
}

func TestMarshalPKIXPrivateKeySuccess(t *testing.T) {
	_, sk, err := KeyPair("DEFAULT")
	require.NoError(t, err)
	_, err = MarshalPKIXPrivateKey(&sk)
	require.NoError(t, err)
}

func TestMarshalPKIXSecretKeyError(t *testing.T) {
	// An incorrect keytype passed should compile,
	// but return an error.
	ecdsaK := ecdsa.PrivateKey{}
	_, err := MarshalPKIXPrivateKey(&ecdsaK)
	require.Error(t, err)
	require.Contains(t, err.Error(), "not a known OQS key type")

	// A correct keytype with an unknown algorithm
	// should return an error.
	_, sk, err := KeyPair("DEFAULT")
	require.NoError(t, err)
	sk.Sig.Algorithm = "I am not a real OQS Algorithm"
	_, err = MarshalPKIXPrivateKey(&sk)
	require.Error(t, err)
	require.Contains(t, err.Error(), "unknown OQS algorithm name")
}

func TestParsePKIXPrivateKeySuccess(t *testing.T) {
	l, err := GetLib()
	require.NoError(t, err)
	for _, sigAlg := range(l.EnabledSigs()) {
		t.Run(string(sigAlg), func(t *testing.T) {
			if string(sigAlg) == "DEFAULT" {
				return
			}
			// Make up some nice key material
			_, sk, err := KeyPair(sigAlg)
			require.NoError(t, err)
			derBytes, err := MarshalPKIXPrivateKey(&sk)
			require.NoError(t, err)
			key, err := ParsePKIXPrivateKey(derBytes)
			require.NoError(t, err)
			oqsKey, ok := key.(*SecretKey)
			require.True(t, ok)
			require.Equal(t, oqsKey.Sk, sk.Sk)
			require.Equal(t, oqsKey.Pk, sk.Pk)
			// require.Equal *can* compare SigType objects, but it gives much more useful error messages
			// for comparing string types.
			require.Equal(t, string(oqsKey.Sig.Algorithm), string(sk.Sig.Algorithm))
		})
	}
}

func TestBuildAltPublicKeyInfoExtensionsSuccess(t *testing.T) {
	// Make up some nice key material
	pk, _, err := KeyPair("DEFAULT")
	require.NoError(t, err)
	_, sk, err := KeyPair("DEFAULT")
	require.NoError(t, err)
	ms := mockSigner{sk: sk}
	ck, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)

	// Turn it into extensions
	_, err = BuildAltPublicKeyExtensions(&pk, ck.Public(), &ms)
	require.NoError(t, err)


	// It's okay to supply no postquantum key for this cert,
	// as long as there is a pq signing key from the issuer
	var empty *PublicKey = nil
	_, err = BuildAltPublicKeyExtensions(empty, ck.Public(), &ms)
	require.NoError(t, err)
}

func TestBuildAltPublicKeyInfoExtensionsError(t *testing.T) {
	// Make up some nice key material
	pk, _, err := KeyPair("DEFAULT")
	require.NoError(t, err)
	_, sk, err := KeyPair("DEFAULT")
	ms := mockSigner{sk: sk}
	require.NoError(t, err)

	// An incorrect keytype passed should compile,
	// but return an error.
	ecdsaK := ecdsa.PublicKey{}
	_, err = BuildAltPublicKeyExtensions(&ecdsaK, &ecdsaK, &ms)
	require.Error(t, err)
	require.Contains(t, err.Error(), "not a known OQS key type")

	// A correct keytype with an unknown algorithm
	// should return an error.
	pk.Sig.Algorithm = "I am not a real OQS Algorithm"
	_, err = BuildAltPublicKeyExtensions(&pk, &ecdsaK, &ms)
	require.Error(t, err)
	require.Contains(t, err.Error(), "unknown OQS algorithm name")
}

func TestParseSubjectAltPublicKeyInfoExtension(t *testing.T) {
	l, err := GetLib()
	require.NoError(t, err)
	for _, sigAlg := range(l.EnabledSigs()) {
		t.Run(string(sigAlg), func(t *testing.T) {

			// generate some nice key material
			pk1, _, err := KeyPair(sigAlg)
			require.NoError(t, err)
			pk2, sk2, err := KeyPair(sigAlg)
			require.NoError(t, err)
			ms := mockSigner{sk: sk2}
			ck, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
			require.NoError(t, err)

			extensions, err := BuildAltPublicKeyExtensions(&pk1, ck.Public(), &ms)
			require.NoError(t, err)

			// First, let's check that we can get the correct quantum public key for the cert subject
			key, err := ParseSubjectAltPublicKeyInfoExtension(extensions)
			require.NotNil(t, key)
			require.NoError(t, err)
			require.Equal(t, key.Pk, pk1.Pk)
			// require.Equal *can* compare SigType objects, but it gives much more useful error messages
			// for comparing string types.
			require.Equal(t, string(key.Sig.Algorithm), string(pk1.Sig.Algorithm))

			// Next, we'll check that the signature matches
			ckBytes, err := x509.MarshalPKIXPublicKey(ck.Public())
			require.NoError(t, err)
			km := buildKmMessage(pk1.Pk, ckBytes)
			sig, err := parseAltSignatureValueExtension(extensions)
			require.NoError(t, err)
			isValid, err := Verify(pk2, sig, km)
			require.NoError(t, err)
			require.True(t, isValid)
		})
	}

	// In the case that extensions do not contain an alternate public key,
	// the parser should return a nil key without raising an error.
	extensions := []pkix.Extension{
		{
			Id:    []int{1, 2, 3, 4},
			Value: []byte("some other extension"),
		},
	}
	key, err := ParseSubjectAltPublicKeyInfoExtension(extensions)
	require.NoError(t, err)
	require.Nil(t, key)
}

func TestParseSubjectAltPublicKeyInfoExtensionError(t *testing.T) {
	// generate some nice key material
	pk, _, err := KeyPair("DEFAULT")
	require.NoError(t, err)
	_, sk, err := KeyPair("DEFAULT")
	require.NoError(t, err)
	ms := mockSigner{sk: sk}
	ck, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)

	extensions1, err := BuildAltPublicKeyExtensions(&pk, ck.Public(), &ms)
	require.NoError(t, err)
	extensions2, err := BuildAltPublicKeyExtensions(&pk, ck.Public(), &ms)
	require.NoError(t, err)

	// This should give us an error because we expect only one of each AltPublicKey extension
	extensions := append(extensions1, extensions2...)
	_, err = ParseSubjectAltPublicKeyInfoExtension(extensions)
	require.Error(t, err)
	_, err = parseAltSignatureValueExtension(extensions)
	require.Error(t, err)
}
