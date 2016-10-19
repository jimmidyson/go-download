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

package download

import (
	"crypto"
	"crypto/md5" // #nosec
	"crypto/sha1"
	"crypto/sha256"
	"crypto/sha512"
	"fmt"
	"hash"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"time"

	pb "gopkg.in/cheggaaa/pb.v1"

	"github.com/pkg/errors"
)

// Options holds the possible configuration options for the Downloader.
type Options struct {
	// HTTPClient is an optional client to perform downloads with. If nil, `http.DefaultClient`
	// will be used.
	HTTPClient *http.Client
	// Checksum is either a checksum string, or a URL or path to a file containing the checksum. The file
	// can either contain the checksum only or contain multiple lines of the format:
	// CHECKSUM FILENAME
	Checksum string
	// Checksum hash is the hash for the checksum. Currently only supports SHA1, SHA256, SHA512 and MD5.
	// If unspecified, defaults to SHA256.
	ChecksumHash crypto.Hash
	// ProgressBars is the configuration of progress bars output. Set to `nil` (default) to disable.
	ProgressBars *ProgressBarOptions
}

// FileOptions holds the possible configuration options to download to a file.
type FileOptions struct {
	// Options is the common set of downloader options.
	Options
	// Mkdirs is the option to create parent directories of target directory if they don't
	// exist. Use `download.MkdirAll` or `download.MkdirNone` (or any `*bool`). Defaults to
	// `download.MkdirAll`.
	Mkdirs Mkdirs
}

// ProgressBarOptions holds the configuration for progress bars if required.
type ProgressBarOptions struct {
	// Writer holds where to output the progress bars to. Defaults to `os.Stdout`.
	Writer io.Writer
	// Width is the maximum width of the progress bar. If output to a narrower terminal then this
	// will be ignored.
	MaxWidth int
}

func newBool(b bool) *bool {
	return &b
}

// Mkdirs is a custom type so we can differentiate between not specified (nil)
// and set.
type Mkdirs *bool

var (
	// MkdirAll is used to create all intermediate directories if required.
	MkdirAll = Mkdirs(newBool(true))
	// MkdirNone is used to create no intermediate directories.
	MkdirNone = Mkdirs(newBool(false))
)

// ToFile downloads the specified `src` URL to `dest` file using
// the specified `FileOptions`.
func ToFile(src, dest string, options FileOptions) error {
	u, err := url.Parse(src)
	if err != nil {
		return errors.Wrap(err, "invalid src URL")
	}

	targetDir := filepath.Dir(dest)
	if _, err = os.Stat(targetDir); err != nil {
		if !os.IsNotExist(err) || (options.Mkdirs != nil && !*options.Mkdirs) {
			return errors.Wrap(err, "failed to check destination directory")
		}
		err = os.MkdirAll(targetDir, 0700)
		if err != nil {
			return errors.Wrap(err, "failed to create destination directory")
		}
	}

	targetName := filepath.Base(dest)
	f, err := ioutil.TempFile(targetDir, ".tmp-"+targetName)
	if err != nil {
		return errors.Wrap(err, "failed to create temp file")
	}

	err = downloadFile(u, f, options.Options)
	if err != nil {
		return errors.Wrap(err, "failed to download")
	}

	err = os.Rename(f.Name(), dest)
	if err != nil {
		_ = os.Remove(f.Name()) // #nosec
		return errors.Wrap(err, "failed to rename temp file to destination")
	}

	return nil
}

func downloadFile(u *url.URL, f *os.File, options Options) error {
	err := FromURL(u, f, options)
	if err != nil {
		_ = os.Remove(f.Name()) // #nosec
		return errors.Wrap(err, "failed to download to temp file")
	}
	err = f.Close()
	if err != nil {
		_ = os.Remove(f.Name()) // #nosec
		return errors.Wrap(err, "failed to close temp file")
	}

	return nil
}

// ToWriter downloads the specified `src` URL to `w` writer using
// the specified `Options`.
func ToWriter(src string, w io.Writer, options Options) error {
	u, err := url.Parse(src)
	if err != nil {
		return errors.Wrap(err, "invalid src URL")
	}
	return FromURL(u, w, options)
}

// FromURL downloads the specified `src` URL to `w` writer using
// the specified `Options`.
func FromURL(src *url.URL, w io.Writer, options Options) error {
	httpClient := getHTTPClient(options)
	var err error
	resp, err := httpClient.Get(src.String())
	if err != nil {
		return errors.Wrap(err, "download failed")
	}
	defer func() { _ = resp.Body.Close() }() // #nosec

	if resp.StatusCode != http.StatusOK {
		return errors.Errorf("received invalid status code: %d (expected %d)", resp.StatusCode, http.StatusOK)
	}

	var (
		validator checksumValidator

		reader io.Reader = resp.Body
	)

	if options.ProgressBars != nil && resp.ContentLength > 0 {
		bar := newProgressBar(resp.ContentLength, options.ProgressBars.MaxWidth, options.ProgressBars.Writer)
		bar.Start()
		reader = bar.NewProxyReader(reader)
		defer func() {
			<-time.After(bar.RefreshRate)
			fmt.Println()
		}()
	}

	if len(options.Checksum) != 0 {
		validator, err = createValidator(options.ChecksumHash, httpClient, options.Checksum, path.Base(src.Path))
		if err != nil {
			return errors.Wrap(err, "failed to create validator")
		}
		reader = io.TeeReader(reader, validator)
	}

	if _, err = io.Copy(w, reader); err != nil {
		return errors.Wrap(err, "failed to copy contents")
	}

	if validator != nil && !validator.validate() {
		return errors.New("checksum validation failed")
	}

	return nil
}

func createValidator(hashType crypto.Hash, httpClient *http.Client, checksum, filename string) (checksumValidator, error) {
	var hasher hash.Hash
	switch hashType {
	case crypto.SHA256, 0:
		hasher = sha256.New()
	case crypto.SHA1:
		hasher = sha1.New()
	case crypto.SHA512:
		hasher = sha512.New()
	case crypto.MD5:
		hasher = md5.New() // #nosec
	default:
		return nil, errors.New("invalid hash function")
	}

	validator, err := newValidator(hasher, httpClient, checksum, filename)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create validator")
	}

	return validator, nil
}

func getHTTPClient(options Options) *http.Client {
	httpClient := options.HTTPClient
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	return httpClient
}

func getBarWriter(w io.Writer) io.Writer {
	if w == nil {
		w = os.Stdout
	}
	return w
}

func newProgressBar(length int64, maxWidth int, w io.Writer) *pb.ProgressBar {
	bar := pb.New64(length).SetUnits(pb.U_BYTES)
	if maxWidth > 0 {
		bar.SetMaxWidth(maxWidth)
	}
	barWriter := getBarWriter(w)
	bar.Output = barWriter
	return bar
}
