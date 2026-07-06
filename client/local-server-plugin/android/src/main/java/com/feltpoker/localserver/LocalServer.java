package com.feltpoker.localserver;

import java.util.HashMap;
import java.util.Map;

// LAN transport core for Android (poker_app issue #28).
//
// Design:
//   - advertise + listen: NsdManager.registerService(_feltpoker._tcp) plus a
//     local WebSocket listener (NanoHTTPD's NanoWSD, or Java-WebSocket's
//     WebSocketServer) bound to the same port, so every peer hosts its own
//     endpoint (no privileged host).
//   - discover: NsdManager.discoverServices + resolveService emits onDiscovered.
//   - connect: a WebSocket client (Java-WebSocket) dials a resolved peer.
//
// The class shape is complete; the marked bodies are stubs because validating
// mDNS + sockets needs two devices on a Wi-Fi LAN. Each TODO is precise. Wire
// the concrete NsdManager/NanoWSD calls here; the plugin bridge above is done.

public class LocalServer {

    public interface OnMessage { void call(String peerId, String data); }
    public interface OnConnection { void call(String peerId, String state); }
    public interface OnDiscovered { void call(String peerId, String host, int port, String displayName); }

    private final OnMessage onMessage;
    private final OnConnection onConnection;
    private final OnDiscovered onDiscovered;

    private final Map<String, Object> connections = new HashMap<>();
    private String selfPeerId = "";
    private int boundPort = 0;

    public LocalServer(OnMessage onMessage, OnConnection onConnection, OnDiscovered onDiscovered) {
        this.onMessage = onMessage;
        this.onConnection = onConnection;
        this.onDiscovered = onDiscovered;
    }

    public int start(String peerId, String serviceType, int port, String displayName) throws Exception {
        this.selfPeerId = peerId;
        // TODO(device): start a NanoWSD/WebSocketServer bound to `port` (0 -> OS
        // picks; read the actual port into boundPort). In its onOpen, read the
        // peer's hello frame (its peerId) then onConnection(peerId, "open"); in
        // onMessage, forward to this.onMessage; onClose -> onConnection(peerId,
        // "closed"). Then registerService via NsdManager with serviceType and
        // displayName, and startDiscovery (see startDiscovery()).
        boundPort = port;
        startDiscovery(serviceType);
        return boundPort;
    }

    private void startDiscovery(String serviceType) {
        // TODO(device): NsdManager.discoverServices(serviceType, PROTOCOL_DNS_SD,
        // listener); on service found, resolveService; on resolved, call
        // onDiscovered(name, host.getHostAddress(), port, name). Skip our own
        // advertised service (name == selfPeerId's display name).
    }

    public void connect(String peerId, String host, int port) {
        if (connections.containsKey(peerId)) return;
        // TODO(device): open a Java-WebSocket client to ws://host:port; on open,
        // send a hello frame carrying selfPeerId, then onConnection(peerId,
        // "open"); on message forward to onMessage; on close onConnection(peerId,
        // "closed"). Store the client in connections.
    }

    public void send(String peerId, String data) {
        Object conn = connections.get(peerId);
        if (conn == null) return;
        // TODO(device): ((WebSocket) conn).send(data);
    }

    public void disconnect(String peerId) {
        connections.remove(peerId);
        onConnection.call(peerId, "closed");
        // TODO(device): close the stored socket.
    }

    public void stop() {
        connections.clear();
        // TODO(device): unregisterService, stopDiscovery, stop the WS server.
    }
}
