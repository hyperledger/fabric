/*
Copyright IBM Corp. 2016 All Rights Reserved.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

		 http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package ca

import (
	"crypto/ecdsa"
	"crypto/rand"
	"crypto/x509"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/golang/protobuf/proto"
	"github.com/golang/protobuf/ptypes/timestamp"
	"github.com/hyperledger/fabric/core/crypto/primitives"
	"github.com/hyperledger/fabric/core/crypto/primitives/ecies"
	pb "github.com/hyperledger/fabric/membersrvc/protos"
	"golang.org/x/net/context"
)

type User struct {
	enrollID               string
	enrollPwd              []byte
	enrollPrivKey          *ecdsa.PrivateKey
	role                   int
	affiliation            string
	registrarRoles         []string
	registrarDelegateRoles []string
}

var (
	ecaFiles    = [6]string{"eca.cert", "eca.db", "eca.priv", "eca.pub", "obc.aes", "obc.ecies"}
	testAdmin   = User{enrollID: "admin", enrollPwd: []byte("Xurw3yU9zI0l")}
	testUser    = User{enrollID: "testUser", role: 1, affiliation: "institution_a"}
	testUser2   = User{enrollID: "testUser2", role: 1, affiliation: "institution_a"}
	testAuditor = User{enrollID: "testAuditor", role: 8}
	testClient1 = User{enrollID: "testClient1", role: 1, affiliation: "institution_a",
		registrarRoles: []string{"client"}, registrarDelegateRoles: []string{"client"}}
	testClient2 = User{enrollID: "testClient2", role: 1, affiliation: "institution_a",
		registrarRoles: []string{"client"}}
	testClient3 = User{enrollID: "testClient2", role: 1, affiliation: "institution_a",
		registrarRoles: []string{"client"}}
	testPeer = User{enrollID: "testPeer", role: 2, affiliation: "institution_a",
		registrarRoles: []string{"peer"}}
)

//helper function for multiple tests
func enrollUser(user *User) error {

	ecap := &ECAP{eca}

	// Phase 1 of the protocol: Generate crypto material
	signPriv, err := primitives.NewECDSAKey()
	user.enrollPrivKey = signPriv
	if err != nil {
		return err
	}
	signPub, err := x509.MarshalPKIXPublicKey(&signPriv.PublicKey)
	if err != nil {
		return err
	}

	encPriv, err := primitives.NewECDSAKey()
	if err != nil {
		return err
	}
	encPub, err := x509.MarshalPKIXPublicKey(&encPriv.PublicKey)
	if err != nil {
		return err
	}

	req := &pb.ECertCreateReq{
		Ts:   &timestamp.Timestamp{Seconds: time.Now().Unix(), Nanos: 0},
		Id:   &pb.Identity{Id: user.enrollID},
		Tok:  &pb.Token{Tok: user.enrollPwd},
		Sign: &pb.PublicKey{Type: pb.CryptoType_ECDSA, Key: signPub},
		Enc:  &pb.PublicKey{Type: pb.CryptoType_ECDSA, Key: encPub},
		Sig:  nil}

	resp, err := ecap.CreateCertificatePair(context.Background(), req)
	if err != nil {
		return err
	}

	//Phase 2 of the protocol
	spi := ecies.NewSPI()
	eciesKey, err := spi.NewPrivateKey(nil, encPriv)
	if err != nil {
		return err
	}

	ecies, err := spi.NewAsymmetricCipherFromPublicKey(eciesKey)
	if err != nil {
		return err
	}

	out, err := ecies.Process(resp.Tok.Tok)
	if err != nil {
		return err
	}

	req.Tok.Tok = out
	req.Sig = nil

	hash := primitives.NewHash()
	raw, _ := proto.Marshal(req)
	hash.Write(raw)

	r, s, err := ecdsa.Sign(rand.Reader, signPriv, hash.Sum(nil))
	if err != nil {
		return err
	}
	R, _ := r.MarshalText()
	S, _ := s.MarshalText()
	req.Sig = &pb.Signature{Type: pb.CryptoType_ECDSA, R: R, S: S}

	resp, err = ecap.CreateCertificatePair(context.Background(), req)
	if err != nil {
		return err
	}

	// Verify we got valid crypto material back
	x509SignCert, err := primitives.DERToX509Certificate(resp.Certs.Sign)
	if err != nil {
		return err
	}

	_, err = primitives.GetCriticalExtension(x509SignCert, ECertSubjectRole)
	if err != nil {
		return err
	}

	x509EncCert, err := primitives.DERToX509Certificate(resp.Certs.Enc)
	if err != nil {
		return err
	}

	_, err = primitives.GetCriticalExtension(x509EncCert, ECertSubjectRole)
	if err != nil {
		return err
	}

	return nil
}

func registerUser(registrar User, user *User) error {

	ecaa := &ECAA{eca}

	//create req
	req := &pb.RegisterUserReq{
		Id:          &pb.Identity{Id: user.enrollID},
		Role:        pb.Role(user.role),
		Affiliation: user.affiliation,
		Registrar: &pb.Registrar{
			Id:            &pb.Identity{Id: registrar.enrollID},
			Roles:         user.registrarRoles,
			DelegateRoles: user.registrarDelegateRoles,
		},
		Sig: nil}

	//sign the req
	hash := primitives.NewHash()
	raw, _ := proto.Marshal(req)
	hash.Write(raw)

	r, s, err := ecdsa.Sign(rand.Reader, registrar.enrollPrivKey, hash.Sum(nil))
	if err != nil {
		msg := "Failed to register user. Error (ECDSA) signing request: " + err.Error()
		return errors.New(msg)
	}
	R, _ := r.MarshalText()
	S, _ := s.MarshalText()
	req.Sig = &pb.Signature{Type: pb.CryptoType_ECDSA, R: R, S: S}

	token, err := ecaa.RegisterUser(context.Background(), req)
	if err != nil {
		return err
	}

	if token == nil {
		return errors.New("Failed to obtain token")
	}

	//need the token for later tests
	user.enrollPwd = token.Tok

	return nil
}

//check that the ECA was created / initialized
func TestNewECA(t *testing.T) {

	//initialization was handled in TestMain
	//check to see if ECA exists
	if eca == nil {
		t.Fatal("Failed to create ECA")
	}

	missing := 0

	//check to see that the expected files were created
	for _, file := range ecaFiles {
		if _, err := os.Stat(eca.CA.path + "/" + file); err != nil {
			missing++
			t.Logf("Failed to find file: [%s]", file)
		}
	}

	if missing > 0 {
		t.Fail()
	}
}

/**
* Test the CreateCertificatePair function by enolling a preloaded admin
* we can use to register additional users in later tests
 */
func TestCreateCertificatePairAdmin(t *testing.T) {
	//enroll testAdmin
	err := enrollUser(&testAdmin)

	if err != nil {
		t.Fatalf("Failed to enroll testAdmin: [%s]", err.Error())
	}
}

//register testUser using testAdmin as the registrar
func TestRegisterUser(t *testing.T) {

	err := registerUser(testAdmin, &testUser)

	if err != nil {
		t.Fatal(err.Error())
	}

}

//now see if we can enroll testUser
func TestCreateCertificatePairTestUser(t *testing.T) {

	err := enrollUser(&testUser)

	if err != nil {
		t.Fatalf("Failed to enroll testUser: [%s]", err.Error())
	}
}

//register testUser again - should get error
func TestRegisterDuplicateUser(t *testing.T) {

	err := registerUser(testAdmin, &testUser)

	if err == nil {
		t.Fatal("Expected an error when registering the same user twice")
	}

	if err.Error() != "User is already registered" {
		t.Fatalf("Expected error was not returned when registering user twice: [%s]", err.Error())
	}
}

//register testAuditor with testAdmin as registrar
//register testUser again - should get error
func TestRegisterAuditor(t *testing.T) {

	err := registerUser(testAdmin, &testAuditor)

	if err != nil {
		t.Fatal(err.Error())
	}
}

/**
* A user with no registrar metadata should not be able to register a new user
 */
func TestRegisterUserNonRegistrar(t *testing.T) {

	//testUser has no registrar metadata
	err := registerUser(testUser, &testUser2)

	if err == nil {
		t.Fatal("User without registrar metadata should not be able to register a new user")
	}
	t.Logf("Expected an error and indeed received: [%s]", err.Error())
}

//testAdmin should NOT be able to register testPeer since testAdmin's
//delegateRoles field DOES NOT contain the value "peer"
func TestRegisterUserPeer(t *testing.T) {

	err := registerUser(testAdmin, &testPeer)

	if err == nil {
		t.Fatal("User without appropriate delegateRoles should not be able to register a new user")
	}
	t.Logf("Expected an error and indeed received: [%s]", err.Error())
}

//testAdmin should be able to register testClient1 since testAdmin's
//delegateRoles field contains the value "client"
func TestRegisterUserClient(t *testing.T) {

	err := registerUser(testAdmin, &testClient1)

	if err != nil {
		t.Error(err.Error())
	}
}

//testClient1 registered in the previous test should be able to enroll
func TestCreateCertificatePairClient(t *testing.T) {

	err := enrollUser(&testClient1)

	if err != nil {
		t.Fatalf("Failed to enroll testClient1: [%s]", err.Error())
	}
}

//testClient1 should be able to register testClient2 since testClient1's
//delegateRoles field contains the value "client"
func TestRegisterUserClientAsRegistrar(t *testing.T) {

	err := registerUser(testClient1, &testClient2)

	if err != nil {
		t.Error(err.Error())
	}

}

//testClient2 should NOT be able to register testClient3 since testClient2's
//delegateRoles field is empty
func TestRegisterUserNoDelegateRoles(t *testing.T) {

	err := enrollUser(&testClient2)

	if err != nil {
		t.Fatalf("Failed to enroll testClient2: [%s]", err.Error())
	}

	err = registerUser(testClient2, &testClient3)

	if err == nil {
		t.Fatal("User without delegateRoles should not be able to register a new user")
	}

	t.Logf("Expected an error and indeed received: [%s]", err.Error())
}

func TestReadCACertificate(t *testing.T) {
	ecap := &ECAP{eca}
	_, err := ecap.ReadCACertificate(context.Background(), &pb.Empty{})

	if err != nil {
		t.Fatalf("Failed to read the CA certificate of the ECA: [%s]: ", err.Error())
	}
}

func TestReadCertificatePair(t *testing.T) {

	ecap := &ECAP{eca}

	req := &pb.ECertReadReq{Id: &pb.Identity{Id: testUser.enrollID}}

	_, err := ecap.ReadCertificatePair(context.Background(), req)

	if err != nil {
		t.Fatalf("Failed to read certificate pair: [%s]", err.Error())
	}
}

func TestReadCertificatePairBadIdentity(t *testing.T) {
	ecap := &ECAP{eca}

	req := &pb.ECertReadReq{Id: &pb.Identity{Id: "badUser"}}

	_, err := ecap.ReadCertificatePair(context.Background(), req)

	if err == nil {
		t.Error("The query result searching by an invalid user identity should have been empty. ")
	}

}

func TestReadCertificateByHash(t *testing.T) {
	ecap := &ECAP{eca}

	req := &pb.ECertReadReq{Id: &pb.Identity{Id: testUser.enrollID}}

	cert, err := ecap.ReadCertificatePair(context.Background(), req)

	if err != nil {
		t.Fatalf("Failed to read certificate pair: [%s]", err.Error())
	}

	hash := primitives.NewHash()
	raw, _ := proto.Marshal(cert)
	hash.Write(raw)

	hashReq := &pb.Hash{Hash: hash.Sum(nil)}

	certByHash, _ := ecap.ReadCertificateByHash(context.Background(), hashReq)

	if certByHash == nil {
		t.Error("A. ")
	}

}

func TestReadCertificateByInvalidHash(t *testing.T) {
	ecap := &ECAP{eca}

	req := &pb.Hash{Hash: nil}

	_, err := ecap.ReadCertificateByHash(context.Background(), req)

	if err == nil {
		t.Error("The query result searching by an invalid hash value should have been empty. ")
	}

}

func TestReadUserSet(t *testing.T) {

	//enroll Auditor
	err := enrollUser(&testAuditor)

	if err != nil {
		t.Fatalf("Failed to read user set [%s]", err.Error())
	}

	ecaa := &ECAA{eca}

	req := &pb.ReadUserSetReq{
		Req:  &pb.Identity{Id: testAuditor.enrollID},
		Role: 1,
		Sig:  nil}

	//sign the req
	hash := primitives.NewHash()
	raw, _ := proto.Marshal(req)
	hash.Write(raw)

	r, s, err := ecdsa.Sign(rand.Reader, testAuditor.enrollPrivKey, hash.Sum(nil))
	if err != nil {
		t.Fatalf("Failed (ECDSA) signing [%s]", err.Error())
	}
	R, _ := r.MarshalText()
	S, _ := s.MarshalText()
	req.Sig = &pb.Signature{Type: pb.CryptoType_ECDSA, R: R, S: S}

	resp, err := ecaa.ReadUserSet(context.Background(), req)

	if err != nil {
		t.Fatalf("Failed to read user set [%s]", err.Error())
	}
	t.Log("number of users: ", len(resp.Users))
}

func TestReadUserSetNonAuditor(t *testing.T) {

	ecaa := &ECAA{eca}

	req := &pb.ReadUserSetReq{
		Req:  &pb.Identity{Id: testUser.enrollID},
		Role: 1,
		Sig:  nil}

	//sign the req
	hash := primitives.NewHash()
	raw, _ := proto.Marshal(req)
	hash.Write(raw)

	r, s, err := ecdsa.Sign(rand.Reader, testUser.enrollPrivKey, hash.Sum(nil))
	if err != nil {
		t.Fatalf("Failed (ECDSA) signing [%s]", err.Error())
	}
	R, _ := r.MarshalText()
	S, _ := s.MarshalText()
	req.Sig = &pb.Signature{Type: pb.CryptoType_ECDSA, R: R, S: S}

	_, err = ecaa.ReadUserSet(context.Background(), req)

	if err == nil {
		t.Fatal("Only auditors should be able to call ReadUserSet")
	}

}

func TestCreateCertificatePairBadIdentity(t *testing.T) {

	ecap := &ECAP{eca}

	req := &pb.ECertCreateReq{
		Ts:   &timestamp.Timestamp{Seconds: time.Now().Unix(), Nanos: 0},
		Id:   &pb.Identity{Id: "badIdentity"},
		Tok:  &pb.Token{Tok: testUser.enrollPwd},
		Sign: &pb.PublicKey{Type: pb.CryptoType_ECDSA, Key: []byte{0}},
		Enc:  &pb.PublicKey{Type: pb.CryptoType_ECDSA, Key: []byte{0}},
		Sig:  nil}

	_, err := ecap.CreateCertificatePair(context.Background(), req)
	if err.Error() != "Identity lookup error: sql: no rows in result set" {
		t.Log(err.Error())
		t.Fatal("The expected error of 'Identity lookup error: sql: no rows in result set' was not returned for bad identity")
	}
}

func TestCreateCertificatePairBadToken(t *testing.T) {

	ecap := &ECAP{eca}

	req := &pb.ECertCreateReq{
		Ts:   &timestamp.Timestamp{Seconds: time.Now().Unix(), Nanos: 0},
		Id:   &pb.Identity{Id: testUser.enrollID},
		Tok:  &pb.Token{Tok: []byte("badPassword")},
		Sign: &pb.PublicKey{Type: pb.CryptoType_ECDSA, Key: []byte{0}},
		Enc:  &pb.PublicKey{Type: pb.CryptoType_ECDSA, Key: []byte{0}},
		Sig:  nil}

	_, err := ecap.CreateCertificatePair(context.Background(), req)
	if err.Error() != "Identity or token does not match." {
		t.Fatal("Expected error was not returned for bad password")
	}
}

func TestRevokeCertificatePair(t *testing.T) {

	ecap := &ECAP{eca}

	_, err := ecap.RevokeCertificatePair(context.Background(), &pb.ECertRevokeReq{})
	if err.Error() != "ECAP:RevokeCertificate method not (yet) implemented" {
		t.Fatalf("Expected error was not returned: [%s]", err.Error())
	}
}

func TestRevokeCertificate(t *testing.T) {

	ecaa := &ECAA{eca}

	_, err := ecaa.RevokeCertificate(context.Background(), &pb.ECertRevokeReq{})
	if err.Error() != "ECAA:RevokeCertificate method not (yet) implemented" {
		t.Fatalf("Expected error was not returned: [%s]", err.Error())
	}
}

func TestPublishCRL(t *testing.T) {
	ecaa := &ECAA{eca}

	_, err := ecaa.PublishCRL(context.Background(), &pb.ECertCRLReq{})
	if err.Error() != "ECAA:PublishCRL method not (yet) implemented" {
		t.Fatalf("Expected error was not returned: [%s]", err.Error())
	}
}
