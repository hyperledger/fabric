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

package chaincode

import (
	"fmt"
	"io/ioutil"
	"os"
	"testing"

	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric/msp"
	msptesttools "github.com/hyperledger/fabric/msp/mgmt/testtools"
	"github.com/hyperledger/fabric/peer/common"
	pcommon "github.com/hyperledger/fabric/protos/common"
	pb "github.com/hyperledger/fabric/protos/peer"
)

func TestMain(m *testing.M) {
	err := msptesttools.LoadMSPSetupForTesting()
	if err != nil {
		panic(fmt.Sprintf("Fatal error when reading MSP config: %s", err))
	}

	os.Exit(m.Run())
}

func newTempDir() string {
	tempDir, err := ioutil.TempDir("/tmp", "packagetest-")
	if err != nil {
		panic(err)
	}
	return tempDir
}

func mockCDSFactory(spec *pb.ChaincodeSpec) (*pb.ChaincodeDeploymentSpec, error) {
	return &pb.ChaincodeDeploymentSpec{ChaincodeSpec: spec, CodePackage: []byte("somecode")}, nil
}

func extractSignedCCDepSpec(env *pcommon.Envelope) (*pcommon.ChannelHeader, *pb.SignedChaincodeDeploymentSpec, error) {
	p := &pcommon.Payload{}
	err := proto.Unmarshal(env.Payload, p)
	if err != nil {
		return nil, nil, err
	}
	ch := &pcommon.ChannelHeader{}
	err = proto.Unmarshal(p.Header.ChannelHeader, ch)
	if err != nil {
		return nil, nil, err
	}

	sp := &pb.SignedChaincodeDeploymentSpec{}
	err = proto.Unmarshal(p.Data, sp)
	if err != nil {
		return nil, nil, err
	}

	return ch, sp, nil
}

// TestCDSPackage tests generation of the old ChaincodeDeploymentSpec install
// which we will presumably continue to support at least for a bit
func TestCDSPackage(t *testing.T) {
	pdir := newTempDir()
	defer os.RemoveAll(pdir)

	ccpackfile := pdir + "/ccpack.file"
	err := createSignedCDSPackage([]string{"-n", "somecc", "-p", "some/go/package", "-v", "0", ccpackfile}, false)
	if err != nil {
		t.Fatalf("Run chaincode package cmd error:%v", err)
	}

	b, err := ioutil.ReadFile(ccpackfile)
	if err != nil {
		t.Fatalf("package file %s not created", ccpackfile)
	}
	cds := &pb.ChaincodeDeploymentSpec{}
	err = proto.Unmarshal(b, cds)
	if err != nil {
		t.Fatalf("could not unmarshall package into CDS")
	}
}

//helper to create a SignedChaincodeDeploymentSpec
func createSignedCDSPackage(args []string, sign bool) error {
	var signer msp.SigningIdentity
	var err error
	if sign {

		signer, err = common.GetDefaultSigner()
		if err != nil {
			return fmt.Errorf("Get default signer error: %v", err)
		}
	}

	mockCF := &ChaincodeCmdFactory{Signer: signer}

	cmd := packageCmd(mockCF, mockCDSFactory)
	addFlags(cmd)

	cmd.SetArgs(args)

	if err := cmd.Execute(); err != nil {
		return err
	}

	return nil
}

// TestSignedCDSPackage generates the new envelope encapsulating
// CDS, policy
func TestSignedCDSPackage(t *testing.T) {
	pdir := newTempDir()
	defer os.RemoveAll(pdir)

	ccpackfile := pdir + "/ccpack.file"
	err := createSignedCDSPackage([]string{"-n", "somecc", "-p", "some/go/package", "-v", "0", "-s", ccpackfile}, false)
	if err != nil {
		t.Fatalf("could not create signed cds package %s", err)
	}

	b, err := ioutil.ReadFile(ccpackfile)
	if err != nil {
		t.Fatalf("package file %s not created", ccpackfile)
	}

	e := &pcommon.Envelope{}
	err = proto.Unmarshal(b, e)
	if err != nil {
		t.Fatalf("could not unmarshall envelope")
	}

	_, p, err := extractSignedCCDepSpec(e)
	if err != nil {
		t.Fatalf("could not extract signed dep spec")
	}

	if p.OwnerEndorsements != nil {
		t.Fatalf("expected no signtures but found endorsements")
	}
}

// TestSignedCDSPackageWithSignature generates the new envelope encapsulating
// CDS, policy and signs the package with local MSP
func TestSignedCDSPackageWithSignature(t *testing.T) {
	pdir := newTempDir()
	defer os.RemoveAll(pdir)

	ccpackfile := pdir + "/ccpack.file"
	err := createSignedCDSPackage([]string{"-n", "somecc", "-p", "some/go/package", "-v", "0", "-s", "-S", ccpackfile}, true)
	if err != nil {
		t.Fatalf("could not create signed cds package %s", err)
	}

	b, err := ioutil.ReadFile(ccpackfile)
	if err != nil {
		t.Fatalf("package file %s not created", ccpackfile)
	}
	e := &pcommon.Envelope{}
	err = proto.Unmarshal(b, e)
	if err != nil {
		t.Fatalf("could not unmarshall envelope")
	}

	_, p, err := extractSignedCCDepSpec(e)
	if err != nil {
		t.Fatalf("could not extract signed dep spec")
	}

	if p.OwnerEndorsements == nil {
		t.Fatalf("expected signtures and found nil")
	}
}

func TestNoOwnerToSign(t *testing.T) {
	pdir := newTempDir()
	defer os.RemoveAll(pdir)

	ccpackfile := pdir + "/ccpack.file"
	//note "-S" requires signer but we are passing fase
	err := createSignedCDSPackage([]string{"-n", "somecc", "-p", "some/go/package", "-v", "0", "-s", "-S", ccpackfile}, false)

	if err == nil {
		t.Fatalf("Expected error with nil signer but succeeded")
	}
}

func TestInvalidPolicy(t *testing.T) {
	pdir := newTempDir()
	defer os.RemoveAll(pdir)

	ccpackfile := pdir + "/ccpack.file"
	err := createSignedCDSPackage([]string{"-n", "somecc", "-p", "some/go/package", "-v", "0", "-s", "-i", "AND('a bad policy')", ccpackfile}, false)

	if err == nil {
		t.Fatalf("Expected error with nil signer but succeeded")
	}
}
