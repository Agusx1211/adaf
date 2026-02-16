package webserver

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestIsWithinAllowedRoot(t *testing.T) {
	tests := []struct {
		name        string
		allowedRoot string
		path        string
		want        bool
	}{
		{
			name:        "path is allowed root",
			allowedRoot: "/home/user",
			path:        "/home/user",
			want:        true,
		},
		{
			name:        "path is subdirectory",
			allowedRoot: "/home/user",
			path:        "/home/user/projects",
			want:        true,
		},
		{
			name:        "path is deeply nested subdirectory",
			allowedRoot: "/home/user",
			path:        "/home/user/projects/foo/bar",
			want:        true,
		},
		{
			name:        "path is parent directory",
			allowedRoot: "/home/user",
			path:        "/home",
			want:        false,
		},
		{
			name:        "path is sibling directory",
			allowedRoot: "/home/user",
			path:        "/home/other",
			want:        false,
		},
		{
			name:        "path is completely different",
			allowedRoot: "/home/user",
			path:        "/etc",
			want:        false,
		},
		{
			name:        "empty allowed root allows everything",
			allowedRoot: "",
			path:        "/any/path",
			want:        true,
		},
		{
			name:        "path with trailing slash",
			allowedRoot: "/home/user",
			path:        "/home/user/",
			want:        true,
		},
		{
			name:        "path tries to escape with ..",
			allowedRoot: "/home/user",
			path:        "/home/user/../other",
			want:        false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := &Server{allowedRoot: tt.allowedRoot}
			got := srv.isWithinAllowedRoot(tt.path)
			if got != tt.want {
				t.Errorf("isWithinAllowedRoot(%q) = %v, want %v", tt.path, got, tt.want)
			}
		})
	}
}

func TestResolveBrowsePath(t *testing.T) {
	tmpDir := t.TempDir()
	allowedDir := filepath.Join(tmpDir, "allowed")
	if err := os.MkdirAll(allowedDir, 0755); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name        string
		allowedRoot string
		rawPath     string
		wantErr     bool
		wantPath    string
	}{
		{
			name:        "empty path defaults to allowed root",
			allowedRoot: allowedDir,
			rawPath:     "",
			wantErr:     false,
			wantPath:    allowedDir,
		},
		{
			name:        "absolute path within allowed root",
			allowedRoot: allowedDir,
			rawPath:     filepath.Join(allowedDir, "subdir"),
			wantErr:     false,
			wantPath:    filepath.Join(allowedDir, "subdir"),
		},
		{
			name:        "relative path resolved against allowed root",
			allowedRoot: allowedDir,
			rawPath:     "subdir",
			wantErr:     false,
			wantPath:    filepath.Join(allowedDir, "subdir"),
		},
		{
			name:        "path outside allowed root returns error",
			allowedRoot: allowedDir,
			rawPath:     tmpDir,
			wantErr:     true,
		},
		{
			name:        "path trying to escape returns error",
			allowedRoot: allowedDir,
			rawPath:     "../outside",
			wantErr:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := &Server{allowedRoot: tt.allowedRoot}
			got, err := srv.resolveBrowsePath(tt.rawPath)
			if tt.wantErr {
				if err == nil {
					t.Errorf("resolveBrowsePath(%q) expected error, got nil", tt.rawPath)
				}
				return
			}
			if err != nil {
				t.Errorf("resolveBrowsePath(%q) unexpected error: %v", tt.rawPath, err)
				return
			}
			if got != tt.wantPath {
				t.Errorf("resolveBrowsePath(%q) = %q, want %q", tt.rawPath, got, tt.wantPath)
			}
		})
	}
}

func TestFSBrowseWithAllowedRoot(t *testing.T) {
	tmpDir := t.TempDir()
	allowedDir := filepath.Join(tmpDir, "allowed")
	outsideDir := filepath.Join(tmpDir, "outside")
	if err := os.MkdirAll(allowedDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(outsideDir, 0755); err != nil {
		t.Fatal(err)
	}

	registry := NewProjectRegistry()
	srv := NewMulti(registry, Options{
		RootDir:     allowedDir,
		AllowedRoot: allowedDir,
	})

	t.Run("browse within allowed root", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/fs/browse?path="+allowedDir, nil)
		rec := httptest.NewRecorder()
		srv.httpServer.Handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
		}
	})

	t.Run("browse outside allowed root returns forbidden", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/fs/browse?path="+outsideDir, nil)
		rec := httptest.NewRecorder()
		srv.httpServer.Handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusForbidden {
			t.Errorf("status = %d, want %d", rec.Code, http.StatusForbidden)
		}
	})

	t.Run("parent at allowed root boundary is empty", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/fs/browse?path=", nil)
		rec := httptest.NewRecorder()
		srv.httpServer.Handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
		}

		var resp fsBrowseResponse
		if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
			t.Fatalf("decode response: %v", err)
		}
		if resp.Parent != "" {
			t.Errorf("parent = %q, want empty (at allowed root boundary)", resp.Parent)
		}
	})
}

func TestFSBrowseDefaultToCurrentDir(t *testing.T) {
	tmpDir := t.TempDir()

	registry := NewProjectRegistry()
	srv := NewMulti(registry, Options{
		RootDir:     tmpDir,
		AllowedRoot: "",
	})

	if srv.allowedRoot != tmpDir {
		t.Errorf("allowedRoot = %q, want %q", srv.allowedRoot, tmpDir)
	}
}
