package storage

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/shhac/grotto/internal/domain"
	"github.com/shhac/grotto/internal/logging"
)

func TestAtomicWriteFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.json")
	data := []byte(`{"hello": "world"}`)

	if err := atomicWriteFile(path, data, 0644); err != nil {
		t.Fatalf("atomicWriteFile failed: %v", err)
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile failed: %v", err)
	}
	if string(got) != string(data) {
		t.Errorf("got %q, want %q", got, data)
	}

	// Verify permissions
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat failed: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0644 {
		t.Errorf("permissions = %o, want 0644", perm)
	}
}

func TestAtomicWriteFile_Overwrite(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.json")

	// Write initial content
	if err := atomicWriteFile(path, []byte("old"), 0644); err != nil {
		t.Fatalf("first write failed: %v", err)
	}

	// Overwrite
	if err := atomicWriteFile(path, []byte("new"), 0644); err != nil {
		t.Fatalf("second write failed: %v", err)
	}

	got, _ := os.ReadFile(path)
	if string(got) != "new" {
		t.Errorf("got %q, want %q", got, "new")
	}
}

func TestAtomicWriteFile_NoTempFileOnFailure(t *testing.T) {
	// Writing to a non-existent directory should fail and leave no temp file
	path := filepath.Join(t.TempDir(), "nodir", "test.json")
	err := atomicWriteFile(path, []byte("data"), 0644)
	if err == nil {
		t.Fatal("expected error writing to non-existent directory")
	}

	// Verify no temp files leaked in the parent of the non-existent dir
	entries, _ := os.ReadDir(t.TempDir())
	for _, e := range entries {
		if e.Name() != "nodir" && filepath.Ext(e.Name()) != "" {
			t.Errorf("unexpected file left behind: %s", e.Name())
		}
	}
}

func TestValidateWorkspaceName(t *testing.T) {
	tests := []struct {
		name    string
		wantErr bool
	}{
		{"my-workspace", false},
		{"workspace 1", false},
		{"with.dots", false},
		{"unicode-名前", false},
		{"", true},
		{"..", true},
		{"foo/../bar", true},
		{"../escape", true},
		{"path/sep", true},
		{"back\\slash", true},
		{string([]byte{0}), true},
		{"has\x00null", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateWorkspaceName(tt.name)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateWorkspaceName(%q) error = %v, wantErr %v", tt.name, err, tt.wantErr)
			}
		})
	}
}

func TestSaveWorkspace_PathTraversal(t *testing.T) {
	logger := logging.NewNopLogger()
	dir := t.TempDir()
	repo := NewJSONRepository(dir, logger)

	malicious := []string{
		"../../etc/passwd",
		"../escape",
		"foo/bar",
		"back\\slash",
	}

	for _, name := range malicious {
		ws := domain.Workspace{Name: name}
		err := repo.SaveWorkspace(ws)
		if err == nil {
			t.Errorf("SaveWorkspace(%q) should have failed", name)
		}
	}
}

func TestLoadWorkspace_PathTraversal(t *testing.T) {
	logger := logging.NewNopLogger()
	dir := t.TempDir()
	repo := NewJSONRepository(dir, logger)

	_, err := repo.LoadWorkspace("../../etc/passwd")
	if err == nil {
		t.Error("LoadWorkspace with path traversal should have failed")
	}
}

func TestDeleteWorkspace_PathTraversal(t *testing.T) {
	logger := logging.NewNopLogger()
	dir := t.TempDir()
	repo := NewJSONRepository(dir, logger)

	err := repo.DeleteWorkspace("../../etc/passwd")
	if err == nil {
		t.Error("DeleteWorkspace with path traversal should have failed")
	}
}

func TestSaveAndLoadWorkspace_RoundTrip(t *testing.T) {
	logger := logging.NewNopLogger()
	dir := t.TempDir()
	repo := NewJSONRepository(dir, logger)

	ws := domain.Workspace{
		Name:            "test-workspace",
		SelectedService: "my.Service",
	}

	if err := repo.SaveWorkspace(ws); err != nil {
		t.Fatalf("SaveWorkspace failed: %v", err)
	}

	loaded, err := repo.LoadWorkspace("test-workspace")
	if err != nil {
		t.Fatalf("LoadWorkspace failed: %v", err)
	}

	if loaded.Name != ws.Name {
		t.Errorf("Name = %q, want %q", loaded.Name, ws.Name)
	}
	if loaded.SelectedService != ws.SelectedService {
		t.Errorf("SelectedService = %q, want %q", loaded.SelectedService, ws.SelectedService)
	}
}
