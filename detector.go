package lifecycle

import (
	"fmt"
	"sync"

	"github.com/pkg/errors"

	"github.com/buildpacks/lifecycle/buildpack"
)

const (
	CodeDetectPass = 0
	CodeDetectFail = 100
)

var (
	errFailedDetection = errors.New("no buildpacks participating")
	errBuildpack       = errors.New("buildpack(s) failed with err")
)

type Detector struct {
	buildpack.DetectConfig
}

func (d *Detector) Detect(order buildpack.BuildpackOrder) (buildpack.BuildpackGroup, buildpack.BuildPlan, error) {
	processor := &Processor{Logger: d.Logger}
	return order.Detect(&d.DetectConfig, processor)
}

type Processor struct {
	Logger Logger
	runs   *sync.Map
}

func (p *Processor) Process(done []buildpack.GroupBuildpack) ([]buildpack.GroupBuildpack, []buildpack.BuildPlanEntry, error) {
	var runs []buildpack.DetectRun
	for _, bp := range done {
		t, ok := p.runs.Load(bp.String())
		if !ok {
			return nil, nil, errors.Errorf("missing detection of '%s'", bp)
		}
		run := t.(buildpack.DetectRun)
		outputLogf := p.Logger.Debugf

		switch run.Code {
		case CodeDetectPass, CodeDetectFail:
		default:
			outputLogf = p.Logger.Infof
		}

		if len(run.Output) > 0 {
			outputLogf("======== Output: %s ========", bp)
			outputLogf(string(run.Output))
		}
		if run.Err != nil {
			outputLogf("======== Error: %s ========", bp)
			outputLogf(run.Err.Error())
		}
		runs = append(runs, run)
	}

	p.Logger.Debugf("======== Results ========")

	results := detectResults{}
	detected := true
	buildpackErr := false
	for i, bp := range done {
		run := runs[i]
		switch run.Code {
		case CodeDetectPass:
			p.Logger.Debugf("pass: %s", bp)
			results = append(results, detectResult{bp, run})
		case CodeDetectFail:
			if bp.Optional {
				p.Logger.Debugf("skip: %s", bp)
			} else {
				p.Logger.Debugf("fail: %s", bp)
			}
			detected = detected && bp.Optional
		case -1:
			p.Logger.Infof("err:  %s", bp)
			buildpackErr = true
			detected = detected && bp.Optional
		default:
			p.Logger.Infof("err:  %s (%d)", bp, run.Code)
			buildpackErr = true
			detected = detected && bp.Optional
		}
	}
	if !detected {
		if buildpackErr {
			return nil, nil, errBuildpack
		}
		return nil, nil, errFailedDetection
	}

	i := 0
	deps, trial, err := results.runTrials(func(trial detectTrial) (depMap, detectTrial, error) {
		i++
		return p.runTrial(i, trial)
	})
	if err != nil {
		return nil, nil, err
	}

	if len(done) != len(trial) {
		p.Logger.Infof("%d of %d buildpacks participating", len(trial), len(done))
	}

	maxLength := 0
	for _, t := range trial {
		l := len(t.ID)
		if l > maxLength {
			maxLength = l
		}
	}

	f := fmt.Sprintf("%%-%ds %%s", maxLength)

	for _, t := range trial {
		p.Logger.Infof(f, t.ID, t.Version)
	}

	var found []buildpack.GroupBuildpack
	for _, r := range trial {
		found = append(found, r.GroupBuildpack.NoOpt())
	}
	var plan []buildpack.BuildPlanEntry
	for _, dep := range deps {
		plan = append(plan, dep.BuildPlanEntry.NoOpt())
	}
	return found, plan, nil
}

func (p *Processor) Runs() *sync.Map {
	return p.runs
}

func (p *Processor) SetRuns(runs *sync.Map) {
	p.runs = runs
	return
}

func (p *Processor) runTrial(i int, trial detectTrial) (depMap, detectTrial, error) {
	p.Logger.Debugf("Resolving plan... (try #%d)", i)

	var deps depMap
	retry := true
	for retry {
		retry = false
		deps = newDepMap(trial)

		if err := deps.eachUnmetRequire(func(name string, bp buildpack.GroupBuildpack) error {
			retry = true
			if !bp.Optional {
				p.Logger.Debugf("fail: %s requires %s", bp, name)
				return errFailedDetection
			}
			p.Logger.Debugf("skip: %s requires %s", bp, name)
			trial = trial.remove(bp)
			return nil
		}); err != nil {
			return nil, nil, err
		}

		if err := deps.eachUnmetProvide(func(name string, bp buildpack.GroupBuildpack) error {
			retry = true
			if !bp.Optional {
				p.Logger.Debugf("fail: %s provides unused %s", bp, name)
				return errFailedDetection
			}
			p.Logger.Debugf("skip: %s provides unused %s", bp, name)
			trial = trial.remove(bp)
			return nil
		}); err != nil {
			return nil, nil, err
		}
	}

	if len(trial) == 0 {
		p.Logger.Debugf("fail: no viable buildpacks in group")
		return nil, nil, errFailedDetection
	}
	return deps, trial, nil
}

type detectResult struct {
	buildpack.GroupBuildpack
	buildpack.DetectRun
}

func (r *detectResult) options() []detectOption {
	var out []detectOption
	for i, sections := range append([]buildpack.PlanSections{r.PlanSections}, r.Or...) {
		bp := r.GroupBuildpack
		bp.Optional = bp.Optional && i == len(r.Or)
		out = append(out, detectOption{bp, sections})
	}
	return out
}

type detectResults []detectResult
type trialFunc func(detectTrial) (depMap, detectTrial, error)

func (rs detectResults) runTrials(f trialFunc) (depMap, detectTrial, error) {
	return rs.runTrialsFrom(nil, f)
}

func (rs detectResults) runTrialsFrom(prefix detectTrial, f trialFunc) (depMap, detectTrial, error) {
	if len(rs) == 0 {
		deps, trial, err := f(prefix)
		return deps, trial, err
	}

	var lastErr error
	for _, option := range rs[0].options() {
		deps, trial, err := rs[1:].runTrialsFrom(append(prefix, option), f)
		if err == nil {
			return deps, trial, nil
		}
		lastErr = err
	}
	return nil, nil, lastErr
}

type detectOption struct {
	buildpack.GroupBuildpack
	buildpack.PlanSections
}

type detectTrial []detectOption

func (ts detectTrial) remove(bp buildpack.GroupBuildpack) detectTrial {
	var out detectTrial
	for _, t := range ts {
		if t.GroupBuildpack != bp {
			out = append(out, t)
		}
	}
	return out
}

type depEntry struct {
	buildpack.BuildPlanEntry
	earlyRequires []buildpack.GroupBuildpack
	extraProvides []buildpack.GroupBuildpack
}

type depMap map[string]depEntry

func newDepMap(trial detectTrial) depMap {
	m := depMap{}
	for _, option := range trial {
		for _, p := range option.Provides {
			m.provide(option.GroupBuildpack, p)
		}
		for _, r := range option.Requires {
			m.require(option.GroupBuildpack, r)
		}
	}
	return m
}

func (m depMap) provide(bp buildpack.GroupBuildpack, provide buildpack.Provide) {
	entry := m[provide.Name]
	entry.extraProvides = append(entry.extraProvides, bp)
	m[provide.Name] = entry
}

func (m depMap) require(bp buildpack.GroupBuildpack, require buildpack.Require) {
	entry := m[require.Name]
	entry.Providers = append(entry.Providers, entry.extraProvides...)
	entry.extraProvides = nil

	if len(entry.Providers) == 0 {
		entry.earlyRequires = append(entry.earlyRequires, bp)
	} else {
		entry.Requires = append(entry.Requires, require)
	}
	m[require.Name] = entry
}

func (m depMap) eachUnmetProvide(f func(name string, bp buildpack.GroupBuildpack) error) error {
	for name, entry := range m {
		if len(entry.extraProvides) != 0 {
			for _, bp := range entry.extraProvides {
				if err := f(name, bp); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func (m depMap) eachUnmetRequire(f func(name string, bp buildpack.GroupBuildpack) error) error {
	for name, entry := range m {
		if len(entry.earlyRequires) != 0 {
			for _, bp := range entry.earlyRequires {
				if err := f(name, bp); err != nil {
					return err
				}
			}
		}
	}
	return nil
}
