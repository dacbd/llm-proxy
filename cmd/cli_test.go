package cmd_test

import (
	"flag"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
)

var update = flag.Bool("update", false, "update golden files")

var testBin string

func TestMain(m *testing.M) {
	flag.Parse()

	bin, err := os.MkdirTemp("", "llm-proxy-test-*")
	if err != nil {
		panic(err)
	}
	defer os.RemoveAll(bin)

	ext := ""
	if runtime.GOOS == "windows" {
		ext = ".exe"
	}
	testBin = filepath.Join(bin, "llm-proxy"+ext)

	// Build the binary once for all tests.
	out, err := exec.Command("go", "build", "-o", testBin, "github.com/dacbd/llm-proxy").CombinedOutput()
	if err != nil {
		panic("failed to build binary: " + err.Error() + "\n" + string(out))
	}

	os.Exit(m.Run())
}

func runCLI(t *testing.T, args ...string) []byte {
	t.Helper()
	cmd := exec.Command(testBin, args...)
	// --help exits with code 0; unknown commands exit non-zero.
	out, _ := cmd.CombinedOutput()
	return out
}

func checkGolden(t *testing.T, got []byte, name string) {
	t.Helper()
	golden := filepath.Join("testdata", name+".golden")
	if *update {
		if err := os.WriteFile(golden, got, 0644); err != nil {
			t.Fatalf("failed to update golden file %s: %v", golden, err)
		}
		return
	}
	want, err := os.ReadFile(golden)
	if err != nil {
		t.Fatalf("golden file %s missing — run with -update to create it", golden)
	}
	if string(got) != string(want) {
		t.Errorf("output mismatch for %s\ngot:\n%s\nwant:\n%s", name, got, want)
	}
}

func TestCLI_Help(t *testing.T) {
	checkGolden(t, runCLI(t, "--help"), "help")
}

func TestCLI_RunHelp(t *testing.T) {
	checkGolden(t, runCLI(t, "run", "--help"), "run-help")
}

func TestCLI_RunServerHelp(t *testing.T) {
	checkGolden(t, runCLI(t, "run", "server", "--help"), "run-server-help")
}
