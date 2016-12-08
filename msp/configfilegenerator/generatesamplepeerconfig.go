package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"os"

	"github.com/hyperledger/fabric/msp"
)

func readFile(file string) ([]byte, error) {
	fileCont, err := ioutil.ReadFile(file)
	if err != nil {
		return nil, fmt.Errorf("Could not read file %s, err %s", file, err)
	}

	return fileCont, nil
}

func writeFile(file string, content []byte) error {
	err := ioutil.WriteFile(file, content, 0660)
	if err != nil {
		return fmt.Errorf("Could not write file %s, err %s", file, err)
	}

	return nil
}

// This sample program shows how one can construct
// the local peer config out of a root cert, a peer
// cert and a signing key
func main() {
	caCertFile := flag.String("cacert", "./cacert.pem", "the ca certificate")
	peerCertFile := flag.String("peercert", "./peer.pem", "the peer certificate")
	keyFile := flag.String("key", "./key.pem", "the signing key of the peer")
	outFile := flag.String("out", "./peer-config.json", "the output file")

	flag.Parse()

	cacertBytes, err := readFile(*caCertFile)
	if err != nil {
		fmt.Printf("%s\n", err)
		os.Exit(-1)
	}

	peerCertBytes, err := readFile(*peerCertFile)
	if err != nil {
		fmt.Printf("%s\n", err)
		os.Exit(-1)
	}

	keyBytes, err := readFile(*keyFile)
	if err != nil {
		fmt.Printf("%s\n", err)
		os.Exit(-1)
	}

	keyinfo := &msp.KeyInfo{KeyIdentifier: "PEER", KeyMaterial: keyBytes}

	sigid := &msp.SigningIdentityInfo{PublicSigner: peerCertBytes, PrivateSigner: keyinfo}

	fmspconf := msp.FabricMSPConfig{Admins: [][]byte{[]byte(cacertBytes)}, RootCerts: [][]byte{[]byte(cacertBytes)}, SigningIdentity: sigid, Name: "DEFAULT"}

	fmpsjs, _ := json.Marshal(fmspconf)

	mspconf := &msp.MSPConfig{Config: fmpsjs, Type: msp.FABRIC}

	bccspconf := &msp.BCCSPConfig{Location: "/path/to/keystore", Name: "DEFAULT"}

	peerconf := msp.NodeLocalConfig{BCCSP: bccspconf, LocalMSP: mspconf}
	peerconfjs, _ := json.Marshal(peerconf)

	err = writeFile(*outFile, peerconfjs)
	if err != nil {
		fmt.Printf("%s\n", err)
		os.Exit(-1)
	}

	os.Exit(0)
}
