package main

import (
	"fmt"
	"log"
	"os"
	"sort"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/shirou/gopsutil/v3/mem"
)

type model struct {
	totalRAM     uint64
	usedRAM      uint64
	gpuInfo      GPUInfo
	processes    []ProcessGPUInfo
	err          error
	isPrivileged bool
	sortBy       string // "RAM", "GTT", "VRAM"
}

type tickMsg struct {
	totalRAM  uint64
	usedRAM   uint64
	gpuInfo   GPUInfo
	processes []ProcessGPUInfo
	err       error
}

func (m model) Init() tea.Cmd {
	return tick()
}

func tick() tea.Cmd {
	return tea.Every(time.Second, func(t time.Time) tea.Msg {
		v, err := mem.VirtualMemory()
		if err != nil {
			return tickMsg{err: err}
		}

		gpu, _ := GetGPUStats()
		procs, _ := GetProcessBreakdown()

		return tickMsg{
			totalRAM:  v.Total,
			usedRAM:   v.Used,
			gpuInfo:   gpu,
			processes: procs,
		}
	})
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit
		case "r":
			m.sortBy = "RAM"
		case "g":
			m.sortBy = "GTT"
		case "v":
			m.sortBy = "VRAM"
		}
	case tickMsg:
		if msg.err != nil {
			m.err = msg.err
		} else {
			m.totalRAM = msg.totalRAM
			m.usedRAM = msg.usedRAM
			m.gpuInfo = msg.gpuInfo
			m.processes = msg.processes

			sort.Slice(m.processes, func(i, j int) bool {
				switch m.sortBy {
				case "RAM":
					return m.processes[i].RAM > m.processes[j].RAM
				case "VRAM":
					return m.processes[i].VRAM > m.processes[j].VRAM
				default: // GTT is default
					return m.processes[i].GTT > m.processes[j].GTT
				}
			})
		}
		return m, tick()
	}
	return m, nil
}

var (
	titleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#FAFAFA")).
			Background(lipgloss.Color("#7D56F4")).
			Padding(0, 1)
	headerStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#7D56F4"))
	activeHeaderStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(lipgloss.Color("#FAFAFA")).
				Background(lipgloss.Color("#7D56F4"))
	infoStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#04B575"))
)

func formatName(name string, maxLen int) string {
	if len(name) <= maxLen {
		return name
	}
	if maxLen <= 3 {
		return name[:maxLen]
	}
	// Show beginning and end with ... in between
	side := (maxLen - 3) / 2
	return name[:side] + "..." + name[len(name)-side:]
}

func (m model) View() string {
	if m.err != nil {
		return fmt.Sprintf("Error: %v\n", m.err)
	}

	// Calculate breakdown for unified memory systems
	// Total Physical = OS Visible RAM + Hardware Reserved VRAM
	physicalTotal := m.totalRAM + m.gpuInfo.VRAMTotal

	gpuInRAM := m.gpuInfo.GTTTotal
	systemUsed := uint64(0)
	if m.usedRAM > gpuInRAM {
		systemUsed = m.usedRAM - gpuInRAM
	}
	systemUsedPercent := float64(systemUsed) / float64(m.totalRAM) * 100
	gttOfSystemPercent := float64(gpuInRAM) / float64(m.totalRAM) * 100

	s := titleStyle.Render("Memory Monitor") + "\n\n"
	s += headerStyle.Render("Physical Memory Breakdown") + "\n"
	s += fmt.Sprintf("Total Physical RAM: %s\n", formatBytes(physicalTotal))
	s += fmt.Sprintf("  ├─ OS Visible:     %s (%.1f%%)\n", formatBytes(m.totalRAM), float64(m.totalRAM)/float64(physicalTotal)*100)
	s += fmt.Sprintf("  │   ├─ System:     %s (%.1f%%)\n", formatBytes(systemUsed), systemUsedPercent)
	s += fmt.Sprintf("  │   └─ GPU GTT:    %s (%.1f%%)\n", formatBytes(gpuInRAM), gttOfSystemPercent)
	s += fmt.Sprintf("  └─ Hardware Res:   %s (Fixed VRAM)\n", formatBytes(m.gpuInfo.VRAMTotal))

	s += "\n" + headerStyle.Render("AMD GPU Memory Status") + "\n"
	s += fmt.Sprintf("VRAM (Dedicated): %s / %s\n", formatBytes(m.gpuInfo.VRAMUsed), formatBytes(m.gpuInfo.VRAMTotal))
	s += fmt.Sprintf("GTT  (Shared):    %s / %s\n", formatBytes(m.gpuInfo.GTTUsed), formatBytes(m.gpuInfo.GTTTotal))

	if !m.isPrivileged {
		s += "\n[!] Run with sudo for full process breakdown.\n"
	} else if len(m.processes) > 0 {
		s += "\n" + headerStyle.Render(fmt.Sprintf("Top Processes (Sorted by %s)", m.sortBy)) + "\n"

		// Header row with active column highlighting
		vramHead := "VRAM"
		gttHead := "GTT"
		ramHead := "RAM"
		if m.sortBy == "VRAM" {
			vramHead = activeHeaderStyle.Render("VRAM")
		}
		if m.sortBy == "GTT" {
			gttHead = activeHeaderStyle.Render("GTT")
		}
		if m.sortBy == "RAM" {
			ramHead = activeHeaderStyle.Render("RAM")
		}

		s += fmt.Sprintf("%-6s %-40s %-12s %-12s %-12s\n", "PID", "COMMAND", vramHead, gttHead, ramHead)

		limit := 15
		if len(m.processes) < limit {
			limit = len(m.processes)
		}
		for i := 0; i < limit; i++ {
			p := m.processes[i]
			displayName := formatName(p.Name, 40)
			s += fmt.Sprintf("%-6d %-40s %-12s %-12s %-12s\n", p.PID, displayName, formatBytes(p.VRAM), formatBytes(p.GTT), formatBytes(p.RAM))
		}
	}

	s += "\nSort: [r] RAM, [g] GTT, [v] VRAM | Quit: [q]\n"
	return s
}

func formatBytes(b uint64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := uint64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %ciB", float64(b)/float64(div), "KMGTPE"[exp])
}

func main() {
	m := model{
		isPrivileged: os.Geteuid() == 0,
		sortBy:       "RAM",
	}

	p := tea.NewProgram(m)
	if _, err := p.Run(); err != nil {
		log.Fatal(err)
	}
}
