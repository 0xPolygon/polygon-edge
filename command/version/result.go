package version

import "fmt"

type VersionResult struct {
	Version   string `json:"version"`
	Commit    string `json:"commit"`
	Branch    string `json:"branch"`
	BuildTime string `json:"buildTime"`
}

func (r *VersionResult) GetOutput() string {
	return fmt.Sprintf("\n[VERSION INFO]\n"+
		"Release version: %s \n"+
		"Git branch: %s\n"+
		"Commit hash: %s\n"+
		"Build time: %s", r.Version, r.Branch, r.Commit, r.BuildTime)
}
