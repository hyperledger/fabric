/*
Copyright DTCC 2016 All Rights Reserved.

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

package java

import (
	"archive/tar"
	"bytes"
	"encoding/hex"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"strings"

	"github.com/golang/protobuf/proto"
	cutil "github.com/hyperledger/fabric/core/container/util"
	"github.com/hyperledger/fabric/core/util"
	pb "github.com/hyperledger/fabric/protos"
)

//hashFilesInDir computes h=hash(h,file bytes) for each file in a directory
//Directory entries are traversed recursively. In the end a single
//hash value is returned for the entire directory structure
func hashFilesInDir(cutoff string, dir string, hash []byte, tw *tar.Writer) ([]byte, error) {
	//ReadDir returns sorted list of files in dir
	fis, err := ioutil.ReadDir(dir)
	if err != nil {
		return hash, fmt.Errorf("ReadDir failed %s\n", err)
	}
	for _, fi := range fis {
		name := fmt.Sprintf("%s/%s", dir, fi.Name())
		if fi.IsDir() {
			var err error
			hash, err = hashFilesInDir(cutoff, name, hash, tw)
			if err != nil {
				return hash, err
			}
			continue
		}
		buf, err := ioutil.ReadFile(name)
		if err != nil {
			fmt.Printf("Error reading %s\n", err)
			return hash, err
		}

		newSlice := make([]byte, len(hash)+len(buf))
		copy(newSlice[len(buf):], hash[:])
		//hash = md5.Sum(newSlice)
		hash = util.ComputeCryptoHash(newSlice)

		if tw != nil {
			is := bytes.NewReader(buf)
			if err = cutil.WriteStreamToPackage(is, name, name[len(cutoff):], tw); err != nil {
				return hash, fmt.Errorf("Error adding file to tar %s", err)
			}
		}
	}
	return hash, nil
}

func isCodeExist(tmppath string) error {
	file, err := os.Open(tmppath)
	if err != nil {
		return fmt.Errorf("Download failer %s", err)
	}
	fi, err := file.Stat()
	if err != nil {
		return fmt.Errorf("could not stat file %s", err)
	}
	if !fi.IsDir() {
		return fmt.Errorf("file %s is not dir\n", file.Name())
	}
	return nil
}

func getCodeFromHTTP(path string) (codegopath string, err error) {

	var tmp string
	tmp, err = ioutil.TempDir("", "javachaincode")

	if err != nil {
		return "", fmt.Errorf("Error creating temporary file: %s", err)
	}
	var out bytes.Buffer

	cmd := exec.Command("git", "clone", path, tmp)
	cmd.Stderr = &out
	cmderr := cmd.Run()
	if cmderr != nil {
		return "", fmt.Errorf("Error cloning git repository %s", cmderr)
	}

	return tmp, nil

}

//generateHashcode gets hashcode of the code under path. If path is a HTTP(s) url
//it downloads the code first to compute the hash.
//NOTE: for dev mode, user builds and runs chaincode manually. The name provided
//by the user is equivalent to the path. This method will treat the name
//as codebytes and compute the hash from it. ie, user cannot run the chaincode
//with the same (name, ctor, args)
func generateHashcode(spec *pb.ChaincodeSpec, tw *tar.Writer) (string, error) {
	if spec == nil {
		return "", fmt.Errorf("Cannot generate hashcode from nil spec")
	}

	chaincodeID := spec.ChaincodeID
	if chaincodeID == nil || chaincodeID.Path == "" {
		return "", fmt.Errorf("Cannot generate hashcode from empty chaincode path")
	}

	ctor := spec.CtorMsg
	if ctor == nil || len(ctor.Args) == 0 {
		return "", fmt.Errorf("Cannot generate hashcode from empty ctor")
	}

	codepath := chaincodeID.Path

	var ishttp bool
	defer func() {
		if ishttp {
			os.RemoveAll(codepath)
		}
	}()

	var err error
	if strings.HasPrefix(codepath, "http://") ||
		strings.HasPrefix(codepath, "https://") {
		ishttp = true
		codepath, err = getCodeFromHTTP(codepath)
	} else if !strings.HasPrefix(codepath, "/") {
		wd := ""
		wd, err = os.Getwd()
		codepath = wd + "/" + codepath
	}

	if err != nil {
		return "", fmt.Errorf("Error getting code %s", err)
	}

	if err = isCodeExist(codepath); err != nil {
		return "", fmt.Errorf("code does not exist %s", err)
	}

	root := codepath
	if strings.LastIndex(root, "/") == len(root)-1 {
		root = root[:len(root)-1]
	}
	root = root[:strings.LastIndex(root, "/")+1]
	ctorbytes, err := proto.Marshal(ctor)
	if err != nil {
		return "", fmt.Errorf("Error marshalling constructor: %s", err)
	}
	hash := util.GenerateHashFromSignature(codepath, ctorbytes)

	hash, err = hashFilesInDir(root, codepath, hash, tw)
	if err != nil {
		return "", fmt.Errorf("Could not get hashcode for %s - %s\n", codepath, err)
	}

	return hex.EncodeToString(hash[:]), nil
}
