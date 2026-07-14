---
status: accepted
---
# Go 单二进制与按需 Web UI

Summer Harness v2 使用 Go 共享 Kernel/CLI，把 React/Vite 静态资源嵌入单一 `summer` binary；只有 `summer ui` 才启动 loopback Web UI、Watcher 和 SQLite。相比 Python 打包、Electron 或 Rust+Go 双技术栈，这一选择同时降低安装成本和默认运行成本，并为后续 Wails 桌面 Adapter 保留同一 Core 与前端。
