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

// collectionPups maps collection names to their respective pups
var collectionPups = map[string][]CollectionPup{
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

// processPupCollections handles the installation of pups from the selected collection
func processPupCollections(sm dogeboxd.StateManager, dbx dogeboxd.Dogeboxd, sessionToken string, collectionName string) {
	// Get the list of pups for the selected collection
	pupsToInstall, exists := collectionPups[collectionName]
	if !exists {
		fmt.Printf(">> Unknown collection: %s\n", collectionName)
	} else {
		// Create and queue jobs for each pup in the collection
		for _, pup := range pupsToInstall {
			req := dogeboxd.InstallPup{
				PupName:      pup.Name,
				PupVersion:   pup.Version,
				SourceId:     pup.SourceId,
				SessionToken: sessionToken,
			}

			//Add the action to the system updater
			dbx.AddAction(req)
		}
	}
}
