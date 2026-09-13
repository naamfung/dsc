// Copyright IBM Corp. 2016, 2025
// SPDX-License-Identifier: MPL-2.0

package cmdrunner

import (
        "context"
        "fmt"
        "net"
        "os"

        "github.com/hashicorp/go-plugin/runner"
)

// ReattachFunc returns a function that allows reattaching to a core running
// as a plain process. The process may or may not be a child process.
func ReattachFunc(pid int, addr net.Addr) runner.ReattachFunc {
        return func() (runner.AttachedRunner, error) {
                p, err := os.FindProcess(pid)
                if err != nil {
                        // On Unix systems, FindProcess never returns an error.
                        // On Windows, for non-existent pids it returns:
                        // os.SyscallError - 'OpenProcess: the paremter is incorrect'
                        return nil, ErrProcessNotFound
                }

                // Attempt to connect to the addr since on Unix systems FindProcess
                // doesn't actually return an error if it can't find the process.
                conn, err := net.Dial(addr.Network(), addr.String())
                if err != nil {
                        return nil, ErrProcessNotFound
                }
                _ = conn.Close()

                return &CmdAttachedRunner{
                        pid:     pid,
                        process: p,
                }, nil
        }
}

// CmdAttachedRunner is mostly a subset of CmdRunner, except the Wait function
// does not assume the process is a child of the host process, and so uses a
// different implementation to wait on the process.
type CmdAttachedRunner struct {
        pid     int
        process *os.Process

        addrTranslator
}

func (c *CmdAttachedRunner) Wait(_ context.Context) error {
        return pidWait(c.pid)
}

func (c *CmdAttachedRunner) Kill(_ context.Context) error {
        return c.process.Kill()
}

// GracefulKill 实现 runner.GracefulRunner 接口：发 SIGTERM 给 reattach 进程。
//
// 与 CmdRunner.GracefulKill 同语义——reattach 进程可能是非子进程，但 SIGTERM 仍可送达
// （只要调用方有权限）。Windows 上为 no-op。
func (c *CmdAttachedRunner) GracefulKill() error {
        return sendTermination(c.process)
}

func (c *CmdAttachedRunner) ID() string {
        return fmt.Sprintf("%d", c.pid)
}
