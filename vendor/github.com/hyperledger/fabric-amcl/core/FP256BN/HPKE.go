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
 */

/* Hybrid Public Key Encryption */

/* Following https://datatracker.ietf.org/doc/draft-irtf-cfrg-hpke/?include_text=1 */

package FP256BN


import "github.com/hyperledger/fabric-amcl/core"

func reverse(X []byte) {
	lx:=len(X)
	for i:=0;i<lx/2;i++ {
		ch :=X[i]
		X[i]=X[lx-i-1]
		X[lx-i-1]=ch
	}
}


func labeledExtract(SALT []byte,SUITE_ID []byte,label string,IKM []byte) []byte {
	rfc:="HPKE-v1"
	RFC:=[]byte(rfc)
	LABEL:=[]byte(label)
	var LIKM []byte
	for i:=0;i<len(RFC);i++ {
		LIKM=append(LIKM,RFC[i])
	}
	for i:=0;i<len(SUITE_ID);i++ {
		LIKM=append(LIKM,SUITE_ID[i])
	}
	for i:=0;i<len(LABEL);i++ {
		LIKM=append(LIKM,LABEL[i])
	}
	if IKM!=nil {
		for i:=0;i<len(IKM);i++ {
			LIKM=append(LIKM,IKM[i])
		}
	}
	return core.HKDF_Extract(core.MC_SHA2,HASH_TYPE,SALT,LIKM);
}

func labeledExpand(PRK []byte,SUITE_ID []byte,label string,INFO []byte,L int) []byte {
	rfc:="HPKE-v1"
	RFC:=[]byte(rfc)
	LABEL:=[]byte(label)
	AR := core.InttoBytes(L,2)
	var LINFO []byte
	for i:=0;i<len(AR);i++ {
		LINFO = append(LINFO, AR[i])
	}
	for i:=0;i<len(RFC);i++ {
		LINFO=append(LINFO,RFC[i])
	}	
	for i:=0;i<len(SUITE_ID);i++ {
		LINFO=append(LINFO,SUITE_ID[i])
	}
	for i:=0;i<len(LABEL);i++ {
		LINFO=append(LINFO,LABEL[i])
	}
	if INFO!=nil {
		for i:=0;i<len(INFO);i++ {
			LINFO=append(LINFO,INFO[i])
		}
	}

	return core.HKDF_Expand(core.MC_SHA2,HASH_TYPE,L,PRK,LINFO)
}

func extractAndExpand(config_id int,DH []byte,context []byte) []byte {
	kem := config_id&255
	txt := "KEM"
	KEM_ID := core.InttoBytes(kem,2)
	KEM := []byte(txt)
	var SUITE_ID []byte
	for i:=0;i<len(KEM);i++ {
		SUITE_ID=append(SUITE_ID,KEM[i])
	}
	SUITE_ID=append(SUITE_ID,KEM_ID[0])
	SUITE_ID=append(SUITE_ID,KEM_ID[1])

	PRK:=labeledExtract(nil,SUITE_ID,"eae_prk",DH);
	return labeledExpand(PRK,SUITE_ID,"shared_secret",context,HASH_TYPE);
}

func DeriveKeyPair(config_id int,SK []byte,PK []byte,SEED []byte) bool {
	counter:=0
	kem := config_id&255

	txt := "KEM"
	KEM_ID := core.InttoBytes(kem,2)
	KEM := []byte(txt)
	var SUITE_ID []byte
	for i:=0;i<len(KEM);i++ {
		SUITE_ID=append(SUITE_ID,KEM[i])
	}
	SUITE_ID=append(SUITE_ID,KEM_ID[0])
	SUITE_ID=append(SUITE_ID,KEM_ID[1])

    PRK:=labeledExtract(nil,SUITE_ID,"dkp_prk",SEED)
    var S []byte
	if kem==32 || kem==33 {
		S=labeledExpand(PRK,SUITE_ID,"sk",nil,EGS)
		reverse(S)
		if kem==32 {
			S[EGS-1]&=248
			S[0]&=127
			S[0]|=64
		} else {
			S[EGS-1]&=252
			S[0]|=128
		}
	} else {
		bit_mask:=0xff
		if kem==18 {
			bit_mask=1
		}
		for i:=0;i<EGS;i++ {
			S=append(S,0);
		}
		for !ECDH_IN_RANGE(S) && counter<256 {
			var INFO [1]byte
			INFO[0]=byte(counter)
			S=labeledExpand(PRK,SUITE_ID,"candidate",INFO[:],EGS)
			S[0]&=byte(bit_mask)
			counter++
		}
	}
	for i:=0;i<EGS;i++ {
		SK[i]=S[i]
	}
	ECDH_KEY_PAIR_GENERATE(nil, SK, PK)
	if kem==32 || kem==33 {
		reverse(PK)
	}
	if counter<256 {
		return true
	}
	return false;
}

func Encap(config_id int,skE []byte,pkE []byte,pkR []byte) []byte {
	DH:=make([]byte,EFS)
	var kemcontext []byte
	kem := config_id&255

	if kem==32 || kem==33 {
		reverse(pkR)
		ECDH_ECPSVDP_DH(skE, pkR, DH[:], 0)
		reverse(pkR)
		reverse(DH[:])
	} else {
		ECDH_ECPSVDP_DH(skE, pkR, DH[:], 0)
	}
	for i:=0;i<len(pkE);i++ {
		kemcontext=append(kemcontext,pkE[i]);
	}
	for i:=0;i<len(pkR);i++ {
		kemcontext=append(kemcontext,pkR[i]);
	}
	return extractAndExpand(config_id,DH[:],kemcontext)
}
		
func Decap(config_id int,skR []byte,pkE []byte,pkR []byte) []byte {
	DH:=make([]byte,EFS)
	var kemcontext [] byte
	kem := config_id&255

	if kem==32 || kem==33 {
		reverse(pkE)
		ECDH_ECPSVDP_DH(skR, pkE, DH[:], 0)
		reverse(pkE)
		reverse(DH[:])
	} else {
		ECDH_ECPSVDP_DH(skR, pkE, DH[:], 0)
	}

	for i:=0;i<len(pkE);i++ {
		kemcontext=append(kemcontext,pkE[i]);
	}
	for i:=0;i<len(pkR);i++ {
		kemcontext=append(kemcontext,pkR[i]);
	}
	return extractAndExpand(config_id,DH[:],kemcontext)
}

func AuthEncap(config_id int,skE []byte,skS []byte,pkE []byte,pkR []byte,pkS []byte) []byte {
	pklen:=len(pkE)
	DH:=make([]byte,EFS)
	DH1:=make([]byte,EFS)

	kemcontext:=make([] byte,3*pklen)
	kem := config_id&255

	if kem==32 || kem==33 {
		reverse(pkR)
		ECDH_ECPSVDP_DH(skE, pkR, DH[:], 0)
		ECDH_ECPSVDP_DH(skS, pkR, DH1[:], 0)
		reverse(pkR)
		reverse(DH[:])
		reverse(DH1[:])
	} else {
		ECDH_ECPSVDP_DH(skE, pkR, DH[:], 0)
		ECDH_ECPSVDP_DH(skS, pkR, DH1[:], 0)
	}
	ZZ:=make([]byte,2*EFS)
	for i:=0;i<EFS;i++ {
		ZZ[i]=DH[i]
		ZZ[EFS+i]=DH1[i]
	}

	for i:=0;i<pklen;i++ {
		kemcontext[i] = pkE[i]
        kemcontext[pklen+i]= pkR[i]
        kemcontext[2*pklen+i]= pkS[i]
	}
	return extractAndExpand(config_id,ZZ[:],kemcontext)
}

func AuthDecap(config_id int,skR []byte,pkE []byte,pkR []byte,pkS []byte) []byte  {
	pklen:=len(pkE)
	DH:=make([]byte,EFS)
	DH1:=make([]byte,EFS)
	kemcontext:=make([] byte,3*pklen)

	kem := config_id&255

	if kem==32 || kem==33 {
		reverse(pkE)
		reverse(pkS)
		ECDH_ECPSVDP_DH(skR[:], pkE, DH[:], 0)
		ECDH_ECPSVDP_DH(skR[:], pkS, DH1[:], 0)
		reverse(pkE)
		reverse(pkS)
		reverse(DH[:])
		reverse(DH1[:])
	} else {
		ECDH_ECPSVDP_DH(skR[:], pkE, DH[:], 0)
		ECDH_ECPSVDP_DH(skR[:], pkS, DH1[:], 0)
	}
	ZZ:=make([]byte,2*EFS)
	for i:=0;i<EFS;i++ {
		ZZ[i]=DH[i];
		ZZ[EFS+i]=DH1[i];
	}

	for i:=0;i<pklen;i++ {
		kemcontext[i] = pkE[i]
        kemcontext[pklen+i]= pkR[i]
        kemcontext[2*pklen+i]= pkS[i]
	}
	return extractAndExpand(config_id,ZZ[:],kemcontext)
}

/*
func printBinary(array []byte) {
	for i := 0; i < len(array); i++ {
		fmt.Printf("%02x", array[i])
	}
	fmt.Printf("\n")
}
*/

func KeySchedule(config_id int,mode int,Z []byte,info []byte,psk []byte,pskID []byte) ([]byte,[]byte,[]byte) {
	var context []byte

	kem:=config_id&255
	kdf:=(config_id>>8)&3
	aead:=(config_id>>10)&3

	txt := "HPKE"
	KEM := []byte(txt)
	var SUITE_ID []byte
	for i:=0;i<len(KEM);i++ {
		SUITE_ID=append(SUITE_ID,KEM[i])
	}
	num := core.InttoBytes(kem,2)
	SUITE_ID=append(SUITE_ID,num[0])
	SUITE_ID=append(SUITE_ID,num[1])
	num = core.InttoBytes(kdf,2)
	SUITE_ID=append(SUITE_ID,num[0])
	SUITE_ID=append(SUITE_ID,num[1])
	num = core.InttoBytes(aead,2)
	SUITE_ID=append(SUITE_ID,num[0])
	SUITE_ID=append(SUITE_ID,num[1])

	ar := core.InttoBytes(mode, 1)
	for i:=0;i<len(ar);i++ {
		context = append(context, ar[i])
	}

	H:=labeledExtract(nil,SUITE_ID,"psk_id_hash",pskID);
	for i:=0;i<HASH_TYPE;i++ {
		context=append(context,H[i])
	}
	H=labeledExtract(nil,SUITE_ID,"info_hash",info)
	for i:=0;i<HASH_TYPE;i++ {
		context=append(context,H[i])
	}
	//H=labeledExtract(nil,SUITE_ID,"psk_hash",psk)
	//secret:=labeledExtract(H,SUITE_ID,"secret",Z)

	secret:=labeledExtract(Z,SUITE_ID,"secret",psk);

	key:=labeledExpand(secret,SUITE_ID,"key",context,AESKEY)
	nonce:=labeledExpand(secret,SUITE_ID,"base_nonce",context,12)
	exp_secret:=labeledExpand(secret,SUITE_ID,"exp",context,HASH_TYPE)

	return key,nonce,exp_secret
}	

