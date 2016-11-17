package msp

import (
	"os"
	"reflect"
	"testing"

	"github.com/hyperledger/fabric/core/crypto/primitives"
)

var mgr PeerMSPManager

func TestMSPSetupBad(t *testing.T) {
	err := mgr.Setup("barf")
	if err == nil {
		t.Fatalf("Setup should have failed on an invalid config file")
		return
	}
}

func TestMSPSetupGood(t *testing.T) {
	err := mgr.Setup("peer-config.json")
	if err != nil {
		t.Fatalf("Setup should have succeeded, got err %s instead", err)
		return
	}
}

func TestGetBadIdentities(t *testing.T) {
	idBad := IdentityIdentifier{Mspid: ProviderIdentifier{Value: "BARF"}, Value: "PEER"}
	id, err := mgr.GetSigningIdentity(&idBad)
	if err == nil {
		t.Fatalf("GetSigningIdentity for invalid identifier should have failed but it returned %s", id)
		return
	}

	idBad = IdentityIdentifier{Mspid: ProviderIdentifier{Value: "DEFAULT"}, Value: "BARF"}
	id, err = mgr.GetSigningIdentity(&idBad)
	if err == nil {
		t.Fatalf("GetSigningIdentity for invalid identifier should have failed but it returned %s", id)
		return
	}
}

func TestSerializeIdentities(t *testing.T) {
	idId := IdentityIdentifier{Mspid: ProviderIdentifier{Value: "DEFAULT"}, Value: "PEER"}
	id, err := mgr.GetSigningIdentity(&idId)
	if err != nil {
		t.Fatalf("GetSigningIdentity should have succeeded")
		return
	}

	serializedID, err := id.Serialize()
	if err != nil {
		t.Fatalf("Serialize should have succeeded")
		return
	}

	idBack, err := mgr.DeserializeIdentity(serializedID)
	if err != nil {
		t.Fatalf("DeserializeIdentity should have succeeded")
		return
	}

	valid, err := idBack.Validate()
	if err != nil || !valid {
		t.Fatalf("The identity should be valid")
		return
	}

	if !reflect.DeepEqual(id.GetPublicVersion(), idBack) {
		t.Fatalf("Identities should be equal (%s) (%s)", id, idBack)
		return
	}
}

func TestSignAndVerify(t *testing.T) {
	idId := IdentityIdentifier{Mspid: ProviderIdentifier{Value: "DEFAULT"}, Value: "PEER"}
	id, err := mgr.GetSigningIdentity(&idId)
	if err != nil {
		t.Fatalf("GetSigningIdentity should have succeeded")
		return
	}

	serializedID, err := id.Serialize()
	if err != nil {
		t.Fatalf("Serialize should have succeeded")
		return
	}

	idBack, err := mgr.DeserializeIdentity(serializedID)
	if err != nil {
		t.Fatalf("DeserializeIdentity should have succeeded")
		return
	}

	msg := []byte("foo")
	sig, err := id.Sign(msg)
	if err != nil {
		t.Fatalf("Sign should have succeeded")
		return
	}

	valid, err := id.Verify(msg, sig)
	if err != nil || !valid {
		t.Fatalf("The signature should be valid")
		return
	}

	valid, err = idBack.Verify(msg, sig)
	if err != nil || !valid {
		t.Fatalf("The signature should be valid")
		return
	}
}

func TestMain(m *testing.M) {
	primitives.SetSecurityLevel("SHA2", 256)
	mgr = GetManager()

	retVal := m.Run()
	os.Exit(retVal)
}
