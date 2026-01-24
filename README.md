# Open Agents Bridge

Local Bridge 程序，连接 AI CLI 工具与 Open Agents 云端服务。

## 功能

- 连接 Kiro、Claude、Cline、Codex、Gemini 等 AI CLI
- WebSocket 实时通信
- 端到端加密
- 权限请求转发
- 多会话管理
- 跨平台支持 (Windows, Linux, macOS)

## 安装

### 从源码构建

```bash
cd bridge
make build
```

### 安装到系统

```bash
make install
```

## 使用

### 配对设备

```bash
# 1. 在 Web 端点击"添加设备"获取配对码
# 2. 运行配对命令
open-agents pair
```

### 启动 Bridge

```bash
# 前台运行
open-agents start

# 查看状态
open-agents status
```

### 安装为系统服务

```bash
# 安装服务
open-agents service install

# 启动服务
open-agents service start

# 停止服务
open-agents service stop

# 卸载服务
open-agents service uninstall
```

## 配置文件

配置文件位置：
- Windows: `%APPDATA%\open-agents\config.json`
- Linux/macOS: `~/.open-agents/config.json`

```json
{
  "userId": "user_xxx",
  "deviceId": "device_xxx",
  "deviceToken": "token_xxx",
  "serverUrl": "wss://open-agents-realtime.workers.dev"
}
```

## 支持的 CLI 工具

| CLI | 状态 |
|-----|------|
| Kiro | ✅ 支持 |
| Cline | ✅ 支持 |
| Claude | 🚧 开发中 |
| Codex | 🚧 开发中 |
| Gemini | 🚧 开发中 |

## 开发

```bash
# 下载依赖
make deps

# 构建
make build

# 运行测试
make test

# 构建所有平台
make build-all
```

## 项目结构

```
bridge/
├── cmd/open-agents/     # CLI 入口
├── internal/
│   ├── adapter/         # CLI 适配器
│   ├── bridge/          # 核心 Bridge 逻辑
│   ├── config/          # 配置管理
│   └── session/         # 会话管理
├── go.mod
├── Makefile
└── README.md
```

## License

MIT
