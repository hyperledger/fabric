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

func createEnv() *VMController {
	vmc := NewVMController()
	var ctxt = context.Background()
	//get the tarball for codechain
	tarRdr, err := getCodeChainBytesInMem()
	if err != nil {
		panic(err)
	}

	//create a the image needed for the rest of the tests obj and send it to Process
	cir := CreateImageReq{CCID: ccintf.CCID{ChaincodeSpec: &pb.ChaincodeSpec{ChaincodeId: &pb.ChaincodeID{Name: "simple"}}}, Reader: tarRdr}
	resp, err := vmc.Process(ctxt, "Docker", cir)
	eResp := VMCResp{}
	if err != nil || resp != eResp {
		panic(fmt.Sprintf("err: %s, resp: %s", err, resp))
	}
	return vmc
}

func init() {
	testutil.SetupTestConfig()
}

func TestVMCStartContainer(t *testing.T) {
	var ctxt = context.Background()

	vmc := createEnv()

	sir := StartContainerReq{CCID: ccintf.CCID{ChaincodeSpec: &pb.ChaincodeSpec{ChaincodeId: &pb.ChaincodeID{Name: "simple"}}}}
	_, err := vmc.Process(ctxt, "Docker", sir)
	if err != nil {
		t.Fail()
		t.Logf("Error starting container: %s", err)
		return
	}
	stopr := StopContainerReq{CCID: ccintf.CCID{ChaincodeSpec: &pb.ChaincodeSpec{ChaincodeId: &pb.ChaincodeID{Name: "simple"}}}, Timeout: 0, Dontremove: true}
	vmc.Process(ctxt, "Docker", stopr)
}

func TestVMCCreateAndStartContainer(t *testing.T) {
	var ctxt = context.Background()

	vmc := createEnv()

	//stop and delete the container first (if it exists)
	stopir := StopContainerReq{CCID: ccintf.CCID{ChaincodeSpec: &pb.ChaincodeSpec{ChaincodeId: &pb.ChaincodeID{Name: "simple"}}}, Timeout: 0}
	vmc.Process(ctxt, "Docker", stopir)

	startir := StartContainerReq{CCID: ccintf.CCID{ChaincodeSpec: &pb.ChaincodeSpec{ChaincodeId: &pb.ChaincodeID{Name: "simple"}}}}
	r, err := vmc.Process(ctxt, "Docker", startir)
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
}

func TestVMCSyncStartContainer(t *testing.T) {
	var ctxt = context.Background()
	vmc := createEnv()

	//creat a StartImageReq obj and send it to Process
	sir := StartContainerReq{CCID: ccintf.CCID{ChaincodeSpec: &pb.ChaincodeSpec{ChaincodeId: &pb.ChaincodeID{Name: "simple"}}}}
	_, err := vmc.Process(ctxt, "Docker", sir)
	if err != nil {
		t.Fail()
		t.Logf("Error starting container: %s", err)
		return
	}
	stopr := StopContainerReq{CCID: ccintf.CCID{ChaincodeSpec: &pb.ChaincodeSpec{ChaincodeId: &pb.ChaincodeID{Name: "simple"}}}, Timeout: 0, Dontremove: true}
	vmc.Process(ctxt, "Docker", stopr)
}

func TestVMCStopContainer(t *testing.T) {
	var ctxt = context.Background()
	vmc := createEnv()

	sir := StopContainerReq{CCID: ccintf.CCID{ChaincodeSpec: &pb.ChaincodeSpec{ChaincodeId: &pb.ChaincodeID{Name: "simple"}}}, Timeout: 0}
	_, err := vmc.Process(ctxt, "Docker", sir)
	if err != nil {
		t.Fail()
		t.Logf("Error stopping container: %s", err)
		return
	}
}

func TestNewVM(t *testing.T) {
	vmc := NewVMController()
	vm := vmc.newVM("Docker")
	dvm := vm.(*dockercontroller.DockerVM)
	assert.NotNil(t, dvm, "Requested Docker VM but newVM did not return dockercontroller.DockerVM")

	vm = vmc.newVM("System")
	ivm := vm.(*inproccontroller.InprocVM)
	assert.NotNil(t, ivm, "Requested System VM but newVM did not return inproccontroller.InprocVM")

	assert.Panics(t, func() { vmc.newVM("") }, "Requested unknown VM but did not panic")
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
