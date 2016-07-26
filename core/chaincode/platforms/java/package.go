package java

import (
	"archive/tar"
	"fmt"
	"strings"
	"time"

	"github.com/spf13/viper"

	pb "github.com/hyperledger/fabric/protos"
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

	var newRunLine string
	if viper.GetBool("security.enabled") {
		//todo
	} else {
		newRunLine = fmt.Sprintf("COPY %s /root/\n"+
			"RUN cd /root/ && gradle build", urlLocation)
	}

	dockerFileContents := fmt.Sprintf("%s\n%s", viper.GetString("chaincode.java.Dockerfile"), newRunLine)
	dockerFileSize := int64(len([]byte(dockerFileContents)))

	//Make headers identical by using zero time
	var zeroTime time.Time
	tw.WriteHeader(&tar.Header{Name: "Dockerfile", Size: dockerFileSize, ModTime: zeroTime, AccessTime: zeroTime, ChangeTime: zeroTime})
	tw.Write([]byte(dockerFileContents))
	return nil
}
