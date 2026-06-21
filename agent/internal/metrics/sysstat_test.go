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

import "testing"

func TestCPUPercent(t *testing.T) {
	prev := cpuTotals{idle: 100, total: 200}
	// 50 jiffies elapsed, 10 of them idle => 80% busy.
	cur := cpuTotals{idle: 110, total: 250}

	got := cpuPercent(prev, cur)
	want := 80.0
	if got != want {
		t.Fatalf("cpuPercent() = %v, want %v", got, want)
	}
}

func TestCPUPercentNoElapsedTime(t *testing.T) {
	same := cpuTotals{idle: 50, total: 100}
	if got := cpuPercent(same, same); got != 0 {
		t.Fatalf("cpuPercent() with no delta = %v, want 0", got)
	}
}

func TestParseMemInfoKB(t *testing.T) {
	cases := map[string]uint64{
		"MemTotal:       16384000 kB": 16384000,
		"MemAvailable:    8192000 kB": 8192000,
		"Malformed":                   0,
	}
	for line, want := range cases {
		if got := parseMemInfoKB(line); got != want {
			t.Errorf("parseMemInfoKB(%q) = %d, want %d", line, got, want)
		}
	}
}
