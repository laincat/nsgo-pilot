# NSGo 试点

[English](README.en.md)

用 [NSGo](https://nsgo.dev/) 单独验证 VeneraCat Core 的最短主线，不改 Nekularity 和 Nekophage。

- Go 端通过 Core 的 `gohost` 装配漫画源和视频源，普通资料仍使用应用资料目录下的 `core/`。源 API 级别为 `1.6.3`。
- 网页登录由 NSGo 的原生 WebView 提供。试点从原生 Cookie 存储读取 HttpOnly Cookie，等待 Cookie 到齐，汇总写入源分区后执行 `onLoginSuccess`。
- iOS 外壳把 AetherEngine 注册为 `aether.player`，使用独立的 `PlayerEngineKit` 动态框架。NSGo 原生标签栏使用 Liquid Glass，图标来自 nsgo-icons-kubai。

## 运行

- Windows 用 `go build -o pilot.exe .` 构建。设置 `NSGO_PILOT_PROBE=1` 使用临时资料与回环源。设置 `NSGO_PILOT_AUTO=1` 依次验证网页登录、详情、真实图片解码与 HLS 清单和媒体字节范围。日志以 `pilot: complete` 表示成功。
- iOS 模拟器用 `go tool nsgo build -embed -target ios .` 构建 XCFramework，由工作流生成 Swift 外壳。播放时间前进后才通过。设置 `NSGO_PILOT_LIVE=1` 可在离线流程结束后额外诊断真实源。

## 带 Cookie 的播放

Swift 直连播放器目前接收播放 URL 和声明的请求头。需要源分区 Cookie 的视频在接通 Go 媒体代理或等价传输前，仍需同步 Swift 侧契约。每一跳及每段媒体均须按目标 URL 匹配分区 Cookie，转发 Range 并在播放关闭时取消请求。

## Core 本地联调

`go.mod` 临时从同级目录 `../VeneraCat Core` 读取共享宿主。发布时先推送 Core，再移除本地 replace，并更新依赖到已发布提交的 Go 伪版本。

## 离线媒体夹具

`app/testdata/hls` 复用 Nekophage 的 fMP4 HLS 片段。画面来自 **Big Buck Bunny**，© 2008 Blender Foundation，[CC BY 3.0](https://creativecommons.org/licenses/by/3.0/)。片段做了缩放、压缩和分段，无音轨。

## 许可

LGPL-3.0-or-later，附 App Store 例外，见 [LICENSE](LICENSE)。

