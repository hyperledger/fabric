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

/* MPIN API Functions */

package FP256BN

import "time"
import "github.com/hyperledger/fabric-amcl/amcl"



const MFS int=int(MODBYTES)
const MGS int=int(MODBYTES)
//const PAS int=16
const BAD_PARAMS int=-11
const INVALID_POINT int=-14
const WRONG_ORDER int=-18
const BAD_PIN int=-19


/* Configure your PIN here */

const MAXPIN int32=10000  /* PIN less than this */
const PBLEN int32=14      /* Number of bits in PIN */
const TS int=10         /* 10 for 4 digit PIN, 14 for 6-digit PIN - 2^TS/TS approx = sqrt(MAXPIN) */
const TRAP int=200      /* 200 for 4 digit PIN, 2000 for 6-digit PIN  - approx 2*sqrt(MAXPIN) */

//const MPIN_HASH_TYPE int=amcl.SHA256

func mpin_hash(sha int,c *FP4,U *ECP) []byte {
	var w [MFS]byte
	var t [6*MFS]byte
	var h []byte

	c.geta().GetA().ToBytes(w[:]); for i:=0;i<MFS;i++ {t[i]=w[i]}
	c.geta().GetB().ToBytes(w[:]); for i:=MFS;i<2*MFS;i++ {t[i]=w[i-MFS]}
	c.getb().GetA().ToBytes(w[:]); for i:=2*MFS;i<3*MFS;i++ {t[i]=w[i-2*MFS]}
	c.getb().GetB().ToBytes(w[:]); for i:=3*MFS;i<4*MFS;i++ {t[i]=w[i-3*MFS]}

	U.GetX().ToBytes(w[:]); for i:=4*MFS;i<5*MFS;i++ {t[i]=w[i-4*MFS]}
	U.GetY().ToBytes(w[:]); for i:=5*MFS;i<6*MFS;i++ {t[i]=w[i-5*MFS]}

	if sha==amcl.SHA256 {
		H:=amcl.NewHASH256()
		H.Process_array(t[:])
		h=H.Hash()
	}
	if sha==amcl.SHA384 {
		H:=amcl.NewHASH384()
		H.Process_array(t[:])
		h=H.Hash()
	}
	if sha==amcl.SHA512 {
		H:=amcl.NewHASH512()
		H.Process_array(t[:])
		h=H.Hash()
	}
	if h==nil {return nil}
	R:=make([]byte,AESKEY)
	for i:=0;i<AESKEY;i++ {R[i]=h[i]}
	return R
}

/* Hash number (optional) and string to coordinate on curve */

func mhashit(sha int,n int32,ID []byte) []byte {
	var R []byte
	if sha==amcl.SHA256 {
		H:=amcl.NewHASH256()
		if n!=0 {H.Process_num(n)}
		H.Process_array(ID)
		R=H.Hash()
	}
	if sha==amcl.SHA384 {
		H:=amcl.NewHASH384()
		if n!=0 {H.Process_num(n)}
		H.Process_array(ID)
		R=H.Hash()
	}
	if sha==amcl.SHA512 {
		H:=amcl.NewHASH512()
		if n!=0 {H.Process_num(n)}
		H.Process_array(ID)
		R=H.Hash()
	}
	if R==nil {return nil}
	const RM int=int(MODBYTES)
	var W [RM]byte
	if sha>=RM {
		for i:=0;i<RM;i++ {W[i]=R[i]}
	} else {
		for i:=0;i<sha;i++ {W[i+RM-sha]=R[i]}
		for i:=0;i<RM-sha;i++ {W[i]=0}


	//	for i:=0;i<sha;i++ {W[i]=R[i]}	
	//	for i:=sha;i<RM;i++ {W[i]=0}
	}

	return W[:]
}

/* return time in slots since epoch */
func Today() int {
	now:=time.Now()
	return int(now.Unix())/(60*1440)
}

/* these next two functions help to implement elligator squared - http://eprint.iacr.org/2014/043 */
/* maps a random u to a point on the curve */
func emap(u *BIG,cb int) *ECP {
	var P *ECP
	x:=NewBIGcopy(u)
	p:=NewBIGints(Modulus)
	x.Mod(p)
	for true {
		P=NewECPbigint(x,cb)
		if !P.Is_infinity() {break}
		x.inc(1);  x.norm()
	}
	return P
}

/* returns u derived from P. Random value in range 1 to return value should then be added to u */
func unmap(u* BIG,P *ECP) int {
	s:=P.GetS()
	var R *ECP
	r:=0
	x:=P.GetX()
	u.copy(x)
	for true {
		u.dec(1); u.norm()
		r++
		R=NewECPbigint(u,s)
		if !R.Is_infinity() {break}
	}
	return r
}

func MPIN_HASH_ID(sha int,ID []byte) []byte {
	return mhashit(sha,0,ID)
}

/* these next two functions implement elligator squared - http://eprint.iacr.org/2014/043 */
/* Elliptic curve point E in format (0x04,x,y} is converted to form {0x0-,u,v} */
/* Note that u and v are indistinguisible from random strings */
func MPIN_ENCODING(rng *amcl.RAND,E []byte) int {
	var T [MFS]byte

	for i:=0;i<MFS;i++ {T[i]=E[i+1]}
	u:=FromBytes(T[:])
	for i:=0;i<MFS;i++ {T[i]=E[i+MFS+1]}
	v:=FromBytes(T[:])
		
	P:=NewECPbigs(u,v)
	if P.Is_infinity() {return INVALID_POINT}

	p:=NewBIGints(Modulus)
	u=Randomnum(p,rng)

	su:=int(rng.GetByte()); /*if (su<0) su=-su;*/ su%=2
		
	W:=emap(u,su)
	P.Sub(W)
	sv:=P.GetS()
	rn:=unmap(v,P)
	m:=int(rng.GetByte()); /*if (m<0) m=-m;*/ m%=rn
	v.inc(m+1)
	E[0]=byte(su+2*sv)
	u.ToBytes(T[:])
	for i:=0;i<MFS;i++ {E[i+1]=T[i]}
	v.ToBytes(T[:])
	for i:=0;i<MFS;i++ {E[i+MFS+1]=T[i]}		
		
	return 0
}

func MPIN_DECODING(D []byte) int {
	var T [MFS]byte

	if (D[0]&0x04)!=0 {return INVALID_POINT}

	for i:=0;i<MFS;i++ {T[i]=D[i+1]}
	u:=FromBytes(T[:])
	for i:=0;i<MFS;i++ {T[i]=D[i+MFS+1]}
	v:=FromBytes(T[:])

	su:=int(D[0]&1)
	sv:=int((D[0]>>1)&1)
	W:=emap(u,su)
	P:=emap(v,sv)
	P.Add(W)
	u=P.GetX()
	v=P.GetY()
	D[0]=0x04
	u.ToBytes(T[:])
	for i:=0;i<MFS;i++ {D[i+1]=T[i]}
	v.ToBytes(T[:])
	for i:=0;i<MFS;i++ {D[i+MFS+1]=T[i]}		
		
	return 0
}

/* R=R1+R2 in group G1 */
func MPIN_RECOMBINE_G1(R1 []byte,R2 []byte,R []byte) int {
	P:=ECP_fromBytes(R1)
	Q:=ECP_fromBytes(R2)

	if (P.Is_infinity() || Q.Is_infinity()) {return INVALID_POINT}

	P.Add(Q)

	P.ToBytes(R[:],false)
	return 0
}

/* W=W1+W2 in group G2 */
func MPIN_RECOMBINE_G2(W1 []byte,W2 []byte,W []byte) int {
	P:=ECP2_fromBytes(W1)
	Q:=ECP2_fromBytes(W2)

	if (P.Is_infinity() || Q.Is_infinity()) {return INVALID_POINT}

	P.Add(Q)
	
	P.ToBytes(W)
	return 0
}
	
/* create random secret S */
func MPIN_RANDOM_GENERATE(rng *amcl.RAND,S []byte) int {
	r:=NewBIGints(CURVE_Order);
	s:=Randomnum(r,rng)
	//if AES_S>0 {
	//	s.mod2m(2*AES_S)
	//}		
	s.ToBytes(S)
	return 0
}


func MPIN_EXTRACT_PIN(sha int,CID []byte,pin int,TOKEN []byte) int {
	return MPIN_EXTRACT_FACTOR(sha,CID,int32(pin)%MAXPIN,PBLEN,TOKEN)
}

/* Extract factor from TOKEN for identity CID */
func MPIN_EXTRACT_FACTOR(sha int,CID []byte,factor int32,facbits int32,TOKEN []byte) int {
	P:=ECP_fromBytes(TOKEN)
	if P.Is_infinity() {return INVALID_POINT}
	h:=mhashit(sha,0,CID)
	R:=ECP_mapit(h)

	R=R.pinmul(factor,facbits)
	P.Sub(R)

	P.ToBytes(TOKEN,false)

	return 0
}

/* Restore factor to TOKEN for identity CID */
func MPIN_RESTORE_FACTOR(sha int,CID []byte,factor int32,facbits int32,TOKEN []byte) int {
	P:=ECP_fromBytes(TOKEN)
	if P.Is_infinity() {return INVALID_POINT}
	h:=mhashit(sha,0,CID)
	R:=ECP_mapit(h)

	R=R.pinmul(factor,facbits)
	P.Add(R)

	P.ToBytes(TOKEN,false)

	return 0
}


/* Extract PIN from TOKEN for identity CID 
func MPIN_EXTRACT_PIN(sha int,CID []byte,pin int,TOKEN []byte) int {
	P:=ECP_fromBytes(TOKEN)
	if P.Is_infinity() {return INVALID_POINT}
	h:=mhashit(sha,0,CID)
	R:=ECP_mapit(h)

	R=R.pinmul(int32(pin)%MAXPIN,PBLEN)
	P.Sub(R)

	P.ToBytes(TOKEN,false)

	return 0
}*/

/* Implement step 2 on client side of MPin protocol */
func MPIN_CLIENT_2(X []byte,Y []byte,SEC []byte) int {
	r:=NewBIGints(CURVE_Order)
	P:=ECP_fromBytes(SEC)
	if P.Is_infinity() {return INVALID_POINT}

	px:=FromBytes(X)
	py:=FromBytes(Y)
	px.add(py)
	px.Mod(r)
	//px.rsub(r)

	P=G1mul(P,px)
	P.neg()
	P.ToBytes(SEC,false)
	//G1mul(P,px).ToBytes(SEC,false)
	return 0
}

/* Implement step 1 on client side of MPin protocol */
func MPIN_CLIENT_1(sha int,date int,CLIENT_ID []byte,rng *amcl.RAND,X []byte,pin int,TOKEN []byte,SEC []byte,xID []byte,xCID []byte,PERMIT []byte) int {
	r:=NewBIGints(CURVE_Order)
		
	var x *BIG
	if (rng!=nil) {
		x=Randomnum(r,rng)
		//if AES_S>0 {
		//	x.mod2m(2*AES_S)
		//}
		x.ToBytes(X)
	} else {
		x=FromBytes(X)
	}

	h:=mhashit(sha,0,CLIENT_ID)
	P:=ECP_mapit(h)
	
	T:=ECP_fromBytes(TOKEN)
	if T.Is_infinity() {return INVALID_POINT}

	W:=P.pinmul(int32(pin)%MAXPIN,PBLEN)
	T.Add(W)
	if date!=0 {
		W=ECP_fromBytes(PERMIT)
		if W.Is_infinity() {return INVALID_POINT}
		T.Add(W)
		h=mhashit(sha,int32(date),h)
		W=ECP_mapit(h)
		if xID!=nil {
			P=G1mul(P,x)
			P.ToBytes(xID,false)
			W=G1mul(W,x)
			P.Add(W)
		} else {
			P.Add(W)
			P=G1mul(P,x)
		}
		if xCID!=nil {P.ToBytes(xCID,false)}
	} else {
		if xID!=nil {
			P=G1mul(P,x)
			P.ToBytes(xID,false)
		}
	}


	T.ToBytes(SEC,false)
	return 0
}

/* Extract Server Secret SST=S*Q where Q is fixed generator in G2 and S is master secret */
func MPIN_GET_SERVER_SECRET(S []byte,SST []byte) int {
	Q:=ECP2_generator(); 

	s:=FromBytes(S)
	Q=G2mul(Q,s)
	Q.ToBytes(SST)
	return 0
}

/*
 W=x*H(G);
 if RNG == NULL then X is passed in 
 if RNG != NULL the X is passed out 
 if type=0 W=x*G where G is point on the curve, else W=x*M(G), where M(G) is mapping of octet G to point on the curve
*/
func MPIN_GET_G1_MULTIPLE(rng *amcl.RAND,typ int,X []byte,G []byte,W []byte) int {
	var x *BIG
	r:=NewBIGints(CURVE_Order)
	if rng!=nil {
		x=Randomnum(r,rng)
		//if AES_S>0 {
		//	x.mod2m(2*AES_S)
		//}
		x.ToBytes(X)
	} else {
		x=FromBytes(X)
	}
	var P *ECP
	if typ==0 {
		P=ECP_fromBytes(G)
		if P.Is_infinity() {return INVALID_POINT}
	} else {P=ECP_mapit(G)}

	G1mul(P,x).ToBytes(W,false)
	return 0
}

/* Client secret CST=S*H(CID) where CID is client ID and S is master secret */
/* CID is hashed externally */
func MPIN_GET_CLIENT_SECRET(S []byte,CID []byte,CST []byte) int {
	return MPIN_GET_G1_MULTIPLE(nil,1,S,CID,CST)
}

/* Time Permit CTT=S*(date|H(CID)) where S is master secret */
func MPIN_GET_CLIENT_PERMIT(sha,date int,S []byte,CID []byte,CTT []byte) int {
	h:=mhashit(sha,int32(date),CID)
	P:=ECP_mapit(h)

	s:=FromBytes(S)
	G1mul(P,s).ToBytes(CTT,false)
	return 0
}

/* Outputs H(CID) and H(T|H(CID)) for time permits. If no time permits set HID=HTID */
func MPIN_SERVER_1(sha int,date int,CID []byte,HID []byte,HTID []byte) {
	h:=mhashit(sha,0,CID)
	P:=ECP_mapit(h)
	
	P.ToBytes(HID,false);
	if date!=0 {
	//	if HID!=nil {P.ToBytes(HID,false)}
		h=mhashit(sha,int32(date),h)
		R:=ECP_mapit(h)
		P.Add(R)
		P.ToBytes(HTID,false)
	} //else {P.ToBytes(HID,false)}
}

/* Implement step 2 of MPin protocol on server side */
func MPIN_SERVER_2(date int,HID []byte,HTID []byte,Y []byte,SST []byte,xID []byte,xCID []byte,mSEC []byte,E []byte,F []byte) int {
//	q:=NewBIGints(Modulus)
	Q:=ECP2_generator(); 

	sQ:=ECP2_fromBytes(SST)
	if sQ.Is_infinity() {return INVALID_POINT}	

	var R *ECP
	if date!=0 {
		R=ECP_fromBytes(xCID)
	} else {
		if xID==nil {return BAD_PARAMS}
		R=ECP_fromBytes(xID)
	}
	if R.Is_infinity() {return INVALID_POINT}

	y:=FromBytes(Y)
	var P *ECP
	if date!=0 {
		P=ECP_fromBytes(HTID)
	} else {
		if HID==nil {return BAD_PARAMS}
		P=ECP_fromBytes(HID)
	}
	
	if P.Is_infinity() {return INVALID_POINT}

	P=G1mul(P,y)
	P.Add(R)
	//P.Affine()
	R=ECP_fromBytes(mSEC)
	if R.Is_infinity() {return INVALID_POINT}

	var g *FP12
//		FP12 g1=new FP12(0);

	g=Ate2(Q,R,sQ,P)
	g=Fexp(g)

	if !g.Isunity() {
		if (HID!=nil && xID!=nil && E!=nil && F!=nil) {
			g.ToBytes(E)
			if date!=0 {
				P=ECP_fromBytes(HID)
				if P.Is_infinity() {return INVALID_POINT}
				R=ECP_fromBytes(xID)
				if R.Is_infinity() {return INVALID_POINT}

				P=G1mul(P,y)
				P.Add(R)
				//P.Affine()
			}
			g=Ate(Q,P)
			g=Fexp(g)
			g.ToBytes(F)
		}
		return BAD_PIN
	}

	return 0
}

/* Pollards kangaroos used to return PIN error */
func MPIN_KANGAROO(E []byte,F []byte) int {
	ge:=FP12_fromBytes(E)
	gf:=FP12_fromBytes(F)
	var distance [TS]int
	t:=NewFP12copy(gf)

	var table []*FP12
	var i int
	s:=1
	for m:=0;m<TS;m++ {
		distance[m]=s
		table=append(table,NewFP12copy(t))
		s*=2
		t.usqr()
	}
	t.one()
	dn:=0
	for j:=0;j<TRAP;j++ {
		i=t.geta().geta().GetA().lastbits(20)%TS
		t.Mul(table[i])
		dn+=distance[i]
	}
	gf.Copy(t); gf.conj()
	steps:=0; dm:=0
	res:=0
	for dm-dn<int(MAXPIN) {
		steps++
		if steps>4*TRAP {break}
		i=ge.geta().geta().GetA().lastbits(20)%TS;
		ge.Mul(table[i])
		dm+=distance[i]
		if ge.Equals(t) {
			res=dm-dn
			break;
		}
		if ge.Equals(gf) {
			res=dn-dm
			break
		}

	}
	if (steps>4*TRAP || dm-dn>=int(MAXPIN)) {res=0 }    // Trap Failed  - probable invalid token
	return int(res)
}

/* Functions to support M-Pin Full */

func MPIN_PRECOMPUTE(TOKEN []byte,CID []byte,G1 []byte,G2 []byte) int {
	var P,T *ECP
	var g *FP12

	T=ECP_fromBytes(TOKEN)
	if T.Is_infinity() {return INVALID_POINT} 

	P=ECP_mapit(CID)

	Q:=ECP2_generator(); 

	g=Ate(Q,T)
	g=Fexp(g)
	g.ToBytes(G1)

	g=Ate(Q,P)
	g=Fexp(g)
	g.ToBytes(G2)

	return 0
}

/* Hash the M-Pin transcript - new */

func MPIN_HASH_ALL(sha int,HID []byte,xID []byte,xCID []byte,SEC []byte,Y []byte,R []byte,W []byte) []byte {
	tlen:=0
	var T [10*int(MODBYTES)+4]byte

	for i:=0;i<len(HID);i++ {T[i]=HID[i]}
	tlen+=len(HID)
	if xCID!=nil {
		for i:=0;i<len(xCID);i++ {T[i+tlen]=xCID[i]}
		tlen+=len(xCID)
	} else {
		for i:=0;i<len(xID);i++ {T[i+tlen]=xID[i]}
		tlen+=len(xID)
	}	
	for i:=0;i<len(SEC);i++ {T[i+tlen]=SEC[i]}
	tlen+=len(SEC)		
	for i:=0;i<len(Y);i++ {T[i+tlen]=Y[i]}
	tlen+=len(Y)
	for i:=0;i<len(R);i++ {T[i+tlen]=R[i]}
	tlen+=len(R)		
	for i:=0;i<len(W);i++ {T[i+tlen]=W[i]}
	tlen+=len(W)	

	return mhashit(sha,0,T[:])
}

/* calculate common key on client side */
/* wCID = w.(A+AT) */
func MPIN_CLIENT_KEY(sha int,G1 []byte,G2 []byte,pin int,R []byte,X []byte,H []byte,wCID []byte,CK []byte) int {

	g1:=FP12_fromBytes(G1)
	g2:=FP12_fromBytes(G2)
	z:=FromBytes(R)
	x:=FromBytes(X)
	h:=FromBytes(H)

	W:=ECP_fromBytes(wCID)
	if W.Is_infinity() {return INVALID_POINT} 

	W=G1mul(W,x)

//	f:=NewFP2bigs(NewBIGints(Fra),NewBIGints(Frb))
	r:=NewBIGints(CURVE_Order)
//	q:=NewBIGints(Modulus)

	z.add(h);	//new
	z.Mod(r);

	g2.pinpow(pin,int(PBLEN))
	g1.Mul(g2)

	c:=g1.Compow(z,r);
/*
	m:=NewBIGcopy(q)
	m.Mod(r)

	a:=NewBIGcopy(z)
	a.Mod(m)

	b:=NewBIGcopy(z)
	b.div(m)


	c:=g1.trace()
	g2.Copy(g1)
	g2.frob(f)
	cp:=g2.trace()
	g1.conj()
	g2.Mul(g1)
	cpm1:=g2.trace()
	g2.Mul(g1)
	cpm2:=g2.trace()

	c=c.xtr_pow2(cp,cpm1,cpm2,a,b)
*/
	t:=mpin_hash(sha,c,W);

	for i:=0;i<AESKEY;i++ {CK[i]=t[i]}

	return 0
}

/* calculate common key on server side */
/* Z=r.A - no time permits involved */

func MPIN_SERVER_KEY(sha int,Z []byte,SST []byte,W []byte,H []byte,HID []byte,xID []byte,xCID []byte,SK []byte) int {
	sQ:=ECP2_fromBytes(SST)
	if sQ.Is_infinity() {return INVALID_POINT} 
	R:=ECP_fromBytes(Z)
	if R.Is_infinity() {return INVALID_POINT} 
	A:=ECP_fromBytes(HID)
	if A.Is_infinity() {return INVALID_POINT} 

	var U *ECP
	if xCID!=nil {
		U=ECP_fromBytes(xCID)
	} else	{U=ECP_fromBytes(xID)}
	if U.Is_infinity() {return INVALID_POINT} 

	w:=FromBytes(W)
	h:=FromBytes(H)
	A=G1mul(A,h)	// new
	R.Add(A)
	//R.Affine()

	U=G1mul(U,w)
	g:=Ate(sQ,R)
	g=Fexp(g)

	c:=g.trace()

	t:=mpin_hash(sha,c,U)

	for i:=0;i<AESKEY;i++ {SK[i]=t[i]}

	return 0
}

/* return time since epoch */
func MPIN_GET_TIME() int {
	now:=time.Now()
	return int(now.Unix())
}

/* Generate Y = H(epoch, xCID/xID) */
func MPIN_GET_Y(sha int,TimeValue int,xCID []byte,Y []byte) {
	h:= mhashit(sha,int32(TimeValue),xCID)
	y:= FromBytes(h)
	q:=NewBIGints(CURVE_Order)
	y.Mod(q)
	//if AES_S>0 {
	//	y.mod2m(2*AES_S)
	//}
	y.ToBytes(Y)
}
        
/* One pass MPIN Client */
func MPIN_CLIENT(sha int,date int,CLIENT_ID []byte,RNG *amcl.RAND,X []byte,pin int,TOKEN []byte,SEC []byte,xID []byte,xCID []byte,PERMIT []byte,TimeValue int,Y []byte) int {
	rtn:=0
        
	var pID []byte
	if date == 0 {
		pID = xID
	} else {pID = xCID}
          
	rtn = MPIN_CLIENT_1(sha,date,CLIENT_ID,RNG,X,pin,TOKEN,SEC,xID,xCID,PERMIT)
	if rtn != 0 {return rtn}
        
	MPIN_GET_Y(sha,TimeValue,pID,Y)
        
	rtn = MPIN_CLIENT_2(X,Y,SEC)
	if rtn != 0 {return rtn}
        
	return 0
}

/* One pass MPIN Server */
func MPIN_SERVER(sha int,date int,HID []byte,HTID []byte,Y []byte,SST []byte,xID []byte,xCID []byte,SEC []byte,E []byte,F []byte,CID []byte,TimeValue int) int {
	rtn:=0
        
	var pID []byte
	if date == 0 {
		pID = xID
	} else {pID = xCID}
       
	MPIN_SERVER_1(sha,date,CID,HID,HTID)
	MPIN_GET_Y(sha,TimeValue,pID,Y);
    
	rtn = MPIN_SERVER_2(date,HID,HTID,Y,SST,xID,xCID,SEC,E,F)
	if rtn != 0 {return rtn}
        
	return 0
}

