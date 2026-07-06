import Capacitor
import Foundation

// Capacitor bridge for the local-server plugin (poker_app issue #28).
// This is the thin @objc surface Capacitor calls; the real networking lives in
// LocalServer.swift. Method bodies are honest stubs: they wire the call/return
// shape and delegate to LocalServer, whose NWListener/Bonjour bodies carry TODOs
// where a device is required to validate them.

@objc(LocalServerPlugin)
public class LocalServerPlugin: CAPPlugin, CAPBridgedPlugin {
    public let identifier = "LocalServerPlugin"
    public let jsName = "LocalServer"
    public let pluginMethods: [CAPPluginMethod] = [
        CAPPluginMethod(name: "start", returnType: CAPPluginReturnPromise),
        CAPPluginMethod(name: "stop", returnType: CAPPluginReturnPromise),
        CAPPluginMethod(name: "connect", returnType: CAPPluginReturnPromise),
        CAPPluginMethod(name: "send", returnType: CAPPluginReturnPromise),
        CAPPluginMethod(name: "disconnect", returnType: CAPPluginReturnPromise)
    ]

    private lazy var server = LocalServer(
        onMessage: { [weak self] peerId, data in
            self?.notifyListeners("message", data: ["peerId": peerId, "data": data])
        },
        onConnection: { [weak self] peerId, state in
            self?.notifyListeners("connection", data: ["peerId": peerId, "state": state])
        },
        onDiscovered: { [weak self] peer in
            self?.notifyListeners("peerDiscovered", data: peer)
        }
    )

    @objc func start(_ call: CAPPluginCall) {
        guard let peerId = call.getString("peerId") else {
            call.reject("peerId is required")
            return
        }
        let serviceType = call.getString("serviceType") ?? "_feltpoker._tcp"
        let port = UInt16(call.getInt("port") ?? 0)
        let displayName = call.getString("displayName") ?? peerId
        do {
            let bound = try server.start(peerId: peerId, serviceType: serviceType, port: port, displayName: displayName)
            call.resolve(["port": Int(bound)])
        } catch {
            call.reject("start failed: \(error.localizedDescription)")
        }
    }

    @objc func stop(_ call: CAPPluginCall) {
        server.stop()
        call.resolve()
    }

    @objc func connect(_ call: CAPPluginCall) {
        guard let peerId = call.getString("peerId"),
              let host = call.getString("host"),
              let port = call.getInt("port") else {
            call.reject("peerId, host, port are required")
            return
        }
        server.connect(peerId: peerId, host: host, port: UInt16(port))
        call.resolve()
    }

    @objc func send(_ call: CAPPluginCall) {
        guard let peerId = call.getString("peerId"), let data = call.getString("data") else {
            call.reject("peerId and data are required")
            return
        }
        server.send(peerId: peerId, data: data)
        call.resolve()
    }

    @objc func disconnect(_ call: CAPPluginCall) {
        guard let peerId = call.getString("peerId") else {
            call.reject("peerId is required")
            return
        }
        server.disconnect(peerId: peerId)
        call.resolve()
    }
}
