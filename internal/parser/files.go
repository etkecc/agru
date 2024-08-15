package parser

import (
	"os"
	"sync"

	"gopkg.in/yaml.v3"

	"github.com/etkecc/agru/internal/models"
	"github.com/etkecc/agru/internal/utils"
)

// ParseFile parses requirements.yml file
func ParseFile(path string) (main, additional models.File) {
	fileb, err := os.ReadFile(path)
	if err != nil {
		utils.Log("ERROR: reading file", path, err)
		return models.File{}, models.File{}
	}
	var req models.File
	if err := yaml.Unmarshal(fileb, &req); err != nil {
		var reqMap models.FileMap
		if err := yaml.Unmarshal(fileb, &reqMap); err == nil {
			req = reqMap.Slice()
		} else {
			utils.Log("ERROR: unmarshalling yaml", err)
		}
	}
	req.Sort()

	return req, parseAdditionalFile(req)
}

// parseAdditionalFile parses additional requirements.yml files
func parseAdditionalFile(req models.File) models.File {
	additional := make([]*models.Entry, 0)
	for _, entry := range req {
		if entry.Include != "" {
			// no recursive iteration over deeper levels, because it's not used anywhere
			additionalLvl1, additionalLvl2 := ParseFile(entry.Include)
			additional = append(additional, additionalLvl1...)
			additional = append(additional, additionalLvl2...)
		}
	}

	return additional
}

// UpdateFile updates the requirements.yml file
func UpdateFile(entries models.File, requirementsPath string) {
	var wg sync.WaitGroup
	wg.Add(len(entries))
	for i, entry := range entries {
		go func(i int, entry *models.Entry, wg *sync.WaitGroup) {
			newVersion := getNewVersion(entry.Src, entry.Version)
			if newVersion != "" {
				utils.Log(entry.Src, entry.Version, "->", newVersion)
				entry.Version = newVersion
				entries[i] = entry
			}
			wg.Done()
		}(i, entry, &wg)
	}
	wg.Wait()

	outb, err := yaml.Marshal(entries)
	if err != nil {
		utils.Log("ERROR: marshaling yaml", err)
		return
	}
	outb = append([]byte("---\n\n"), outb...) // preserve the separator to make yaml lint happy
	if err := os.WriteFile(requirementsPath, outb, 0o600); err != nil {
		utils.Log("ERROR: writing file", requirementsPath, err)
	}
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
