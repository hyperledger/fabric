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
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/hyperledger/fabric/core/chaincode/platforms/util"
	"github.com/hyperledger/fabric/core/config/configtest"
	pb "github.com/hyperledger/fabric/protos/peer"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func testerr(err error, succ bool) error {
	if succ && err != nil {
		return fmt.Errorf("Expected success but got error %s", err)
	} else if !succ && err == nil {
		return fmt.Errorf("Expected failure but succeeded")
	}
	return nil
}

func writeBytesToPackage(name string, payload []byte, mode int64, tw *tar.Writer) error {
	//Make headers identical by using zero time
	var zeroTime time.Time
	tw.WriteHeader(&tar.Header{Name: name, Size: int64(len(payload)), ModTime: zeroTime, AccessTime: zeroTime, ChangeTime: zeroTime, Mode: mode})
	tw.Write(payload)

	return nil
}

func generateFakeCDS(ccname, path, file string, mode int64) (*pb.ChaincodeDeploymentSpec, error) {
	codePackage := bytes.NewBuffer(nil)
	gw := gzip.NewWriter(codePackage)
	tw := tar.NewWriter(gw)

	payload := make([]byte, 25, 25)
	err := writeBytesToPackage(file, payload, mode, tw)
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

type spec struct {
	CCName          string
	Path, File      string
	Mode            int64
	SuccessExpected bool
	RealGen         bool
}

func TestValidateCDS(t *testing.T) {
	platform := &Platform{}

	specs := make([]spec, 0)
	specs = append(specs, spec{CCName: "NoCode", Path: "path/to/nowhere", File: "/bin/warez", Mode: 0100400, SuccessExpected: false})
	specs = append(specs, spec{CCName: "NoCode", Path: "path/to/somewhere", File: "/src/path/to/somewhere/main.go", Mode: 0100400, SuccessExpected: true})
	specs = append(specs, spec{CCName: "NoCode", Path: "path/to/somewhere", File: "/bad-src/path/to/somewhere/main.go", Mode: 0100400, SuccessExpected: false})
	specs = append(specs, spec{CCName: "NoCode", Path: "path/to/somewhere", File: "/src/path/to/somewhere/warez", Mode: 0100555, SuccessExpected: false})
	specs = append(specs, spec{CCName: "NoCode", Path: "path/to/somewhere", File: "/META-INF/path/to/a/meta1", Mode: 0100555, SuccessExpected: false})
	specs = append(specs, spec{CCName: "NoCode", Path: "path/to/somewhere", File: "/META-Inf/path/to/a/meta2", Mode: 0100400, SuccessExpected: false})
	specs = append(specs, spec{CCName: "NoCode", Path: "path/to/somewhere", File: "META-INF/path/to/a/meta3", Mode: 0100400, SuccessExpected: true})

	for _, s := range specs {
		cds, err := generateFakeCDS(s.CCName, s.Path, s.File, s.Mode)

		err = platform.ValidateCodePackage(cds.CodePackage)
		if s.SuccessExpected == true && err != nil {
			t.Errorf("Unexpected failure: %s", err)
		}
		if s.SuccessExpected == false && err == nil {
			t.Log("Expected validation failure")
			t.Fail()
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

	source, err = findSource(gopath, "github.com/hyperledger/fabric/cmd/peer")
	if err != nil {
		t.Errorf("failed to find source: %s", err)
	}

	if _, ok := source["src/github.com/hyperledger/fabric/cmd/peer/main.go"]; !ok {
		t.Errorf("Failed to find expected source file: %v", source)
	}

	source, err = findSource(gopath, "acme.com/this/should/not/exist")
	if err == nil {
		t.Errorf("Success when failure was expected")
	}
}

func Test_DeploymentPayload(t *testing.T) {
	platform := &Platform{}

	payload, err := platform.GetDeploymentPayload("github.com/hyperledger/fabric/core/chaincode/platforms/golang/testdata/src/chaincodes/noop")
	assert.NoError(t, err)

	t.Logf("payload size: %d", len(payload))

	is := bytes.NewReader(payload)
	gr, err := gzip.NewReader(is)
	if err == nil {
		tr := tar.NewReader(gr)

		for {
			header, err := tr.Next()
			if err != nil {
				// We only get here if there are no more entries to scan
				break
			}

			t.Logf("%s (%d)", header.Name, header.Size)
		}
	}
}

func Test_DeploymentPayloadWithStateDBArtifacts(t *testing.T) {
	platform := &Platform{}

	payload, err := platform.GetDeploymentPayload("github.com/hyperledger/fabric/core/chaincode/platforms/golang/testdata/src/chaincodes/noopWithMETA")
	assert.NoError(t, err)

	t.Logf("payload size: %d", len(payload))

	is := bytes.NewReader(payload)
	gr, err := gzip.NewReader(is)
	if err == nil {
		tr := tar.NewReader(gr)

		var foundIndexArtifact bool
		for {
			header, err := tr.Next()
			if err != nil {
				// We only get here if there are no more entries to scan
				break
			}

			t.Logf("%s (%d)", header.Name, header.Size)
			if header.Name == "META-INF/statedb/couchdb/indexes/indexOwner.json" {
				foundIndexArtifact = true
			}
		}
		assert.Equal(t, true, foundIndexArtifact, "should have found statedb index artifact in noopWithMETA META-INF directory")
	}
}

func Test_decodeUrl(t *testing.T) {
	path := "http://example.com/foo/bar"
	if _, err := decodeUrl(path); err != nil {
		t.Fail()
		t.Logf("Error to decodeUrl unsuccessfully with valid path: %s, %s", path, err)
	}

	path = ""
	if _, err := decodeUrl(path); err == nil {
		t.Fail()
		t.Logf("Error to decodeUrl successfully with invalid path: %s", path)
	}

	path = "/"
	if _, err := decodeUrl(path); err == nil {
		t.Fatalf("Error to decodeUrl successfully with invalid path: %s", path)
	}

	path = "http:///"
	if _, err := decodeUrl(path); err == nil {
		t.Fatalf("Error to decodeUrl successfully with invalid path: %s", path)
	}
}

func TestValidatePath(t *testing.T) {
	platform := &Platform{}

	var tests = []struct {
		path string
		succ bool
	}{
		{path: "http://github.com/hyperledger/fabric/core/chaincode/platforms/golang/testdata/src/chaincodes/noop", succ: true},
		{path: "https://github.com/hyperledger/fabric/core/chaincode/platforms/golang/testdata/src/chaincodes/noop", succ: true},
		{path: "github.com/hyperledger/fabric/core/chaincode/platforms/golang/testdata/src/chaincodes/noop", succ: true},
		{path: "github.com/hyperledger/fabric/bad/chaincode/golang/testdata/src/chaincodes/noop", succ: false},
		{path: ":github.com/hyperledger/fabric/core/chaincode/platforms/golang/testdata/src/chaincodes/noop", succ: false},
	}

	for _, tst := range tests {
		err := platform.ValidatePath(tst.path)
		if err = testerr(err, tst.succ); err != nil {
			t.Errorf("Error validating chaincode spec: %s, %s", tst.path, err)
		}
	}
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
	testdataPath, err := filepath.Abs("testdata")
	require.NoError(t, err)

	platform := &Platform{}

	var tests = []struct {
		gopath string
		path   string
		succ   bool
	}{
		{gopath: testdataPath, path: "chaincodes/noop", succ: true},
		{gopath: testdataPath, path: "bad/chaincodes/noop", succ: false},
		{gopath: testdataPath, path: "chaincodes/BadImport", succ: false},
		{gopath: testdataPath, path: "chaincodes/BadMetadataInvalidIndex", succ: false},
		{gopath: testdataPath, path: "chaincodes/BadMetadataUnexpectedFolderContent", succ: false},
		{gopath: testdataPath, path: "chaincodes/BadMetadataIgnoreHiddenFile", succ: true},
		{gopath: testdataPath, path: "chaincodes/empty/", succ: false},
	}

	for _, tst := range tests {
		reset := updateGopath(t, tst.gopath)
		_, err := platform.GetDeploymentPayload(tst.path)
		t.Log(err)
		if err = testerr(err, tst.succ); err != nil {
			t.Errorf("Error validating chaincode spec: %s, %s", tst.path, err)
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
