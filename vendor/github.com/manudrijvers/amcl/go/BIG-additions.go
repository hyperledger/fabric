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

package amcl

func (r *BIG) ToBytes(b []byte) {
	r.tobytearray(b, 0)
}
func (r *BIG) ToString() string {
	return r.toString()
}
func (r *BIG) Dec(x int) {
	r.dec(x)
}
func (r *BIG) Inc(x int) {
	r.inc(x)
}
func (r *BIG) Sub(x *BIG) {
	r.sub(x)
}
func (r *BIG) Mod(m *BIG) {
	r.mod(m)
}
func (r *BIG) Equals(m *BIG) bool {
	// Arrays are not pointers, so we can use ==
	return r.w == m.w
}
func FromBytes(b []byte) *BIG {
	return frombytearray(b, 0)
}
func Randomnum(q *BIG, rng *RAND) *BIG {
	return randomnum(q, rng) 
}
func Modneg(a, m *BIG) *BIG {
	return modneg(a, m)
}
func Modmul(a, b, m *BIG) *BIG {
	return modmul(a, b, m)
}
func Modadd(a, b, m *BIG) *BIG {
	c := a.plus(b)
	c.Mod(m)
	return c
}
func (r *BIG) Invmodp(p *BIG) {
	r.invmodp(p)
}
func Modsub(a, b, m *BIG) *BIG {
	return Modadd(a, Modneg(b, m), m)
}

func (r *BIG) Powmod(e *BIG,m *BIG) *BIG {
	return r.powmod(e, m)
}

