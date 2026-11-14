// The pilot's engine, in a dynamic framework of its own: the app links it at
// run time instead of compiling the engine in. AetherEngine is LGPL-3.0, and
// Apple's guidance for it is a dynamic link, so a modified engine replaces this
// one without rebuilding the app.
import AetherEngine
import Combine
import UIKit

/// One engine for the app's lifetime, as AetherEngine asks, and its view.
@MainActor
public final class PilotPlayer {
    public let view: UIView
    public var onState: ((String) -> Void)?
    public var onError: ((String) -> Void)?

    private let engine: AetherEngine
    private let surface: AetherPlayerView
    private var watch: AnyCancellable?
    private var task: Task<Void, Never>?

    public init() throws {
        engine = try AetherEngine()
        surface = AetherPlayerView(frame: .zero)
        view = surface
        engine.bind(view: surface)
        watch = engine.$state.sink { [weak self] state in self?.onState?("\(state)") }
    }

    /// Loads a title; playback starts by itself.
    public func load(url: URL, headers: [String: String]) {
        task?.cancel()
        task = Task { @MainActor [weak self] in
            guard let self else { return }
            do {
                try await engine.load(url: url, options: LoadOptions(httpHeaders: headers))
            } catch is CancellationError {
            } catch {
                onError?("\(error)")
            }
        }
    }

    public func stop() {
        task?.cancel()
        engine.stop()
    }
}
