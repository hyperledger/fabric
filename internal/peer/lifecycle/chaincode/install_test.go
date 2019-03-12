/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package chaincode

import (
	"io/ioutil"
	"os"
	"testing"

	"github.com/hyperledger/fabric/internal/peer/common"
	"github.com/hyperledger/fabric/internal/peer/lifecycle/chaincode/mock"
	pb "github.com/hyperledger/fabric/protos/peer"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
)

//go:generate counterfeiter -o mock/reader.go -fake-name Reader . reader
type reader interface {
	Reader
}

func initInstallTest(t *testing.T, fsPath string, ec pb.EndorserClient, mockResponse *pb.ProposalResponse) (*cobra.Command, *CmdFactory) {
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

	mockCF := &CmdFactory{
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

func TestInstallCmd(t *testing.T) {
	resetFlags()

	i, cleanup := newInstallerForTest(t, nil, nil)
	defer cleanup()
	cmd := installCmd(nil, i)
	i.Command = cmd
	args := []string{"pkgFile"}
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
			PackageFile: "chaincode-install-package.tar.gz",
		}

		err := i.install()
		assert.NoError(err)
	})

	t.Run("validatation fails", func(t *testing.T) {
		i, cleanup := newInstallerForTest(t, nil, nil)
		defer cleanup()
		i.Input = &InstallInput{}

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
			PackageFile: "pkgFile",
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
			PackageFile: "pkgFile",
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
			PackageFile: "chaincode-install-package.tar.gz",
		}

		err := i.validateInput()
		assert.NoError(err)
	})

	t.Run("package file not provided", func(t *testing.T) {
		i.Input = &InstallInput{}

		err := i.validateInput()
		assert.Error(err)
		assert.Equal("chaincode install package must be provided", err.Error())
	})
}
