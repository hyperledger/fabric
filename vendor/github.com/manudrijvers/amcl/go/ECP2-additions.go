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

/* AMCL Weierstrass elliptic curve functions over FP2 */

package amcl

func (E *ECP2) Is_infinity() bool {
	return E.is_infinity()
}
func (E *ECP2) Copy(P *ECP2) {
	E.copy(P)
}
func (E *ECP2) Neg() {
	E.neg()
}
func (E *ECP2) Equals(Q *ECP2) bool {
	return E.equals(Q)
}
func (E *ECP2) GetX() *FP2 {
	return E.getX()
}
func (E *ECP2) GetY() *FP2 {
	return E.getY()
}
func (E *ECP2) ToBytes(b []byte) {
	E.toBytes(b)
}
func (E *ECP2) ToString() string {
	return E.toString()
}
func (E *ECP2) Add(Q *ECP2) int {
	return E.add(Q)
}
func (E *ECP2) Sub(Q *ECP2) int {
	return E.sub(Q)
}
func (E *ECP2) Mul(e *BIG) *ECP2 {
	return E.mul(e)
}


