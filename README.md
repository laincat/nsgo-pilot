# NSGo 试点

[English](README.en.md)

用 [NSGo](https://nsgo.dev/) 单独验证 VeneraCat Core 的最短主线，不改 Nekularity 和 Nekophage。

- Core 直接编进 App：源列表与安装、搜索、详情、大图阅读器。
- 网页登录：登录页由 NSGo 的系统网页视图显示，Cookie（含 HttpOnly）由 Core 的 `webcookie` 包从系统存储读取（Windows：WebView2；iOS：WKWebsiteDataStore），写入源的分区后再调用 `onLoginSuccess`。
- iOS 播放：`ios/` 是 Swift 外壳，把 AetherEngine 注册成 `aether.player` 原生组件。引擎装在独立的动态框架 `PlayerEngineKit` 里，App 只在运行期链接它：AetherEngine 是 LGPL-3.0 且官方要求动态链接，这样换一个改过的引擎不用重编 App。
- 底部浮动标签栏用 Liquid Glass 材质（iOS 26 / macOS 26 起生效），图标来自 nsgo-icons-kubai。

## 运行

- Windows：`go build -o pilot.exe .`；设置 `NSGO_PILOT_PROBE=1` 会额外安装本机回环探针源（登录页、漫画、HLS 视频），`NSGO_PILOT_AUTO=1` 会自动走完整条路径并在日志里输出 `pilot:` 行。
- iOS 模拟器：见 `.github/workflows/ios-simulator.yml`（`xcode-27` 镜像）：`go tool nsgo build -embed -target ios .` 产出 XCFramework，XcodeGen 生成外壳工程，启动模拟器自动驾驶并截图。

## 许可

LGPL-3.0-or-later，附 App Store 例外，见 [LICENSE](LICENSE)。

