## 证书授权安装（CA）

_证书授权_（CA）为区块链的用户提供了几个证书服务。更具体地说，这些服务涉及到区块链上的_用户注册_，_事务_调用和用户与区块链组件之间的_TLS_安全连接。

本文内容基于[fabric开发者设置](../dev-setup/devenv.md)或[fabric网络设置](Network-setup.md)。如果你尚未根据这两个文档中的任意一个设置过环境，请先完成设置之后再继续下文。

### 注册证书授权服务

_注册证书授权服务_（ECA）允许新用户在区块链网络中注册，并让注册用户能请求_一对注册证书_。其中一个证书用于数据签名，一个用于数据加密。证书里的公钥必须是ECDSA类型的，这意味着，为了能在[ECIES](https://en.wikipedia.org/wiki/Integrated_Encryption_Scheme)（椭圆曲线集成加密系统）中使用，用户要转换数据加密私钥。

### 事务证书授权服务

用户注册成功之后，就可以向_事务证书授权服务(TCA)_请求_事务证书_了。这些证书将被用于在区块链上部署Chaincode事务和调用Chaincode事务。尽管一个_事务证书_能被用于多个事务，但出于隐私原因的考虑，建议每个事务都用一个新的_交易证书_。

### TLS证书授权服务

除了_注册证书_和_事务证书_，用户还需要用_TLS证书_保护通信频道。_TLS证书_可以向_TLS证书授权服务_（TLSCA）请求。

## 配置

所有的CA服务都是同一个进程提供的，可以通过设置`membersrvc.yaml`中的参数来配置，该文件和CA的二进制可执行文件放在同一个目录下。更具体点说，有以下可以设置的参数：

- `server.gomaxprocs`: 限制CA可以使用的操作系统线程数量。
- `server.rootpath`: CA存储其状态的根目录。
- `server.cadir`: CA存储其状态的目录的名称。
- `server.port`: 所有的CA服务所监听的端口（membersrvc利用[GRPC](http://www.grpc.io)通过同一个端口对外提供多种CA服务）。

此外，通过调整下面的参数可以启用/禁用不同等级的日志：

- `logging.trace` (默认关闭，仅用于测试)
- `logging.info`
- `logging.warning`
- `logging.error`
- `logging.panic`

这些字段也可以通过环境变量设置，环境变量的优先等级比yaml文件高。和yaml中对应的环境变量是：

```
    MEMBERSRVC_CA_SERVER_GOMAXPROCS
    MEMBERSRVC_CA_SERVER_ROOTPATH
    MEMBERSRVC_CA_SERVER_CADIR
    MEMBERSRVC_CA_SERVER_PORT
```

除此之外，CA能预加载一些注册用户，可以像下面一样指定用户名，角色和密码：

```
    eca:
    	users:
    		alice: 2 DRJ20pEql15a
    		bob: 4 7avZQLwcUe9q
```
角色的值是简单的掩码：
```
    CLIENT = 1;
    PEER = 2;
    VALIDATOR = 4;
    AUDITOR = 8;
```

比如，一个peer如果也是validator的话，角色值就是6。

CA第一次启动时，会生成所有必需的状态（比如内嵌数据库，CA证书，区块链密钥等），并将这些状态写入配置的目录下。CA服务（比如ECA，TCA和TLSCA）现在默认是自签名的。如果这些证书应该被某些根CA签名，可以手动的用CA状态目录下的`*.priv`（私钥）和`*.pub`（公钥）来做，然后用被某根CA签名过的证书替换自签名的证书`*.cert`。CA下次运行时，会读取并使用这些证书。

## CA运维

你可以通过源码[构建并运行](#build-and-run)CA，或者，也可以使用Docker Compose和发布在DockerHub或其他注册中心上的镜像。使用Docker Compose显然是最简单的方式。

### Docker Compose

这是一个CA的docker-compose.yml例子：

```
membersrvc:
  image: hyperledger/fabric-membersrvc
  command: membersrvc
```

在Mac或Windows上原生的使用docker，docker-compose.yml要这么写：

```
membersrvc:
  image: hyperledger/fabric-membersrvc
  ports:
    - "7054:7054"
  command: membersrvc
```

如果在同一个docker-compose.yml中配置一个或多个`peer`节点，需要让peer延迟启动，因为peer会连接CA，所以需要留出足够的时间让CA先完成启动。

```
membersrvc:
  image: hyperledger/fabric-membersrvc
  command: membersrvc
vp0:
  image: hyperledger/fabric-peer
  environment:
    - CORE_PEER_ADDRESSAUTODETECT=true
    - CORE_VM_ENDPOINT=http://172.17.0.1:2375
    - CORE_LOGGING_LEVEL=DEBUG
    - CORE_PEER_ID=vp0
    - CORE_SECURITY_ENROLLID=test_vp0
    - CORE_SECURITY_ENROLLSECRET=MwYpmSRjupbT
  links:
    - membersrvc
  command: sh -c "sleep 5; peer node start"
```

在Mac或Windows上原生的使用docker，docker-compose.yml要这么写：

```
membersrvc:
  image: hyperledger/fabric-membersrvc
  ports:
    - "7054:7054"
  command: membersrvc
vp0:
  image: hyperledger/fabric-peer
  ports:
    - "7050:7050"
    - "7051:7051"
    - "7052:7052"
  environment:
    - CORE_PEER_ADDRESSAUTODETECT=true
    - CORE_VM_ENDPOINT=unix:///var/run/docker.sock
    - CORE_LOGGING_LEVEL=DEBUG
    - CORE_PEER_ID=vp0
    - CORE_SECURITY_ENROLLID=test_vp0
    - CORE_SECURITY_ENROLLSECRET=MwYpmSRjupbT
  links:
    - membersrvc
  command: sh -c "sleep 5; peer node start"
```

### 构建和运行

在`membersrvc`目录中执行以下命令可以构建CA：

```
cd $GOPATH/src/github.com/hyperledger/fabric
make membersrvc
```

构建完成后运行下面的命令启动CA：

```
build/bin/membersrvc
```

**注意：** CA必需要在任何fabric的peer节点之前启动，这样就可以保证CA完成初始化之后才有peer节点尝试连接它。

CA会在$GOPATH/src/github.com/hyperledger/fabric/membersrvc目录下找`membersrvc.yaml`配置文件。如果CA是第一次启动，它会创建所有必需的状态（比如，内嵌数据库，CA证书，区块链密钥等），并将这些状态写入配置的目录下。

<!-- 这里要谨记：

如果启动peer时启用了security/privacy设置，就必须添加如下环境变量：security，CA地址和peer的ID和密码。另外，fabric-membersrvc容器必须在所有的peer之前启动。所以我们要在执行peer命令之前添加一些延迟。下面是在**Vagrant**环境中运行单个peer和一个membership服务的docker-compose.yml：


```
vp0:
  image: hyperledger/fabric-peer
  environment:
  - CORE_PEER_ADDRESSAUTODETECT=true
  - CORE_VM_ENDPOINT=http://172.17.0.1:2375
  - CORE_LOGGING_LEVEL=DEBUG
  - CORE_PEER_ID=vp0
  - CORE_PEER_TLS_ENABLED=true
  - CORE_PEER_TLS_SERVERHOSTOVERRIDE=OBC
  - CORE_PEER_TLS_CERT_FILE=./bddtests/tlsca.cert
  - CORE_PEER_TLS_KEY_FILE=./bddtests/tlsca.priv
  command: sh -c "sleep 5; peer node start"

membersrvc:
   image: hyperledger/fabric-membersrvc
   command: membersrvc
```

```
docker run --rm -it -e CORE_VM_ENDPOINT=http://172.17.0.1:2375 -e CORE_PEER_ID=vp0 -e CORE_PEER_ADDRESSAUTODETECT=true -e CORE_SECURITY_ENABLED=true -e CORE_SECURITY_PRIVACY=true -e CORE_PEER_PKI_ECA_PADDR=172.17.0.1:7054 -e CORE_PEER_PKI_TCA_PADDR=172.17.0.1:7054 -e CORE_PEER_PKI_TLSCA_PADDR=172.17.0.1:7054 -e CORE_SECURITY_ENROLLID=vp0 -e CORE_SECURITY_ENROLLSECRET=vp0_secret  hyperledger/fabric-peer peer node start
```

另外，validating peer的`enrollID`和`enrollSecret`（`vp0`和`vp0_secret`）必须添加到[membersrvc.yaml](https://github.com/hyperledger/fabric/blob/master/membersrvc/membersrvc.yaml)。
-->
