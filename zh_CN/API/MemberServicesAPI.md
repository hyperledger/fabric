# 证书和授权 API

每个CA下的服务被拆分成两类[GRPC](http://www.grpc.io)接口，一类是以_P_结尾的公共接口，另一类是以_A_结尾的管理接口。

## ECA（Enrollment Certificate Authority － 注册CA）

ECA的管理员接口提供了以下函数：

	service ECAA { // 管理
	    rpc RegisterUser(RegisterUserReq) returns (Token);
	    rpc ReadUserSet(ReadUserSetReq) returns (UserSet);
	    rpc RevokeCertificate(ECertRevokeReq) returns (CAStatus); // 尚未实现
	    rpc PublishCRL(ECertCRLReq) returns (CAStatus); // 尚未实现
	}

`RegisterUser`函数用于注册用户，接收一个`RegisterUserReq`结构体，该结构体中包含用户名和多个角色(角色id使用位码表示，一个数字可以代表多个角色)。如果该用户之前没有注册过，ECA就注册该用户并返回一个唯一的一次性的密码，用户可以使用该密码请求ECA的公共接口，获取自己的注册证书对（注册证书对包含一个签名证书和一个加密证书）。如果该用户已经注册过就返回一个错误。

`ReadUserSet`函数允许且只允许审计用户，获取该区块链上已注册用户列表。

ECA的公共接口提供了以下函数：

	service ECAP { // 公共
	    rpc ReadCACertificate(Empty) returns (Cert);
	    rpc CreateCertificatePair(ECertCreateReq) returns (ECertCreateResp);
	    rpc ReadCertificatePair(ECertReadReq) returns (CertPair);
	    rpc ReadCertificateByHash(Hash) returns (Cert);
	    rpc RevokeCertificatePair(ECertRevokeReq) returns (CAStatus); // 尚未实现
	}

`ReadCACertificate`函数返回了ECA自身的证书。

`CreateCertificatePair`函数用于用户创建，获取自己的证书对。为此，用户需要连续两次调用该函数。第一次，把签名公钥，加密公钥以及上面`RegisterUser`函数返回的一次性密码一起发送给ECA。用户要用自己的签名私钥签名本次请求，ECA会用用户的签名公钥验证签名是否正确，如果正确，就能证明用户拥有该签名私钥。接着，ECA会用该用户的加密公钥加密一个challenge，返回给用户。用户如果能正确解密challenge，就能证明其确实拥有该加密私钥。然后用户要用解密后的challenge替换掉一次性密码重新请求`CreateCertificatePair`。如果发现用户对challenge的解密是正确的，ECA就会为该用户发布并返回其注册证书对。(解释：证书中只包含公钥，不包含私钥，需要发布给其他用户。假设用户a是请求发送方，用户b是请求接收方。a用自己的签名私钥签名了一个请求，b需要用对应的签名公钥验证签名是否正确，如果正确就能证明该请求未被篡改，且一定是a发送的请求，因为别人没有a的私钥。如果a使用了其加密私钥对请求内容进行了加密，b可以使用对用的公钥进行解密。签名密钥对和加密密钥对本质上没有区别，在这里只是用途不同。a需要私钥证明请求是自己自己发送的，所以a必须要有私钥，私钥泄漏后，别人可以随意假装成a，所以a必须要对私钥严格保密。fabric的ECA要求用户生成密钥对（实际上是validating peer为用户生成的），且只在网络上传输公钥，并通过上述两轮请求验证用户确实拥有其签名私钥和加密私钥，这种做法是安全的。ECA确定了密钥的正确性后，将公钥发布，供任意其他用户读取。)

`ReadCertificatePair`函数允许该区块链的任意用户读取任意其他用户的证书对。

`ReadCertificatePairByHash`函数允许该区块链的任意用户从ECA上读取与给定的hash匹配的证书。（另一种翻译：用户得到一个hash，调用`ReadCertificatePairByHash`函数并传入该hash，ECA返回与该hash匹配的证书。）

## TCA（Transaction Certificate Authority － 交易CA）

TCA的管理员接口提供了以下函数：

	service TCAA { // 管理
	    rpc RevokeCertificate(TCertRevokeReq) returns (CAStatus); // 尚未实现
	    rpc RevokeCertificateSet(TCertRevokeSetReq) returns (CAStatus); // 尚未实现
	    rpc PublishCRL(TCertCRLReq) returns (CAStatus); // 尚未实现
	}

TCA的公共接口提供了以下函数：

	service TCAP { // 公共
	    rpc ReadCACertificate(Empty) returns (Cert);
	    rpc CreateCertificate(TCertCreateReq) returns (TCertCreateResp);
	    rpc CreateCertificateSet(TCertCreateSetReq) returns (TCertCreateSetResp);
	    rpc RevokeCertificate(TCertRevokeReq) returns (CAStatus); // 尚未实现
	    rpc RevokeCertificateSet(TCertRevokeSetReq) returns (CAStatus); // 尚未实现
	}

`ReadCACertificate`函数返回了TCA自身的证书。

`CreateCertificate`函数允许用户创建取回一个新的交易证书。

`CreateCertificateSet`函数允许用户在一次调用中，创建取回一批交易证书。（一个交易证书可以给多笔交易用，但是fabric建议为每笔交易都创建一个新证书，所以才会提供该方法）

## TLSCA（TLS Certificate Authority － 安全传输层协议CA）

TLSCA的管理员接口提供了以下函数：

	service TLSCAA { // 管理
	    rpc RevokeCertificate(TLSCertRevokeReq) returns (CAStatus); // 尚未实现
	}

TLSCA的公共接口提供了以下函数：

	service TLSCAP { // 公共
	    rpc ReadCACertificate(Empty) returns (Cert);
	    rpc CreateCertificate(TLSCertCreateReq) returns (TLSCertCreateResp);
	    rpc ReadCertificate(TLSCertReadReq) returns (Cert);
	    rpc RevokeCertificate(TLSCertRevokeReq) returns (CAStatus); // 尚未实现
	}

`ReadCACertificate`函数返回了TLSCA自身的证书。

`CreateCertificate`函数允许用户创建获取一个新的TLS证书。

`ReadCertificate`函数允许用户获取之前已经创建号的TLS证书。
