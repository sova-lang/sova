package javascript

// sovaReifyRuntime is the JS snippet that powers cross-tier class-instance reification. Plain JSON arriving from the wire is a graph of objects + arrays + primitives; the receiver does not get instances of the matching Sova classes back unless something rebuilds them. This runtime does that: `__sovaReify(value, descriptor)` walks `value` against a compact descriptor tree (`{kind: "struct", name: ...}`, `{kind: "slice", elem: ...}`, etc.) and produces, for every `struct` node, an instance of the registered class with the right prototype — so methods declared on the class are callable on the reified value.
//
// Field descriptors for struct kinds are stored as a static `__sovaFields` property on each emitted class; that lets the recursion read the field shape without baking it into every call site. The registry maps mangled class names to constructor references so the recursion can look up the prototype without holding a class reference per descriptor.
//
// The reifier is intentionally permissive: unknown descriptor kinds, missing registry entries, or shape mismatches pass the value through unchanged so a wire whose return type is `any` (or one whose class wasn't emitted on this side, e.g. a backend-only type that briefly leaked through) still produces *something* usable rather than throwing.
const sovaReifyRuntime = `
var __sovaTypeRegistry = (typeof globalThis !== 'undefined' && globalThis.__sovaTypeRegistry) || {};
if (typeof globalThis !== 'undefined') { globalThis.__sovaTypeRegistry = __sovaTypeRegistry; }
function __sovaRegisterType(name, ctor, fields) {
  __sovaTypeRegistry[name] = { ctor: ctor, fields: fields || {} };
}
function __sovaReify(value, desc) {
  if (value == null || desc == null) { return value; }
  switch (desc.kind) {
    case 'primitive':
    case 'any':
      return value;
    case 'option':
      return value === null ? null : __sovaReify(value, desc.elem);
    case 'slice':
    case 'array':
      if (!Array.isArray(value)) { return value; }
      return value.map(function (v) { return __sovaReify(v, desc.elem); });
    case 'tuple':
      if (!Array.isArray(value)) { return value; }
      return value.map(function (v, i) { return __sovaReify(v, desc.elems[i]); });
    case 'map':
      if (typeof value !== 'object') { return value; }
      var out = {};
      for (var k in value) {
        if (Object.prototype.hasOwnProperty.call(value, k)) {
          out[k] = __sovaReify(value[k], desc.value);
        }
      }
      return out;
    case 'struct':
      var entry = __sovaTypeRegistry[desc.name];
      if (!entry) { return value; }
      var inst = Object.create(entry.ctor.prototype);
      if (typeof value === 'object') {
        for (var f in value) {
          if (!Object.prototype.hasOwnProperty.call(value, f)) { continue; }
          var fd = entry.fields[f];
          inst[f] = __sovaReify(value[f], fd);
        }
      }
      return inst;
    default:
      return value;
  }
}
`
