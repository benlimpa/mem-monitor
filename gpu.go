package main

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/shirou/gopsutil/v3/process"
)

type GPUInfo struct {
	VRAMTotal uint64
	VRAMUsed  uint64
	GTTTotal  uint64
	GTTUsed   uint64
}

func GetGPUStats() (GPUInfo, error) {
	var info GPUInfo

	// Find the first amdgpu card
	cards, err := filepath.Glob("/sys/class/drm/card*/device/mem_info_vram_used")
	if err != nil || len(cards) == 0 {
		return info, fmt.Errorf("no AMD GPU found in sysfs")
	}

	deviceDir := filepath.Dir(cards[0])

	info.VRAMUsed = readUint64(filepath.Join(deviceDir, "mem_info_vram_used"))
	info.VRAMTotal = readUint64(filepath.Join(deviceDir, "mem_info_vram_total"))
	info.GTTUsed = readUint64(filepath.Join(deviceDir, "mem_info_gtt_used"))
	info.GTTTotal = readUint64(filepath.Join(deviceDir, "mem_info_gtt_total"))

	return info, nil
}

func readUint64(path string) uint64 {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0
	}
	val, _ := strconv.ParseUint(strings.TrimSpace(string(data)), 10, 64)
	return val
}

type ProcessGPUInfo struct {
	PID  int32
	Name string
	VRAM uint64
	GTT  uint64
	RAM  uint64
}

func GetProcessBreakdown() ([]ProcessGPUInfo, error) {
	var results []ProcessGPUInfo

	procs, err := process.Processes()
	if err != nil {
		return nil, err
	}

	for _, p := range procs {
		pid := p.Pid
		fdinfoDir := filepath.Join("/proc", strconv.Itoa(int(pid)), "fdinfo")
		fds, err := os.ReadDir(fdinfoDir)
		if err != nil {
			continue // Likely permission denied or process ended
		}

		var vram, gtt uint64
		foundAMD := false
		for _, fd := range fds {
			v, g, ok := parseFdInfo(filepath.Join(fdinfoDir, fd.Name()))
			if ok {
				vram += v
				gtt += g
				foundAMD = true
			}
		}

		if foundAMD || true { // We want all processes or just AMD? Let's show all for context if they have RAM
			memInfo, _ := p.MemoryInfo()
			var rss uint64
			if memInfo != nil {
				rss = memInfo.RSS
			}

			// Only add if it uses some significant memory to avoid noise
			if vram > 0 || gtt > 0 || rss > 1024*1024 {
				cmdline, _ := p.Cmdline()
				if cmdline == "" {
					cmdline, _ = p.Name()
				}

				// Subtract GTT from RAM for consistent reporting on unified systems
				ram := rss
				if ram > gtt {
					ram -= gtt
				} else {
					ram = 0
				}

				results = append(results, ProcessGPUInfo{
					PID:  pid,
					Name: cmdline,
					VRAM: vram,
					GTT:  gtt,
					RAM:  ram,
				})
			}
		}
	}

	return results, nil
}

func parseFdInfo(path string) (vram, gtt uint64, ok bool) {
	file, err := os.Open(path)
	if err != nil {
		return 0, 0, false
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "drm-driver:	amdgpu") {
			ok = true
		}
		if strings.HasPrefix(line, "drm-memory-vram:") {
			parts := strings.Fields(line)
			if len(parts) >= 2 {
				val, _ := strconv.ParseUint(parts[1], 10, 64)
				vram += val * 1024 // Assuming KiB if not specified, check unit
			}
		}
		if strings.HasPrefix(line, "drm-memory-gtt:") {
			parts := strings.Fields(line)
			if len(parts) >= 2 {
				val, _ := strconv.ParseUint(parts[1], 10, 64)
				gtt += val * 1024
			}
		}
	}
	return vram, gtt, ok
}
