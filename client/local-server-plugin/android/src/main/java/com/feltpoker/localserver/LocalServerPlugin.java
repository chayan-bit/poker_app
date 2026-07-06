package com.feltpoker.localserver;

import com.getcapacitor.JSObject;
import com.getcapacitor.Plugin;
import com.getcapacitor.PluginCall;
import com.getcapacitor.PluginMethod;
import com.getcapacitor.annotation.CapacitorPlugin;

// Capacitor bridge for the local-server plugin on Android (poker_app issue #28).
// The @PluginMethod surface delegates to LocalServer, which owns NSD (mDNS)
// discovery/advertising and a NanoHTTPD-style local WebSocket listener. Bodies
// are honest stubs: the bridge shape is complete; the native networking in
// LocalServer.java carries precise TODOs where two devices on a LAN are needed
// to validate.

@CapacitorPlugin(name = "LocalServer")
public class LocalServerPlugin extends Plugin {

    private LocalServer server;

    @Override
    public void load() {
        server = new LocalServer(
            (peerId, data) -> {
                JSObject e = new JSObject();
                e.put("peerId", peerId);
                e.put("data", data);
                notifyListeners("message", e);
            },
            (peerId, state) -> {
                JSObject e = new JSObject();
                e.put("peerId", peerId);
                e.put("state", state);
                notifyListeners("connection", e);
            },
            (peerId, host, port, displayName) -> {
                JSObject e = new JSObject();
                e.put("peerId", peerId);
                e.put("host", host);
                e.put("port", port);
                e.put("displayName", displayName);
                notifyListeners("peerDiscovered", e);
            }
        );
    }

    @PluginMethod
    public void start(PluginCall call) {
        String peerId = call.getString("peerId");
        if (peerId == null) {
            call.reject("peerId is required");
            return;
        }
        String serviceType = call.getString("serviceType", "_feltpoker._tcp");
        int port = call.getInt("port", 0);
        String displayName = call.getString("displayName", peerId);
        try {
            int bound = server.start(peerId, serviceType, port, displayName);
            JSObject ret = new JSObject();
            ret.put("port", bound);
            call.resolve(ret);
        } catch (Exception ex) {
            call.reject("start failed: " + ex.getMessage());
        }
    }

    @PluginMethod
    public void stop(PluginCall call) {
        server.stop();
        call.resolve();
    }

    @PluginMethod
    public void connect(PluginCall call) {
        String peerId = call.getString("peerId");
        String host = call.getString("host");
        Integer port = call.getInt("port");
        if (peerId == null || host == null || port == null) {
            call.reject("peerId, host, port are required");
            return;
        }
        server.connect(peerId, host, port);
        call.resolve();
    }

    @PluginMethod
    public void send(PluginCall call) {
        String peerId = call.getString("peerId");
        String data = call.getString("data");
        if (peerId == null || data == null) {
            call.reject("peerId and data are required");
            return;
        }
        server.send(peerId, data);
        call.resolve();
    }

    @PluginMethod
    public void disconnect(PluginCall call) {
        String peerId = call.getString("peerId");
        if (peerId == null) {
            call.reject("peerId is required");
            return;
        }
        server.disconnect(peerId);
        call.resolve();
    }
}
