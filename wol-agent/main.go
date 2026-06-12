package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"math"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/shirou/gopsutil/v4/cpu"
	"github.com/shirou/gopsutil/v4/disk"
	"github.com/shirou/gopsutil/v4/mem"
	"github.com/shirou/gopsutil/v4/net"
)

var startTime = time.Now()

type StatusResponse struct {
	Online   bool   `json:"online"`
	Hostname string `json:"hostname"`
	OS       string `json:"os"`
	Uptime   int64  `json:"uptime"`
}

type ActionResponse struct {
	Success bool   `json:"success"`
	Action  string `json:"action"`
	Delay   int    `json:"delay"`
	Message string `json:"message"`
}

type CPUStats struct {
	Model        string  `json:"model"`
	Cores        int     `json:"cores"`
	UsagePercent float64 `json:"usage_percent"`
}

type MemoryStats struct {
	TotalGB      float64 `json:"total_gb"`
	UsedGB       float64 `json:"used_gb"`
	UsagePercent float64 `json:"usage_percent"`
	DDRType      string  `json:"ddr_type"`
	SpeedMHz     int     `json:"speed_mhz"`
}

type GPUStats struct {
	Model        string   `json:"model"`
	VRAMTotalGB  float64  `json:"vram_total_gb"`
	UsagePercent *float64 `json:"usage_percent"`
}

type PartitionInfo struct {
	Name    string  `json:"name"`
	TotalGB float64 `json:"total_gb"`
	UsedGB  float64 `json:"used_gb"`
}

type DiskStats struct {
	Name       string          `json:"name"`
	Model      string          `json:"model"`
	TotalGB    float64         `json:"total_gb"`
	UsedGB     float64         `json:"used_gb"`
	ReadBytes  uint64          `json:"read_bytes"`
	WriteBytes uint64          `json:"write_bytes"`
	Partitions []PartitionInfo `json:"partitions"`
}

type NetStats struct {
	Name      string `json:"name"`
	Desc      string `json:"desc"`
	RecvBytes uint64 `json:"recv_bytes"`
	SentBytes uint64 `json:"sent_bytes"`
}

type StatsResponse struct {
	CPU     CPUStats    `json:"cpu"`
	Memory  MemoryStats `json:"memory"`
	GPU     *GPUStats   `json:"gpu"`
	Disks   []DiskStats `json:"disks"`
	Network []NetStats  `json:"network"`
}

func gb(b uint64) float64 {
	return math.Round(float64(b)/1073741824*10) / 10
}

func handleStatus(w http.ResponseWriter, r *http.Request) {
	hostname, _ := os.Hostname()
	resp := StatusResponse{
		Online:   true,
		Hostname: hostname,
		OS:       runtime.GOOS,
		Uptime:   int64(time.Since(startTime).Seconds()),
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

func handleShutdown(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	resp := ActionResponse{
		Success: true,
		Action:  "shutdown",
		Delay:   5,
		Message: fmt.Sprintf("System will shutdown in %d seconds", 5),
	}

	go func() {
		time.Sleep(500 * time.Millisecond)
		exec.Command("shutdown", "/s", "/t", "5").Run()
	}()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

func collectStats() StatsResponse {
	s := StatsResponse{}

	// --- CPU ---
	if infos, err := cpu.Info(); err == nil && len(infos) > 0 {
		s.CPU.Model = infos[0].ModelName
		logicalCores, _ := cpu.Counts(true)
		s.CPU.Cores = logicalCores
	}
	if pcts, err := cpu.Percent(0, false); err == nil && len(pcts) > 0 {
		s.CPU.UsagePercent = math.Round(pcts[0]*10) / 10
	}

	// --- Memory ---
	if vm, err := mem.VirtualMemory(); err == nil {
		s.Memory.TotalGB = gb(vm.Total)
		s.Memory.UsedGB = gb(vm.Used)
		s.Memory.UsagePercent = math.Round(vm.UsedPercent*10) / 10
	}
	if ddrType, speed := collectMemoryInfo(); ddrType != "" {
		s.Memory.DDRType = ddrType
		s.Memory.SpeedMHz = speed
	}

	// --- GPU ---
	s.GPU = collectGPU()

	// --- Disks ---
	s.Disks = collectDisks()

	// --- Network ---
	s.Network = collectNetwork()

	return s
}

// parseWmicCSV parses wmic /format:csv output. Returns column index map and data rows.
// wmic sorts columns ALPHABETICALLY, so we must parse by header name.
func parseWmicCSV(out []byte) (colIdx map[string]int, rows [][]string) {
	colIdx = make(map[string]int)
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	foundHeader := false
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := splitCSVLine(line)
		if !foundHeader && len(parts) >= 2 && strings.EqualFold(strings.TrimSpace(parts[0]), "node") {
			for i, h := range parts {
				colIdx[strings.TrimSpace(h)] = i
			}
			foundHeader = true
			continue
		}
		if foundHeader {
			rows = append(rows, parts)
		}
	}
	return
}

func colVal(row []string, colIdx map[string]int, col string) string {
	if idx, ok := colIdx[col]; ok && idx < len(row) {
		return strings.TrimSpace(row[idx])
	}
	return ""
}

func splitCSVLine(line string) []string {
	// Simple CSV split that handles the wmic format (comma-separated, no quoting needed for these fields)
	return strings.Split(line, ",")
}

func collectGPU() *GPUStats {
	colIdx, rows := parseWmicCSV(execWmic("path", "Win32_VideoController", "get", "Name,AdapterRAM", "/format:csv"))
	if len(colIdx) == 0 {
		return nil
	}

	// First pass: find a real GPU with VRAM > 0
	var fallback *GPUStats
	for _, row := range rows {
		model := colVal(row, colIdx, "Name")
		if model == "" {
			continue
		}
		low := strings.ToLower(model)
		if strings.Contains(low, "microsoft") || strings.Contains(low, "virtual") ||
			strings.Contains(low, "oray") || strings.Contains(low, "gameviewer") ||
			strings.Contains(low, "idd") {
			continue
		}

		var g GPUStats
		g.Model = model
		if ramStr := colVal(row, colIdx, "AdapterRAM"); ramStr != "" {
			if ramBytes, err := strconv.ParseUint(ramStr, 10, 64); err == nil && ramBytes > 0 {
				g.VRAMTotalGB = gb(ramBytes)
			}
		}
		if g.VRAMTotalGB > 0 {
			// Real GPU found — use it immediately
			usage := collectGPUUsage()
			if usage >= 0 {
				g.UsagePercent = &usage
			}
			return &g
		}
		// Save as fallback (GPU with name but no VRAM reported)
		if fallback == nil {
			fallback = &g
		}
	}

	// Fallback: use first non-virtual GPU even without VRAM info
	if fallback != nil && fallback.Model != "" {
		usage := collectGPUUsage()
		if usage >= 0 {
			fallback.UsagePercent = &usage
		}
		return fallback
	}
	return nil
}

func execWmic(args ...string) []byte {
	var stdout, stderr bytes.Buffer
	cmd := exec.Command("wmic", args...)
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		log.Printf("wmic %v failed: %v, stderr: %s", args, err, strings.TrimSpace(stderr.String()))
		return nil
	}
	return stdout.Bytes()
}

func collectGPUUsage() float64 {
	colIdx, rows := parseWmicCSV(execWmic("path", "Win32_PerfFormattedData_GPUPerformanceCounters_GPUEngine", "get", "UtilizationPercentage", "/format:csv"))
	if len(colIdx) == 0 {
		return -1
	}
	var maxPct float64 = -1
	for _, row := range rows {
		for _, col := range []string{"UtilizationPercentage"} {
			if v, err := strconv.ParseFloat(colVal(row, colIdx, col), 64); err == nil && v > maxPct {
				maxPct = v
			}
		}
	}
	return maxPct
}

func collectMemoryInfo() (ddrType string, speedMHz int) {
	colIdx, rows := parseWmicCSV(execWmic("memorychip", "get", "SMBIOSMemoryType,Speed", "/format:csv"))
	if len(colIdx) == 0 {
		return "", 0
	}
	for _, row := range rows {
		tStr := colVal(row, colIdx, "SMBIOSMemoryType")
		if tStr == "" {
			continue
		}
		t, err := strconv.Atoi(tStr)
		if err != nil {
			continue
		}
		ddrType = smbiosToDDR(t)
		if sStr := colVal(row, colIdx, "Speed"); sStr != "" {
			if s, err := strconv.Atoi(sStr); err == nil {
				speedMHz = s
			}
		}
		return ddrType, speedMHz
	}
	return "", 0
}

func smbiosToDDR(t int) string {
	switch t {
	case 20:
		return "DDR"
	case 21:
		return "DDR2"
	case 22:
		return "DDR2"
	case 24:
		return "DDR3"
	case 25:
		return "DDR3"
	case 26:
		return "DDR4"
	case 27:
		return "LPDDR"
	case 28:
		return "LPDDR2"
	case 29:
		return "LPDDR3"
	case 30:
		return "LPDDR4"
	case 34:
		return "DDR5"
	case 35:
		return "LPDDR5"
	case 36, 37, 38:
		return "HBM"
	default:
		return ""
	}
}

func collectDisks() []DiskStats {
	partitions, err := disk.Partitions(false)
	if err != nil {
		return nil
	}

	ioCounters, _ := disk.IOCounters()

	// Build physical disk model map (best-effort, Windows only)
	diskModels := collectDiskModels()

	// Group partitions by physical disk
	// On Windows, gopsutil IOCounters keys are like \\.\PhysicalDrive0
	// Build a map from physical drive index to IO counters
	physIOMap := make(map[int]struct{ read, write uint64 })
	for name, io := range ioCounters {
		n := strings.ToUpper(name)
		n = strings.TrimPrefix(n, "\\\\.\\")
		n = strings.TrimPrefix(n, "\\\\")
		if idx, err := strconv.Atoi(strings.TrimPrefix(n, "PHYSICALDRIVE")); err == nil {
			physIOMap[idx] = struct{ read, write uint64 }{io.ReadBytes, io.WriteBytes}
		}
	}
	// Fallback: also try matching by mountpoint name (e.g. "C:" on non-Windows or alt APIs)
	mountIOMap := make(map[string]struct{ read, write uint64 })
	for name, io := range ioCounters {
		mountIOMap[strings.ToUpper(strings.TrimRight(name, "\\"))] = struct{ read, write uint64 }{io.ReadBytes, io.WriteBytes}
	}

	// Build partition-to-physical-disk mapping via WMI
	partToDiskIdx := buildPartitionDiskMap()
	hasMapping := len(partToDiskIdx) > 0

	var disks []DiskStats
	diskMap := make(map[string]*DiskStats) // key: "disk:N" or partition device name

	for _, p := range partitions {
		usage, err := disk.Usage(p.Mountpoint)
		if err != nil {
			continue
		}

		// Determine grouping key and disk index
		var groupKey string
		var diskIdx int
		if hasMapping {
			var ok bool
			diskIdx, ok = partToDiskIdx[p.Device]
			if !ok {
				// No mapping for this partition — group by device name alone
				groupKey = p.Device
			} else {
				groupKey = fmt.Sprintf("disk:%d", diskIdx)
			}
		} else {
			groupKey = p.Device
		}

		dk, ok := diskMap[groupKey]
		if !ok {
			model := diskModels[diskIdx]
			dk = &DiskStats{
				Name:  p.Device,
				Model: model,
			}
			// Apply IO counters for this physical disk (only once, not per partition)
			if io, ok := physIOMap[diskIdx]; ok {
				dk.ReadBytes = io.read
				dk.WriteBytes = io.write
			} else if io, ok := mountIOMap[strings.ToUpper(strings.TrimRight(p.Mountpoint, "\\"))]; ok {
				dk.ReadBytes = io.read
				dk.WriteBytes = io.write
			}
			diskMap[groupKey] = dk
		}

		dk.TotalGB += gb(usage.Total)
		dk.UsedGB += gb(usage.Used)
		dk.Partitions = append(dk.Partitions, PartitionInfo{
			Name:    strings.TrimRight(strings.TrimRight(p.Mountpoint, "\\"), ":"),
			TotalGB: gb(usage.Total),
			UsedGB:  gb(usage.Used),
		})
	}

	for _, dk := range diskMap {
		disks = append(disks, *dk)
	}
	return disks
}

func collectDiskModels() map[int]string {
	result := make(map[int]string)
	out, err := exec.Command("wmic", "diskdrive", "get", "Index,Model", "/format:csv").Output()
	if err != nil {
		return result
	}
	lines := strings.Split(string(out), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "Node,") || strings.HasPrefix(line, "Index,") {
			continue
		}
		parts := strings.SplitN(line, ",", 3)
		if len(parts) >= 3 {
			idx, err := strconv.Atoi(strings.TrimSpace(parts[1]))
			if err == nil {
				result[idx] = strings.TrimSpace(parts[2])
			}
		}
	}
	return result
}

func buildPartitionDiskMap() map[string]int {
	result := make(map[string]int)

	// Step 1: LogicalDisk -> Partition: wmic path Win32_LogicalDiskToPartition
	logicalToPart := make(map[string]string) // drive letter -> partition DeviceID
	out1, err := exec.Command("wmic", "path", "Win32_LogicalDiskToPartition", "get", "Antecedent,Dependent", "/format:csv").Output()
	if err == nil {
		lines := strings.Split(string(out1), "\n")
		for _, line := range lines {
			line = strings.TrimSpace(line)
			if line == "" || strings.HasPrefix(line, "Node,") {
				continue
			}
			// Parse: Node,Antecedent,Dependent
			// Antecedent = Win32_DiskPartition (Disk #0, Partition #0)
			// Dependent = Win32_LogicalDisk (DeviceID="C:")
			ante := extractBetweenQuotes(line, "Win32_DiskPartition.DeviceID=\"", "\"")
			dep := extractBetweenQuotes(line, "Win32_LogicalDisk.DeviceID=\"", "\"")
			// dep should be something like "C:" or "Disk #1, Partition #0"
			// Actually, looking at wmic output, Dependent is like:
			// \\HOST\root\cimv2:Win32_LogicalDisk.DeviceID="C:"
			if ante != "" && dep != "" {
				logicalToPart[dep] = ante
			}
		}
	}

	// Step 2: Partition -> DiskDrive: wmic path Win32_DiskDriveToDiskPartition
	partToDisk := make(map[string]int) // partition DeviceID -> disk index
	out2, err := exec.Command("wmic", "path", "Win32_DiskDriveToDiskPartition", "get", "Antecedent,Dependent", "/format:csv").Output()
	if err == nil {
		lines := strings.Split(string(out2), "\n")
		for _, line := range lines {
			line = strings.TrimSpace(line)
			if line == "" || strings.HasPrefix(line, "Node,") {
				continue
			}
			// Antecedent = Win32_DiskDrive (Index=0)
			// Dependent = Win32_DiskPartition (DeviceID="Disk #0, Partition #0")
			ante := extractBetweenQuotes(line, "Win32_DiskDrive.DeviceID=\"", "\"")
			dep := extractBetweenQuotes(line, "Win32_DiskPartition.DeviceID=\"", "\"")
			if ante != "" && dep != "" {
				// Parse disk index from ante: \\\\.\\PHYSICALDRIVE0
				ante = strings.TrimPrefix(ante, "\\\\")
				ante = strings.TrimPrefix(ante, ".")
				ante = strings.TrimPrefix(ante, "\\")
				if idx, err := strconv.Atoi(strings.TrimPrefix(strings.ToUpper(ante), "PHYSICALDRIVE")); err == nil {
					partToDisk[dep] = idx
				}
			}
		}
	}

	// Step 3: Build logical drive -> disk index map
	for drive, part := range logicalToPart {
		if idx, ok := partToDisk[part]; ok {
			result[drive] = idx
		}
	}

	return result
}

func extractBetweenQuotes(s, prefix, suffix string) string {
	start := strings.Index(s, prefix)
	if start < 0 {
		return ""
	}
	start += len(prefix)
	end := strings.Index(s[start:], suffix)
	if end < 0 {
		return ""
	}
	return s[start : start+end]
}

// collectNicDescriptions queries wmic for NIC index → description mapping.
// We match by InterfaceIndex (numeric, no encoding issues) instead of
// NetConnectionID which has GBK vs UTF-8 encoding mismatch with gopsutil.
func collectNicDescriptions() map[int]string {
	result := make(map[int]string)
	colIdx, rows := parseWmicCSV(execWmic("path", "Win32_NetworkAdapter", "get", "Description,InterfaceIndex", "/format:csv"))
	if len(colIdx) == 0 {
		return result
	}
	for _, row := range rows {
		idxStr := colVal(row, colIdx, "InterfaceIndex")
		desc := colVal(row, colIdx, "Description")
		if idxStr != "" {
			if idx, err := strconv.Atoi(idxStr); err == nil {
				result[idx] = desc
			}
		}
	}
	return result
}

func collectNetwork() []NetStats {
	counters, err := net.IOCounters(true)
	if err != nil {
		return nil
	}

	interfaces, _ := net.Interfaces()
	ifaceMap := make(map[string]net.InterfaceStat)
	for _, iface := range interfaces {
		ifaceMap[iface.Name] = iface
	}

	// Get NIC descriptions for virtual adapter detection
	// Match by InterfaceIndex (numeric) to avoid GBK vs UTF-8 encoding mismatch
	nicDescs := collectNicDescriptions()

	type nicEntry struct {
		name     string
		recv     uint64
		sent     uint64
		isFilter bool
		baseName string
	}
	var entries []nicEntry

	for _, c := range counters {
		iface, ok := ifaceMap[c.Name]
		if !ok {
			continue
		}
		allFlags := strings.Join(iface.Flags, ",")
		if allFlags == "" {
			continue
		}
		if strings.Contains(strings.ToLower(allFlags), "loopback") {
			continue
		}
		if strings.Contains(strings.ToLower(allFlags), "down") {
			continue
		}

		// Check both the adapter name and its hardware description
		name := c.Name
		nameLow := strings.ToLower(name)
		desc := nicDescs[iface.Index]
		descLow := strings.ToLower(desc)

		if isVirtualNic(nameLow, descLow) {
			continue
		}

		// Detect WFP/Native filter duplicates on Windows
		// e.g. "Ethernet-WFP Native MAC Layer LightWeight Filter-0000"
		isFilter := strings.Contains(nameLow, "wfp") ||
			strings.Contains(nameLow, "lightweight filter") ||
			strings.Contains(nameLow, "native mac layer") ||
			strings.Contains(nameLow, "qos packet scheduler") ||
			strings.Contains(nameLow, "network timestamp")

		baseName := name
		if isFilter {
			// Try to extract base adapter name
			if idx := strings.Index(name, "-WFP"); idx >= 0 {
				baseName = name[:idx]
			} else if idx := strings.Index(name, "-Native"); idx >= 0 {
				baseName = name[:idx]
			} else if idx := strings.Index(name, "-LightWeight"); idx >= 0 {
				baseName = name[:idx]
			} else if idx := strings.Index(name, "-QoS"); idx >= 0 {
				baseName = name[:idx]
			}
		}

		entries = append(entries, nicEntry{
			name:     name,
			recv:     c.BytesRecv,
			sent:     c.BytesSent,
			isFilter: isFilter,
			baseName: strings.TrimSpace(baseName),
		})
	}

	// Deduplicate: skip filter entries if a matching base adapter exists
	baseNames := make(map[string]bool)
	for _, e := range entries {
		if !e.isFilter {
			baseNames[strings.ToLower(e.baseName)] = true
		}
	}

	var result []NetStats
	for _, e := range entries {
		if e.isFilter && baseNames[strings.ToLower(e.baseName)] {
			continue
		}
		// Use hardware description as display name when available
		desc := ""
		if iface, ok := ifaceMap[e.name]; ok {
			if d, ok := nicDescs[iface.Index]; ok && !strings.EqualFold(d, e.name) {
				desc = d
			}
		}
		if desc == "" {
			desc = strings.TrimSuffix(e.name, ":") // fallback: clean adapter name
		}
		result = append(result, NetStats{
			Name:      e.name,
			Desc:      desc,
			RecvBytes: e.recv,
			SentBytes: e.sent,
		})
	}
	return result
}

// isVirtualNic checks both the adapter name and its hardware description
// against known virtual adapter patterns.
func isVirtualNic(nameLow, descLow string) bool {
	patterns := []string{
		"hyper-v", "wsl", "vmware", "virtualbox", "vbox",
		"tap", "tun", "vethernet", "docker", "bluetooth",
		"isatap", "teredo", "6to4", "loopback", "pseudo",
		"miniport", "wan miniport", "ras async",
		"host-only", "bridge", "nat", "virtual ethernet",
	}
	for _, p := range patterns {
		if strings.Contains(nameLow, p) || strings.Contains(descLow, p) {
			return true
		}
	}
	return false
}

func handleStats(w http.ResponseWriter, r *http.Request) {
	s := collectStats()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(s)
}

func main() {
	port := flag.String("port", "32249", "listen port")
	flag.Parse()

	http.HandleFunc("/api/v1/status", handleStatus)
	http.HandleFunc("/api/v1/shutdown", handleShutdown)
	http.HandleFunc("/api/v1/stats", handleStats)

	addr := fmt.Sprintf(":%s", *port)
	log.Printf("wol-agent listening on %s", addr)
	if err := http.ListenAndServe(addr, nil); err != nil {
		log.Fatalf("server error: %v", err)
	}
}
