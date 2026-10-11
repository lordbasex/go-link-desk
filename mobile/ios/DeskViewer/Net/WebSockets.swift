// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

import Foundation
import DeskCore

/**
 * signalhub WebSockets with URLSessionWebSocketTask. Native clients send
 * no Origin header, which signalhub accepts. Events are delivered on the
 * main actor, as GoLinkCore expects.
 */
@MainActor
final class URLSessionSockets: SignalSocketFactory {
    func open(url: String, listener: SignalSocketListener) -> SignalSocket? {
        guard let u = URL(string: url) else { return nil }
        return Socket(url: u, listener: listener)
    }
}

@MainActor
private final class Socket: NSObject, SignalSocket, URLSessionWebSocketDelegate {
    private var listener: SignalSocketListener?
    private var session: URLSession!
    private var task: URLSessionWebSocketTask!
    private var closed = false

    init(url: URL, listener: SignalSocketListener) {
        self.listener = listener
        super.init()
        let config = URLSessionConfiguration.ephemeral
        config.waitsForConnectivity = false
        config.timeoutIntervalForRequest = 15
        session = URLSession(configuration: config, delegate: self, delegateQueue: .main)
        task = session.webSocketTask(with: url)
        task.maximumMessageSize = 1 << 20
        task.resume()
        receive()
    }

    private func receive() {
        task.receive { [weak self] result in
            DispatchQueue.main.async {
                guard let self, !self.closed else { return }
                switch result {
                case let .success(message):
                    if case let .string(text) = message { self.listener?.onMessage(text) }
                    self.receive()
                case .failure:
                    self.finish()
                }
            }
        }
    }

    func send(_ text: String) -> Bool {
        guard !closed else { return false }
        task.send(.string(text)) { [weak self] error in
            if error != nil { DispatchQueue.main.async { self?.finish() } }
        }
        return true
    }

    func close() {
        guard !closed else { return }
        closed = true
        listener = nil // the listener points back at the client
        task.cancel(with: .normalClosure, reason: nil)
        session.invalidateAndCancel()
    }

    private func finish() {
        guard !closed else { return }
        let l = listener
        close()
        l?.onClosed()
    }

    nonisolated func urlSession(_ session: URLSession, webSocketTask: URLSessionWebSocketTask, didCloseWith closeCode: URLSessionWebSocketTask.CloseCode, reason: Data?) {
        DispatchQueue.main.async { self.finish() }
    }

    nonisolated func urlSession(_ session: URLSession, task: URLSessionTask, didCompleteWithError error: Error?) {
        DispatchQueue.main.async { self.finish() }
    }
}
