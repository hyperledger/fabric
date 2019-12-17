/*
   Copyright (C) 2019 MIRACL UK Ltd.

    This program is free software: you can redistribute it and/or modify
    it under the terms of the GNU Affero General Public License as
    published by the Free Software Foundation, either version 3 of the
    License, or (at your option) any later version.


    This program is distributed in the hope that it will be useful,
    but WITHOUT ANY WARRANTY; without even the implied warranty of
    MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
    GNU Affero General Public License for more details.

     https://www.gnu.org/licenses/agpl-3.0.en.html

    You should have received a copy of the GNU Affero General Public License
    along with this program.  If not, see <https://www.gnu.org/licenses/>.

   Unless required by applicable law or agreed to in writing, software
   distributed under the License is distributed on an "AS IS" BASIS,
   WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
   See the License for the specific language governing permissions and
   limitations under the License.

   You can be released from the requirements of the license by purchasing
   a commercial license. Buying such a license is mandatory as soon as you
   develop commercial activities involving the MIRACL Core Crypto SDK
   without disclosing the source code of your own applications, or shipping
   the MIRACL Core Crypto SDK with a closed source product.
*/

/* Finite Field arithmetic  Fp^2 functions */

/* FP2 elements are of the form a+ib, where i is sqrt(-1) */

package FP256BN



type FP2 struct {
	a *FP
	b *FP
}

func NewFP2() *FP2 {
	F := new(FP2)
	F.a = NewFP()
	F.b = NewFP()
	return F
}

/* Constructors */
func NewFP2int(a int) *FP2 {
	F := new(FP2)
	F.a = NewFPint(a)
	F.b = NewFP()
	return F
}

func NewFP2ints(a int, b int) *FP2 {
	F := new(FP2)
	F.a = NewFPint(a)
	F.b = NewFPint(b)
	return F
}

func NewFP2copy(x *FP2) *FP2 {
	F := new(FP2)
	F.a = NewFPcopy(x.a)
	F.b = NewFPcopy(x.b)
	return F
}

func NewFP2fps(c *FP, d *FP) *FP2 {
	F := new(FP2)
	F.a = NewFPcopy(c)
	F.b = NewFPcopy(d)
	return F
}

func NewFP2bigs(c *BIG, d *BIG) *FP2 {
	F := new(FP2)
	F.a = NewFPbig(c)
	F.b = NewFPbig(d)
	return F
}

func NewFP2fp(c *FP) *FP2 {
	F := new(FP2)
	F.a = NewFPcopy(c)
	F.b = NewFP()
	return F
}

func NewFP2big(c *BIG) *FP2 {
	F := new(FP2)
	F.a = NewFPbig(c)
	F.b = NewFP()
	return F
}

/* reduce components mod Modulus */
func (F *FP2) reduce() {
	F.a.reduce()
	F.b.reduce()
}

/* normalise components of w */
func (F *FP2) norm() {
	F.a.norm()
	F.b.norm()
}

/* test this=0 ? */
func (F *FP2) iszilch() bool {
	return (F.a.iszilch() && F.b.iszilch())
}

func (F *FP2) cmove(g *FP2, d int) {
	F.a.cmove(g.a, d)
	F.b.cmove(g.b, d)
}

/* test this=1 ? */
func (F *FP2) isunity() bool {
	one := NewFPint(1)
	return (F.a.Equals(one) && F.b.iszilch())
}

/* test this=x */
func (F *FP2) Equals(x *FP2) bool {
	return (F.a.Equals(x.a) && F.b.Equals(x.b))
}

/* extract a */
func (F *FP2) GetA() *BIG {
	return F.a.redc()
}

/* extract b */
func (F *FP2) GetB() *BIG {
	return F.b.redc()
}

/* copy this=x */
func (F *FP2) copy(x *FP2) {
	F.a.copy(x.a)
	F.b.copy(x.b)
}

/* set this=0 */
func (F *FP2) zero() {
	F.a.zero()
	F.b.zero()
}

/* set this=1 */
func (F *FP2) one() {
	F.a.one()
	F.b.zero()
}
/* Return sign */
func (F *FP2) sign() int {
	m := F.a.redc()
	return m.parity() 
}

/* negate this mod Modulus */
func (F *FP2) neg() {
	m := NewFPcopy(F.a)
	t := NewFP()

	m.add(F.b)
	m.neg()
	t.copy(m)
	t.add(F.b)
	F.b.copy(m)
	F.b.add(F.a)
	F.a.copy(t)
}

/* set to a-ib */
func (F *FP2) conj() {
	F.b.neg()
	F.b.norm()
}

/* this+=a */
func (F *FP2) add(x *FP2) {
	F.a.add(x.a)
	F.b.add(x.b)
}

/* this-=a */
func (F *FP2) sub(x *FP2) {
	m := NewFP2copy(x)
	m.neg()
	F.add(m)
}

/* this-=a */
func (F *FP2) rsub(x *FP2) {
	F.neg()
	F.add(x)
}

/* this*=s, where s is an FP */
func (F *FP2) pmul(s *FP) {
	F.a.mul(s)
	F.b.mul(s)
}

/* this*=i, where i is an int */
func (F *FP2) imul(c int) {
	F.a.imul(c)
	F.b.imul(c)
}

/* this*=this */
func (F *FP2) sqr() {
	w1 := NewFPcopy(F.a)
	w3 := NewFPcopy(F.a)
	mb := NewFPcopy(F.b)
	w1.add(F.b)

	w3.add(F.a)
	w3.norm()
	F.b.mul(w3)

	mb.neg()
	F.a.add(mb)

	w1.norm()
	F.a.norm()

	F.a.mul(w1)
}

/* this*=y */
/* Now using Lazy reduction */
func (F *FP2) mul(y *FP2) {

	if int64(F.a.XES+F.b.XES)*int64(y.a.XES+y.b.XES) > int64(FEXCESS) {
		if F.a.XES > 1 {
			F.a.reduce()
		}
		if F.b.XES > 1 {
			F.b.reduce()
		}
	}

	pR := NewDBIG()
	C := NewBIGcopy(F.a.x)
	D := NewBIGcopy(y.a.x)
	p := NewBIGints(Modulus)

	pR.ucopy(p)

	A := mul(F.a.x, y.a.x)
	B := mul(F.b.x, y.b.x)

	C.add(F.b.x)
	C.norm()
	D.add(y.b.x)
	D.norm()

	E := mul(C, D)
	FF := NewDBIGcopy(A)
	FF.add(B)
	B.rsub(pR)

	A.add(B)
	A.norm()
	E.sub(FF)
	E.norm()

	F.a.x.copy(mod(A))
	F.a.XES = 3
	F.b.x.copy(mod(E))
	F.b.XES = 2

}
/*
func (F *FP2) pow(b *BIG)  {
	w := NewFP2copy(F);
	r := NewFP2int(1)
	z := NewBIGcopy(b)
	for true {
		bt := z.parity()
		z.shr(1)
		if bt==1 {
			r.mul(w)
		}
		if z.iszilch() {break}
		w.sqr()
	}
	r.reduce()
	F.copy(r)
}
*/
func (F *FP2) qr() int {
	c := NewFP2copy(F)
	c.conj()
	c.mul(F)
	return c.a.qr(nil)
}

/* sqrt(a+ib) = sqrt(a+sqrt(a*a-n*b*b)/2)+ib/(2*sqrt(a+sqrt(a*a-n*b*b)/2)) */
func (F *FP2) sqrt() {
	if F.iszilch() {
		return 
	}
	w1 := NewFPcopy(F.b)
	w2 := NewFPcopy(F.a)
	w3 := NewFP();
	w1.sqr()
	w2.sqr()
	w1.add(w2); w1.norm()

	w1 = w1.sqrt(nil)
	w2.copy(F.a)
	w3.copy(F.a)

	w2.add(w1)
	w2.norm()
	w2.div2()

	w3.sub(w1)
	w3.norm()
	w3.div2()
	
	w2.cmove(w3,w3.qr(nil))

	w2 = w2.sqrt(nil)
	F.a.copy(w2)
	w2.add(w2); w2.norm()
	w2.inverse()
	F.b.mul(w2)
}

/* output to hex string */
func (F *FP2) ToString() string {
	return ("[" + F.a.toString() + "," + F.b.toString() + "]")
}

/* output to hex string */
func (F *FP2) toString() string {
	return ("[" + F.a.toString() + "," + F.b.toString() + "]")
}

/* this=1/this */
func (F *FP2) inverse() {
	F.norm()
	w1 := NewFPcopy(F.a)
	w2 := NewFPcopy(F.b)

	w1.sqr()
	w2.sqr()
	w1.add(w2)
	w1.inverse()
	F.a.mul(w1)
	w1.neg()
	w1.norm()
	F.b.mul(w1)
}

/* this/=2 */
func (F *FP2) div2() {
	F.a.div2()
	F.b.div2()
}

/* this*=sqrt(-1) */
func (F *FP2) times_i() {
	z := NewFPcopy(F.a)
	F.a.copy(F.b)
	F.a.neg()
	F.b.copy(z)
}

/* w*=(1+sqrt(-1)) */
/* where X*2-(2^i+sqrt(-1)) is irreducible for FP4 */
func (F *FP2) mul_ip() {
	t := NewFP2copy(F)
	i := QNRI
	F.times_i()
	for i > 0 {
		t.add(t)
		t.norm()
		i--
	}
	F.add(t)

	if TOWER == POSITOWER {
		F.norm()
		F.neg()
	}

}

/* w/=(2^i+sqrt(-1)) */
func (F *FP2) div_ip() {
	z := NewFP2ints(1<<uint(QNRI), 1)
	z.inverse()
	F.norm()
	F.mul(z)
	if TOWER == POSITOWER {
		F.neg()
		F.norm()
	}
}
