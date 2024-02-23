package cpython

import (
	"io/fs"
	"os"
	"path/filepath"

	"github.com/paketo-buildpacks/packit/v2/pexec"
	"github.com/paketo-buildpacks/packit/v2/scribe"
)

//go:generate faux --interface ExecutableFileDeleter --output fakes/executable_file_deleter.go

// This function serves as a constant for packages to be uninstalled
func pipPackagesToBeUninstalled() []string {
	return []string{"setuptools"}
}

// ExecutableFileDeleter defines the interface for executing and deleting pip executables in the python installation bin folder
type ExecutableFileDeleter interface {
	Executable(filepath string) Executable
	Delete(filepath string) error
}

// PipExecutableDeleter implements the ExecutableFileDeleter interface.
type PipExecutableDeleter struct{}

func (d PipExecutableDeleter) Executable(filepath string) Executable {
	return pexec.NewExecutable(filepath)
}

func (d PipExecutableDeleter) Delete(filepath string) error {
	return os.Remove(filepath)
}

// PipCleanup implements the PythonPipCleanup interface.
type PipCleanup struct {
	pythonProcess         Executable
	executableFileDeleter ExecutableFileDeleter
	logger                scribe.Emitter
}

// NewPipCleanup creates an instance of PipCleanup given a python Executable, an executableFileDeleter, and a scribe.Emitter.
func NewPipCleanup(pythonProcess Executable, executableFileDeleter ExecutableFileDeleter, logger scribe.Emitter) PipCleanup {
	return PipCleanup{
		pythonProcess:         pythonProcess,
		executableFileDeleter: executableFileDeleter,
		logger:                logger,
	}
}

func (i PipCleanup) Cleanup(packages []string, targetLayer string, targetLayerBinFs fs.FS, pipGlobPattern string) error {
	env := environWithUpdatedPath(os.Environ(), "PATH", filepath.Join(targetLayer, "bin"))
	env = environWithUpdatedPath(env, "LD_LIBRARY_PATH", filepath.Join(targetLayer, "lib"))

	if len(packages) > 0 {
		// Verify pip --version works to ensure subsequent pip commands will work
		err := i.pythonProcess.Execute(pexec.Execution{
			Args:   []string{"-m", "pip", "--version"},
			Env:    env,
			Stdout: i.logger.Debug.ActionWriter,
			Stderr: i.logger.Debug.ActionWriter,
		})
		if err != nil {
			i.logger.Subprocess("pip --version failed. Run with --env BP_LOG_LEVEL=DEBUG to see more information")
			return err
		}

		// Remove packages from site-packages in the targetLayer
		for _, name := range packages {
			i.logger.Debug.Subprocess("Checking if '%s' package is installed", name)
			err := i.pythonProcess.Execute(pexec.Execution{
				Args:   []string{"-m", "pip", "show", "-q", name},
				Env:    env,
				Stdout: i.logger.Debug.ActionWriter,
				Stderr: i.logger.Debug.ActionWriter,
			})

			if err == nil {
				i.logger.Debug.Subprocess("Uninstalling '%s' package", name)
				err = i.pythonProcess.Execute(pexec.Execution{
					Args:   []string{"-m", "pip", "uninstall", "-y", name},
					Env:    env,
					Stdout: i.logger.Debug.ActionWriter,
					Stderr: i.logger.Debug.ActionWriter,
				})
				if err != nil {
					i.logger.Subprocess("pip uninstall failed. Run with --env BP_LOG_LEVEL=DEBUG to see more information")
					return err
				}
			}
		}
	}

	// Get pip executables in the bin directory and remove them if they are broken
	files, err := fs.Glob(targetLayerBinFs, pipGlobPattern)
	if err != nil {
		return err
	}

	for _, f := range files {
		filepath := filepath.Join(targetLayer, "bin", f)

		executable := i.executableFileDeleter.Executable(filepath)
		err = executable.Execute(pexec.Execution{
			Args:   []string{"--version"},
			Env:    env,
			Stdout: i.logger.Debug.ActionWriter,
			Stderr: i.logger.Debug.ActionWriter,
		})

		if err != nil {
			i.logger.Debug.Subprocess("Deleting broken pip executable '%s'", f)
			if err := i.executableFileDeleter.Delete(filepath); err != nil {
				return err
			}
		}
	}

	return nil
}
