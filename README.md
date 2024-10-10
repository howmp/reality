# grs

1. grss(Golang Reverse SOCKS5 Server) 服务端，需要有公网IP的机器上
1. grsc(Golang Reverse SOCKS5 Client) 客户端，需要运行于想要穿透的内网中机器上
1. grsu(Golang Reverse SOCKS5 User) 用户端，需要运行于用户机器上，提供socks5服务


grs是一个反向socks5代理,其中grss和grsc和grsu是通过REALITY协议通信

关于REALITY协议: [README-REALITY.md](./README-REALITY.md)

相对于frp，nps等内网穿透工具有以下特点

1. 完美消除网络特征
1. 防止服务端被主动探测
1. 客户端和用户端内嵌配置，不需要命令行或额外配置文件

## 使用步骤

### 生成配置、客户端、用户端

`grss gen www.qq.com:443 127.0.0.1:443`

1. `www.qq.com:443` 是被模拟的目标
1. `127.0.0.1:443` 是服务器监听地址，这里要填写公网IP，端口最好和模拟目标一致

若SNIAddr或ServerAddr不指定，则尝试加载已有配置文件

```txt
Usage:
  grss [OPTIONS] gen [gen-OPTIONS] [SNIAddr] [ServerAddr]

generate server config and client

Help Options:
  -h, --help                                                 Show this help message

[gen command options]
      -d                                                     debug
      -f=[chrome|firefox|safari|ios|android|edge|360|qq]     client finger print (default: chrome)
      -e=                                                    expire second (default: 30)
      -o=                                                    server config output path (default: config.json)
          --dir=                                             client output directory (default: .)

[gen command arguments]
  SNIAddr:                                                   tls server address, e.g. example.com:443
  ServerAddr:                                                server address, e.g. 8.8.8.8:443
```

### 启动服务端

`grss serv`

```txt
Usage:
  grss [OPTIONS] serv [serv-OPTIONS]

run server

Help Options:
  -h, --help      Show this help message

[serv command options]
      -o=         server config path (default: config.json)
```

### 启动客户端

`grsc`

### 启动用户端

`grsu`

```txt
Usage of grsu:
  -l string
        socks5 listen address (default "127.0.0.1:61080")
```