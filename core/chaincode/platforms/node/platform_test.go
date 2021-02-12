/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package node

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/hyperledger/fabric/core/chaincode/platforms/util"
	"github.com/hyperledger/fabric/core/config/configtest"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/require"
)

var platform = &Platform{}

type packageFile struct {
	packagePath string
	mode        int64
}

func TestValidatePath(t *testing.T) {
	err := platform.ValidatePath("there/is/no/way/this/path/exists")
	if err == nil {
		t.Fatalf("should have returned an error on non-existent chaincode path")
	} else if !strings.HasPrefix(err.Error(), "path to chaincode does not exist") {
		t.Fatalf("should have returned an error about chaincode path not existent, but got '%v'", err)
	}

	err = platform.ValidatePath("http://something bad/because/it/has/the/space")
	if err == nil {
		t.Fatalf("should have returned an error on an empty chaincode path")
	} else if !strings.HasPrefix(err.Error(), "invalid path") {
		t.Fatalf("should have returned an error about parsing the path, but got '%v'", err)
	}
}

func TestValidateCodePackage(t *testing.T) {
	err := platform.ValidateCodePackage([]byte("dummy CodePackage content"))
	if err == nil {
		t.Fatalf("should have returned an error on an invalid chaincode package")
	} else if !strings.HasPrefix(err.Error(), "failure opening codepackage gzip stream") {
		t.Fatalf("should have returned an error about opening the invalid archive, but got '%v'", err)
	}

	cp, err := makeCodePackage([]*packageFile{{"filename.txt", 0o100744}})
	if err != nil {
		t.Fatal(err)
	}

	err = platform.ValidateCodePackage(cp)
	if err == nil {
		t.Fatal("should have failed to validate because file in the archive is in the root folder instead of 'src'")
	} else if !strings.HasPrefix(err.Error(), "illegal file detected in payload") {
		t.Fatalf("should have returned error about illegal file detected, but got '%s'", err)
	}

	cp, err = makeCodePackage([]*packageFile{{"src/filename.txt", 0o100744}})
	if err != nil {
		t.Fatal(err)
	}

	err = platform.ValidateCodePackage(cp)
	if err == nil {
		t.Fatal("should have failed to validate because file in the archive is executable")
	} else if !strings.HasPrefix(err.Error(), "illegal file mode detected for file") {
		t.Fatalf("should have returned error about illegal file mode detected, but got '%s'", err)
	}

	cp, err = makeCodePackage([]*packageFile{{"src/filename.txt", 0o100666}})
	if err != nil {
		t.Fatal(err)
	}

	err = platform.ValidateCodePackage(cp)
	if err == nil {
		t.Fatal("should have failed to validate because no 'package.json' found")
	} else if !strings.HasPrefix(err.Error(), "no package.json found at the root of the chaincode package") {
		t.Fatalf("should have returned error about no package.json found, but got '%s'", err)
	}

	cp, err = makeCodePackage([]*packageFile{{"src/package.json", 0o100666}, {"META-INF/path/to/meta", 0o100744}})
	if err != nil {
		t.Fatal(err)
	}

	err = platform.ValidateCodePackage(cp)
	if err == nil {
		t.Fatalf("should have failed to validate because file in the archive is executable")
	} else if !strings.HasPrefix(err.Error(), "illegal file mode detected for file") {
		t.Fatalf("should have returned error about illegal file mode detected, but got '%s'", err)
	}
	cp, err = makeCodePackage([]*packageFile{{"src/package.json", 0o100666}, {"META-INF/path/to/meta", 0o100666}})
	if err != nil {
		t.Fatal(err)
	}

	err = platform.ValidateCodePackage(cp)
	if err != nil {
		t.Fatalf("should have returned no errors, but got '%s'", err)
	}
}

func TestGetDeploymentPayload(t *testing.T) {
	_, err := platform.GetDeploymentPayload("")
	if err == nil {
		t.Fatal("should have failed to product deployment payload due to empty chaincode path")
	} else if !strings.HasPrefix(err.Error(), "ChaincodeSpec's path cannot be empty") {
		t.Fatalf("should have returned error about path being empty, but got '%s'", err)
	}
}

func TestGenerateDockerfile(t *testing.T) {
	str, _ := platform.GenerateDockerfile()
	if !strings.Contains(str, "/fabric-nodeenv:") {
		t.Fatalf("should have generated a docker file using the fabric-nodeenv, but got %s", str)
	}

	if !strings.Contains(str, "ADD binpackage.tar /usr/local/src") {
		t.Fatalf("should have generated a docker file that adds code package content to /usr/local/src, but got %s", str)
	}
}

var expectedBuildScript = `
set -e
if [ -x /chaincode/build.sh ]; then
	/chaincode/build.sh
else
	cp -R /chaincode/input/src/. /chaincode/output && cd /chaincode/output && npm install --production
fi
`

func TestGenerateBuildOptions(t *testing.T) {
	opts, err := platform.DockerBuildOptions("pathname")
	require.NoError(t, err)

	expectedOpts := util.DockerBuildOptions{
		Image: "hyperledger/fabric-nodeenv:latest",
		Cmd:   expectedBuildScript,
	}
	require.Equal(t, expectedOpts, opts)
}

func makeCodePackage(pfiles []*packageFile) ([]byte, error) {
	contents := []byte("fake file's content")

	payload := bytes.NewBuffer(nil)
	gw := gzip.NewWriter(payload)
	tw := tar.NewWriter(gw)

	for _, f := range pfiles {
		if err := tw.WriteHeader(&tar.Header{
			Name: f.packagePath,
			Mode: f.mode,
			Size: int64(len(contents)),
		}); err != nil {
			return nil, fmt.Errorf("Error write header: %s", err)
		}

		if _, err := tw.Write(contents); err != nil {
			return nil, fmt.Errorf("Error writing contents: %s", err)
		}
	}

	// Write the tar file out
	if err := tw.Close(); err != nil {
		return nil, fmt.Errorf("Error writing Chaincode package contents: %s", err)
	}

	gw.Close()

	return payload.Bytes(), nil
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
