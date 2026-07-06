# Protobuf 生成说明 / Protobuf Generation Guide

本文档说明如何从 `protobuf/new_douyin.proto` 重新生成 Go 代码。
This guide explains how to regenerate Go code from `protobuf/new_douyin.proto`.

## 目录说明 / Directory Layout

- `new_douyin.proto`：当前项目实际使用的抖音直播 protobuf 定义。
- `webcast.data.proto`、`webcast/data/*.proto`：历史或参考定义，当前默认生成流程不会使用。
- `../generated/new_douyin/new_douyin.pb.go`：由 `new_douyin.proto` 生成的 Go 代码，不建议手动修改。
- `../generated/messagepool.go`、`../generated/struct.go`：项目维护的消息实例池和消息类型映射，不由 `protoc` 自动生成。

## 环境要求 / Requirements

需要安装两个工具：

- `protoc`：Protocol Buffers 编译器。
- `protoc-gen-go`：Go 代码生成插件。

### 安装 protoc-gen-go

```powershell
go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
```

确认 `$GOPATH/bin` 或 `$HOME/go/bin` 已经在 `PATH` 中：

```powershell
protoc-gen-go --version
```

### 安装 protoc

Windows 可以使用 `winget` 查询可用包，或手动下载：

```powershell
winget search protobuf
winget install --id <查询到的 protobuf 包 ID>
```

如果 `winget` 找不到合适的包，可以从 Protocol Buffers Release 页面下载对应系统的压缩包，并把其中的 `bin` 目录加入 `PATH`。

验证安装：

```powershell
protoc --version
```

## 生成命令 / Generation Commands

推荐在仓库根目录执行：

```powershell
make proto
```

等价命令是：

```powershell
protoc --proto_path=protobuf --go_out=. protobuf/new_douyin.proto
```

如果当前目录在 `protobuf/` 下，也可以使用：

```cmd
new_pb.cmd
```

生成后的文件位置：

```text
generated/new_douyin/new_douyin.pb.go
```

## 修改 proto 后的流程 / Workflow After Editing Proto

1. 修改 `protobuf/new_douyin.proto`。
2. 运行 `make proto` 重新生成 Go 代码。
3. 如果新增或改名了消息类型，同步检查 `generated/struct.go` 和 `generated/messagepool.go` 是否需要补充映射。
4. 运行测试和编译：

```powershell
go test ./...
go build ./...
```

## 常见问题 / Troubleshooting

### `protoc-gen-go: program not found or is not executable`

说明 `protoc-gen-go` 没有安装，或者 Go 的 bin 目录没有加入 `PATH`。

处理方式：

```powershell
go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
```

然后确认以下目录之一在 `PATH` 中：

```text
%USERPROFILE%\go\bin
```

### 生成路径不对

`new_douyin.proto` 内部配置了：

```proto
option go_package = "generated/new_douyin/";
```

所以必须从仓库根目录使用：

```powershell
protoc --proto_path=protobuf --go_out=. protobuf/new_douyin.proto
```

这样生成结果才会落到 `generated/new_douyin/`。

### 新消息没有被 WebSocket 输出

`protoc` 只负责生成 protobuf 结构体。项目实际解析消息时还依赖 `generated.GetMessageInstance` 的消息类型映射。

如果新增了 `WebcastXXXMessage`，需要检查：

- `generated/struct.go` 是否包含新消息的 method 到结构体映射。
- `generated/messagepool.go` 是否需要增加对象池逻辑。
- 根目录 `struct.go` 是否需要新增公开的消息类型常量。

## 注意事项 / Notes

- 不要手动编辑 `generated/new_douyin/new_douyin.pb.go`，应该改 `.proto` 后重新生成。
- 默认只生成 `new_douyin.proto`，不要无脑把旧的 `webcast.data.proto` 合进去。
- 提交前建议检查 `git diff`，确认只有预期的 proto 和生成文件发生变化。
