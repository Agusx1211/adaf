package cli

import (
	"testing"
)

func TestExtractDiffFileNames(t *testing.T) {
	diff := `diff --git a/main.go b/main.go
--- a/main.go
+++ b/main.go
@@ -1,3 +1,4 @@
 package main
+import "fmt"
diff --git a/new.go b/new.go
--- /dev/null
+++ b/new.go
@@ -0,0 +1,3 @@
+package main
diff --git a/removed.go b/removed.go
--- a/removed.go
+++ /dev/null
@@ -1,3 +0,0 @@
-package main
`

	names := extractDiffFileNames(diff)
	want := []string{"main.go", "new.go", "removed.go"}
	if len(names) != len(want) {
		t.Fatalf("extractDiffFileNames() = %v (len %d), want %v (len %d)", names, len(names), want, len(want))
	}
	for i, name := range names {
		if name != want[i] {
			t.Errorf("names[%d] = %q, want %q", i, name, want[i])
		}
	}
}

func TestExtractDiffFileNames_Empty(t *testing.T) {
	names := extractDiffFileNames("")
	if len(names) != 0 {
		t.Fatalf("extractDiffFileNames(\"\") = %v, want empty", names)
	}
}
