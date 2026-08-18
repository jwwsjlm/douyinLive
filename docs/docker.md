# Docker 部署

[返回项目首页](../README.md)

本文介绍 Docker、Docker Compose、配置挂载、长期运行和健康检查。

## 1. 直接启动最新版镜像

```bash
docker run --rm -p 1088:1088 ghcr.io/jwwsjlm/douyinlive:latest
```

程序启动后，对外提供的 WebSocket 地址仍然是：

```text
ws://127.0.0.1:1088/ws/直播间标识
```

如果你需要固定版本，也可以直接拉指定 tag：

```bash
docker run --rm -p 1088:1088 ghcr.io/jwwsjlm/douyinlive:v2.1.0
```

测试版不会覆盖 `latest`。如果你要验证某个测试版，请把下面的 `<tag>` 替换为实际发布的 beta tag：

```bash
docker pull ghcr.io/jwwsjlm/douyinlive:<tag>
docker run --rm -p 1088:1088 ghcr.io/jwwsjlm/douyinlive:<tag>
```

Docker 镜像也支持查看构建信息：

```bash
docker run --rm ghcr.io/jwwsjlm/douyinlive:v2.1.0 --version
```

如果要使用 TikHub 在线签名，仍然使用同一个镜像，只需要在配置文件、环境变量或命令行里指定签名来源并提供 TikHub API Key：

```bash
docker run --rm -p 1088:1088 \
  -e APP_SIGN_PROVIDER=tikhub \
  -e APP_TIKHUB_KEY=YOUR_TIKHUB_KEY \
  ghcr.io/jwwsjlm/douyinlive:v2.1.0
```

## 2. 通过 Docker 挂载 `config.yaml`

如果你希望加载自定义配置，先在宿主机准备一个 `config.yaml`，再把它挂载到容器中的 `/app/config.yaml`，并通过 `--config` 显式传入：

```bash
docker run --rm -p 1088:1088 \
  -v $(pwd)/config.yaml:/app/config.yaml:ro \
  ghcr.io/jwwsjlm/douyinlive:latest --config /app/config.yaml
```

说明：

- `-v $(pwd)/config.yaml:/app/config.yaml:ro`：把宿主机当前目录下的 `config.yaml` 挂载到容器内
- `:ro`：只读挂载，避免容器误改宿主机配置
- `--config /app/config.yaml`：显式指定程序读取这个配置文件

## 3. 持久化运行（推荐长期使用）

如果你希望容器长期后台运行，不要使用 `--rm`，建议改成：

```bash
docker run -d \
  --name douyinlive \
  --restart unless-stopped \
  -p 1088:1088 \
  -v $(pwd)/config.yaml:/app/config.yaml:ro \
  ghcr.io/jwwsjlm/douyinlive:latest --config /app/config.yaml
```

这样即使容器被删除或重建，宿主机上的 `config.yaml` 仍然保留，达到配置持久化的效果。

镜像内置健康检查，也可以手动查看：

```bash
docker inspect --format '{{json .State.Health}}' douyinlive
```

镜像健康检查只探测容器内的 TCP `1088` 端口是否正在监听，不提供额外的 HTTP 健康接口，也不代表某个具体直播间一定处于开播状态。

## 4. 挂载整个目录（适合后续扩展）

如果你后续不只想挂一个配置文件，也可以直接挂整个目录：

```bash
mkdir -p ./data
cp config.example.yaml ./data/config.yaml

docker run -d \
  --name douyinlive \
  --restart unless-stopped \
  -p 1088:1088 \
  -v $(pwd)/data:/app/data \
  ghcr.io/jwwsjlm/douyinlive:latest --config /app/data/config.yaml
```

这种方式更适合统一管理容器运行时使用到的文件。

## 5. 使用 Docker Compose（推荐）

项目已自带两个 compose 示例文件：

- `compose.yaml`：挂载单个 `config.yaml`
- `compose.data.yaml`：挂载整个 `data` 目录

### 方案 A：使用 `compose.yaml`

先准备配置文件：

```bash
cp config.example.yaml config.yaml
```

然后直接启动：

```bash
docker compose up -d
docker compose logs -f
docker compose down
```

`compose.yaml` 内容如下：

```yaml
services:
  douyinlive:
    image: ghcr.io/jwwsjlm/douyinlive:latest
    container_name: douyinlive
    restart: unless-stopped
    ports:
      - "1088:1088"
    volumes:
      - ./config.yaml:/app/config.yaml:ro
    command: ["--config", "/app/config.yaml"]
    healthcheck:
      test: ["CMD-SHELL", "nc -z -w 2 127.0.0.1 1088 || exit 1"]
      interval: 30s
      timeout: 3s
      start_period: 10s
      retries: 3
```

### 方案 B：使用 `compose.data.yaml`

如果你想把配置统一收纳到目录里，先执行：

```bash
mkdir -p data
cp config.example.yaml data/config.yaml
```

然后用下面命令启动：

```bash
docker compose -f compose.data.yaml up -d
docker compose -f compose.data.yaml logs -f
docker compose -f compose.data.yaml down
```

`compose.data.yaml` 内容如下：

```yaml
services:
  douyinlive:
    image: ghcr.io/jwwsjlm/douyinlive:latest
    container_name: douyinlive
    restart: unless-stopped
    ports:
      - "1088:1088"
    volumes:
      - ./data:/app/data
    command: ["--config", "/app/data/config.yaml"]
    healthcheck:
      test: ["CMD-SHELL", "nc -z -w 2 127.0.0.1 1088 || exit 1"]
      interval: 30s
      timeout: 3s
      start_period: 10s
      retries: 3
```

此时你只需要保证宿主机存在：

```text
./data/config.yaml
```

## 6. 常用查看命令

```bash
docker logs -f douyinlive
docker ps
docker stop douyinlive
docker rm -f douyinlive
```

---
