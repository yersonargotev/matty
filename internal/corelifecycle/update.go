package corelifecycle

import (
	"strings"

	"github.com/yersonargotev/matty/internal/bootstrap"
)

func (facade *Facade) validateUpdateInstalledSource() error {
	if !facade.config.SkillSourceIsDefault {
		return nil
	}
	releaseRef := ""
	if strings.HasPrefix(facade.config.RunningVersion, "v") {
		releaseRef = facade.config.RunningVersion
	}
	return bootstrap.ValidateInstalledSourceRef(bootstrap.BootstrapOptions{
		SourceRoot:    facade.config.InstalledSourceRoot,
		RepositoryRef: releaseRef,
		HomeDir:       facade.config.HomeDir,
		ConfigHome:    facade.config.ConfigHome,
	})
}
