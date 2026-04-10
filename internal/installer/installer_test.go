package installer

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/etkecc/agru/internal/models"
)

// fakeRunner records calls and returns preset outputs matched by prefix
type fakeRunner struct {
	outputs map[string]string
	errors  map[string]error
	calls   []string
}

func newFakeRunner() *fakeRunner {
	return &fakeRunner{
		outputs: make(map[string]string),
		errors:  make(map[string]error),
	}
}

func (r *fakeRunner) Run(command, _ string) (string, error) {
	r.calls = append(r.calls, command)
	for key, out := range r.outputs {
		if strings.HasPrefix(command, key) {
			return out, r.errors[key]
		}
	}
	return "", nil
}

func (r *fakeRunner) called(prefix string) bool {
	for _, c := range r.calls {
		if strings.HasPrefix(c, prefix) {
			return true
		}
	}
	return false
}

// callbackRunner calls a function for each Run invocation
type callbackRunner struct {
	fn func(command, dir string) (string, error)
}

func (r *callbackRunner) Run(command, dir string) (string, error) {
	return r.fn(command, dir)
}

func TestGetInstalled(t *testing.T) {
	fsys := fstest.MapFS{
		"role-a/meta/.galaxy_install_info": &fstest.MapFile{
			Data: []byte("version: v1.0.0\n"),
		},
	}

	inst := &Installer{
		runner:    newFakeRunner(),
		fsys:      fsys,
		rolesPath: "/roles",
	}

	entries := models.File{
		{Name: "role-a", Version: "v1.0.0"},
		{Name: "role-b", Version: "v2.0.0"}, // no install info
		{Name: "role-c", Version: "v3.0.0"}, // not in FS
	}

	installed := inst.GetInstalled(entries)
	if len(installed) != 1 {
		t.Fatalf("GetInstalled() len = %d, want 1", len(installed))
	}
	if installed[0].GetName() != "role-a" {
		t.Errorf("GetInstalled()[0] = %q, want role-a", installed[0].GetName())
	}
}

func TestRunCloneSuccess(t *testing.T) {
	fr := newFakeRunner()
	fr.outputs["git clone"] = ""

	inst := &Installer{runner: fr}
	_, err := inst.runClone("git clone -q --depth 1 -b v1.0.0 https://github.com/org/role /tmp/dir", 0)
	if err != nil {
		t.Errorf("runClone() unexpected error = %v", err)
	}
	if !fr.called("git clone") {
		t.Error("runClone() should have called git clone")
	}
}

func TestRunCloneRetryOnNetworkError(t *testing.T) {
	callCount := 0
	inst := &Installer{runner: &callbackRunner{
		fn: func(_, _ string) (string, error) {
			callCount++
			if callCount == 1 {
				return "Couldn't connect to server", errors.New("exit status 128")
			}
			return "", nil
		},
	}}

	_, err := inst.runClone("git clone -q --depth 1 -b v1.0.0 https://example.com/role /tmp/dir", 0)
	if err != nil {
		t.Errorf("runClone() should succeed after retry, got error = %v", err)
	}
	if callCount != 2 {
		t.Errorf("runClone() call count = %d, want 2 (1 failure + 1 retry)", callCount)
	}
}

func TestRunCloneMaxRetries(t *testing.T) {
	callCount := 0
	inst := &Installer{runner: &callbackRunner{
		fn: func(_, _ string) (string, error) {
			callCount++
			return "Couldn't connect to server", errors.New("exit status 128")
		},
	}}

	_, err := inst.runClone("git clone -q --depth 1 -b v1.0.0 https://example.com/role /tmp/dir", 0)
	if err == nil {
		t.Error("runClone() should return error when max retries exceeded")
	}
	// initial call + RetriesMax retries
	if callCount != RetriesMax+1 {
		t.Errorf("runClone() call count = %d, want %d (1 initial + %d retries)", callCount, RetriesMax+1, RetriesMax)
	}
}

func TestRunCloneNoRetryOnOtherError(t *testing.T) {
	callCount := 0
	inst := &Installer{runner: &callbackRunner{
		fn: func(_, _ string) (string, error) {
			callCount++
			return "some other error", errors.New("exit status 128")
		},
	}}

	_, err := inst.runClone("git clone -q --depth 1 -b v1.0.0 https://example.com/role /tmp/dir", 0)
	if err == nil {
		t.Error("runClone() should return error for non-network failures")
	}
	if callCount != 1 {
		t.Errorf("runClone() should not retry non-network errors, call count = %d, want 1", callCount)
	}
}

func TestInstallRoleCallsGitInOrder(t *testing.T) {
	tmpDir := t.TempDir()
	rolesPath := filepath.Join(tmpDir, "roles")
	if err := os.MkdirAll(rolesPath, 0o700); err != nil {
		t.Fatal(err)
	}

	commitSHA := "abc123def456abc123def456abc123def456abc12"
	calledCmds := []string{}

	// Use a callback runner so tar can simulate directory creation
	inst := &Installer{
		runner: &callbackRunner{fn: func(command, _ string) (string, error) {
			calledCmds = append(calledCmds, command)
			if strings.HasPrefix(command, "git rev-parse HEAD") {
				return commitSHA, nil
			}
			if strings.HasPrefix(command, "tar -xf") {
				// Simulate tar extracting the archive: create role dir structure
				if err := os.MkdirAll(filepath.Join(rolesPath, "my-role", "meta"), 0o700); err != nil {
					return "", err
				}
			}
			return "", nil
		}},
		fsys:      os.DirFS(rolesPath),
		rolesPath: rolesPath,
		cleanup:   false,
	}

	entry := &models.Entry{}
	entry.Name = "my-role"
	entry.Src = "git+https://github.com/org/my-role.git"
	entry.Version = "v1.0.0"

	ok, err := inst.installRole(entry)
	if err != nil {
		t.Fatalf("installRole() error = %v", err)
	}
	if !ok {
		t.Error("installRole() = false, want true for new installation")
	}

	called := func(prefix string) bool {
		for _, c := range calledCmds {
			if strings.HasPrefix(c, prefix) {
				return true
			}
		}
		return false
	}
	for _, expectedCmd := range []string{"git clone", "git rev-parse HEAD", "git archive", "tar -xf"} {
		if !called(expectedCmd) {
			t.Errorf("installRole() should have called %q, called: %v", expectedCmd, calledCmds)
		}
	}
}

func TestInstallRoleAlreadyUpToDate(t *testing.T) {
	commitSHA := "abc123def456abc123def456abc123def456abc12"

	fsys := fstest.MapFS{
		"my-role/meta/.galaxy_install_info": &fstest.MapFile{
			Data: fmt.Appendf(nil, "install_commit: %s\nversion: v1.0.0\n", commitSHA),
		},
	}

	fr := newFakeRunner()
	fr.outputs["git clone"] = ""
	fr.outputs["git rev-parse HEAD"] = commitSHA

	inst := &Installer{
		runner:    fr,
		fsys:      fsys,
		rolesPath: t.TempDir(),
		cleanup:   false,
	}

	entry := &models.Entry{}
	entry.Name = "my-role"
	entry.Src = "git+https://github.com/org/my-role.git"
	entry.Version = "v1.0.0"

	ok, err := inst.installRole(entry)
	if err != nil {
		t.Fatalf("installRole() error = %v", err)
	}
	if ok {
		t.Error("installRole() = true, want false when already up to date (same commit SHA)")
	}
	if fr.called("git archive") {
		t.Error("installRole() should not call git archive when already up to date")
	}
}

func TestInstallRoleCommitSHAVersion(t *testing.T) {
	tmpDir := t.TempDir()
	rolesPath := filepath.Join(tmpDir, "roles")
	if err := os.MkdirAll(rolesPath, 0o700); err != nil {
		t.Fatal(err)
	}

	commitSHA := strings.Repeat("a", 40) // 40-char SHA triggers commit-mode clone
	calledCmds := []string{}

	inst := &Installer{
		runner: &callbackRunner{fn: func(command, _ string) (string, error) {
			calledCmds = append(calledCmds, command)
			if strings.HasPrefix(command, "git rev-parse HEAD") {
				return commitSHA, nil
			}
			if strings.HasPrefix(command, "tar -xf") {
				if err := os.MkdirAll(filepath.Join(rolesPath, "sha-role", "meta"), 0o700); err != nil {
					return "", err
				}
			}
			return "", nil
		}},
		fsys:      os.DirFS(rolesPath),
		rolesPath: rolesPath,
		cleanup:   false,
	}

	entry := &models.Entry{}
	entry.Name = "sha-role"
	entry.Src = "git+https://github.com/org/sha-role.git"
	entry.Version = commitSHA

	_, err := inst.installRole(entry)
	if err != nil {
		t.Fatalf("installRole() error = %v", err)
	}

	cloneCmd := ""
	for _, c := range calledCmds {
		if strings.HasPrefix(c, "git clone") {
			cloneCmd = c
			break
		}
	}
	if !strings.Contains(cloneCmd, "remote.origin.fetch") {
		t.Errorf("installRole() with SHA version should use remote.origin.fetch, clone cmd: %q", cloneCmd)
	}
	if strings.Contains(cloneCmd, " -b ") {
		t.Errorf("installRole() with SHA version should not use -b flag, clone cmd: %q", cloneCmd)
	}
}

func TestInstallMissingConcurrentNoDatRace(t *testing.T) {
	// Run with -race to detect data races. Uses 4 concurrent workers installing
	// 8 roles in parallel, exercising shared state (i.fsys, changes).
	tmpDir := t.TempDir()
	rolesPath := filepath.Join(tmpDir, "roles")
	if err := os.MkdirAll(rolesPath, 0o700); err != nil {
		t.Fatal(err)
	}

	commitSHA := "abc123def456abc123def456abc123def456abc12"
	inst := &Installer{
		runner: &callbackRunner{fn: func(command, _ string) (string, error) {
			if strings.HasPrefix(command, "git rev-parse HEAD") {
				return commitSHA, nil
			}
			if strings.HasPrefix(command, "tar -xf ") {
				// Extract role name from "tar -xf /tmp/agru-ROLENAME-*"
				// The archive would contain ROLENAME/ but we simulate by creating the meta dir.
				// We determine the role from the archive path suffix pattern: agru-ROLENAME-*.tar
				parts := strings.Split(command, " ")
				if len(parts) >= 3 {
					archivePath := parts[2]
					base := strings.TrimSuffix(archivePath[strings.LastIndex(archivePath, "agru-")+5:], ".tar")
					// base is "ROLENAME-RANDOMSUFFIX" — strip the random suffix
					if idx := strings.LastIndex(base, "-"); idx != -1 {
						roleName := base[:idx]
						metaDir := filepath.Join(rolesPath, roleName, "meta")
						_ = os.MkdirAll(metaDir, 0o700)
					}
				}
			}
			return "", nil
		}},
		fsys:      os.DirFS(rolesPath),
		rolesPath: rolesPath,
		limit:     4,
		cleanup:   false,
	}

	const numRoles = 8
	entries := make(models.File, numRoles)
	for idx := range numRoles {
		entries[idx] = &models.Entry{}
		entries[idx].Name = fmt.Sprintf("role-%d", idx)
		entries[idx].Src = fmt.Sprintf("git+https://github.com/org/role-%d.git", idx)
		entries[idx].Version = "v1.0.0"
	}

	if err := inst.InstallMissing(entries); err != nil {
		t.Fatalf("InstallMissing() concurrent error = %v", err)
	}
}

func TestInstallMissingSkipsIncludeEntries(t *testing.T) {
	fr := newFakeRunner()
	inst := &Installer{
		runner:    fr,
		fsys:      fstest.MapFS{},
		rolesPath: t.TempDir(),
		limit:     1,
	}

	// Entry with Include set should be skipped entirely
	entries := models.File{
		{Include: "other-requirements.yml"},
	}

	// bootstrapRoles will try os.Stat on the rolesPath (temp dir exists, so no error)
	err := inst.InstallMissing(entries)
	if err != nil {
		t.Fatalf("InstallMissing() error = %v", err)
	}
	if fr.called("git clone") {
		t.Error("InstallMissing() should not call git clone for include-only entries")
	}
}
