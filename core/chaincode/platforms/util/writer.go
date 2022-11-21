/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package util

import (
	"archive/tar"
	"bufio"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/hyperledger/fabric/internal/ccmetadata"
	"github.com/pkg/errors"
)

// WriteFolderToTarPackage writes source files to a tarball.
// This utility is used for node js chaincode packaging, but not golang chaincode.
// Golang chaincode has more sophisticated file packaging, as implemented in golang/platform.go.
func WriteFolderToTarPackage(tw *tar.Writer, srcPath string, excludeDirs []string, includeFileTypeMap map[string]bool, excludeFileTypeMap map[string]bool) error {
	rootDirectory := filepath.Clean(srcPath)

	logger.Debugw("writing folder to package", "rootDirectory", rootDirectory)

	var success bool
	walkFn := func(localpath string, info os.FileInfo, err error) error {
		if err != nil {
			logger.Errorf("Visit %s failed: %s", localpath, err)
			return err
		}

		if info.Mode().IsDir() {
			for _, excluded := range append(excludeDirs, ".git") {
				if info.Name() == excluded {
					return filepath.SkipDir
				}
			}
			return nil
		}

		ext := filepath.Ext(localpath)
		if _, ok := includeFileTypeMap[ext]; includeFileTypeMap != nil && !ok {
			return nil
		}
		if excludeFileTypeMap[ext] {
			return nil
		}

		relpath, err := filepath.Rel(rootDirectory, localpath)
		if err != nil {
			return err
		}
		packagepath := filepath.ToSlash(relpath)

		// if file is metadata, keep the /META-INF directory, e.g: META-INF/statedb/couchdb/indexes/indexOwner.json
		// otherwise file is source code, put it in /src dir, e.g: src/marbles_chaincode.js
		if strings.HasPrefix(localpath, filepath.Join(rootDirectory, "META-INF")) {
			// Hidden files are not supported as metadata, therefore ignore them.
			// User often doesn't know that hidden files are there, and may not be able to delete them, therefore warn user rather than error out.
			if strings.HasPrefix(info.Name(), ".") {
				logger.Warningf("Ignoring hidden file in metadata directory: %s", packagepath)
				return nil
			}

			fileBytes, err := os.ReadFile(localpath)
			if err != nil {
				return err
			}

			// Validate metadata file for inclusion in tar
			// Validation is based on the fully qualified path of the file
			err = ccmetadata.ValidateMetadataFile(packagepath, fileBytes)
			if err != nil {
				return err
			}
		} else { // file is not metadata, include in src
			packagepath = path.Join("src", packagepath)
		}

		err = WriteFileToPackage(localpath, packagepath, tw)
		if err != nil {
			return fmt.Errorf("Error writing file to package: %s", err)
		}

		success = true
		return nil
	}

	if err := filepath.Walk(rootDirectory, walkFn); err != nil {
		logger.Infof("Error walking rootDirectory: %s", err)
		return err
	}

	if !success {
		return errors.Errorf("no source files found in '%s'", srcPath)
	}
	return nil
}

// WriteFileToPackage writes a file to a tar stream.
func WriteFileToPackage(localpath string, packagepath string, tw *tar.Writer) error {
	logger.Debug("Writing file to tarball:", packagepath)
	fd, err := os.Open(localpath)
	if err != nil {
		return fmt.Errorf("%s: %s", localpath, err)
	}
	defer fd.Close()

	fi, err := fd.Stat()
	if err != nil {
		return fmt.Errorf("%s: %s", localpath, err)
	}

	header, err := tar.FileInfoHeader(fi, localpath)
	if err != nil {
		return fmt.Errorf("failed calculating FileInfoHeader: %s", err)
	}

	// Take the variance out of the tar by using zero time and fixed uid/gid.
	var zeroTime time.Time
	header.AccessTime = zeroTime
	header.ModTime = zeroTime
	header.ChangeTime = zeroTime
	header.Name = packagepath
	header.Mode = 0o100644
	header.Uid = 500
	header.Gid = 500
	header.Uname = ""
	header.Gname = ""

	err = tw.WriteHeader(header)
	if err != nil {
		return fmt.Errorf("failed to write header for %s: %s", localpath, err)
	}

	is := bufio.NewReader(fd)
	_, err = io.Copy(tw, is)
	if err != nil {
		return fmt.Errorf("failed to write %s as %s: %s", localpath, packagepath, err)
	}

	return nil
}
