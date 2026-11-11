# 贡献指南

欢迎通过 Issue 反馈问题，或提交 Pull Request。

## 本地开发

使用 `go.mod` 声明的 Go 版本，在项目根目录运行：

```sh
go run ./cmd/v2term
go test -race ./...
go vet ./...
go build -o /tmp/v2term ./cmd/v2term
```

修改的 Go 文件需要经过 `gofmt`。CI 会检查格式、竞态测试、静态分析和构建。

测试默认使用本机 HTTP 服务，不会向真实账号发送回复或感谢。可选的公开页面检查需要联网：

```sh
V2TERM_LIVE=1 go test ./internal/v2ex -run TestLivePublicPages -v
```

## 代码结构

| 目录 | 职责 |
| --- | --- |
| `cmd/v2term` | 命令、配置入口与程序生命周期 |
| `internal/tui` | 事件循环、布局和键鼠交互 |
| `internal/v2ex` | HTTP 会话、页面解析和对话关联 |
| `internal/preview` | 图片加载、缩略图和终端图像协议 |
| `internal/config` | 配置默认值、校验与持久化 |
| `internal/state` | 阅读进度、导航历史与草稿 |
| `internal/storage` | 私有文件的原子写入 |

网络请求应可取消且有超时，不在 UI 更新中执行阻塞 I/O。页面结构依赖留在 `internal/v2ex`，远端文本进入终端前清理控制字符。写操作必须由用户显式触发，不自动重试。

## 提交改动

- 说明解决的问题、用户可见变化和验证方式，保持改动集中。
- 网络协议、会话恢复及交互状态变化需添加针对性测试。
- 功能或配置变化时同步更新中文使用说明。
- 不提交 Cookie、阅读状态、私人草稿、构建产物或本机配置。

反馈问题时请附上系统、终端及版本、复现步骤和必要截图；移除账号凭据及私人内容。安全问题见 [安全说明](SECURITY.md)。

## 构建与发布

CI 会运行测试、静态检查，并通过 GoReleaser 交叉构建 macOS、Linux、Windows 的 amd64／arm64 版本。测试构建可从 Actions 下载，不会发布正式版本。

安装 GoReleaser v2.20.1 后，可以在本地验证打包：

```sh
goreleaser check
goreleaser release --snapshot --clean
python3 scripts/package-metadata.py v0.0.0-dev
```

推送 `v*.*.*` 版本标签会自动验证并发布 GitHub Release，包含六个平台的归档、两个 Linux `.deb` 包、SHA256 校验和、Homebrew 公式和 WinGet 清单。`--version` 显示构建时注入的发行版本；源码直接运行显示 `dev`。

Homebrew 通过 [自有 tap](https://github.com/synrise25/homebrew-tap) 分发，定时同步最新稳定版公式，不需要在主仓库保存跨仓库写入令牌。tap 可以容纳多个项目，每个项目使用独立公式和更新流程。

APT 软件源托管在 GitHub Pages，发布稳定版后自动下载并校验 `.deb`，生成 amd64／arm64 索引并签名。索引选择最新版本，历史包保留在 pool 中，兼容客户端缓存的旧索引；部署后按预期版本执行真实安装验证。公钥位于 `packaging/apt/v2term.asc`，私钥仅通过 GitHub Secret `APT_SIGNING_KEY` 提供；CI 不使用真实签名私钥。需要重建时手动运行「APT 软件源」工作流。

WinGet 清单需要通过社区审核后才能从默认源安装。清单生成采用社区当前的 1.28 schema，提交收录 PR 前需核对版本、架构、下载地址和发行文件校验和。
