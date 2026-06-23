# AICEO Gateway Lite

AICEO Gateway Lite（轻量网关）是一个可独立部署的 AI API gateway（网关）运行时，提供后台页面、上游账号管理、模型转发、并发控制、用量统计和主站同步能力。

## 一键安装

Ubuntu/Debian 服务器执行：

```bash
curl -fsSL https://raw.githubusercontent.com/JnmHub/aiceo-gateway-lite/main/scripts/install.sh | sudo bash
```

脚本会让你输入编号选择安装方式：

1. 本机安装：自动安装 PostgreSQL、Redis 和 gateway-lite systemd 服务。
2. Docker 安装：自动安装 Docker，克隆本仓库，下载 GitHub Release 二进制，然后用 Docker Compose 启动 PostgreSQL、Redis 和 gateway-lite。

Docker 方式不会在用户服务器上编译前端或 Go 二进制。

再次执行同一条命令时，脚本会先检测已有安装，不会重复初始化第二套服务：

- 有新版本：可选择升级覆盖、立即备份、卸载或退出。
- 没有新版本：可选择立即备份、卸载或退出；脚本会拒绝用同版本重复覆盖安装。
- 指定 `AICEO_GATEWAY_LITE_VERSION` 时，升级判断会以指定版本为目标；当前版本相同则拒绝重复覆盖。
- 卸载前会自动做一次备份，默认清理数据库/容器 volume；如需保留数据，可按提示选择保留。

## 可选环境变量

```bash
curl -fsSL https://raw.githubusercontent.com/JnmHub/aiceo-gateway-lite/main/scripts/install.sh \
  | sudo env AICEO_GATEWAY_LITE_INSTALL_MODE=2 AICEO_GATEWAY_LITE_PORT=18089 AICEO_GATEWAY_LITE_VERSION=latest bash
```

- `AICEO_GATEWAY_LITE_INSTALL_MODE`：安装模式，`1` 为本机安装，`2` 为 Docker 安装；不填时会提示输入编号。
- `AICEO_GATEWAY_LITE_PORT`：对外 HTTP 端口，默认 `18089`。
- `AICEO_GATEWAY_LITE_VERSION`：安装版本，默认 `latest`，也可以指定 `v0.1.0`。
- `AICEO_GATEWAY_LITE_HOME`：本机安装目录，默认 `/opt/aiceo/gateway-lite`。
- `AICEO_GATEWAY_LITE_DOCKER_HOME`：Docker 安装目录，默认 `/opt/aiceo/gateway-lite-docker`。
- `AICEO_GATEWAY_LITE_ADMIN_EMAIL`：初始管理员邮箱，默认 `105626@qq.com`。
- `AICEO_GATEWAY_LITE_ADMIN_PASSWORD`：初始管理员密码，不填时自动生成并在安装完成后打印。
- `AICEO_GATEWAY_LITE_ACTION`：维护动作，可选 `upgrade`、`backup`、`uninstall`、`exit`。
- `AICEO_GATEWAY_LITE_TARGET`：已有多种安装记录时指定维护目标，可选 `native`、`docker`。
- `AICEO_GATEWAY_LITE_PURGE`：卸载时是否清理数据库/volume，`1` 清理，`0` 保留。
- `AICEO_GATEWAY_LITE_BACKUP_ROOT`：备份目录，默认 `/opt/aiceo/backups/gateway-lite`。

非交互备份示例：

```bash
curl -fsSL https://raw.githubusercontent.com/JnmHub/aiceo-gateway-lite/main/scripts/install.sh \
  | sudo env AICEO_GATEWAY_LITE_INSTALL_MODE=2 AICEO_GATEWAY_LITE_ACTION=backup bash
```

## 安装后访问

安装完成后访问：

```text
http://SERVER_IP:18089
```

后台账号、后台密码、数据库密码、JWT secret、主站通信 token 和网关管理员同步 key 会在安装脚本结束时打印。默认邮箱是 `105626@qq.com`，密码默认随机生成。

## 运维命令

本机安装：

```bash
systemctl status aiceo-gateway-lite
journalctl -u aiceo-gateway-lite -f
systemctl restart aiceo-gateway-lite
```

Docker 安装：

```bash
cd /opt/aiceo/gateway-lite-docker
docker compose ps
docker compose logs -f gateway-lite
docker compose restart gateway-lite
```

## 配置文件

本机安装：

```text
/etc/sub2api/config.yaml
```

Docker 安装：

```text
/opt/aiceo/gateway-lite-docker/data/config.yaml
```

修改配置后重启服务生效。

## Release

GitHub Actions 会在推送 `v*` tag 或手动触发工作流时发布二进制：

- `aiceo-gateway-lite-linux-amd64`
- `aiceo-gateway-lite-linux-arm64`

安装脚本默认从 `latest` release 下载对应架构的二进制。
