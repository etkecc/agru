package main

import (
	"log"
	"os"
	"sync"

	"gopkg.in/yaml.v3"
)

// RequirementsFile structure
type RequirementsFile []RequirementsEntry

// parseRequirements parses requirements.yml file and tries to update it
// if it founds any includes within that file, they will be returned as second return value
func parseRequirements(path string) (RequirementsFile, RequirementsFile) {
	fileb, err := os.ReadFile(path)
	if err != nil {
		log.Println("ERROR: ", err)
		return RequirementsFile{}, RequirementsFile{}
	}
	var req RequirementsFile
	if err := yaml.Unmarshal(fileb, &req); err != nil {
		log.Println("ERROR: ", err)
	}

	return req, parseAdditionalRequirements(req)
}

func parseAdditionalRequirements(req RequirementsFile) RequirementsFile {
	additional := make([]RequirementsEntry, 0)
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
		go func(i int, entry RequirementsEntry, wg *sync.WaitGroup) {
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
	if err := os.WriteFile(requirementsPath, outb, 0o600); err != nil {
		log.Println("ERROR: ", err)
	}
}
