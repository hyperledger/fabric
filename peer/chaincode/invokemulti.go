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

package chaincode

import (
	"errors"
	"fmt"
	"time"

	"github.com/spf13/cobra"
)

var chaincodeInvokeMultiCmd *cobra.Command

// invokeCmd returns the cobra command for Chaincode Invoke
func invokeMultiCmd(cf *ChaincodeCmdFactory) *cobra.Command {
	chaincodeInvokeMultiCmd = &cobra.Command{
		Use:       "invokemulti",
		Short:     fmt.Sprintf("Invoke the specified %s multiple times.", chainFuncName),
		Long:      fmt.Sprintf("Invoke the specified %s multiple times. It will try to commit the endorsed transaction to the network.", chainFuncName),
		ValidArgs: []string{"1"},
		RunE: func(cmd *cobra.Command, args []string) error {
			// START TEST CODE
			concurrencyLimit := 2000
			requests := 2000
			semaphoreChan := make(chan struct{}, concurrencyLimit)

			// make sure we close these channels when we're done with them
			defer func() {
				close(semaphoreChan)
			}()

			var ch = make(chan bool)

			timeelapsed := time.Now().UnixNano()
			for i := 0; i < requests; i++ {
				semaphoreChan <- struct{}{}
				go func(i int) {
					err := chaincodeInvokeMulti(cmd, args, nil)
					if err != nil {
						fmt.Printf("error invoking chaincode xdd - %v\n", err)
					}
					<-semaphoreChan
					ch <- true
				}(i)
			}
			counter := 0
		counter_start:
			switch {
			case <-ch:
				fmt.Printf("COUNTER %d\n", counter)
				if counter < requests-1 {
					// if counter%2000 == 0 {
					// 	runtime.GC()
					// }
					counter++
					goto counter_start
				} else {
					timeelapsed = time.Now().UnixNano() - timeelapsed
				}
			}
			// END TEST CODE
			fmt.Printf("2k requests complete. Time taken: %dms\n", timeelapsed/1e6)
			//return chaincodeInvokeMulti(cmd, args, cf)
			return nil
		},
	}
	flagList := []string{
		"name",
		"ctor",
		"channelID",
	}
	attachFlags(chaincodeInvokeMultiCmd, flagList)

	return chaincodeInvokeMultiCmd
}

func chaincodeInvokeMulti(cmd *cobra.Command, args []string, cf *ChaincodeCmdFactory) error {
	if channelID == "" {
		return errors.New("The required parameter 'channelID' is empty. Rerun the command with -C flag")
	}
	var err error
	if cf == nil {
		cf, err = InitCmdFactory(true, true)
		if err != nil {
			return err
		}
	}
	defer cf.BroadcastClient.Close()

	return chaincodeInvokeOrQueryMulti(cmd, args, true, cf)
}
