package parser

import (
	"bytes"
	"fmt"
	"os"
	"strings"
	"sync"

	"gopkg.in/yaml.v3"

	"github.com/etkecc/agru/internal/models"
	"github.com/etkecc/agru/internal/runner"
)

var ignoredVersions = map[string]bool{
	"main":   true,
	"master": true,
}

const maxIncludeDepth = 2

// CheckProgress represents a version check result for a single role or collection.
type CheckProgress struct {
	Name   string
	OldVer string
	NewVer string // empty = up to date; non-empty = newer version found
	Notice string // unsupported-entry reason, empty when supported
	Err    error
}

// Parser parses and updates Ansible Galaxy requirements.yml files.
type Parser struct {
	runner runner.Runner
}

// New creates a new Parser with the given runner
func New(r runner.Runner) *Parser {
	return &Parser{runner: r}
}

// ParseFile parses requirements.yml; returns roles, collections, extras, and mapForm flag.
//
//nolint:gocritic // 6 values by design, matches 4 parsed concerns
func (p *Parser) ParseFile(path string) (models.File, models.File, models.Collections, map[string]yaml.Node, bool, error) {
	return p.parseFile(path, 0)
}

// parseFile is the depth-tracked ParseFile implementation, used for include expansion
func (p *Parser) parseFile(path string, depth int) (main, additional models.File, colls models.Collections, extras map[string]yaml.Node, mapForm bool, err error) { //nolint:gocritic // 6 results by design
	fileb, err := os.ReadFile(path)
	if err != nil {
		return models.File{}, models.File{}, nil, nil, false, fmt.Errorf("reading file %s: %w", path, err)
	}
	var req models.File
	if err := yaml.Unmarshal(fileb, &req); err != nil {
		var reqMap models.FileMap
		if err := yaml.Unmarshal(fileb, &reqMap); err != nil {
			return models.File{}, models.File{}, nil, nil, false, fmt.Errorf("unmarshalling yaml %s: %w", path, err)
		}
		req = reqMap.Slice()
		extras = reqMap.Rest
		colls = reqMap.Collections.Deduplicate()
		colls.Sort()
		mapForm = true
	}
	req = req.Deduplicate()
	req.Sort()

	// Flag unsupported role entries for both list and map forms
	for _, entry := range req {
		if entry.Include != "" {
			continue
		}
		if entry.Src != "" && !models.IsGitURL(entry.Src) {
			entry.SetUnsupported("not a git source")
		}
	}

	additional, err = p.parseAdditionalFile(req, depth)
	if err != nil {
		return models.File{}, models.File{}, nil, nil, false, fmt.Errorf("parsing additional file: %w", err)
	}

	return req, additional, colls, extras, mapForm, nil
}

// parseAdditionalFile parses additional requirements.yml files referenced via include
func (p *Parser) parseAdditionalFile(req models.File, depth int) (models.File, error) {
	additional := make([]*models.Entry, 0)
	for _, entry := range req {
		if entry.Include == "" {
			continue
		}
		if depth+1 > maxIncludeDepth {
			return nil, fmt.Errorf("include depth exceeded (max %d levels)", maxIncludeDepth)
		}
		additionalLvl1, additionalLvl2, _, _, _, err := p.parseFile(entry.Include, depth+1)
		if err != nil {
			return nil, err
		}
		additional = append(additional, additionalLvl1...)
		additional = append(additional, additionalLvl2...)
	}

	return additional, nil
}

// UpdateFile updates requirements.yml with the latest versions; closes the progress channel (if non-nil) when done.
func (p *Parser) UpdateFile(entries models.File, colls models.Collections, extras map[string]yaml.Node, mapForm bool, requirementsPath string, progress chan<- CheckProgress) error {
	// Build checkItems from roles and collections.
	items := make([]checkItem, 0, len(entries)+len(colls))
	for _, entry := range entries {
		if entry.Include != "" {
			continue
		}
		items = append(items, checkItem{
			display: entry.GetName(),
			src:     entry.Src,
			version: entry.Version,
			notice:  entry.Unsupported(),
			setVer:  func(v string) { entry.Version = v },
		})
	}
	for _, col := range colls {
		items = append(items, checkItem{
			display: col.GetFQCN(),
			src:     col.Name,
			version: col.Version,
			notice:  col.Unsupported(),
			setVer:  func(v string) { col.Version = v },
		})
	}

	_, errs := p.checkItems(items, progress)
	if len(errs) > 0 {
		errStrs := make([]string, 0, len(errs))
		for _, err := range errs {
			errStrs = append(errStrs, err.Error())
		}
		return fmt.Errorf("errors occurred during updating:\n%s", strings.Join(errStrs, "\n"))
	}

	var (
		outb []byte
		err  error
	)
	if mapForm || len(colls) > 0 || len(extras) > 0 {
		outb, err = p.marshal(models.FileMap{Roles: entries, Collections: colls, Rest: extras})
	} else {
		outb, err = p.marshal(entries)
	}
	if err != nil {
		return fmt.Errorf("marshaling yaml: %w", err)
	}
	outb = append([]byte("---\n\n"), outb...) // preserve the separator to make yaml lint happy
	if err := os.WriteFile(requirementsPath, outb, 0o600); err != nil {
		return fmt.Errorf("writing file %s: %w", requirementsPath, err)
	}
	return nil
}

// marshal yaml with proper indentation
func (p *Parser) marshal(v any) ([]byte, error) {
	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(v); err != nil {
		return nil, err
	}
	enc.Close()
	return buf.Bytes(), nil
}

// checkItem is a single version-check item for a role or collection.
type checkItem struct {
	display string // roles: GetName(); collections: GetFQCN()
	src     string // roles: Src; collections: Name
	version string
	notice  string // Unsupported() reason, empty when supported
	setVer  func(string)
}

// checkItems concurrently checks and updates all items, closing progress (if non-nil) when done.
func (p *Parser) checkItems(items []checkItem, progress chan<- CheckProgress) (models.UpdatedItems, []error) {
	var (
		mu      sync.Mutex
		wg      sync.WaitGroup
		changes models.UpdatedItems
		errs    []error
	)

	for _, item := range items {
		wg.Go(func() { p.checkOneItem(item, &mu, &changes, &errs, progress) })
	}
	wg.Wait()
	if progress != nil {
		close(progress)
	}
	return changes, errs
}

// checkOneItem checks a single item for a newer version and updates it in place.
func (p *Parser) checkOneItem(item checkItem, mu *sync.Mutex, changes *models.UpdatedItems, errs *[]error, progress chan<- CheckProgress) {
	// Unsupported entry: emit notice row, no network.
	if item.notice != "" {
		if progress != nil {
			mu.Lock()
			progress <- CheckProgress{Name: item.display, Notice: item.notice}
			mu.Unlock()
		}
		return
	}

	// Empty version: HEAD semantics, -u never pins it. Emit up-to-date row.
	if item.version == "" {
		if progress != nil {
			mu.Lock()
			progress <- CheckProgress{Name: item.display, OldVer: ""}
			mu.Unlock()
		}
		return
	}

	// Check for newer version via ls-remote.
	newVersion, err := p.getNewVersion(item.src, item.version)
	mu.Lock()
	defer mu.Unlock()
	if err != nil {
		*errs = append(*errs, fmt.Errorf("getting new version for %s@%s: %w", item.display, item.version, err))
		if progress != nil {
			progress <- CheckProgress{Name: item.display, OldVer: item.version, Err: err}
		}
		return
	}
	if newVersion != "" {
		*changes = changes.Add(item.display, item.version, newVersion)
		if progress != nil {
			progress <- CheckProgress{Name: item.display, OldVer: item.version, NewVer: newVersion}
		}
		item.setVer(newVersion)
		return
	}
	if progress != nil {
		progress <- CheckProgress{Name: item.display, OldVer: item.version}
	}
}

// MergeFiles merges all requirements.yml entries into one deduplicated slice.
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
	if !models.IsGitURL(src) {
		return "", nil
	}

	repo := strings.Replace(src, "git+https", "https", 1)
	if err := models.ValidateGitArg(repo); err != nil {
		return "", fmt.Errorf("invalid role repo URL %q: %w", repo, err)
	}
	tags, err := p.runner.RunArgs([]string{"git", "ls-remote", "-tq", "--sort=-version:refname", repo}, "")
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
