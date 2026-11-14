// The pilot's iOS host: the NSGo app fills the window, and PlayerEngineKit
// plays what the app's play page shows as its "aether.player" Native element.
import NsgoPilot
import PlayerEngineKit
import UIKit

/// The app's player element: props url and headers (name to value); it emits
/// "state" with the engine's state and "error" with a failed load's message.
/// NSGo calls it on the main thread.
final class AetherComponent: NSObject, NSGoComponent {
    let context: NSGoComponentContext
    private var player: PilotPlayer?
    private var url: String?
    private var headers: [String: String] = [:]
    private var loaded: String?
    private var scheduled = false

    init(_ context: NSGoComponentContext) {
        self.context = context
        super.init()
    }

    var view: UIView {
        MainActor.assumeIsolated { make()?.view ?? UIView() }
    }

    private func make() -> PilotPlayer? {
        if let player { return player }
        return MainActor.assumeIsolated { () -> PilotPlayer? in
            guard let player = try? PilotPlayer() else { return nil }
            player.onState = { [weak self] state in
                print("pilot: aether state \(state)")
                self?.context.emit("state", state)
            }
            player.onError = { [weak self] message in
                print("pilot: aether error \(message)")
                self?.context.emit("error", message)
            }
            self.player = player
            return player
        }
    }

    func setProp(_ name: String, value: Any?) {
        switch name {
        case "url": url = value as? String
        case "headers": headers = (value as? [String: Any])?.compactMapValues { $0 as? String } ?? [:]
        default: return
        }
        // The props of one update arrive one by one: load once they are all in.
        guard !scheduled else { return }
        scheduled = true
        DispatchQueue.main.async { [weak self] in
            self?.scheduled = false
            self?.start()
        }
    }

    private func start() {
        guard let url, url != loaded, let address = URL(string: url), let player = make() else { return }
        loaded = url
        let headers = headers
        MainActor.assumeIsolated { player.load(url: address, headers: headers) }
    }

    func destroy() {
        MainActor.assumeIsolated { [player] in
            player?.stop()
        }
        player = nil
    }
}

final class HostController: UIViewController {
    let app = NSGoView(frame: .zero)

    override func viewDidLoad() {
        super.viewDidLoad()
        let support = FileManager.default.urls(for: .applicationSupportDirectory, in: .userDomainMask)[0]
        app.dataDirectory = support.path
        app.onError = { message in print("pilot: app failed: \(message)") }
        app.register(component: "aether.player") { context in AetherComponent(context) }
        app.frame = view.bounds
        app.autoresizingMask = [.flexibleWidth, .flexibleHeight]
        view.addSubview(app)
    }

    override var preferredStatusBarStyle: UIStatusBarStyle { app.preferredStatusBarStyle }
}

final class AppDelegate: UIResponder, UIApplicationDelegate {
    func application(_ application: UIApplication,
                     didFinishLaunchingWithOptions options: [UIApplication.LaunchOptionsKey: Any]?) -> Bool {
        NSGoStart()
        return true
    }
}

@objc(HostSceneDelegate)
final class SceneDelegate: UIResponder, UIWindowSceneDelegate {
    var window: UIWindow?

    func scene(_ scene: UIScene, willConnectTo session: UISceneSession, options: UIScene.ConnectionOptions) {
        guard let scene = scene as? UIWindowScene else { return }
        window = UIWindow(windowScene: scene)
        window?.rootViewController = HostController()
        window?.makeKeyAndVisible()
    }
}

UIApplicationMain(CommandLine.argc, CommandLine.unsafeArgv, nil, NSStringFromClass(AppDelegate.self))

