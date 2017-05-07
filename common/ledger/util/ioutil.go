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

package util

import (
	"io"
	"io/ioutil"
	"os"
	"path"
	"strings"

	"github.com/hyperledger/fabric/common/flogging"
)

var logger = flogging.MustGetLogger("kvledger.util")

// CreateDirIfMissing creates a dir for dirPath if not already exists. If the dir is empty it returns true
func CreateDirIfMissing(dirPath string) (bool, error) {
	// if dirPath does not end with a path separator, it leaves out the last segment while creating directories
	if !strings.HasSuffix(dirPath, "/") {
		dirPath = dirPath + "/"
	}
	logger.Debugf("CreateDirIfMissing [%s]", dirPath)
	logDirStatus("Before creating dir", dirPath)
	err := os.MkdirAll(path.Dir(dirPath), 0755)
	if err != nil {
		logger.Debugf("Error while creating dir [%s]", dirPath)
		return false, err
	}
	logDirStatus("After creating dir", dirPath)
	return DirEmpty(dirPath)
}

// DirEmpty returns true if the dir at dirPath is empty
func DirEmpty(dirPath string) (bool, error) {
	f, err := os.Open(dirPath)
	if err != nil {
		logger.Debugf("Error while opening dir [%s]: %s", dirPath, err)
		return false, err
	}
	defer f.Close()

	_, err = f.Readdir(1)
	if err == io.EOF {
		return true, nil
	}
	return false, err
}

// FileExists checks whether the given file exists.
// If the file exists, this method also returns the size of the file.
func FileExists(filePath string) (bool, int64, error) {
	fileInfo, err := os.Stat(filePath)
	if os.IsNotExist(err) {
		return false, 0, nil
	}
	return true, fileInfo.Size(), err
}

// ListSubdirs returns the subdirectories
func ListSubdirs(dirPath string) ([]string, error) {
	subdirs := []string{}
	files, err := ioutil.ReadDir(dirPath)
	if err != nil {
		return nil, err
	}
	for _, f := range files {
		if f.IsDir() {
			subdirs = append(subdirs, f.Name())
		}
	}
	return subdirs, nil
}

func logDirStatus(msg string, dirPath string) {
	exists, _, err := FileExists(dirPath)
	if err != nil {
		logger.Errorf("Error while checking for dir existence")
	}
	if exists {
		logger.Debugf("%s - [%s] exists", msg, dirPath)
	} else {
		logger.Debugf("%s - [%s] does not exist", msg, dirPath)
	}
}
