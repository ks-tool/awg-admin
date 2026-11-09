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

package exampleprovider

import (
	"testing"

	"github.com/horchestra/pack/sdk"
)

func TestPackGenerates(t *testing.T) {
	if _, err := sdk.Schema(&Config{}); err != nil {
		t.Fatal(err)
	}
	if err := sdk.Generate(t.TempDir(), &Config{}); err != nil {
		t.Fatal(err)
	}
}
