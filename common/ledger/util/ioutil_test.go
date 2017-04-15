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
	"fmt"
	"os"
	"testing"

	"github.com/hyperledger/fabric/common/ledger/testutil"
)

var DbPathTest = "/tmp/v2/test/util"
var DbFileTest = DbPathTest + "/testUtilFileDat1"

func TestCreatingDBDirWithPathSeperator(t *testing.T) {

	//test creating a directory with path separator passed
	DbPathTestWSeparator := DbPathTest + "/"
	cleanup(DbPathTestWSeparator) //invoked prior to test to make sure test is executed in desired environment
	defer cleanup(DbPathTestWSeparator)

	dirEmpty, err := CreateDirIfMissing(DbPathTestWSeparator)
	testutil.AssertNoError(t, err, fmt.Sprintf("Error when trying to create a test db directory at [%s]", DbPathTestWSeparator))
	testutil.AssertEquals(t, dirEmpty, true) //test directory is empty is returning true
}

func TestCreatingDBDirWhenDirDoesAndDoesNotExists(t *testing.T) {

	cleanup(DbPathTest) //invoked prior to test to make sure test is executed in desired environment
	defer cleanup(DbPathTest)

	//test creating a directory without path separator passed
	dirEmpty, err := CreateDirIfMissing(DbPathTest)
	testutil.AssertNoError(t, err, fmt.Sprintf("Error when trying to create a test db directory at [%s]", DbPathTest))
	testutil.AssertEquals(t, dirEmpty, true)

	//test creating directory AGAIN, that is the directory already exists
	dirEmpty2, err2 := CreateDirIfMissing(DbPathTest)
	testutil.AssertNoError(t, err2, fmt.Sprintf("Error not handling existing directory when trying to create a test db directory at [%s]", DbPathTest))
	testutil.AssertEquals(t, dirEmpty2, true)

}

func TestDirNotEmptyAndFileExists(t *testing.T) {

	cleanup(DbPathTest)
	defer cleanup(DbPathTest)

	//create the directory
	dirEmpty, err := CreateDirIfMissing(DbPathTest)
	testutil.AssertNoError(t, err, fmt.Sprintf("Error when trying to create a test db directory at [%s]", DbPathTest))
	testutil.AssertEquals(t, dirEmpty, true)

	//test file does not exists and size is returned correctly
	exists2, size2, err2 := FileExists(DbFileTest)
	testutil.AssertNoError(t, err2, fmt.Sprintf("Error when trying to determine if file exist when it does not at [%s]", DbFileTest))
	testutil.AssertEquals(t, size2, int64(0))
	testutil.AssertEquals(t, exists2, false) //test file that does not exists reports false

	//create file
	testStr := "This is some test data in a file"
	sizeOfFileCreated, err3 := createAndWriteAFile(testStr)
	testutil.AssertNoError(t, err3, fmt.Sprintf("Error when trying to create and write to file at [%s]", DbFileTest))
	testutil.AssertEquals(t, sizeOfFileCreated, len(testStr)) //test file size returned is correct

	//test that the file exists and size is returned correctly
	exists, size, err4 := FileExists(DbFileTest)
	testutil.AssertNoError(t, err4, fmt.Sprintf("Error when trying to determine if file exist at [%s]", DbFileTest))
	testutil.AssertEquals(t, size, int64(sizeOfFileCreated))
	testutil.AssertEquals(t, exists, true) //test file that does exists reports true

	//test that if the directory is not empty
	dirEmpty5, err5 := DirEmpty(DbPathTest)
	testutil.AssertNoError(t, err5, fmt.Sprintf("Error when detecting if empty at db directory [%s]", DbPathTest))
	testutil.AssertEquals(t, dirEmpty5, false) //test directory is empty is returning false
}

func createAndWriteAFile(sentence string) (int, error) {
	//create a file in the direcotry
	f, err2 := os.Create(DbFileTest)
	if err2 != nil {
		return 0, err2
	}
	defer f.Close()

	//write to the file
	return f.WriteString(sentence)
}

func cleanup(path string) {
	os.RemoveAll(path)
}
