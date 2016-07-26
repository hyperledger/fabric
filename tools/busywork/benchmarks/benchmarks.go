/*
Copyright IBM Corp. 2016. All Rights Reserved.

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
	"fmt"
	"os"
	"strconv"
)

// Call as
//
//    crypto <operation> <n>
//
// Where operation is one of
//
//    P256Sign
//    P256Verify
//    P384Sign
//    P384Verify
//    SHA256x8
//    SHA256x1K
//    SHA256x8K
//    SHA512x8
//    SHA512x1K
//    SHA512x8K
//    SHA3_256x8
//    SHA3_256x1K
//    SHA3_256x8K
//    SHA3_512x8
//    SHA3_512x1K
//    SHA3_512x8K
//
// and <n> is the number of iterations
func main() {

	operation := os.Args[1]
	n, err := strconv.Atoi(os.Args[2])
	if err != nil {
		fmt.Printf("Can't parse %s as an integer : %s\n", os.Args[2], err)
		os.Exit(1)
	}

	var rate, thisRate int
	var what string
	times := 1
	if len(os.Args) == 4 {
		times, err = strconv.Atoi(os.Args[3])
		if err != nil {
			fmt.Printf("Can't parse %s as an integer : %s\n", os.Args[2], err)
			os.Exit(1)
		}
	}

	for i := 0; i < times; i++ {

		switch operation {
		case "P256Sign":
			thisRate, what = P256Sign(n)
		case "P256Verify":
			thisRate, what = P256Verify(n)
		case "P384Sign":
			thisRate, what = P384Sign(n)
		case "P384Verify":
			thisRate, what = P384Verify(n)
		case "SHA256x8":
			thisRate, what = SHA256x8(n)
		case "SHA256x1K":
			thisRate, what = SHA256x1K(n)
		case "SHA256x8K":
			thisRate, what = SHA256x8K(n)
		case "SHA512x8":
			thisRate, what = SHA512x8(n)
		case "SHA512x1K":
			thisRate, what = SHA512x1K(n)
		case "SHA512x8K":
			thisRate, what = SHA512x8K(n)
		case "SHA3_256x8":
			thisRate, what = SHA3_256x8(n)
		case "SHA3_256x1K":
			thisRate, what = SHA3_256x1K(n)
		case "SHA3_256x8K":
			thisRate, what = SHA3_256x8K(n)
		case "SHA3_512x8":
			thisRate, what = SHA3_512x8(n)
		case "SHA3_512x1K":
			thisRate, what = SHA3_512x1K(n)
		case "SHA3_512x8K":
			thisRate, what = SHA3_512x8K(n)
		default:
			fmt.Printf("Unrecognized operation : %s\n", operation)
			os.Exit(1)
		}

		rate += thisRate
	}
	rate = rate / times

	fmt.Printf("%-12s : %12s operations X %2d -> %12s %s per second\n", operation, commify(n), times, commify(rate), what)
}

// commify adds American-style comma separators to an integer. We should
// consider adding an open-source Go package that does things like this more
// generally.
func commify(n int) string {
	if n == 0 {
		return "0"
	}
	sDigits := map[int]string{
		0: "0",
		1: "1", -1: "1",
		2: "2", -2: "2",
		3: "3", -3: "3",
		4: "4", -4: "4",
		5: "5", -5: "5",
		6: "6", -6: "6",
		7: "7", -7: "7",
		8: "8", -8: "8",
		9: "9", -9: "9"}
	var sign string
	if n < 0 {
		sign = "-"
	} else {
		sign = ""
	}
	nDigits := 0
	s := ""
	for n != 0 {
		if nDigits == 3 {
			s = "," + s
			nDigits = 0
		}
		nDigits++
		s = sDigits[n%10] + s
		n = n / 10
	}
	return sign + s
}
