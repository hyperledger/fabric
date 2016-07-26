package primitives

import (
	"crypto/ecdsa"
	"crypto/rand"
	"encoding/asn1"
	"fmt"
	"math/big"
	"os"
	"reflect"
	"testing"
)

type TestParameters struct {
	hashFamily    string
	securityLevel int
}

func (t *TestParameters) String() string {
	return t.hashFamily + "-" + string(t.securityLevel)
}

var testParametersSet = []*TestParameters{
	&TestParameters{"SHA3", 256},
	&TestParameters{"SHA3", 384},
	&TestParameters{"SHA2", 256},
	&TestParameters{"SHA2", 384}}

func TestMain(m *testing.M) {
	for _, params := range testParametersSet {
		err := InitSecurityLevel(params.hashFamily, params.securityLevel)
		if err == nil {
			m.Run()
		} else {
			panic(fmt.Errorf("Failed initiliazing crypto layer at [%s]", params.String()))
		}

		err = SetSecurityLevel(params.hashFamily, params.securityLevel)
		if err == nil {
			m.Run()
		} else {
			panic(fmt.Errorf("Failed initiliazing crypto layer at [%s]", params.String()))
		}

		if GetHashAlgorithm() != params.hashFamily {
			panic(fmt.Errorf("Failed initiliazing crypto layer. Invalid Hash family [%s][%s]", GetHashAlgorithm(), params.hashFamily))
		}

	}
	os.Exit(0)
}

func TestInitSecurityLevel(t *testing.T) {
	err := SetSecurityLevel("SHA2", 1024)
	if err == nil {
		t.Fatalf("Initialization should fail")
	}

	err = SetSecurityLevel("SHA", 1024)
	if err == nil {
		t.Fatalf("Initialization should fail")
	}

	err = SetSecurityLevel("SHA3", 2048)
	if err == nil {
		t.Fatalf("Initialization should fail")
	}
}

func TestECDSA(t *testing.T) {

	key, err := NewECDSAKey()
	if err != nil {
		t.Fatalf("Failed generating ECDSA key [%s]", err)
	}

	for i := 1; i < 100; i++ {
		length, err := rand.Int(rand.Reader, big.NewInt(1024))
		if err != nil {
			t.Fatalf("Failed generating AES key [%s]", err)
		}
		msg, err := GetRandomBytes(int(length.Int64()) + 1)
		if err != nil {
			t.Fatalf("Failed generating AES key [%s]", err)
		}

		sigma, err := ECDSASign(key, msg)
		if err != nil {
			t.Fatalf("Failed signing [%s]", err)
		}

		ok, err := ECDSAVerify(key.Public(), msg, sigma)
		if err != nil {
			t.Fatalf("Failed verifying [%s]", err)
		}
		if !ok {
			t.Fatalf("Failed verification.")
		}

		ok, err = ECDSAVerify(key.Public(), msg[:len(msg)-1], sigma)
		if err != nil {
			t.Fatalf("Failed verifying [%s]", err)
		}
		if ok {
			t.Fatalf("Verification should fail.")
		}

		ok, err = ECDSAVerify(key.Public(), msg[:1], sigma[:1])
		if err != nil {
			t.Fatalf("Failed verifying [%s]", err)
		}
		if ok {
			t.Fatalf("Verification should fail.")
		}

		R, S, err := ECDSASignDirect(key, msg)
		if err != nil {
			t.Fatalf("Failed signing (direct) [%s]", err)
		}
		if sigma, err = asn1.Marshal(ECDSASignature{R, S}); err != nil {
			t.Fatalf("Failed marshalling (R,S) [%s]", err)
		}
		ok, err = ECDSAVerify(key.Public(), msg, sigma)
		if err != nil {
			t.Fatalf("Failed verifying [%s]", err)
		}
		if !ok {
			t.Fatalf("Failed verification.")
		}
	}
}

func TestECDSAKeys(t *testing.T) {
	key, err := NewECDSAKey()
	if err != nil {
		t.Fatalf("Failed generating ECDSA key [%s]", err)
	}

	// Private Key DER format
	der, err := PrivateKeyToDER(key)
	if err != nil {
		t.Fatalf("Failed converting private key to DER [%s]", err)
	}
	keyFromDER, err := DERToPrivateKey(der)
	if err != nil {
		t.Fatalf("Failed converting DER to private key [%s]", err)
	}
	ecdsaKeyFromDer := keyFromDER.(*ecdsa.PrivateKey)
	// TODO: check the curve
	if key.D.Cmp(ecdsaKeyFromDer.D) != 0 {
		t.Fatalf("Failed converting DER to private key. Invalid D.")
	}
	if key.X.Cmp(ecdsaKeyFromDer.X) != 0 {
		t.Fatalf("Failed converting DER to private key. Invalid X coordinate.")
	}
	if key.Y.Cmp(ecdsaKeyFromDer.Y) != 0 {
		t.Fatalf("Failed converting DER to private key. Invalid Y coordinate.")
	}

	// Private Key PEM format
	pem, err := PrivateKeyToPEM(key, nil)
	if err != nil {
		t.Fatalf("Failed converting private key to PEM [%s]", err)
	}
	keyFromPEM, err := PEMtoPrivateKey(pem, nil)
	if err != nil {
		t.Fatalf("Failed converting DER to private key [%s]", err)
	}
	ecdsaKeyFromPEM := keyFromPEM.(*ecdsa.PrivateKey)
	// TODO: check the curve
	if key.D.Cmp(ecdsaKeyFromPEM.D) != 0 {
		t.Fatalf("Failed converting PEM to private key. Invalid D.")
	}
	if key.X.Cmp(ecdsaKeyFromPEM.X) != 0 {
		t.Fatalf("Failed converting PEM to private key. Invalid X coordinate.")
	}
	if key.Y.Cmp(ecdsaKeyFromPEM.Y) != 0 {
		t.Fatalf("Failed converting PEM to private key. Invalid Y coordinate.")
	}

	// Nil Private Key <-> PEM
	_, err = PrivateKeyToPEM(nil, nil)
	if err == nil {
		t.Fatalf("PublicKeyToPEM should fail on nil")
	}

	_, err = PEMtoPrivateKey(nil, nil)
	if err == nil {
		t.Fatalf("PEMtoPublicKey should fail on nil")
	}

	_, err = PEMtoPrivateKey([]byte{0, 1, 3, 4}, nil)
	if err == nil {
		t.Fatalf("PEMtoPublicKey should fail invalid PEM")
	}

	_, err = DERToPrivateKey(nil)
	if err == nil {
		t.Fatalf("DERToPrivateKey should fail on nil")
	}

	_, err = DERToPrivateKey([]byte{0, 1, 3, 4})
	if err == nil {
		t.Fatalf("DERToPrivateKey should fail on invalid DER")
	}

	_, err = PrivateKeyToDER(nil)
	if err == nil {
		t.Fatalf("DERToPrivateKey should fail on nil")
	}

	// Private Key Encrypted PEM format
	encPEM, err := PrivateKeyToPEM(key, []byte("passwd"))
	if err != nil {
		t.Fatalf("Failed converting private key to encrypted PEM [%s]", err)
	}
	encKeyFromPEM, err := PEMtoPrivateKey(encPEM, []byte("passwd"))
	if err != nil {
		t.Fatalf("Failed converting DER to private key [%s]", err)
	}
	ecdsaKeyFromEncPEM := encKeyFromPEM.(*ecdsa.PrivateKey)
	// TODO: check the curve
	if key.D.Cmp(ecdsaKeyFromEncPEM.D) != 0 {
		t.Fatalf("Failed converting encrypted PEM to private key. Invalid D.")
	}
	if key.X.Cmp(ecdsaKeyFromEncPEM.X) != 0 {
		t.Fatalf("Failed converting encrypted PEM to private key. Invalid X coordinate.")
	}
	if key.Y.Cmp(ecdsaKeyFromEncPEM.Y) != 0 {
		t.Fatalf("Failed converting encrypted PEM to private key. Invalid Y coordinate.")
	}

	// Public Key PEM format
	pem, err = PublicKeyToPEM(&key.PublicKey, nil)
	if err != nil {
		t.Fatalf("Failed converting public key to PEM [%s]", err)
	}
	keyFromPEM, err = PEMtoPublicKey(pem, nil)
	if err != nil {
		t.Fatalf("Failed converting DER to public key [%s]", err)
	}
	ecdsaPkFromPEM := keyFromPEM.(*ecdsa.PublicKey)
	// TODO: check the curve
	if key.X.Cmp(ecdsaPkFromPEM.X) != 0 {
		t.Fatalf("Failed converting PEM to private key. Invalid X coordinate.")
	}
	if key.Y.Cmp(ecdsaPkFromPEM.Y) != 0 {
		t.Fatalf("Failed converting PEM to private key. Invalid Y coordinate.")
	}

	// Nil Public Key <-> PEM
	_, err = PublicKeyToPEM(nil, nil)
	if err == nil {
		t.Fatalf("PublicKeyToPEM should fail on nil")
	}

	_, err = PEMtoPublicKey(nil, nil)
	if err == nil {
		t.Fatalf("PEMtoPublicKey should fail on nil")
	}

	_, err = PEMtoPublicKey([]byte{0, 1, 3, 4}, nil)
	if err == nil {
		t.Fatalf("PEMtoPublicKey should fail on invalid PEM")
	}

	// Public Key Encrypted PEM format
	encPEM, err = PublicKeyToPEM(&key.PublicKey, []byte("passwd"))
	if err != nil {
		t.Fatalf("Failed converting private key to encrypted PEM [%s]", err)
	}
	pkFromEncPEM, err := PEMtoPublicKey(encPEM, []byte("passwd"))
	if err != nil {
		t.Fatalf("Failed converting DER to private key [%s]", err)
	}
	ecdsaPkFromEncPEM := pkFromEncPEM.(*ecdsa.PublicKey)
	// TODO: check the curve
	if key.X.Cmp(ecdsaPkFromEncPEM.X) != 0 {
		t.Fatalf("Failed converting encrypted PEM to private key. Invalid X coordinate.")
	}
	if key.Y.Cmp(ecdsaPkFromEncPEM.Y) != 0 {
		t.Fatalf("Failed converting encrypted PEM to private key. Invalid Y coordinate.")
	}

	_, err = PEMtoPublicKey(encPEM, []byte("passw"))
	if err == nil {
		t.Fatalf("PEMtoPublicKey should fail on wrong password")
	}

	_, err = PEMtoPublicKey(encPEM, []byte("passw"))
	if err == nil {
		t.Fatalf("PEMtoPublicKey should fail on nil password")
	}

	_, err = PEMtoPublicKey(nil, []byte("passwd"))
	if err == nil {
		t.Fatalf("PEMtoPublicKey should fail on nil PEM")
	}

	_, err = PEMtoPublicKey([]byte{0, 1, 3, 4}, []byte("passwd"))
	if err == nil {
		t.Fatalf("PEMtoPublicKey should fail on invalid PEM")
	}

	_, err = PEMtoPublicKey(nil, []byte("passw"))
	if err == nil {
		t.Fatalf("PEMtoPublicKey should fail on nil PEM and wrong password")
	}
}

func TestRandom(t *testing.T) {
	nonce, err := GetRandomNonce()
	if err != nil {
		t.Fatalf("Failed getting nonce [%s]", err)
	}

	if len(nonce) != NonceSize {
		t.Fatalf("Invalid nonce size. Expecting [%d], was [%d]", NonceSize, len(nonce))
	}
}

func TestHMAC(t *testing.T) {
	key, err := GenAESKey()
	if err != nil {
		t.Fatalf("Failed generating AES key [%s]", err)
	}

	for i := 1; i < 100; i++ {
		len, err := rand.Int(rand.Reader, big.NewInt(1024))
		if err != nil {
			t.Fatalf("Failed generating AES key [%s]", err)
		}
		msg, err := GetRandomBytes(int(len.Int64()) + 1)
		if err != nil {
			t.Fatalf("Failed generating AES key [%s]", err)
		}

		out1 := HMACAESTruncated(key, msg)
		out2 := HMACTruncated(key, msg, AESKeyLength)
		out3 := HMAC(key, msg)

		if !reflect.DeepEqual(out1, out2) {
			t.Fatalf("Wrong hmac output [%x][%x]", out1, out2)
		}
		if !reflect.DeepEqual(out2, out3[:AESKeyLength]) {
			t.Fatalf("Wrong hmac output [%x][%x]", out1, out2)
		}

	}

}

func TestX509(t *testing.T) {

	// Generate a self signed cert
	der, key, err := NewSelfSignedCert()
	if err != nil {
		t.Fatalf("Failed genereting self signed cert")
	}

	// Test DERCertToPEM
	pem := DERCertToPEM(der)
	certFromPEM, derFromPem, err := PEMtoCertificateAndDER(pem)
	if err != nil {
		t.Fatalf("Failed converting PEM to (x509, DER) [%s]", err)
	}
	if !reflect.DeepEqual(certFromPEM.Raw, der) {
		t.Fatalf("Invalid der from PEM [%x][%x]", der, certFromPEM.Raw)
	}
	if !reflect.DeepEqual(der, derFromPem) {
		t.Fatalf("Invalid der from PEM [%x][%x]", der, derFromPem)
	}

	if err := CheckCertPKAgainstSK(certFromPEM, key); err != nil {
		t.Fatalf("Failed checking cert vk against sk [%s]", err)
	}

	// Test PEMtoDER
	if derFromPem, err = PEMtoDER(pem); err != nil {
		t.Fatalf("Failed converting PEM to (DER) [%s]", err)
	}
	if !reflect.DeepEqual(der, derFromPem) {
		t.Fatalf("Invalid der from PEM [%x][%x]", der, derFromPem)
	}

	// Test PEMtoCertificate
	if certFromPEM, err = PEMtoCertificate(pem); err != nil {
		t.Fatalf("Failed converting PEM to (x509) [%s]", err)
	}
	if !reflect.DeepEqual(certFromPEM.Raw, der) {
		t.Fatalf("Invalid der from PEM [%x][%x]", der, certFromPEM.Raw)
	}
	if err := CheckCertPKAgainstSK(certFromPEM, key); err != nil {
		t.Fatalf("Failed checking cert vk against sk [%s]", err)
	}

	// Test DERToX509Certificate
	if certFromPEM, err = DERToX509Certificate(der); err != nil {
		t.Fatalf("Failed converting DER to (x509) [%s]", err)
	}
	if !reflect.DeepEqual(certFromPEM.Raw, der) {
		t.Fatalf("Invalid x509 from PEM [%x][%x]", der, certFromPEM.Raw)
	}
	if err := CheckCertPKAgainstSK(certFromPEM, key); err != nil {
		t.Fatalf("Failed checking cert vk against sk [%s]", err)
	}

	// Test errors
	if _, err = DERToX509Certificate(pem); err == nil {
		t.Fatalf("Converting DER to (x509) should fail on PEM [%s]", err)
	}

	if _, err = PEMtoCertificate(der); err == nil {
		t.Fatalf("Converting PEM to (x509) should fail on DER [%s]", err)
	}

	certFromPEM.PublicKey = nil
	if err := CheckCertPKAgainstSK(certFromPEM, key); err == nil {
		t.Fatalf("Checking cert vk against sk shoud failed. Invalid VK [%s]", err)
	}
}
