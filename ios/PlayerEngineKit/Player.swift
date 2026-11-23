// The pilot's engine, in a dynamic framework of its own: the app links it at
// run time instead of compiling the engine in. AetherEngine is LGPL-3.0, and
// Apple's guidance for it is a dynamic link, so a modified engine replaces this
// one without rebuilding the app.
import AetherEngine
import Combine
import UIKit

/// One playback component and its engine.
@MainActor
public final class PilotPlayer {
    public let view: UIView
    public var onState: ((String) -> Void)?
    public var onError: ((String) -> Void)?
    public var onProgress: ((Double) -> Void)?

    private let engine: AetherEngine
    private let surface: AetherPlayerView
    private var watch: AnyCancellable?
    private var clockWatch: AnyCancellable?
    private var task: Task<Void, Never>?
    private var awaitingProgress = false

    public init() throws {
        engine = try AetherEngine()
        surface = AetherPlayerView(frame: .zero)
        view = surface
        engine.bind(view: surface)
        watch = engine.$state.sink { [weak self] state in self?.onState?("\(state)") }
        clockWatch = engine.clock.$currentTime.sink { [weak self] seconds in
            self?.reportProgress(seconds)
        }
    }

    /// Loads a title; playback starts by itself.
    public func load(url: URL, headers: [String: String]) {
        stop()
        task = Task { @MainActor [weak self] in
            guard let self else { return }
            do {
                try await engine.load(url: url, options: LoadOptions(httpHeaders: headers))
                guard !Task.isCancelled else { return }
                awaitingProgress = true
                reportProgress(engine.currentTime)
            } catch is CancellationError {
            } catch {
                if !Task.isCancelled { onError?("\(error)") }
            }
        }
    }

    public func stop() {
        awaitingProgress = false
        task?.cancel()
        task = nil
        engine.stop()
    }

    private func reportProgress(_ seconds: Double) {
        guard awaitingProgress, seconds > 0.25, case .playing = engine.state else { return }
        awaitingProgress = false
        onProgress?(seconds)
    }
}
