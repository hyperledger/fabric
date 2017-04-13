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

package chaincodes

import (
	"fmt"
	"sync"
	"time"

	"github.com/hyperledger/fabric/msp"
	"github.com/hyperledger/fabric/peer/chaincode"
	"github.com/hyperledger/fabric/peer/common"
	pb "github.com/hyperledger/fabric/protos/peer"

	"golang.org/x/net/context"
)

//ShadowCCIntf interfaces to be implemented by shadow chaincodes
type ShadowCCIntf interface {
	//InitShadowCC initializes the shadow chaincode (will be called once for each chaincode)
	InitShadowCC(initArgs []string)

	//GetInvokeArgs gets invoke arguments from shadow
	GetInvokeArgs(ccnum int, iter int) [][]byte

	//PostInvoke passes the retvalue from the invoke to the shadow for post-processing
	PostInvoke(args [][]byte, retval []byte) error

	//GetQueryArgs mimics the Invoke and gets the query for an invoke
	GetQueryArgs(ccnum int, iter int) [][]byte

	//Validate the results against the query arguments
	Validate(args [][]byte, value []byte) error

	//GetNumQueries returns number of queries to perform
	GetNumQueries(numSuccessfulInvokes int) int

	//OverrideNumInvokes overrides the users number of invoke request
	OverrideNumInvokes(numInvokesPlanned int) int
}

//CCClient chaincode properties, config and runtime
type CCClient struct {
	//-------------config properties ------------
	//Name of the chaincode
	Name string

	//InitArgs used for deploying the chaincode
	InitArgs []string

	//Path to the chaincode
	Path string

	//NumFinalQueryAttempts number of times to try final query before giving up
	NumFinalQueryAttempts int

	//NumberOfInvokes number of iterations to do invoke on
	NumberOfInvokes int

	//DelayBetweenInvokeMs delay between each invoke
	DelayBetweenInvokeMs int

	//DelayBetweenQueryMs delay between each query
	DelayBetweenQueryMs int

	//TimeoutToAbortSecs timeout for aborting this chaincode processing
	TimeoutToAbortSecs int

	//Lang of chaincode
	Lang string

	//WaitAfterInvokeMs wait time before validating invokes for this chaincode
	WaitAfterInvokeMs int

	//Concurrency number of goroutines to spin
	Concurrency int

	//-------------runtime properties ------------
	//Unique number assigned to this CC by CCChecker
	ID int

	//shadow CC where the chaincode stats is maintained
	shadowCC ShadowCCIntf

	//number of iterations a shadow cc actually wants
	//this could be different from NumberOfInvokes
	overriddenNumInvokes int

	//numer of queries to perform
	//retrieved from the shadow CC
	numQueries int

	//current iteration of invoke
	currentInvokeIter int

	//start of invokes in epoch seconds
	invokeStartTime int64

	//end of invokes in epoch seconds
	invokeEndTime int64

	//error that stopped invoke iterations
	invokeErr error

	//current iteration of query
	currQueryIter []int

	//did the query work ?
	queryWorked []bool

	//error on a query in an iteration
	queryErrs []error
}

func (cc *CCClient) getChaincodeSpec(args [][]byte) *pb.ChaincodeSpec {
	return &pb.ChaincodeSpec{
		Type:        pb.ChaincodeSpec_Type(pb.ChaincodeSpec_Type_value[cc.Lang]),
		ChaincodeId: &pb.ChaincodeID{Path: cc.Path, Name: cc.Name},
		Input:       &pb.ChaincodeInput{Args: args},
	}
}

//doInvokes calls invoke for each iteration for the chaincode
//Stops at the first invoke with error
//currentInvokeIter contains the number of successful iterations
func (cc *CCClient) doInvokes(ctxt context.Context, chainID string,
	bc common.BroadcastClient, ec pb.EndorserClient, signer msp.SigningIdentity,
	wg *sync.WaitGroup, quit func() bool) error {

	//perhaps the shadow CC wants to override the number of iterations
	cc.overriddenNumInvokes = cc.shadowCC.OverrideNumInvokes(cc.NumberOfInvokes)

	var err error
	for cc.currentInvokeIter = 0; cc.currentInvokeIter < cc.overriddenNumInvokes; cc.currentInvokeIter++ {
		if quit() {
			break
		}
		args := cc.shadowCC.GetInvokeArgs(cc.ID, cc.currentInvokeIter)

		spec := cc.getChaincodeSpec(args)

		if quit() {
			break
		}

		var pResp *pb.ProposalResponse
		if pResp, err = chaincode.ChaincodeInvokeOrQuery(spec, chainID, true, signer, ec, bc); err != nil {
			cc.invokeErr = err
			break
		}

		resp := pResp.Response.Payload
		if err = cc.shadowCC.PostInvoke(args, resp); err != nil {
			cc.invokeErr = err
			break
		}

		if quit() {
			break
		}

		//don't sleep for the last iter
		if cc.DelayBetweenInvokeMs > 0 && cc.currentInvokeIter < (cc.overriddenNumInvokes-1) {
			time.Sleep(time.Duration(cc.DelayBetweenInvokeMs) * time.Millisecond)
		}
	}

	return err
}

//Run test over given number of iterations
//  i will be unique across chaincodes and can be used as a key
//    this is useful if chaincode occurs multiple times in the array of chaincodes
func (cc *CCClient) Run(ctxt context.Context, chainID string, bc common.BroadcastClient, ec pb.EndorserClient, signer msp.SigningIdentity, wg *sync.WaitGroup) error {
	defer wg.Done()

	var (
		quit bool
		err  error
	)

	done := make(chan struct{})
	go func() {
		defer func() { done <- struct{}{} }()

		//return the quit closure for validation within validateIter
		quitF := func() bool { return quit }

		//start of invokes
		cc.invokeStartTime = time.Now().UnixNano() / 1000000

		err = cc.doInvokes(ctxt, chainID, bc, ec, signer, wg, quitF)

		//end of invokes
		cc.invokeEndTime = time.Now().UnixNano() / 1000000
	}()

	//we could be done or cancelled or timedout
	select {
	case <-ctxt.Done():
		quit = true
		return nil
	case <-done:
		return err
	case <-time.After(time.Duration(cc.TimeoutToAbortSecs) * time.Second):
		quit = true
		return fmt.Errorf("Aborting due to timeoutout!!")
	}
}

//validates the invoke iteration for this chaincode
func (cc *CCClient) validateIter(ctxt context.Context, iter int, chainID string, bc common.BroadcastClient, ec pb.EndorserClient, signer msp.SigningIdentity, wg *sync.WaitGroup, quit func() bool) {
	defer wg.Done()
	args := cc.shadowCC.GetQueryArgs(cc.ID, iter)

	spec := cc.getChaincodeSpec(args)

	//lets try a few times
	for cc.currQueryIter[iter] = 0; cc.currQueryIter[iter] < cc.NumFinalQueryAttempts; cc.currQueryIter[iter]++ {
		if quit() {
			break
		}

		var pResp *pb.ProposalResponse
		var err error
		if pResp, err = chaincode.ChaincodeInvokeOrQuery(spec, chainID, false, signer, ec, bc); err != nil {
			cc.queryErrs[iter] = err
			break
		}

		resp := pResp.Response.Payload

		if quit() {
			break
		}

		//if it fails, we try again
		if err = cc.shadowCC.Validate(args, resp); err == nil {
			//appears to have worked
			cc.queryWorked[iter] = true
			cc.queryErrs[iter] = nil
			break
		}

		//save query error
		cc.queryErrs[iter] = err

		if quit() {
			break
		}

		//try again
		if cc.DelayBetweenQueryMs > 0 {
			time.Sleep(time.Duration(cc.DelayBetweenQueryMs) * time.Millisecond)
		}
	}

	return
}

//Validate test that was Run. Each successful iteration in the run is validated against
func (cc *CCClient) Validate(ctxt context.Context, chainID string, bc common.BroadcastClient, ec pb.EndorserClient, signer msp.SigningIdentity, wg *sync.WaitGroup) error {
	defer wg.Done()

	//this will signal inner validators to get out via
	//closure
	var quit bool

	//use 1 so sender doesn't block (he doesn't care if is was receivd.
	//makes sure goroutine exits)
	done := make(chan struct{}, 1)
	go func() {
		defer func() { done <- struct{}{} }()

		var innerwg sync.WaitGroup
		innerwg.Add(cc.currentInvokeIter)

		//initialize for querying
		cc.currQueryIter = make([]int, cc.currentInvokeIter)
		cc.queryWorked = make([]bool, cc.currentInvokeIter)
		cc.queryErrs = make([]error, cc.currentInvokeIter)

		//give some time for the invokes to commit for this cc
		time.Sleep(time.Duration(cc.WaitAfterInvokeMs) * time.Millisecond)

		//return the quit closure for validation within validateIter
		quitF := func() bool { return quit }

		cc.numQueries = cc.shadowCC.GetNumQueries(cc.currentInvokeIter)

		//try only till successful invoke iterations
		for i := 0; i < cc.numQueries; i++ {
			go func(iter int) {
				cc.validateIter(ctxt, iter, chainID, bc, ec, signer, &innerwg, quitF)
			}(i)
		}

		//shouldn't block the sender go routine on cleanup
		qDone := make(chan struct{}, 1)

		//wait for the above queries to be done
		go func() { innerwg.Wait(); qDone <- struct{}{} }()

		//we could be done or cancelled
		select {
		case <-qDone:
		case <-ctxt.Done():
		}
	}()

	//we could be done or cancelled or timedout
	select {
	case <-ctxt.Done():
		//we don't know why it was cancelled but it was cancelled
		quit = true
		return nil
	case <-done:
		//for done does not return an err. The query validation stores chaincode errors
		//Only error that's left to handle is timeout error for this chaincode below
		return nil
	case <-time.After(time.Duration(cc.TimeoutToAbortSecs) * time.Second):
		quit = true
		return fmt.Errorf("Aborting due to timeoutout!!")
	}
}

//Report reports chaincode test execution, iter by iter
func (cc *CCClient) Report(verbose bool, chainID string) {
	fmt.Printf("%s/%s(%d)\n", cc.Name, chainID, cc.ID)
	fmt.Printf("\tNum successful invokes: %d(%d,%d)\n", cc.currentInvokeIter, cc.NumberOfInvokes, cc.overriddenNumInvokes)
	if cc.invokeErr != nil {
		fmt.Printf("\tError on invoke: %s\n", cc.invokeErr)
	}
	//test to see if validate was called (validate alloc the arrays, one of which is queryWorked)
	if cc.queryWorked != nil {
		for i := 0; i < cc.numQueries; i++ {
			fmt.Printf("\tQuery(%d) : succeeded-%t, num trials-%d(%d), error if any(%s)\n", i, cc.queryWorked[i], cc.currQueryIter[i], cc.NumFinalQueryAttempts, cc.queryErrs[i])
		}
	} else {
		fmt.Printf("\tQuery validation appears not have been performed(#invokes-%d). timed out ?\n", cc.currentInvokeIter)
	}
	//total actual time for cc.currentInvokeIter
	invokeTime := cc.invokeEndTime - cc.invokeStartTime - int64(cc.DelayBetweenInvokeMs*(cc.currentInvokeIter-1))
	fmt.Printf("\tTime for invokes(ms): %d\n", invokeTime)

	fmt.Printf("\tFinal query worked ? %t\n", cc.queryWorked)
}
