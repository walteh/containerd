//go:build !windows

/*
   Copyright The containerd Authors.

   Licensed under the Apache License, Version 2.0 (the "License");
   you may not use this file except in compliance with the License.
   You may obtain a copy of the License at

       http://www.apache.org/licenses/LICENSE-2.0

   Unless required by applicable law or agreed to in writing, software
   distributed under the License is distributed on an "AS IS" BASIS,
   WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
   See the License for the specific language governing permissions and
   limitations under the License.
*/

package sys

import (
	"errors"
	"fmt"
	"io/fs"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"syscall"

	"github.com/containerd/log"
	"golang.org/x/sys/unix"
)

// CreateUnixSocket creates a unix socket and returns the listener
func CreateUnixSocket(path string) (net.Listener, error) {
	// BSDs have a 104 limit
	if len(path) > 104 {
		return nil, fmt.Errorf("%q: unix socket path too long (> 104)", path)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0660); err != nil {
		return nil, err
	}
	if err := unix.Unlink(path); err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	return net.Listen("unix", path)
}

// GetLocalListener returns a listener out of a unix socket.
func GetLocalListener(path string, uid, gid int) (net.Listener, error) {
	// Ensure parent directory is created
	if err := mkdirAs(filepath.Dir(path), uid, gid); err != nil {
		return nil, err
	}

	l, err := CreateUnixSocket(path)
	if err != nil {
		return l, fmt.Errorf("failed to create unix socket on %s: %w", path, err)
	}

	if err := os.Chmod(path, 0o660); err != nil { // rw‑rw‑---
		l.Close()
		return nil, err
	}

	if err := os.Chown(path, uid, gid); err != nil {
		// part of rootless macos
		// TODO: figure out if this is actually needed
		// didn't put much thought into this or test it much, could have security implications
		var pathErr *fs.PathError
		if syscall.Getuid() != 0 && runtime.GOOS == "darwin" && errors.As(err, &pathErr) {
			// log.L.WithError(err).WithFields(log.Fields{
			// 	"path":           path,
			// 	"uid":            uid,
			// 	"gid":            gid,
			// 	"type":           reflect.TypeOf(err).String(),
			// 	"pathErr":        pathErr,
			// 	"pathErr.Op":     pathErr.Op,
			// 	"pathErr.Path":   pathErr.Path,
			// 	"pathErr.Err":    pathErr.Err,
			// 	"pathErr.ErrPtr": uintptr((pathErr.Err).(syscall.Errno)),
			// 	"pathErrType":    reflect.TypeOf(pathErr.Err).String(),
			// }).Error("failed to chown socket")

			if pathErr.Op == "chown" && errors.Is(pathErr.Err, syscall.EPERM) {
				log.L.Warn("ignoring darwin EPERM from chown; socket will stay private to current user")
				return l, nil // already owned by desired user/group
			}

			// if st, statErr := os.Stat(path); statErr == nil {
			// 	if sys, ok := st.Sys().(*syscall.Stat_t); ok &&
			// 		int(sys.Uid) == uid && int(sys.Gid) == gid {
			// 		log.L.Warn("ignoring darwin EPERM from chown; socket will stay private to current user")
			// 		return l, nil // already owned by desired user/group
			// 	}
			// }
		}
		l.Close()
		return nil, err
	}

	return l, nil
}

func mkdirAs(path string, uid, gid int) error {
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		return err
	}

	if err := os.MkdirAll(path, 0770); err != nil {
		return err
	}

	return os.Chown(path, uid, gid)
}
