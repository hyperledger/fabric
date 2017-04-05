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

/* Finite Field arithmetic */
/* CLINT mod p functions */

package amcl


func (F *FP) ToString() string {
	return F.toString()
}

func (F *FP) Zero() {
	F.zero()
}

func (F *FP) One() {
	F.one()
}

func (F *FP) Mul(b *FP) {
	F.mul(b)
}

func (F *FP) Neg() {
	F.neg()
}

func (F *FP) Add(b *FP) {
	F.add(b)
}

func (F *FP) Sub(b *FP) {
	F.sub(b)
}

func (F *FP) Inverse() {
	F.inverse()
}

func (F *FP) Equals(a *FP) bool {
	return F.equals(a)
}

func (F *FP) Pow(e *BIG) *FP {
	return F.pow(e)
}

