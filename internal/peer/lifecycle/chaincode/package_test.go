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

	"github.com/hyperledger/fabric/internal/peer/common"
	"github.com/hyperledger/fabric/internal/peer/lifecycle/chaincode/mock"
	"github.com/hyperledger/fabric/internal/pkg/identity"
	msptesttools "github.com/hyperledger/fabric/msp/mgmt/testtools"
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

func mockChaincodeCmdFactoryForTest(sign bool) (*CmdFactory, error) {
	var signer identity.SignerSerializer
	var err error
	if sign {
		signer, err = common.GetDefaultSigner()
		if err != nil {
			return nil, fmt.Errorf("Get default signer error: %v", err)
		}
	}

	cf := &CmdFactory{Signer: signer}
	return cf, nil
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
		packageLabel = "label"

		err := p.packageChaincode(args)
		assert.NoError(err)
	})

	t.Run("input validation failure", func(t *testing.T) {
		resetFlags()

		p := newPackagerForTest(t, nil, nil, false)
		args := []string{"output"}
		chaincodePath = ""
		chaincodeLang = "golang"

		err := p.packageChaincode(args)
		assert.Error(err)
		assert.Equal("chaincode path must be set", err.Error())
	})

	t.Run("getting the chaincode bytes fails", func(t *testing.T) {
		resetFlags()

		mockPlatformRegistry := &mock.PlatformRegistry{}
		mockPlatformRegistry.GetDeploymentPayloadReturns(nil, errors.New("seitan"))
		p := newPackagerForTest(t, mockPlatformRegistry, nil, false)
		args := []string{"outputFile"}
		chaincodePath = "testPath"
		chaincodeLang = "golang"
		packageLabel = "label"

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
		packageLabel = "label"

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
		packageLabel = "label"
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

}

func TestPackageCmd(t *testing.T) {
	defer resetFlags()
	assert := assert.New(t)

	t.Run("success", func(t *testing.T) {
		resetFlags()
		chaincodePath = "testPath"
		chaincodeLang = "golang"
		packageLabel = "label"
		outputFile := "testFile"

		p := newPackagerForTest(t, nil, nil, false)
		cmd := packageCmd(nil, p)
		cmd.SetArgs([]string{outputFile})
		err := cmd.Execute()
		assert.NoError(err)
	})

	t.Run("invalid number of args", func(t *testing.T) {
		resetFlags()
		outputFile := "testFile"

		p := newPackagerForTest(t, nil, nil, false)
		cmd := packageCmd(nil, p)
		cmd.SetArgs([]string{outputFile, "extraArg"})
		err := cmd.Execute()
		assert.Error(err)
		assert.Contains(err.Error(), "output file not specified or invalid number of args")
	})
}
