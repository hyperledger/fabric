package msp

import (
	"os"
	"reflect"
	"testing"
)

var localMsp MSP

func TestNoopMSP(t *testing.T) {
	noopmsp := NewNoopMsp()

	id, err := noopmsp.GetDefaultSigningIdentity()
	if err != nil {
		t.Fatalf("GetSigningIdentity should have succeeded")
		return
	}

	serializedID, err := id.Serialize()
	if err != nil {
		t.Fatalf("Serialize should have succeeded")
		return
	}

	idBack, err := noopmsp.DeserializeIdentity(serializedID)
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

	err = id.Verify(msg, sig)
	if err != nil {
		t.Fatalf("The signature should be valid")
		return
	}

	err = idBack.Verify(msg, sig)
	if err != nil {
		t.Fatalf("The signature should be valid")
		return
	}
}

func TestMSPSetupBad(t *testing.T) {
	_, err := GetLocalMspConfig("barf")
	if err == nil {
		t.Fatalf("Setup should have failed on an invalid config file")
		return
	}
}

func TestMSPSetupGood(t *testing.T) {
	conf, err := GetLocalMspConfig("./sampleconfig/")
	if err != nil {
		t.Fatalf("Setup should have succeeded, got err %s instead", err)
		return
	}

	localMsp, err = NewBccspMsp()
	if err != nil {
		t.Fatalf("Constructor for msp should have succeeded, got err %s instead", err)
		return
	}

	err = localMsp.Setup(conf)
	if err != nil {
		t.Fatalf("Setup for msp should have succeeded, got err %s instead", err)
		return
	}
}

func TestGetIdentities(t *testing.T) {
	_, err := localMsp.GetDefaultSigningIdentity()
	if err != nil {
		t.Fatalf("GetDefaultSigningIdentity failed with err %s", err)
		return
	}
}

func TestSerializeIdentities(t *testing.T) {
	id, err := localMsp.GetDefaultSigningIdentity()
	if err != nil {
		t.Fatalf("GetSigningIdentity should have succeeded, got err %s", err)
		return
	}

	serializedID, err := id.Serialize()
	if err != nil {
		t.Fatalf("Serialize should have succeeded, got err %s", err)
		return
	}

	idBack, err := localMsp.DeserializeIdentity(serializedID)
	if err != nil {
		t.Fatalf("DeserializeIdentity should have succeeded, got err %s", err)
		return
	}

	err = localMsp.Validate(idBack)
	if err != nil {
		t.Fatalf("The identity should be valid, got err %s", err)
		return
	}

	if !reflect.DeepEqual(id.GetPublicVersion(), idBack) {
		t.Fatalf("Identities should be equal (%s) (%s)", id, idBack)
		return
	}
}

func TestSignAndVerify(t *testing.T) {
	id, err := localMsp.GetDefaultSigningIdentity()
	if err != nil {
		t.Fatalf("GetSigningIdentity should have succeeded")
		return
	}

	serializedID, err := id.Serialize()
	if err != nil {
		t.Fatalf("Serialize should have succeeded")
		return
	}

	idBack, err := localMsp.DeserializeIdentity(serializedID)
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

	err = id.Verify(msg, sig)
	if err != nil {
		t.Fatalf("The signature should be valid")
		return
	}

	err = idBack.Verify(msg, sig)
	if err != nil {
		t.Fatalf("The signature should be valid")
		return
	}
}

func TestSignAndVerify_longMessage(t *testing.T) {
	id, err := localMsp.GetDefaultSigningIdentity()
	if err != nil {
		t.Fatalf("GetSigningIdentity should have succeeded")
		return
	}

	serializedID, err := id.Serialize()
	if err != nil {
		t.Fatalf("Serialize should have succeeded")
		return
	}

	idBack, err := localMsp.DeserializeIdentity(serializedID)
	if err != nil {
		t.Fatalf("DeserializeIdentity should have succeeded")
		return
	}

	msg := []byte("ABCDEFGABCDEFGABCDEFGABCDEFGABCDEFGABCDEFGABCDEFGABCDEFGABCDEFGABCDEFGABCDEFGABCDEFGABCDEFGABCDEFG")
	sig, err := id.Sign(msg)
	if err != nil {
		t.Fatalf("Sign should have succeeded")
		return
	}

	err = id.Verify(msg, sig)
	if err != nil {
		t.Fatalf("The signature should be valid")
		return
	}

	err = idBack.Verify(msg, sig)
	if err != nil {
		t.Fatalf("The signature should be valid")
		return
	}
}

func TestMain(m *testing.M) {
	retVal := m.Run()
	os.Exit(retVal)
}
