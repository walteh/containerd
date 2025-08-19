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
		if err := handleChownError(err, path, uid, gid); err != nil {
			l.Close()
			return nil, err
		}
	}

	return l, nil
}

// keeping this for debugging purposes for now, really only the first if statement is needed
// it basically just bypasses the error if we are on darwin not running as root
// the rest is just a sanity check
func handleChownError(err error, path string, uid, gid int) error {
	if runtime.GOOS != "darwin" || syscall.Getuid() == 0 {
		return err
	}

	var fi syscall.Stat_t
	if ferr := syscall.Stat(path, &fi); ferr != nil {
		return err
	}

	var pathErr *fs.PathError
	if !errors.As(err, &pathErr) {
		return err
	}

	if !errors.Is(pathErr.Err, syscall.EPERM) {
		return err
	}

	if pathErr.Op != "chown" {
		return err
	}

	log.L.WithFields(log.Fields{
		"path":        path,
		"actual_uid":  fi.Uid,
		"actual_gid":  fi.Gid,
		"desired_uid": uid,
		"desired_gid": gid,
	}).Warn("ignoring darwin EPERM from chown; socket will stay private to current user")

	return nil

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
