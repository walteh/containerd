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

package mount

import (
	"fmt"
	"os"
	"os/exec"
)

func getDawinMountSystem() (string, error) {
	// DARWIN_MOUNT_SYSTEM can be 'macfuse' (default if empty), 'macfuse-fskit' or 'fuse-t'
	// 'macfuse' is the most stable but requires a kext to be installed
	mountSystem := os.Getenv("DARWIN_MOUNT_SYSTEM")
	if mountSystem == "" {
		mountSystem = "macfuse"
	}

	if mountSystem != "fuse-t" && mountSystem != "macfuse" && mountSystem != "macfuse-fskit" {
		return "", fmt.Errorf("invalid DARWIN_MOUNT_SYSTEM: %s", mountSystem)
	}

	return mountSystem, nil
}

// Mount to the provided target.
// Mount to the provided target.
func (m *Mount) mount(target string) error {

	mountSystem, err := getDawinMountSystem()
	if err != nil {
		return err
	}

	var commandName string
	if m.Type == "bind" {
		// macOS doesn't natively support bindfs/nullfs
		// The way to emulate it is via FUSE fs named "bindfs"
		commandName = "bindfs"
	} else {
		commandName = fmt.Sprintf("mount_%s", m.Type)
	}

	var args []string
	for _, option := range m.Options {
		if option == "rbind" {
			// On one side, rbind is not supported by macOS mounting tools
			// On the other, bindfs works as if rbind is enabled anyway
			continue
		}

		args = append(args, "-o", option)
	}

	if commandName == "bindfs" && mountSystem == "macfuse-fskit" {
		args = append(args, "-o", "backend=fskit")
	}

	args = append(args, m.Source, target)

	cmd := exec.Command(commandName, args...)
	cmd.Env = os.Environ()
	if commandName == "bindfs" && mountSystem == "fuse-t" {
		cmd.Env = append(cmd.Env, "LC_RPATH=/usr/local/lib")
		cmd.Env = append(cmd.Env, "fuse_CFLAGS=-I/usr/local/include/fuse -D_FILE_OFFSET_BITS=64 -D_DARWIN_C_SOURCE")
		cmd.Env = append(cmd.Env, "fuse_LIBS=-L/usr/local/lib -lfuse-t -pthread")
	}
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s [%v] failed: %q: %w", commandName, args, string(output), err)
	}

	return nil
}
