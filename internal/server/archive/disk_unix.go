//go:build unix

package archive

import "golang.org/x/sys/unix"

// diskFree returns bytes available to this user on the file system holding dir, following symlinks.
func diskFree(dir string) (int64, error) {
	var st unix.Statfs_t
	if err := unix.Statfs(dir, &st); err != nil {
		return 0, err
	}
	return int64(st.Bavail) * int64(st.Bsize), nil
}
