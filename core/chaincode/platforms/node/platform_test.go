/*
# Copyright IBM Corp. All Rights Reserved.
#
# SPDX-License-Identifier: Apache-2.0
*/

package node

import (
	"archive/tar"
	"bufio"
	"bytes"
	"compress/gzip"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hyperledger/fabric/core/config/configtest"
	"github.com/hyperledger/fabric/protos/peer"
	"github.com/spf13/viper"
)

var platform = &Platform{}

type packageFile struct {
	packagePath string
	mode        int64
}

func TestValidateSpec(t *testing.T) {
	ccSpec := &peer.ChaincodeSpec{
		Type:        peer.ChaincodeSpec_NODE,
		ChaincodeId: &peer.ChaincodeID{Path: "there/is/no/way/this/path/exists"},
		Input:       &peer.ChaincodeInput{Args: [][]byte{[]byte("invoke")}}}

	err := platform.ValidateSpec(ccSpec)
	if err == nil {
		t.Fatalf("should have returned an error on non-existent chaincode path")
	} else if !strings.HasPrefix(err.Error(), "path to chaincode does not exist") {
		t.Fatalf("should have returned an error about chaincode path not existent, but got '%v'", err)
	}

	ccSpec.ChaincodeId.Path = "http://something bad/because/it/has/the/space"
	err = platform.ValidateSpec(ccSpec)
	if err == nil {
		t.Fatalf("should have returned an error on an empty chaincode path")
	} else if !strings.HasPrefix(err.Error(), "invalid path") {
		t.Fatalf("should have returned an error about parsing the path, but got '%v'", err)
	}

}

func TestValidateDeploymentSpec(t *testing.T) {
	cds := &peer.ChaincodeDeploymentSpec{
		ChaincodeSpec: &peer.ChaincodeSpec{
			Type:        peer.ChaincodeSpec_NODE,
			ChaincodeId: &peer.ChaincodeID{Path: "there/is/no/way/this/path/exists"},
			Input:       &peer.ChaincodeInput{Args: [][]byte{[]byte("invoke")}}},
		CodePackage: []byte("dummy CodePackage content")}

	err := platform.ValidateDeploymentSpec(cds)
	if err == nil {
		t.Fatalf("should have returned an error on an invalid chaincode package")
	} else if !strings.HasPrefix(err.Error(), "failure opening codepackage gzip stream") {
		t.Fatalf("should have returned an error about opening the invalid archive, but got '%v'", err)
	}

	content := []byte("temporary file's content")
	tmpfile, err := ioutil.TempFile("", "nodejs-chaincode-test")
	if err != nil {
		t.Fatal(err)
	}

	defer os.Remove(tmpfile.Name()) // clean up

	if _, err := tmpfile.Write(content); err != nil {
		t.Fatal(err)
	}
	if err := tmpfile.Close(); err != nil {
		t.Fatal(err)
	}

	cp, err := writeCodePackage(tmpfile.Name(), []*packageFile{{"filename.txt", 0100744}})
	if err != nil {
		t.Fatal(err)
	}

	cds.CodePackage = cp
	err = platform.ValidateDeploymentSpec(cds)
	if err == nil {
		t.Fatal("should have failed to validate because file in the archive is in the root folder instead of 'src'")
	} else if !strings.HasPrefix(err.Error(), "illegal file detected in payload") {
		t.Fatalf("should have returned error about illegal file detected, but got '%s'", err)
	}

	cp, err = writeCodePackage(tmpfile.Name(), []*packageFile{{"src/filename.txt", 0100744}})
	if err != nil {
		t.Fatal(err)
	}

	cds.CodePackage = cp
	err = platform.ValidateDeploymentSpec(cds)
	if err == nil {
		t.Fatal("should have failed to validate because file in the archive is executable")
	} else if !strings.HasPrefix(err.Error(), "illegal file mode detected for file") {
		t.Fatalf("should have returned error about illegal file mode detected, but got '%s'", err)
	}

	cp, err = writeCodePackage(tmpfile.Name(), []*packageFile{{"src/filename.txt", 0100666}})
	if err != nil {
		t.Fatal(err)
	}

	cds.CodePackage = cp
	err = platform.ValidateDeploymentSpec(cds)
	if err == nil {
		t.Fatal("should have failed to validate because no 'package.json' found")
	} else if !strings.HasPrefix(err.Error(), "no package.json found at the root of the chaincode package") {
		t.Fatalf("should have returned error about no package.json found, but got '%s'", err)
	}

	cp, err = writeCodePackage(tmpfile.Name(), []*packageFile{{"src/package.json", 0100666}})
	if err != nil {
		t.Fatal(err)
	}

	cds.CodePackage = cp
	err = platform.ValidateDeploymentSpec(cds)
	if err != nil {
		t.Fatalf("should have returned no errors, but got '%s'", err)
	}

	cp, err = writeCodePackage(tmpfile.Name(), []*packageFile{{"src/package.json", 0100666}, {"META-INF/path/to/meta", 0100744}})
	if err != nil {
		t.Fatal(err)
	}

	cds.CodePackage = cp
	err = platform.ValidateDeploymentSpec(cds)
	if err == nil {
		t.Fatalf("should have failed to validate because file in the archive is executable")
	} else if !strings.HasPrefix(err.Error(), "illegal file mode detected for file") {
		t.Fatalf("should have returned error about illegal file mode detected, but got '%s'", err)
	}

	cp, err = writeCodePackage(tmpfile.Name(), []*packageFile{{"src/package.json", 0100666}, {"META-INF/path/to/meta", 0100666}})
	if err != nil {
		t.Fatal(err)
	}

	cds.CodePackage = cp
	err = platform.ValidateDeploymentSpec(cds)
	if err != nil {
		t.Fatalf("should have returned no errors, but got '%s'", err)
	}
}

func TestGetDeploymentPayload(t *testing.T) {
	ccSpec := &peer.ChaincodeSpec{
		Type:        peer.ChaincodeSpec_NODE,
		ChaincodeId: &peer.ChaincodeID{Path: ""},
		Input:       &peer.ChaincodeInput{Args: [][]byte{[]byte("invoke")}}}

	_, err := platform.GetDeploymentPayload(ccSpec)
	if err == nil {
		t.Fatal("should have failed to product deployment payload due to empty chaincode path")
	} else if !strings.HasPrefix(err.Error(), "ChaincodeSpec's path cannot be empty") {
		t.Fatalf("should have returned error about path being empty, but got '%s'", err)
	}
}

func TestGenerateDockerfile(t *testing.T) {
	cds := &peer.ChaincodeDeploymentSpec{
		ChaincodeSpec: &peer.ChaincodeSpec{
			Type:        peer.ChaincodeSpec_NODE,
			ChaincodeId: &peer.ChaincodeID{Path: "there/is/no/way/this/path/exists"},
			Input:       &peer.ChaincodeInput{Args: [][]byte{[]byte("invoke")}}},
		CodePackage: []byte("dummy CodePackage content")}

	str, _ := platform.GenerateDockerfile(cds)
	if !strings.Contains(str, "/fabric-baseimage:") {
		t.Fatalf("should have generated a docker file using the fabric-baseimage, but got %s", str)
	}

	if !strings.Contains(str, "ADD binpackage.tar /usr/local/src") {
		t.Fatalf("should have generated a docker file that adds code package content to /usr/local/src, but got %s", str)
	}
}

func TestGenerateDockerBuild(t *testing.T) {
	dir, err := ioutil.TempDir("", "nodejs-chaincode-test")
	if err != nil {
		t.Fatal(err)
	}

	content := []byte(`
		{
		  "name": "fabric-shim-test",
		  "version": "1.0.0-snapshot",
	      "script": {
	        "start": "node chaincode.js"
	      },
		  "dependencies": {
		    "is-sorted": "*"
		  }
		}`)

	defer os.RemoveAll(dir) // clean up

	tmpfn := filepath.Join(dir, "package.json")
	if err := ioutil.WriteFile(tmpfn, content, 0666); err != nil {
		t.Fatal(err)
	}

	content = []byte(`
		const shim = require('fabric-shim');

		var chaincode = {};
		chaincode.Init = function(stub) {
			return Promise.resolve(shim.success());
		};

		chaincode.Invoke = function(stub) {
			console.log('Transaction ID: ' + stub.getTxID());

			return stub.getState('dummy')
			.then(() => {
				return shim.success();
			}, () => {
				return shim.error();
			});
		};

		shim.start(chaincode);`)

	tmpfn = filepath.Join(dir, "chaincode.js")
	if err := ioutil.WriteFile(tmpfn, content, 0666); err != nil {
		t.Fatal(err)
	}

	ccSpec := &peer.ChaincodeSpec{
		Type:        peer.ChaincodeSpec_NODE,
		ChaincodeId: &peer.ChaincodeID{Path: dir},
		Input:       &peer.ChaincodeInput{Args: [][]byte{[]byte("init")}}}

	cp, _ := platform.GetDeploymentPayload(ccSpec)

	cds := &peer.ChaincodeDeploymentSpec{
		ChaincodeSpec: ccSpec,
		CodePackage:   cp}

	payload := bytes.NewBuffer(nil)
	gw := gzip.NewWriter(payload)
	tw := tar.NewWriter(gw)

	err = platform.GenerateDockerBuild(cds, tw)
	if err != nil {
		t.Fatal(err)
	}
}

func writeCodePackage(file string, pfiles []*packageFile) ([]byte, error) {
	payload := bytes.NewBuffer(nil)
	gw := gzip.NewWriter(payload)
	tw := tar.NewWriter(gw)

	for _, f := range pfiles {
		if err := writeFileToPackage(file, f.packagePath, tw, f.mode); err != nil {
			return nil, fmt.Errorf("Error writing Chaincode package contents: %s", err)
		}
	}

	// Write the tar file out
	if err := tw.Close(); err != nil {
		return nil, fmt.Errorf("Error writing Chaincode package contents: %s", err)
	}

	tw.Close()
	gw.Close()

	return payload.Bytes(), nil
}

func writeFileToPackage(localpath string, packagepath string, tw *tar.Writer, mode int64) error {
	fd, err := os.Open(localpath)
	if err != nil {
		return fmt.Errorf("%s: %s", localpath, err)
	}
	defer fd.Close()

	is := bufio.NewReader(fd)
	info, err := os.Stat(localpath)
	if err != nil {
		return fmt.Errorf("%s: %s", localpath, err)
	}
	header, err := tar.FileInfoHeader(info, localpath)
	if err != nil {
		return fmt.Errorf("Error getting FileInfoHeader: %s", err)
	}

	//Let's take the variance out of the tar, make headers identical by using zero time
	oldname := header.Name
	header.Name = packagepath
	header.Mode = mode
	//header.Mode = 0100744

	if err = tw.WriteHeader(header); err != nil {
		return fmt.Errorf("Error write header for (path: %s, oldname:%s,newname:%s,sz:%d) : %s", localpath, oldname, packagepath, header.Size, err)
	}
	if _, err := io.Copy(tw, is); err != nil {
		return fmt.Errorf("Error copy (path: %s, oldname:%s,newname:%s,sz:%d) : %s", localpath, oldname, packagepath, header.Size, err)
	}

	return nil
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
