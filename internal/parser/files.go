package parser

import (
	"fmt"
	"os"
	"strings"
	"sync"

	"gopkg.in/yaml.v3"

	"github.com/etkecc/agru/internal/models"
	"github.com/etkecc/agru/internal/utils"
)

// ParseFile parses requirements.yml file
func ParseFile(path string) (main, additional models.File, err error) {
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

	additional, err = parseAdditionalFile(req)
	if err != nil {
		return models.File{}, models.File{}, fmt.Errorf("parsing additional file: %w", err)
	}

	return req, additional, nil
}

// parseAdditionalFile parses additional requirements.yml files
func parseAdditionalFile(req models.File) (models.File, error) {
	additional := make([]*models.Entry, 0)
	for _, entry := range req {
		if entry.Include != "" {
			// no recursive iteration over deeper levels, because it's not used anywhere
			additionalLvl1, additionalLvl2, err := ParseFile(entry.Include)
			if err != nil {
				return nil, err
			}
			additional = append(additional, additionalLvl1...)
			additional = append(additional, additionalLvl2...)
		}
	}

	return additional, nil
}

// UpdateFile updates the requirements.yml file
//
//nolint:gocognit // TODO: refactor
func UpdateFile(entries models.File, requirementsPath string) error {
	changes := models.UpdatedItems{}
	bar := utils.NewProgressbar(entries.RolesLen(), "updating requirements file")
	errchan := make(chan error, entries.RolesLen())
	errs := []error{}
	go func(errchan chan error) {
		for err := range errchan {
			errs = append(errs, err)
		}
	}(errchan)

	var wg sync.WaitGroup
	for i, entry := range entries {
		if entry.Include != "" { // skip entries with include directive
			continue
		}
		wg.Add(1)
		go func(i int, entry *models.Entry, wg *sync.WaitGroup, errchan chan error) {
			defer wg.Done()
			defer bar.Add(1) //nolint:errcheck // don't care about error here

			newVersion, err := getNewVersion(entry.Src, entry.Version)
			if err != nil {
				errchan <- fmt.Errorf("getting new version for %s@%s: %w", entry.GetName(), entry.Version, err)
				bar.AddDetail(fmt.Sprintf("failed %s@%s", entry.GetName(), entry.Version)) //nolint:errcheck // don't care about error here
				return
			}
			if newVersion == "" {
				return
			}

			changes = changes.Add(entry.GetName(), entry.Version, newVersion)
			bar.AddDetail(fmt.Sprintf("updated %s@%s -> %s", entry.GetName(), entry.Version, newVersion)) //nolint:errcheck // that's ok
			entry.Version = newVersion
			entries[i] = entry
		}(i, entry, &wg, errchan)
	}
	wg.Wait()

	if len(changes) > 0 {
		utils.Log(changes.String("requirements changes:\n"))
	}
	close(errchan)
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

// MergeFiles merges all requirements.yml files entries into one slice,
// deduplicates them and prioritizes entries from the main requirements.yml file
func MergeFiles(mainReq models.File, additionalReqs ...models.File) models.File {
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
