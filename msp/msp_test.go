package msp

import (
	"os"
	"reflect"
	"testing"

	"encoding/json"
	"fmt"
	"io/ioutil"

	"github.com/hyperledger/fabric/core/crypto/primitives"
)

func getPeerConfFromFile(configFile string) (*NodeLocalConfig, error) {
	file, err := ioutil.ReadFile(configFile)
	if err != nil {
		return nil, fmt.Errorf("Could not read file %s, err %s", configFile, err)
	}

	var localConf NodeLocalConfig
	err = json.Unmarshal(file, &localConf)
	if err != nil {
		return nil, fmt.Errorf("Could not unmarshal config, err %s", err)
	}

	return &localConf, nil
}

func LoadLocalMSPConfig(configFile string) error {
	localConf, err := getPeerConfFromFile(configFile)
	if err != nil {
		return err
	}

	if localConf.LocalMSP == nil {
		return fmt.Errorf("nil LocalMSP")
	}

	err = GetLocalMSP().Setup(localConf.LocalMSP)
	if err != nil {
		return fmt.Errorf("Could not setup local msp, err %s", err)
	}

	// TODO: setup BCCSP here using localConf.BCCSP

	return nil
}

func TestMSPSetupBad(t *testing.T) {
	err := LoadLocalMSPConfig("barf")
	if err == nil {
		t.Fatalf("Setup should have failed on an invalid config file")
		return
	}
}

func TestMSPSetupGood(t *testing.T) {
	err := LoadLocalMSPConfig("peer-config.json")
	if err != nil {
		t.Fatalf("Setup should have succeeded, got err %s instead", err)
		return
	}
}

func TestGetIdentities(t *testing.T) {
	_, err := GetLocalMSP().GetDefaultSigningIdentity()
	if err != nil {
		t.Fatalf("GetDefaultSigningIdentity failed with err %s", err)
		return
	}
}

func TestSerializeIdentities(t *testing.T) {
	id, err := GetLocalMSP().GetDefaultSigningIdentity()
	if err != nil {
		t.Fatalf("GetSigningIdentity should have succeeded, got err %s", err)
		return
	}

	serializedID, err := id.Serialize()
	if err != nil {
		t.Fatalf("Serialize should have succeeded, got err %s", err)
		return
	}

	idBack, err := GetLocalMSP().DeserializeIdentity(serializedID)
	if err != nil {
		t.Fatalf("DeserializeIdentity should have succeeded, got err %s", err)
		return
	}

	valid, err := GetLocalMSP().IsValid(idBack)
	if err != nil || !valid {
		t.Fatalf("The identity should be valid, got err %s", err)
		return
	}

	if !reflect.DeepEqual(id.GetPublicVersion(), idBack) {
		t.Fatalf("Identities should be equal (%s) (%s)", id, idBack)
		return
	}
}

func TestSignAndVerify(t *testing.T) {
	id, err := GetLocalMSP().GetDefaultSigningIdentity()
	if err != nil {
		t.Fatalf("GetSigningIdentity should have succeeded")
		return
	}

	serializedID, err := id.Serialize()
	if err != nil {
		t.Fatalf("Serialize should have succeeded")
		return
	}

	idBack, err := GetLocalMSP().DeserializeIdentity(serializedID)
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

	retVal := m.Run()
	os.Exit(retVal)
}
