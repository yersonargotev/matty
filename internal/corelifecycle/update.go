package corelifecycle

import (
	"strings"

	"github.com/yersonargotev/packy/internal/bootstrap"
)

func (facade *Facade) validateUpdateInstalledSource() error {
	if !facade.config.SkillSource.IsDefault {
		return nil
	}
	releaseRef := ""
	if strings.HasPrefix(facade.config.RunningVersion, "v") {
		releaseRef = facade.config.RunningVersion
	}
	return bootstrap.ValidateInstalledSourceRef(bootstrap.BootstrapOptions{
		InstalledSource: facade.config.InstalledSource,
		RepositoryRef:   releaseRef,
	})
}
