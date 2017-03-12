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

package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"sync"
	"time"

	"golang.org/x/net/context"

	"github.com/hyperledger/fabric/examples/ccchecker/chaincodes"
	"github.com/hyperledger/fabric/peer/common"
)

//global ccchecker params
var ccchecker *CCChecker

//CCChecker encapsulates ccchecker properties and runtime
type CCChecker struct {
	//Chaincodes to do ccchecker over (see ccchecker.json for defaults)
	Chaincodes []*chaincodes.CCClient
	//TimeoutToAbortSecs abort deadline
	TimeoutToAbortSecs int
	//ChainName name of the chain
	ChainName string
}

//LoadCCCheckerParams read the ccchecker params from a file
func LoadCCCheckerParams(file string) error {
	var b []byte
	var err error
	if b, err = ioutil.ReadFile(file); err != nil {
		return fmt.Errorf("Cannot read config file %s\n", err)
	}
	sp := &CCChecker{}
	err = json.Unmarshal(b, &sp)
	if err != nil {
		return fmt.Errorf("error unmarshalling ccchecker: %s\n", err)
	}

	ccchecker = &CCChecker{}
	id := 0
	for _, scc := range sp.Chaincodes {
		//concurrency <=0 will be dropped
		if scc.Concurrency > 0 {
			for i := 0; i < scc.Concurrency; i++ {
				tmp := &chaincodes.CCClient{}
				*tmp = *scc
				tmp.ID = id
				id = id + 1
				ccchecker.Chaincodes = append(ccchecker.Chaincodes, tmp)
			}
		}
	}

	ccchecker.TimeoutToAbortSecs = sp.TimeoutToAbortSecs
	ccchecker.ChainName = sp.ChainName

	return nil
}

//CCCheckerInit assigns shadow chaincode to each of the CCClient from registered shadow chaincodes
func CCCheckerInit() {
	if ccchecker == nil {
		fmt.Printf("LoadCCCheckerParams needs to be called before init\n")
		os.Exit(1)
	}

	if err := chaincodes.RegisterCCClients(ccchecker.Chaincodes); err != nil {
		panic(fmt.Sprintf("%s", err))
	}
}

//CCCheckerRun main loops that will run the tests and cleanup
func CCCheckerRun(orderingEndpoint string, report bool, verbose bool) error {
	//connect with Broadcast client
	bc, err := common.GetBroadcastClient(orderingEndpoint, false, "")
	if err != nil {
		return err
	}
	defer bc.Close()

	ec, err := common.GetEndorserClient()
	if err != nil {
		return err
	}

	signer, err := common.GetDefaultSigner()
	if err != nil {
		return err
	}

	//when the wait's timeout and get out of ccchecker, we
	//cancel and release all goroutines
	ctxt, cancel := context.WithCancel(context.Background())
	defer cancel()

	var ccsWG sync.WaitGroup
	ccsWG.Add(len(ccchecker.Chaincodes))

	//an anonymous struct to hold failures
	var failures struct {
		sync.Mutex
		failedCCClients int
	}

	//run the invokes
	ccerrs := make([]error, len(ccchecker.Chaincodes))
	for _, cc := range ccchecker.Chaincodes {
		go func(cc2 *chaincodes.CCClient) {
			if ccerrs[cc2.ID] = cc2.Run(ctxt, ccchecker.ChainName, bc, ec, signer, &ccsWG); ccerrs[cc2.ID] != nil {
				failures.Lock()
				failures.failedCCClients = failures.failedCCClients + 1
				failures.Unlock()
			}
		}(cc)
	}

	//wait or timeout
	err = ccchecker.wait(&ccsWG)

	//verify results
	if err == nil && failures.failedCCClients < len(ccchecker.Chaincodes) {
		ccsWG = sync.WaitGroup{}
		ccsWG.Add(len(ccchecker.Chaincodes) - failures.failedCCClients)
		for _, cc := range ccchecker.Chaincodes {
			go func(cc2 *chaincodes.CCClient) {
				if ccerrs[cc2.ID] == nil {
					ccerrs[cc2.ID] = cc2.Validate(ctxt, ccchecker.ChainName, bc, ec, signer, &ccsWG)
				} else {
					fmt.Printf("Ignoring [%v] for validation as it returned err %s\n", cc2, ccerrs[cc2.ID])
				}
			}(cc)
		}

		//wait or timeout
		err = ccchecker.wait(&ccsWG)
	}

	if report {
		for _, cc := range ccchecker.Chaincodes {
			cc.Report(verbose, ccchecker.ChainName)
		}
	}

	return err
}

func (s *CCChecker) wait(ccsWG *sync.WaitGroup) error {
	done := make(chan struct{})
	go func() {
		ccsWG.Wait()
		done <- struct{}{}
	}()
	select {
	case <-done:
		return nil
	case <-time.After(time.Duration(s.TimeoutToAbortSecs) * time.Second):
		return fmt.Errorf("Aborting due to timeoutout!!")
	}
}
