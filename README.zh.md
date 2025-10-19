# HFBridge - Hugging Face 代理与可视化面板

[English](./README.md) | [中文](./README.zh.md)

HFBridge 是一个一体化的 Hugging Face 代理服务，内置可视化仪表盘。它支持请求透传、Large File Storage (LFS) 链接改写以及实时的使用量统计，帮助在内网或受限网络环境中的团队继续访问 Hugging Face 资源。

## 功能特性

- 透传 Hugging Face 仓库、模型与文件请求，允许自定义默认上游源
- 自动附加 Hugging Face 访问令牌，便于访问私有资源
- 重写 LFS `batch` 响应，让大文件通过代理的 `/lfs/content` 端点回源
- 将请求次数与流量写入 SQLite，并在仪表盘上实时展示
- 集成式 Web UI，用于监控使用量、仓库访问排行与流量趋势
- 支持单容器部署，也可编译为自包含的二进制

## 快速开始

仓库内已提供可直接运行的 Compose 栈，会拉取已发布的 `gouryella/hfbridge:latest` 镜像。

### 前提条件

- Docker Engine 20.10+，并安装 `docker compose` 插件（或使用 Docker Desktop）

### 使用 Docker Compose 启动

1. 克隆本仓库，并视需求修改 `docker-compose.yml` 中的环境变量（如 `DEFAULT_UPSTREAM`、`HF_TOKEN`、`PROXY_ORIGIN`）。
2. 启动服务：

   ```bash
   docker compose up -d
   ```

3. 访问 `http://localhost:8080` 打开 Web 仪表盘。
4. 排查问题时可查看日志：

   ```bash
   docker compose logs -f hfbridge
   ```

5. 不再使用时关闭服务：

   ```bash
   docker compose down
   ```

容器默认将 SQLite 数据库持久化到主机的 `./data` 目录，如需调整可在 `docker-compose.yml` 中修改卷设置。

### 可选：自行构建镜像

```bash
docker build -t hfbridge .
docker run -p 8080:8080 -v $(pwd)/data:/data hfbridge
```

如需本地编译二进制，请参考后文“开发”章节。

## 使用方式

### Web 仪表盘

访问 `http://localhost:8080`，可查看实时使用量、仓库访问排行及流量图表。

### 通过代理克隆仓库

```bash
# 克隆公开仓库
git clone http://localhost:8080/google/bert

# 大文件会自动通过代理下载
git clone http://localhost:8080/runwayml/stable-diffusion-v1.5
```

### 让 Hugging Face 工具走代理

服务启动后，可通过 `http://<host>:8080` 访问代理。将现有工具指向该地址即可让所有流量经由 HFBridge：

- **huggingface-hub / transformers（Python）**  
  在使用 `huggingface-cli` 或加载模型前设置环境变量：

  ```bash
  export HF_ENDPOINT=http://localhost:8080
  export HF_TOKEN=hf_你的访问令牌_如需私有资源
  huggingface-cli whoami
  ```

  之后依赖 `huggingface_hub` 或 `transformers` 的脚本将会通过代理下载模型与数据集。

- **Git 客户端**  
  想要自动通过代理克隆任意仓库，可设置 Git 重写规则：

  ```bash
  git config --global url."http://localhost:8080/".insteadOf "https://huggingface.co/"
  ```

  此后访问 `https://huggingface.co/...` 的 Git 命令（含 Git LFS）都会改走 HFBridge。

- **直接发起 HTTP 请求**  
  在脚本中将 `https://huggingface.co` 替换为 `http://localhost:8080` 即可。例如：

  ```bash
  curl http://localhost:8080/api/models/gpt2
  ```

### API 端点

```bash
# 健康检查
curl http://localhost:8080/v1/healthz

# 查看当前使用量（JSON）
curl http://localhost:8080/v1/usage

# 订阅实时使用流（SSE）
curl http://localhost:8080/v1/usage/stream
```

## 配置项

所有配置均可通过环境变量设置。

| 环境变量           | 是否必填 | 默认值                                                               | 说明 |
|--------------------|----------|------------------------------------------------------------------------|------|
| `BIND_ADDR`        | 否       | `:8080`                                                               | HTTP 监听地址 |
| `DEFAULT_UPSTREAM` | 否       | `https://huggingface.co`                                              | 当请求路径为相对路径时使用的默认上游 |
| `PROXY_ORIGIN`     | 否       | 自动检测                                                              | 用于改写 LFS 链接的代理外部访问地址，如自动推断不正确可自行覆盖 |
| `ALLOW_HOSTS`      | 否       | `huggingface.co,cdn-lfs.huggingface.co,*.huggingface.co,*.hf.co`      | 允许透传与 LFS 下载的上游主机列表 |
| `HF_TOKEN`         | 否       | 空                                                                    | Hugging Face 访问令牌，用于访问私有资源 |
| `DB_PATH`          | 否       | `/data/hfbridge.db`                                                   | 保存请求日志与统计数据的 SQLite 文件路径 |

## 架构说明

HFBridge 通过单个二进制打包了代理后端与 Web UI。

- **前端**：基于 Next.js 构建的仪表盘，编译后以静态文件形式嵌入 Go 二进制
- **后端**：Go 语言实现的反向代理，提供 API 端点与静态文件服务
- **数据库**：SQLite 用于记录请求日志与聚合统计数据

路由概览：

- `/` 提供仪表盘首页
- `/_next/*`、`/icons/*`、`/favicon.ico` 等路径提供静态资源
- `/v1/usage`、`/v1/usage/stream`、`/v1/healthz` 暴露 API 接口
- 其他路径（如 `/google/bert`）会被转发到配置的上游

## 开发

```
hfbridge/
├── frontend/          # Next.js 前端
│   ├── app/           # App Router 页面
│   ├── components/    # React 组件
│   └── out/           # 静态构建输出
├── backend/           # Go 后端
│   ├── cmd/hfbridge/  # 主程序入口
│   └── internal/      # 内部包
│       └── server/    # HTTP 服务器与静态资源
├── Dockerfile         # 多阶段构建
└── docker-compose.yml
```

### 本地开发流程

```bash
# 前端开发（热更新）
cd frontend
pnpm install
pnpm dev

# 后端开发（需要先构建一次前端）
cd backend
go run ./cmd/hfbridge/
```

如需生成自包含二进制：

1. 构建前端（`pnpm build`）并将静态文件复制到 `backend/internal/server/static`。
2. 编译后端：`go build -o hfbridge ./cmd/hfbridge/`。
3. 运行 `./hfbridge`，在浏览器访问 `http://localhost:8080`。

## License

GNU GPLv3（详见 `LICENSE`）
