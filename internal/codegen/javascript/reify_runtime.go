package javascript

const sovaReifyRuntime = `
var __sovaTypeRegistry = (typeof globalThis !== 'undefined' && globalThis.__sovaTypeRegistry) || {};
if (typeof globalThis !== 'undefined') { globalThis.__sovaTypeRegistry = __sovaTypeRegistry; }
function __sovaRegisterType(name, ctor, fields) {
  __sovaTypeRegistry[name] = { ctor: ctor, fields: fields || {} };
}
function __sovaZero(desc) {
  if (desc == null) { return null; }
  switch (desc.kind) {
    case 'option':
    case 'any':
      return null;
    case 'primitive':
      switch (desc.prim) {
        case 'int':
        case 'float':
        case 'byte':
        case 'char':
          return 0;
        case 'bool':
          return false;
        case 'string':
          return "";
        default:
          return null;
      }
    case 'slice':
    case 'array':
      return [];
    case 'map':
      return {};
    case 'tuple':
      if (!Array.isArray(desc.elems)) { return null; }
      return desc.elems.map(function (e) { return __sovaZero(e); });
    case 'struct':
      var entry = __sovaTypeRegistry[desc.name];
      if (!entry) { return null; }
      try { return new entry.ctor(); } catch (e) { return Object.create(entry.ctor.prototype); }
    default:
      return null;
  }
}
function __sovaReify(value, desc) {
  if (desc == null) { return value; }
  if (value == null) { return __sovaZero(desc); }
  switch (desc.kind) {
    case 'primitive':
    case 'any':
      return value;
    case 'option':
      return __sovaReify(value, desc.elem);
    case 'slice':
    case 'array':
      if (!Array.isArray(value)) { return __sovaZero(desc); }
      return value.map(function (v) { return __sovaReify(v, desc.elem); });
    case 'tuple':
      if (!Array.isArray(value)) { return __sovaZero(desc); }
      return value.map(function (v, i) { return __sovaReify(v, desc.elems[i]); });
    case 'map':
      if (typeof value !== 'object') { return __sovaZero(desc); }
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
