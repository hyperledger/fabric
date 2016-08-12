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
	"archive/tar"
	"bufio"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/op/go-logging"
	"github.com/spf13/viper"
)

var vmLogger = logging.MustGetLogger("container")

var includeFileTypes = map[string]bool{
	".c":    true,
	".h":    true,
	".go":   true,
	".yaml": true,
	".json": true,
}

// These filetypes are excluded while creating the tar package sent to Docker
// Generated .class and other temporary files can be excluded
var javaExcludeFileTypes = map[string]bool{
	".class": true,
}

func WriteFolderToTarPackage(tw *tar.Writer, srcPath string, excludeDir string, includeFileTypeMap map[string]bool, excludeFileTypeMap map[string]bool) error {
	rootDirectory := srcPath
	vmLogger.Infof("rootDirectory = %s", rootDirectory)

	//append "/" if necessary
	if excludeDir != "" && strings.LastIndex(excludeDir, "/") < len(excludeDir)-1 {
		excludeDir = excludeDir + "/"
	}

	rootDirLen := len(rootDirectory)
	walkFn := func(path string, info os.FileInfo, err error) error {

		// If path includes .git, ignore
		if strings.Contains(path, ".git") {
			return nil
		}

		if info.Mode().IsDir() {
			return nil
		}

		//exclude any files with excludeDir prefix. They should already be in the tar
		if excludeDir != "" && strings.Index(path, excludeDir) == rootDirLen+1 {
			//1 for "/"
			return nil
		}
		// Because of scoping we can reference the external rootDirectory variable
		if len(path[rootDirLen:]) == 0 {
			return nil
		}
		ext := filepath.Ext(path)

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

		newPath := fmt.Sprintf("src%s", path[rootDirLen:])
		//newPath := path[len(rootDirectory):]

		err = WriteFileToPackage(path, newPath, tw)
		if err != nil {
			return fmt.Errorf("Error writing file to package: %s", err)
		}
		return nil
	}

	if err := filepath.Walk(rootDirectory, walkFn); err != nil {
		vmLogger.Infof("Error walking rootDirectory: %s", err)
		return err
	}
	return nil
}

//WriteGopathSrc tars up files under gopath src
func WriteGopathSrc(tw *tar.Writer, excludeDir string) error {
	gopath := os.Getenv("GOPATH")
	// Only take the first element of GOPATH
	gopath = filepath.SplitList(gopath)[0]

	rootDirectory := filepath.Join(gopath, "src")
	vmLogger.Infof("rootDirectory = %s", rootDirectory)

	if err := WriteFolderToTarPackage(tw, rootDirectory, excludeDir, includeFileTypes, nil); err != nil {
		vmLogger.Errorf("Error writing folder to tar package %s", err)
		return err
	}

	// Add the certificates to tar
	if viper.GetBool("peer.tls.enabled") {
		err := WriteFileToPackage(viper.GetString("peer.tls.cert.file"), "src/certs/cert.pem", tw)
		if err != nil {
			return fmt.Errorf("Error writing cert file to package: %s", err)
		}
	}

	// Write the tar file out
	if err := tw.Close(); err != nil {
		return err
	}
	//ioutil.WriteFile("/tmp/chaincode_deployment.tar", inputbuf.Bytes(), 0644)
	return nil
}

//Package Java project to tar file from the source path
func WriteJavaProjectToPackage(tw *tar.Writer, srcPath string) error {

	vmLogger.Debugf("Packaging Java project from path %s", srcPath)

	if err := WriteFolderToTarPackage(tw, srcPath, "", nil, javaExcludeFileTypes); err != nil {

		vmLogger.Errorf("Error writing folder to tar package %s", err)
		return err
	}
	// Write the tar file out
	if err := tw.Close(); err != nil {
		return err
	}
	return nil

}

//WriteFileToPackage writes a file to the tarball
func WriteFileToPackage(localpath string, packagepath string, tw *tar.Writer) error {
	fd, err := os.Open(localpath)
	if err != nil {
		return fmt.Errorf("%s: %s", localpath, err)
	}
	defer fd.Close()

	is := bufio.NewReader(fd)
	return WriteStreamToPackage(is, localpath, packagepath, tw)

}

//WriteStreamToPackage writes bytes (from a file reader) to the tarball
func WriteStreamToPackage(is io.Reader, localpath string, packagepath string, tw *tar.Writer) error {
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
	var zeroTime time.Time
	header.AccessTime = zeroTime
	header.ModTime = zeroTime
	header.ChangeTime = zeroTime
	header.Name = packagepath

	if err = tw.WriteHeader(header); err != nil {
		return fmt.Errorf("Error write header for (path: %s, oldname:%s,newname:%s,sz:%d) : %s", localpath, oldname, packagepath, header.Size, err)
	}
	if _, err := io.Copy(tw, is); err != nil {
		return fmt.Errorf("Error copy (path: %s, oldname:%s,newname:%s,sz:%d) : %s", localpath, oldname, packagepath, header.Size, err)
	}

	return nil
}
