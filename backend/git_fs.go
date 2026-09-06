package backend

import (
	"os"
	"time"

	"github.com/go-git/go-billy/v5"
)

// ----------------------------------------------------------------------
// Filesystem workarounds for Android
// ----------------------------------------------------------------------
//
// This file was part of git_helper.go until 26.09.22. That file held
// 2195 lines and four separate concerns. See the banner of git_repo.go
// for the split and for what each file holds.
//
// go-git speaks to the worktree and to the object store through a
// go-billy filesystem. The two wrappers below change what that
// filesystem answers, and each one repairs a real fault on Android.
// ----------------------------------------------------------------------
// Android filesystem workarounds
// ----------------------------------------------------------------------

type NoLockFS struct {
	billy.Filesystem
}

func (fs *NoLockFS) Create(filename string) (billy.File, error) {
	f, err := fs.Filesystem.Create(filename)
	if err != nil {
		return nil, err
	}
	return &NoLockFile{f}, nil
}

func (fs *NoLockFS) Open(filename string) (billy.File, error) {
	f, err := fs.Filesystem.Open(filename)
	if err != nil {
		return nil, err
	}
	return &NoLockFile{f}, nil
}

func (fs *NoLockFS) OpenFile(filename string, flag int, perm os.FileMode) (billy.File, error) {
	f, err := fs.Filesystem.OpenFile(filename, flag, perm)
	if err != nil {
		return nil, err
	}
	return &NoLockFile{f}, nil
}

func (fs *NoLockFS) TempFile(dir, prefix string) (billy.File, error) {
	f, err := fs.Filesystem.TempFile(dir, prefix)
	if err != nil {
		return nil, err
	}
	return &NoLockFile{f}, nil
}

func (fs *NoLockFS) Chroot(path string) (billy.Filesystem, error) {
	c, err := fs.Filesystem.Chroot(path)
	if err != nil {
		return nil, err
	}
	return &NoLockFS{c}, nil
}

type NoLockFile struct {
	billy.File
}

func (f *NoLockFile) Lock() error   { return nil }
func (f *NoLockFile) Unlock() error { return nil }

// ---------------------------------------------------------------
// Stable mtime wrapper (forces content‑hash based status check)
// ---------------------------------------------------------------

type stableMtimeFS struct {
	billy.Filesystem
}

func (fs *stableMtimeFS) Stat(path string) (os.FileInfo, error) {
	fi, err := fs.Filesystem.Stat(path)
	if err != nil {
		return nil, err
	}
	return &stableFileInfo{fi}, nil
}

func (fs *stableMtimeFS) Lstat(path string) (os.FileInfo, error) {
	fi, err := fs.Filesystem.Lstat(path)
	if err != nil {
		return nil, err
	}
	return &stableFileInfo{fi}, nil
}

type stableFileInfo struct {
	os.FileInfo
}

func (fi *stableFileInfo) ModTime() time.Time {
	return time.Unix(0, 0)
}
