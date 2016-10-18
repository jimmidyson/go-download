//    Copyright 2016 Red Hat, Inc.
//
//    Licensed under the Apache License, Version 2.0 (the "License");
//    you may not use this file except in compliance with the License.
//    You may obtain a copy of the License at
//
//        http://www.apache.org/licenses/LICENSE-2.0
//
//    Unless required by applicable law or agreed to in writing, software
//    distributed under the License is distributed on an "AS IS" BASIS,
//    WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
//    See the License for the specific language governing permissions and
//    limitations under the License.

package download_test

import (
	"bytes"
	"crypto"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	download "github.com/jimmidyson/go-download"
)

func TestDownloadToFileFailOnMkdirs(t *testing.T) {
	err := download.DownloadToFile("http://whatever:12345", "./non-existent-directory", download.FileDownloadOptions{Mkdirs: download.MkdirNone})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestDownloadToFileSuccess(t *testing.T) {
	srv := httptest.NewServer(http.FileServer(http.Dir("testdata")))
	defer srv.Close()

	tmpFile, err := ioutil.TempFile("", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer func() { _ = os.Remove(tmpFile.Name()) }()

	err = download.DownloadToFile(srv.URL+"/testfile", tmpFile.Name(), download.FileDownloadOptions{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	testData, err := ioutil.ReadFile("testdata/testfile")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	downloadedData, err := ioutil.ReadFile(tmpFile.Name())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !bytes.Equal(testData, downloadedData) {
		t.Fatal("wrong downloaded data")
	}
}

func TestDownloadToFileSuccessMkdirs(t *testing.T) {
	srv := httptest.NewServer(http.FileServer(http.Dir("testdata")))
	defer srv.Close()

	tmpDir, err := ioutil.TempDir("", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer func() { _ = os.Remove(tmpDir) }()
	_ = os.Remove(tmpDir)

	tmpFile := filepath.Join(tmpDir, "tmp")
	err = download.DownloadToFile(srv.URL+"/testfile", tmpFile, download.FileDownloadOptions{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	testData, err := ioutil.ReadFile("testdata/testfile")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	downloadedData, err := ioutil.ReadFile(tmpFile)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !bytes.Equal(testData, downloadedData) {
		t.Fatal("wrong downloaded data")
	}
}

func TestDownloadToFileSuccessMD5Checksum(t *testing.T) {
	srv := httptest.NewServer(http.FileServer(http.Dir("testdata")))
	defer srv.Close()

	tmpFile, err := ioutil.TempFile("", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer func() { _ = os.Remove(tmpFile.Name()) }()

	err = download.DownloadToFile(srv.URL+"/testfile", tmpFile.Name(), download.FileDownloadOptions{
		DownloadOptions: download.DownloadOptions{
			Checksum:     "d577273ff885c3f84dadb8578bb41399",
			ChecksumHash: crypto.MD5,
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	testData, err := ioutil.ReadFile("testdata/testfile")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	downloadedData, err := ioutil.ReadFile(tmpFile.Name())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !bytes.Equal(testData, downloadedData) {
		t.Fatal("wrong downloaded data")
	}
}

func TestDownloadToFileFailChecksum(t *testing.T) {
	srv := httptest.NewServer(http.FileServer(http.Dir("testdata")))
	defer srv.Close()

	tmpFile, err := ioutil.TempFile("", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer func() { _ = os.Remove(tmpFile.Name()) }()

	err = download.DownloadToFile(srv.URL+"/testfile", tmpFile.Name(), download.FileDownloadOptions{
		DownloadOptions: download.DownloadOptions{
			Checksum:     "d577273f",
			ChecksumHash: crypto.MD5,
		},
	})
	if err == nil {
		t.Fatalf("expected error")
	}
}

type checksum struct {
	checksumFile string
	hash         crypto.Hash
}

var checksumTests = []checksum{
	{"testfile.md5", crypto.MD5},
	{"CHECKSUMS.md5", crypto.MD5},
	{"testfile.sha1", crypto.SHA1},
	{"CHECKSUMS.sha1", crypto.SHA1},
	{"testfile.sha256", crypto.SHA256},
	{"CHECKSUMS.sha256", crypto.SHA256},
	{"testfile.sha512", crypto.SHA512},
	{"CHECKSUMS.sha512", crypto.SHA512},
}

func TestDownloadToFileWithChecksumValidation(t *testing.T) {
	srv := httptest.NewServer(http.FileServer(http.Dir("testdata")))
	defer srv.Close()

	for _, chk := range checksumTests {
		func() {
			tmpFile, err := ioutil.TempFile("", "")
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			defer func() { _ = os.Remove(tmpFile.Name()) }()

			err = download.DownloadToFile(srv.URL+"/testfile", tmpFile.Name(), download.FileDownloadOptions{
				DownloadOptions: download.DownloadOptions{
					Checksum:     srv.URL + "/" + chk.checksumFile,
					ChecksumHash: chk.hash,
				},
			})
			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}

			testData, err := ioutil.ReadFile("testdata/testfile")
			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}

			downloadedData, err := ioutil.ReadFile(tmpFile.Name())
			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}

			if !bytes.Equal(testData, downloadedData) {
				t.Error("wrong downloaded data")
			}
		}()
	}
}
