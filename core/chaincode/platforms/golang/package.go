/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package golang

import (
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/pkg/errors"
)

type CodeDescriptor struct {
	Gopath string
	Pkg    string
}

// getCodeDescriptor returns GOPATH and package information
func getCodeDescriptor(path string) (CodeDescriptor, error) {
	if path == "" {
		return CodeDescriptor{}, errors.New("cannot collect files from empty chaincode path")
	}

	gopath, err := getGopath()
	if err != nil {
		return CodeDescriptor{}, err
	}
	sourcePath := filepath.Join(gopath, "src", path)

	fi, err := os.Stat(sourcePath)
	if err != nil {
		return CodeDescriptor{}, errors.Wrap(err, "failed to get code")
	}
	if !fi.IsDir() {
		return CodeDescriptor{}, errors.Errorf("path is not a directory: %s", path)
	}

	return CodeDescriptor{Gopath: gopath, Pkg: path}, nil
}

type SourceDescriptor struct {
	Name       string
	Path       string
	IsMetadata bool
}

type Sources []SourceDescriptor

func (s Sources) Len() int           { return len(s) }
func (s Sources) Swap(i, j int)      { s[i], s[j] = s[j], s[i] }
func (s Sources) Less(i, j int) bool { return s[i].Name < s[j].Name }

type SourceMap map[string]SourceDescriptor

func (s SourceMap) values() Sources {
	var sources Sources
	for _, src := range s {
		sources = append(sources, src)
	}

	sort.Sort(sources)
	return sources
}

func findSource(cd CodeDescriptor) (SourceMap, error) {
	sources := SourceMap{}

	tld := filepath.Join(cd.Gopath, "src", cd.Pkg)
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
				return nil
			}

			// Do not import any other directories into chaincode code package
			return filepath.SkipDir
		}

		name, err := filepath.Rel(cd.Gopath, path)
		if err != nil {
			return errors.Wrapf(err, "failed to calculate relative path for %s", path)
		}

		sources[name] = SourceDescriptor{Name: name, Path: path, IsMetadata: isMetadataDir(path, tld)}
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
