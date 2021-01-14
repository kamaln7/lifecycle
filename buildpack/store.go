package buildpack

import (
	"path/filepath"

	"github.com/BurntSushi/toml"

	"github.com/buildpacks/lifecycle/launch"
)

type Buildpack interface {
	Build(bpPlan BuildpackPlan, config BuildConfig) (BuildResult, error)
}

type DirBuildpackStore struct {
	Dir string
}

func (f *DirBuildpackStore) Lookup(bpID, bpVersion string) (Buildpack, error) {
	bpTOML := BuildpackTOML{}
	bpPath := filepath.Join(f.Dir, launch.EscapeID(bpID), bpVersion)
	tomlPath := filepath.Join(bpPath, "buildpack.toml")
	if _, err := toml.DecodeFile(tomlPath, &bpTOML); err != nil {
		return nil, err
	}
	bpTOML.Dir = bpPath
	return &bpTOML, nil
}
