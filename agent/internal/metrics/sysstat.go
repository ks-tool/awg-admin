/*
  Copyright © 2026 Alexey Shulutkov <github@shulutkov.ru>

  Licensed under the Apache License, Version 2.0 (the "License");
  you may not use this file except in compliance with the License.
  You may obtain a copy of the License at

  	http://www.apache.org/licenses/LICENSE-2.0

  Unless required by applicable law or agreed to in writing, software
  distributed under the License is distributed on an "AS IS" BASIS,
  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
  See the License for the specific language governing permissions and
  limitations under the License.
*/

package metrics

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"
)

// Host-level stats are read straight from /proc instead of pulling in a
// system-info library — keeps the agent's footprint small (it already runs
// as a long-lived systemd unit with CAP_NET_ADMIN/CAP_NET_RAW only, see
// internal/deploy/assets/awg-agent.service) and /proc is always present on
// the Linux hosts this agent targets.

// cpuTotals is one snapshot of the aggregate "cpu" line from /proc/stat, in
// USER_HZ jiffies. CPU percent is derived from the delta between two
// snapshots, so a single reading on its own is meaningless.
type cpuTotals struct {
	idle  uint64
	total uint64
}

func readCPUTotals() (cpuTotals, error) {
	data, err := os.ReadFile("/proc/stat")
	if err != nil {
		return cpuTotals{}, err
	}

	line, _, _ := strings.Cut(string(data), "\n")
	fields := strings.Fields(line)
	if len(fields) < 5 || fields[0] != "cpu" {
		return cpuTotals{}, fmt.Errorf("unexpected /proc/stat format: %q", line)
	}

	var total uint64
	var idle uint64
	for i, f := range fields[1:] {
		v, err := strconv.ParseUint(f, 10, 64)
		if err != nil {
			return cpuTotals{}, fmt.Errorf("parse /proc/stat field %d: %w", i, err)
		}
		total += v
		// idle (index 3) + iowait (index 4): both count as "not busy".
		if i == 3 || i == 4 {
			idle += v
		}
	}
	return cpuTotals{idle: idle, total: total}, nil
}

// cpuPercent returns the busy percentage between two cpuTotals snapshots.
func cpuPercent(prev, cur cpuTotals) float64 {
	totalDelta := cur.total - prev.total
	if totalDelta == 0 {
		return 0
	}
	idleDelta := cur.idle - prev.idle
	return (1 - float64(idleDelta)/float64(totalDelta)) * 100
}

type memStats struct {
	totalBytes uint64
	usedBytes  uint64
}

func readMemStats() (memStats, error) {
	f, err := os.Open("/proc/meminfo")
	if err != nil {
		return memStats{}, err
	}
	defer func() { _ = f.Close() }()

	var totalKB, availableKB uint64
	seen := 0
	scanner := bufio.NewScanner(f)
	for scanner.Scan() && seen < 2 {
		line := scanner.Text()
		switch {
		case strings.HasPrefix(line, "MemTotal:"):
			totalKB = parseMemInfoKB(line)
			seen++
		case strings.HasPrefix(line, "MemAvailable:"):
			availableKB = parseMemInfoKB(line)
			seen++
		}
	}
	if err := scanner.Err(); err != nil {
		return memStats{}, err
	}

	total := totalKB * 1024
	available := availableKB * 1024
	if available > total {
		available = total
	}
	return memStats{totalBytes: total, usedBytes: total - available}, nil
}

func parseMemInfoKB(line string) uint64 {
	fields := strings.Fields(line)
	if len(fields) < 2 {
		return 0
	}
	v, _ := strconv.ParseUint(fields[1], 10, 64)
	return v
}

type loadAvg struct {
	load1, load5, load15 float64
}

func readLoadAvg() (loadAvg, error) {
	data, err := os.ReadFile("/proc/loadavg")
	if err != nil {
		return loadAvg{}, err
	}
	fields := strings.Fields(string(data))
	if len(fields) < 3 {
		return loadAvg{}, fmt.Errorf("unexpected /proc/loadavg format: %q", data)
	}
	l1, err := strconv.ParseFloat(fields[0], 64)
	if err != nil {
		return loadAvg{}, err
	}
	l5, err := strconv.ParseFloat(fields[1], 64)
	if err != nil {
		return loadAvg{}, err
	}
	l15, err := strconv.ParseFloat(fields[2], 64)
	if err != nil {
		return loadAvg{}, err
	}
	return loadAvg{load1: l1, load5: l5, load15: l15}, nil
}

type netStats struct {
	rxBytes, txBytes uint64
}

// readNetStats sums rx/tx bytes across every interface in /proc/net/dev
// except loopback — a simple host-wide throughput figure rather than
// per-interface (per-WireGuard-interface peer traffic is tracked
// separately and far more usefully via wgctrl, see collector.go).
func readNetStats() (netStats, error) {
	f, err := os.Open("/proc/net/dev")
	if err != nil {
		return netStats{}, err
	}
	defer func() { _ = f.Close() }()

	var out netStats
	scanner := bufio.NewScanner(f)
	lineNum := 0
	for scanner.Scan() {
		lineNum++
		if lineNum <= 2 {
			continue // two header lines
		}
		line := scanner.Text()
		name, rest, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		if strings.TrimSpace(name) == "lo" {
			continue
		}
		fields := strings.Fields(rest)
		if len(fields) < 9 {
			continue
		}
		rx, _ := strconv.ParseUint(fields[0], 10, 64)
		tx, _ := strconv.ParseUint(fields[8], 10, 64)
		out.rxBytes += rx
		out.txBytes += tx
	}
	if err := scanner.Err(); err != nil {
		return netStats{}, err
	}
	return out, nil
}
