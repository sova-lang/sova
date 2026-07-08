package javascript

const sovaWireClientRuntime = `
const __sovaWireHooks = { beforeCall: null, onUnauthorized: null };

function __sovaInstallWireHooks(before, onUnauth) {
  __sovaWireHooks.beforeCall = (typeof before === 'function') ? before : null;
  __sovaWireHooks.onUnauthorized = (typeof onUnauth === 'function') ? onUnauth : null;
}

if (typeof globalThis !== 'undefined') {
  globalThis.__sovaInstallWireHooks = __sovaInstallWireHooks;
}

async function __sovaWireCall(ctx) {
  const h = __sovaWireHooks;
  if (h.beforeCall) {
    const decision = await h.beforeCall(ctx);
    if (decision === 'abort') {
      return { ok: false, status: 401, type: 'basic', headers: new Headers(), json: async () => ({}), text: async () => '', blob: async () => new Blob() };
    }
  }
  let res = await fetch(ctx.url, ctx.opts);
  if (res.status === 401 && h.onUnauthorized) {
    const shouldRetry = await h.onUnauthorized(ctx);
    if (shouldRetry) {
      res = await fetch(ctx.url, ctx.opts);
    }
  }
  return res;
}
`
