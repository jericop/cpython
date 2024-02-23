package cpython_test

import (
	"bytes"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
	"testing/fstest"

	"github.com/paketo-buildpacks/cpython"
	"github.com/paketo-buildpacks/cpython/fakes"
	"github.com/paketo-buildpacks/packit/v2/pexec"
	"github.com/paketo-buildpacks/packit/v2/scribe"
	"github.com/sclevine/spec"

	. "github.com/onsi/gomega"
	// . "github.com/onsi/gomega/gstruct"
)

func testPipCleanup(t *testing.T, context spec.G, it spec.S) {
	var (
		Expect = NewWithT(t).Expect

		layerPath             string
		pythonProcess         *fakes.Executable
		pipCleanup            cpython.PythonPipCleanup
		executableFileDeleter *fakes.ExecutableFileDeleter
	)

	it.Before(func() {
		var err error

		layerPath, err = os.MkdirTemp("", "layer")
		Expect(err).NotTo(HaveOccurred())

		Expect(os.MkdirAll(filepath.Join(layerPath, "bin"), 0755)).To(Succeed())

		pythonProcess = &fakes.Executable{}
		executableFileDeleter = &fakes.ExecutableFileDeleter{}

		pipCleanup = cpython.NewPipCleanup(pythonProcess, executableFileDeleter, scribe.NewEmitter(bytes.NewBuffer(nil)))
	})

	it.After(func() {
		Expect(os.RemoveAll(layerPath)).To(Succeed())
	})

	context("Execute", func() {
		var (
			pythonInvocationArgs     [][]string
			packages                 []string
			pythonBinFs              fstest.MapFS
			pipGlobPattern           string
			pipExecutables           []*fakes.Executable
			pipExecutableInvocations [][]string
			deleteFileInvocations    []string
		)

		it.Before(func() {
			pythonInvocationArgs = [][]string{}
			pipExecutableInvocations = [][]string{}
			packages = []string{}
			pythonBinFs = fstest.MapFS{}
			pipGlobPattern = "pip*"

			pythonProcess.ExecuteCall.Stub = func(e pexec.Execution) error {
				pythonInvocationArgs = append(pythonInvocationArgs, e.Args)
				return nil
			}

			executableFileDeleter.ExecutableCall.Stub = func(filepath string) cpython.Executable {
				executable := &fakes.Executable{}
				executable.ExecuteCall.Stub = func(e pexec.Execution) error {
					pipExecutableInvocations = append(pipExecutableInvocations, e.Args)
					return nil
				}
				pipExecutables = append(pipExecutables, executable)
				return executable
			}

			executableFileDeleter.DeleteCall.Stub = func(filepath string) error {
				deleteFileInvocations = append(deleteFileInvocations, filepath)
				return nil
			}
		})

		context("when packages are installed", func() {
			it.Before(func() {
				packages = []string{"somepkg"}
			})

			it("uninstalls packages", func() {
				err := pipCleanup.Cleanup(packages, layerPath, pythonBinFs, pipGlobPattern)
				Expect(err).NotTo(HaveOccurred())

				Expect(pythonProcess.ExecuteCall.CallCount).To(Equal(3))
				Expect(pythonInvocationArgs[0]).To(Equal([]string{"-m", "pip", "--version"}))
				Expect(pythonInvocationArgs[1]).To(Equal([]string{"-m", "pip", "show", "-q", packages[0]}))
				Expect(pythonInvocationArgs[2]).To(Equal([]string{"-m", "pip", "uninstall", "-y", packages[0]}))
			})
		})

		context("when packages are not installed", func() {
			it.Before(func() {
				packages = []string{"somepkg-that-does-not-exist"}

				pythonProcess.ExecuteCall.Stub = func(e pexec.Execution) error {
					pythonInvocationArgs = append(pythonInvocationArgs, e.Args)
					if e.Args[2] == "show" {
						return errors.New("pip package not found")
					}
					return nil
				}
			})

			it("does not uninstall packages", func() {
				err := pipCleanup.Cleanup(packages, layerPath, pythonBinFs, pipGlobPattern)
				Expect(err).NotTo(HaveOccurred())

				Expect(pythonProcess.ExecuteCall.CallCount).To(Equal(2))
				Expect(pythonInvocationArgs[0]).To(Equal([]string{"-m", "pip", "--version"}))
				Expect(pythonInvocationArgs[1]).To(Equal([]string{"-m", "pip", "show", "-q", packages[0]}))
			})
		})

		context("when pip executables exist", func() {
			context("and they are not broken", func() {
				it.Before(func() {
					pythonBinFs = fstest.MapFS{
						"pip3":         &fstest.MapFile{Data: []byte(""), Mode: fs.ModePerm},
						"some-dir/pip": &fstest.MapFile{Data: []byte(""), Mode: fs.ModePerm},
					}
				})

				it("no files are deleted", func() {
					err := pipCleanup.Cleanup(packages, layerPath, pythonBinFs, pipGlobPattern)
					Expect(err).NotTo(HaveOccurred())

					Expect(pythonProcess.ExecuteCall.CallCount).To(Equal(0))
					Expect(len(pipExecutableInvocations)).To(Equal(1))
					Expect(len(deleteFileInvocations)).To(Equal(0))
				})
			})

			context("and they are broken", func() {
				it.Before(func() {
					pythonBinFs = fstest.MapFS{
						"pip3":         &fstest.MapFile{Data: []byte(""), Mode: fs.ModePerm},
						"some-dir/pip": &fstest.MapFile{Data: []byte(""), Mode: fs.ModePerm}, // pip files in subdirs are ignored
					}

					// Force the executable to return an error to trigger a file delete
					executableFileDeleter.ExecutableCall.Stub = func(filepath string) cpython.Executable {
						executable := &fakes.Executable{}
						executable.ExecuteCall.Stub = func(e pexec.Execution) error {
							pipExecutableInvocations = append(pipExecutableInvocations, e.Args)
							return errors.New("bad pip executable")
						}
						pipExecutables = append(pipExecutables, executable)
						return executable
					}
				})

				it("files are deleted", func() {
					err := pipCleanup.Cleanup(packages, layerPath, pythonBinFs, pipGlobPattern)
					Expect(err).NotTo(HaveOccurred())

					Expect(pythonProcess.ExecuteCall.CallCount).To(Equal(0))
					Expect(executableFileDeleter.ExecutableCall.CallCount).To(Equal(1))
					Expect(pipExecutableInvocations[0]).To(Equal([]string{"--version"}))

					Expect(executableFileDeleter.DeleteCall.CallCount).To(Equal(1))
					Expect(deleteFileInvocations[0]).To(Equal(filepath.Join(layerPath, "bin", "pip3")))
				})
			})
		})

		context("when pip executables do not exist", func() {
			it.Before(func() {
				pythonBinFs = fstest.MapFS{
					"some-binary":    &fstest.MapFile{Data: []byte(""), Mode: fs.ModePerm},
					"another-binary": &fstest.MapFile{Data: []byte(""), Mode: fs.ModePerm},
				}
			})

			it("no files are executed or deleted", func() {
				err := pipCleanup.Cleanup(packages, layerPath, pythonBinFs, pipGlobPattern)
				Expect(err).NotTo(HaveOccurred())

				Expect(pythonProcess.ExecuteCall.CallCount).To(Equal(0))
				Expect(executableFileDeleter.ExecutableCall.CallCount).To(Equal(0))
				Expect(executableFileDeleter.DeleteCall.CallCount).To(Equal(0))
			})
		})

		context("failure cases", func() {
			context("when pip version returns an error", func() {
				it.Before(func() {
					packages = []string{"somepkg"}

					pythonProcess.ExecuteCall.Stub = func(e pexec.Execution) error {
						pythonInvocationArgs = append(pythonInvocationArgs, e.Args)
						return errors.New("pip is broken")
					}
				})

				it("fails with error", func() {
					err := pipCleanup.Cleanup(packages, layerPath, pythonBinFs, pipGlobPattern)
					Expect(err).Should(MatchError(And(
						ContainSubstring("pip is broken"),
					)))
					Expect(pythonProcess.ExecuteCall.CallCount).To(Equal(1))
					Expect(pythonInvocationArgs[0]).To(Equal([]string{"-m", "pip", "--version"}))
				})
			})

			context("when uninstalling packages", func() {
				it.Before(func() {
					packages = []string{"somepkg"}

					pythonProcess.ExecuteCall.Stub = func(e pexec.Execution) error {
						pythonInvocationArgs = append(pythonInvocationArgs, e.Args)
						if e.Args[2] == "uninstall" {
							return errors.New("failed to uninstall pip package")
						}
						return nil
					}
				})

				it("fails with error", func() {
					err := pipCleanup.Cleanup(packages, layerPath, pythonBinFs, pipGlobPattern)
					Expect(err).Should(MatchError(And(
						ContainSubstring("failed to uninstall pip package"),
					)))
					Expect(pythonProcess.ExecuteCall.CallCount).To(Equal(3))
					Expect(pythonInvocationArgs[0]).To(Equal([]string{"-m", "pip", "--version"}))
					Expect(pythonInvocationArgs[1]).To(Equal([]string{"-m", "pip", "show", "-q", packages[0]}))
					Expect(pythonInvocationArgs[2]).To(Equal([]string{"-m", "pip", "uninstall", "-y", packages[0]}))
				})
			})

			context("when deleting broken pip executables", func() {
				it.Before(func() {
					pythonBinFs = fstest.MapFS{
						"pip": &fstest.MapFile{Data: []byte(""), Mode: fs.ModePerm},
					}

					// Force the executable to return an error to trigger a file delete
					executableFileDeleter.ExecutableCall.Stub = func(filepath string) cpython.Executable {
						executable := &fakes.Executable{}
						executable.ExecuteCall.Stub = func(e pexec.Execution) error {
							pipExecutableInvocations = append(pipExecutableInvocations, e.Args)
							return errors.New("bad pip executable")
						}
						pipExecutables = append(pipExecutables, executable)
						return executable
					}

					executableFileDeleter.DeleteCall.Stub = func(filepath string) error {
						deleteFileInvocations = append(deleteFileInvocations, filepath)
						return errors.New("failed to delete file")
					}
				})

				it("fails with error", func() {
					err := pipCleanup.Cleanup(packages, layerPath, pythonBinFs, pipGlobPattern)
					Expect(err).Should(MatchError(And(
						ContainSubstring("failed to delete file"),
					)))
					Expect(pythonProcess.ExecuteCall.CallCount).To(Equal(0))
					Expect(executableFileDeleter.ExecutableCall.CallCount).To(Equal(1))
					Expect(pipExecutableInvocations[0]).To(Equal([]string{"--version"}))

					Expect(executableFileDeleter.DeleteCall.CallCount).To(Equal(1))
					Expect(deleteFileInvocations[0]).To(Equal(filepath.Join(layerPath, "bin", "pip")))
				})
			})

			context("when glob pattern is invalid", func() {
				it.Before(func() {
					pipGlobPattern = "["
				})

				it("fails with error", func() {
					err := pipCleanup.Cleanup(packages, layerPath, pythonBinFs, pipGlobPattern)
					Expect(err).Should(MatchError(And(
						ContainSubstring("syntax error in pattern"),
					)))
					Expect(pythonProcess.ExecuteCall.CallCount).To(Equal(0))
					Expect(executableFileDeleter.ExecutableCall.CallCount).To(Equal(0))
					Expect(executableFileDeleter.DeleteCall.CallCount).To(Equal(0))
				})
			})
		})
	})
}
