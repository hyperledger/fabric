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

package container

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"fmt"
	"io"
	"io/ioutil"
	"testing"
	"time"

	"github.com/hyperledger/fabric/core/container/ccintf"
	"github.com/hyperledger/fabric/core/container/dockercontroller"
	"github.com/hyperledger/fabric/core/container/inproccontroller"
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

//set to true by providing "-run-controller-tests" command line option... Tests will create a docker image called "simple"
var runTests bool

func testForSkip(t *testing.T) {
	//run tests
	if !runTests {
		t.SkipNow()
	}
}

func TestVMCBuildImage(t *testing.T) {
	testForSkip(t)
	var ctxt = context.Background()

	//get the tarball for codechain
	tarRdr, err := getCodeChainBytesInMem()
	if err != nil {
		t.Fail()
		t.Logf("Error reading tar file: %s", err)
		return
	}

	c := make(chan struct{})

	//creat a CreateImageReq obj and send it to VMCProcess
	go func() {
		defer close(c)
		cir := CreateImageReq{CCID: ccintf.CCID{ChaincodeSpec: &pb.ChaincodeSpec{ChaincodeId: &pb.ChaincodeID{Name: "simple"}}}, Reader: tarRdr}
		_, err := VMCProcess(ctxt, "Docker", cir)
		if err != nil {
			t.Fail()
			t.Logf("Error creating image: %s", err)
			return
		}
	}()

	//wait for VMController to complete.
	fmt.Println("VMCBuildImage-waiting for response")
	<-c
}

func TestVMCStartContainer(t *testing.T) {
	testForSkip(t)

	var ctxt = context.Background()

	c := make(chan struct{})

	//create a StartImageReq obj and send it to VMCProcess
	go func() {
		defer close(c)
		sir := StartImageReq{CCID: ccintf.CCID{ChaincodeSpec: &pb.ChaincodeSpec{ChaincodeId: &pb.ChaincodeID{Name: "simple"}}}}
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
	stopr := StopImageReq{CCID: ccintf.CCID{ChaincodeSpec: &pb.ChaincodeSpec{ChaincodeId: &pb.ChaincodeID{Name: "simple"}}}, Timeout: 0, Dontremove: true}
	VMCProcess(ctxt, "Docker", stopr)
}

func TestVMCCreateAndStartContainer(t *testing.T) {
	testForSkip(t)

	var ctxt = context.Background()

	c := make(chan struct{})

	//create a StartImageReq obj and send it to VMCProcess
	go func() {
		defer close(c)

		//stop and delete the container first (if it exists)
		stopir := StopImageReq{CCID: ccintf.CCID{ChaincodeSpec: &pb.ChaincodeSpec{ChaincodeId: &pb.ChaincodeID{Name: "simple"}}}, Timeout: 0}
		VMCProcess(ctxt, "Docker", stopir)

		startir := StartImageReq{CCID: ccintf.CCID{ChaincodeSpec: &pb.ChaincodeSpec{ChaincodeId: &pb.ChaincodeID{Name: "simple"}}}}
		r, err := VMCProcess(ctxt, "Docker", startir)
		if err != nil {
			t.Fail()
			t.Logf("Error starting container: %s", err)
			return
		}
		vmcresp, ok := r.(VMCResp)
		if !ok {
			t.Fatalf("invalid response from VMCProcess")
		}
		if vmcresp.Err != nil {
			t.Fail()
			t.Logf("docker error starting container: %s", vmcresp.Err)
			return
		}
	}()

	//wait for VMController to complete.
	fmt.Println("VMCStartContainer-waiting for response")
	<-c
	//stopr := StopImageReq{ID: "simple", Timeout: 0, Dontremove: true}
	//VMCProcess(ctxt, "Docker", stopr)
}

func TestVMCSyncStartContainer(t *testing.T) {
	testForSkip(t)

	var ctxt = context.Background()

	//creat a StartImageReq obj and send it to VMCProcess
	sir := StartImageReq{CCID: ccintf.CCID{ChaincodeSpec: &pb.ChaincodeSpec{ChaincodeId: &pb.ChaincodeID{Name: "simple"}}}}
	_, err := VMCProcess(ctxt, "Docker", sir)
	if err != nil {
		t.Fail()
		t.Logf("Error starting container: %s", err)
		return
	}
	stopr := StopImageReq{CCID: ccintf.CCID{ChaincodeSpec: &pb.ChaincodeSpec{ChaincodeId: &pb.ChaincodeID{Name: "simple"}}}, Timeout: 0, Dontremove: true}
	VMCProcess(ctxt, "Docker", stopr)
}

func TestVMCStopContainer(t *testing.T) {
	testForSkip(t)

	var ctxt = context.Background()

	c := make(chan struct{})

	//creat a StopImageReq obj and send it to VMCProcess
	go func() {
		defer close(c)
		sir := StopImageReq{CCID: ccintf.CCID{ChaincodeSpec: &pb.ChaincodeSpec{ChaincodeId: &pb.ChaincodeID{Name: "simple"}}}, Timeout: 0}
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

	vm = vmcontroller.newVM("")
	dvm = vm.(*dockercontroller.DockerVM)
	assert.NotNil(t, dvm, "Requested default VM but newVM did not return dockercontroller.DockerVM")
}
