package repo

import (
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"path"
	"path/filepath"
	"time"

	"github.com/Masterminds/semver/v3"
	"helm.sh/helm/v3/pkg/chart/loader"
	"helm.sh/helm/v3/pkg/chartutil"
	"helm.sh/helm/v3/pkg/repo"
	"sigs.k8s.io/yaml"
)

// SetupRouter creates router with modified index
func (s *RepoServer) SetupRouter() error {
	mux := http.NewServeMux()

	// Add route handlers
	fileServer := http.FileServer(http.Dir(s.Config.RepoDir))
	mux.Handle("/liveness", http.HandlerFunc(s.livenessHandler))
	mux.Handle("/readiness", http.HandlerFunc(s.readinessHandler))
	mux.Handle("/charts/index.yaml", loggingMiddleware(http.HandlerFunc(s.indexHandler)))
	mux.Handle("/charts/", loggingMiddleware(http.StripPrefix("/charts/", fileServer)))

	s.Router = mux
	return nil
}

// StatusWriter adds a field an http.ResponseWriter to track status
type StatusWriter struct {
	http.ResponseWriter
	status int
}

// WriteHeader populates the status field before calling WriteHeader
func (w *StatusWriter) WriteHeader(status int) {
	w.status = status // Store the status for our own use
	w.ResponseWriter.WriteHeader(status)
}

// loggingMiddleware logs each request sent to the server
func loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Use custom ResponseWriter to track statuscode
		crw := &StatusWriter{ResponseWriter: w}

		startTime := time.Now()
		next.ServeHTTP(crw, r)
		duration := time.Since(startTime)

		log.Printf("%d %3dms %s", crw.status, duration.Milliseconds(), r.RequestURI)
	})
}

// CreateIndex creates an index from a flat directory
func (s *RepoServer) CreateIndex() error {
	url := fmt.Sprintf("https://%s/charts", s.Config.Host)
	index, err := repo.IndexDirectory(filepath.Clean(s.Config.RepoDir), url)
	if err != nil {
		return err
	}

	indexBytes, err := yaml.Marshal(index)
	if err != nil {
		return err
	}

	s.Index = indexBytes

	return nil
}

// PackageCharts package all the helm charts from chart directory
func (s *RepoServer) PackageCharts() error {
	if _, err := semver.NewVersion(s.Config.Version); err != nil {
		return err
	}

	files, err := ioutil.ReadDir(s.Config.ChartDir)
	if err != nil {
		return err
	}

	for _, f := range files {
		if f.IsDir() {
			if _, err := packageChart(path.Join(s.Config.ChartDir, f.Name()), s.Config.RepoDir, s.Config.Version); err != nil {
				return fmt.Errorf("failed to package directory %s: %w", f.Name(), err)
			}
		}
	}

	return nil
}

// packageChart packages a sgingle helm chart from source directory
func packageChart(src string, dst string, chartVersion string) (string, error) {
	ch, err := loader.LoadDir(src)
	if err != nil {
		return "", fmt.Errorf("failed to load: %w", err)
	}

	ch.Metadata.Version = chartVersion
	name, err := chartutil.Save(ch, dst)
	if err != nil {
		return "", fmt.Errorf("failed to save: %w", err)
	}

	log.Printf("Packaged chart as %s", name)

	return name, nil
}

// indexHandler serves the index.yaml file from in memory
func (s *RepoServer) indexHandler(w http.ResponseWriter, r *http.Request) {
	s.Lock()
	defer s.Unlock()
	if _, err := w.Write(s.Index); err != nil {
		log.Println(err)
	}
}

// livenessHandler returns a 200 status as long as the server is running
func (s *RepoServer) livenessHandler(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
}

// readinessHandler returns a 200 status as long as the server is running
func (s *RepoServer) readinessHandler(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
}
