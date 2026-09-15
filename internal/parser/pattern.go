package parser

import (
	"fmt"
	"os"
	"sort"
	"strings"
	"sync"

	"github.com/bmatcuk/doublestar/v4"
	"gopkg.in/yaml.v3"

	"github.com/etkecc/agru/internal/models"
)

// RequirementsFile bundles one matched requirements file with its parsed content.
type RequirementsFile struct {
	Path        string
	Entries     models.File
	Additional  models.File
	Extras      map[string]yaml.Node
	Collections models.Collections
	MapForm     bool
}

// ParsePattern expands pattern and parses matched files; multiple patterns can be separated by ';'.
func (p *Parser) ParsePattern(pattern string) ([]RequirementsFile, error) {
	var files []RequirementsFile
	patterns := strings.Split(pattern, ";")
	for _, pat := range patterns {
		pat = strings.TrimSpace(pat)
		if pat == "" {
			continue
		}
		paths, err := p.expandPattern(pat)
		if err != nil {
			return nil, err
		}
		for _, path := range paths {
			entries, additional, colls, extras, mapForm, err := p.ParseFile(path)
			if err != nil {
				return nil, err
			}
			files = append(files, RequirementsFile{Path: path, Entries: entries, Additional: additional, Collections: colls, Extras: extras, MapForm: mapForm})
		}
	}
	if len(files) == 0 {
		return nil, fmt.Errorf("no requirements files match pattern %s", pattern)
	}
	return files, nil
}

// MergeAll merges every parsed file into one deduplicated, sorted slice; earlier files win on name clashes.
func (p *Parser) MergeAll(files []RequirementsFile) models.File {
	var merged models.File
	for i, f := range files {
		if i == 0 {
			merged = p.MergeFiles(f.Entries, f.Additional)
		} else {
			merged = p.MergeFiles(merged, f.Entries, f.Additional)
		}
	}
	return merged
}

// MergeAllCollections merges all collections across files; earlier files win on FQCN clashes.
func (p *Parser) MergeAllCollections(files []RequirementsFile) models.Collections {
	var merged models.Collections
	for _, f := range files {
		merged = append(merged, f.Collections...)
	}
	merged = merged.Deduplicate()
	merged.Sort()
	return merged
}

// UpdateAll updates every file concurrently, sharing one progress channel that is closed when all files finish.
func (p *Parser) UpdateAll(files []RequirementsFile, progress chan<- CheckProgress) []error {
	errs := make([]error, len(files))
	var wg sync.WaitGroup
	for i, f := range files {
		ch := make(chan CheckProgress)
		wg.Add(2)
		go func() {
			defer wg.Done()
			for pr := range ch {
				progress <- pr
			}
		}()
		go func() {
			defer wg.Done()
			errs[i] = p.UpdateFile(f.Entries, f.Collections, f.Extras, f.MapForm, f.Path, ch)
		}()
	}
	wg.Wait()
	close(progress)
	return errs
}

// expandPattern returns the sorted regular files matching pattern; wildcard-free patterns pass through as-is.
func (p *Parser) expandPattern(pattern string) ([]string, error) {
	if !strings.ContainsAny(pattern, "*?[{") {
		return []string{pattern}, nil
	}
	matches, err := doublestar.FilepathGlob(pattern)
	if err != nil {
		return nil, fmt.Errorf("expanding pattern %s: %w", pattern, err)
	}
	files := make([]string, 0, len(matches))
	for _, m := range matches {
		info, statErr := os.Stat(m)
		if statErr != nil || !info.Mode().IsRegular() {
			continue // drop directories and entries that vanished between glob and stat
		}
		files = append(files, m)
	}
	if len(files) == 0 {
		return nil, fmt.Errorf("no requirements files match pattern %s", pattern)
	}
	sort.Strings(files)
	return files, nil
}
