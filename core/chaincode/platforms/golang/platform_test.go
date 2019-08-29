// +build go1.9

/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package golang

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	pb "github.com/hyperledger/fabric-protos-go/peer"
	"github.com/hyperledger/fabric/core/chaincode/platforms/util"
	"github.com/hyperledger/fabric/core/config/configtest"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func generateFakeCDS(ccname, path, file string, mode int64) (*pb.ChaincodeDeploymentSpec, error) {
	codePackage := bytes.NewBuffer(nil)
	gw := gzip.NewWriter(codePackage)
	tw := tar.NewWriter(gw)

	payload := make([]byte, 25, 25)
	err := tw.WriteHeader(&tar.Header{Name: file, Mode: mode, Size: int64(len(payload))})
	if err != nil {
		return nil, err
	}
	_, err = tw.Write(payload)
	if err != nil {
		return nil, err
	}

	tw.Close()
	gw.Close()

	cds := &pb.ChaincodeDeploymentSpec{
		ChaincodeSpec: &pb.ChaincodeSpec{
			ChaincodeId: &pb.ChaincodeID{
				Name: ccname,
				Path: path,
			},
		},
		CodePackage: codePackage.Bytes(),
	}

	return cds, nil
}

func TestName(t *testing.T) {
	platform := &Platform{}
	assert.Equal(t, "GOLANG", platform.Name())
}

func TestValidatePath(t *testing.T) {
	var tests = []struct {
		path string
		succ bool
	}{
		{path: "http://github.com/hyperledger/fabric/core/chaincode/platforms/golang/testdata/src/chaincodes/noop", succ: false},
		{path: "https://github.com/hyperledger/fabric/core/chaincode/platforms/golang/testdata/src/chaincodes/noop", succ: false},
		{path: "github.com/hyperledger/fabric/core/chaincode/platforms/golang/testdata/src/chaincodes/noop", succ: true},
		{path: "github.com/hyperledger/fabric/bad/chaincode/golang/testdata/src/chaincodes/noop", succ: false},
		{path: ":github.com/hyperledger/fabric/core/chaincode/platforms/golang/testdata/src/chaincodes/noop", succ: false},
	}

	for _, tt := range tests {
		platform := &Platform{}
		err := platform.ValidatePath(tt.path)
		if tt.succ {
			assert.NoError(t, err, "expected %s to be a valid path", tt.path)
		} else {
			assert.Errorf(t, err, "expected %s to be an invalid path", tt.path)
		}
	}
}

func TestValidateCodePackage(t *testing.T) {
	tests := []struct {
		name            string
		path            string
		file            string
		mode            int64
		successExpected bool
	}{
		{name: "NoCode", path: "path/to/somewhere", file: "/src/path/to/somewhere/main.go", mode: 0100400, successExpected: true},
		{name: "NoCode", path: "path/to/somewhere", file: "/src/path/to/somewhere/warez", mode: 0100555, successExpected: false},
		{name: "NoCode", path: "path/to/somewhere", file: "/META-INF/path/to/a/meta1", mode: 0100555, successExpected: false},
		{name: "NoCode", path: "path/to/somewhere", file: "META-INF/path/to/a/meta3", mode: 0100400, successExpected: true},
	}

	for _, tt := range tests {
		cds, err := generateFakeCDS(tt.name, tt.path, tt.file, tt.mode)
		assert.NoError(t, err, "failed to generate fake cds")

		platform := &Platform{}
		err = platform.ValidateCodePackage(cds.CodePackage)
		if tt.successExpected {
			assert.NoError(t, err, "expected success for path: %s, file: %s", tt.path, tt.file)
		} else {
			assert.Errorf(t, err, "expected error for path: %s, file: %s", tt.path, tt.file)
		}
	}
}

func TestPlatform_GoPathNotSet(t *testing.T) {
	gopath := os.Getenv("GOPATH")
	defer os.Setenv("GOPATH", gopath)
	os.Setenv("GOPATH", "")

	// Go 1.9 sets GOPATH to $HOME/go if GOPATH is not set
	defaultGopath := filepath.Join(os.Getenv("HOME"), "go")
	currentGopath, err := getGopath()
	assert.NoError(t, err, "Expected default GOPATH")
	assert.Equal(t, defaultGopath, currentGopath)
}

func Test_findSource(t *testing.T) {
	gopath, err := getGopath()
	if err != nil {
		t.Errorf("failed to get GOPATH: %s", err)
	}

	var source SourceMap

	source, err = findSource(CodeDescriptor{Gopath: gopath, Pkg: "github.com/hyperledger/fabric/cmd/peer"})
	if err != nil {
		t.Errorf("failed to find source: %s", err)
	}

	if _, ok := source["src/github.com/hyperledger/fabric/cmd/peer/main.go"]; !ok {
		t.Errorf("Failed to find expected source file: %v", source)
	}

	source, err = findSource(CodeDescriptor{Gopath: gopath, Pkg: "acme.com/this/should/not/exist"})
	if err == nil {
		t.Errorf("Success when failure was expected")
	}
}

func TestDeploymentPayload(t *testing.T) {
	platform := &Platform{}

	payload, err := platform.GetDeploymentPayload("github.com/hyperledger/fabric/core/chaincode/platforms/golang/testdata/src/chaincodes/noop")
	assert.NoError(t, err)

	is := bytes.NewReader(payload)
	gr, err := gzip.NewReader(is)
	assert.NoError(t, err, "failed to create new gzip reader")
	tr := tar.NewReader(gr)

	var foundIndexArtifact bool
	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		assert.NoError(t, err, "unexpected error while processing package")

		if header.Name == "META-INF/statedb/couchdb/indexes/indexOwner.json" {
			foundIndexArtifact = true
			break
		}
	}
	assert.Equal(t, true, foundIndexArtifact, "should have found statedb index artifact in noop META-INF directory")
}

func updateGopath(t *testing.T, path string) func() {
	initialGopath, set := os.LookupEnv("GOPATH")

	if path == "" {
		err := os.Unsetenv("GOPATH")
		require.NoError(t, err)
	} else {
		err := os.Setenv("GOPATH", path)
		require.NoError(t, err)
	}

	if !set {
		return func() { os.Unsetenv("GOPATH") }
	}
	return func() { os.Setenv("GOPATH", initialGopath) }
}

func TestGetDeploymentPayload(t *testing.T) {
	const testdata = "github.com/hyperledger/fabric/core/chaincode/platforms/golang/testdata/src/"

	gopath, err := getGopath()
	require.NoError(t, err)

	platform := &Platform{}

	var tests = []struct {
		gopath string
		path   string
		succ   bool
	}{
		{gopath: gopath, path: testdata + "chaincodes/noop", succ: true},
		{gopath: gopath, path: testdata + "bad/chaincodes/noop", succ: false},
		{gopath: gopath, path: testdata + "chaincodes/BadImport", succ: false},
		{gopath: gopath, path: testdata + "chaincodes/BadMetadataInvalidIndex", succ: false},
		{gopath: gopath, path: testdata + "chaincodes/BadMetadataUnexpectedFolderContent", succ: false},
		{gopath: gopath, path: testdata + "chaincodes/BadMetadataIgnoreHiddenFile", succ: true},
		{gopath: gopath, path: testdata + "chaincodes/empty/", succ: false},
	}

	for _, tst := range tests {
		reset := updateGopath(t, tst.gopath)
		_, err := platform.GetDeploymentPayload(tst.path)
		if tst.succ {
			assert.NoError(t, err, "expected success for path: %s", tst.path)
		} else {
			assert.Errorf(t, err, "expected error for path: %s", tst.path)
		}
		reset()
	}
}

//TestGetLDFlagsOpts tests handling of chaincode.golang.dynamicLink
func TestGetLDFlagsOpts(t *testing.T) {
	viper.Set("chaincode.golang.dynamicLink", true)
	if getLDFlagsOpts() != dynamicLDFlagsOpts {
		t.Error("Error handling chaincode.golang.dynamicLink configuration. ldflags should be for dynamic linkink")
	}
	viper.Set("chaincode.golang.dynamicLink", false)
	if getLDFlagsOpts() != staticLDFlagsOpts {
		t.Error("Error handling chaincode.golang.dynamicLink configuration. ldflags should be for static linkink")
	}
}

func TestDockerBuildOptions(t *testing.T) {
	platform := &Platform{}

	opts, err := platform.DockerBuildOptions("the-path")
	assert.NoError(t, err, "unexpected error from DockerBuildOptions")

	expectedOpts := util.DockerBuildOptions{
		Cmd: "GOPATH=/chaincode/input:$GOPATH go build  -ldflags \"-linkmode external -extldflags '-static'\" -o /chaincode/output/chaincode the-path",
	}
	assert.Equal(t, expectedOpts, opts)
}

func TestMain(m *testing.M) {
	viper.SetConfigName("core")
	viper.SetEnvPrefix("CORE")
	configtest.AddDevConfigPath(nil)
	viper.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	viper.AutomaticEnv()
	if err := viper.ReadInConfig(); err != nil {
		fmt.Printf("could not read config %s\n", err)
		os.Exit(-1)
	}
	os.Exit(m.Run())
}
