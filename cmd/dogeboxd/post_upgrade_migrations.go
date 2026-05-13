package main

import (
	"log"

	dogeboxd "github.com/Dogebox-WG/dogeboxd/pkg"
	"github.com/Dogebox-WG/dogeboxd/pkg/system/migrations"
)

type postUpgradeMigration struct {
	name string
	run  func() (string, bool, error)
}

func (t server) checkAndPerformPostUpgradeMigrations(dbx dogeboxd.Dogeboxd) bool {
	postUpgradeMigrations := []postUpgradeMigration{
		{
			name: "OS flake migrator",
			run: func() (string, bool, error) {
				return migrations.QueueOSFlakeMigratorIfNeeded(t.config, dbx.AddAction)
			},
		},
	}

	for _, migration := range postUpgradeMigrations {
		jobID, queued, err := migration.run()
		if err != nil {
			log.Printf("%s check failed: %v", migration.name, err)
			continue
		}
		if queued {
			log.Printf("Queued startup %s job: %s", migration.name, jobID)
			return true
		}
	}

	return false
}
