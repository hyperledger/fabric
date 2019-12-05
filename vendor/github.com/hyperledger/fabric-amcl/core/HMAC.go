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

/*
 * Implementation of the Secure Hashing Algorithm (SHA-256)
 *
 * Generates a 256 bit message digest. It should be impossible to come
 * come up with two messages that hash to the same value ("collision free").
 *
 * For use with byte-oriented messages only.
 */

package core

const MC_SHA2 int=2
const MC_SHA3 int=3

/* Convert Integer to n-byte array */
func InttoBytes(n int, len int) []byte {
	var b []byte
	var i int
	for i = 0; i < len; i++ {
		b = append(b, 0)
	}
	i = len
	for n > 0 && i > 0 {
		i--
		b[i] = byte(n & 0xff)
		n /= 256
	}
	return b
}
/* general purpose hashing of Byte array|integer|Byte array. Output of length olen, padded with leading zeros if required */

func GPhashit(hash int,hlen int, olen int, A []byte, n int32, B []byte) []byte {
	var R []byte
	if hash == MC_SHA2 {
		if hlen == SHA256 {
			H := NewHASH256()
			if A != nil {
				H.Process_array(A)
			}
			if n >= 0 {
				H.Process_num(int32(n))
			}
			if B != nil {
				H.Process_array(B)
			}
			R = H.Hash()
		}
		if hlen == SHA384 {
			H := NewHASH384()
			if A != nil {
				H.Process_array(A)
			}
			if n >= 0 {
				H.Process_num(int32(n))
			}
			if B != nil {
				H.Process_array(B)
			}
			R = H.Hash()
		}
		if hlen == SHA512 {
			H := NewHASH512()
			if A != nil {
				H.Process_array(A)
			}
			if n >= 0 {
				H.Process_num(int32(n))
			}
			if B != nil {
				H.Process_array(B)
			}
			R = H.Hash()
		}
	}
	if hash == MC_SHA3 {
		H := NewSHA3(hlen)
		if A != nil {
			H.Process_array(A)
		}
		if n >= 0 {
			H.Process_num(int32(n))
		}
		if B != nil {
			H.Process_array(B)
		}
		R = H.Hash()
	}

	if R == nil {
		return nil
	}

	if olen == 0 {
		return R
	}
	var W []byte
	for i := 0; i < olen; i++ {
		W = append(W, 0)
	}
	if olen <= hlen {
		for i := 0; i < olen; i++ {
			W[i] = R[i]
		}
	} else {
		for i := 0; i < hlen; i++ {
			W[i+olen-hlen] = R[i]
		}
		for i := 0; i < olen-hlen; i++ {
			W[i] = 0
		}
	}
	return W
}

/* Simple hashing of byte array */
func SPhashit(hash int,hlen int, A []byte) []byte {
	return GPhashit(hash,hlen,0,A,-1,nil)
}

/* Key Derivation Function */
/* Input octet Z */
/* Output key of length olen */

func KDF2(hash int, sha int, Z []byte, P []byte, olen int) []byte {
	/* NOTE: the parameter olen is the length of the output k in bytes */
	hlen := sha
	var K []byte
	k := 0

	for i := 0; i < olen; i++ {
		K = append(K, 0)
	}

	cthreshold := olen / hlen
	if olen%hlen != 0 {
		cthreshold++
	}

	for counter := 1; counter <= cthreshold; counter++ {
		B := GPhashit(hash,sha, 0, Z, int32(counter), P)
		if k+hlen > olen {
			for i := 0; i < olen%hlen; i++ {
				K[k] = B[i]
				k++
			}
		} else {
			for i := 0; i < hlen; i++ {
				K[k] = B[i]
				k++
			}
		}
	}
	return K
}

/* Password based Key Derivation Function */
/* Input password p, salt s, and repeat count */
/* Output key of length olen */
func PBKDF2(hash int, sha int, Pass []byte, Salt []byte, rep int, olen int) []byte {
	d := olen / sha
	if olen%sha != 0 {
		d++
	}

	var F []byte
	var U []byte
	var S []byte
	var K []byte

	for i := 0; i < sha; i++ {
		F = append(F, 0)
		U = append(U, 0)
	}

	for i := 1; i <= d; i++ {
		for j := 0; j < len(Salt); j++ {
			S = append(S, Salt[j])
		}
		N := InttoBytes(i, 4)
		for j := 0; j < 4; j++ {
			S = append(S, N[j])
		}

		HMAC(MC_SHA2, sha, F[:], sha, Pass, S)

		for j := 0; j < sha; j++ {
			U[j] = F[j]
		}
		for j := 2; j <= rep; j++ {
			HMAC(MC_SHA2, sha, U[:], sha, Pass, U[:])
			for k := 0; k < sha; k++ {
				F[k] ^= U[k]
			}
		}
		for j := 0; j < sha; j++ {
			K = append(K, F[j])
		}
	}
	var key []byte
	for i := 0; i < olen; i++ {
		key = append(key, K[i])
	}
	return key
}

/* Calculate HMAC of m using key k. HMAC is tag of length olen (which is length of tag) */
func HMAC(hash int, sha int, tag []byte, olen int, K []byte, M []byte) int {
	/* Input is from an octet m        *
	* olen is requested output length in bytes. k is the key  *
	* The output is the calculated tag */
	var B []byte
	b := 0
	if hash == MC_SHA2 {
		b = 64
		if sha > 32 {
			b = 128
		}
	}
	if hash == MC_SHA3 {
		b=200-2*sha
	}
	if b == 0 {return 0}

	var K0 [200]byte
	//olen := len(tag)

	for i := 0; i < b; i++ {
		K0[i] = 0
	}

	if len(K) > b {
		B = SPhashit(hash, sha, K)
		for i := 0; i < sha; i++ {
			K0[i] = B[i]
		}
	} else {
		for i := 0; i < len(K); i++ {
			K0[i] = K[i]
		}
	}

	for i := 0; i < b; i++ {
		K0[i] ^= 0x36
	}
	B = GPhashit(hash, sha, 0, K0[0:b], -1, M)

	for i := 0; i < b; i++ {
		K0[i] ^= 0x6a
	}
	B = GPhashit(hash, sha, olen, K0[0:b], -1, B)

	for i := 0; i < olen; i++ {
		tag[i] = B[i]
	}

	return 1
}

func HKDF_Extract(hash int, hlen int, SALT []byte, IKM []byte)  []byte { 
	var PRK []byte
	for i:=0;i<hlen;i++ {
		PRK = append(PRK,0)
	}
	if SALT == nil {
		var H []byte
		for i := 0; i < hlen; i++ {
			H = append(H, 0)
		}
		HMAC(hash,hlen,PRK,hlen,H,IKM)
	} else {
		HMAC(hash,hlen,PRK,hlen,SALT,IKM)
	}
	return PRK
}

func HKDF_Expand(hash int, hlen int, olen int, PRK []byte, INFO []byte) []byte { 
	n := olen/hlen;
	flen := olen%hlen;

	var OKM []byte
	var T []byte
	var K [64]byte

	for i:=1;i<=n;i++ {
		for j := 0; j < len(INFO); j++ {
			T = append(T, INFO[j])
		}
		T = append(T, byte(i))
		HMAC(hash,hlen,K[:],hlen,PRK,T);
		T = nil
		for j := 0; j < hlen; j++ {
			OKM = append(OKM, K[j])
			T= append(T,K[j])
		}
	}
	if flen > 0 {
		for j := 0; j < len(INFO); j++ {
			T = append(T, INFO[j])
		}
		T = append(T, byte(n+1))
		HMAC(hash,hlen,K[:],flen,PRK,T);
		for j := 0; j < flen; j++ {
			OKM = append(OKM, K[j])
		}
	}
	return OKM
}


/*
func main() {
	var ikm []byte
	var salt []byte
	var info []byte
	
	for i:=0;i<22;i++ {ikm=append(ikm,0x0b)}
	for i:=0;i<13;i++ {salt=append(salt,byte(i))}
	for i:=0;i<10;i++ {info=append(info,byte(0xf0+i))}

	prk:=core.HKDF_Extract(core.MC_SHA2,32,salt,ikm)
	fmt.Printf("PRK= ")
	for i := 0; i < len(prk); i++ {
		fmt.Printf("%02x", prk[i])
	}

	okm:=core.HKDF_Expand(core.MC_SHA2,32,42,prk,info)
	fmt.Printf("\nOKM= ")
	for i := 0; i < len(okm); i++ {
		fmt.Printf("%02x", okm[i])
	}
	
}
*/

