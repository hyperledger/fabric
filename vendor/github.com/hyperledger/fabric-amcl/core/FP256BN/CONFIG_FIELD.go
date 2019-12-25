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


package FP256BN

// Modulus types
const NOT_SPECIAL int = 0
const PSEUDO_MERSENNE int = 1
const MONTGOMERY_FRIENDLY int = 2
const GENERALISED_MERSENNE int = 3

const NEGATOWER int = 0
const POSITOWER int = 1

// Modulus details
const MODBITS uint = 256 /* Number of bits in Modulus */
const PM1D2 uint = 1  /* Modulus mod 8 */
const MODTYPE int = NOT_SPECIAL //NOT_SPECIAL
const QNRI int = 0    // Fp2 QNR
const TOWER int = NEGATOWER   // Tower type
const FEXCESS int32=((int32(1)<<24)-1)

// Modulus Masks
const OMASK Chunk = ((Chunk(-1)) << (MODBITS % BASEBITS))
const TBITS uint = MODBITS % BASEBITS // Number of active bits in top word
const TMASK Chunk = (Chunk(1) << TBITS) - 1

