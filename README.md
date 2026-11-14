# Morphogenesis Laboratory

A generative-art laboratory for Linux terminals, implemented in one Go source file with the Go standard library. It simulates the Gray–Scott reaction–diffusion system and renders evolving patterns as ANSI truecolor half-block pixels or exports reproducible images and state files.

## Features

- Gray–Scott two-field reaction–diffusion simulation with nine-point finite-difference Laplacian
- Configurable feed and kill rates, diffusion coefficients and integration timestep
- Ten pattern presets: bloom, chaos, coral, fingerprints, labyrinth, mitosis, solitons, spots, waves and worms
- Seven color palettes: aurora, ember, moon, orchid, forest, ice and mono
- Deterministic initial conditions using a user-selected seed
- Periodic, clamped or mirrored boundary conditions
- Interactive ANSI truecolor terminal animation with alternate-screen rendering
- Live controls for pause, simulation speed, palette and reseeding
- JSON snapshots containing complete concentration fields and configuration
- Snapshot continuation, PPM image output, ASCII output, JSON statistics and benchmarks
- No external Go dependencies; no write operations to block devices or system configuration

## Requirements

Linux, Go 1.21 or newer, and a terminal with ANSI truecolor support. The program uses Linux terminal ioctls and is intended for Linux rather than cross-platform builds. It does not require root privileges.

## Build

```sh
go build -o morphogenesis main.go
```

## Quick start

```sh
./morphogenesis run --preset coral --palette ember --width 100 --height 42 --fps 20 --steps 8 --frames 0
```

`--frames 0` runs continuously until `q` or Ctrl+C. The default is 300 frames. A 100x42 simulation needs at least 100 terminal columns and about 23 rows. If your terminal is smaller, lower `--width` and `--height`.

Other examples:

```sh
./morphogenesis run --preset labyrinth --palette aurora --frames 0
./morphogenesis run --preset worms --palette moon --seed 1234 --frames 0
./morphogenesis render --preset mitosis --frames 5000 --scale 5 --out mitosis.ppm
./morphogenesis ascii --preset spots --frames 2500 --width 80 --height 32 --out spots.txt
./morphogenesis snapshot --preset coral --frames 3000 --out coral.json
./morphogenesis resume --in coral.json --frames 1000 --out coral-next.json
./morphogenesis resume --in coral.json --frames 1000 --out coral-next.ppm
./morphogenesis stats --preset bloom --frames 1000
./morphogenesis benchmark --width 200 --height 120 --frames 1000
./morphogenesis presets
./morphogenesis palettes
```

PPM uses the binary P6 format. Most image viewers and ImageMagick can read it. A snapshot may be large because it stores two full floating-point arrays as JSON.

## Controls

| Key | Action |
| --- | --- |
| `q` | Quit |
| Space | Pause/resume |
| `+` | Increase simulation steps per rendered frame |
| `-` | Decrease simulation steps per rendered frame |
| `p` | Cycle color palettes |
| `r` | Reinitialize the simulation from the configured seed |
| Ctrl+C | Exit and restore the terminal |

The program attempts to restore terminal state when exiting normally or via SIGINT/SIGTERM. If a process is forcibly killed, `reset` can restore a disturbed terminal.

## Mathematical model

The chemical fields A and B evolve according to:

```text
dA/dt = Da * laplacian(A) - A*B*B + f*(1-A)
dB/dt = Db * laplacian(B) + A*B*B - (k+f)*B
```

The numerical implementation uses explicit Euler updates and a nine-point Laplacian with axial weights 0.2, diagonal weights 0.05 and center weight -1. Field concentrations are clamped to [0, 1]. These are artistic simulations, not validated chemical predictions. The explicit integrator can become unstable with aggressive parameter changes; use conservative diffusion and timestep settings.

## Options

| Flag | Default | Purpose |
| --- | --- | --- |
| `--width` | 100 | Simulation columns |
| `--height` | 48 | Simulation rows |
| `--preset` | coral | Gray–Scott feed/kill preset |
| `--palette` | aurora | Render color palette |
| `--seed` | 42 | Initial random seed |
| `--feed` | preset-specific | Override feed rate |
| `--kill` | preset-specific | Override kill rate |
| `--diff-a` | 1 | Diffusion coefficient A |
| `--diff-b` | 0.5 | Diffusion coefficient B |
| `--dt` | 1 | Integration timestep |
| `--boundary` | wrap | wrap, clamp or mirror |
| `--noise` | 0.02 | Initial perturbation probability |
| `--fps` | 20 | Target animation frames per second |
| `--steps` | 8 | Simulation steps per animation frame |
| `--frames` | 300 | Animation frames or offline simulation steps; 0 means unlimited only for run |
| `--scale` | 4 | PPM output enlargement |
| `--out` | command-specific | Output path |

For `resume`, provide `--in snapshot.json`; its saved model parameters are restored. Output format is selected from the `--out` extension (`.ppm`, `.txt`, or JSON by default).

## Technical notes

Simulation complexity is O(width × height) per integration step. Two concentration fields and two next-step buffers require approximately 32 bytes per cell, excluding snapshot and renderer overhead. Truecolor terminal rendering can be much slower than the mathematical simulation on large grids. The program is single-threaded for numerical updates, while a small goroutine reads keyboard input. Terminal behavior depends on the terminal emulator.

## License

GNU General Public License version 3. See `LICENSE`.
