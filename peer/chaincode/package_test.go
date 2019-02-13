/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package chaincode

import (
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"testing"

	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric/msp"
	msptesttools "github.com/hyperledger/fabric/msp/mgmt/testtools"
	"github.com/hyperledger/fabric/peer/chaincode/mock"
	"github.com/hyperledger/fabric/peer/common"
	pcommon "github.com/hyperledger/fabric/protos/common"
	pb "github.com/hyperledger/fabric/protos/peer"
	"github.com/stretchr/testify/assert"
)

//go:generate counterfeiter -o mock/writer.go -fake-name Writer . writer
type writer interface {
	Writer
}

//go:generate counterfeiter -o mock/platform_registry.go -fake-name PlatformRegistry . platformRegistryIntf
type platformRegistryIntf interface {
	PlatformRegistry
}

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
	err := createSignedCDSPackage(t, []string{"-n", "somecc", "-p", "some/go/package", "-v", "0", ccpackfile}, false)
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

// helper to create a SignedChaincodeDeploymentSpec
func createSignedCDSPackage(t *testing.T, args []string, sign bool) error {
	p := newPackagerForTest(t, nil, nil, sign)

	cmd := packageCmd(nil, mockCDSFactory, p)
	addFlags(cmd)

	cmd.SetArgs(args)

	if err := cmd.Execute(); err != nil {
		return err
	}

	return nil
}

func mockChaincodeCmdFactoryForTest(sign bool) (*ChaincodeCmdFactory, error) {
	var signer msp.SigningIdentity
	var err error
	if sign {
		signer, err = common.GetDefaultSigner()
		if err != nil {
			return nil, fmt.Errorf("Get default signer error: %v", err)
		}
	}

	cf := &ChaincodeCmdFactory{Signer: signer}
	return cf, nil
}

// TestSignedCDSPackage generates the new envelope encapsulating
// CDS, policy
func TestSignedCDSPackage(t *testing.T) {
	pdir := newTempDir()
	defer os.RemoveAll(pdir)

	ccpackfile := pdir + "/ccpack.file"
	err := createSignedCDSPackage(t, []string{"-n", "somecc", "-p", "some/go/package", "-v", "0", "-s", ccpackfile}, false)
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
		t.Fatalf("expected no signatures but found endorsements")
	}
}

// TestSignedCDSPackageWithSignature generates the new envelope encapsulating
// CDS, policy and signs the package with local MSP
func TestSignedCDSPackageWithSignature(t *testing.T) {
	pdir := newTempDir()
	defer os.RemoveAll(pdir)

	ccpackfile := pdir + "/ccpack.file"
	err := createSignedCDSPackage(t, []string{"-n", "somecc", "-p", "some/go/package", "-v", "0", "-s", "-S", ccpackfile}, true)
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
		t.Fatalf("expected signatures and found nil")
	}
}

func TestNoOwnerToSign(t *testing.T) {
	pdir := newTempDir()
	defer os.RemoveAll(pdir)

	ccpackfile := pdir + "/ccpack.file"
	// note "-S" requires signer but we are passing fase
	err := createSignedCDSPackage(t, []string{"-n", "somecc", "-p", "some/go/package", "-v", "0", "-s", "-S", ccpackfile}, false)

	if err == nil {
		t.Fatalf("Expected error with nil signer but succeeded")
	}
}

func TestInvalidPolicy(t *testing.T) {
	pdir := newTempDir()
	defer os.RemoveAll(pdir)

	ccpackfile := pdir + "/ccpack.file"
	err := createSignedCDSPackage(t, []string{"-n", "somecc", "-p", "some/go/package", "-v", "0", "-s", "-i", "AND('a bad policy')", ccpackfile}, false)

	if err == nil {
		t.Fatalf("Expected error with nil signer but succeeded")
	}
}

func newPackagerForTest(t *testing.T, pr PlatformRegistry, w Writer, sign bool) *Packager {
	if pr == nil {
		pr = &mock.PlatformRegistry{}
	}

	if w == nil {
		w = &mock.Writer{}
	}

	mockCF, err := mockChaincodeCmdFactoryForTest(sign)
	if err != nil {
		t.Fatal("error creating mock ChaincodeCmdFactory", err)
	}

	p := &Packager{
		ChaincodeCmdFactory: mockCF,
		CDSFactory:          mockCDSFactory,
		PlatformRegistry:    pr,
		Writer:              w,
	}

	return p
}

func TestPackageCC(t *testing.T) {
	assert := assert.New(t)

	t.Run("success", func(t *testing.T) {
		resetFlags()

		p := newPackagerForTest(t, nil, nil, false)
		args := []string{"output"}
		chaincodePath = "testPath"
		chaincodeLang = "golang"
		newLifecycle = true

		err := p.packageChaincode(args)
		assert.NoError(err)
	})

	t.Run("input validation failure", func(t *testing.T) {
		resetFlags()

		p := newPackagerForTest(t, nil, nil, false)
		args := []string{"output"}
		chaincodePath = "testPath"
		chaincodeLang = "golang"
		chaincodeName = "testcc"
		newLifecycle = true

		err := p.packageChaincode(args)
		assert.Error(err)
		assert.Equal("chaincode name not supported by _lifecycle", err.Error())
	})

	t.Run("getting the chaincode bytes fails", func(t *testing.T) {
		resetFlags()

		mockPlatformRegistry := &mock.PlatformRegistry{}
		mockPlatformRegistry.GetDeploymentPayloadReturns(nil, errors.New("seitan"))
		p := newPackagerForTest(t, mockPlatformRegistry, nil, false)
		args := []string{"outputFile"}
		chaincodePath = "testPath"
		chaincodeLang = "golang"
		newLifecycle = true

		err := p.packageChaincode(args)
		assert.Error(err)
		assert.Equal("error getting chaincode bytes: seitan", err.Error())
	})

	t.Run("writing the file fails", func(t *testing.T) {
		mockWriter := &mock.Writer{}
		mockWriter.WriteFileReturns(errors.New("quinoa"))
		p := newPackagerForTest(t, nil, mockWriter, false)
		args := []string{"outputFile"}
		chaincodePath = "testPath"
		chaincodeLang = "golang"
		newLifecycle = true

		err := p.packageChaincode(args)
		assert.Error(err)
		assert.Equal("error writing chaincode package to outputFile: quinoa", err.Error())
	})
}

func TestPackagerValidateInput(t *testing.T) {
	defer resetFlags()
	assert := assert.New(t)
	p := newPackagerForTest(t, nil, nil, false)

	t.Run("success - no unsupported flags set", func(t *testing.T) {
		resetFlags()
		chaincodePath = "testPath"
		chaincodeLang = "golang"
		p.setInput("outputFile")

		err := p.validateInput()
		assert.NoError(err)
	})

	t.Run("path not set", func(t *testing.T) {
		resetFlags()
		chaincodePath = ""
		chaincodeLang = "golang"
		p.setInput("outputFile")

		err := p.validateInput()
		assert.Error(err)
		assert.Equal("chaincode path must be set", err.Error())
	})

	t.Run("language not set", func(t *testing.T) {
		resetFlags()
		chaincodeLang = ""
		chaincodePath = "testPath"
		p.setInput("outputFile")

		err := p.validateInput()
		assert.Error(err)
		assert.Equal("chaincode language must be set", err.Error())
	})

	t.Run("name not supported", func(t *testing.T) {
		resetFlags()
		chaincodePath = "testPath"
		chaincodeLang = "golang"
		chaincodeName = "yeehaw"
		p.setInput("outputFile")

		err := p.validateInput()
		assert.Error(err)
		assert.Equal("chaincode name not supported by _lifecycle", err.Error())
	})

	t.Run("version not supported", func(t *testing.T) {
		resetFlags()
		chaincodePath = "testPath"
		chaincodeLang = "golang"
		chaincodeVersion = "hah"
		p.setInput("outputFile")

		err := p.validateInput()
		assert.Error(err)
		assert.Equal("chaincode version not supported by _lifecycle", err.Error())
	})

	t.Run("instantiation policy not supported", func(t *testing.T) {
		resetFlags()
		chaincodePath = "testPath"
		chaincodeLang = "golang"
		instantiationPolicy = "notachance"
		p.setInput("outputFile")

		err := p.validateInput()
		assert.Error(err)
		assert.Equal("instantiation policy not supported by _lifecycle", err.Error())
	})

	t.Run("signed package not supported", func(t *testing.T) {
		resetFlags()
		chaincodePath = "testPath"
		chaincodeLang = "golang"
		createSignedCCDepSpec = true
		p.setInput("outputFile")

		err := p.validateInput()
		assert.Error(err)
		assert.Equal("signed package not supported by _lifecycle", err.Error())
	})

	t.Run("signing of chaincode package not supported", func(t *testing.T) {
		resetFlags()
		chaincodePath = "testPath"
		chaincodeLang = "golang"
		signCCDepSpec = true
		p.setInput("outputFile")

		err := p.validateInput()
		assert.Error(err)
		assert.Equal("signing of chaincode package not supported by _lifecycle", err.Error())
	})
}

func TestPackageCmd(t *testing.T) {
	defer resetFlags()
	assert := assert.New(t)

	t.Run("success", func(t *testing.T) {
		resetFlags()
		chaincodePath = "testPath"
		chaincodeLang = "golang"
		outputFile := "testFile"
		newLifecycle = true

		p := newPackagerForTest(t, nil, nil, false)
		cmd := packageCmd(nil, nil, p)
		cmd.SetArgs([]string{outputFile})
		err := cmd.Execute()
		assert.NoError(err)
	})

	t.Run("invalid number of args", func(t *testing.T) {
		resetFlags()
		outputFile := "testFile"
		newLifecycle = true

		p := newPackagerForTest(t, nil, nil, false)
		cmd := packageCmd(nil, nil, p)
		cmd.SetArgs([]string{outputFile, "extraArg"})
		err := cmd.Execute()
		assert.Error(err)
		assert.Contains(err.Error(), "output file not specified or invalid number of args")
	})
}
