package metrics

import (
	"os"
	"testing"
)

// The upstream tests exercise the metrics machinery, so they run with an
// endpoint set (path on); TestLiteBuildIsOff checks the shipped default.
func TestMain(m *testing.M) {
	shipped := DefaultEndpoint
	DefaultEndpoint = "https://metrics.test.invalid/mp/collect"
	code := m.Run()
	DefaultEndpoint = shipped
	os.Exit(code)
}

func TestLiteBuildIsOff(t *testing.T) {
	DefaultEndpoint = ""
	defer func() { DefaultEndpoint = "https://metrics.test.invalid/mp/collect" }()
	if CompiledIn() {
		t.Fatal("CompiledIn() true with empty endpoint")
	}
	if _, err := Init("test", true, ""); err != nil {
		t.Fatal(err)
	}
	if Enabled() {
		t.Fatal("Init(enable=true) enabled metrics in lite build")
	}
	if err := EnsureUserConfigDefaults(); err != nil {
		t.Fatal(err)
	}
}
