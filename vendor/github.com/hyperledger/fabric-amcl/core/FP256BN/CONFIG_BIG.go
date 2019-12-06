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

// BIG length in bytes and number base
const MODBYTES uint = 32
const BASEBITS uint = 56

// BIG lengths and Masks
const NLEN int = int((1 + ((8*MODBYTES - 1) / BASEBITS)))
const DNLEN int = 2 * NLEN
const BMASK Chunk = ((Chunk(1) << BASEBITS) - 1)
const HBITS uint = (BASEBITS / 2)
const HMASK Chunk = ((Chunk(1) << HBITS) - 1)
const NEXCESS int = (1 << (uint(CHUNK) - BASEBITS - 1))

const BIGBITS int = int(MODBYTES * 8)

