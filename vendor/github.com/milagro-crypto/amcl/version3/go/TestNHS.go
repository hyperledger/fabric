/*
Licensed to the Apache Software Foundation (ASF) under one
or more contributor license agreements.  See the NOTICE file
distributed with this work for additional information
regarding copyright ownership.  The ASF licenses this file
to you under the Apache License, Version 2.0 (the
"License"); you may not use this file except in compliance
with the License.  You may obtain a copy of the License at

  http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing,
software distributed under the License is distributed on an
"AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
KIND, either express or implied.  See the License for the
specific language governing permissions and limitations
under the License.
*/

/* test driver and function exerciser for NewHope Simple Functions */
// See https://eprint.iacr.org/2016/1157 (Alkim, Ducas, Popplemann and Schwabe)


package main

import "fmt"
import "amcl"

func main() {

	srng:=amcl.NewRAND()
	var sraw [100]byte
	for i:=0;i<100;i++ {sraw[i]=byte(i+1)}
	srng.Seed(100,sraw[:])

							crng:=amcl.NewRAND()
							var craw [100]byte
							for i:=0;i<100;i++ {craw[i]=byte(i+2)}
							crng.Seed(100,craw[:])

	var S [1792] byte

				var SB [1824] byte	
	amcl.NHS_SERVER_1(srng,SB[:],S[:])
				var UC [2176] byte
							var KEYB [32] byte
							amcl.NHS_CLIENT(crng,SB[:],UC[:],KEYB[:])
				
							fmt.Printf("Bob's Key= ")
							for i:=0;i<32;i++ {
								fmt.Printf("%02x", KEYB[i])
							}
							fmt.Printf("\n")
	var KEYA [32] byte
	amcl.NHS_SERVER_2(S[:],UC[:],KEYA[:])

	fmt.Printf("Alice Key= ")
	for i:=0;i<32;i++ {
		fmt.Printf("%02x", KEYA[i])
	}
	
} 
