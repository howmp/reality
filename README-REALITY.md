## reality

<https://github.com/XTLS/REALITY>

reality是安全传输层的实现，其和TLS类似都实现了安全传输，除此之外还进行TLS指纹伪装

简单来说就是：

1. 确定一个伪装服务器目标，比如https://example.com
1. 当普通客户端来访问reality服务端时，将其代理到example.com
1. 当特殊客户端来访问reality服务端时，进行特定处理流程

### reality原理

具体来说就是在客户端与伪装服务器进行TLS握手的同时，也进行了私有握手

首先reality服务端和特殊客户端预先共享一对公私密钥(x25519)

私有握手关键步骤如下:

1. 特殊客户端在Client Hello中
   1. 生成临时公私密钥对(x25519)
   1. Client Hello中将Extension的key_share修改为临时公钥
   1. 通过临时私钥与预先共享的公钥,以及hkdf算法生成authkey
   1. 通过authkey对版本号、时间戳等信息加密，并替换Client Hello中的Session ID字段
1. reality服务端收到Client Hello后
   1. 通过预先共享的私钥和Client Hello中的临时公钥，以及hkdf算法生成authkey
   1. 通过authkey解密Session ID字段，并验证时间戳、版本号信息
   1. 验证成功则生成一个临时可信证书(ed25519)
   1. 验证失败则代理到伪装服务器
1. 特殊客户端在收到reality服务端证书后
   1. 通过hmac算法和authkey计算证书签名，与收到的证书签名对比
   1. 若签名一致，进行特定处理流程
   1. 若签名不一致
      1. 但签名是example.com的真证书，则进入爬虫模式
      1. 否则发送TLS alert

<https://github.com/XTLS/Xray-core/issues/1697#issuecomment-1441215569>

### reality的特点和限制

特点：

1. 完美模拟了伪装服务器的TLS指纹
1. 特殊客户端巧妙的利用TLS1.3的key_share和Session ID字段进行私有握手
   1. 这两字段原本都是随机的，即使替换也没有特征
1. 不需要域名，也不需要证书

限制：

只能使用TLS1.3，且必须使用x25519

1. key_share是TLS1.3新增内容<https://www.rfc-editor.org/rfc/rfc8446#section-4.2.8>
1. reality服务端返回的临时证书本质上是有特征的，但TLS1.3中Certificate包是加密的，也就规避了这一问题
1. 如果伪装服务器目标不使用x25519，则私有握手无法成功


## 与原版的reality的区别

1. 使用两组预共享公私钥，分别用于密钥交换/验签，验签使用额外一次通信进行
2. 模仿站必须是tls1.2，且最好使用aead的套件
    1. TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305
    1. TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305
    1. TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256
    1. TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256
    1. TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384
    1. TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384
    1. TLS_RSA_WITH_AES_128_GCM_SHA256
    1. TLS_RSA_WITH_AES_256_GCM_SHA384
3. 服务端代码实现更简单，不需要修改tls库，用读写过滤的方式来判断是否已经握手完成