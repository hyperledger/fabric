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

/* AMCL BN Curve Pairing functions */

package amcl


func Ate(P *ECP2, Q *ECP) *FP12 {
	return ate(P, Q)
}
func Ate2(P *ECP2, Q *ECP, R *ECP2, S *ECP) *FP12 {
	return ate2(P, Q, R, S)
}
func Fexp(m *FP12) *FP12 {
	return fexp(m)
}
