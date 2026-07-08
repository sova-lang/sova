package javascript

const sovaWireClientRuntime = `
const __sovaWireHooks = { beforeCall: [], onUnauthorized: [] };

function __sovaAddBeforeCall(fn) {
  if (typeof fn !== 'function') return () => {};
  __sovaWireHooks.beforeCall.push(fn);
  return () => {
    const i = __sovaWireHooks.beforeCall.indexOf(fn);
    if (i >= 0) __sovaWireHooks.beforeCall.splice(i, 1);
  };
}

function __sovaAddOnUnauthorized(fn) {
  if (typeof fn !== 'function') return () => {};
  __sovaWireHooks.onUnauthorized.push(fn);
  return () => {
    const i = __sovaWireHooks.onUnauthorized.indexOf(fn);
    if (i >= 0) __sovaWireHooks.onUnauthorized.splice(i, 1);
  };
}

if (typeof globalThis !== 'undefined') {
  globalThis.__sovaAddBeforeCall = __sovaAddBeforeCall;
  globalThis.__sovaAddOnUnauthorized = __sovaAddOnUnauthorized;
}

async function __sovaWireCall(ctx) {
  ctx.aborted = false;
  for (const h of __sovaWireHooks.beforeCall) {
    try { await h(ctx); } catch (_) {}
    if (ctx.aborted) break;
  }
  if (ctx.aborted) {
    return { ok: false, status: 401, type: 'basic', headers: new Headers(), json: async () => ({}), text: async () => '', blob: async () => new Blob() };
  }
  let res = await fetch(ctx.url, ctx.opts);
  if (res.status === 401 && __sovaWireHooks.onUnauthorized.length > 0) {
    let retry = false;
    for (const h of __sovaWireHooks.onUnauthorized) {
      try {
        if (await h(ctx)) { retry = true; break; }
      } catch (_) {}
    }
    if (retry) {
      res = await fetch(ctx.url, ctx.opts);
    }
  }
  return res;
}
`
