package main

import (
	"log"
	"os"
	"sort"
	"sync"

	"gopkg.in/yaml.v3"
)

// RequirementsFile structure
type RequirementsFile []*RequirementsEntry

// Sort entries by name
func (r RequirementsFile) Sort() {
	sort.Slice(r, func(i, j int) bool {
		return r[i].GetName() < r[j].GetName()
	})
}

// parseRequirements parses requirements.yml file and tries to update it
// if it founds any includes within that file, they will be returned as second return value
func parseRequirements(path string) (main, additional RequirementsFile) {
	fileb, err := os.ReadFile(path)
	if err != nil {
		log.Println("ERROR: ", err)
		return RequirementsFile{}, RequirementsFile{}
	}
	var req RequirementsFile
	if err := yaml.Unmarshal(fileb, &req); err != nil {
		log.Println("ERROR: ", err)
	}
	req.Sort()

	return req, parseAdditionalRequirements(req)
}

func parseAdditionalRequirements(req RequirementsFile) RequirementsFile {
	additional := make([]*RequirementsEntry, 0)
	for _, entry := range req {
		if entry.Include != "" {
			// no recursive iteration over deeper levels, because it's not used anywhere
			additionalLvl1, additionalLvl2 := parseRequirements(entry.Include)
			additional = append(additional, additionalLvl1...)
			additional = append(additional, additionalLvl2...)
		}
	}

	return additional
}

// updateRequirements updates requirements file
func updateRequirements(entries RequirementsFile) {
	var wg sync.WaitGroup
	wg.Add(len(entries))
	for i, entry := range entries {
		go func(i int, entry *RequirementsEntry, wg *sync.WaitGroup) {
			newVersion := getNewVersion(entry.Src, entry.Version)
			if newVersion != "" {
				log.Println(entry.Src, entry.Version, "->", newVersion)
				entry.Version = newVersion
				entries[i] = entry
			}
			wg.Done()
		}(i, entry, &wg)
	}
	wg.Wait()

	outb, err := yaml.Marshal(entries)
	if err != nil {
		log.Println("ERROR: ", err)
		return
	}
	outb = append([]byte("---\n\n"), outb...) // preserve the separator to make yaml lint happy
	if err := os.WriteFile(requirementsPath, outb, 0o600); err != nil {
		log.Println("ERROR: ", err)
	}
}

// mergeRequirementsEntries merges all requirements.yml files entries into one slice,
// deduplicates them and prioritizes entries from the main requirements.yml file
func mergeRequirementsEntries(mainReq RequirementsFile, additionalReqs ...RequirementsFile) RequirementsFile {
	uniq := make(map[string]*RequirementsEntry, 0)
	for _, entry := range mainReq {
		uniq[entry.GetName()] = entry
	}
	additionalEntries := make(RequirementsFile, 0, len(additionalReqs))
	for _, additionalReq := range additionalReqs {
		additionalEntries = append(additionalEntries, additionalReq...)
	}

	for _, entry := range additionalEntries {
		if _, ok := uniq[entry.GetName()]; !ok {
			uniq[entry.GetName()] = entry
		}
	}

	entries := make(RequirementsFile, 0, len(uniq))
	for _, entry := range uniq {
		entries = append(entries, entry)
	}
	entries.Sort()

	return entries
}
