# Memory Monitor

Shows RAM, processes, and GTT memory for AMD systems with unified memory.

## Requirements

- Reads `/proc` and `/sys/class/drm`
- Only tested on AMD 7840u

## Installation & Building

### Prerequisites
- [Go](https://go.dev/doc/install) (1.25 or later recommended)

### Build
Run the provided build script:
```bash
./build.sh
```

To install it to `/usr/local/bin`:
```bash
./build.sh --install
```

## Usage

Run with sudo to get the full process breakdown:
```bash
sudo ./mem-monitor
```

### Shortcuts
- `r`: Sort by System RAM usage
- `g`: Sort by GPU GTT usage
- `v`: Sort by GPU VRAM usage
- `q` or `Ctrl+C`: Quit

## License

Distributed under the MIT License. See `LICENSE` for more information.
