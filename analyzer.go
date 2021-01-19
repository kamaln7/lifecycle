package lifecycle

import (
	"github.com/buildpacks/imgutil"
	"github.com/pkg/errors"

	"github.com/buildpacks/lifecycle/api"
)

type Analyzer struct {
	Buildpacks  []GroupBuildpack
	LayersDir   string
	Logger      Logger
	SkipLayers  bool
	PlatformAPI *api.Version
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

	if a.PlatformAPI.Compare(api.MustParse("0.6")) < 0 { // platform API < 0.6
		restorer := Restorer{
			LayersDir:  a.LayersDir,
			Buildpacks: a.Buildpacks,
			Logger:     a.Logger,
			SkipLayers: a.SkipLayers,
		}

		meta, err := restorer.retrieveMetadataFrom(cache)
		if err != nil {
			return AnalyzedMetadata{}, err
		}

		if err := restorer.restoreStoreTOML(appMeta); err != nil {
			return AnalyzedMetadata{}, err
		}

		if err := restorer.analyzeLayers(appMeta, meta); err != nil {
			return AnalyzedMetadata{}, err
		}
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
