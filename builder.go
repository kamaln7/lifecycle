package lifecycle

import (
	"io"
	"path/filepath"
	"sort"

	"github.com/buildpacks/lifecycle/buildpack"

	"github.com/buildpacks/lifecycle/api"
	"github.com/buildpacks/lifecycle/env"
	"github.com/buildpacks/lifecycle/launch"
	"github.com/buildpacks/lifecycle/layers"
)

type BuildEnv interface {
	AddRootDir(baseDir string) error
	AddEnvDir(envDir string, defaultAction env.ActionType) error
	WithPlatform(platformDir string) ([]string, error)
	List() []string
}

type BuildpackStore interface {
	Lookup(bpID, bpVersion string) (buildpack.Buildpack, error)
}

type Buildpack interface {
	Build(bpPlan buildpack.BuildpackPlan, config buildpack.BuildConfig) (buildpack.BuildResult, error)
}

type Builder struct {
	AppDir         string
	LayersDir      string
	PlatformDir    string
	PlatformAPI    *api.Version
	Env            BuildEnv
	Group          buildpack.BuildpackGroup
	Plan           buildpack.BuildPlan
	Out, Err       io.Writer
	BuildpackStore BuildpackStore
}

func (b *Builder) Build() (*BuildMetadata, error) {
	config, err := b.BuildConfig()
	if err != nil {
		return nil, err
	}

	procMap := processMap{}
	plan := b.Plan
	var bom []buildpack.BOMEntry
	var slices []layers.Slice
	var labels []buildpack.Label

	for _, bp := range b.Group.Group {
		bpTOML, err := b.BuildpackStore.Lookup(bp.ID, bp.Version)
		if err != nil {
			return nil, err
		}

		bpPlan := plan.Find(bp.ID)
		br, err := bpTOML.Build(bpPlan, config)
		if err != nil {
			return nil, err
		}

		bom = append(bom, br.BOM...)
		labels = append(labels, br.Labels...)
		plan = plan.Filter(br.MetRequires)
		procMap.add(br.Processes)
		slices = append(slices, br.Slices...)
	}

	if b.PlatformAPI.Compare(api.MustParse("0.4")) < 0 { // PlatformAPI <= 0.3
		for i := range bom {
			bom[i].ConvertMetadataToVersion()
		}
	}

	return &BuildMetadata{
		BOM:        bom,
		Buildpacks: b.Group.Group,
		Labels:     labels,
		Processes:  procMap.list(),
		Slices:     slices,
	}, nil
}

func (b *Builder) BuildConfig() (buildpack.BuildConfig, error) {
	appDir, err := filepath.Abs(b.AppDir)
	if err != nil {
		return buildpack.BuildConfig{}, err
	}
	platformDir, err := filepath.Abs(b.PlatformDir)
	if err != nil {
		return buildpack.BuildConfig{}, err
	}
	layersDir, err := filepath.Abs(b.LayersDir)
	if err != nil {
		return buildpack.BuildConfig{}, err
	}

	return buildpack.BuildConfig{
		Env:         b.Env,
		AppDir:      appDir,
		PlatformDir: platformDir,
		LayersDir:   layersDir,
		Out:         b.Out,
		Err:         b.Err,
	}, nil
}

type processMap map[string]launch.Process

func (m processMap) add(l []launch.Process) {
	for _, proc := range l {
		m[proc.Type] = proc
	}
}

func (m processMap) list() []launch.Process {
	var keys []string
	for key := range m {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	procs := []launch.Process{}
	for _, key := range keys {
		procs = append(procs, m[key])
	}
	return procs
}
