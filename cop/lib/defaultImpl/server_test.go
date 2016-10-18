package defaultImpl

import (
	"crypto/ecdsa"
	"crypto/rand"
	"errors"
	"testing"

	"golang.org/x/net/context"

	"github.com/golang/protobuf/proto"
	pb "github.com/hyperledger/fabric/cop/protos"
	"github.com/hyperledger/fabric/core/crypto/primitives"
)

type User struct {
	enrollID    string
	enrollPwd   []byte
	affiliation string
	attributes  []*pb.Attribute
}

var (
	testAdmin = User{enrollID: "admin", enrollPwd: []byte("Xurw3yU9zI0l")}
	testUser  = User{enrollID: "testUser", affiliation: "institution_a", attributes: []*pb.Attribute{&pb.Attribute{Name: "roles", Value: []byte("peer,validator")}}}
)

func registerUser(registrar User, user *User) error {
	server := NewServer()

	//create req
	req := &pb.RegisterReq{
		Id:          user.enrollID,
		Affiliation: user.affiliation,
		Attributes:  user.attributes,
		Nonce: &pb.Nonce{
			Name:  []byte("nonce"),
			Value: []byte("12345"),
		},
		CallerId: registrar.enrollID,
		Sig:      nil,
	}

	//sign the req
	primitives.InitSecurityLevel("SHA3", 256)
	hash := primitives.NewHash()
	raw, _ := proto.Marshal(req)
	hash.Write(raw)

	signPriv, err := primitives.NewECDSAKey()

	r, s, err := ecdsa.Sign(rand.Reader, signPriv, hash.Sum(nil))
	if err != nil {
		msg := "Failed to register user. Error (ECDSA) signing request: " + err.Error()
		return errors.New(msg)
	}
	R, _ := r.MarshalText()
	S, _ := s.MarshalText()
	req.Sig = &pb.Signature{Type: pb.CryptoType_ECDSA, R: R, S: S}

	token, err := server.Register(context.Background(), req)
	if err != nil {
		return err
	}

	if token == nil {
		return errors.New("Failed to obtain token")
	}

	//need the token for later tests
	user.enrollPwd = token.EnrollSecret
	// fmt.Println("user.enrollPwd: ", user.enrollPwd)

	return nil
}

func TestRegisterUserWithRegistrar(t *testing.T) {
	err := registerUser(testAdmin, &testUser)
	if err != nil {
		t.Error("Failed to register user with registrar ", err)
	}
}

func TestRegisterUserWithNonRegistrar(t *testing.T) {
	err := registerUser(testUser, &testUser)
	if err == nil {
		t.Error("Should failed to register with non-registrat user ", err)
	}
}
