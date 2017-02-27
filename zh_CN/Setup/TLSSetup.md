### 为所有CA服务以及ACA客户端和服务器之间的交互建立TLS的步骤：

1. 打开fabric/membersrvc目录下的**memebersrvc.yaml**文件，编辑security部分：
```
 security:
   serverhostoverride:
   tls_enabled: false
   client:
     cert:  
      file:
```
把`tls_enbabled`设置为`true`可以在ACA客户端和CA服务的REST之间启用TLS。

2. 下一步，把**serverhostoverride**设置为TLS服务端证书的**CN**（Common Name）。如果要从TLS服务端证书中获取Common Name，比如以使用OpenSSL的方式，可以执行下面的命令：

```
openssl x509 -in <<certificate.crt -text -noout
```
`certficate.crt`是服务端证书。如果你安装了openssl，而且能在电脑的任何位置执行openssl的话，那就可以得到如下格式的一个输出：

```
Certificate:
    Data:
        Version: 3 (0x2)
        Serial Number:
            4f:39:0f:ac:7b:ce:2b:9f:28:57:52:4a:bb:94:a6:e5:9c:69:99:56
        Signature Algorithm: ecdsa-with-SHA256
        Issuer: C=US, ST=California, L=San Francisco, O=Internet Widgets, Inc., OU=WWW
        Validity
            Not Before: Aug 24 16:27:00 2016 GMT
            Not After : Aug 24 16:27:00 2017 GMT
        **Subject**: C=US, ST=California, L=San Francisco, O=example.com, **CN=www.example.com**
        Subject Public Key Info:
            Public Key Algorithm: id-ecPublicKey
            EC Public Key:
                pub: 
                    04:38:d2:62:75:4a:18:d9:f7:fe:6a:e7:df:32:e2:
                    15:0f:01:9c:1b:4f:dc:ff:22:97:5c:2a:d9:5c:c3:
                    a3:ef:e3:90:3b:3c:8a:d2:45:b1:60:11:94:5e:a7:
                    51:e8:e5:5d:be:38:39:da:66:e1:99:46:0c:d3:45:
                    3d:76:7e:b7:8c
                ASN1 OID: prime256v1
        X509v3 extensions:
            X509v3 Key Usage: critical
                Digital Signature, Key Encipherment
            X509v3 Extended Key Usage: 
                TLS Web Server Authentication
            X509v3 Basic Constraints: critical
                CA:FALSE
            X509v3 Subject Key Identifier: 
                E8:9C:86:81:59:D4:D7:76:43:C7:2E:92:88:30:1B:30:A5:B3:A4:5C
            X509v3 Authority Key Identifier: 
                keyid:5E:33:AC:E0:9D:B9:F9:71:5F:1F:96:B5:84:85:35:BE:89:8C:35:C2

            X509v3 Subject Alternative Name: 
                DNS:www.example.com
    Signature Algorithm: ecdsa-with-SHA256
        30:45:02:21:00:9f:7e:93:93:af:3d:cf:7b:77:f0:55:2d:57:
        9d:a9:bf:b0:8c:9c:2e:cf:b2:b4:d8:de:f3:79:c7:66:7c:e7:
        4d:02:20:7e:9b:36:d1:3a:df:e4:d2:d7:3b:9d:73:c7:61:a8:
        2e:a5:b1:23:10:65:81:96:b1:3b:79:d4:a6:12:fe:f2:69
```

现在从输出中得到CN了（上例是**www.example.com**），把它设置成**serverhostoverride**的值。

3. 最后，确保`security.client.cert.file`正确的设置为TLS服务端证书的路径。
