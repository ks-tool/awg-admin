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

package models

import "testing"

func TestDeterministicUUIDCompatibility(t *testing.T) {
	const expected = "5c9ef7cb-8c94-5df0-8168-aaa47e3d5da2"
	if got := NewDeterministicUUID("example").String(); got != expected {
		t.Fatalf("UUID = %q, want %q", got, expected)
	}
}
