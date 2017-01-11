package util

import (
	"archive/tar"
	"bytes"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/hyperledger/fabric/common/util"
	cutil "github.com/hyperledger/fabric/core/container/util"
	"github.com/op/go-logging"
)

var logger = logging.MustGetLogger("util")

//ComputeHash computes contents hash based on previous hash
func ComputeHash(contents []byte, hash []byte) []byte {
	newSlice := make([]byte, len(hash)+len(contents))

	//copy the contents
	copy(newSlice[0:len(contents)], contents[:])

	//add the previous hash
	copy(newSlice[len(contents):], hash[:])

	//compute new hash
	hash = util.ComputeCryptoHash(newSlice)

	return hash
}

//HashFilesInDir computes h=hash(h,file bytes) for each file in a directory
//Directory entries are traversed recursively. In the end a single
//hash value is returned for the entire directory structure
func HashFilesInDir(rootDir string, dir string, hash []byte, tw *tar.Writer) ([]byte, error) {
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
			hash, err = HashFilesInDir(rootDir, name, hash, tw)
			if err != nil {
				return hash, err
			}
			continue
		}
		fqp := filepath.Join(rootDir, name)
		buf, err := ioutil.ReadFile(fqp)
		if err != nil {
			logger.Errorf("Error reading %s\n", err)
			return hash, err
		}

		//get the new hash from file contents
		hash = ComputeHash(buf, hash)

		if tw != nil {
			is := bytes.NewReader(buf)
			if err = cutil.WriteStreamToPackage(is, fqp, filepath.Join("src", name), tw); err != nil {
				return hash, fmt.Errorf("Error adding file to tar %s", err)
			}
		}
	}
	return hash, nil
}

//IsCodeExist checks the chaincode if exists
func IsCodeExist(tmppath string) error {
	file, err := os.Open(tmppath)
	if err != nil {
		return fmt.Errorf("Could not open file %s", err)
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
