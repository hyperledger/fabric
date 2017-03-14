/*
Copyright IBM Corp. 2017 All Rights Reserved.

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
package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/hyperledger/fabric/common/tools/cryptogen/ca"
	"github.com/hyperledger/fabric/common/tools/cryptogen/msp"
)

const (
	peerOrgBaseName  = "peerOrg"
	peerBaseName     = "Peer"
	userBaseName     = "User"
	adminBaseName    = "Admin"
	orderOrgBaseName = "ordererOrg"
	ordererBaseName  = "Orderer"
)

//command line flags
var (
	numPeerOrgs = flag.Int("peerOrgs", 2,
		"number of unique organizations with peers")
	numPeers = flag.Int("peersPerOrg", 1,
		"number of peers per organization")
	numPeerOrgUsers = flag.Int("peerOrgUsers", 1,
		"number of users per peer organization")
	numOrderers = flag.Int("ordererNodes", 1,
		"number of ordering service nodes")
	baseDir = flag.String("baseDir", ".",
		"directory in which to place artifacts")
)

var numOrdererOrgs = 1

func main() {
	flag.Parse()

	if flag.NFlag() == 0 {
		fmt.Println("\nYou must specify at least one parameter")
		flag.Usage()
		os.Exit(1)
	}

	genDir := filepath.Join(*baseDir, "crypto-config")
	if *numPeerOrgs > 0 {
		fmt.Printf("Generating %d peer organization(s) each with %d peer(s) ...\n",
			*numPeerOrgs, *numPeers)

		// TODO: add ability to specify peer org names
		// for name just use default base name
		peerOrgNames := []string{}
		for i := 1; i <= *numPeerOrgs; i++ {
			peerOrgNames = append(peerOrgNames, fmt.Sprintf("%s%d", peerOrgBaseName, i))
		}
		generatePeerOrgs(genDir, peerOrgNames)

	}

	if *numOrderers > 0 {
		fmt.Printf("Generating %d orderer organization(s) and %d ordering node(s) ...\n",
			numOrdererOrgs, *numOrderers)
		generateOrdererOrg(genDir, fmt.Sprintf("%s1", orderOrgBaseName))
	}

}

func generatePeerOrgs(baseDir string, orgNames []string) {

	for _, orgName := range orgNames {
		fmt.Println(orgName)
		// generate CA
		orgDir := filepath.Join(baseDir, "peerOrganizations", orgName)
		caDir := filepath.Join(orgDir, "ca")
		mspDir := filepath.Join(orgDir, "msp")
		peersDir := filepath.Join(orgDir, "peers")
		usersDir := filepath.Join(orgDir, "users")
		adminCertsDir := filepath.Join(mspDir, "admincerts")
		rootCA, err := ca.NewCA(caDir, orgName)
		if err != nil {
			fmt.Printf("Error generating CA for org %s:\n%v\n", orgName, err)
			os.Exit(1)
		}
		err = msp.GenerateVerifyingMSP(mspDir, rootCA)
		if err != nil {
			fmt.Printf("Error generating MSP for org %s:\n%v\n", orgName, err)
			os.Exit(1)
		}

		// TODO: add ability to specify peer names
		// for name just use default base name
		peerNames := []string{}
		for i := 1; i <= *numPeers; i++ {
			peerNames = append(peerNames, fmt.Sprintf("%s%s%d",
				orgName, peerBaseName, i))
		}
		generateNodes(peersDir, peerNames, rootCA)

		// TODO: add ability to specify usernames
		usernames := []string{}
		for j := 1; j <= *numPeerOrgUsers; j++ {
			usernames = append(usernames, fmt.Sprintf("%s%s%d",
				orgName, userBaseName, j))
		}
		// add an admin user
		usernames = append(usernames, fmt.Sprintf("%s%s",
			orgName, adminBaseName))
		generateNodes(usersDir, usernames, rootCA)

		// copy the admin cert to the org's MSP admincerts
		adminUserName := fmt.Sprintf("%s%s",
			orgName, adminBaseName)
		err = copyAdminCert(usersDir, adminCertsDir, adminUserName)
		if err != nil {
			fmt.Printf("Error copying admin cert for org %s:\n%v\n",
				orgName, err)
			os.Exit(1)
		}

	}
}

func copyAdminCert(usersDir, adminCertsDir, adminUserName string) error {
	// delete the contents of admincerts
	err := os.RemoveAll(adminCertsDir)
	if err != nil {
		return err
	}
	// recreate the admincerts directory
	err = os.MkdirAll(adminCertsDir, 0755)
	if err != nil {
		return err
	}
	err = copyFile(filepath.Join(usersDir, adminUserName, "signcerts",
		adminUserName+"-cert.pem"), filepath.Join(adminCertsDir,
		adminUserName+"-cert.pem"))
	if err != nil {
		return err
	}
	return nil

}

func generateNodes(baseDir string, nodeNames []string, rootCA *ca.CA) {

	for _, nodeName := range nodeNames {
		nodeDir := filepath.Join(baseDir, nodeName)
		err := msp.GenerateLocalMSP(nodeDir, nodeName, rootCA)
		if err != nil {
			fmt.Printf("Error generating local MSP for %s:\n%v\n", nodeName, err)
			os.Exit(1)
		}
	}

}

func generateOrdererOrg(baseDir, orgName string) {

	// generate CA
	orgDir := filepath.Join(baseDir, "ordererOrganizations", orgName)
	caDir := filepath.Join(orgDir, "ca")
	mspDir := filepath.Join(orgDir, "msp")
	orderersDir := filepath.Join(orgDir, "orderers")
	usersDir := filepath.Join(orgDir, "users")
	adminCertsDir := filepath.Join(mspDir, "admincerts")
	rootCA, err := ca.NewCA(caDir, orgName)
	if err != nil {
		fmt.Printf("Error generating CA for org %s:\n%v\n", orgName, err)
		os.Exit(1)
	}
	err = msp.GenerateVerifyingMSP(mspDir, rootCA)
	if err != nil {
		fmt.Printf("Error generating MSP for org %s:\n%v\n", orgName, err)
		os.Exit(1)
	}

	// TODO: add ability to specify orderer names
	// for name just use default base name
	ordererNames := []string{}
	for i := 1; i <= *numOrderers; i++ {
		ordererNames = append(ordererNames, fmt.Sprintf("%s%s%d",
			orgName, ordererBaseName, i))
	}
	generateNodes(orderersDir, ordererNames, rootCA)

	// generate an admin for the orderer org
	usernames := []string{}
	// add an admin user
	usernames = append(usernames, fmt.Sprintf("%s%s",
		orgName, adminBaseName))
	generateNodes(usersDir, usernames, rootCA)

	// copy the admin cert to the org's MSP admincerts
	adminUserName := fmt.Sprintf("%s%s",
		orgName, adminBaseName)
	err = copyAdminCert(usersDir, adminCertsDir, adminUserName)
	if err != nil {
		fmt.Printf("Error copying admin cert for org %s:\n%v\n",
			orgName, err)
		os.Exit(1)
	}

}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, in)
	cerr := out.Close()
	if err != nil {
		return err
	}
	return cerr
}
