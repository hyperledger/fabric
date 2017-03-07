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
	"errors"
	"fmt"

	"bytes"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/hyperledger/fabric/common/flogging"
	ccutil "github.com/hyperledger/fabric/core/chaincode/platforms/util"
	pb "github.com/hyperledger/fabric/protos/peer"
	"github.com/spf13/viper"
)

var includeFileTypes = map[string]bool{
	".c":    true,
	".h":    true,
	".go":   true,
	".yaml": true,
	".json": true,
}

var logger = flogging.MustGetLogger("golang-platform")

func getCodeFromHTTP(path string) (codegopath string, err error) {
	codegopath = ""
	err = nil
	logger.Debugf("getCodeFromHTTP %s", path)

	env := getEnv()
	gopath, err := getGopath()
	if err != nil {
		return
	}

	// Define a new gopath in which to download the code
	newgopath := filepath.Join(gopath, "_usercode_")

	//ignore errors.. _usercode_ might exist. TempDir will catch any other errors
	os.Mkdir(newgopath, 0755)

	if codegopath, err = ioutil.TempDir(newgopath, ""); err != nil {
		err = fmt.Errorf("could not create tmp dir under %s(%s)", newgopath, err)
		return
	}
	defer func() {
		// Clean up after ourselves IFF we return a failure in the future
		if err != nil {
			os.RemoveAll(codegopath)
		}
	}()

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

	origgopath := env["GOPATH"]
	env["GOPATH"] = codegopath + string(os.PathListSeparator) + origgopath

	// Use a 'go get' command to pull the chaincode from the given repo
	logger.Debugf("go get %s", path)
	cmd := exec.Command("go", "get", path)
	cmd.Env = flattenEnv(env)
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

	ccDeployTimeout := viper.GetDuration("chaincode.deploytimeout")
	if ccDeployTimeout < time.Duration(30)*time.Second {
		logger.Warningf("Invalid chaincode deploy timeout value %s (should be at least 30s); defaulting to 30s", ccDeployTimeout)
		ccDeployTimeout = time.Duration(30) * time.Second
	} else {
		logger.Debugf("Setting chaincode deploy timeout to %s", ccDeployTimeout)
	}
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

func getCodeFromFS(path string) (string, error) {
	logger.Debugf("getCodeFromFS %s", path)
	gopath, err := getGopath()
	if err != nil {
		return "", err
	}

	tmppath := filepath.Join(gopath, "src", path)
	if err := ccutil.IsCodeExist(tmppath); err != nil {
		return "", fmt.Errorf("code does not exist %s", err)
	}

	return gopath, nil
}

type CodeDescriptor struct {
	Gopath, Pkg string
	Cleanup     func()
}

// collectChaincodeFiles collects chaincode files. If path is a HTTP(s) url it
// downloads the code first.
//
//NOTE: for dev mode, user builds and runs chaincode manually. The name provided
//by the user is equivalent to the path.
func getCode(spec *pb.ChaincodeSpec) (*CodeDescriptor, error) {
	if spec == nil {
		return nil, errors.New("Cannot collect files from nil spec")
	}

	chaincodeID := spec.ChaincodeId
	if chaincodeID == nil || chaincodeID.Path == "" {
		return nil, errors.New("Cannot collect files from empty chaincode path")
	}

	var err error

	//code root will point to the directory where the code exists
	//in the case of http it will be a temporary dir that
	//will have to be deleted
	var gopath string
	var ishttp bool
	cleanup := func() {
		if ishttp && gopath != "" {
			os.RemoveAll(gopath)
		}
	}

	path := chaincodeID.Path

	var pkg string
	if strings.HasPrefix(path, "http://") {
		ishttp = true
		pkg = path[7:]
		gopath, err = getCodeFromHTTP(pkg)
	} else if strings.HasPrefix(path, "https://") {
		ishttp = true
		pkg = path[8:]
		gopath, err = getCodeFromHTTP(pkg)
	} else {
		pkg = path
		gopath, err = getCodeFromFS(path)
	}

	if err != nil {
		return nil, fmt.Errorf("Error getting code %s", err)
	}

	return &CodeDescriptor{Gopath: gopath, Pkg: pkg, Cleanup: cleanup}, nil
}

type SourceDescriptor struct {
	Name, Path string
	Info       os.FileInfo
}
type SourceMap map[string]SourceDescriptor

type Sources []SourceDescriptor

func (s Sources) Len() int {
	return len(s)
}

func (s Sources) Swap(i, j int) {
	s[i], s[j] = s[j], s[i]
}

func (s Sources) Less(i, j int) bool {
	return strings.Compare(s[i].Name, s[j].Name) < 0
}

func findSource(gopath, pkg string) (SourceMap, error) {
	sources := make(SourceMap)
	tld := filepath.Join(gopath, "src", pkg)
	walkFn := func(path string, info os.FileInfo, err error) error {

		if err != nil {
			return err
		}

		if info.IsDir() {
			if path == tld {
				// We dont want to import any directories, but we don't want to stop processing
				// at the TLD either.
				return nil
			}

			// Do not recurse
			logger.Debugf("skipping dir: %s", path)
			return filepath.SkipDir
		}

		ext := filepath.Ext(path)
		// we only want 'fileTypes' source files at this point
		if _, ok := includeFileTypes[ext]; ok != true {
			return nil
		}

		name, err := filepath.Rel(gopath, path)
		if err != nil {
			return fmt.Errorf("error obtaining relative path for %s: %s", path, err)
		}

		sources[name] = SourceDescriptor{Name: name, Path: path, Info: info}

		return nil
	}

	if err := filepath.Walk(tld, walkFn); err != nil {
		return nil, fmt.Errorf("Error walking directory: %s", err)
	}

	return sources, nil
}
