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

/* Finite Field arithmetic  Fp^2 functions */

/* FP2 elements are of the form a+ib, where i is sqrt(-1) */

package amcl

func (F *FP2)  Isunity() bool {
	return F.isunity()
}

func (F *FP2) Equals(x *FP2) bool {
	return F.equals(x)
}

func (F *FP2) GetA() *BIG {
	return F.getA()
}

func (F *FP2) GetB() *BIG {
	return F.getB()
}

func (F *FP2) Copy(x *FP2) {
	F.copy(x)
}

func (F *FP2) Zero() {
	F.zero()
}

func (F *FP2) One() {
	F.one()
}

func (F *FP2) Neg() {
	F.neg()
}

func (F *FP2) Add(x *FP2) {
	F.add(x)
}

func (F *FP2) Sub(x *FP2) {
	F.sub(x)
}

func (F *FP2) Mul(y *FP2) {
	F.mul(y)
}

func (F *FP2) ToString() string {
	return F.toString()
}

func (F *FP2) Inverse() {
	F.inverse()
}


