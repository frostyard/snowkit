package lintgate

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestLintRequiresGolangciLint(t *testing.T) {
	makePath, err := exec.LookPath("make")
	if err != nil {
		t.Fatalf("make is required: %v", err)
	}

	repositoryRoot, err := filepath.Abs("../..")
	if err != nil {
		t.Fatalf("find repository root: %v", err)
	}

	emptyPath := t.TempDir()
	env := make([]string, 0, len(os.Environ())+1)
	for _, variable := range os.Environ() {
		if len(variable) >= 5 && variable[:5] == "PATH=" {
			continue
		}
		env = append(env, variable)
	}
	env = append(env, "PATH="+emptyPath)

	command := exec.Command(makePath, "-s", "lint")
	command.Dir = repositoryRoot
	command.Env = env
	output, err := command.CombinedOutput()
	if err == nil {
		t.Fatal("make lint succeeded without golangci-lint")
	}
	var exitError *exec.ExitError
	if !errors.As(err, &exitError) {
		t.Fatalf("make lint failed unexpectedly: %v", err)
	}
	if exitError.ExitCode() == 0 {
		t.Fatal("make lint returned a zero exit code without golangci-lint")
	}
	if !bytes.Contains(output, []byte("golangci-lint is required: install v2.15.0, the version CI pins")) {
		t.Fatalf("make lint output did not contain the required error; output:\n%s", output)
	}
}
