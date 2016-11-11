package java

import (
	"archive/tar"
	"fmt"
	"strings"
	"time"

	cutil "github.com/hyperledger/fabric/core/container/util"
	pb "github.com/hyperledger/fabric/protos/peer"
	"github.com/spf13/viper"
)

//tw is expected to have the chaincode in it from GenerateHashcode.
//This method will just package the dockerfile
func writeChaincodePackage(spec *pb.ChaincodeSpec, tw *tar.Writer) error {

	var urlLocation string
	if strings.HasPrefix(spec.ChaincodeID.Path, "http://") {
		urlLocation = spec.ChaincodeID.Path[7:]
	} else if strings.HasPrefix(spec.ChaincodeID.Path, "https://") {
		urlLocation = spec.ChaincodeID.Path[8:]
	} else {
		urlLocation = spec.ChaincodeID.Path
		//		if !strings.HasPrefix(urlLocation, "/") {
		//			wd := ""
		//			wd, _ = os.Getwd()
		//			urlLocation = wd + "/" + urlLocation
		//		}
	}

	if urlLocation == "" {
		return fmt.Errorf("empty url location")
	}

	if strings.LastIndex(urlLocation, "/") == len(urlLocation)-1 {
		urlLocation = urlLocation[:len(urlLocation)-1]
	}
	urlLocation = urlLocation[strings.LastIndex(urlLocation, "/")+1:]

	var dockerFileContents string
	var buf []string

	if viper.GetBool("security.enabled") {
		//todo
	} else {
		buf = append(buf, cutil.GetDockerfileFromConfig("chaincode.java.Dockerfile"))
		buf = append(buf, "COPY src /root")
		buf = append(buf, "RUN gradle -b build.gradle build")
		buf = append(buf, "RUN unzip -od /root build/distributions/Chaincode.zip")

	}
	dockerFileContents = strings.Join(buf, "\n")

	dockerFileSize := int64(len([]byte(dockerFileContents)))

	//Make headers identical by using zero time
	var zeroTime time.Time
	tw.WriteHeader(&tar.Header{Name: "Dockerfile", Size: dockerFileSize, ModTime: zeroTime, AccessTime: zeroTime, ChangeTime: zeroTime})
	tw.Write([]byte(dockerFileContents))
	err := cutil.WriteJavaProjectToPackage(tw, spec.ChaincodeID.Path)
	if err != nil {
		return fmt.Errorf("Error writing Chaincode package contents: %s", err)
	}

	return nil
}
