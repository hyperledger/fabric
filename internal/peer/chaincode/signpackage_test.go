/*
Copyright Digital Asset Holdings, LLC. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package chaincode

import (
	"fmt"
	"io/ioutil"
	"testing"

	"github.com/golang/protobuf/proto"
	pcommon "github.com/hyperledger/fabric-protos-go/common"
	"github.com/hyperledger/fabric/bccsp"
	"github.com/hyperledger/fabric/bccsp/sw"
	"github.com/hyperledger/fabric/internal/peer/common"
	"github.com/stretchr/testify/require"
)

// helper to sign an existing package
func signExistingPackage(env *pcommon.Envelope, infile, outfile string, cryptoProvider bccsp.BCCSP) error {
	signer, err := common.GetDefaultSigner()
	if err != nil {
		return fmt.Errorf("Get default signer error: %v", err)
	}

	mockCF := &ChaincodeCmdFactory{Signer: signer}

	cmd := signpackageCmd(mockCF, cryptoProvider)
	addFlags(cmd)

	cmd.SetArgs([]string{infile, outfile})

	if err := cmd.Execute(); err != nil {
		return err
	}

	return nil
}

// TestSignExistingPackage signs an existing package
func TestSignExistingPackage(t *testing.T) {
	resetFlags()
	defer resetFlags()
	pdir := t.TempDir()

	cryptoProvider, err := sw.NewDefaultSecurityLevelWithKeystore(sw.NewDummyKeyStore())
	require.NoError(t, err)

	ccpackfile := pdir + "/ccpack.file"
	err = createSignedCDSPackage(t, []string{"-n", "somecc", "-p", "some/go/package", "-v", "0", "-s", "-S", ccpackfile}, true)
	if err != nil {
		t.Fatalf("error creating signed :%v", err)
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

	signedfile := pdir + "/signed.file"
	err = signExistingPackage(e, ccpackfile, signedfile, cryptoProvider)
	if err != nil {
		t.Fatalf("could not sign envelope")
	}

	b, err = ioutil.ReadFile(signedfile)
	if err != nil {
		t.Fatalf("signed package file %s not created", signedfile)
	}

	e = &pcommon.Envelope{}
	err = proto.Unmarshal(b, e)
	if err != nil {
		t.Fatalf("could not unmarshall signed envelope")
	}

	_, p, err := extractSignedCCDepSpec(e)
	if err != nil {
		t.Fatalf("could not extract signed dep spec")
	}

	if p.OwnerEndorsements == nil {
		t.Fatalf("expected endorsements")
	}

	if len(p.OwnerEndorsements) != 2 {
		t.Fatalf("expected 2 endorserments but found %d", len(p.OwnerEndorsements))
	}
}

// TestFailSignUnsignedPackage tries to signs a package that was not originally signed
func TestFailSignUnsignedPackage(t *testing.T) {
	resetFlags()
	defer resetFlags()
	pdir := t.TempDir()
	cryptoProvider, err := sw.NewDefaultSecurityLevelWithKeystore(sw.NewDummyKeyStore())
	require.NoError(t, err)

	ccpackfile := pdir + "/ccpack.file"
	// don't sign it ... no "-S"
	err = createSignedCDSPackage(t, []string{"-n", "somecc", "-p", "some/go/package", "-v", "0", "-s", ccpackfile}, true)
	if err != nil {
		t.Fatalf("error creating signed :%v", err)
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

	signedfile := pdir + "/signed.file"
	err = signExistingPackage(e, ccpackfile, signedfile, cryptoProvider)
	if err == nil {
		t.Fatalf("expected signing a package that's not originally signed to fail")
	}
}
