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

/* AMCL Fp^12 functions */
/* FP12 elements are of the form a+i.b+i^2.c */

package amcl

//import "fmt"

func (F *FP12) Geta() *FP4 {
	return F.geta()
}

func (F *FP12) Getb() *FP4 {
	return F.getb()
}

func (F *FP12) Getc() *FP4 {
	return F.getc()
}

func (F *FP12) Copy(x *FP12) {
	F.copy(x)
}

func (F *FP12) One() {
	F.one()
}

func (F *FP12) ToString() string {
	return F.toString()
}

func (F *FP12) Equals(x *FP12) bool {
	return F.equals(x)
}

func (F *FP12) Mul(y *FP12) {
	F.mul(y)
}

func (F *FP12) Inverse() {
	F.inverse()
}
func (F *FP12) Isunity() bool {
	return F.isunity()
}

func (F *FP12) ToBytes(w []byte) {
	F.toBytes(w)
}

func (F *FP12) Pow(e *BIG) *FP12 {
	return F.pow(e)
}

func Pow4(q []*FP12,u []*BIG) *FP12 {
	return pow4(q, u)
}

func (F *FP12) Pinpow(e int,bts int) {
	F.pinpow(e, bts)
}

