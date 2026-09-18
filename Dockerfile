# YunSSH 隔离开发与验证环境
#
# 用途：在 Linux 容器中运行单元测试与端到端验证。
#
# 为什么需要容器：
#   OpenSSH 通过 passwd 数据库中的 pw_dir 定位 ~/.ssh/config，
#   **不读 $HOME / %USERPROFILE% 环境变量**，因此无法用环境变量把 ssh
#   重定向到临时目录（Windows 上实测会静默返回系统默认值，看着合理但完全错误）。
#   隔离 ssh 的配置路径只能靠容器：独立文件系统 + 独立 passwd 数据库。
#   端到端测试因此既能使用真实的 ssh，又不触碰任何真实配置。
#
# 用法：
#   docker build -t yssh-dev .
#   docker run --rm yssh-dev
FROM golang:1.27-alpine

# Alpine 官方包源在部分网络下不可达，默认改用阿里云镜像。
# 如需切回官方源：docker build --build-arg ALPINE_MIRROR=dl-cdn.alpinelinux.org .
ARG ALPINE_MIRROR=mirrors.aliyun.com

# openssh-client 供端到端验证使用；bash 是验证脚本的运行时依赖
# （脚本用到 set -o pipefail 与 $'\x1b' 这类 bash 特性，busybox sh 不支持）。
RUN sed -i "s|dl-cdn.alpinelinux.org|${ALPINE_MIRROR}|g" /etc/apk/repositories \
    && apk add --no-cache openssh-client bash

WORKDIR /src

# 本项目零第三方依赖，无需 go mod download。
COPY . .

RUN go build -trimpath -o /usr/local/bin/yssh ./cmd/yssh

CMD ["bash", "scripts/ci.sh"]
