# ⚠️ 重要：Bridge 需要重启

## 问题

你遇到的问题是：**Bridge 正在运行旧版本**，没有使用新的协议系统。

### 症状
- ✅ Bridge 接收到消息
- ✅ Bridge 发送 `session:output`
- ❌ 但没有发送 `chat:response`（新协议消息）
- ❌ Web UI 看不到 AI 回复

## 解决方案（3 步）

### 方法 1：使用自动脚本（推荐）

```bash
cd bridge
./scripts/full-restart.sh
```

### 方法 2：手动操作

```bash
# 1. 停止旧 Bridge
pkill -f "open-agents start"

# 2. 重新编译
cd bridge
go build -o build/open-agents ./cmd/open-agents/

# 3. 启动新 Bridge
./build/open-agents start
```

## 验证成功

重启后，检查日志应该看到：

```bash
tail -f ~/.open-agents/logs/bridge.log
```

**旧版本日志：**
```
[SessionManager] Output event received: type=stdout, len=878
[Bridge] Forwarding session output: sessionId=xxx, type=stdout, len=878
```

**新版本日志：**
```
[SessionManager] Message received: type=content
[Bridge] Forwarding protocol message: sessionId=xxx, type=content
[Protocol] Using PTY protocol
```

## 测试

重启后，在 Web UI 中：
1. 发送消息给 AI
2. 应该看到 AI 回复显示在聊天界面
3. 不再只是终端输出

## 需要帮助？

如果还是不行，运行诊断：
```bash
cd bridge
./scripts/diagnose.sh
```
