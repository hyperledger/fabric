/*
Copyright IBM Corp. 2016 All Rights Reserved.

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

package golang

import (
	"archive/tar"
	"bytes"
	"encoding/hex"
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/golang/protobuf/proto"
	"github.com/op/go-logging"
	"github.com/spf13/viper"

	cutil "github.com/hyperledger/fabric/core/container/util"
	"github.com/hyperledger/fabric/core/util"
	pb "github.com/hyperledger/fabric/protos"
)

var logger = logging.MustGetLogger("golang/hash")

//core hash computation factored out for testing
func computeHash(contents []byte, hash []byte) []byte {
	newSlice := make([]byte, len(hash)+len(contents))

	//copy the contents
	copy(newSlice[0:len(contents)], contents[:])

	//add the previous hash
	copy(newSlice[len(contents):], hash[:])

	//compute new hash
	hash = util.ComputeCryptoHash(newSlice)

	return hash
}

//hashFilesInDir computes h=hash(h,file bytes) for each file in a directory
//Directory entries are traversed recursively. In the end a single
//hash value is returned for the entire directory structure
func hashFilesInDir(rootDir string, dir string, hash []byte, tw *tar.Writer) ([]byte, error) {
	currentDir := filepath.Join(rootDir, dir)
	logger.Debugf("hashFiles %s", currentDir)
	//ReadDir returns sorted list of files in dir
	fis, err := ioutil.ReadDir(currentDir)
	if err != nil {
		return hash, fmt.Errorf("ReadDir failed %s\n", err)
	}
	for _, fi := range fis {
		name := filepath.Join(dir, fi.Name())
		if fi.IsDir() {
			var err error
			hash, err = hashFilesInDir(rootDir, name, hash, tw)
			if err != nil {
				return hash, err
			}
			continue
		}
		fqp := filepath.Join(rootDir, name)
		buf, err := ioutil.ReadFile(fqp)
		if err != nil {
			fmt.Printf("Error reading %s\n", err)
			return hash, err
		}

		//get the new hash from file contents
		hash = computeHash(buf, hash)

		if tw != nil {
			is := bytes.NewReader(buf)
			if err = cutil.WriteStreamToPackage(is, fqp, filepath.Join("src", name), tw); err != nil {
				return hash, fmt.Errorf("Error adding file to tar %s", err)
			}
		}
	}
	return hash, nil
}

func isCodeExist(tmppath string) error {
	file, err := os.Open(tmppath)
	if err != nil {
		return fmt.Errorf("Download failed %s", err)
	}

	fi, err := file.Stat()
	if err != nil {
		return fmt.Errorf("Could not stat file %s", err)
	}

	if !fi.IsDir() {
		return fmt.Errorf("File %s is not dir\n", file.Name())
	}

	return nil
}

func getCodeFromHTTP(path string) (codegopath string, err error) {
	codegopath = ""
	err = nil
	logger.Debugf("getCodeFromHTTP %s", path)

	// The following could be done with os.Getenv("GOPATH") but we need to change it later so this prepares for that next step
	env := os.Environ()
	var origgopath string
	var gopathenvIndex int
	for i, v := range env {
		if strings.Index(v, "GOPATH=") == 0 {
			p := strings.SplitAfter(v, "GOPATH=")
			origgopath = p[1]
			gopathenvIndex = i
			break
		}
	}
	if origgopath == "" {
		err = fmt.Errorf("GOPATH not defined")
		return
	}
	// Only take the first element of GOPATH
	gopath := filepath.SplitList(origgopath)[0]

	// Define a new gopath in which to download the code
	newgopath := filepath.Join(gopath, "_usercode_")

	//ignore errors.. _usercode_ might exist. TempDir will catch any other errors
	os.Mkdir(newgopath, 0755)

	if codegopath, err = ioutil.TempDir(newgopath, ""); err != nil {
		err = fmt.Errorf("could not create tmp dir under %s(%s)", newgopath, err)
		return
	}

	//go paths can have multiple dirs. We create a GOPATH with two source tree's as follows
	//
	//    <temporary empty folder to download chaincode source> : <local go path with OBC source>
	//
	//This approach has several goodness:
	// . Go will pick the first path to download user code (which we will delete after processing)
	// . GO will not download OBC as it is in the second path. GO will use the local OBC for generating chaincode image
	//     . network savings
	//     . more secure
	//     . as we are not downloading OBC, private, password-protected OBC repo's become non-issue

	env[gopathenvIndex] = "GOPATH=" + codegopath + string(os.PathListSeparator) + origgopath

	// Use a 'go get' command to pull the chaincode from the given repo
	logger.Debugf("go get %s", path)
	cmd := exec.Command("go", "get", path)
	cmd.Env = env
	var out bytes.Buffer
	cmd.Stdout = &out
	var errBuf bytes.Buffer
	cmd.Stderr = &errBuf //capture Stderr and print it on error
	err = cmd.Start()

	// Create a go routine that will wait for the command to finish
	done := make(chan error, 1)
	go func() {
		done <- cmd.Wait()
	}()

	select {
	case <-time.After(time.Duration(viper.GetInt("chaincode.deploytimeout")) * time.Millisecond):
		// If pulling repos takes too long, we should give up
		// (This can happen if a repo is private and the git clone asks for credentials)
		if err = cmd.Process.Kill(); err != nil {
			err = fmt.Errorf("failed to kill: %s", err)
		} else {
			err = errors.New("Getting chaincode took too long")
		}
	case err = <-done:
		// If we're here, the 'go get' command must have finished
		if err != nil {
			err = fmt.Errorf("'go get' failed with error: \"%s\"\n%s", err, string(errBuf.Bytes()))
		}
	}
	return
}

func getCodeFromFS(path string) (codegopath string, err error) {
	logger.Debugf("getCodeFromFS %s", path)
	gopath := os.Getenv("GOPATH")
	if gopath == "" {
		err = fmt.Errorf("GOPATH not defined")
		return
	}
	// Only take the first element of GOPATH
	codegopath = filepath.SplitList(gopath)[0]

	return
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

	//code root will point to the directory where the code exists
	//in the case of http it will be a temporary dir that
	//will have to be deleted
	var codegopath string

	var ishttp bool
	defer func() {
		if ishttp && codegopath != "" {
			os.RemoveAll(codegopath)
		}
	}()

	path := chaincodeID.Path

	var err error
	var actualcodepath string
	if strings.HasPrefix(path, "http://") {
		ishttp = true
		actualcodepath = path[7:]
		codegopath, err = getCodeFromHTTP(actualcodepath)
	} else if strings.HasPrefix(path, "https://") {
		ishttp = true
		actualcodepath = path[8:]
		codegopath, err = getCodeFromHTTP(actualcodepath)
	} else {
		actualcodepath = path
		codegopath, err = getCodeFromFS(path)
	}

	if err != nil {
		return "", fmt.Errorf("Error getting code %s", err)
	}

	tmppath := filepath.Join(codegopath, "src", actualcodepath)
	if err = isCodeExist(tmppath); err != nil {
		return "", fmt.Errorf("code does not exist %s", err)
	}
	ctorbytes, err := proto.Marshal(ctor)
	if err != nil {
		return "", fmt.Errorf("Error marshalling constructor: %s", err)
	}
	hash := util.GenerateHashFromSignature(actualcodepath, ctorbytes)

	hash, err = hashFilesInDir(filepath.Join(codegopath, "src"), actualcodepath, hash, tw)
	if err != nil {
		return "", fmt.Errorf("Could not get hashcode for %s - %s\n", path, err)
	}

	return hex.EncodeToString(hash[:]), nil
}
