# NSGo pilot

[中文](README.md)

The shortest path through VeneraCat Core on [NSGo](https://nsgo.dev/), on its own, without touching Nekularity or Nekophage.

- Go uses Core's optional `gohost` for comic and video sources. The regular profile stays under the app data directory at `core/`. The source API level is `1.6.3`.
- Browser login uses NSGo's native WebView and reads HttpOnly cookies from its native cookie store. The pilot waits for cookies, saves them together in the source partition and then calls `onLoginSuccess`.
- The iOS host registers AetherEngine as `aether.player` in the dynamic `PlayerEngineKit` framework. NSGo's native tab bar uses Liquid Glass with icons from nsgo-icons-kubai.

## Running

- On Windows, build with `go build -o pilot.exe .`. Set `NSGO_PILOT_PROBE=1` to use a temporary profile and loopback sources. Set `NSGO_PILOT_AUTO=1` to check native login, details, full image decode, both HLS playlists and a media byte range. Success is logged as `pilot: complete`.
- The iOS simulator workflow calls `go tool nsgo build -embed -target ios .`, generates the Swift host, and waits for playback time to advance. Set `NSGO_PILOT_LIVE=1` to run a live source diagnostic after the offline path.

## Cookie-backed playback

Swift's native player currently receives the playback URL and declared request headers. Video that depends on the source cookie partition requires a Go media proxy or equivalent transport integration in Swift. The transport must match partition cookies for each redirect and media URL, carry Range requests and cancel on playback closure.

## Local Core integration

The temporary replace in `go.mod` reads the shared host from the sibling `../VeneraCat Core` checkout. Before publishing the pilot, push Core first, drop the local replace and update to the Go pseudo-version of the published revision.

## Offline media fixture

`app/testdata/hls` reuses Nekophage’s fMP4 HLS clip. The picture is **Big Buck Bunny**, © 2008 Blender Foundation, [CC BY 3.0](https://creativecommons.org/licenses/by/3.0/). The derivative scales, compresses and segments the clip and has no audio track.

## License

LGPL-3.0-or-later with an App Store exception; see [LICENSE](LICENSE).

