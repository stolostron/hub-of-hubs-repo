package repo

import (
	"fmt"
	"net/http"
	"sync"
	"time"
)

type RepoServer struct {
	sync.Mutex
	Index  []byte
	Config *RepoConfig
	Server *http.Server
	Router *http.ServeMux
}

// NewRepoServer returns a new http Server that serve the helm chart repository
func NewRepoServer(repoConfig *RepoConfig) (*RepoServer, error) {
	var err error

	repoServer := &RepoServer{
		Config: repoConfig,
	}

	// create packages from charts
	err = repoServer.PackageCharts()
	if err != nil {
		return nil, err
	}

	// create index file in memory
	err = repoServer.CreateIndex()
	if err != nil {
		return nil, err
	}

	err = repoServer.SetupRouter()
	if err != nil {
		return nil, err
	}

	repoServer.Server = &http.Server{
		Addr: fmt.Sprintf(":%d", repoServer.Config.Port),
		// Good practice to set timeouts to avoid Slowloris attacks.
		WriteTimeout: time.Second * 30,
		ReadTimeout:  time.Second * 30,
		IdleTimeout:  time.Second * 30,
		Handler:      repoServer.Router,
	}

	return repoServer, nil
}
