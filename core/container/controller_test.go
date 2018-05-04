/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package container

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"testing"
	"time"

	"github.com/hyperledger/fabric/common/util"
	"github.com/hyperledger/fabric/core/container/api"
	"github.com/hyperledger/fabric/core/container/ccintf"
	"github.com/hyperledger/fabric/core/container/dockercontroller"
	"github.com/hyperledger/fabric/core/container/inproccontroller"
	"github.com/hyperledger/fabric/core/testutil"
	pb "github.com/hyperledger/fabric/protos/peer"
	"github.com/stretchr/testify/assert"
	"golang.org/x/net/context"
)

/**** not using actual files from file system for testing.... use these funcs if we want to do that
func getCodeChainBytes(pathtocodechain string) (io.Reader, error) {
	inputbuf := bytes.NewBuffer(nil)
	gw := gzip.NewWriter(inputbuf)
	tr := tar.NewWriter(gw)
	// Get the Tar contents for the image
	err := writeCodeChainTar(pathtocodechain, tr)
	tr.Close()
	gw.Close()
	if err != nil {
		return nil, errors.New(fmt.Sprintf("Error getting codechain tar: %s", err))
	}
        ioutil.WriteFile("/tmp/chaincode.tar", inputbuf.Bytes(), 0644)
	return inputbuf, nil
}

func writeCodeChainTar(pathtocodechain string, tw *tar.Writer) error {
	root_directory := pathtocodechain //use full path
	fmt.Printf("tar %s start(%s)\n", root_directory, time.Now())

	walkFn := func(path string, info os.FileInfo, err error) error {
	        fmt.Printf("path %s(%s)\n", path, info.Name())
                if info == nil {
	             return errors.New(fmt.Sprintf("Error walking the path: %s", path))
                }

		if info.Mode().IsDir() {
			return nil
		}
		// Because of scoping we can reference the external root_directory variable
		//new_path := fmt.Sprintf("%s", path[len(root_directory):])
		new_path := info.Name()

		if len(new_path) == 0 {
			return nil
		}

		fr, err := os.Open(path)
		if err != nil {
			return err
		}
		defer fr.Close()

		if h, err := tar.FileInfoHeader(info, new_path); err != nil {
			fmt.Printf(fmt.Sprintf("Error getting FileInfoHeader: %s\n", err))
			return err
		} else {
			h.Name = new_path
			if err = tw.WriteHeader(h); err != nil {
				fmt.Printf(fmt.Sprintf("Error writing header: %s\n", err))
				return err
			}
		}
		if length, err := io.Copy(tw, fr); err != nil {
			return err
		} else {
			fmt.Printf("Length of entry = %d\n", length)
		}
		return nil
	}

	if err := filepath.Walk(root_directory, walkFn); err != nil {
		fmt.Printf("Error walking root_directory: %s\n", err)
		return err
	} else {
		// Write the tar file out
		if err := tw.Close(); err != nil {
                    return err
		}
	}
	fmt.Printf("tar end = %s\n", time.Now())
	return nil
}
*********************/

func getCodeChainBytesInMem() (io.Reader, error) {
	startTime := time.Now()
	inputbuf := bytes.NewBuffer(nil)
	gw := gzip.NewWriter(inputbuf)
	tr := tar.NewWriter(gw)
	dockerFileContents := []byte("FROM busybox:latest\n\nCMD echo hello")
	dockerFileSize := int64(len([]byte(dockerFileContents)))

	tr.WriteHeader(&tar.Header{Name: "Dockerfile", Size: dockerFileSize, ModTime: startTime, AccessTime: startTime, ChangeTime: startTime})
	tr.Write([]byte(dockerFileContents))
	tr.Close()
	gw.Close()
	ioutil.WriteFile("/tmp/chaincode.tar", inputbuf.Bytes(), 0644)
	return inputbuf, nil
}

//CreateImageReq - properties for creating an container image
type CreateImageReq struct {
	ccintf.CCID
	Reader io.Reader
	Args   []string
	Env    []string
}

func (bp CreateImageReq) do(ctxt context.Context, v api.VM) VMCResp {
	var resp VMCResp
	if err := v.Deploy(ctxt, bp.CCID, bp.Args, bp.Env, bp.Reader); err != nil {
		resp = VMCResp{Err: err}
	} else {
		resp = VMCResp{}
	}
	return resp
}
func (bp CreateImageReq) getCCID() ccintf.CCID {
	return bp.CCID
}

func createImage() {
	var ctxt = context.Background()
	//get the tarball for codechain
	tarRdr, err := getCodeChainBytesInMem()
	if err != nil {
		panic(err)
	}

	//create a the image needed for the rest of the tests obj and send it to VMCProcess
	cir := CreateImageReq{CCID: ccintf.CCID{ChaincodeSpec: &pb.ChaincodeSpec{ChaincodeId: &pb.ChaincodeID{Name: "simple"}}}, Reader: tarRdr}
	resp, err := VMCProcess(ctxt, "Docker", cir)
	eResp := VMCResp{}
	if err != nil || resp != eResp {
		panic(fmt.Sprintf("err: %s, resp: %s", err, resp))
	}
}

func TestMain(m *testing.M) {
	testutil.SetupTestConfig()
	createImage()
	os.Exit(m.Run())
}

func TestVMCStartContainer(t *testing.T) {
	var ctxt = context.Background()

	c := make(chan struct{})

	//create a StartImageReq obj and send it to VMCProcess
	go func() {
		defer close(c)
		sir := StartContainerReq{CCID: ccintf.CCID{ChaincodeSpec: &pb.ChaincodeSpec{ChaincodeId: &pb.ChaincodeID{Name: "simple"}}}}
		_, err := VMCProcess(ctxt, "Docker", sir)
		if err != nil {
			t.Fail()
			t.Logf("Error starting container: %s", err)
			return
		}
	}()

	//wait for VMController to complete.
	fmt.Println("VMCStartContainer-waiting for response")
	<-c
	stopr := StopContainerReq{CCID: ccintf.CCID{ChaincodeSpec: &pb.ChaincodeSpec{ChaincodeId: &pb.ChaincodeID{Name: "simple"}}}, Timeout: 0, Dontremove: true}
	VMCProcess(ctxt, "Docker", stopr)
}

func TestVMCCreateAndStartContainer(t *testing.T) {
	var ctxt = context.Background()

	c := make(chan struct{})

	//create a StartImageReq obj and send it to VMCProcess
	go func() {
		defer close(c)

		//stop and delete the container first (if it exists)
		stopir := StopContainerReq{CCID: ccintf.CCID{ChaincodeSpec: &pb.ChaincodeSpec{ChaincodeId: &pb.ChaincodeID{Name: "simple"}}}, Timeout: 0}
		VMCProcess(ctxt, "Docker", stopir)

		startir := StartContainerReq{CCID: ccintf.CCID{ChaincodeSpec: &pb.ChaincodeSpec{ChaincodeId: &pb.ChaincodeID{Name: "simple"}}}}
		r, err := VMCProcess(ctxt, "Docker", startir)
		if err != nil {
			t.Fail()
			t.Logf("Error starting container: %s", err)
			return
		}
		if r.Err != nil {
			t.Fail()
			t.Logf("docker error starting container: %s", r.Err)
			return
		}
	}()

	//wait for VMController to complete.
	fmt.Println("VMCStartContainer-waiting for response")
	<-c
	//stopr := StopContainerReq{ID: "simple", Timeout: 0, Dontremove: true}
	//VMCProcess(ctxt, "Docker", stopr)
}

func TestVMCSyncStartContainer(t *testing.T) {
	var ctxt = context.Background()

	//creat a StartImageReq obj and send it to VMCProcess
	sir := StartContainerReq{CCID: ccintf.CCID{ChaincodeSpec: &pb.ChaincodeSpec{ChaincodeId: &pb.ChaincodeID{Name: "simple"}}}}
	_, err := VMCProcess(ctxt, "Docker", sir)
	if err != nil {
		t.Fail()
		t.Logf("Error starting container: %s", err)
		return
	}
	stopr := StopContainerReq{CCID: ccintf.CCID{ChaincodeSpec: &pb.ChaincodeSpec{ChaincodeId: &pb.ChaincodeID{Name: "simple"}}}, Timeout: 0, Dontremove: true}
	VMCProcess(ctxt, "Docker", stopr)
}

func TestVMCStopContainer(t *testing.T) {
	var ctxt = context.Background()

	c := make(chan struct{})

	//creat a StopContainerReq obj and send it to VMCProcess
	go func() {
		defer close(c)
		sir := StopContainerReq{CCID: ccintf.CCID{ChaincodeSpec: &pb.ChaincodeSpec{ChaincodeId: &pb.ChaincodeID{Name: "simple"}}}, Timeout: 0}
		_, err := VMCProcess(ctxt, "Docker", sir)
		if err != nil {
			t.Fail()
			t.Logf("Error stopping container: %s", err)
			return
		}
	}()

	//wait for VMController to complete.
	fmt.Println("VMCStopContainer-waiting for response")
	<-c
}

func TestNewVM(t *testing.T) {
	vm := vmcontroller.newVM("Docker")
	dvm := vm.(*dockercontroller.DockerVM)
	assert.NotNil(t, dvm, "Requested Docker VM but newVM did not return dockercontroller.DockerVM")

	vm = vmcontroller.newVM("System")
	ivm := vm.(*inproccontroller.InprocVM)
	assert.NotNil(t, ivm, "Requested System VM but newVM did not return inproccontroller.InprocVM")

	assert.Panics(t, func() { vmcontroller.newVM("") }, "Requested unknown VM but did not panic")
}

func TestVM_GetChaincodePackageBytes(t *testing.T) {
	_, err := GetChaincodePackageBytes(nil)
	assert.Error(t, err,
		"GetChaincodePackageBytes did not return error when chaincode spec is nil")
	spec := &pb.ChaincodeSpec{ChaincodeId: nil}
	_, err = GetChaincodePackageBytes(spec)
	assert.Error(t, err, "Error expected when GetChaincodePackageBytes is called with nil chaincode ID")
	assert.Contains(t, err.Error(), "invalid chaincode spec")
	spec = &pb.ChaincodeSpec{Type: pb.ChaincodeSpec_GOLANG,
		ChaincodeId: nil,
		Input:       &pb.ChaincodeInput{Args: util.ToChaincodeArgs("f")}}
	_, err = GetChaincodePackageBytes(spec)
	assert.Error(t, err,
		"GetChaincodePackageBytes did not return error when chaincode ID is nil")
}
