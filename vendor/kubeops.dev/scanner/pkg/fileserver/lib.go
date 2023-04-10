/*
Copyright AppsCode Inc. and Contributors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package fileserver

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"

	"kubeops.dev/scanner/apis/trivy"

	"github.com/dustin/go-humanize"
	_ "github.com/dustin/go-humanize"
	"github.com/go-chi/chi/v5"
	hw "go.wandrs.dev/http"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
)

func Router(prefix, dir string) http.Handler {
	_ = os.MkdirAll(dir, 0o755)
	fileServer := http.FileServer(http.Dir(dir))

	r := chi.NewRouter()

	pattern := path.Join(prefix, "*")
	r.Options(pattern, http.StripPrefix(prefix, fileServer).ServeHTTP)
	r.Get(pattern, http.StripPrefix(prefix, fileServer).ServeHTTP)
	r.Post(pattern, func(w http.ResponseWriter, r *http.Request) {
		err := FileSave(prefix, dir, r)

		status := hw.ErrorToAPIStatus(err)
		code := int(status.Code)
		// when writing an error, check to see if the status indicates a retry after period
		if status.Details != nil && status.Details.RetryAfterSeconds > 0 {
			delay := strconv.Itoa(int(status.Details.RetryAfterSeconds))
			w.Header().Set("Retry-After", delay)
		}
		if code == http.StatusNoContent {
			w.WriteHeader(code)
			return
		}
		data, _ := json.MarshalIndent(status, "", "  ")
		_, _ = w.Write(data)
	})

	return r
}

const MaxUploadSize = 100 << 30 // 1 GB

// FileSave fetches the file and saves to disk
func FileSave(prefix, dir string, r *http.Request) error {
	// left shift 100 << 20 which results in 32*2^20 = 33554432
	// x << y, results in x*2^y
	// 1 MB max memory
	err := r.ParseMultipartForm(1 << 20)
	if err != nil {
		return err
	}
	// Retrieve the file from form data
	f, h, err := r.FormFile("file")
	if err != nil {
		return err
	}
	defer f.Close()

	size, err := getSize(f)
	if err != nil {
		//// logger.WithError(err).Error("failed to get the size of the uploaded content")
		//w.WriteHeader(http.StatusInternalServerError)
		//writeError(w, err)
		return err
	}
	if size > MaxUploadSize {
		// logger.WithField("size", size).Info("file size exceeded")
		// w.WriteHeader(http.StatusRequestEntityTooLarge)
		// writeError(w, errors.New("uploaded file size exceeds the limit"))
		return apierrors.NewRequestEntityTooLargeError(fmt.Sprintf("received %s, limit %s", humanize.Bytes(uint64(size)), humanize.Bytes(MaxUploadSize)))
	}

	filename := h.Filename
	if filename == "" {
		return errors.New("missing file name")
	}

	fullPath := filepath.Join(dir, strings.TrimPrefix(r.URL.Path, prefix), filename)
	_ = os.MkdirAll(filepath.Dir(fullPath), 0o755)
	file, err := os.OpenFile(fullPath, os.O_WRONLY|os.O_CREATE, 0o644)
	if err != nil {
		return err
	}
	defer file.Close()
	// Copy the file to the destination path
	_, err = io.Copy(file, f)
	if err != nil {
		return err
	}
	return nil
}

func getSize(content io.Seeker) (int64, error) {
	size, err := content.Seek(0, io.SeekEnd)
	if err != nil {
		return 0, err
	}
	_, err = content.Seek(0, io.SeekStart)
	if err != nil {
		return 0, err
	}
	return size, nil
}

func VulnerabilityDBLastUpdatedAt(fsDir string) (*trivy.Time, error) {
	dir := filepath.Join(fsDir, "trivy")
	fsdata, err := fs.ReadFile(os.DirFS(dir), "metadata.json")
	if err != nil {
		return nil, err
	}

	var ver trivy.VulnerabilityDBStruct
	err = json.Unmarshal(fsdata, &ver)
	if err != nil {
		return nil, err
	}
	return &ver.UpdatedAt, nil
}
