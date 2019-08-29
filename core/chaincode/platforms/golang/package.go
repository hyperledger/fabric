/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package golang

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/hyperledger/fabric/common/flogging"
	"github.com/pkg/errors"
)

var logger = flogging.MustGetLogger("chaincode.platform.golang")

func getCodeFromFS(path string) (codegopath string, err error) {
	logger.Debugf("getCodeFromFS %s", path)

	if path == "" {
		return "", errors.New("cannot collect files from empty chaincode path")
	}

	gopath, err := getGopath()
	if err != nil {
		return "", err
	}

	tmppath := filepath.Join(gopath, "src", path)
	if !isDir(tmppath) {
		return "", errors.Wrap(err, "code does not exist")
	}

	return gopath, nil
}

func isDir(path string) bool {
	fi, err := os.Stat(path)
	if err != nil {
		return false
	}

	return fi.IsDir()
}

type CodeDescriptor struct {
	Gopath  string
	Pkg     string
	Cleanup func()
}

// collectChaincodeFiles collects chaincode files. If path is a HTTP(s) url it
// downloads the code first.
func getCode(path string) (*CodeDescriptor, error) {
	// code root will point to the directory where the code exists
	gopath, err := getCodeFromFS(path)
	if err != nil {
		return nil, errors.WithMessage(err, "failed to get code")
	}

	return &CodeDescriptor{Gopath: gopath, Pkg: path, Cleanup: nil}, nil
}

type SourceDescriptor struct {
	Name, Path string
	IsMetadata bool
	Info       os.FileInfo
}

type SourceMap map[string]SourceDescriptor

func findSource(gopath, pkg string) (SourceMap, error) {
	sources := SourceMap{}

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

		name, err := filepath.Rel(gopath, path)
		if err != nil {
			return errors.Wrapf(err, "failed to calculate relative path for %s", path)
		}

		sources[name] = SourceDescriptor{Name: name, Path: path, IsMetadata: isMetadataDir(path, tld), Info: info}
		return nil
	}

	if err := filepath.Walk(tld, walkFn); err != nil {
		return nil, errors.Wrap(err, "walk failed")
	}

	return sources, nil
}

// isMetadataDir checks to see if the current path is in the META-INF directory at the root of the chaincode directory
func isMetadataDir(path, tld string) bool {
	return strings.HasPrefix(path, filepath.Join(tld, "META-INF"))
}
