package fakes

import (
	"sync"

	"github.com/paketo-buildpacks/cpython"
)

type ExecutableFileDeleter struct {
	DeleteCall struct {
		mutex     sync.Mutex
		CallCount int
		Receives  struct {
			Filepath string
		}
		Returns struct {
			Error error
		}
		Stub func(string) error
	}
	ExecutableCall struct {
		mutex     sync.Mutex
		CallCount int
		Receives  struct {
			Filepath string
		}
		Returns struct {
			Executable cpython.Executable
		}
		Stub func(string) cpython.Executable
	}
}

func (f *ExecutableFileDeleter) Delete(param1 string) error {
	f.DeleteCall.mutex.Lock()
	defer f.DeleteCall.mutex.Unlock()
	f.DeleteCall.CallCount++
	f.DeleteCall.Receives.Filepath = param1
	if f.DeleteCall.Stub != nil {
		return f.DeleteCall.Stub(param1)
	}
	return f.DeleteCall.Returns.Error
}
func (f *ExecutableFileDeleter) Executable(param1 string) cpython.Executable {
	f.ExecutableCall.mutex.Lock()
	defer f.ExecutableCall.mutex.Unlock()
	f.ExecutableCall.CallCount++
	f.ExecutableCall.Receives.Filepath = param1
	if f.ExecutableCall.Stub != nil {
		return f.ExecutableCall.Stub(param1)
	}
	return f.ExecutableCall.Returns.Executable
}
