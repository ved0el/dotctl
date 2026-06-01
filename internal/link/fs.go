package link

import "os"

// FS is the filesystem surface the Linker depends on. The real implementation
// (OsFS) wraps the os package; tests can substitute a fake or use OsFS against a
// temporary directory.
type FS interface {
	Lstat(name string) (os.FileInfo, error)
	Readlink(name string) (string, error)
	ReadDir(name string) ([]os.DirEntry, error)
	Symlink(oldname, newname string) error
	MkdirAll(path string, perm os.FileMode) error
	Rename(oldpath, newpath string) error
	Remove(name string) error
}

// OsFS implements FS against the real filesystem.
type OsFS struct{}

func (OsFS) Lstat(name string) (os.FileInfo, error)       { return os.Lstat(name) }
func (OsFS) Readlink(name string) (string, error)         { return os.Readlink(name) }
func (OsFS) ReadDir(name string) ([]os.DirEntry, error)   { return os.ReadDir(name) }
func (OsFS) Symlink(oldname, newname string) error        { return os.Symlink(oldname, newname) }
func (OsFS) MkdirAll(path string, perm os.FileMode) error { return os.MkdirAll(path, perm) }
func (OsFS) Rename(oldpath, newpath string) error         { return os.Rename(oldpath, newpath) }
func (OsFS) Remove(name string) error                     { return os.Remove(name) }
