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

/*
 *   Cryptographic strong random number generator 
 *
 *   Unguessable seed -> SHA -> PRNG internal state -> SHA -> random numbers
 *   Slow - but secure
 *
 *   See ftp://ftp.rsasecurity.com/pub/pdfs/bull-1.pdf for a justification
 */

/* Marsaglia & Zaman Random number generator constants */


package amcl

//import "fmt"

const rand_NK int=21
const rand_NJ int=6
const rand_NV int=8

type RAND struct {
	ira [rand_NK]uint32  /* random number...   */
	rndptr int
	borrow uint32
	pool_ptr int
	pool [32]byte
}

/* Terminate and clean up */
func (R *RAND) Clean() { /* kill internal state */
	R.pool_ptr=0; R.rndptr=0;
	for i:=0;i<32;i++ {R.pool[i]=0}
	for i:=0;i<rand_NK;i++ {R.ira[i]=0}
	R.borrow=0;
}

func NewRAND() *RAND {
	R:=new(RAND)
	R.Clean()
	return R
}

func (R *RAND) sbrand() uint32 { /* Marsaglia & Zaman random number generator */
	R.rndptr++
	if R.rndptr<rand_NK {return R.ira[R.rndptr]}
	R.rndptr=0
	k:=rand_NK-rand_NJ
	for i:=0;i<rand_NK;i++{ /* calculate next NK values */
		if k==rand_NK {k=0}
		t:=R.ira[k]
		pdiff:=t-R.ira[i]-R.borrow
		if pdiff<t {R.borrow=0}
		if pdiff>t {R.borrow=1}
		R.ira[i]=pdiff 
		k++
	}

	return R.ira[0];
}

func (R *RAND) sirand(seed uint32) {
	var m uint32=1;
	R.borrow=0
	R.rndptr=0
	R.ira[0]^=seed;
	for i:=1;i<rand_NK;i++ { /* fill initialisation vector */
		in:=(rand_NV*i)%rand_NK;
		R.ira[in]^=m;      /* note XOR */
		t:=m
		m=seed-m
		seed=t
	}
	for i:=0;i<10000;i++ {R.sbrand()} /* "warm-up" & stir the generator */
}

func (R *RAND) fill_pool() {
	sh:=NewHASH256()
	for i:=0;i<128;i++ {sh.Process(byte(R.sbrand()&0xff))}
	W:=sh.Hash()
	for i:=0;i<32;i++ {R.pool[i]=W[i]}
	R.pool_ptr=0;
}

func pack(b [4]byte) uint32 { /* pack 4 bytes into a 32-bit Word */
	return (((uint32(b[3]))&0xff)<<24)|((uint32(b[2])&0xff)<<16)|((uint32(b[1])&0xff)<<8)|(uint32(b[0])&0xff)
}

/* Initialize RNG with some real entropy from some external source */
func (R *RAND) Seed(rawlen int,raw []byte) { /* initialise from at least 128 byte string of raw random entropy */
	var b [4]byte
	sh:=NewHASH256()
	R.pool_ptr=0;

	for i:=0;i<rand_NK;i++ {R.ira[i]=0}
	if rawlen>0 {
		for i:=0;i<rawlen;i++ {
			sh.Process(raw[i])
		}
		digest:=sh.Hash()

/* initialise PRNG from distilled randomness */

		for i:=0;i<8;i++  {
			b[0]=digest[4*i]; b[1]=digest[4*i+1]; b[2]=digest[4*i+2]; b[3]=digest[4*i+3]
			R.sirand(pack(b))
		}
	}
	R.fill_pool()
}

/* get random byte */
func (R *RAND) GetByte() byte { 
	r:=R.pool[R.pool_ptr]
	R.pool_ptr++
	if R.pool_ptr>=32 {R.fill_pool()}
	return byte(r&0xff)
}

/* test main program */
/*
func main() {
	var raw [100]byte
	rng:=NewRAND()

	rng.Clean()
	for i:=0;i<100;i++ {raw[i]=byte(i)}

	rng.Seed(100,raw[:])
 
	for i:=0;i<1000;i++ {
		fmt.Printf("%03d ",rng.GetByte())
	}
}
*/
