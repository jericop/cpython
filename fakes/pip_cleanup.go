package fakes

import (
	"io/fs"
	"sync"
)

type PythonPipCleanup struct {
	CleanupCall struct {
		mutex     sync.Mutex
		CallCount int
		Receives  struct {
			Packages         []string
			TargetLayer      string
			TargetLayerBinFs fs.FS
			PipGlobPattern   string
		}
		Returns struct {
			Error error
		}
		Stub func([]string, string, fs.FS, string) error
	}
}

func (f *PythonPipCleanup) Cleanup(param1 []string, param2 string, param3 fs.FS, param4 string) error {
	f.CleanupCall.mutex.Lock()
	defer f.CleanupCall.mutex.Unlock()
	f.CleanupCall.CallCount++
	f.CleanupCall.Receives.Packages = param1
	f.CleanupCall.Receives.TargetLayer = param2
	f.CleanupCall.Receives.TargetLayerBinFs = param3
	f.CleanupCall.Receives.PipGlobPattern = param4
	if f.CleanupCall.Stub != nil {
		return f.CleanupCall.Stub(param1, param2, param3, param4)
	}
	return f.CleanupCall.Returns.Error
}
