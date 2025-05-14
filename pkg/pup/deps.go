package pup

import (
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/Masterminds/semver"
	dogeboxd "github.com/dogeorg/dogeboxd/pkg"
)

func (t PupManager) CalculateDeps(pupID string) ([]dogeboxd.PupDependencyReport, error) {
	// First try to get the pup from the state map
	pup, ok := t.state[pupID]
	if !ok {
		// Let's check if this is an uninstalled pup by looking in sources
		sourceList, err := t.sourceManager.GetAll(false)
		if err != nil {
			return []dogeboxd.PupDependencyReport{}, errors.New("no such pup and failed to check sources")
		}
		
		// Parse the pupID to get name and version
		parts := strings.Split(pupID, "-")
		if len(parts) != 2 {
			return []dogeboxd.PupDependencyReport{}, errors.New("invalid pup ID format")
		}
		pupName := parts[0]
		pupVersion := parts[1]
		
		// Search through sources for this pup
		for _, list := range sourceList {
			for _, p := range list.Pups {
				if p.Name == pupName && p.Version == pupVersion {
					// Create a temporary state for this uninstalled pup
					tempState := &dogeboxd.PupState{
						Manifest: p.Manifest,
						Providers: make(map[string]string),
					}
					return t.calculateDeps(tempState), nil
				}
			}
		}
		
		return []dogeboxd.PupDependencyReport{}, errors.New("no such pup")
	}
	
	return t.calculateDeps(pup), nil
}

// This function calculates a DependencyReport for every
// dep that a given pup requires
func (t PupManager) calculateDeps(pupState *dogeboxd.PupState) []dogeboxd.PupDependencyReport {
	deps := []dogeboxd.PupDependencyReport{}
	for _, dep := range pupState.Manifest.Dependencies {
		report := dogeboxd.PupDependencyReport{
			Interface: dep.InterfaceName,
			Version:   dep.InterfaceVersion,
			Optional:  dep.Optional,
		}

		constraint, err := semver.NewConstraint(dep.InterfaceVersion)
		if err != nil {
			fmt.Printf("Invalid version constraint: %s, %s:%s\n", pupState.Manifest.Meta.Name, dep.InterfaceName, dep.InterfaceVersion)
			deps = append(deps, report)
			continue
		}

		// Is there currently a provider set?
		report.CurrentProvider = pupState.Providers[dep.InterfaceName]

		// What are all installed pups that can provide the interface?
		installed := []string{}
		for id, p := range t.state {
			// search the interfaces and check against constraint
			for _, iface := range p.Manifest.Interfaces {
				ver, err := semver.NewVersion(iface.Version)
				if err != nil {
					continue
				}
				if iface.Name == dep.InterfaceName && constraint.Check(ver) == true {
					installed = append(installed, id)
				}
			}
		}
		report.InstalledProviders = installed

		// What are all available pups that can provide the interface?
		available := []dogeboxd.PupManifestDependencySource{}
		sourceList, err := t.sourceManager.GetAll(false)
		if err == nil {
			for _, list := range sourceList {
				// search the interfaces and check against constraint
				for _, p := range list.Pups {
					for _, iface := range p.Manifest.Interfaces {
						ver, err := semver.NewVersion(iface.Version)
						if err != nil {
							continue
						}
						if iface.Name == dep.InterfaceName && constraint.Check(ver) == true {
							// check if this isnt alread installed..
							alreadyInstalled := false
							for _, installedPupID := range installed {
								iPup, _, err := t.GetPup(installedPupID)
								if err != nil {
									continue
								}
								if iPup.Source.Location == list.Config.Location && iPup.Manifest.Meta.Name == p.Name {
									// matching location and name, assume already installed
									alreadyInstalled = true
									break
								}
							}

							if !alreadyInstalled {
								// Check if this provider is already in the available list
								isDuplicate := false
								for _, existing := range available {
									if existing.SourceLocation == list.Config.Location &&
										existing.PupName == p.Name &&
										existing.PupVersion == p.Version {
										isDuplicate = true
										break
									}
								}

								if !isDuplicate {
									available = append(available, dogeboxd.PupManifestDependencySource{
										SourceLocation: list.Config.Location,
										PupName:        p.Name,
										PupVersion:     p.Version,
									})
								}
							}
						}
					}
				}
			}
			// Sort available providers by version in descending order
			sort.Slice(available, func(i, j int) bool {
				vi, err := semver.NewVersion(available[i].PupVersion)
				if err != nil {
					return false
				}
				vj, err := semver.NewVersion(available[j].PupVersion)
				if err != nil {
					return false
				}
				return vi.GreaterThan(vj)
			})
			report.InstallableProviders = available
		}

		// Is there a DefaultSourceProvider
		report.DefaultSourceProvider = dep.DefaultSource

		deps = append(deps, report)
	}
	return deps
}
