package version

type VersionResult struct {
	Version string `json:"version"`
}

func (r *VersionResult) GetOutput() string {
	return r.Version
}
