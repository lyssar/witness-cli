package internal

import (
	"fmt"

	"github.com/lyssar/skuld-cli/utils"
)

type ReconcileRun struct {
	configRoot string
	manifest   Observer
}

func NewReconcileRun(configRoot string) ReconcileRun {
	reconcileRun := ReconcileRun{
		configRoot: configRoot,
	}

	return reconcileRun
}

func (rr ReconcileRun) getManifestFilePath() string {
	return fmt.Sprintf("%s/manifest.yaml", rr.configRoot)
}

func (rr ReconcileRun) getAgeFilePath() string {
	return fmt.Sprintf("%s/age.key", rr.configRoot)
}

func (rr ReconcileRun) Validate() (bool, error) {
	manifestFile := rr.getManifestFilePath()
	if !utils.FileExists(manifestFile) {
		return false, fmt.Errorf("Manifest file could not be found in config root (%s)", manifestFile)
	}

	ageFile := rr.getAgeFilePath()
	if !utils.FileExists(ageFile) {
		return false, fmt.Errorf("Age file could not be found in config root (%s)", ageFile)
	}

	return true, nil
}

func (rr ReconcileRun) LoadManifest() error {
	manifestFile := rr.getManifestFilePath()
	ageFile := rr.getAgeFilePath()
	observer, err := NewObserverFromManifest(manifestFile, ageFile)
	if err != nil {
		return err
	}
	rr.manifest = observer
	return nil
}

func (rr ReconcileRun) LoadState() error {
	// stateFile := fmt.Sprintf("%s/state.yaml", rr.configRoot)
	return nil
}

func (rr ReconcileRun) Reconcile() error {
	// TODO
	// - checkout external repo into configRoot/repo
	// -

	return nil
}
