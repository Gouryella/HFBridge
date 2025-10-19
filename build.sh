#!/bin/bash
set -e

# 获取当前脚本所在目录（而不是执行时的工作目录）
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"

echo "🏗️  开始构建 HFBridge..."

# 构建前端
echo "📦 正在构建前端..."
cd "$SCRIPT_DIR/frontend"
pnpm install --frozen-lockfile
pnpm build

# 复制前端静态文件到 backend/static/web 供 go:embed 使用
echo "📁 正在复制静态文件..."
rm -rf "$SCRIPT_DIR/backend/static/web"
mkdir -p "$SCRIPT_DIR/backend/static/web"
cp -a "$SCRIPT_DIR/frontend/out/." "$SCRIPT_DIR/backend/static/web/"
touch "$SCRIPT_DIR/backend/static/web/.gitkeep"

# 构建后端
echo "🔨 正在构建后端..."
cd "$SCRIPT_DIR/backend"
go build -o "$SCRIPT_DIR/hfbridge" ./cmd/hfbridge/

echo "✅ 构建完成！"
echo ""
echo "可执行文件已生成：$SCRIPT_DIR/hfbridge"
echo ""
echo "运行以下命令启动服务："
echo "  ./hfbridge"
echo ""
echo "或者使用 Docker："
echo "  docker-compose up -d"
