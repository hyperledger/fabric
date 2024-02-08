/* 
 * Copyright (c) 2012-2020 MIRACL UK Ltd.
 *
 * This file is part of MIRACL Core
 * (see https://github.com/miracl/core).
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.

	Arwa Alblooshi 15/12/2022
 */
package core



//q = 8380417
const DL_LGN = 8
const DL_DEGREE = (1 << DL_LGN)
const DL_PRIME = 0x7fe001
const DL_D = 13
const DL_TD = (23 - DL_D)
 
const DL_ONE = 0x3FFE00    // R mod Q
const DL_COMBO = 0xA3FA    // ONE*inv mod Q
const DL_R2MODP = 0x2419FF // R^2 mod Q
const DL_ND = 0xFC7FDFFF   // 1/(R-Q) mod R
 
const DL_MAXLG = 19
const DL_MAXK = 8 // could reduce these if not using highest security
const DL_MAXL = 7
const DL_YBYTES = (((DL_MAXLG + 1) * DL_DEGREE) / 8)

const DL_SK_SIZE_2 = (32*3+DL_DEGREE* (4*13+4*3+4*3)/8)
const DL_PK_SIZE_2 = ((4*DL_DEGREE*DL_TD)/8 + 32)
const DL_SIG_SIZE_2 = ((DL_DEGREE*4*(17+1))/8 + 80 + 4 + 32)

const DL_SK_SIZE_3 = (32*3 + DL_DEGREE*(6*13+5*4+6*4)/8)
const DL_PK_SIZE_3 = ((6*DL_DEGREE*DL_TD)/8 + 32)
const DL_SIG_SIZE_3 = ((DL_DEGREE*5*(19+1))/8 + 55 + 6 + 32)

const DL_SK_SIZE_5 = (32*3 + DL_DEGREE*(8*13+7*3+8*3)/8)
const DL_PK_SIZE_5 = ((8*DL_DEGREE*DL_TD)/8 + 32)
const DL_SIG_SIZE_5 = ((DL_DEGREE*7*(19+1))/8 + 75 + 8 + 32)
 
// parameters for each security level
// tau,gamma1,gamma2,K,L,eta,lg(2*eta+1),omega

var DL_PARAMS_2 = []int{39, 17, 88, 4, 4, 2, 3, 80}
var DL_PARAMS_3 = []int{49, 19, 32, 6, 5, 4, 4, 55}
var DL_PARAMS_5 = []int{60, 19, 32, 8, 7, 2, 3, 75}
 
var DL_roots = []int32{0x3ffe00, 0x64f7, 0x581103, 0x77f504, 0x39e44, 0x740119, 0x728129, 0x71e24, 0x1bde2b, 0x23e92b, 0x7a64ae, 0x5ff480, 0x2f9a75, 0x53db0a, 0x2f7a49, 0x28e527, 0x299658, 0xfa070, 0x6f65a5, 0x36b788, 0x777d91, 0x6ecaa1, 0x27f968, 0x5fb37c, 0x5f8dd7, 0x44fae8, 0x6a84f8, 0x4ddc99, 0x1ad035, 0x7f9423, 0x3d3201, 0x445c5, 0x294a67, 0x17620, 0x2ef4cd, 0x35dec5, 0x668504, 0x49102d, 0x5927d5, 0x3bbeaf, 0x44f586, 0x516e7d, 0x368a96, 0x541e42, 0x360400, 0x7b4a4e, 0x23d69c, 0x77a55e, 0x65f23e, 0x66cad7, 0x357e1e, 0x458f5a, 0x35843f, 0x5f3618, 0x67745d, 0x38738c, 0xc63a8, 0x81b9a, 0xe8f76, 0x3b3853, 0x3b8534, 0x58dc31, 0x1f9d54, 0x552f2e, 0x43e6e6, 0x688c82, 0x47c1d0, 0x51781a, 0x69b65e, 0x3509ee, 0x2135c7, 0x67afbc, 0x6caf76, 0x1d9772, 0x419073, 0x709cf7, 0x4f3281, 0x4fb2af, 0x4870e1, 0x1efca, 0x3410f2, 0x70de86, 0x20c638, 0x296e9f, 0x5297a4, 0x47844c, 0x799a6e, 0x5a140a, 0x75a283, 0x6d2114, 0x7f863c, 0x6be9f8, 0x7a0bde, 0x1495d4, 0x1c4563, 0x6a0c63, 0x4cdbea, 0x40af0, 0x7c417, 0x2f4588, 0xad00, 0x6f16bf, 0xdcd44, 0x3c675a, 0x470bcb, 0x7fbe7f, 0x193948, 0x4e49c1, 0x24756c, 0x7ca7e0, 0xb98a1, 0x6bc809, 0x2e46c, 0x49a809, 0x3036c2, 0x639ff7, 0x5b1c94, 0x7d2ae1, 0x141305, 0x147792, 0x139e25, 0x67b0e1, 0x737945, 0x69e803, 0x51cea3, 0x44a79d, 0x488058, 0x3a97d9, 0x1fea93, 0x33ff5a, 0x2358d4, 0x3a41f8, 0x4cdf73, 0x223dfb, 0x5a8ba0, 0x498423, 0x412f5, 0x252587, 0x6d04f1, 0x359b5d, 0x4a28a1, 0x4682fd, 0x6d9b57, 0x4f25df, 0xdbe5e, 0x1c5e1a, 0xde0e6, 0xc7f5a, 0x78f83, 0x67428b, 0x7f3705, 0x77e6fd, 0x75e022, 0x503af7, 0x1f0084, 0x30ef86, 0x49997e, 0x77dcd7, 0x742593, 0x4901c3, 0x53919, 0x4610c, 0x5aad42, 0x3eb01b, 0x3472e7, 0x4ce03c, 0x1a7cc7, 0x31924, 0x2b5ee5, 0x291199, 0x585a3b, 0x134d71, 0x3de11c, 0x130984, 0x25f051, 0x185a46, 0x466519, 0x1314be, 0x283891, 0x49bb91, 0x52308a, 0x1c853f, 0x1d0b4b, 0x6fd6a7, 0x6b88bf, 0x12e11b, 0x4d3e3f, 0x6a0d30, 0x78fde5, 0x1406c7, 0x327283, 0x61ed6f, 0x6c5954, 0x1d4099, 0x590579, 0x6ae5ae, 0x16e405, 0xbdbe7, 0x221de8, 0x33f8cf, 0x779935, 0x54aa0d, 0x665ff9, 0x63b158, 0x58711c, 0x470c13, 0x910d8, 0x463e20, 0x612659, 0x251d8b, 0x2573b7, 0x7d5c90, 0x1ddd98, 0x336898, 0x2d4bb, 0x6d73a8, 0x4f4cbf, 0x27c1c, 0x18aa08, 0x2dfd71, 0xc5ca5, 0x19379a, 0x478168, 0x646c3e, 0x51813d, 0x35c539, 0x3b0115, 0x41dc0, 0x21c4f7, 0x70fbf5, 0x1a35e7, 0x7340e, 0x795d46, 0x1a4cd0, 0x645caf, 0x1d2668, 0x666e99, 0x6f0634, 0x7be5db, 0x455fdc, 0x530765, 0x5dc1b0, 0x7973de, 0x5cfd0a, 0x2cc93, 0x70f806, 0x189c2a, 0x49c5aa, 0x776a51, 0x3bcf2c, 0x7f234f, 0x6b16e0, 0x3c15ca, 0x155e68, 0x72f6b7, 0x1e29ce}
var DL_iroots = []int32{0x3ffe00, 0x7f7b0a, 0x7eafd, 0x27cefe, 0x78c1dd, 0xd5ed8, 0xbdee8, 0x7c41bd, 0x56fada, 0x5065b8, 0x2c04f7, 0x50458c, 0x1feb81, 0x57b53, 0x5bf6d6, 0x6401d6, 0x7b9a3c, 0x42ae00, 0x4bde, 0x650fcc, 0x320368, 0x155b09, 0x3ae519, 0x20522a, 0x202c85, 0x57e699, 0x111560, 0x86270, 0x492879, 0x107a5c, 0x703f91, 0x5649a9, 0x2ab0d3, 0x6042ad, 0x2703d0, 0x445acd, 0x44a7ae, 0x71508b, 0x77c467, 0x737c59, 0x476c75, 0x186ba4, 0x20a9e9, 0x4a5bc2, 0x3a50a7, 0x4a61e3, 0x19152a, 0x19edc3, 0x83aa3, 0x5c0965, 0x495b3, 0x49dc01, 0x2bc1bf, 0x49556b, 0x2e7184, 0x3aea7b, 0x442152, 0x26b82c, 0x36cfd4, 0x195afd, 0x4a013c, 0x50eb34, 0x7e69e1, 0x56959a, 0x454828, 0x375fa9, 0x3b3864, 0x2e115e, 0x15f7fe, 0xc66bc, 0x182f20, 0x6c41dc, 0x6b686f, 0x6bccfc, 0x2b520, 0x24c36d, 0x1c400a, 0x4fa93f, 0x3637f8, 0x7cfb95, 0x1417f8, 0x744760, 0x33821, 0x5b6a95, 0x319640, 0x66a6b9, 0x2182, 0x38d436, 0x4378a7, 0x7212bd, 0x10c942, 0x7f3301, 0x509a79, 0x781bea, 0x7bd511, 0x330417, 0x15d39e, 0x639a9e, 0x6b4a2d, 0x5d423, 0x13f609, 0x59c5, 0x12beed, 0xa3d7e, 0x25cbf7, 0x64593, 0x385bb5, 0x2d485d, 0x567162, 0x5f19c9, 0xf017b, 0x4bcf0f, 0x7df037, 0x376f20, 0x302d52, 0x30ad80, 0xf430a, 0x3e4f8e, 0x62488f, 0x13308b, 0x183045, 0x5eaa3a, 0x4ad613, 0x1629a3, 0x2e67e7, 0x381e31, 0x17537f, 0x3bf91b, 0x61b633, 0xce94a, 0x6a8199, 0x43ca37, 0x14c921, 0xbcb2, 0x4410d5, 0x875b0, 0x361a57, 0x6743d7, 0xee7fb, 0x7d136e, 0x22e2f7, 0x66c23, 0x221e51, 0x2cd89c, 0x3a8025, 0x3fa26, 0x10d9cd, 0x197168, 0x62b999, 0x1b8352, 0x659331, 0x682bb, 0x78abf3, 0x65aa1a, 0xee40c, 0x5e1b0a, 0x7bc241, 0x44deec, 0x4a1ac8, 0x2e5ec4, 0x1b73c3, 0x385e99, 0x66a867, 0x73835c, 0x51e290, 0x6735f9, 0x7d63e5, 0x309342, 0x126c59, 0x7d0b46, 0x4c7769, 0x620269, 0x28371, 0x5a6c4a, 0x5ac276, 0x1eb9a8, 0x39a1e1, 0x76cf29, 0x38d3ee, 0x276ee5, 0x1c2ea9, 0x198008, 0x2b35f4, 0x846cc, 0x4be732, 0x5dc219, 0x74041a, 0x68fbfc, 0x14fa53, 0x26da88, 0x629f68, 0x1386ad, 0x1df292, 0x4d6d7e, 0x6bd93a, 0x6e21c, 0x15d2d1, 0x32a1c2, 0x6cfee6, 0x145742, 0x10095a, 0x62d4b6, 0x635ac2, 0x2daf77, 0x362470, 0x57a770, 0x6ccb43, 0x397ae8, 0x6785bb, 0x59efb0, 0x6cd67d, 0x41fee5, 0x6c9290, 0x2785c6, 0x56ce68, 0x54811c, 0x7cc6dd, 0x65633a, 0x32ffc5, 0x4b6d1a, 0x412fe6, 0x2532bf, 0x7b7ef5, 0x7aa6e8, 0x36de3e, 0xbba6e, 0x8032a, 0x364683, 0x4ef07b, 0x60df7d, 0x2fa50a, 0x9ffdf, 0x7f904, 0xa8fc, 0x189d76, 0x78507e, 0x7360a7, 0x71ff1b, 0x6381e7, 0x7221a3, 0x30ba22, 0x1244aa, 0x395d04, 0x35b760, 0x4a44a4, 0x12db10, 0x5aba7a, 0x7bcd0c, 0x365bde, 0x255461, 0x5da206, 0x33008e, 0x459e09, 0x5c872d, 0x4be0a7, 0x5ff56e}
 
func DL_round(a int32, b int32) int32 {
	return (a + b/2) / b
}

// constant time absolute vaue
func DL_nabs(x int32) int32 {
	mask := (x >> 31)
	return (x + mask)^mask
}

// Montgomery stuff
func DL_redc(T uint64) int32 {
	m := uint32(T) * (uint32(DL_ND))
	return int32((uint64(m)*DL_PRIME + T) >> 32)
}
 
func DL_nres(x uint32) int32 {
	return DL_redc(uint64(x)*DL_R2MODP)
}
 
func DL_modmul(a uint32, b uint32) int32 {
	return DL_redc(uint64(a)*uint64(b))
}
 
//make all elements +ve
func DL_poly_pos(p []int32) {
	for i := 0; i < DL_DEGREE; i++ {
		p[i] += (p[i] >> 31)&DL_PRIME
	}
}
 
// NTT code
 
// Important!
// DL_nres(x); DL_ntt(x)
// DL_nres(y); DL_ntt(y)
// z=x*y
// DL_intt(z);
// DL_redc(z);
 
// is equivalent to (note that DL_nres() and DL_redc() cancel out)
 
// DL_ntt(x);
// DL_nres(y); DL_ntt(y);
// z=x*y
// DL_intt(z)
 
// is equivalent to
 
// DL_ntt(x)
// DL_ntt(y)
// z=x*y
// DL_intt(z)
// DL_nres(z)
 
// In all cases z ends up in normal (non-Montgomery) form!
// So the conversion to Montgomery form can be "pushed" through the calculation.
 
// Here DL_intt(z) <- DL_intt(z);DL_nres(z);
// Combining is more efficient
// note that DL_ntt() and DL_intt() are not mutually inverse
 
/* NTT code */
/* Cooley-Tukey NTT */
/* Excess of 2 allowed on input - coefficients must be < 2*PRIME */
 
func DL_ntt(x []int32) {

	var start int
	len := DL_DEGREE/2

	var S, V int32
	var q int32 = DL_PRIME

	//Make positive
	DL_poly_pos(x)
	m := 1
	for m < DL_DEGREE {
		start = 0
		for i := 0; i < m; i++ {
			S = DL_roots[m+i]
			for j := start; j < start+len; j++ {
				V = DL_modmul(uint32(x[j+len]), uint32(S))
				x[j+len] = x[j] + 2 * q - V
				x[j] = x[j] + V
			}
			start += 2 * len
		}
		len /= 2
		m *= 2
	}
}
 
// Gentleman-Sande INTT
// Excess of 2 allowed on input - coefficients must be < 2*PRIME
// Output fully reduced
const NTTL = 1
 
func DL_intt(x []int32) {
	var k,lim int
	t := 1
	var S,U,V,W int32
	var q int32 = DL_PRIME
	m := DL_DEGREE/2
	n := DL_LGN
	for m >= 1 {
		lim = NTTL >> n
		n--
		k = 0
		for i := 0; i < m; i++ {
			S = DL_iroots[m+i]
			for j := k; j < k+t; j++ {
				if NTTL > 1 {
					if m < NTTL && j < k+lim {
						U = DL_modmul(uint32(x[j]), DL_ONE)
						V = DL_modmul(uint32(x[j+t]), DL_ONE)
					} else {
						U = x[j]
						V = x[j+t]
					}
				} else {
					U = x[j]
					V = x[j+t]
				}
				x[j] = U + V
				W = U + (DL_DEGREE/NTTL)*q - V
				x[j+t] = DL_modmul(uint32(W), uint32(S))
				//T := int64(W)*int64(S)
				//var M int64 = (T*DL_ND) & 0xffffffff
				//R := int((M * int64(DL_PRIME+T)) >> 32)
				//fmt.Printf("x,W,S= %d %d %d %x %x %d\n",x[j+t],W,S,T,M,R)
			}
			k += 2 * t
		}
		t *= 2
		m /= 2
	}
	//fmt.Printf("\nx[0]= %d\n",x[0])
	//fmt.Printf("x[1]= %d\n",x[1])
	for j := 0; j < DL_DEGREE; j++ {
		//fully reduce, DL_nres combined with 1/DEGREE
		x[j] = DL_modmul(uint32(x[j]), DL_COMBO)
		x[j] -= q
		x[j] += (x[j] >> 31)&q
	}
}
 
func DL_nres_it(p []int32) {
	for i := 0; i < DL_DEGREE; i++ {
		p[i] = DL_nres(uint32(p[i]))
	}
}
 
func DL_redc_it(p []int32) {
	for i := 0; i < DL_DEGREE; i++ {
		p[i] = DL_redc(uint64(p[i]))
	}
}
 
//copy polynomial
func DL_poly_copy(p1 []int32, p2 []int32) {
	for i := 0; i < DL_DEGREE; i++ {
		p1[i] = p2[i]
	}
}
 
//copy from small polynomial
func DL_poly_scopy(p1 []int32, p2 []int8) {
	for i := 0; i < DL_DEGREE; i++ {
		p1[i] = int32(p2[i])
	}
}
 
//copy from medium polynomial
func DL_poly_mcopy(p1 []int32, p2 []int16) {
	for i := 0; i < DL_DEGREE; i++ {
		p1[i] = int32(p2[i])
	}
}
 
func DL_poly_zero(p1 []int32) {
	for i := 0; i < DL_DEGREE; i++ {
		p1[i] = 0
	}
}
 
func DL_poly_negate(p1 []int32, p2 []int32) {
	for i := 0; i < DL_DEGREE; i++ {
		p1[i] = DL_PRIME - p2[i]
	}
}
 
func DL_poly_mul(p1 []int32, p2 []int32, p3 []int32) {
	for i := 0; i < DL_DEGREE; i++ {
		p1[i] = DL_modmul(uint32(p2[i]), uint32(p3[i]))
	}
}
 
func DL_poly_add(p1 []int32, p2 []int32, p3 []int32) {
	for i := 0; i < DL_DEGREE; i++ { 
		p1[i] = p2[i] + p3[i]
	}
}
 
func DL_poly_sub(p1 []int32, p2 []int32, p3 []int32) {
	for i := 0; i < DL_DEGREE; i++ {
		p1[i] = p2[i] + DL_PRIME - p3[i]
	}
}
 
//reduce inputs that are already < 2q
func DL_poly_soft_reduce(poly []int32) {
	var e int32
	for i := 0; i < DL_DEGREE; i++ {
		e = poly[i] - DL_PRIME
		poly[i] = e + ((e >> 31)&DL_PRIME)
		
	}
}

//fully reduces modulo q
func DL_poly_hard_reduce(poly []int32) {
	var e int32
	for i := 0; i < DL_DEGREE; i++ {
		e = DL_modmul(uint32(poly[i]), DL_ONE) //reduces to < 2q
		e = e - DL_PRIME
		poly[i] = e + ((e >> 31)&DL_PRIME) //finishes it off
	}
}
 
//Generate A[i][j] from rho
func DL_ExpandAij(rho []byte, Aij []int32, i int, j int) {
	sh := NewSHA3(SHA3_SHAKE128)
	var cf int32
	//var b0, b1, b2 uint32
	var buff [4*DL_DEGREE]byte // should be plenty
 
	for m := 0; m < 32; m++ {
		sh.Process(rho[m])
	}
	sh.Process(byte(j & 0xFF))
	sh.Process(byte(i & 0xff))
	sh.Shake(buff[:], 4*DL_DEGREE)
	m := 0
	n := 0
	for m < DL_DEGREE {
		b0 := uint32(buff[n]&0xff)
		n++
		b1 := uint32(buff[n]&0xff)
		n++
		b2 := uint32(buff[n]&0xff)
		n++
		cf = int32(((b2 & 0x7f) << 16) + (b1 << 8) + b0)
		if cf >= DL_PRIME {
			continue
		}
		Aij[m] = cf
		m++
	}
}
 
// array t has ab active bits per word
// extract bytes from array of words
// if max!=0 then -max<=t[i]<=+max
func DL_nextbyte32(ab int, max int, t []int32, position []int) byte {
	ptr := position[0] // index in array
	bts := position[1] // bit index in word
	left := ab - bts   // number of bits left in this word
	i := 0
	k := ptr % 256
	var r int32
	var w int32 = t[k]

	if max != 0 {
		w = int32(max) - w
	}
	r = w >> bts
	for left < 8 {
		i++
		w = t[k+i]
		if max != 0 {
			w = int32(max) - w
		}
		r |= w << left
		left += ab
	}
	bts += 8
	for bts >= ab {
		bts -= ab
		ptr++
	}
	position[0] = ptr
	position[1] = bts
 
	return byte(r & 0xff)
}

// array t has ab active bits per word
// extract dense bytes from array of words
// if max!=0 then -max<=t[i]<=+max
func DL_nextbyte16(ab int, max int, t []int16, position []int) byte {
	ptr := position[0] // index in array
	bts := position[1] // bit index in word
	left := ab - bts   // number of bits left in this word
	i := 0
	k := ptr % 256
	var r int32
	var w int32 = int32(t[k])
	if max != 0 {
		w = int32(max) - w
	}
	r = w >> bts
	for left < 8 {
		i++
		w = int32(t[k+i])
		if max != 0 {
			w = int32(max) - w
		}
		r |= w << left
		left += ab
	}
	bts += 8
	for bts >= ab {
		bts -= ab
		ptr++
	}
	position[0] = ptr
	position[1] = bts
 
	return byte(r & 0xff)
}
 
// array t has ab active bits per word
// extract dense bytes from array of words
// if max!=0 then -max<=t[i]<=+max
func DL_nextbyte8(ab int, max int, t []int8, position []int) byte {
	ptr := position[0] // index in array
	bts := position[1] // bit index in word
	left := ab - bts   // number of bits left in this word
	i := 0
	k := ptr % 256
	var r int32
	var w int32 = int32(t[k])
	if max != 0 {
		w = int32(max) - w
	}
	r = w >> bts
	for left < 8 {
		i++
		w = int32(t[k+i])
		if max != 0 {
			w = int32(max) - w
		}
		r |= w << left
		left += ab
	}
	bts += 8
	for bts >= ab {
		bts -= ab
		ptr++
	}
	position[0] = ptr
	position[1] = bts
 
	return byte(r&0xff)
}
 
// extract ab bits into word from dense byte stream
func DL_nextword(ab int, max int, t []byte, position []int) int32 {
	ptr := position[0] // index in array
	bts := position[1] // bit index in word
	r := int32(t[ptr]>>bts)
	mask := int32((1<<ab)-1)
	var w int32
	i := 0
	gotbits := 8 - bts // bits left in current byte
	for gotbits < ab {
		i++
		w = int32(t[ptr+i])
		r |= w<<gotbits
		gotbits += 8
	}
	bts += ab
	for bts >= 8 {
		bts -= 8
		ptr++
	}
	w = r&mask
	if max != 0 {
		w = int32(max) - w
	}
	position[0] = ptr
	position[1] = bts
	return w
}
 
//pack public key
func DL_pack_pk(params []int, pk []byte, rho []byte, t1 [][DL_DEGREE]int16) int {
	var pos [2]int
	pos[0] = 0; pos[1] = 0
	ck := params[3]
	for i := 0; i < 32; i++ {
		pk[i] = rho[i]
	}
	n := 32
	for j := 0; j < ck; j++ {
		for i := 0; i < (DL_DEGREE*DL_TD)/8; i++ {
			pk[n] = DL_nextbyte16(DL_TD, 0, t1[j][:], pos[:])
			n++
		}
	}
	return n
}
 
//unpack public key
func DL_unpack_pk(params []int, rho []byte, t1 [][DL_DEGREE]int16, pk []byte) {
	var pos [2]int
	pos[0] = 32; pos[1] = 0
	ck := params[3]
	for i := 0; i < 32; i++ {
		rho[i] = pk[i]
	}
	for j := 0; j < ck; j++ {
		for i := 0; i < DL_DEGREE; i++ {
			t1[j][i] = int16(DL_nextword(DL_TD, 0, pk, pos[:]))
		}
	}
}
 
// secret key of size 32*3+DEGREE*(K*D+L*LG2ETA1+K*LG2ETA1)/8
func DL_pack_sk(params []int, sk []byte, rho []byte, bK []byte, tr []byte, s1 [][DL_DEGREE]int8, s2 [][DL_DEGREE]int8, t0 [][DL_DEGREE]int16) int {
	n := 32
	ck := params[3]
	el := params[4]
	eta := params[5]
	lg2eta1 := params[6]
 
	for i := 0; i < 32; i++ {
		sk[i] = rho[i]
	}
	for i := 0; i < 32; i++ {
		sk[n] = bK[i]
		n++
	}
	for i := 0; i < 32; i++ {
		sk[n] = tr[i]
		n++
	}
 
	var pos [2]int
	pos[0] = 0; pos[1] = 0
 
	for j := 0; j < el; j++ {
		for i := 0; i < (DL_DEGREE*lg2eta1)/8; i++ {
			sk[n] = DL_nextbyte8(lg2eta1, eta, s1[j][:], pos[:])
			n++
		}
	}
	pos[0] = 0; pos[1] = 0
	for j := 0; j < ck; j++ {
		for i := 0; i < (DL_DEGREE*lg2eta1)/8; i++ {
			sk[n] = DL_nextbyte8(lg2eta1, eta, s2[j][:], pos[:])
			n++
		}
	}
	pos[0] = 0; pos[1] = 0
	for j := 0; j < ck; j++ {
		for i := 0; i < (DL_DEGREE*DL_D)/8; i++ {
			sk[n] = DL_nextbyte16(DL_D, (1 << (DL_D - 1)), t0[j][:], pos[:])
			n++
		}
	}
	return n
}
 
func DL_unpack_sk(params []int, rho []byte, bK []byte, tr []byte, s1 [][DL_DEGREE]int8, s2 [][DL_DEGREE]int8, t0 [][DL_DEGREE]int16, sk []byte) {
	n := 32
	ck := params[3]
	el := params[4]
	eta := params[5]
	lg2eta1 := params[6]
 
	for i := 0; i < 32; i++ {
		rho[i] = sk[i]
	}
	for i := 0; i < 32; i++ {
		bK[i] = sk[n]
		n++
	}
	for i := 0; i < 32; i++ {
		tr[i] = sk[n]
		n++
	}
 
	var pos [2]int
	pos[0] = n; pos[1] = 0
	for j := 0; j < el; j++ {
		for i := 0; i < DL_DEGREE; i++ {
			s1[j][i] = int8(DL_nextword(lg2eta1, eta, sk, pos[:]))
		}
	}
	//fmt.Printf("lg2eta1 = %d eta= %d\n",lg2eta1,eta)
	//fmt.Printf("s1[0][0]= %d sk[0] = %d\n",s1[0][0],sk[n])
	for j := 0; j < ck; j++ {
		for i := 0; i < DL_DEGREE; i++ {
			s2[j][i] = int8(DL_nextword(lg2eta1, eta, sk, pos[:]))
		}
	}
	for j := 0; j < ck; j++ {
		for i := 0; i < DL_DEGREE; i++ {
			t0[j][i] = int16(DL_nextword(DL_D, (1 << (DL_D - 1)), sk, pos[:]))
		}
	}
}
 
// pack signature - change z
func DL_pack_sig(params []int, sig []byte, z [][DL_DEGREE]int32, ct []byte, h []byte) int {
	lg := params[1]
	gamma1 := 1 << lg
	ck := params[3]
	el := params[4]
	omega := params[7]

	var t int32
 
	for i := 0; i < 32; i++ {
		sig[i] = ct[i]
	}

	n := 32
	var pos [2]int
	pos[0] = 0; pos[1] = 0
	//pre-process z
	for i := 0; i < el; i++ {
		for m := 0; m < DL_DEGREE; m++ {
			t = z[i][m]
			if t > DL_PRIME/2 {
				t -= DL_PRIME
			}
			t = int32(gamma1) - t
			z[i][m] = t
		}
	}
	for j := 0; j < el; j++ {
		for i := 0; i < (DL_DEGREE*(lg+1))/8; i++ {
			sig[n] = DL_nextbyte32(lg+1, 0, z[j][:], pos[:])
			n++
		}
	}
	for i := 0; i < omega+ck; i++ {
		sig[n] = h[i]
		n++
	}
	return n
}
 
func DL_unpack_sig(params []int, z [][DL_DEGREE]int32, ct []byte, h []byte, sig []byte) {
	lg := params[1]
	gamma1 := 1 << lg
	ck := params[3]
	el := params[4]
	omega := params[7]
	var t int32
	m := 32 + (el*DL_DEGREE*(lg+1))/8

	for i := 0; i < 32; i++ {
		ct[i] = sig[i]
	}
 
	var pos [2]int
	pos[0] = 32; pos[1] = 0
	for j := 0; j < el; j++ {
		for i := 0; i < DL_DEGREE; i++ {
			t = DL_nextword(lg+1, 0, sig, pos[:])
			t = int32(gamma1)-t
			if t < 0 {
				t += DL_PRIME
			}
			z[j][i] = t
		}
	}
	for i := 0; i < omega+ck; i++ {
		h[i] = sig[m]
		m++
	}
}
 
// rejection sampling in range -ETA to +ETA
func DL_sample_Sn(params []int, rhod []byte, s []int8, n int) {
	eta := int8(params[5])
	lg2eta1 := params[6]
	sh := NewSHA3(SHA3_SHAKE256)
	var buff [272]byte
 
	for m := 0; m < 64; m++ {
		sh.Process(rhod[m])
	}
	sh.Process(byte(n & 0xff))
	sh.Process(byte((n >> 8) & 0xff))
	sh.Shake(buff[:], 272)
 
	var pos [2]int
	pos[0] = 0; pos[1] = 0
	for m := 0; m < DL_DEGREE; m++ {
		for next := true; next; next = s[m] > 2*eta {
			s[m] = int8(DL_nextword(lg2eta1, 0, buff[:], pos[:]))
		}
		s[m] = int8(eta - s[m])
	}
}
 
//uniform random sampling
func DL_sample_Y(params []int, k int, rhod []byte, y [][DL_DEGREE]int32) {
	lg := params[1]
	gamma1 := 1 << lg
	el := params[4]
	var ki int
	var w, t int32
	var buff [DL_YBYTES]byte
 
	for i := 0; i < el; i++ {
		sh := NewSHA3(SHA3_SHAKE256)
		for j := 0; j < 64; j++ {
			sh.Process(rhod[j])
		}
		ki = k + i
		sh.Process(byte(ki & 0xff))
		sh.Process(byte(ki >> 8))
		sh.Shake(buff[:], DL_YBYTES)
 
		var pos [2]int
		pos[0] = 0; pos[1] = 0
		for m := 0; m < DL_DEGREE; m++ {
			//take in LG+1 bits at a time
			w = DL_nextword(lg+1, 0, buff[:], pos[:])
			w = int32(gamma1) - w
			t = w >> 31
			y[i][m] = w + (DL_PRIME & t)
		}
	}
}
 
//CRH(rho,t1)
func DL_CRH1(params []int, H []byte, rho []byte, t1 [][DL_DEGREE]int16) {
	var pos [2]int
	pos[0] = 0; pos[1] = 0
	ck := params[3]
	sh := NewSHA3(SHA3_SHAKE256)
	//fmt.Printf("rho[1]= %d\n",rho[1])
	for i := 0; i < 32; i++ {
		sh.Process(rho[i])
	}
	for j := 0; j < ck; j++ {
		//fmt.Printf("X t1[j][0]= %d\n",t1[j][0])
		//fmt.Printf("Y t1[j][1]= %d\n",t1[j][1])
		//fmt.Printf("Z t1[j][2]= %d\n",t1[j][2])
		for i := 0; i < (DL_DEGREE*DL_TD)/8; i++ {
			nxt := DL_nextbyte16(DL_TD, 0, t1[j][:], pos[:])
			/* if i==0 && j==0 {
				//fmt.Printf(" nxt= %d",nxt)
			}
			*/
			sh.Process(nxt)
		}
	}
	sh.Shake(H, 32)
	//fmt.Printf("H[0]= %d\n",H[0])
}
 
//CRH(tr,M)
func DL_CRH2(H []byte, tr []byte, mess []byte, mlen int) {
	sh := NewSHA3(SHA3_SHAKE256)
	for i := 0; i < 32; i++ {
		sh.Process(tr[i])
	}
	for i := 0; i < mlen; i++ {
		sh.Process(mess[i])
	}
	sh.Shake(H, 64)
}
 
//CRH(K,mu)
func DL_CRH3(H []byte, bK []byte, mu []byte) {
	sh := NewSHA3(SHA3_SHAKE256)
	for i := 0; i < 32; i++ {
		sh.Process(bK[i])
	}
	for i := 0; i < 64; i++ {
		sh.Process(mu[i])
	}
	sh.Shake(H, 64)
}
 
//H(mu,w1)
func DL_H4(params []int, CT []byte, mu []byte, w1 [][DL_DEGREE]int8) {
	var pos [2]int
	pos[0] = 0; pos[1] = 0
	ck := params[3]
	dv := params[2]
	w1b := 4
	if dv == 88 {
		w1b = 6
	}
 
	sh := NewSHA3(SHA3_SHAKE256)
	for i := 0; i < 64; i++ {
		sh.Process(mu[i])
	}
	for j := 0; j < ck; j++ {
		for i := 0; i < (DL_DEGREE*w1b)/8; i++ {
			sh.Process(DL_nextbyte8(w1b, 0, w1[j][:], pos[:]))
		}
	}
	sh.Shake(CT, 32)
}
 
func DL_SampleInBall(params []int, ct []byte, c []int32) {
	tau := params[0]
	var signs [8]byte
	var buff [136]byte
	sh := NewSHA3(SHA3_SHAKE256)
 
	for i := 0; i < 32; i++ {
		sh.Process(ct[i])
	}
	sh.Shake(buff[:], 136)
 
	for i := 0; i < 8; i++ {
		signs[i] = buff[i]
	}
 
	k := 8
	b := 0
	DL_poly_zero(c)
	var sn int = int(signs[0])
	n := 1
	var j int
 
	for i := DL_DEGREE - tau; i < DL_DEGREE; i++ {
		for next := true; next; next = j > i {
			j = int(buff[k])
			k++
		}
		c[i] = c[j]
		c[j] = 1 - 2*(int32(sn&1))
		sn >>= 1
		b++
		if b == 8 {
			sn = int(signs[n])
			n++
			b = 0
		}
	}
}
 
func DL_Power2Round(t []int32, t0 []int16, t1 []int16) {
	var w, d, r int32
	for m := 0; m < DL_DEGREE; m++ {
		w = t[m]
		d = 1 << DL_D
		r = (w + d/2 - 1) >> DL_D
		w -= r << DL_D
		t1[m] = int16(r)
		t0[m] = int16(w)
	}
}
 
// ALPHA = (Q-1)/16 - borrowed from dilithium ref implementation
func DL_decompose_lo(params []int, a int32) int32 {
	var gamma2 int
	dv := params[2]
	var a0, a1 int32
	a1 = (a + 127) >> 7
	if dv == 32 {
		a1 = (a1*1025 + (1 << 21)) >> 22
		a1 &= 15
		gamma2 = (DL_PRIME - 1) / 32
	} else { //88
		a1 = (a1*11275 + (1 << 23)) >> 24
		a1 ^= ((43 - a1) >> 31) & a1
		gamma2 = (DL_PRIME - 1) / 88
	}
 
	a0 = a - a1 * 2 * int32(gamma2)
	a0 -= (((DL_PRIME-1)/2 - a0) >> 31) & DL_PRIME
	a0 += (a0 >> 31) & DL_PRIME
 
	return a0
}
 
// ALPHA = (Q-1)/16
func DL_decompose_hi(params []int, a int32) int8 {
	dv := params[2]
	var a1 int32 = (a + 127) >> 7

	if dv == 32 {
		a1 = (a1*1025 + (1 << 21)) >> 22
		a1 &= 15
	} else {
		a1 = (a1*11275 + (1 << 23)) >> 24
		a1 ^= ((43 - a1) >> 31) & a1
	}
	return int8(a1)
}
 
func DL_lobits(params []int, r0 []int32, r []int32) {
	for m := 0; m < DL_DEGREE; m++ {
		r0[m] = DL_decompose_lo(params, r[m])
	}
}
 
func DL_hibits(params []int, r1 []int8, r []int32) {
	for m := 0; m < DL_DEGREE; m++ {
		r1[m] = DL_decompose_hi(params, r[m])
	}
}
 
// before h initialised to zeros, hptr=0
// after new hptr returned and h[OMEGA+i]= hptr
func DL_MakePartialHint(params []int, h []byte, hptr int, z []int32, r []int32) int {
	var a0, a1 int8
	var rz int32
	omega := params[7]
	for m := 0; m < DL_DEGREE; m++ {
		a0 = DL_decompose_hi(params, r[m])
		rz = r[m] + z[m]
		rz -= DL_PRIME
		rz = rz + ((rz >> 31) & DL_PRIME)
		a1 = DL_decompose_hi(params, rz)
		if a0 != a1 {
			if hptr >= omega {
				return omega + 1
			}
			h[hptr] = byte(m) & 0xff
			hptr++
		}
	}
	return hptr
}
 
func DL_UsePartialHint(params []int, r []int8, h []byte, hptr int, i int, w []int32) int {
	dv := int8(params[2])
	omega := params[7]
	md := int8(dv / 2)
	var a0 int32
	var a1 int8
	for m := 0; m < DL_DEGREE; m++ {
		a1 = DL_decompose_hi(params, w[m])
		if m == int(h[hptr]) && hptr < int(h[omega+i]) { 
			hptr++
			a0 = DL_decompose_lo(params, w[m])
			if a0 <= DL_PRIME/2 {
				a1++
				if a1 >= md {
					a1 -= md
				}
			} else {
				a1--
				if a1 < 0 {
					a1 += md
				}
			}
		}
		r[m] = a1
	}
	return hptr
}
 
func DL_infinity_norm(w []int32) int32 {
	var az int32
	var n int32
	n = 0
	for m := 0; m < DL_DEGREE; m++ {
		az = w[m]
		if az > (DL_PRIME/2) {
			az = DL_PRIME - az
		}
		if az > n {
			n = az
		}
	}
	return n
}
 
//Dilithium API
func DL_keypair(params []int, tau []byte, sk []byte, pk []byte) {
	var buff [128]byte
	var rho [32]byte
	var rhod [64]byte
	var bK [32]byte
	var tr [32]byte        // 320 bytes
	var Aij [DL_DEGREE]int32 // 1024 bytes
	var w [DL_DEGREE]int32   // work space  1024 bytes
	var r [DL_DEGREE]int32   // work space  1024 bytes total = 12352
 
	ck := params[3]
	el := params[4]
 
	s1 := make([][DL_DEGREE]int8, el)  // 1280 bytes
	s2 := make([][DL_DEGREE]int8, ck)  // 1536 bytes
	t0 := make([][DL_DEGREE]int16, ck) // 3072 bytes
	t1 := make([][DL_DEGREE]int16, ck) // 3072 bytes
 
	sh := NewSHA3(SHA3_SHAKE256)
 
	for i := 0; i < 32; i++ {
		sh.Process(tau[i])
	}
	sh.Shake(buff[:], 128)
 
	for i := 0; i < 32; i++ {
		rho[i] = buff[i]
		bK[i] = buff[i+96]
	}
 
	for i := 0; i < 64; i++ {
		rhod[i] = buff[32+i]
	}
	//fmt.Printf("rhod[0]= %d\n", rhod[0])
 
	for i := 0; i < el; i++ {
		DL_sample_Sn(params, rhod[:], s1[i][:], i)
	}
	//fmt.Printf("s1[0][0]= %d\n",s1[0][0])
 
	for i := 0; i < ck; i++ {
		DL_sample_Sn(params, rhod[:], s2[i][:], el+i)
		//fmt.Printf("s2[0][0]= %d\n",s2[0][0])
		DL_poly_zero(r[:])
		for j := 0; j < el; j++ {
			DL_poly_scopy(w[:], s1[j][:])
			DL_ntt(w[:])
			DL_ExpandAij(rho[:], Aij[:], i, j) //This is bottleneck
			DL_poly_mul(w[:], w[:], Aij[:])
			DL_poly_add(r[:], r[:], w[:])
			//DL_poly_soft_reduce(r[:])  // be lazy
		}
		DL_poly_hard_reduce(r[:])
		/*for k:=0; k<DL_DEGREE;k++{
			fmt.Printf(" %d",r[k])
		}*/
		DL_intt(r[:])
		//fmt.Printf("\n1. r[0]= %d\n",r[0])
		//fmt.Printf("1. r[1]= %d\n",r[1])
		//os.exit(0)
		DL_poly_scopy(w[:], s2[i][:])
		DL_poly_pos(w[:])
		DL_poly_add(r[:], r[:], w[:])
		DL_poly_soft_reduce(r[:])
		//fmt.Printf("2. r[0]= %d\n",r[0])
		//fmt.Printf("2. r[1]= %d\n",r[1])
		DL_Power2Round(r[:], t0[i][:], t1[i][:])
		//fmt.Printf("t0[0][0]= %d\n",t0[0][0])
		//fmt.Printf("t0[0][1]= %d\n",t0[0][1])
		//fmt.Printf("t1[0][0]= %d\n",t1[0][0])
		//fmt.Printf("t1[0][1]= %d\n",t1[0][1])
	}
	DL_CRH1(params, tr[:], rho[:], t1)
	//fmt.Printf("tr[0]= %d\n",tr[0])
	//fmt.Printf("rho[0]= %d\n",rho[0])
	DL_pack_pk(params, pk, rho[:], t1)
	DL_pack_sk(params, sk, rho[:], bK[:], tr[:], s1, s2, t0)
}
 
func DL_signature(params []int, sk []byte, M []byte, sig []byte) int {
	var fk, nh, k int
	var badone bool

	var rho [32]byte
	var bK [32]byte
	var ct [32]byte
	var tr [32]byte
	var mu [64]byte
	var rhod [64]byte  //288 bytes
	var hint [100]byte //61 bytes
 
	var c [DL_DEGREE]int32 // 1024 bytes
	var w [DL_DEGREE]int32 // work space 1024 bytes
	var r [DL_DEGREE]int32 // work space 1024 bytes total= 21673 bytes
	//var Aij [DL_DEGREE]int32 //1024 bytes
 
	ck := params[3]
	el := params[4]
 
	s1 := make([][DL_DEGREE]int8, el)  // 1280 bytes
	s2 := make([][DL_DEGREE]int8, ck)  // 1536 bytes
	t0 := make([][DL_DEGREE]int16, ck) // 3072 bytes
	y := make([][DL_DEGREE]int32, el)    // 5120 bytes
	Ay := make([][DL_DEGREE]int32, ck)   // 6144 bytes
	w1 := make([][DL_DEGREE]int8, ck)  // 1280 bytes

 
	tau := params[0]
	lg := params[1]
	gamma1 := int32(1 << lg)
	dv := int32(params[2])
	gamma2 := (DL_PRIME - 1) / dv
	eta := params[5]
	beta := int32(tau * eta)
	omega := params[7]

	DL_unpack_sk(params, rho[:], bK[:], tr[:], s1, s2, t0, sk)
	
	//signature
	DL_CRH2(mu[:], tr[:], M, len(M))
	DL_CRH3(rhod[:], bK[:], mu[:])
	for k = 0; ; k++ {
		fk = k * el
		DL_sample_Y(params, fk, rhod[:], y)

		//fmt.Printf("X y[0][0] = %d y[0][1] = %d\n",y[0][0],y[0][1])
		//NTT y
		for i := 0; i < el; i++ {
			DL_ntt(y[i][:])
		}
		//fmt.Printf("Y y[0][0] = %d y[0][1] = %d\n",y[0][0],y[0][1])
		// Calculate Ay
		for i := 0; i < ck; i++ {
			DL_poly_zero(r[:])
			for j := 0; j < el; j++ {
				//Note : no NTTs in here
				DL_poly_copy(w[:], y[j][:])
				DL_ExpandAij(rho[:], c[:], i, j) // This is bottleneck // re-use c for Aij
				DL_poly_mul(w[:], w[:], c[:])
				DL_poly_add(r[:], r[:], w[:])
				//DL_poly_soft_reduce(r[:]) // be lazy
			}
			DL_poly_hard_reduce(r[:])
			DL_intt(r[:])
			DL_poly_copy(Ay[i][:], r[:])
			// Calculate w1
			DL_hibits(params, w1[i][:], Ay[i][:])

		}

		// Calculate c
		DL_H4(params, ct[:], mu[:], w1)
		DL_SampleInBall(params, ct[:], c[:])

		badone = false
		// Calculate z=y+c.s1
		DL_ntt(c[:])
		for i := 0; i<el; i++ {
			DL_poly_scopy(w[:], s1[i][:])
			DL_ntt(w[:])
			DL_poly_mul(w[:], w[:], c[:])
			DL_nres_it(w[:])
 
			DL_poly_add(y[i][:], y[i][:], w[:]) // re-use y for z
			DL_redc_it(y[i][:])                 // unNTT y
			DL_intt(y[i][:])
			DL_poly_soft_reduce(y[i][:])
			
			if DL_infinity_norm(y[i][:]) >= gamma1-beta { //seems to be always true
				badone = true;
				break;
			}
		}
		if badone {
			continue
		}
		// Calculate Ay=w-c.s2 and r0=DL_lobits(w-c.s2)
		nh = 0
		for i := 0; i < omega+ck; i++ {
			hint[i] = 0
		}
		for i := 0; i < ck; i++ {
			DL_poly_scopy(w[:], s2[i][:])
			DL_ntt(w[:])
			DL_poly_mul(w[:], w[:], c[:])
 
			DL_intt(w[:])
			DL_poly_sub(Ay[i][:], Ay[i][:], w[:])
			DL_poly_soft_reduce(Ay[i][:])     // Ay=w-cs2
			DL_lobits(params, w[:], Ay[i][:]) // r0
			if DL_infinity_norm(w[:]) >= gamma2-beta {
				badone = true
				break
			}
 
			DL_poly_mcopy(w[:], t0[i][:])
			DL_ntt(w[:])
			DL_poly_mul(w[:], w[:], c[:])
 
			DL_intt(w[:])
			DL_poly_negate(r[:], w[:]) // -ct0
			if DL_infinity_norm(r[:]) >= gamma2 {
				badone = true
				break
			}
			DL_poly_sub(Ay[i][:], Ay[i][:], r[:])
			DL_poly_soft_reduce(Ay[i][:])
 
			nh = DL_MakePartialHint(params, hint[:], nh, r[:], Ay[i][:])
			if nh > omega {
				badone = true
				break
			}
			hint[omega+i] = byte(nh)
		}
		if badone {
			continue
		}
		break
	}
	//fmt.Printf("y[0][0] = %d y[0][1] = %d\n",y[0][0],y[0][1])
	DL_pack_sig(params, sig, y[:], ct[:], hint[:])
	return k + 1
}
 
func DL_verify(params []int, pk []byte, M []byte, sig []byte) bool {
	var rho [32]byte
	var mu [64]byte
	var ct [32]byte
	var cct [32]byte
	var tr [32]byte // 192 bytes
	var hint [100]byte
 
	var Aij [DL_DEGREE]int32 // 1024 bytes
	var c [DL_DEGREE]int32   // 1024 bytes
	var w [DL_DEGREE]int32   // work space // 1024 bytes
	var r [DL_DEGREE]int32   // work space // 1024 bytes total=14077 bytes
 
	ck := params[3]
	el := params[4]
 
	z := make([][DL_DEGREE]int32, el)
	t1 := make([][DL_DEGREE]int16, ck)
	w1d := make([][DL_DEGREE]int8, ck)
 
	tau := params[0]
	lg := params[1]
	gamma1 := int32(1<<lg)
	eta := params[5]
	beta := int32(tau*eta)
	omega := params[7]
 
	// unpack public key and signature
	DL_unpack_pk(params, rho[:], t1, pk)
	DL_unpack_sig(params, z, ct[:], hint[:], sig)
	
	for i := 0; i < el; i++ {
		if DL_infinity_norm(z[i][:]) >= gamma1-beta {
			return false
		}
		DL_ntt(z[i][:]) // convert to ntt form
	}
 
	DL_CRH1(params, tr[:], rho[:], t1)
	DL_CRH2(mu[:], tr[:], M, len(M))
	DL_SampleInBall(params, ct[:], c[:])
	DL_ntt(c[:])
 
	// Calculate Az
	hints := 0
	for i := 0; i < ck; i++ {
		DL_poly_zero(r[:])
		for j := 0; j < el; j++ {
			//Note : no NTTs in here
			DL_poly_copy(w[:], z[j][:])
			DL_ExpandAij(rho[:], Aij[:], i, j)
			DL_poly_mul(w[:], w[:], Aij[:])
			DL_poly_add(r[:], r[:], w[:])
			//DL_poly_soft_reduce(r[:]) // be lazy
		}
		DL_poly_hard_reduce(r[:])
 
		// Calculate Az-ct1.s^d
		for m := 0; m < DL_DEGREE; m++ {
			w[m] = int32(t1[i][m])<<DL_D
		}
		DL_ntt(w[:])
		DL_poly_mul(w[:], w[:], c[:])
		DL_poly_sub(r[:], r[:], w[:])
		DL_intt(r[:])
 
		hints = DL_UsePartialHint(params, w1d[i][:], hint[:], hints, i, r[:])
		if hints > omega {
			return false
		}
	}
 
	DL_H4(params, cct[:], mu[:], w1d)
 
	for i := 0; i < 32; i++ {
		if ct[i] != cct[i] {
			return false
		}
	}
	return true
}
 
func DL_keypair_2(tau []byte, sk []byte, pk []byte) {
	DL_keypair(DL_PARAMS_2[:], tau, sk, pk)
}

func DL_signature_2(sk []byte, M []byte, sig []byte) int {
	return DL_signature(DL_PARAMS_2[:], sk, M, sig)
}
 
func DL_verify_2(pk []byte, M []byte, sig []byte) bool {
	return DL_verify(DL_PARAMS_2[:], pk, M, sig)
}
 
func DL_keypair_3(tau []byte, sk []byte, pk []byte) {
	DL_keypair(DL_PARAMS_3[:], tau, sk, pk)
}
 
func DL_signature_3(sk []byte, M []byte, sig []byte) int {
	return DL_signature(DL_PARAMS_3[:], sk, M, sig)
}
 
func DL_verify_3(pk []byte, M []byte, sig []byte) bool {
	return DL_verify(DL_PARAMS_3[:], pk, M, sig)
}
 
func DL_keypair_5(tau []byte, sk []byte, pk []byte) {
	DL_keypair(DL_PARAMS_5[:], tau, sk, pk)
}
 
func DL_signature_5(sk []byte, M []byte, sig []byte) int {
	return DL_signature(DL_PARAMS_5[:], sk, M, sig)
}
 
func DL_verify_5(pk []byte, M []byte, sig []byte) bool {
	return DL_verify(DL_PARAMS_5[:], pk, M, sig)
}
