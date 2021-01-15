package lifecycle

import (
	"path/filepath"

	"github.com/buildpacks/imgutil"
	"github.com/pkg/errors"

	"github.com/buildpacks/lifecycle/launch"
)

type Analyzer struct {
	Buildpacks []GroupBuildpack
	LayersDir  string
	Logger     Logger
	SkipLayers bool
}

// Analyze restores metadata for launch and cache layers into the layers directory.
// If a usable cache is not provided, Analyze will not restore any cache=true layer metadata.
func (a *Analyzer) Analyze(image imgutil.Image, cache Cache) (AnalyzedMetadata, error) {
	imageID, err := a.getImageIdentifier(image)
	if err != nil {
		return AnalyzedMetadata{}, errors.Wrap(err, "retrieving image identifier")
	}

	var appMeta LayersMetadata
	// continue even if the label cannot be decoded
	if err := DecodeLabel(image, LayerMetadataLabel, &appMeta); err != nil {
		appMeta = LayersMetadata{}
	}

	for _, bp := range a.Buildpacks {
		if store := appMeta.MetadataForBuildpack(bp.ID).Store; store != nil {
			if err := WriteTOML(filepath.Join(a.LayersDir, launch.EscapeID(bp.ID), "store.toml"), store); err != nil {
				return AnalyzedMetadata{}, err
			}
		}
	}

	restorer := Restorer{
		LayersDir:  a.LayersDir,
		Buildpacks: a.Buildpacks,
		Logger:     a.Logger,
		SkipLayers: a.SkipLayers,
	}

	if err := restorer.analyzeLayers(appMeta, cache); err != nil {
		return AnalyzedMetadata{}, err
	}

	return AnalyzedMetadata{
		Image:    imageID,
		Metadata: appMeta,
	}, nil
}

func (a *Analyzer) getImageIdentifier(image imgutil.Image) (*ImageIdentifier, error) {
	if !image.Found() {
		a.Logger.Infof("Previous image with name %q not found", image.Name())
		return nil, nil
	}
	identifier, err := image.Identifier()
	if err != nil {
		return nil, err
	}
	a.Logger.Debugf("Analyzing image %q", identifier.String())
	return &ImageIdentifier{
		Reference: identifier.String(),
	}, nil
}
