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
	"strings"

	"os"
	"path/filepath"

	"github.com/hyperledger/fabric/common/flogging"
	ccutil "github.com/hyperledger/fabric/core/chaincode/platforms/util"
	pb "github.com/hyperledger/fabric/protos/peer"
)

var includeFileTypes = map[string]bool{
	".c":    true,
	".h":    true,
	".s":    true,
	".go":   true,
	".yaml": true,
	".json": true,
}

var logger = flogging.MustGetLogger("golang-platform")

func getCodeFromFS(path string) (codegopath string, err error) {
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

	// code root will point to the directory where the code exists
	var gopath string
	gopath, err := getCodeFromFS(chaincodeID.Path)
	if err != nil {
		return nil, fmt.Errorf("Error getting code %s", err)
	}

	return &CodeDescriptor{Gopath: gopath, Pkg: chaincodeID.Path, Cleanup: nil}, nil
}

type SourceDescriptor struct {
	Name, Path string
	IsMetadata bool
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

			// Allow import of the top level chaincode directory into chaincode code package
			if path == tld {
				return nil
			}

			// Allow import of META-INF metadata directories into chaincode code package tar.
			// META-INF directories contain chaincode metadata artifacts such as statedb index definitions
			if isMetadataDir(path, tld) {
				logger.Debug("Files in META-INF directory will be included in code package tar:", path)
				return nil
			}

			// Do not import any other directories into chaincode code package
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

		sources[name] = SourceDescriptor{Name: name, Path: path, IsMetadata: isMetadataDir(path, tld), Info: info}

		return nil
	}

	if err := filepath.Walk(tld, walkFn); err != nil {
		return nil, fmt.Errorf("Error walking directory: %s", err)
	}

	return sources, nil
}

// isMetadataDir checks to see if the current path is in the META-INF directory at the root of the chaincode directory
func isMetadataDir(path, tld string) bool {
	return strings.HasPrefix(path, filepath.Join(tld, "META-INF"))
}
