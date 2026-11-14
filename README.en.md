# NSGo pilot

[中文](README.md)

The shortest path through VeneraCat Core on [NSGo](https://nsgo.dev/), on its own, without touching Nekularity or Nekophage.

- Core compiled into the app: the source list and installs, search, details and a full-size reader.
- Browser login: NSGo's system web view shows the login page; Core's `webcookie` package reads the cookies, HttpOnly ones included, from the system store (WebView2 on Windows, WKWebsiteDataStore on iOS) into the source's partition before `onLoginSuccess`.
- Playback on iOS: `ios/` is the Swift host that registers AetherEngine 7.66.1 as the `aether.player` native component. The engine sits in `PlayerEngineKit`, a dynamic framework of its own that the app links at run time: AetherEngine is LGPL-3.0 and ships dynamically linked, so a modified engine replaces it without rebuilding the app.
- The floating tab bar uses Liquid Glass (from iOS 26 / macOS 26), with icons from nsgo-icons-kubai.

## Running

- Windows: `go build -o pilot.exe .`; `NSGO_PILOT_PROBE=1` also installs loopback probe sources (a login page, a comic, an HLS episode), and `NSGO_PILOT_AUTO=1` walks the whole path by itself, logging `pilot:` lines.
- iOS Simulator: `.github/workflows/ios-simulator.yml` on the `xcode-27` image builds the XCFramework with `go tool nsgo build -embed -target ios .`, generates the host with XcodeGen, and runs the path in a simulator with screenshots.

## License

LGPL-3.0-or-later with an App Store exception; see [LICENSE](LICENSE).

