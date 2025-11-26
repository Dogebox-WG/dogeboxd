package web

import (
	"context"
	"encoding/gob"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"

	dogeboxd "github.com/dogeorg/dogeboxd/pkg"
	"github.com/dogeorg/dogeboxd/pkg/conductor"
	"github.com/rs/cors"
)

func RESTAPI(
	config dogeboxd.ServerConfig,
	sm dogeboxd.StateManager,
	dbx dogeboxd.Dogeboxd,
	pups dogeboxd.PupManager,
	sources dogeboxd.SourceManager,
	lifecycle dogeboxd.LifecycleManager,
	nix dogeboxd.NixManager,
	dkm dogeboxd.DKMManager,
	ws WSRelay,
) conductor.Service {
	sessions = []Session{}

	if config.DevMode {
		log.Println("In development mode: Loading REST API sessions..")
		file, err := os.Open(fmt.Sprintf("%s/dev-sessions.gob", config.DataDir))
		if err == nil {
			decoder := gob.NewDecoder(file)
			err = decoder.Decode(&sessions)
			if err != nil {
				log.Printf("Failed to decode sessions from dev-sessions.gob: %v", err)
			}
			file.Close()
			log.Printf("Loaded %d sessions from dev-sessions.gob", len(sessions))
		} else {
			log.Printf("Failed to open dev-sessions.gob: %v", err)
		}
	}

	a := api{
		mux:       http.NewServeMux(),
		config:    config,
		sm:        sm,
		dbx:       dbx,
		pups:      pups,
		ws:        ws,
		dkm:       dkm,
		lifecycle: lifecycle,
		nix:       nix,
		sources:   sources,
	}

	routes := map[string]http.HandlerFunc{}

	// Recovery routes are the _only_ routes loaded in recovery mode.
	recoveryRoutes := map[string]http.HandlerFunc{
		"POST /authenticate":    a.authenticate,
		"POST /logout":          a.logout,
		"POST /change-password": a.changePassword,

		"GET /system/bootstrap":          a.getBootstrap,
		"GET /system/recovery-bootstrap": a.getRecoveryBootstrap,
		"GET /system/keymap":             a.getKeymap,
		"GET /system/keymaps":            a.getKeymaps,
		"POST /system/keymap":            a.setKeyMap,
		"GET /system/timezone":           a.getTimezone,
		"GET /system/timezones":          a.getTimezones,
		"POST /system/timezone":          a.setTimezone,
		"GET /system/disks":              a.getInstallDisks,
		"POST /system/hostname":          a.setHostname,
		"POST /system/storage":           a.setStorageDevice,
		"POST /system/install":           a.installToDisk,

		"GET /system/network/list":        a.getNetwork,
		"PUT /system/network/set-pending": a.setPendingNetwork,
		"POST /system/network/test":       a.testConnectNetwork,
		"POST /system/network/connect":    a.connectNetwork,
		"POST /system/host/shutdown":      a.hostShutdown,
		"POST /system/host/reboot":        a.hostReboot,
		"POST /keys/create-master":        a.createMasterKey,
		"GET /keys":                       a.listKeys,
		"POST /system/bootstrap":          a.initialBootstrap,

		"GET /system/ssh/state":               a.getSSHState,
		"PUT /system/ssh/state":               a.setSSHState,
		"GET /system/ssh/keys":                a.listSSHKeys,
		"PUT /system/ssh/key":                 a.addSSHKey,
		"DELETE /system/ssh/key/{id}":         a.removeSSHKey,
		"POST /system/import-blockchain-data": a.importBlockchainData,
		"/ws/state/":                          a.getUpdateSocket,
		"/ws/jobs":                            a.getJobsSocket,
	}

	// Normal routes are used when we are not in recovery mode.
	// nb. These are used in _addition_ to recovery routes.
	normalRoutes := map[string]http.HandlerFunc{
		"GET /pup/{ID}/metrics":               a.getPupMetrics,
		"POST /pup/{ID}/{action}":             a.pupAction,
		"PUT /pup":                            a.installPup,
		"PUT /pups":                           a.installPups,
		"POST /config/{PupID}":                a.updateConfig,
		"POST /providers/{PupID}":             a.updateProviders,
		"GET /providers/{PupID}":              a.getPupProviders,
		"POST /hooks/{PupID}":                 a.updateHooks,
		"GET /sources":                        a.getSources,
		"PUT /source":                         a.createSource,
		"GET /sources/store":                  a.getStoreList,
		"DELETE /source/{id}":                 a.deleteSource,
		"/ws/log/pup/{PupID}":                 a.getPupLogSocket,
		"/ws/log/job/{JobID}":                 a.getJobLogSocket,
		"POST /system/welcome-complete":       a.setWelcomeComplete,
		"POST /system/install-pup-collection": a.installPupCollection,
		"GET /missing-deps/{PupID}":           a.getMissingDeps,

		"GET /system/binary-caches":        a.getBinaryCaches,
		"PUT /system/binary-cache":         a.addBinaryCache,
		"DELETE /system/binary-cache/{id}": a.removeBinaryCache,

		"GET /system/updates": a.checkForUpdates,
		"POST /system/update": a.commenceUpdate,

		"GET /jobs":                  a.getJobs,
		"GET /jobs/active":           a.getActiveJobs,
		"GET /jobs/recent":           a.getRecentJobs,
		"GET /jobs/stats":            a.getJobStats,
		"GET /jobs/{jobID}":          a.getJob,
		"POST /jobs/clear-completed": a.clearCompletedJobs,
		"POST /jobs/clear-all":       a.clearAllJobs,
	}

	// We always want to load recovery routes.
	for k, v := range recoveryRoutes {
		routes[k] = v
	}

	// If we're not in recovery mode, also load our normal routes.
	if !config.Recovery {
		for k, v := range normalRoutes {
			routes[k] = v
		}
		log.Printf("Loaded %d API routes", len(routes))
	} else {
		log.Printf("In recovery mode: Loading limited routes")
	}

	// If we have a Unix socket configured, create an unauthenticated mux for it
	var unixMux *http.ServeMux
	if config.UnixSocketPath != "" {
		unixMux = http.NewServeMux()
	}

	for p, h := range routes {
		a.mux.HandleFunc(p, authReq(dbx, sm, p, h))
		if unixMux != nil {
			unixMux.HandleFunc(p, h) // no auth on unix socket
		}
	}

	a.unixMux = unixMux

	return a
}

type api struct {
	dbx       dogeboxd.Dogeboxd
	sm        dogeboxd.StateManager
	dkm       dogeboxd.DKMManager
	mux       *http.ServeMux
	pups      dogeboxd.PupManager
	config    dogeboxd.ServerConfig
	sources   dogeboxd.SourceManager
	lifecycle dogeboxd.LifecycleManager
	nix       dogeboxd.NixManager
	ws        WSRelay
	unixMux   *http.ServeMux
}

func (t api) Run(started, stopped chan bool, stop chan context.Context) error {
	go func() {
		handler := cors.AllowAll().Handler(t.mux)
		srv := &http.Server{Addr: fmt.Sprintf("%s:%d", t.config.Bind, t.config.Port), Handler: handler}
		// Start TCP server
		go func() {
			if err := srv.ListenAndServe(); err != http.ErrServerClosed {
				log.Fatalf("HTTP server public ListenAndServe: %v", err)
			}
		}()

		// If unix socket enabled, start that server too
		if t.unixMux != nil {
			go func() {
				// Ensure socket does not already exist
				_ = os.Remove(t.config.UnixSocketPath)
				ln, err := net.Listen("unix", t.config.UnixSocketPath)
				if err != nil {
					log.Fatalf("HTTP unix listen: %v", err)
				}
				os.Chmod(t.config.UnixSocketPath, 0660)
				srvUnix := &http.Server{Handler: t.unixMux}
				if err := srvUnix.Serve(ln); err != http.ErrServerClosed {
					log.Fatalf("HTTP server unix Serve: %v", err)
				}
			}()
		}

		started <- true
		ctx := <-stop
		srv.Shutdown(ctx)
		stopped <- true
	}()
	return nil
}
