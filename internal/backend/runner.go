package backend

import (
	"errors"
	"os"
	"os/exec"
)

// Run 执行命令并把当前终端完整交给子进程。
//
// 返回子进程的退出码。正常的非零退出码不会作为 error 返回，
// 以便调用方原样透传给上层 shell（例如远端命令以 1 结束、ssh 以 255 结束）。
//
// Windows 没有 exec 系统调用族，无法替换进程映像，因此采用句柄继承的方式。
// 关键约束：本进程在交出终端之前绝不能读取 stdin，否则会吞掉用户的输入缓冲。
func Run(exe string, args []string) (int, error) {
	cmd := exec.Command(exe, args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	err := cmd.Run()
	if err == nil {
		return 0, nil
	}

	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		if code := exitErr.ExitCode(); code >= 0 {
			return code, nil
		}
	}
	return 1, err
}
