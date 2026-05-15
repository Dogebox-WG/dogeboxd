package main

import (
	"log"

	dogeboxd "github.com/Dogebox-WG/dogeboxd/pkg"
	"github.com/Dogebox-WG/dogeboxd/pkg/system/migrations"
)

func (t server) checkAndPerformPostUpgradeMigrations(dbx dogeboxd.Dogeboxd) bool {
	_, queued, err := migrations.RunPostUpgradeMigrations(migrations.Context{
		Config:  t.config,
		Enqueue: dbx.AddAction,
	})
	if err != nil {
		log.Printf("Post-upgrade migration runner failed: %v", err)
		return false
	}

	return queued
}
