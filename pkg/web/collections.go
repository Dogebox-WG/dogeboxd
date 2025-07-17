package web

import (
	"fmt"

	dogeboxd "github.com/dogeorg/dogeboxd/pkg"
)

// CollectionPup represents a pup that belongs to a collection
type CollectionPup struct {
	Name     string
	Version  string
	SourceId string
}

// FoundationPupCollections maps collection names to their respective pups
var FoundationPupCollections = map[string][]CollectionPup{
	"core": {
		{
			Name:     "Dogecoin Core",
			Version:  "0.0.7",
			SourceId: "dogeorg.pups",
		},
	},
	"essentials": {
		{
			Name:     "Dogecoin Core",
			Version:  "0.0.7",
			SourceId: "dogeorg.pups",
		},
		{
			Name:     "Dogenet",
			Version:  "0.0.2",
			SourceId: "dogeorg.pups",
		},
		{
			Name:     "Dogemap",
			Version:  "0.0.2",
			SourceId: "dogeorg.pups",
		},
		{
			Name:     "Identity",
			Version:  "0.0.3",
			SourceId: "dogeorg.pups",
		},
	},
	"custom": {},
}

// processPupCollections handles the installation of pups for a given collection
func processPupCollections(sm dogeboxd.StateManager, dbx dogeboxd.Dogeboxd, sessionToken string, collectionName string) {
	// Get the list of pups for the selected collection
	pupsToInstall, exists := FoundationPupCollections[collectionName]
	if !exists {
		fmt.Printf(">> Unknown collection: %s\n", collectionName)
		return
	}

	// Create a batch installation request
	installRequests := make([]dogeboxd.InstallPup, len(pupsToInstall))
	for i, pup := range pupsToInstall {
		installRequests[i] = dogeboxd.InstallPup{
			PupName:      pup.Name,
			PupVersion:   pup.Version,
			SourceId:     pup.SourceId,
			Options:      dogeboxd.AdoptPupOptions{},
			SessionToken: sessionToken,
		}
	}

	// Add the batch installation action
	dbx.AddAction(dogeboxd.InstallPups(installRequests))
}
