// Package backend 负责把「一个主机条目」翻译成「一条可直接执行的命令」。
//
// 设计要点：
//   - Build 只产出可执行文件与参数，不负责执行；执行统一交给 Run，
//     这样终端接管的逻辑只有一份实现
//   - 后端可插拔：将来接入密码层或自研 SSH 客户端时，只需新增实现并调整 Select
package backend

// Request 描述一次连接请求。
type Request struct {
	Alias     string
	HostName  string
	User      string
	Port      int
	Env       string   // 环境标识，用于终端标题与色带
	ExtraArgs []string // 用户透传给底层客户端的参数
}

// Backend 把一个连接请求翻译成可执行命令。
type Backend interface {
	// Name 返回后端名称，用于展示与诊断。
	Name() string
	// Build 返回可执行文件路径与参数列表。
	Build(req Request) (exe string, args []string, err error)
}

// Select 选择本次连接要使用的后端。
//
// 当前版本（不做密码）恒定使用系统 ssh 客户端，认证方式由 ~/.ssh/config
// 与用户的密钥决定。后续接入密码层时，这里会根据凭据可用性在 ssh / plink
// 之间切换，并在需要密码但后端不可用时明确报错而不是静默降级。
func Select(req Request) (Backend, error) {
	return SSH{}, nil
}
