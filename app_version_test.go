package main

import (
	"encoding/json"
	"os"
	"testing"
)

// appVersion is stamped into every recorded event, and wails.json stamps the
// same version into the app bundle. Two hardcoded copies of one fact drift, and
// this project has already shipped that exact bug: the plist said 1.0.0 for
// every release ever made, because nothing compared it to anything.
//
// A recorded event carrying the wrong build is worse than one carrying none —
// it sends whoever is debugging to the wrong source.
func TestAppVersionMatchesWailsConfig(t *testing.T) {
	data, err := os.ReadFile("wails.json")
	if err != nil {
		t.Fatal(err)
	}
	var config struct {
		Info struct {
			ProductVersion string `json:"productVersion"`
		} `json:"info"`
	}
	if err := json.Unmarshal(data, &config); err != nil {
		t.Fatal(err)
	}
	if config.Info.ProductVersion == "" {
		t.Fatal("wails.json has no info.productVersion; the bundle would fall back to a hardcoded version")
	}
	if config.Info.ProductVersion != appVersion {
		t.Errorf("version drift: wails.json says %q, appVersion says %q. "+
			"The bundle and the event log would disagree about which build this is.",
			config.Info.ProductVersion, appVersion)
	}
}
