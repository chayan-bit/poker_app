import Foundation
import Network

// LAN transport core for iOS (poker_app issue #28).
//
// Design (Network.framework, iOS 13+):
//   - advertise + listen: NWListener with an NWListener.Service for Bonjour
//     (_feltpoker._tcp) and the WebSocket protocol option, so every peer hosts
//     its own endpoint. No privileged host, matching the mesh design.
//   - browse: NWBrowser over the same service type emits peerDiscovered.
//   - connect: NWConnection to a discovered endpoint with .ws options.
//
// The structure below is complete and compilable in intent; the marked bodies
// are stubs because they need a real device / two devices on a LAN to validate,
// which cannot be done from this environment. Each TODO is precise.

final class LocalServer {
    typealias OnMessage = (_ peerId: String, _ data: String) -> Void
    typealias OnConnection = (_ peerId: String, _ state: String) -> Void
    typealias OnDiscovered = (_ peer: [String: Any]) -> Void

    private let onMessage: OnMessage
    private let onConnection: OnConnection
    private let onDiscovered: OnDiscovered

    private var listener: NWListener?
    private var browser: NWBrowser?
    private var connections: [String: NWConnection] = [:]
    private var selfPeerId: String = ""
    private let queue = DispatchQueue(label: "feltpoker.localserver")

    init(onMessage: @escaping OnMessage, onConnection: @escaping OnConnection, onDiscovered: @escaping OnDiscovered) {
        self.onMessage = onMessage
        self.onConnection = onConnection
        self.onDiscovered = onDiscovered
    }

    func start(peerId: String, serviceType: String, port: UInt16, displayName: String) throws -> UInt16 {
        selfPeerId = peerId
        // TODO(device): build NWParameters with .tcp + NWProtocolWebSocket.Options,
        // create NWListener(using:on:), set listener.service =
        // NWListener.Service(name: displayName, type: serviceType), set
        // newConnectionHandler to accept peers and pump receiveMessage into
        // onMessage, then listener.start(queue:). Also start browse().
        // Return listener.port?.rawValue once bound (0 -> OS-assigned).
        startBrowse(serviceType: serviceType)
        return port
    }

    private func startBrowse(serviceType: String) {
        // TODO(device): NWBrowser(for: .bonjour(type: serviceType, domain: nil),
        // using: .init()); on browseResultsChangedHandler, resolve endpoints and
        // call onDiscovered(["peerId": ..., "host": ..., "port": ...]). The peer
        // id is exchanged as a first WS frame on connect (endpoints only carry
        // the Bonjour name).
    }

    func connect(peerId: String, host: String, port: UInt16) {
        guard connections[peerId] == nil else { return }
        // TODO(device): NWConnection(host:port:using: ws params); on .ready send a
        // hello frame carrying selfPeerId, then onConnection(peerId, "open"); pump
        // receiveMessage into onMessage; on failed/cancelled onConnection(peerId,
        // "closed"). Store in connections[peerId].
    }

    func send(peerId: String, data: String) {
        guard let conn = connections[peerId] else { return }
        let metadata = NWProtocolWebSocket.Metadata(opcode: .text)
        let context = NWConnection.ContentContext(identifier: "text", metadata: [metadata])
        conn.send(content: data.data(using: .utf8), contentContext: context, completion: .contentProcessed { _ in })
    }

    func disconnect(peerId: String) {
        connections[peerId]?.cancel()
        connections[peerId] = nil
        onConnection(peerId, "closed")
    }

    func stop() {
        listener?.cancel()
        browser?.cancel()
        for (_, c) in connections { c.cancel() }
        connections.removeAll()
    }
}
