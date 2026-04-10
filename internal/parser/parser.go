package parser

import (
	"fmt"
	"os"
	"strings"
	"sync"

	"gopkg.in/yaml.v3"

	"github.com/etkecc/agru/internal/models"
	"github.com/etkecc/agru/internal/runner"
	"github.com/etkecc/agru/internal/utils"
)

var ignoredVersions = map[string]bool{
	"main":   true,
	"master": true,
}

// Parser handles parsing and updating of Ansible Galaxy requirements.yml files.
// It uses a Runner to check for newer versions of roles via git ls-remote.
type Parser struct {
	runner  runner.Runner
	verbose bool
}

// New creates a new Parser with the given runner and verbose flag
func New(r runner.Runner, verbose bool) *Parser {
	return &Parser{runner: r, verbose: verbose}
}

// ParseFile parses requirements.yml file
func (p *Parser) ParseFile(path string) (main, additional models.File, err error) {
	fileb, err := os.ReadFile(path)
	if err != nil {
		return models.File{}, models.File{}, fmt.Errorf("reading file %s: %w", path, err)
	}
	var req models.File
	if err := yaml.Unmarshal(fileb, &req); err != nil {
		var reqMap models.FileMap
		if err := yaml.Unmarshal(fileb, &reqMap); err != nil {
			return models.File{}, models.File{}, fmt.Errorf("unmarshalling yaml %s: %w", path, err)
		}
		req = reqMap.Slice()
	}
	req = req.Deduplicate()
	req.Sort()

	additional, err = p.parseAdditionalFile(req)
	if err != nil {
		return models.File{}, models.File{}, fmt.Errorf("parsing additional file: %w", err)
	}

	return req, additional, nil
}

// parseAdditionalFile parses additional requirements.yml files referenced via include
func (p *Parser) parseAdditionalFile(req models.File) (models.File, error) {
	additional := make([]*models.Entry, 0)
	for _, entry := range req {
		if entry.Include != "" {
			// no recursive iteration over deeper levels, because it's not used anywhere
			additionalLvl1, additionalLvl2, err := p.ParseFile(entry.Include)
			if err != nil {
				return nil, err
			}
			additional = append(additional, additionalLvl1...)
			additional = append(additional, additionalLvl2...)
		}
	}

	return additional, nil
}

// UpdateFile updates the requirements.yml file with the latest versions
func (p *Parser) UpdateFile(entries models.File, requirementsPath string) error {
	changes, errs := p.checkVersions(entries)

	if len(changes) > 0 {
		utils.Log(changes.String("requirements changes:\n"))
	}
	if len(errs) > 0 {
		errStrs := make([]string, 0, len(errs))
		for _, err := range errs {
			errStrs = append(errStrs, err.Error())
		}
		return fmt.Errorf("errors occurred during updating:\n%s", strings.Join(errStrs, "\n"))
	}

	outb, err := yaml.Marshal(entries)
	if err != nil {
		return fmt.Errorf("marshaling yaml: %w", err)
	}
	outb = append([]byte("---\n\n"), outb...) // preserve the separator to make yaml lint happy
	if err := os.WriteFile(requirementsPath, outb, 0o600); err != nil {
		return fmt.Errorf("writing file %s: %w", requirementsPath, err)
	}
	return nil
}

// checkVersions concurrently checks all entries for newer versions and updates them in place.
// Returns the set of updated items and any errors encountered.
func (p *Parser) checkVersions(entries models.File) (models.UpdatedItems, []error) {
	var (
		mu      sync.Mutex
		wg      sync.WaitGroup
		changes models.UpdatedItems
		errs    []error
	)

	bar := utils.NewProgressbar(entries.RolesLen(), "updating requirements file")
	for i, entry := range entries {
		if entry.Include != "" { // skip entries with include directive
			continue
		}
		wg.Add(1)
		go func(i int, entry *models.Entry) {
			defer wg.Done()
			defer bar.Add(1) //nolint:errcheck // don't care about error here

			newVersion, err := p.getNewVersion(entry.Src, entry.Version)
			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				errs = append(errs, fmt.Errorf("getting new version for %s@%s: %w", entry.GetName(), entry.Version, err))
				bar.AddDetail(fmt.Sprintf("failed %s@%s", entry.GetName(), entry.Version)) //nolint:errcheck // don't care about error here
				return
			}
			if newVersion == "" {
				return
			}
			changes = changes.Add(entry.GetName(), entry.Version, newVersion)
			entry.Version = newVersion
			entries[i] = entry
			bar.AddDetail(fmt.Sprintf("updated %s@%s -> %s", entry.GetName(), entry.Version, newVersion)) //nolint:errcheck // that's ok
		}(i, entry)
	}
	wg.Wait()
	return changes, errs
}

// MergeFiles merges all requirements.yml files entries into one slice,
// deduplicates them and prioritizes entries from the main requirements.yml file
func (p *Parser) MergeFiles(mainReq models.File, additionalReqs ...models.File) models.File {
	uniq := make(map[string]*models.Entry, 0)
	for _, entry := range mainReq {
		uniq[entry.GetName()] = entry
	}
	additionalEntries := make(models.File, 0, len(additionalReqs))
	for _, additionalReq := range additionalReqs {
		additionalEntries = append(additionalEntries, additionalReq...)
	}

	for _, entry := range additionalEntries {
		if _, ok := uniq[entry.GetName()]; !ok {
			uniq[entry.GetName()] = entry
		}
	}

	entries := make(models.File, 0, len(uniq))
	for _, entry := range uniq {
		entries = append(entries, entry)
	}
	entries.Sort()

	return entries
}

// getNewVersion checks for newer git tag available on the src's remote
func (p *Parser) getNewVersion(src, version string) (string, error) {
	if ignoredVersions[version] {
		return "", nil
	}

	// not a git repo
	if !strings.Contains(src, "git") {
		return "", nil
	}

	repo := strings.Replace(src, "git+https", "https", 1)
	tags, err := p.runner.Run("git ls-remote -tq --sort=-version:refname "+repo, "")
	if err != nil {
		return "", fmt.Errorf("running git ls-remote: %w", err)
	}
	if tags == "" {
		return "", nil
	}

	lastline := strings.Split(tags, "\n")[0]
	tagidx := strings.Index(lastline, "refs/tags/")
	if tagidx == -1 {
		return "", fmt.Errorf("cannot find tag in git ls-remote output, lastline: %s", lastline)
	}
	last := strings.Replace(lastline[tagidx:], "refs/tags/", "", 1)
	last = strings.Replace(last, "^{}", "", 1) // NOTE: very weird case with some github repos, didn't find out why it does that
	if last != version {
		return last, nil
	}

	return "", nil
}
