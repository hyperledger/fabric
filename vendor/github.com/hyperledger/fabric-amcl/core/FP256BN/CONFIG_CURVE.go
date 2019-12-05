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

// Curve types
const WEIERSTRASS int = 0
const EDWARDS int = 1
const MONTGOMERY int = 2

// Pairing Friendly?
const NOT int = 0
const BN int = 1
const BLS int = 2

// Pairing Twist type
const D_TYPE int = 0
const M_TYPE int = 1

// Sparsity
const FP_ZERO int = 0
const FP_ONE int = 1
const FP_SPARSEST int = 2
const FP_SPARSER int = 3
const FP_SPARSE int = 4
const FP_DENSE int = 5

// Pairing x parameter sign
const POSITIVEX int = 0
const NEGATIVEX int = 1

// Curve type

const CURVETYPE int = WEIERSTRASS
const CURVE_PAIRING_TYPE int = BN

// Pairings only

const SEXTIC_TWIST int = M_TYPE
const SIGN_OF_X int = NEGATIVEX
const ATE_BITS int = 66
const G2_TABLE int = 83

// associated hash function and AES key size

const HASH_TYPE int = 32
const AESKEY int = 16

// These are manually decided policy decisions. To block any potential patent issues set to false.

const USE_GLV bool = true
const USE_GS_G2 bool = true
const USE_GS_GT bool = true

