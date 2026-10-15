package archive

import "golang.org/x/sys/windows"

// diskFree returns bytes available to this user on the volume holding dir, following junctions.
func diskFree(dir string) (int64, error) {
	p, err := windows.UTF16PtrFromString(dir)
	if err != nil {
		return 0, err
	}
	var free, total, all uint64
	if err := windows.GetDiskFreeSpaceEx(p, &free, &total, &all); err != nil {
		return 0, err
	}
	return int64(free), nil
}
