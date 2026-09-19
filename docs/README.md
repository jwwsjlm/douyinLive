# 使用文档

[返回项目首页](../README.md)

README 只保留项目介绍和最短上手路径，详细使用说明按主题拆分如下：

| 文档 | 内容 |
| --- | --- |
| [架构与维护入口](architecture.md) | 服务、根库和签名组件的职责、调用流与代码入口 |
| [Docker 部署](docker.md) | Docker、Compose、配置挂载、长期运行和 HTTP 健康检查 |
| [CLI 使用指南](cli.md) | 启动参数、直播间标识、日志、签名方式和故障排查 |
| [配置文件](configuration.md) | YAML、环境变量、Cookie、TikHub 和配置优先级 |
| [HTTP API](http-api.md) | 直播间与主播资料查询、批量状态、URL 解析、认证和 Prometheus 指标 |
| [OpenAPI](openapi.yaml) | HTTP API 的机器可读接口定义 |
| [作为 Go 库使用](library.md) | 状态检查、消息订阅、protobuf、关闭和资源释放 |
| [WebSocket 客户端与消息格式](websocket-client.md) | 客户端示例、状态码、业务消息和重连建议 |
| [`sign` 包说明](../sign/README.md) | `a_bogus` 签名和 CookieManager 的直接调用 |

示例文件的提交规则见 [`examples/README.md`](../examples/README.md)。
