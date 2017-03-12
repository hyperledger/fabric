package golang

import (
	"testing"

	"archive/tar"
	"bytes"
	"compress/gzip"
	"time"

	pb "github.com/hyperledger/fabric/protos/peer"
)

func writeBytesToPackage(name string, payload []byte, mode int64, tw *tar.Writer) error {
	//Make headers identical by using zero time
	var zeroTime time.Time
	tw.WriteHeader(&tar.Header{Name: name, Size: int64(len(payload)), ModTime: zeroTime, AccessTime: zeroTime, ChangeTime: zeroTime, Mode: mode})
	tw.Write(payload)

	return nil
}

func generateFakeCDS(path, file string, mode int64) (*pb.ChaincodeDeploymentSpec, error) {
	codePackage := bytes.NewBuffer(nil)
	gw := gzip.NewWriter(codePackage)
	tw := tar.NewWriter(gw)

	payload := make([]byte, 25, 25)
	err := writeBytesToPackage(file, payload, mode, tw)
	if err != nil {
		return nil, err
	}

	tw.Close()
	gw.Close()

	cds := &pb.ChaincodeDeploymentSpec{
		ChaincodeSpec: &pb.ChaincodeSpec{
			ChaincodeId: &pb.ChaincodeID{
				Name: "Bad Code",
				Path: path,
			},
		},
		CodePackage: codePackage.Bytes(),
	}

	return cds, nil
}

type spec struct {
	Path, File      string
	Mode            int64
	SuccessExpected bool
}

func TestValidateCDS(t *testing.T) {
	platform := &Platform{}

	specs := make([]spec, 0)
	specs = append(specs, spec{Path: "path/to/nowhere", File: "/bin/warez", Mode: 0100400, SuccessExpected: false})
	specs = append(specs, spec{Path: "path/to/somewhere", File: "/src/path/to/somewhere/main.go", Mode: 0100400, SuccessExpected: true})
	specs = append(specs, spec{Path: "path/to/somewhere", File: "/src/path/to/somewhere/warez", Mode: 0100555, SuccessExpected: false})

	for _, s := range specs {
		cds, err := generateFakeCDS(s.Path, s.File, s.Mode)

		err = platform.ValidateDeploymentSpec(cds)
		if s.SuccessExpected == true && err != nil {
			t.Errorf("Unexpected failure: %s", err)
		}
		if s.SuccessExpected == false && err == nil {
			t.Log("Expected validation failure")
			t.Fail()
		}
	}
}

func Test_writeGopathSrc(t *testing.T) {

	inputbuf := bytes.NewBuffer(nil)
	tw := tar.NewWriter(inputbuf)

	err := writeGopathSrc(tw, "")
	if err != nil {
		t.Fail()
		t.Logf("Error writing gopath src: %s", err)
	}
	//ioutil.WriteFile("/tmp/chaincode_deployment.tar", inputbuf.Bytes(), 0644)

}
