package buildpack

import (
	"path/filepath"

	"github.com/BurntSushi/toml"

	"github.com/buildpacks/lifecycle/launch"
)

type BuildpackTOML struct {
	API       string         `toml:"api"`
	Buildpack BuildpackInfo  `toml:"buildpack"`
	Order     BuildpackOrder `toml:"order"`
	Dir       string         `toml:"-"`
}

func (b *BuildpackTOML) String() string {
	return b.Buildpack.Name + " " + b.Buildpack.Version
}

type BuildpackInfo struct {
	ClearEnv bool   `toml:"clear-env,omitempty"`
	Homepage string `toml:"homepage,omitempty"`
	ID       string `toml:"id"`
	Name     string `toml:"name"`
	Version  string `toml:"version"`
}

type BuildpackOrder []BuildpackGroup

type BuildpackGroup struct {
	Group []GroupBuildpack `toml:"group"`
}

type GroupBuildpack struct {
	API      string `toml:"api,omitempty" json:"-"` // TODO: why is this repeated from BuildpackTOML?
	Homepage string `toml:"homepage,omitempty" json:"homepage,omitempty"`
	ID       string `toml:"id" json:"id"`
	// TODO: where is Name?
	Optional bool   `toml:"optional,omitempty" json:"optional,omitempty"`
	Version  string `toml:"version" json:"version"`
}

func (bp GroupBuildpack) String() string {
	return bp.ID + "@" + bp.Version
}

func (bp GroupBuildpack) NoOpt() GroupBuildpack {
	bp.Optional = false
	return bp
}

func (bp GroupBuildpack) NoAPI() GroupBuildpack {
	bp.API = ""
	return bp
}

func (bp GroupBuildpack) NoHomepage() GroupBuildpack {
	bp.Homepage = ""
	return bp
}

// TODO: this logic is duplicated in buildpack store
func (bp GroupBuildpack) Lookup(buildpacksDir string) (*BuildpackTOML, error) {
	bpTOML := BuildpackTOML{}
	bpPath, err := filepath.Abs(filepath.Join(buildpacksDir, launch.EscapeID(bp.ID), bp.Version))
	if err != nil {
		return nil, err
	}
	tomlPath := filepath.Join(bpPath, "buildpack.toml")
	if _, err := toml.DecodeFile(tomlPath, &bpTOML); err != nil {
		return nil, err
	}
	bpTOML.Dir = bpPath
	return &bpTOML, nil
}
