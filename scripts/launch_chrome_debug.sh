#!/bin/bash
# 启动带有远程调试端口 (9222) 的 Chrome，并指定独立的用户数据目录保留登录态
PROFILE_DIR="$HOME/.chrome-x-debug-profile"
mkdir -p "$PROFILE_DIR"

echo "=================================================="
echo "🌐 正在启动 Chrome (调试端口 9222)..."
echo "📁 数据目录: $PROFILE_DIR"
echo "👉 请在打开的浏览器窗口中登录 X (Twitter)。"
echo "=================================================="

"/Applications/Google Chrome.app/Contents/MacOS/Google Chrome" \
  --remote-debugging-port=9222 \
  --user-data-dir="$PROFILE_DIR" \
  --no-first-run \
  --no-default-browser-check \
  "https://x.com" &
