// Copyright 2024 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      https://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// Command validate_readme_canonicalocspec validates Canonical OCs listed by MarkDown
// (READMEs) against the most recent repository states in
// github.com/openconfig/featureprofiles.
package main

import (
	goflag "flag"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	log "github.com/golang/glog"
	"github.com/openconfig/featureprofiles/tools/internal/canonicalocspec"
	"github.com/openconfig/featureprofiles/tools/internal/fpciutil"
	"github.com/openconfig/ygot/ygot"
	flag "github.com/spf13/pflag"
	"golang.org/x/exp/maps"
)

// Config is the set of flags for this binary.
type Config struct {
	FeatureDir     string
	NonTestREADMEs stringMap
}

func newConfig() *Config {
	return &Config{
		NonTestREADMEs: map[string]struct{}{},
	}
}

type stringMap map[string]struct{}

func (m stringMap) String() string {
	return strings.Join(maps.Keys(m), ",")
}

func (m stringMap) Type() string {
	return "stringMap"
}

func (m stringMap) Set(readmePath string) error {
	m[readmePath] = struct{}{}
	return nil
}

// New registers a flagset with the configuration needed by this binary.
func New(fs *flag.FlagSet) *Config {
	c := newConfig()

	if fs == nil {
		fs = flag.CommandLine
	}
	fs.StringVar(&c.FeatureDir, "feature-dir", "", "path to the feature directory of featureprofiles, for which all README.md files are validated for their coverage spec")
	fs.Var(&c.NonTestREADMEs, "non-test-readme", "README that's exempt from coverage spec validation (can be specified multiple times)")

	return c
}

var (
	config *Config
)

func init() {
	config = New(nil)
}

func readmeFiles(featureDir string) ([]string, error) {
	var files []string
	err := filepath.WalkDir(featureDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.Name() != fpciutil.READMEname {
			return nil
		}

		files = append(files, path)
		return nil
	})
	return files, err
}

func isValidOC(canonicalOC ygot.GoStruct) error {
	return ygot.ValidateGoStruct(canonicalOC)
}

func main() {
	flag.CommandLine.AddGoFlagSet(goflag.CommandLine) // for compatibility with glog
	flag.Parse()

	fileCount := flag.NArg()
	var files []string
	switch {
	case fileCount != 0 && config.FeatureDir != "":
		log.Exit("If -feature-dir flag is specified, README files must not be specified as positional arguments.")
	case fileCount != 0:
		files = flag.Args()
	case config.FeatureDir == "":
		var err error
		config.FeatureDir, err = fpciutil.FeatureDir()
		if err != nil {
			log.Exitf("Unable to locate feature root: %v", err)
		}
		fallthrough
	case config.FeatureDir != "":
		var err error
		files, err = readmeFiles(config.FeatureDir)
		if err != nil {
			log.Exitf("Error gathering README.md files for validation: %v", err)
		}
	default:
		log.Exit("Program internal error: input not handled.")
	}

	erredFiles := map[string]struct{}{}
	for _, file := range files {
		if _, ok := config.NonTestREADMEs[file]; ok {
			// Allowlist
			continue
		}

		log.Infof("Validating %q", file)
		b, err := os.ReadFile(file)
		if err != nil {
			log.Exitf("Error reading file: %q", file)
		}
		ocStructs, err := canonicalocspec.Parse(b)
		if err != nil {
			log.Errorf("file %v: %v", file, err)
			erredFiles[file] = struct{}{}
			continue
		}

		errored := false
		for _, ocStruct := range ocStructs {
			if err := isValidOC(ocStruct); err != nil {
				log.Errorf("%q contains invalid Canonical OCs: %v", file, err)
				errored = true
			}
		}
		if errored {
			erredFiles[file] = struct{}{}
		} else {
			log.Infof("%q contains %d valid Canonical OCs\n", file, len(ocStructs))
		}
	}
	if len(erredFiles) > 0 {
		log.Exitf("The following files have errors:\n%v", strings.Join(maps.Keys(erredFiles), "\n"))
	}
}
