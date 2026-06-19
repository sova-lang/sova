package javascript

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
