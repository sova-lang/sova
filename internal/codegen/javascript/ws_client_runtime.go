package javascript

// sovaWSClientRuntime is the JavaScript that boots the frontend WebSocket connection to /__sova/ws, registers the per-frontend-wire handler map, and dispatches incoming `op:"call"` envelopes from the backend. It is injected at the top of the emitted bundle when the build needs the session manager (frontend wires, mutable wire vars, or lifecycle hooks present). Same-origin URL derivation works without configuration; reconnects use exponential backoff capped at 30 seconds.
const sovaWSClientRuntime = `
var __sova_ws = null;
var __sova_ws_wires = {};
var __sova_ws_var_listeners = {};
var __sova_ws_pending = {};
var __sova_ws_reconnect_ms = 1000;
var __sova_ws_call_seq = 0;
function __sovaRegisterWire(name, fn) { __sova_ws_wires[name] = fn; }
function __sovaOnWireVar(name, fn) {
  (__sova_ws_var_listeners[name] = __sova_ws_var_listeners[name] || []).push(fn);
}
function __sovaWSCall(name, args) {
  return new Promise(function (resolve) {
    if (!__sova_ws || __sova_ws.readyState !== 1) {
      resolve({ value: null, state: 4 });
      return;
    }
    __sova_ws_call_seq += 1;
    var id = 'c' + __sova_ws_call_seq;
    __sova_ws_pending[id] = resolve;
    __sova_ws.send(JSON.stringify({ op: 'call', id: id, fn: name, args: (args || []) }));
  });
}
async function __sovaWSDispatch(env) {
  if (!env) return;
  if (env.op === 'reply') {
    var pending = __sova_ws_pending[env.id];
    if (pending) {
      delete __sova_ws_pending[env.id];
      var value = env.value;
      if (typeof value === 'string') { try { value = JSON.parse(value); } catch (_) {} }
      var state = 0;
      if (value && typeof value === 'object' && 'value' in value && 'state' in value) {
        state = value.state || 0;
        value = value.value;
      }
      pending({ value: value, state: state });
      return;
    }
    __sovaWSDeliverReplyInbound(env);
    return;
  }
  if (env.op === 'call') {
    var fn = __sova_ws_wires[env.fn];
    if (!fn) return;
    var args = [];
    try { args = env.args ? JSON.parse(typeof env.args === 'string' ? env.args : JSON.stringify(env.args)) : []; }
    catch (e) { args = []; }
    if (!Array.isArray(args)) args = [];
    try {
      var result = await fn.apply(null, args);
      if (env.id && __sova_ws && __sova_ws.readyState === 1) {
        __sova_ws.send(JSON.stringify({ op: 'reply', id: env.id, value: result === undefined ? null : result }));
      }
    } catch (err) {
      if (env.id && __sova_ws && __sova_ws.readyState === 1) {
        __sova_ws.send(JSON.stringify({ op: 'reply', id: env.id, error: String(err) }));
      }
    }
    return;
  }
  if (env.op === 'var') {
    var listeners = __sova_ws_var_listeners[env.fn] || [];
    var value = env.value;
    if (typeof value === 'string') { try { value = JSON.parse(value); } catch (_) {} }
    for (var i = 0; i < listeners.length; i++) {
      try { listeners[i](value); } catch (_) {}
    }
  }
}
function __sovaWSDeliverReplyInbound(env) { /* placeholder for backend-side server-push reply path */ }
function __sovaConnectWS() {
  if (typeof window === 'undefined' || typeof WebSocket === 'undefined') return;
  try {
    var proto = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
    var host = window.location.host;
    var url = (typeof window !== 'undefined' && window.__sovaWSOverrideURL) ? window.__sovaWSOverrideURL : (proto + '//' + host + '/__sova/ws');
    var sock = new WebSocket(url);
    __sova_ws = sock;
    sock.onopen = function () { __sova_ws_reconnect_ms = 1000; };
    sock.onmessage = function (ev) {
      var env;
      try { env = JSON.parse(ev.data); } catch (_) { return; }
      __sovaWSDispatch(env);
    };
    sock.onclose = function () {
      __sova_ws = null;
      setTimeout(__sovaConnectWS, __sova_ws_reconnect_ms);
      __sova_ws_reconnect_ms = Math.min(__sova_ws_reconnect_ms * 2, 30000);
    };
    sock.onerror = function () { try { sock.close(); } catch (_) {} };
  } catch (e) {
    setTimeout(__sovaConnectWS, __sova_ws_reconnect_ms);
    __sova_ws_reconnect_ms = Math.min(__sova_ws_reconnect_ms * 2, 30000);
  }
}
if (typeof window !== 'undefined') {
  if (document.readyState === 'complete' || document.readyState === 'interactive') {
    __sovaConnectWS();
  } else {
    window.addEventListener('DOMContentLoaded', __sovaConnectWS);
  }
}
`
