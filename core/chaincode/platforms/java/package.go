/*
Copyright DTCC 2016 All Rights Reserved.

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

package java

import (
	"archive/tar"
	"fmt"
	"io/ioutil"
	"strings"
	"time"

	"os"

	cutil "github.com/hyperledger/fabric/core/container/util"
	pb "github.com/hyperledger/fabric/protos"
	"github.com/spf13/viper"
)

var buildCmd = map[string]string{
	"build.gradle": "gradle -b build.gradle clean && gradle -b build.gradle build",
	"pom.xml":      "mvn -f pom.xml clean && mvn -f pom.xml package",
}

//return the type of build gradle/maven based on the file
//found in java chaincode project root
//build.gradle - gradle  - returns the first found build type
//pom.xml - maven
func getBuildCmd(packagePath string) (string, error) {
	files, err := ioutil.ReadDir(packagePath)
	if err != nil {
		return "", err
	} else {
		for _, f := range files {
			if !f.IsDir() {
				if buildCmd, ok := buildCmd[f.Name()]; ok == true {
					return buildCmd, nil
				}
			}
		}
		return "", fmt.Errorf("Build file not found")
	}
}

//tw is expected to have the chaincode in it from GenerateHashcode.
//This method will just package the dockerfile
func writeChaincodePackage(spec *pb.ChaincodeSpec, tw *tar.Writer) error {

	var urlLocation string
	var err error

	if strings.HasPrefix(spec.ChaincodeID.Path, "http://") ||
		strings.HasPrefix(spec.ChaincodeID.Path, "https://") {

		urlLocation, err = getCodeFromHTTP(spec.ChaincodeID.Path)
		defer func() {
			os.RemoveAll(urlLocation)
		}()
		if err != nil {
			return err
		}
	} else {
		urlLocation = spec.ChaincodeID.Path
	}

	if urlLocation == "" {
		return fmt.Errorf("empty url location")
	}

	if strings.LastIndex(urlLocation, "/") == len(urlLocation)-1 {
		urlLocation = urlLocation[:len(urlLocation)-1]
	}

	buildCmd, err := getBuildCmd(urlLocation)
	if err != nil {
		return err
	}
	var dockerFileContents string
	var buf []string

	if viper.GetBool("security.enabled") {
		//todo
	} else {
		buf = append(buf, cutil.GetDockerfileFromConfig("chaincode.java.Dockerfile"))
		buf = append(buf, "COPY src /root/chaincode")
		buf = append(buf, "RUN  cd /root/chaincode && "+buildCmd)
		buf = append(buf, "RUN  cp /root/chaincode/build/chaincode.jar /root")
		buf = append(buf, "RUN  cp /root/chaincode/build/libs/* /root/libs")
	}

	dockerFileContents = strings.Join(buf, "\n")
	dockerFileSize := int64(len([]byte(dockerFileContents)))
	//Make headers identical by using zero time
	var zeroTime time.Time
	tw.WriteHeader(&tar.Header{Name: "Dockerfile", Size: dockerFileSize, ModTime: zeroTime, AccessTime: zeroTime, ChangeTime: zeroTime})
	tw.Write([]byte(dockerFileContents))
	err = cutil.WriteJavaProjectToPackage(tw, urlLocation)
	if err != nil {
		return fmt.Errorf("Error writing Chaincode package contents: %s", err)
	}

	return nil
}
