package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"math"
	"math/rand"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"syscall"
	"time"
	"unsafe"
)

type Config struct {
	Width    int     `json:"width"`
	Height   int     `json:"height"`
	Feed     float64 `json:"feed"`
	Kill     float64 `json:"kill"`
	DiffA    float64 `json:"diff_a"`
	DiffB    float64 `json:"diff_b"`
	DT       float64 `json:"dt"`
	Seed     int64   `json:"seed"`
	Preset   string  `json:"preset"`
	Boundary string  `json:"boundary"`
	Palette  string  `json:"palette"`
	Steps    int     `json:"steps"`
	FPS      int     `json:"fps"`
	Scale    int     `json:"scale"`
	Noise    float64 `json:"noise"`
}
type Snapshot struct {
	Version   int       `json:"version"`
	Created   string    `json:"created"`
	Config    Config    `json:"config"`
	Iteration int64     `json:"iteration"`
	A         []float64 `json:"a"`
	B         []float64 `json:"b"`
}
type Stats struct {
	Iteration   int64   `json:"iteration"`
	MeanA       float64 `json:"mean_a"`
	MeanB       float64 `json:"mean_b"`
	MinB        float64 `json:"min_b"`
	MaxB        float64 `json:"max_b"`
	VarianceB   float64 `json:"variance_b"`
	ActiveCells int     `json:"active_cells"`
	Width       int     `json:"width"`
	Height      int     `json:"height"`
	Feed        float64 `json:"feed"`
	Kill        float64 `json:"kill"`
}
type Model struct {
	C            Config
	A, B, NA, NB []float64
	Iteration    int64
	RNG          *rand.Rand
}
type RGB struct{ R, G, B uint8 }
type Stop struct {
	X float64
	C RGB
}

var presets = map[string][2]float64{"coral": {0.0545, 0.062}, "mitosis": {0.0367, 0.0649}, "worms": {0.078, 0.061}, "labyrinth": {0.029, 0.057}, "spots": {0.035, 0.065}, "solitons": {0.03, 0.062}, "waves": {0.014, 0.054}, "chaos": {0.026, 0.051}, "fingerprints": {0.037, 0.060}, "bloom": {0.022, 0.051}}
var palettes = map[string][]Stop{
	"ember":  {{0, RGB{8, 5, 13}}, {0.18, RGB{47, 13, 25}}, {0.42, RGB{153, 45, 27}}, {0.7, RGB{247, 138, 42}}, {1, RGB{255, 231, 157}}},
	"aurora": {{0, RGB{3, 9, 20}}, {0.25, RGB{10, 51, 73}}, {0.5, RGB{23, 139, 132}}, {0.75, RGB{116, 219, 168}}, {1, RGB{228, 255, 212}}},
	"moon":   {{0, RGB{4, 8, 22}}, {0.25, RGB{24, 36, 72}}, {0.5, RGB{66, 94, 143}}, {0.75, RGB{156, 184, 213}}, {1, RGB{243, 249, 255}}},
	"orchid": {{0, RGB{13, 5, 23}}, {0.25, RGB{55, 19, 90}}, {0.5, RGB{136, 55, 159}}, {0.75, RGB{228, 112, 192}}, {1, RGB{255, 220, 238}}},
	"forest": {{0, RGB{4, 14, 11}}, {0.25, RGB{13, 56, 34}}, {0.5, RGB{39, 115, 61}}, {0.75, RGB{130, 185, 84}}, {1, RGB{243, 245, 172}}},
	"ice":    {{0, RGB{4, 12, 27}}, {0.25, RGB{13, 62, 102}}, {0.5, RGB{41, 144, 190}}, {0.75, RGB{137, 223, 237}}, {1, RGB{246, 255, 255}}},
	"mono":   {{0, RGB{5, 5, 5}}, {1, RGB{245, 245, 245}}},
}

func clamp(v, lo, hi float64) float64 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
func lerp(a, b float64, t float64) float64 { return a + (b-a)*t }
func color(v float64, p string) RGB {
	s, ok := palettes[p]
	if !ok {
		s = palettes["aurora"]
	}
	v = clamp(v, 0, 1)
	for i := 1; i < len(s); i++ {
		if v <= s[i].X {
			t := (v - s[i-1].X) / (s[i].X - s[i-1].X)
			a, b := s[i-1].C, s[i].C
			return RGB{uint8(lerp(float64(a.R), float64(b.R), t)), uint8(lerp(float64(a.G), float64(b.G), t)), uint8(lerp(float64(a.B), float64(b.B), t))}
		}
	}
	return s[len(s)-1].C
}
func defaults() Config {
	return Config{Width: 100, Height: 48, Feed: 0.0545, Kill: 0.062, DiffA: 1, DiffB: 0.5, DT: 1, Seed: 42, Preset: "coral", Boundary: "wrap", Palette: "aurora", Steps: 8, FPS: 20, Scale: 4, Noise: 0.02}
}
func validate(c Config) error {
	if c.Width < 8 || c.Height < 8 || c.Width > 2048 || c.Height > 2048 {
		return errors.New("dimensions must be between 8 and 2048")
	}
	if c.Width*c.Height > 2000000 {
		return errors.New("grid exceeds 2 million cells")
	}
	if c.Feed < 0 || c.Feed > 0.2 || c.Kill < 0 || c.Kill > 0.2 {
		return errors.New("feed and kill must be within 0..0.2")
	}
	if c.DiffA <= 0 || c.DiffB <= 0 || c.DT <= 0 || c.DT > 1 {
		return errors.New("diffusion and dt must be positive, dt <= 1")
	}
	if c.FPS < 1 || c.FPS > 120 || c.Steps < 1 || c.Steps > 1000 || c.Scale < 1 || c.Scale > 20 {
		return errors.New("invalid fps, steps or scale")
	}
	if c.Noise < 0 || c.Noise > 1 {
		return errors.New("noise must be 0..1")
	}
	if c.Boundary != "wrap" && c.Boundary != "clamp" && c.Boundary != "mirror" {
		return errors.New("boundary must be wrap, clamp or mirror")
	}
	if _, ok := palettes[c.Palette]; !ok {
		return fmt.Errorf("unknown palette %q", c.Palette)
	}
	return nil
}
func newModel(c Config) *Model {
	n := c.Width * c.Height
	m := &Model{C: c, A: make([]float64, n), B: make([]float64, n), NA: make([]float64, n), NB: make([]float64, n), RNG: rand.New(rand.NewSource(c.Seed))}
	for i := range m.A {
		m.A[i] = 1
	}
	m.SeedPattern()
	return m
}
func (m *Model) SeedPattern() {
	w, h := m.C.Width, m.C.Height
	r := m.RNG
	count := int(math.Max(3, float64(w*h)/2400))
	for j := 0; j < count; j++ {
		cx := w/2 + r.Intn(max(1, w/2)) - w/4
		cy := h/2 + r.Intn(max(1, h/2)) - h/4
		radius := 2 + r.Intn(max(2, min(w, h)/14))
		for y := cy - radius; y <= cy+radius; y++ {
			for x := cx - radius; x <= cx+radius; x++ {
				if x < 0 || y < 0 || x >= w || y >= h {
					continue
				}
				dx, dy := x-cx, y-cy
				if dx*dx+dy*dy <= radius*radius {
					idx := y*w + x
					m.A[idx] = 0.3 + r.Float64()*0.2
					m.B[idx] = 0.6 + r.Float64()*0.4
				}
			}
		}
	}
	for i := range m.A {
		if r.Float64() < m.C.Noise {
			m.B[i] = clamp(m.B[i]+r.Float64()*0.25, 0, 1)
			m.A[i] = clamp(m.A[i]-r.Float64()*0.1, 0, 1)
		}
	}
}
func (m *Model) coord(x, maxv int) int {
	switch m.C.Boundary {
	case "wrap":
		x %= maxv
		if x < 0 {
			x += maxv
		}
		return x
	case "mirror":
		if x < 0 {
			x = -x
		}
		if x >= maxv {
			x = 2*maxv - x - 2
		}
		return max(0, min(maxv-1, x))
	default:
		return max(0, min(maxv-1, x))
	}
}
func (m *Model) at(field []float64, x, y int) float64 {
	return field[m.coord(y, m.C.Height)*m.C.Width+m.coord(x, m.C.Width)]
}
func (m *Model) lap(f []float64, x, y int) float64 {
	v := -f[y*m.C.Width+x]
	v += 0.2 * (m.at(f, x-1, y) + m.at(f, x+1, y) + m.at(f, x, y-1) + m.at(f, x, y+1))
	v += 0.05 * (m.at(f, x-1, y-1) + m.at(f, x+1, y-1) + m.at(f, x-1, y+1) + m.at(f, x+1, y+1))
	return v
}
func (m *Model) Step() {
	c := m.C
	for y := 0; y < c.Height; y++ {
		for x := 0; x < c.Width; x++ {
			i := y*c.Width + x
			a, b := m.A[i], m.B[i]
			reaction := a * b * b
			m.NA[i] = clamp(a+(c.DiffA*m.lap(m.A, x, y)-reaction+c.Feed*(1-a))*c.DT, 0, 1)
			m.NB[i] = clamp(b+(c.DiffB*m.lap(m.B, x, y)+reaction-(c.Kill+c.Feed)*b)*c.DT, 0, 1)
		}
	}
	m.A, m.NA = m.NA, m.A
	m.B, m.NB = m.NB, m.B
	m.Iteration++
}
func (m *Model) Advance(n int) {
	for i := 0; i < n; i++ {
		m.Step()
	}
}
func (m *Model) Stats() Stats {
	s := Stats{Iteration: m.Iteration, MinB: 1, Width: m.C.Width, Height: m.C.Height, Feed: m.C.Feed, Kill: m.C.Kill}
	for i, v := range m.B {
		s.MeanB += v
		s.MeanA += m.A[i]
		s.MinB = math.Min(s.MinB, v)
		s.MaxB = math.Max(s.MaxB, v)
		if v > 0.1 {
			s.ActiveCells++
		}
	}
	n := float64(len(m.B))
	s.MeanA /= n
	s.MeanB /= n
	for _, v := range m.B {
		d := v - s.MeanB
		s.VarianceB += d * d
	}
	s.VarianceB /= n
	return s
}
func (m *Model) Snapshot() Snapshot {
	return Snapshot{Version: 1, Created: time.Now().UTC().Format(time.RFC3339), Config: m.C, Iteration: m.Iteration, A: m.A, B: m.B}
}
func writeJSON(path string, v any) error {
	data, e := json.MarshalIndent(v, "", "  ")
	if e != nil {
		return e
	}
	if path == "-" {
		_, e = os.Stdout.Write(append(data, '\n'))
		return e
	}
	return os.WriteFile(path, append(data, '\n'), 0644)
}
func load(path string) (*Model, error) {
	data, e := os.ReadFile(path)
	if e != nil {
		return nil, e
	}
	var s Snapshot
	if e = json.Unmarshal(data, &s); e != nil {
		return nil, e
	}
	if s.Version != 1 {
		return nil, errors.New("unsupported snapshot version")
	}
	if e = validate(s.Config); e != nil {
		return nil, e
	}
	n := s.Config.Width * s.Config.Height
	if len(s.A) != n || len(s.B) != n {
		return nil, errors.New("snapshot array dimensions mismatch")
	}
	m := &Model{C: s.Config, A: s.A, B: s.B, NA: make([]float64, n), NB: make([]float64, n), Iteration: s.Iteration, RNG: rand.New(rand.NewSource(s.Config.Seed))}
	return m, nil
}
func (m *Model) RenderANSI(showStats bool) string {
	var b strings.Builder
	w, h := m.C.Width, m.C.Height
	b.Grow(w * h * 35)
	b.WriteString("\x1b[H")
	for y := 0; y < h; y += 2 {
		for x := 0; x < w; x++ {
			top := color(clamp(m.B[y*w+x]*2.4, 0, 1), m.C.Palette)
			bot := RGB{}
			if y+1 < h {
				bot = color(clamp(m.B[(y+1)*w+x]*2.4, 0, 1), m.C.Palette)
			}
			fmt.Fprintf(&b, "\x1b[38;2;%d;%d;%dm\x1b[48;2;%d;%d;%dm▀", top.R, top.G, top.B, bot.R, bot.G, bot.B)
		}
		b.WriteString("\x1b[0m\x1b[K\n")
	}
	if showStats {
		s := m.Stats()
		fmt.Fprintf(&b, "\x1b[0m\x1b[K Morphogenesis Laboratory  preset=%s  palette=%s  step=%d  feed=%.4f  kill=%.4f  meanB=%.4f  active=%d  [q quit | space pause | +/- speed | p palette | r reseed]\x1b[K", m.C.Preset, m.C.Palette, m.Iteration, m.C.Feed, m.C.Kill, s.MeanB, s.ActiveCells)
	}
	return b.String()
}
func (m *Model) ExportPPM(path string, scale int) error {
	f, e := os.Create(path)
	if e != nil {
		return e
	}
	defer f.Close()
	w, h := m.C.Width, m.C.Height
	bw := bufio.NewWriterSize(f, 1<<20)
	fmt.Fprintf(bw, "P6\n%d %d\n255\n", w*scale, h*scale)
	line := make([]byte, w*scale*3)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			c := color(clamp(m.B[y*w+x]*2.4, 0, 1), m.C.Palette)
			for sx := 0; sx < scale; sx++ {
				i := (x*scale + sx) * 3
				line[i], line[i+1], line[i+2] = c.R, c.G, c.B
			}
		}
		for sy := 0; sy < scale; sy++ {
			if _, e = bw.Write(line); e != nil {
				return e
			}
		}
	}
	return bw.Flush()
}
func (m *Model) ExportASCII(path string) error {
	runes := []rune(" .,:;irsXA253hMHGS#9B&@")
	var b strings.Builder
	for y := 0; y < m.C.Height; y++ {
		for x := 0; x < m.C.Width; x++ {
			v := clamp(m.B[y*m.C.Width+x]*2.4, 0, 1)
			idx := int(v * float64(len(runes)-1))
			b.WriteRune(runes[idx])
		}
		b.WriteByte('\n')
	}
	if path == "-" {
		fmt.Print(b.String())
		return nil
	}
	return os.WriteFile(path, []byte(b.String()), 0644)
}
func paletteNames() []string {
	return []string{"aurora", "ember", "moon", "orchid", "forest", "ice", "mono"}
}
func parseConfig(args []string) (Config, string, int, error) {
	c := defaults()
	fs := flag.NewFlagSet("options", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	fs.IntVar(&c.Width, "width", c.Width, "simulation width")
	fs.IntVar(&c.Height, "height", c.Height, "simulation height")
	fs.Float64Var(&c.Feed, "feed", c.Feed, "chemical feed rate")
	fs.Float64Var(&c.Kill, "kill", c.Kill, "chemical kill rate")
	fs.Float64Var(&c.DiffA, "diff-a", c.DiffA, "A diffusion coefficient")
	fs.Float64Var(&c.DiffB, "diff-b", c.DiffB, "B diffusion coefficient")
	fs.Float64Var(&c.DT, "dt", c.DT, "integration timestep")
	fs.Int64Var(&c.Seed, "seed", c.Seed, "random seed")
	fs.StringVar(&c.Preset, "preset", c.Preset, "reaction preset")
	fs.StringVar(&c.Boundary, "boundary", c.Boundary, "wrap, clamp, mirror")
	fs.StringVar(&c.Palette, "palette", c.Palette, "color palette")
	fs.IntVar(&c.Steps, "steps", c.Steps, "simulation steps per frame")
	fs.IntVar(&c.FPS, "fps", c.FPS, "target frames per second")
	fs.IntVar(&c.Scale, "scale", c.Scale, "image scale")
	fs.Float64Var(&c.Noise, "noise", c.Noise, "initial noise probability")
	out := fs.String("out", "", "output file")
	frames := fs.Int("frames", 300, "number of frames or iterations depending on command")
	if e := fs.Parse(args); e != nil {
		return c, "", 0, e
	}
	if p, ok := presets[c.Preset]; ok {
		explicitFeed, explicitKill := false, false
		fs.Visit(func(f *flag.Flag) {
			if f.Name == "feed" {
				explicitFeed = true
			}
			if f.Name == "kill" {
				explicitKill = true
			}
		})
		if !explicitFeed {
			c.Feed = p[0]
		}
		if !explicitKill {
			c.Kill = p[1]
		}
	} else if c.Preset != "custom" {
		return c, "", 0, fmt.Errorf("unknown preset %q", c.Preset)
	}
	if e := validate(c); e != nil {
		return c, "", 0, e
	}
	if *frames < 0 || *frames > 10000000 {
		return c, "", 0, errors.New("frames out of range")
	}
	return c, *out, *frames, nil
}
func terminalSize() (int, int) {
	w, h := 80, 24
	var ws struct{ Row, Col, X, Y uint16 }
	_, _, errno := syscall.Syscall(syscall.SYS_IOCTL, uintptr(os.Stdout.Fd()), uintptr(syscall.TIOCGWINSZ), uintptr(unsafe.Pointer(&ws)))
	if errno == 0 && ws.Col > 0 {
		w = int(ws.Col)
		h = int(ws.Row)
	}
	return w, h
}
func setupTerminal() (func(), error) {
	fd := int(os.Stdin.Fd())
	var old syscall.Termios
	_, _, errno := syscall.Syscall(syscall.SYS_IOCTL, uintptr(fd), uintptr(syscall.TCGETS), uintptr(unsafe.Pointer(&old)))
	if errno != 0 {
		return func() {}, errno
	}
	raw := old
	raw.Lflag &^= syscall.ICANON | syscall.ECHO
	raw.Cc[syscall.VMIN] = 0
	raw.Cc[syscall.VTIME] = 0
	_, _, errno = syscall.Syscall(syscall.SYS_IOCTL, uintptr(fd), uintptr(syscall.TCSETS), uintptr(unsafe.Pointer(&raw)))
	if errno != 0 {
		return func() {}, errno
	}
	return func() {
		syscall.Syscall(syscall.SYS_IOCTL, uintptr(fd), uintptr(syscall.TCSETS), uintptr(unsafe.Pointer(&old)))
	}, nil
}
func animate(m *Model, frames int) error {
	tw, th := terminalSize()
	if m.C.Width > tw || ((m.C.Height+1)/2+2) > th {
		fmt.Fprintf(os.Stderr, "warning: terminal %dx%d smaller than rendered %dx%d; use --width and --height\n", tw, th, m.C.Width, (m.C.Height+1)/2+1)
	}
	restore, e := setupTerminal()
	if e != nil {
		fmt.Fprintln(os.Stderr, "warning: interactive controls unavailable:", e)
		restore = func() {}
	}
	defer restore()
	fmt.Print("\x1b[?1049h\x1b[?25l\x1b[2J")
	defer fmt.Print("\x1b[0m\x1b[?25h\x1b[?1049l")
	sig := make(chan os.Signal, 2)
	signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(sig)
	input := make(chan byte, 128)
	stop := make(chan struct{})
	defer close(stop)
	go func() {
		buf := make([]byte, 64)
		for {
			select {
			case <-stop:
				return
			default:
			}
			n, e := os.Stdin.Read(buf)
			if e != nil {
				return
			}
			for _, v := range buf[:n] {
				select {
				case input <- v:
				default:
				}
			}
			if n == 0 {
				time.Sleep(25 * time.Millisecond)
			}
		}
	}()
	paused := false
	steps := m.C.Steps
	names := paletteNames()
	frameDuration := time.Second / time.Duration(m.C.FPS)
	next := time.Now()
	for frame := 0; frames == 0 || frame < frames; frame++ {
		select {
		case <-sig:
			return nil
		default:
		}
		for {
			select {
			case key := <-input:
				switch key {
				case 'q', 'Q':
					return nil
				case ' ':
					paused = !paused
				case '+', '=':
					steps = min(1000, steps+1)
				case '-', '_':
					steps = max(1, steps-1)
				case 'p', 'P':
					for i, v := range names {
						if v == m.C.Palette {
							m.C.Palette = names[(i+1)%len(names)]
							break
						}
					}
				case 'r', 'R':
					m = newModel(m.C)
				}
			default:
				goto render
			}
		}
	render:
		if !paused {
			m.Advance(steps)
		}
		fmt.Print(m.RenderANSI(true))
		next = next.Add(frameDuration)
		if delay := time.Until(next); delay > 0 {
			time.Sleep(delay)
		} else {
			next = time.Now()
		}
	}
	return nil
}
func usage() {
	fmt.Println(`Morphogenesis Laboratory
Usage: morphogenesis <command> [flags]
Commands:
  run         Animate in an ANSI truecolor terminal
  render      Render a PPM image after simulation steps
  ascii       Export ASCII art
  snapshot    Save full JSON simulation state
  resume      Load snapshot, advance and save or render
  stats       Print simulation statistics as JSON
  presets     List reaction parameter presets
  palettes    List available color palettes
  benchmark   Measure simulation throughput
  help        Show help
Common flags: --width --height --preset --palette --seed --steps --fps --frames --out --feed --kill --diff-a --diff-b --dt --noise --boundary --scale
Examples:
  morphogenesis run --preset labyrinth --palette ember --width 100 --height 42
  morphogenesis render --preset coral --frames 3000 --out coral.ppm
  morphogenesis snapshot --frames 1500 --out state.json
  morphogenesis resume --in state.json --frames 1000 --out next.json
  morphogenesis stats --preset spots --frames 2000`)
}
func mainErr() error {
	if len(os.Args) < 2 {
		usage()
		return nil
	}
	cmd := os.Args[1]
	if cmd == "help" || cmd == "-h" || cmd == "--help" {
		usage()
		return nil
	}
	if cmd == "presets" {
		for _, name := range sortedPresets() {
			v := presets[name]
			fmt.Printf("%-16s feed=%.5f kill=%.5f\n", name, v[0], v[1])
		}
		return nil
	}
	if cmd == "palettes" {
		fmt.Println(strings.Join(paletteNames(), "\n"))
		return nil
	}
	args := os.Args[2:]
	in := ""
	if cmd == "resume" {
		for i := 0; i < len(args); i++ {
			if args[i] == "--in" && i+1 < len(args) {
				in = args[i+1]
				args = append(append([]string{}, args[:i]...), args[i+2:]...)
				break
			}
			if strings.HasPrefix(args[i], "--in=") {
				in = strings.TrimPrefix(args[i], "--in=")
				args = append(append([]string{}, args[:i]...), args[i+1:]...)
				break
			}
		}
		if in == "" {
			return errors.New("resume requires --in FILE")
		}
	}
	c, out, frames, e := parseConfig(args)
	if e != nil {
		return e
	}
	var m *Model
	if cmd == "resume" {
		m, e = load(in)
		if e != nil {
			return e
		}

	} else {
		m = newModel(c)
	}
	switch cmd {
	case "run":
		return animate(m, frames)
	case "render":
		if out == "" {
			out = "morphogenesis.ppm"
		}
		m.Advance(frames)
		return m.ExportPPM(out, c.Scale)
	case "ascii":
		m.Advance(frames)
		if out == "" {
			out = "-"
		}
		return m.ExportASCII(out)
	case "snapshot":
		if out == "" {
			out = "morphogenesis.json"
		}
		m.Advance(frames)
		return writeJSON(out, m.Snapshot())
	case "resume":
		m.Advance(frames)
		if out == "" {
			out = "resumed.json"
		}
		switch strings.ToLower(filepath.Ext(out)) {
		case ".ppm":
			return m.ExportPPM(out, c.Scale)
		case ".txt":
			return m.ExportASCII(out)
		default:
			return writeJSON(out, m.Snapshot())
		}
	case "stats":
		m.Advance(frames)
		return writeJSON("-", m.Stats())
	case "benchmark":
		start := time.Now()
		m.Advance(frames)
		elapsed := time.Since(start)
		fmt.Printf("iterations=%d cells=%d elapsed=%s cell_updates_per_second=%.0f go=%s\n", frames, c.Width*c.Height, elapsed, float64(frames*c.Width*c.Height)/math.Max(elapsed.Seconds(), 1e-9), runtime.Version())
		return nil
	default:
		usage()
		return fmt.Errorf("unknown command %q", cmd)
	}
}
func sortedPresets() []string {
	names := make([]string, 0, len(presets))
	for k := range presets {
		names = append(names, k)
	}
	sort.Strings(names)
	return names
}
func main() {
	if e := mainErr(); e != nil {
		fmt.Fprintln(os.Stderr, "error:", e)
		os.Exit(1)
	}
}
