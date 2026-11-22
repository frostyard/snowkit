package workflowpin

import (
	"bufio"
	"os"
	"regexp"
	"strings"
	"testing"
)

var (
	toolchainSetting = regexp.MustCompile(`(?:^|\s)GOTOOLCHAIN\s*[:=]\s*("[^"]*"|'[^']*'|[^\s#]*)`)
	exactToolchain   = regexp.MustCompile(`^go1\.[0-9]+\.[0-9]+$`)
)

// goToolchainValue returns the value of a GOTOOLCHAIN setting on 1 workflow
// line (`GOTOOLCHAIN: v` or `GOTOOLCHAIN=v`), without quotes or a trailing
// comment. A YAML comment line has no setting.
func goToolchainValue(line string) (string, bool) {
	if strings.HasPrefix(strings.TrimSpace(line), "#") {
		return "", false
	}
	line, _, _ = strings.Cut(line, " #")
	m := toolchainSetting.FindStringSubmatch(line)
	if m == nil {
		return "", false
	}
	return strings.Trim(m[1], `"'`), true
}

// toolchainPinned reports whether value names 1 exact Go release or local.
func toolchainPinned(value string) bool {
	return value == "local" || exactToolchain.MatchString(value)
}

// TestGoToolchainPinned fails on a GOTOOLCHAIN setting in a workflow that is
// not an exact release or local: auto once resolved to a Go release whose
// checksum was not yet published, and the job failed (frostyard/snowkit#20).
func TestGoToolchainPinned(t *testing.T) {
	files := workflowFiles(t)
	checked := 0
	for _, file := range files {
		f, err := os.Open(file)
		if err != nil {
			t.Fatal(err)
		}
		sc := bufio.NewScanner(f)
		n := 0
		for sc.Scan() {
			n++
			value, ok := goToolchainValue(sc.Text())
			if !ok {
				continue
			}
			checked++
			if !toolchainPinned(value) {
				t.Errorf("%s:%d: GOTOOLCHAIN value %q is not pinned: a floating toolchain (auto) can resolve to a Go release whose checksum is not yet published (frostyard/snowkit#20); pin an exact release such as go1.25.14, or local", file, n, value)
			}
		}
		_ = f.Close()
		if err := sc.Err(); err != nil {
			t.Fatal(err)
		}
	}
	t.Logf("checked %d GOTOOLCHAIN settings", checked)
}

func TestToolchainHelpers(t *testing.T) {
	for _, tc := range []struct {
		line  string
		value string
		ok    bool
	}{
		{"        GOTOOLCHAIN: go1.25.14", "go1.25.14", true},
		{`          GOTOOLCHAIN: "auto"`, "auto", true},
		{"      GOTOOLCHAIN: 'local' # why", "local", true},
		{"        run: GOTOOLCHAIN=auto go install x@v1", "auto", true},
		{`        run: GOTOOLCHAIN="go1.25.14" go install x@v1 # why`, "go1.25.14", true},
		{"      # GOTOOLCHAIN: auto", "", false},
		{"        run: go install x@v1", "", false},
		{"", "", false},
		{"GOTOOLCHAIN: # missing", "", true},
		{"NOT_GOTOOLCHAIN: auto", "", false},
		{"run: go install x@v1 # GOTOOLCHAIN=auto", "", false},
	} {
		t.Run(tc.line, func(t *testing.T) {
			value, ok := goToolchainValue(tc.line)
			if value != tc.value || ok != tc.ok {
				t.Errorf("goToolchainValue(%q) = (%q, %t), want (%q, %t)", tc.line, value, ok, tc.value, tc.ok)
			}
		})
	}
	for _, tc := range []struct {
		value  string
		pinned bool
	}{
		{"local", true},
		{"go1.25.14", true},
		{"go1.24.6", true},
		{"auto", false},
		{"path", false},
		{"go1.25", false},
		{"go1.25.14+auto", false},
		{"go1.25.14+path", false},
		{"local+auto", false},
		{"go1.25rc1", false},
		{"", false},
		{"go2.25.14", false},
		{"go1.25.14\n", false},
	} {
		t.Run(tc.value, func(t *testing.T) {
			if got := toolchainPinned(tc.value); got != tc.pinned {
				t.Errorf("toolchainPinned(%q) = %t, want %t", tc.value, got, tc.pinned)
			}
		})
	}
}
