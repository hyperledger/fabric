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

package car

import (
	"archive/tar"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"strings"
	"time"

	cutil "github.com/hyperledger/fabric/core/container/util"
	pb "github.com/hyperledger/fabric/protos"
	"github.com/spf13/viper"
)

func download(path string) (string, error) {
	if strings.HasPrefix(path, "http://") {
		// The file is remote, so we need to download it to a temporary location first

		var tmp *os.File
		var err error
		tmp, err = ioutil.TempFile("", "car")
		if err != nil {
			return "", fmt.Errorf("Error creating temporary file: %s", err)
		}
		defer os.Remove(tmp.Name())
		defer tmp.Close()

		resp, err := http.Get(path)
		if err != nil {
			return "", fmt.Errorf("Error with HTTP GET: %s", err)
		}
		defer resp.Body.Close()

		_, err = io.Copy(tmp, resp.Body)
		if err != nil {
			return "", fmt.Errorf("Error downloading bytes: %s", err)
		}

		return tmp.Name(), nil
	}

	return path, nil
}

// WritePackage satisfies the platform interface for generating a docker package
// that encapsulates the environment for a CAR based chaincode
func (carPlatform *Platform) WritePackage(spec *pb.ChaincodeSpec, tw *tar.Writer) error {

	path, err := download(spec.ChaincodeID.Path)
	if err != nil {
		return err
	}

	spec.ChaincodeID.Name, err = generateHashcode(spec, path)
	if err != nil {
		return fmt.Errorf("Error generating hashcode: %s", err)
	}

	var buf []string

	//let the executable's name be chaincode ID's name
	buf = append(buf, viper.GetString("chaincode.car.Dockerfile"))
	buf = append(buf, "COPY package.car /tmp/package.car")
	buf = append(buf, fmt.Sprintf("RUN chaintool buildcar /tmp/package.car -o $GOPATH/bin/%s && rm /tmp/package.car", spec.ChaincodeID.Name))

	dockerFileContents := strings.Join(buf, "\n")
	dockerFileSize := int64(len([]byte(dockerFileContents)))

	//Make headers identical by using zero time
	var zeroTime time.Time
	tw.WriteHeader(&tar.Header{Name: "Dockerfile", Size: dockerFileSize, ModTime: zeroTime, AccessTime: zeroTime, ChangeTime: zeroTime})
	tw.Write([]byte(dockerFileContents))

	err = cutil.WriteFileToPackage(path, "package.car", tw)
	if err != nil {
		return err
	}

	return nil
}
