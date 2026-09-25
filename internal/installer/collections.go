package installer

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"sync"

	"gopkg.in/yaml.v3"

	"github.com/etkecc/agru/internal/models"
)

// installMarkerName is the file agru drops into a collection dir while it is being replaced.
const installMarkerName = ".agru-install-marker"

// installCollection executes a single collection install inside the workpool goroutine.
func (i *Installer) installCollection(entry *models.Collection, mu *sync.Mutex, changes *models.UpdatedItems, errs *[]error, progress chan<- Progress) {
	fqcn := entry.GetFQCN()
	if progress != nil {
		progress <- Progress{Name: fqcn, Version: entry.Version, Status: "active"}
	}

	// Check if already installed at this version
	if entry.IsInstalled(i.collFS) {
		if progress != nil {
			mu.Lock()
			progress <- Progress{Name: fqcn, Version: entry.Version, Status: "skipped"}
			mu.Unlock()
		}
		return
	}

	oldVersion := entry.GetInstalledVersion(i.collFS)
	logLine, err := i.installCollectionFiles(entry)
	mu.Lock()
	defer mu.Unlock()
	if err != nil {
		*errs = append(*errs, fmt.Errorf("installing collection %s@%s: %w", fqcn, entry.Version, err))
		if progress != nil {
			progress <- Progress{Name: fqcn, Version: entry.Version, Status: "error", Log: logLine, Err: err}
		}
		return
	}

	version := entry.Version
	if version == "" {
		version = "HEAD"
	}
	if !ignoredVersions[version] {
		*changes = changes.Add(fqcn, oldVersion, version)
	}
	if progress != nil {
		progress <- Progress{Name: fqcn, Version: entry.Version, OldVersion: oldVersion, Status: "done", Log: logLine}
	}
}

// installCollectionFiles clones a collection repo, validates galaxy.yml, and extracts into collections path.
func (i *Installer) installCollectionFiles(entry *models.Collection) (string, error) {
	fqcn := entry.GetFQCN()
	repoURL := entry.GetRepoURL()
	target := entry.GetPath(i.collPath)
	if target == "" {
		return "", fmt.Errorf("cannot determine install path for unsupported collection %s", fqcn)
	}

	// Validate URL and version: no spaces, no leading dash (RCE guard).
	if err := models.ValidateGitArg(repoURL); err != nil {
		return "", fmt.Errorf("invalid collection repo URL %q: %w", repoURL, err)
	}
	if entry.Version != "" {
		if err := models.ValidateGitArg(entry.Version); err != nil {
			return "", fmt.Errorf("invalid collection version %q: %w", entry.Version, err)
		}
	}

	tmpdir, err := os.MkdirTemp("", "agru-coll-"+fqcn+"-*")
	if err != nil {
		return "", fmt.Errorf("creating tmp dir: %w", err)
	}
	tmpfile := tmpdir + ".tar"
	if i.cleanup {
		defer i.cleanupCollection(tmpdir, tmpfile)
	}

	logLine := fmt.Sprintf("[%s] cloning %s @ %s", fqcn, repoURL, orHead(entry.Version))
	cloneArgs := collectionCloneArgs(repoURL, entry.Version, tmpdir)
	out, err := i.runCloneArgs(cloneArgs, 0)
	if err != nil {
		return logLine, fmt.Errorf("cloning repo: %w\n%s", err, out)
	}

	if err := verifyRoleTree(tmpdir); err != nil {
		return logLine, fmt.Errorf("unsafe collection content: %w", err)
	}

	// Read and validate galaxy.yml
	galaxy, err := i.readGalaxy(tmpdir)
	if err != nil {
		return logLine, err
	}

	if err := i.validateGalaxyNamespace(galaxy, entry); err != nil {
		return logLine, err
	}

	// Archive and extract into target
	if err := i.archiveAndExtract(entry, galaxy, tmpdir, tmpfile, target); err != nil {
		return logLine, err
	}

	return logLine, nil
}

// writeCollectionManifest replaces MANIFEST.json in target with the generated manifest.
func (i *Installer) writeCollectionManifest(entry *models.Collection, galaxy map[string]any, target string) error {
	manifestPath := path.Join(target, "MANIFEST.json")
	if err := os.Remove(manifestPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("removing existing MANIFEST.json: %w", err)
	}
	manifestData, err := entry.GenerateManifest(galaxy)
	if err != nil {
		return fmt.Errorf("generating manifest: %w", err)
	}
	if err := os.WriteFile(manifestPath, manifestData, 0o600); err != nil {
		return fmt.Errorf("writing MANIFEST.json: %w", err)
	}
	return nil
}

// archiveAndExtract creates a git archive of the clone and extracts it into the target dir.
func (i *Installer) archiveAndExtract(entry *models.Collection, galaxy map[string]any, tmpdir, tmpfile, target string) error {
	archiveRef := entry.Version
	if archiveRef == "" {
		archiveRef = "HEAD"
	}
	archiveArgs := []string{"git", "archive", "--output=" + tmpfile, archiveRef}
	if _, err := i.runner.RunArgs(archiveArgs, tmpdir); err != nil {
		return fmt.Errorf("archiving repo: %w", err)
	}
	return i.extractCollection(entry, galaxy, target, tmpfile)
}

// readGalaxy reads and parses galaxy.yml from the cloned repo.
func (i *Installer) readGalaxy(tmpdir string) (map[string]any, error) {
	data, err := os.ReadFile(path.Join(tmpdir, "galaxy.yml"))
	if err != nil {
		return nil, fmt.Errorf("not an ansible collection repository (galaxy.yml missing)")
	}
	var galaxy map[string]any
	if err := yaml.Unmarshal(data, &galaxy); err != nil {
		return nil, fmt.Errorf("parsing galaxy.yml: %w", err)
	}
	return galaxy, nil
}

// validateGalaxyNamespace checks that galaxy.yml ns/name matches the expected collection name.
func (i *Installer) validateGalaxyNamespace(galaxy map[string]any, entry *models.Collection) error {
	ns, _ := galaxy["namespace"].(string) //nolint:errcheck // empty on type mismatch; caught by comparison below
	cn, _ := galaxy["name"].(string)      //nolint:errcheck // empty on type mismatch; caught by comparison below
	if ns != entry.Namespace() || cn != entry.CollectionName() {
		return fmt.Errorf(
			"galaxy.yml namespace.name (%s.%s) does not match repository name (%s)",
			ns, cn, entry.GetFQCN(),
		)
	}
	return nil
}

// extractCollection replaces an installed collection with the archive, marked as agru's while it runs.
func (i *Installer) extractCollection(entry *models.Collection, galaxy map[string]any, target, tmpfile string) error {
	if err := RemoveCollectionDir(target); err != nil {
		return err
	}
	if err := os.MkdirAll(path.Dir(target), 0o755); err != nil {
		return fmt.Errorf("creating target dir: %w", err)
	}
	if err := os.Mkdir(target, 0o700); err != nil && !errors.Is(err, fs.ErrExist) {
		return fmt.Errorf("creating target dir: %w", err)
	}
	marker := path.Join(target, installMarkerName)
	// marker first: an interrupted install stays replaceable, and a manifest here would read as installed
	if err := os.WriteFile(marker, []byte(entry.GetFQCN()), 0o600); err != nil {
		return fmt.Errorf("writing install marker: %w", err)
	}
	tarArgs := []string{"tar", "-xf", tmpfile}
	if _, err := i.runner.RunArgs(tarArgs, target); err != nil {
		return fmt.Errorf("extracting archive: %w", err)
	}
	// manifest after the tree is in place, so a half-extracted dir never claims to be installed
	if err := i.writeCollectionManifest(entry, galaxy, target); err != nil {
		return err
	}
	if err := os.Remove(marker); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("removing install marker: %w", err)
	}
	return nil
}

// CollFS returns the filesystem used for reading collection metadata.
func (i *Installer) CollFS() fs.FS {
	return i.collFS
}

// GetInstalledCollections returns collections whose GetInstalledVersion returns a non-empty version.
func (i *Installer) GetInstalledCollections(colls models.Collections) models.Collections {
	installed := make(models.Collections, 0, len(colls))
	for _, c := range colls {
		if c.GetInstalledVersion(i.collFS) != "" {
			installed = append(installed, c)
		}
	}
	return installed
}

// bootstrapCollections creates the collections path if it doesn't exist.
func (i *Installer) bootstrapCollections() error {
	cp := path.Join(i.collPath, "ansible_collections")
	_, err := os.Stat(cp)
	if err != nil && os.IsNotExist(err) {
		if mkerr := os.MkdirAll(cp, 0o755); mkerr != nil {
			return fmt.Errorf("creating collections path: %w", mkerr)
		}
	}
	return nil
}

// cleanupCollection removes all temporary dirs and files created during collection installation.
func (i *Installer) cleanupCollection(tmpdir, tmpfile string) {
	os.RemoveAll(tmpdir)
	os.Remove(tmpfile)
}

// orHead returns "HEAD" when version is empty, otherwise the version.
func orHead(v string) string {
	if v == "" {
		return "HEAD"
	}
	return v
}

// collectionCloneArgs builds a safe argv slice for git clone of a collection repo.
func collectionCloneArgs(repoURL, version, tmpdir string) []string {
	args := []string{"git", "clone", "-q", "--depth", "1"}
	if version != "" {
		if len(version) >= 40 {
			args = append(args, "-c", "remote.origin.fetch=+"+version+":refs/remotes/origin/"+version)
		} else {
			args = append(args, "-b", version)
		}
	}
	return append(args, "--", repoURL, tmpdir)
}
