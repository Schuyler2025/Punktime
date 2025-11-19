# Punktime - 简约计时器应用

Punktime 是一个基于 Wails 框架开发的桌面计时器应用，提供简洁的时间显示和倒计时功能。

## 功能特性

- 🕐 双模式显示：时间模式 / 倒计时模式
- ⏱️ 倒计时功能：支持自定义时长，暂停/继续
- 🎯 便携操作：支持快捷键完成所有操作
- 🎨 简约界面：半透明模糊背景效果

## 环境安装

```bash
# 下载并安装 Go
https://golang.org/dl/

# 下载并安装 Node.js
https://nodejs.org/

# 安装 Wails CLI
go install github.com/wailsapp/wails/v2/cmd/wails@latest
```

## 运行项目

```bash
# 开发模式运行（热重载）
wails dev

# 构建应用
wails build
```

## 技术栈

- **后端**: Go + Wails框架
- **前端**: HTML/CSS/JavaScript
- **系统集成**: systray托盘支持

## 项目结构

Punktime/
├── main.go # 主程序逻辑
├── tray.go # 系统托盘
├── frontend/ # 前端界面
└── pkg/ # 内部包（日志、配置）