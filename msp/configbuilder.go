package msp

import (
	"fmt"
	"io/ioutil"

	"github.com/golang/protobuf/proto"

	"encoding/pem"
	"path/filepath"

	"github.com/hyperledger/fabric/protos/msp"
)

func readFile(file string) ([]byte, error) {
	fileCont, err := ioutil.ReadFile(file)
	if err != nil {
		return nil, fmt.Errorf("Could not read file %s, err %s", file, err)
	}

	return fileCont, nil
}

func readPemFile(file string) ([]byte, error) {
	bytes, err := readFile(file)
	if err != nil {
		return nil, err
	}

	b, _ := pem.Decode(bytes)
	if b == nil { // TODO: also check that the type is what we expect (cert vs key..)
		return nil, fmt.Errorf("No pem content for file %s", file)
	}

	return bytes, nil
}

func getPemMaterialFromDir(dir string) ([][]byte, error) {
	mspLogger.Infof("Reading directory %s", dir)

	content := make([][]byte, 0)
	files, err := ioutil.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("Could not read directory %s, err %s", err, dir)
	}

	for _, f := range files {
		if f.IsDir() {
			continue
		}

		fullName := filepath.Join(dir, string(filepath.Separator), f.Name())
		mspLogger.Infof("Inspecting file %s", fullName)

		item, err := readPemFile(fullName)
		if err != nil {
			continue
		}

		content = append(content, item)
	}

	return content, nil
}

const (
	cacerts    = "cacerts"
	admincerts = "admincerts"
	signcerts  = "signcerts"
	keystore   = "keystore"
)

func GetLocalMspConfig(dir string) (*msp.MSPConfig, error) {
	cacertDir := filepath.Join(dir, cacerts)
	signcertDir := filepath.Join(dir, signcerts)
	admincertDir := filepath.Join(dir, admincerts)
	keystoreDir := filepath.Join(dir, keystore)

	cacerts, err := getPemMaterialFromDir(cacertDir)
	if err != nil || len(cacerts) == 0 {
		return nil, fmt.Errorf("Could not load a valid ca certificate from directory %s, err %s", cacertDir, err)
	}

	signcert, err := getPemMaterialFromDir(signcertDir)
	if err != nil || len(signcert) == 0 {
		return nil, fmt.Errorf("Could not load a valid signer certificate from directory %s, err %s", signcertDir, err)
	}

	admincert, err := getPemMaterialFromDir(admincertDir)
	if err != nil || len(admincert) == 0 {
		return nil, fmt.Errorf("Could not load a valid admin certificate from directory %s, err %s", admincertDir, err)
	}

	keys, err := getPemMaterialFromDir(keystoreDir)
	if err != nil || len(keys) == 0 {
		return nil, fmt.Errorf("Could not load a valid signing key from directory %s, err %s", keystoreDir, err)
	}

	// FIXME: for now we're making the following assumptions
	// 1) there is exactly one signing cert
	// 2) there is exactly one signing key
	// 3) the cert and the key match

	keyinfo := &msp.KeyInfo{KeyIdentifier: "PEER", KeyMaterial: keys[0]}

	sigid := &msp.SigningIdentityInfo{PublicSigner: signcert[0], PrivateSigner: keyinfo}

	fmspconf := &msp.FabricMSPConfig{Admins: admincert, RootCerts: cacerts, SigningIdentity: sigid, Name: "DEFAULT"}

	fmpsjs, _ := proto.Marshal(fmspconf)

	mspconf := &msp.MSPConfig{Config: fmpsjs, Type: int32(FABRIC)}

	return mspconf, nil
}
