package workflowpin

import (
	"bufio"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

var (
	usesLine       = regexp.MustCompile(`^\s*(?:-\s+)?uses:\s*(\S+)(.*)$`)
	pinnedRef      = regexp.MustCompile(`^[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+(/[A-Za-z0-9_./-]+)?@[0-9a-f]{40}$`)
	versionComment = regexp.MustCompile(`^\s+#\s*v\S*`)
	// A reusable workflow from a repository with no release tags (such as
	// frostyard/core) names the pull request that made the pinned commit:
	// "# core#194".
	reusableWorkflow = regexp.MustCompile(`^[^@]+/\.github/workflows/[^@]+@`)
	prComment        = regexp.MustCompile(`^\s+#\s*[A-Za-z0-9_.-]+#[0-9]+\b`)
)

// TestActionsPinnedToCommitSHA enforces ADR-0021: every external action is
// pinned to a full commit SHA with a "# v..." version comment. A reusable
// workflow from a repository with no release tags may name its pull request
// instead ("# core#194").
func TestActionsPinnedToCommitSHA(t *testing.T) {
	var files []string
	for _, dir := range []string{"../../" + ".github/workflows", "../../" + ".github/actions"} {
		if _, err := os.Stat(dir); err != nil {
			continue
		}
		err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			ext := filepath.Ext(path)
			if !d.IsDir() && (ext == ".yml" || ext == ".yaml") {
				files = append(files, path)
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	if len(files) == 0 {
		t.Fatal("found 0 workflow files: the path is probably wrong")
	}
	for _, file := range files {
		f, err := os.Open(file)
		if err != nil {
			t.Fatal(err)
		}
		sc := bufio.NewScanner(f)
		n := 0
		for sc.Scan() {
			n++
			line := sc.Text()
			m := usesLine.FindStringSubmatch(line)
			if m == nil {
				continue
			}
			ref := strings.Trim(m[1], `"'`)
			if strings.HasPrefix(ref, "./") || strings.HasPrefix(ref, "docker://") {
				continue
			}
			commented := versionComment.MatchString(m[2]) ||
				(reusableWorkflow.MatchString(ref) && prComment.MatchString(m[2]))
			if !pinnedRef.MatchString(ref) || !commented {
				t.Errorf("%s:%d: action must be pinned to a 40-char commit SHA with a `# v...` comment: %s", file, n, strings.TrimSpace(line))
			}
		}
		_ = f.Close()
		if err := sc.Err(); err != nil {
			t.Fatal(err)
		}
	}
}
