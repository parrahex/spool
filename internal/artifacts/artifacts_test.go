package artifacts

import "testing"

func TestSafePathRejectsDirectoryTraversal(t *testing.T) {
	destination := t.TempDir()

	_, err := safePath(destination, "../../escape.txt")
	if err == nil {
		t.Fatal("expected path traversal to be rejected")
	}
}
