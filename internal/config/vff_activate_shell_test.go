package config

import (
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
)

func TestVFFOpsShell(t *testing.T) {
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	modRoot := filepath.Clean(filepath.Join(filepath.Dir(thisFile), "..", ".."))
	cmd := exec.Command("bash", "scripts/test/vff_ops_test.sh")
	cmd.Dir = modRoot
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("vff_ops_test.sh failed: %v\n%s", err, out)
	}
	t.Logf("%s", out)
}
