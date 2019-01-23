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

	"github.com/hyperledger/fabric/peer/chaincode/mock"
	"github.com/hyperledger/fabric/peer/common"
	pb "github.com/hyperledger/fabric/protos/peer"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
)

//go:generate counterfeiter -o mock/reader.go -fake-name Reader . reader
type reader interface {
	Reader
}

func initInstallTest(t *testing.T, fsPath string, ec pb.EndorserClient, mockResponse *pb.ProposalResponse) (*cobra.Command, *ChaincodeCmdFactory) {
	viper.Set("peer.fileSystemPath", fsPath)

	signer, err := common.GetDefaultSigner()
	if err != nil {
		t.Fatalf("Get default signer error: %v", err)
	}

	if mockResponse == nil {
		mockResponse = &pb.ProposalResponse{
			Response:    &pb.Response{Status: 200},
			Endorsement: &pb.Endorsement{},
		}
	}
	if ec == nil {
		ec = common.GetMockEndorserClient(mockResponse, nil)
	}

	mockCF := &ChaincodeCmdFactory{
		Signer:          signer,
		EndorserClients: []pb.EndorserClient{ec},
	}

	cmd := installCmd(mockCF, nil)
	addFlags(cmd)

	return cmd, mockCF
}

func cleanupInstallTest(fsPath string) {
	os.RemoveAll(fsPath)
}

func TestInstallLegacyBadVersion(t *testing.T) {
	fsPath, err := ioutil.TempDir("", "installbadversion")
	assert.NoError(t, err)

	cmd, _ := initInstallTest(t, fsPath, nil, nil)
	defer cleanupInstallTest(fsPath)

	args := []string{"-n", "mychaincode", "-p", "github.com/hyperledger/fabric/peer/chaincode/testdata/src/chaincodes/noop"}
	cmd.SetArgs(args)

	if err := cmd.Execute(); err == nil {
		t.Fatal("Expected error executing install command for version not specified")
	}
}

func TestInstallLegacyNonExistentCC(t *testing.T) {
	fsPath, err := ioutil.TempDir("", "install-nonexistentcc")
	assert.NoError(t, err)

	cmd, _ := initInstallTest(t, fsPath, nil, nil)
	defer cleanupInstallTest(fsPath)

	args := []string{"-n", "badmychaincode", "-p", "github.com/hyperledger/fabric/peer/chaincode/testdata/src/chaincodes/bad_mychaincode", "-v", "testversion"}
	cmd.SetArgs(args)

	if err := cmd.Execute(); err == nil {
		t.Fatal("Expected error executing install command for bad chaincode")
	}

	if _, err := os.Stat(fsPath + "/chaincodes/badmychaincode.testversion"); err == nil {
		t.Fatal("chaincode mychaincode.testversion should not exist")
	}
}

func TestInstallLegacyFromPackage(t *testing.T) {
	pdir := newTempDir()
	defer os.RemoveAll(pdir)

	ccpackfile := pdir + "/ccpack.file"
	err := createSignedCDSPackage(t, []string{"-n", "somecc", "-p", "some/go/package", "-v", "0", ccpackfile}, false)
	if err != nil {
		t.Fatalf("could not create package :%v", err)
	}

	fsPath := "/tmp/installtest"

	cmd, mockCF := initInstallTest(t, fsPath, nil, nil)
	defer cleanupInstallTest(fsPath)

	mockResponse := &pb.ProposalResponse{
		Response:    &pb.Response{Status: 200},
		Endorsement: &pb.Endorsement{},
	}
	mockEndorserClient := common.GetMockEndorserClient(mockResponse, nil)
	mockCF.EndorserClients = []pb.EndorserClient{mockEndorserClient}

	args := []string{ccpackfile}
	cmd.SetArgs(args)

	if err := cmd.Execute(); err != nil {
		t.Fatal("error executing install command from package")
	}
}

func TestInstallLegacyFromBadPackage(t *testing.T) {
	pdir := newTempDir()
	defer os.RemoveAll(pdir)

	ccpackfile := pdir + "/ccpack.file"
	err := ioutil.WriteFile(ccpackfile, []byte("really bad CC package"), 0700)
	if err != nil {
		t.Fatalf("could not create package :%v", err)
	}

	fsPath := "/tmp/installtest"

	cmd, _ := initInstallTest(t, fsPath, nil, nil)
	defer cleanupInstallTest(fsPath)

	args := []string{ccpackfile}
	cmd.SetArgs(args)

	if err := cmd.Execute(); err == nil {
		t.Fatal("expected error installing bad package")
	}
}

func installLegacyCC(t *testing.T) error {
	defer viper.Reset()

	fsPath, err := ioutil.TempDir("", "installLegacyEx02")
	assert.NoError(t, err)
	cmd, _ := initInstallTest(t, fsPath, nil, nil)
	defer cleanupInstallTest(fsPath)

	args := []string{"-n", "mychaincode", "-p", "github.com/hyperledger/fabric/peer/chaincode/testdata/src/chaincodes/noop", "-v", "anotherversion"}
	cmd.SetArgs(args)

	if err := cmd.Execute(); err != nil {
		return fmt.Errorf("Run chaincode upgrade cmd error:%v", err)
	}

	return nil
}

func TestInstallLegacy(t *testing.T) {
	if err := installLegacyCC(t); err != nil {
		t.Fatalf("Install failed with error: %v", err)
	}
}

func TestInstallCmd(t *testing.T) {
	resetFlags()

	i, cleanup := newInstallerForTest(t, nil, nil)
	defer cleanup()
	cmd := installCmd(nil, i)
	i.Command = cmd
	args := []string{"--newLifecycle", "-n", "testcc", "-v", "2.0", "pkgFile"}
	cmd.SetArgs(args)

	err := cmd.Execute()
	assert.NoError(t, err)
}

func TestInstaller(t *testing.T) {
	assert := assert.New(t)

	t.Run("success", func(t *testing.T) {
		i, cleanup := newInstallerForTest(t, nil, nil)
		defer cleanup()

		i.Input = &InstallInput{
			Name:        "testcc",
			Version:     "test.0",
			PackageFile: "chaincode-install-package.tar.gz",
		}

		err := i.install()
		assert.NoError(err)
	})

	t.Run("validatation fails", func(t *testing.T) {
		i, cleanup := newInstallerForTest(t, nil, nil)
		defer cleanup()
		i.Input = &InstallInput{
			Name:    "testcc",
			Version: "test.0",
		}

		err := i.install()
		assert.Error(err)
		assert.Equal("chaincode install package must be provided", err.Error())
	})

	t.Run("reading the package file fails", func(t *testing.T) {
		mockReader := &mock.Reader{}
		mockReader.ReadFileReturns(nil, errors.New("balloon"))

		i, cleanup := newInstallerForTest(t, mockReader, nil)
		defer cleanup()
		i.Input = &InstallInput{
			Name:        "testcc",
			Version:     "test.0",
			PackageFile: "pkgFile",
		}

		err := i.install()
		assert.Error(err)
		assert.Equal("error reading chaincode package at pkgFile: balloon", err.Error())
	})

	t.Run("endorser client returns error", func(t *testing.T) {
		ec := common.GetMockEndorserClient(nil, errors.New("blimp"))
		i, cleanup := newInstallerForTest(t, nil, ec)
		defer cleanup()
		i.Input = &InstallInput{
			Name:        "testcc",
			Version:     "test.0",
			PackageFile: "pkgFile",
		}

		err := i.install()
		assert.Error(err)
		assert.Equal("error endorsing chaincode install: blimp", err.Error())
	})

	t.Run("endorser client returns a proposal response with nil response", func(t *testing.T) {
		mockResponse := &pb.ProposalResponse{
			Response: nil,
		}
		ec := common.GetMockEndorserClient(mockResponse, nil)
		i, cleanup := newInstallerForTest(t, nil, ec)
		defer cleanup()
		i.Input = &InstallInput{
			Name:        "testcc",
			Version:     "test.0",
			PackageFile: "pkgFile",
		}

		err := i.install()
		assert.Error(err)
		assert.Equal("error during install: received proposal response with nil response", err.Error())
	})

	t.Run("endorser client returns a failure status code", func(t *testing.T) {
		mockResponse := &pb.ProposalResponse{
			Response: &pb.Response{
				Status:  500,
				Message: "dangerdanger",
			},
		}
		ec := common.GetMockEndorserClient(mockResponse, nil)
		i, cleanup := newInstallerForTest(t, nil, ec)
		defer cleanup()
		i.Input = &InstallInput{
			Name:        "testcc",
			Version:     "test.0",
			PackageFile: "pkgFile",
		}

		err := i.install()
		assert.Error(err)
		assert.Equal("install failed with status: 500 - dangerdanger", err.Error())
	})

	t.Run("endorser client returns nil proposal response", func(t *testing.T) {
		ec := common.GetMockEndorserClient(nil, nil)
		i, cleanup := newInstallerForTest(t, nil, ec)
		defer cleanup()
		i.Input = &InstallInput{
			Name:         "testcc",
			Version:      "test.0",
			PackageFile:  "pkgFile",
			NewLifecycle: true,
		}

		err := i.install()
		assert.Error(err)
		assert.Contains(err.Error(), "error during install: received nil proposal response")
	})

	t.Run("unexpected proposal response response payload", func(t *testing.T) {
		mockResponse := &pb.ProposalResponse{
			Response: &pb.Response{
				Status:  200,
				Payload: []byte("supplies!")},
		}
		ec := common.GetMockEndorserClient(mockResponse, nil)
		i, cleanup := newInstallerForTest(t, nil, ec)
		defer cleanup()
		i.Input = &InstallInput{
			Name:         "testcc",
			Version:      "test.0",
			PackageFile:  "pkgFile",
			NewLifecycle: true,
		}

		err := i.install()
		assert.Error(err)
		assert.Contains(err.Error(), "error unmarshaling proposal response's response payload")
	})
}

func TestValidateInput(t *testing.T) {
	assert := assert.New(t)
	i, cleanup := newInstallerForTest(t, nil, nil)
	defer cleanup()

	t.Run("success", func(t *testing.T) {
		i.Input = &InstallInput{
			Name:        "testcc",
			Version:     "test.0",
			PackageFile: "chaincode-install-package.tar.gz",
		}

		err := i.validateInput()
		assert.NoError(err)
	})

	t.Run("package file not provided", func(t *testing.T) {
		i.Input = &InstallInput{
			Name:    "testcc",
			Version: "test.0",
		}

		err := i.validateInput()
		assert.Error(err)
		assert.Equal("chaincode install package must be provided", err.Error())
	})

	t.Run("chaincode name not specified", func(t *testing.T) {
		i.Input = &InstallInput{
			Version:     "test.0",
			PackageFile: "testPkg",
		}

		err := i.validateInput()
		assert.Error(err)
		assert.Equal("chaincode name must be specified", err.Error())
	})

	t.Run("chaincode version not specified", func(t *testing.T) {
		i.Input = &InstallInput{
			Name:        "testcc",
			PackageFile: "testPkg",
		}

		err := i.validateInput()
		assert.Error(err)
		assert.Equal("chaincode version must be specified", err.Error())
	})

	t.Run("chaincode path specified but not supported by _lifecycle", func(t *testing.T) {
		i.Input = &InstallInput{
			Name:        "testcc",
			Version:     "test.0",
			PackageFile: "testPkg",
			Path:        "test/path",
		}

		err := i.validateInput()
		assert.Error(err)
		assert.Equal("chaincode path parameter not supported by _lifecycle", err.Error())
	})
}

func newInstallerForTest(t *testing.T, r Reader, ec pb.EndorserClient) (installer *Installer, cleanup func()) {
	fsPath, err := ioutil.TempDir("", "installerForTest")
	assert.NoError(t, err)
	_, mockCF := initInstallTest(t, fsPath, ec, nil)
	if r == nil {
		r = &mock.Reader{}
	}

	i := &Installer{
		Reader:          r,
		EndorserClients: mockCF.EndorserClients,
		Signer:          mockCF.Signer,
	}

	cleanupFunc := func() {
		cleanupInstallTest(fsPath)
	}

	return i, cleanupFunc
}
