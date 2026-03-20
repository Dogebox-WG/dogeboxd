package web

import (
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	dogeboxd "github.com/Dogebox-WG/dogeboxd/pkg"
)

type stubSourceManager struct {
	addSource        func(location string) (dogeboxd.ManifestSource, error)
	addSourcePending func(location string) error
}

type stubNetworkManager struct {
	hasInternetConnectivity func() bool
	getLocalIP              func() (net.IP, error)
}

func (s stubSourceManager) GetAll(bool) (map[string]dogeboxd.ManifestSourceList, error) {
	panic("unexpected call to GetAll")
}

func (s stubSourceManager) GetSourceManifest(string, string, string) (dogeboxd.PupManifest, dogeboxd.ManifestSource, error) {
	panic("unexpected call to GetSourceManifest")
}

func (s stubSourceManager) GetSourcePup(string, string, string) (dogeboxd.ManifestSourcePup, error) {
	panic("unexpected call to GetSourcePup")
}

func (s stubSourceManager) GetSource(string) (dogeboxd.ManifestSource, error) {
	panic("unexpected call to GetSource")
}

func (s stubSourceManager) AddSource(location string) (dogeboxd.ManifestSource, error) {
	if s.addSource == nil {
		panic("unexpected call to AddSource")
	}
	return s.addSource(location)
}

func (s stubSourceManager) AddSourcePending(location string) error {
	if s.addSourcePending == nil {
		panic("unexpected call to AddSourcePending")
	}
	return s.addSourcePending(location)
}

func (s stubSourceManager) RetryPendingSources() (int, error) {
	panic("unexpected call to RetryPendingSources")
}

func (s stubSourceManager) RemoveSource(string) error {
	panic("unexpected call to RemoveSource")
}

func (s stubSourceManager) DownloadPup(string, string, string, string) (dogeboxd.PupManifest, error) {
	panic("unexpected call to DownloadPup")
}

func (s stubSourceManager) GetAllSourceConfigurations() []dogeboxd.ManifestSourceConfiguration {
	panic("unexpected call to GetAllSourceConfigurations")
}

func (s stubNetworkManager) GetAvailableNetworks() []dogeboxd.NetworkConnection {
	panic("unexpected call to GetAvailableNetworks")
}

func (s stubNetworkManager) SetPendingNetwork(dogeboxd.SelectedNetwork, dogeboxd.Job) error {
	panic("unexpected call to SetPendingNetwork")
}

func (s stubNetworkManager) TryConnect(dogeboxd.NixPatch) error {
	panic("unexpected call to TryConnect")
}

func (s stubNetworkManager) TestConnect() error {
	panic("unexpected call to TestConnect")
}

func (s stubNetworkManager) HasInternetConnectivity() bool {
	if s.hasInternetConnectivity == nil {
		panic("unexpected call to HasInternetConnectivity")
	}
	return s.hasInternetConnectivity()
}

func (s stubNetworkManager) GetLocalIP() (net.IP, error) {
	if s.getLocalIP == nil {
		panic("unexpected call to GetLocalIP")
	}
	return s.getLocalIP()
}

func TestCreateSourceFallsBackToPendingWhenOffline(t *testing.T) {
	location := "https://github.com/elusiveshiba/test-pup.git"
	addPendingCalls := 0

	handler := api{
		dbx: dogeboxd.Dogeboxd{
			NetworkManager: stubNetworkManager{
				hasInternetConnectivity: func() bool {
					return false
				},
			},
		},
		sources: stubSourceManager{
			addSource: func(gotLocation string) (dogeboxd.ManifestSource, error) {
				if gotLocation != location {
					t.Fatalf("AddSource location = %q, want %q", gotLocation, location)
				}

				return nil, fmt.Errorf("repository not found")
			},
			addSourcePending: func(gotLocation string) error {
				addPendingCalls++
				if gotLocation != location {
					t.Fatalf("AddSourcePending location = %q, want %q", gotLocation, location)
				}
				return nil
			},
		},
	}

	req := httptest.NewRequest(http.MethodPut, "/source", strings.NewReader(`{"location":"`+location+`"}`))
	rec := httptest.NewRecorder()

	handler.createSource(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	if addPendingCalls != 1 {
		t.Fatalf("AddSourcePending calls = %d, want 1", addPendingCalls)
	}
	if !strings.Contains(rec.Body.String(), `"pending":true`) {
		t.Fatalf("expected pending success response, got %s", rec.Body.String())
	}
}

func TestCreateSourceReturnsErrorWhenOnline(t *testing.T) {
	location := "https://github.com/elusiveshiba/test-pup.git"
	addPendingCalls := 0

	handler := api{
		dbx: dogeboxd.Dogeboxd{
			NetworkManager: stubNetworkManager{
				hasInternetConnectivity: func() bool {
					return true
				},
			},
		},
		sources: stubSourceManager{
			addSource: func(string) (dogeboxd.ManifestSource, error) {
				return nil, fmt.Errorf("no valid semver tags found")
			},
			addSourcePending: func(string) error {
				addPendingCalls++
				return nil
			},
		},
	}

	req := httptest.NewRequest(http.MethodPut, "/source", strings.NewReader(`{"location":"`+location+`"}`))
	rec := httptest.NewRecorder()

	handler.createSource(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusInternalServerError, rec.Body.String())
	}
	if addPendingCalls != 0 {
		t.Fatalf("AddSourcePending calls = %d, want 0", addPendingCalls)
	}
}

func TestCreateSourceReturnsErrorForNonGitSourcesEvenWhenOffline(t *testing.T) {
	location := "/does/not/exist"
	addPendingCalls := 0

	handler := api{
		dbx: dogeboxd.Dogeboxd{
			NetworkManager: stubNetworkManager{
				hasInternetConnectivity: func() bool {
					return false
				},
			},
		},
		sources: stubSourceManager{
			addSource: func(string) (dogeboxd.ManifestSource, error) {
				return nil, fmt.Errorf("location looks like disk path, but path %s does not exist", location)
			},
			addSourcePending: func(string) error {
				addPendingCalls++
				return nil
			},
		},
	}

	req := httptest.NewRequest(http.MethodPut, "/source", strings.NewReader(`{"location":"`+location+`"}`))
	rec := httptest.NewRecorder()

	handler.createSource(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusInternalServerError, rec.Body.String())
	}
	if addPendingCalls != 0 {
		t.Fatalf("AddSourcePending calls = %d, want 0", addPendingCalls)
	}
}
