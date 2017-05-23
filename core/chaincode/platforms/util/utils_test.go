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
	"bytes"
	"encoding/hex"
	"math/rand"
	"testing"
	"time"

	"archive/tar"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/hyperledger/fabric/common/util"
	"github.com/hyperledger/fabric/core/config"
	cutil "github.com/hyperledger/fabric/core/container/util"
	"github.com/spf13/viper"
)

// TestHashContentChange changes a random byte in a content and checks for hash change
func TestHashContentChange(t *testing.T) {
	b := []byte("firstcontent")
	hash := util.ComputeSHA256(b)

	b2 := []byte("To be, or not to be- that is the question: Whether 'tis nobler in the mind to suffer The slings and arrows of outrageous fortune Or to take arms against a sea of troubles, And by opposing end them. To die- to sleep- No more; and by a sleep to say we end The heartache, and the thousand natural shocks That flesh is heir to. 'Tis a consummation Devoutly to be wish'd.")

	h1 := ComputeHash(b2, hash)

	r := rand.New(rand.NewSource(time.Now().UnixNano()))
	randIndex := (int(r.Uint32())) % len(b2)

	randByte := byte((int(r.Uint32())) % 128)

	//make sure the two bytes are different
	for {
		if randByte != b2[randIndex] {
			break
		}

		randByte = byte((int(r.Uint32())) % 128)
	}

	//change a random byte
	b2[randIndex] = randByte

	//this is the core hash func under test
	h2 := ComputeHash(b2, hash)

	//the two hashes should be different
	if bytes.Compare(h1, h2) == 0 {
		t.Error("Hash expected to be different but is same")
	}
}

// TestHashLenChange changes a random length of a content and checks for hash change
func TestHashLenChange(t *testing.T) {
	b := []byte("firstcontent")
	hash := util.ComputeSHA256(b)

	b2 := []byte("To be, or not to be-")

	h1 := ComputeHash(b2, hash)

	r := rand.New(rand.NewSource(time.Now().UnixNano()))
	randIndex := (int(r.Uint32())) % len(b2)

	b2 = b2[0:randIndex]

	h2 := ComputeHash(b2, hash)

	//hash should be different
	if bytes.Compare(h1, h2) == 0 {
		t.Error("Hash expected to be different but is same")
	}
}

// TestHashOrderChange changes a order of hash computation over a list of lines and checks for hash change
func TestHashOrderChange(t *testing.T) {
	b := []byte("firstcontent")
	hash := util.ComputeSHA256(b)

	b2 := [][]byte{[]byte("To be, or not to be- that is the question:"),
		[]byte("Whether 'tis nobler in the mind to suffer"),
		[]byte("The slings and arrows of outrageous fortune"),
		[]byte("Or to take arms against a sea of troubles,"),
		[]byte("And by opposing end them."),
		[]byte("To die- to sleep- No more; and by a sleep to say we end"),
		[]byte("The heartache, and the thousand natural shocks"),
		[]byte("That flesh is heir to."),
		[]byte("'Tis a consummation Devoutly to be wish'd.")}
	h1 := hash

	for _, l := range b2 {
		h1 = ComputeHash(l, h1)
	}

	r := rand.New(rand.NewSource(time.Now().UnixNano()))
	randIndex1 := (int(r.Uint32())) % len(b2)
	randIndex2 := (int(r.Uint32())) % len(b2)

	//make sure the two indeces are different
	for {
		if randIndex2 != randIndex1 {
			break
		}

		randIndex2 = (int(r.Uint32())) % len(b2)
	}

	//switch two arbitrary lines
	tmp := b2[randIndex2]
	b2[randIndex2] = b2[randIndex1]
	b2[randIndex1] = tmp

	h2 := hash
	for _, l := range b2 {
		h2 = ComputeHash(l, hash)
	}

	//hash should be different
	if bytes.Compare(h1, h2) == 0 {
		t.Error("Hash expected to be different but is same")
	}
}

// TestHashOverFiles computes hash over a directory and ensures it matches precomputed, hardcoded, hash
func TestHashOverFiles(t *testing.T) {
	b := []byte("firstcontent")
	hash := util.ComputeSHA256(b)

	hash, err := HashFilesInDir(".", "hashtestfiles1", hash, nil)

	if err != nil {
		t.Fail()
		t.Logf("error : %s", err)
	}

	//as long as no files under "hashtestfiles1" are changed, hash should always compute to the following
	expectedHash := "0c92180028200dfabd08d606419737f5cdecfcbab403e3f0d79e8d949f4775bc"

	computedHash := hex.EncodeToString(hash[:])

	if expectedHash != computedHash {
		t.Error("Hash expected to be unchanged")
	}
}

func TestHashDiffDir(t *testing.T) {
	b := []byte("firstcontent")
	hash := util.ComputeSHA256(b)

	hash1, err := HashFilesInDir(".", "hashtestfiles1", hash, nil)
	if err != nil {
		t.Errorf("Error getting code %s", err)
	}
	hash2, err := HashFilesInDir(".", "hashtestfiles2", hash, nil)
	if err != nil {
		t.Errorf("Error getting code %s", err)
	}
	if bytes.Compare(hash1, hash2) == 0 {
		t.Error("Hash should be different for 2 different remote repos")
	}

}
func TestHashSameDir(t *testing.T) {
	b := []byte("firstcontent")
	hash := util.ComputeSHA256(b)

	hash1, err := HashFilesInDir(".", "hashtestfiles1", hash, nil)
	if err != nil {
		t.Errorf("Error getting code %s", err)
	}
	hash2, err := HashFilesInDir(".", "hashtestfiles1", hash, nil)
	if err != nil {
		t.Errorf("Error getting code %s", err)
	}
	if bytes.Compare(hash1, hash2) != 0 {
		t.Error("Hash should be same across multiple downloads")
	}
}

func TestDockerPull(t *testing.T) {
	codepackage, output := io.Pipe()
	go func() {
		tw := tar.NewWriter(output)

		tw.Close()
		output.Close()
	}()

	binpackage := bytes.NewBuffer(nil)

	// Perform a nop operation within a fixed target.  We choose 1.0.0-alpha2 because we know it's
	// published and available.  Ideally we could choose something that we know is both multi-arch
	// and ok to delete prior to executing DockerBuild.  This would ensure that we exercise the
	// image pull logic.  However, no suitable target exists that meets all the criteria.  Therefore
	// we settle on using a known released image.  We don't know if the image is already
	// downloaded per se, and we don't want to explicitly delete this particular image first since
	// it could be in use legitimately elsewhere.  Instead, we just know that this should always
	// work and call that "close enough".
	//
	// Future considerations: publish a known dummy image that is multi-arch and free to randomly
	// delete, and use that here instead.
	err := DockerBuild(DockerBuildOptions{
		Image:        cutil.ParseDockerfileTemplate("hyperledger/fabric-ccenv:$(ARCH)-1.0.0-alpha2"),
		Cmd:          "/bin/true",
		InputStream:  codepackage,
		OutputStream: binpackage,
	})
	if err != nil {
		t.Errorf("Error during build: %s", err)
	}
}

func TestMain(m *testing.M) {
	viper.SetConfigName("core")
	viper.SetEnvPrefix("CORE")
	config.AddDevConfigPath(nil)
	viper.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	viper.AutomaticEnv()
	if err := viper.ReadInConfig(); err != nil {
		fmt.Printf("could not read config %s\n", err)
		os.Exit(-1)
	}
	os.Exit(m.Run())
}
