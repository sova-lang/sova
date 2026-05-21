# Sova

The Sova compiler and toolchain.

Sova is a multi-tier programming language. Backend code transpiles to Go, frontend code to JavaScript, and the compiler automatically generates the wiring (transport, serialization, async handling, auth, routing) for declarations that cross the frontend/backend boundary.

## Installation

### Linux & macOS (bash, zsh, sh)

```sh
curl -fsSL https://raw.githubusercontent.com/sova-lang/sova/main/install.sh | sh
```

### Linux & macOS (fish)

```fish
curl -fsSL https://raw.githubusercontent.com/sova-lang/sova/main/install.fish | fish
```

### Windows (PowerShell)

```powershell
iwr -useb https://raw.githubusercontent.com/sova-lang/sova/main/install.ps1 | iex
```

Open a new terminal afterwards so the updated `PATH` takes effect, then check that everything works:

```sh
sova version
```

### Updating

Re-run the same install command — the script overwrites the existing binary and stdlib in place. Alternatively, once installed:

```sh
sova upgrade
```

### Installing a specific version

```sh
curl -fsSL https://raw.githubusercontent.com/sova-lang/sova/main/install.sh | SOVA_VERSION=v1.2.3 sh
```

```powershell
$env:SOVA_VERSION = 'v1.2.3'; iwr -useb https://raw.githubusercontent.com/sova-lang/sova/main/install.ps1 | iex
```

### Install locations

| Platform        | Path                                                |
| --------------- | --------------------------------------------------- |
| Linux / macOS   | `~/.sova/` (binary + `std/`)                        |
| Windows         | `%LOCALAPPDATA%\sova\` (binary + `std\`)            |

Override with the `SOVA_INSTALL_DIR` environment variable before running the installer.

### Supported platforms

| OS      | x64 | arm64 |
| ------- | --- | ----- |
| Linux   | ✓   | ✓     |
| macOS   | ✓   | ✓     |
| Windows | ✓   | ✓     |

## Building from source

```sh
git clone https://github.com/sova-lang/sova
cd sova
go build -o sova .
```

The compiler discovers the stdlib via `<binary-dir>/std`, `<binary-dir>/../std`, the current working directory's `std/`, or `$SOVA_HOME/std` — in that order.

## License

See [LICENSE](LICENSE).
