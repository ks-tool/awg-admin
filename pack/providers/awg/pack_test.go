package awg

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
