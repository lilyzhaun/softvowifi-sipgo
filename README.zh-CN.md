# softvowifi-sipgo

[English](README.md) · **简体中文**

基于[emiago/sipgo](https://github.com/emiago/sipgo) v1.4.3保留源码的独立公开维护仓，
导入路径仍为`github.com/emiago/sipgo`及其`/sip`子包。不是完整上游镜像或官方Release，
也不是完整IMS、SIM鉴权、IPsec、RTP/编解码或VoWiFi产品。

## 维护内容

- TCP分块时保留逻辑头部/续行，完整缓冲解析语义不变。
- 分消息的部分帧长度预算、溢出保护及正确的keepalive边界。
- 可选`WithTransportLayerRequireExistingConnection()`：连接缓存未命中时拒绝新建；
  原默认行为不变。
- 在关闭事务Done前发布终态Err，终止回调可安全读取Err；客户端显式终止为
  ErrTransactionCanceled，服务端保持ErrTransactionTerminated。

上游测试仅保留部分，加上聚焦回归；不能拿本仓CI代替完整上游/运营商验收。
Go模块声明1.23.0、工具链1.24.2；CI使用Linux/Go1.26。

## 安装与验证

在已有Go模块中保持原导入路径，并明确版本限定的replace：

```sh
go mod edit -replace=github.com/emiago/sipgo@v1.4.3=github.com/lilyzhaun/softvowifi-sipgo@v0.0.0-20260916002955-f30e2a8e5daa
go get github.com/emiago/sipgo@v1.4.3
go list -m -json github.com/emiago/sipgo
```

如果版本选择结果不再是v1.4.3，上述限定replace不会覆盖其他版本；以实际Replace
读回为准，不自动跟随main。英文README提供可编译的离线合成SIP解析示例。

```sh
go test -p 1 -race -shuffle=on -count=1 -timeout=120s ./...
go vet -p 1 -composites=false ./...
```

完整vet仍有client.go旧HeaderKV未具名字面量警告。CI关闭整个composites分析器，
其余默认分析照常执行；旧格式只报告、不重写，不能宣称全vet绿色。

## 参与与许可

流程见[贡献指南](CONTRIBUTING.md)，非漏洞使用
[issue模板](https://github.com/lilyzhaun/softvowifi-sipgo/issues/new/choose)，
漏洞按[安全策略](SECURITY.md)私密报告。请勿提交真实SIP凭据、号码、短信正文、
鉴权材料、完整配置或原始抓包。

保留[BSD-2-Clause原文及Emir Aganovic版权](LICENSE)。
[PROVENANCE](PROVENANCE.md)与[旧README](docs/README-imported.md)保留版本、
原始摘要、补丁和gosip/Pion归属说明，不以旧私有迁移状态冒充当前情况。
