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
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/hyperledger/fabric/common/flogging"
	"github.com/hyperledger/fabric/internal/ccmetadata"
	"github.com/pkg/errors"
)

var vmLogger = flogging.MustGetLogger("container")

// WriteFolderToTarPackage writes source files to a tarball.
// This utility is used for node js chaincode packaging, but not golang chaincode.
// Golang chaincode has more sophisticated file packaging, as implemented in golang/platform.go.
func WriteFolderToTarPackage(tw *tar.Writer, srcPath string, excludeDirs []string, includeFileTypeMap map[string]bool, excludeFileTypeMap map[string]bool) error {
	fileCount := 0
	rootDirectory := srcPath

	// trim trailing slash if it was passed
	if rootDirectory[len(rootDirectory)-1] == '/' {
		rootDirectory = rootDirectory[:len(rootDirectory)-1]
	}

	vmLogger.Debugf("rootDirectory = %s", rootDirectory)

	//append "/" if necessary
	updatedExcludeDirs := make([]string, 0)
	for _, excludeDir := range excludeDirs {
		if excludeDir != "" && strings.LastIndex(excludeDir, "/") < len(excludeDir)-1 {
			excludeDir = excludeDir + "/"
			updatedExcludeDirs = append(updatedExcludeDirs, excludeDir)
		}
	}

	rootDirLen := len(rootDirectory)
	walkFn := func(localpath string, info os.FileInfo, err error) error {
		if err != nil {
			vmLogger.Errorf("Visit %s failed: %s", localpath, err)
			return err
		}

		// If localpath includes .git, ignore
		if strings.Contains(localpath, ".git") {
			return nil
		}

		if info.Mode().IsDir() {
			return nil
		}

		//exclude any files with excludeDir prefix. They should already be in the tar
		for _, excludeDir := range updatedExcludeDirs {
			if strings.Index(localpath, excludeDir) == rootDirLen+1 {
				return nil
			}
		}
		// Because of scoping we can reference the external rootDirectory variable
		if len(localpath[rootDirLen:]) == 0 {
			return nil
		}
		ext := filepath.Ext(localpath)

		if includeFileTypeMap != nil {
			// we only want 'fileTypes' source files at this point
			if _, ok := includeFileTypeMap[ext]; ok != true {
				return nil
			}
		}

		//exclude the given file types
		if excludeFileTypeMap != nil {
			if exclude, ok := excludeFileTypeMap[ext]; ok && exclude {
				return nil
			}
		}

		var packagepath string

		// if file is metadata, keep the /META-INF directory, e.g: META-INF/statedb/couchdb/indexes/indexOwner.json
		// otherwise file is source code, put it in /src dir, e.g: src/marbles_chaincode.js
		if strings.HasPrefix(localpath, filepath.Join(rootDirectory, "META-INF")) {
			packagepath = localpath[rootDirLen+1:]

			// Split the tar packagepath into a tar package directory and filename
			_, filename := filepath.Split(packagepath)

			// Hidden files are not supported as metadata, therefore ignore them.
			// User often doesn't know that hidden files are there, and may not be able to delete them, therefore warn user rather than error out.
			if strings.HasPrefix(filename, ".") {
				vmLogger.Warningf("Ignoring hidden file in metadata directory: %s", packagepath)
				return nil
			}

			fileBytes, errRead := ioutil.ReadFile(localpath)
			if errRead != nil {
				return errRead
			}

			// Validate metadata file for inclusion in tar
			// Validation is based on the fully qualified path of the file
			err = ccmetadata.ValidateMetadataFile(packagepath, fileBytes)
			if err != nil {
				return err
			}

		} else { // file is not metadata, include in src
			packagepath = fmt.Sprintf("src%s", localpath[rootDirLen:])
		}

		err = WriteFileToPackage(localpath, packagepath, tw)
		if err != nil {
			return fmt.Errorf("Error writing file to package: %s", err)
		}
		fileCount++

		return nil
	}

	if err := filepath.Walk(rootDirectory, walkFn); err != nil {
		vmLogger.Infof("Error walking rootDirectory: %s", err)
		return err
	}
	// return error if no files were found
	if fileCount == 0 {
		return errors.Errorf("no source files found in '%s'", srcPath)
	}
	return nil
}

// WriteFileToPackage writes a file to a tar stream.
func WriteFileToPackage(localpath string, packagepath string, tw *tar.Writer) error {
	vmLogger.Debug("Writing file to tarball:", packagepath)
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
	header.Mode = 0100644
	header.Uid = 500
	header.Gid = 500

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
