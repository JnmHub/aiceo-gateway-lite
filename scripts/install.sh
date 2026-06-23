#!/usr/bin/env bash
set -euo pipefail

REPO="${AICEO_GATEWAY_LITE_REPO:-JnmHub/aiceo-gateway-lite}"
VERSION="${AICEO_GATEWAY_LITE_VERSION:-latest}"
PORT="${AICEO_GATEWAY_LITE_PORT:-18089}"
INSTALL_ROOT="${AICEO_GATEWAY_LITE_HOME:-/opt/aiceo/gateway-lite}"
DOCKER_ROOT="${AICEO_GATEWAY_LITE_DOCKER_HOME:-/opt/aiceo/gateway-lite-docker}"
CONFIG_DIR="/etc/sub2api"
CONFIG_FILE="${CONFIG_DIR}/config.yaml"
SERVICE_NAME="aiceo-gateway-lite"
SYSTEM_USER="aiceo"

log() { printf '\033[1;32m[ok]\033[0m %s\n' "$*"; }
info() { printf '\033[1;34m[info]\033[0m %s\n' "$*"; }
warn() { printf '\033[1;33m[warn]\033[0m %s\n' "$*"; }
fail() { printf '\033[1;31m[error]\033[0m %s\n' "$*" >&2; exit 1; }

need_root() {
  if [[ "${EUID}" -ne 0 ]]; then
    fail "please run as root, for example: curl -fsSL ... | sudo bash"
  fi
}

detect_arch() {
  case "$(uname -m)" in
    x86_64|amd64) echo "amd64" ;;
    aarch64|arm64) echo "arm64" ;;
    *) fail "unsupported architecture: $(uname -m)" ;;
  esac
}

rand_secret() {
  if command -v openssl >/dev/null 2>&1; then
    openssl rand -base64 36 | tr -d '\n'
  else
    tr -dc 'A-Za-z0-9' </dev/urandom | head -c 48
  fi
}

apt_install() {
  export DEBIAN_FRONTEND=noninteractive
  apt-get update -y
  apt-get install -y "$@"
}

prompt_input() {
  local __var="$1"
  local __prompt="$2"
  local __value=""
  if [[ -r /dev/tty ]]; then
    read -r -p "${__prompt}" __value </dev/tty || true
  else
    read -r -p "${__prompt}" __value || true
  fi
  printf -v "${__var}" '%s' "${__value}"
}

yaml_quote() {
  printf "'%s'" "$(printf '%s' "$1" | sed "s/'/''/g")"
}

download_binary() {
  local target="$1"
  local arch asset url
  arch="$(detect_arch)"
  asset="aiceo-gateway-lite-linux-${arch}"
  if [[ "${VERSION}" == "latest" ]]; then
    url="https://github.com/${REPO}/releases/latest/download/${asset}"
  else
    url="https://github.com/${REPO}/releases/download/${VERSION}/${asset}"
  fi
  info "downloading ${asset} from ${url}"
  mkdir -p "$(dirname "${target}")"
  curl -fL --retry 3 --connect-timeout 15 -o "${target}" "${url}"
  chmod 0755 "${target}"
}

read_initial_admin() {
  local default_email="105626@qq.com"
  local generated_password
  generated_password="$(rand_secret | head -c 16)"
  ADMIN_EMAIL="${AICEO_GATEWAY_LITE_ADMIN_EMAIL:-}"
  ADMIN_PASSWORD="${AICEO_GATEWAY_LITE_ADMIN_PASSWORD:-}"
  if [[ -z "${ADMIN_EMAIL}" ]]; then
    prompt_input ADMIN_EMAIL "Admin email [${default_email}]: "
  fi
  ADMIN_EMAIL="${ADMIN_EMAIL:-$default_email}"
  if [[ -z "${ADMIN_PASSWORD}" ]]; then
    prompt_input ADMIN_PASSWORD "Admin password [auto generated]: "
  fi
  ADMIN_PASSWORD="${ADMIN_PASSWORD:-$generated_password}"
}

write_config() {
  local path="$1"
  local db_host="$2"
  local db_port="$3"
  local db_name="$4"
  local db_user="$5"
  local db_password="$6"
  local redis_host="$7"
  local redis_port="$8"
  local jwt_secret="$9"
  local admin_sync_key="${10}"
  local cp_token="${11}"
  local q_admin_email q_admin_password q_db_password q_jwt_secret q_admin_sync_key q_cp_token
  q_admin_email="$(yaml_quote "${ADMIN_EMAIL}")"
  q_admin_password="$(yaml_quote "${ADMIN_PASSWORD}")"
  q_db_password="$(yaml_quote "${db_password}")"
  q_jwt_secret="$(yaml_quote "${jwt_secret}")"
  q_admin_sync_key="$(yaml_quote "${admin_sync_key}")"
  q_cp_token="$(yaml_quote "${cp_token}")"

  mkdir -p "$(dirname "${path}")"
  cat > "${path}" <<YAML
run_mode: gateway-lite
timezone: Asia/Shanghai

server:
  host: 0.0.0.0
  port: ${PORT}
  mode: release

admin:
  email: ${q_admin_email}
  password: ${q_admin_password}

jwt:
  secret: ${q_jwt_secret}
  expire_hour: 24

database:
  host: ${db_host}
  port: ${db_port}
  user: ${db_user}
  password: ${q_db_password}
  dbname: ${db_name}
  sslmode: disable

redis:
  host: ${redis_host}
  port: ${redis_port}
  password: ""
  db: 0
  enable_tls: false

default:
  api_key_prefix: sk-
  user_balance: 0
  user_concurrency: 5
  rate_multiplier: 1

gateway_lite:
  gateway_code: gateway-$(hostname | tr -cd 'a-zA-Z0-9-' | tr 'A-Z' 'a-z')
  region: local
  redis_prefix: aiceo:gateway-lite
  control_plane_url: ""
  control_plane_token: ${q_cp_token}
  admin_sync_key: ${q_admin_sync_key}
  control_plane_timeout_ms: 1000
  config_sync_interval_seconds: 30
  cache_invalidation_interval_seconds: 5
  runtime_health_interval_seconds: 15
  runtime_active_window_seconds: 300
YAML
  chmod 0600 "${path}"
}

install_native() {
  info "install mode: native Ubuntu service"
  apt_install ca-certificates curl openssl postgresql postgresql-contrib redis-server

  id -u "${SYSTEM_USER}" >/dev/null 2>&1 || useradd --system --home "${INSTALL_ROOT}" --shell /usr/sbin/nologin "${SYSTEM_USER}"
  mkdir -p "${INSTALL_ROOT}" /var/lib/aiceo/gateway-lite "${CONFIG_DIR}"

  local db_name="aiceo_gateway_lite"
  local db_user="aiceo_gateway_lite"
  local db_password jwt_secret admin_sync_key cp_token
  db_password="$(rand_secret)"
  jwt_secret="$(rand_secret)"
  admin_sync_key="$(rand_secret)"
  cp_token="$(rand_secret)"

  info "configuring PostgreSQL user/database"
  local -a psql_cmd
  if command -v sudo >/dev/null 2>&1; then
    psql_cmd=(sudo -u postgres psql -v ON_ERROR_STOP=1)
  else
    psql_cmd=(runuser -u postgres -- psql -v ON_ERROR_STOP=1)
  fi
  "${psql_cmd[@]}" <<SQL
DO \$\$
BEGIN
  IF NOT EXISTS (SELECT FROM pg_catalog.pg_roles WHERE rolname = '${db_user}') THEN
    CREATE ROLE ${db_user} LOGIN PASSWORD '${db_password}';
  ELSE
    ALTER ROLE ${db_user} WITH PASSWORD '${db_password}';
  END IF;
END
\$\$;
SELECT 'CREATE DATABASE ${db_name} OWNER ${db_user}'
WHERE NOT EXISTS (SELECT FROM pg_database WHERE datname = '${db_name}')\gexec
GRANT ALL PRIVILEGES ON DATABASE ${db_name} TO ${db_user};
SQL

  read_initial_admin
  write_config "${CONFIG_FILE}" "127.0.0.1" "5432" "${db_name}" "${db_user}" "${db_password}" "127.0.0.1" "6379" "${jwt_secret}" "${admin_sync_key}" "${cp_token}"
  chown root:"${SYSTEM_USER}" "${CONFIG_FILE}"
  chmod 0640 "${CONFIG_FILE}"

  download_binary "${INSTALL_ROOT}/gateway-lite"
  chown -R "${SYSTEM_USER}:${SYSTEM_USER}" "${INSTALL_ROOT}" /var/lib/aiceo/gateway-lite

  cat > "/etc/systemd/system/${SERVICE_NAME}.service" <<UNIT
[Unit]
Description=AICEO Gateway Lite
After=network-online.target postgresql.service redis-server.service
Wants=network-online.target

[Service]
Type=simple
User=${SYSTEM_USER}
Group=${SYSTEM_USER}
WorkingDirectory=/var/lib/aiceo/gateway-lite
Environment=RUN_MODE=gateway-lite
ExecStart=${INSTALL_ROOT}/gateway-lite
Restart=always
RestartSec=3
LimitNOFILE=1048576

[Install]
WantedBy=multi-user.target
UNIT

  systemctl daemon-reload
  systemctl enable --now "${SERVICE_NAME}"
  verify_health
  print_success
}

install_docker_if_needed() {
  if command -v docker >/dev/null 2>&1 && docker compose version >/dev/null 2>&1; then
    log "Docker and Docker Compose are already installed"
    return
  fi
  apt_install ca-certificates curl git openssl
  info "installing Docker from get.docker.com"
  curl -fsSL https://get.docker.com | sh
}

install_docker_mode() {
  info "install mode: Docker Compose with release binary"
  install_docker_if_needed
  apt_install git curl openssl

  if [[ -d "${DOCKER_ROOT}/.git" ]]; then
    git -C "${DOCKER_ROOT}" pull --ff-only
  else
    rm -rf "${DOCKER_ROOT}"
    git clone "https://github.com/${REPO}.git" "${DOCKER_ROOT}"
  fi

  cd "${DOCKER_ROOT}"
  mkdir -p bin data

  local db_password jwt_secret admin_sync_key cp_token
  db_password="$(rand_secret)"
  jwt_secret="$(rand_secret)"
  admin_sync_key="$(rand_secret)"
  cp_token="$(rand_secret)"

  read_initial_admin
  cat > .env <<ENV
GATEWAY_LITE_PORT=${PORT}
POSTGRES_DB=aiceo_gateway_lite
POSTGRES_USER=aiceo
POSTGRES_PASSWORD=${db_password}
TZ=Asia/Shanghai
ENV

  local host_port="${PORT}"
  # Docker Compose maps host ${GATEWAY_LITE_PORT} to container 18089.
  # The app config inside the container must therefore keep listening on 18089.
  PORT="18089"
  write_config "data/config.yaml" "postgres" "5432" "aiceo_gateway_lite" "aiceo" "${db_password}" "redis" "6379" "${jwt_secret}" "${admin_sync_key}" "${cp_token}"
  PORT="${host_port}"
  download_binary "bin/gateway-lite"

  docker compose up -d
  verify_health
  print_success
}

verify_health() {
  info "waiting for gateway-lite health endpoint"
  for _ in $(seq 1 60); do
    if curl -fsS "http://127.0.0.1:${PORT}/health" >/dev/null 2>&1; then
      log "gateway-lite is healthy"
      return
    fi
    sleep 2
  done
  warn "health check did not pass yet. Check logs with:"
  warn "native: journalctl -u ${SERVICE_NAME} -f"
  warn "docker: cd ${DOCKER_ROOT} && docker compose logs -f gateway-lite"
}

print_success() {
  cat <<EOF

Install complete.

URL: http://SERVER_IP:${PORT}
Admin email: ${ADMIN_EMAIL}
Admin password: ${ADMIN_PASSWORD}

Please save the admin password now. It is only printed once.
EOF
}

main() {
  need_root
  if ! command -v apt-get >/dev/null 2>&1; then
    fail "this installer currently supports Ubuntu/Debian servers with apt-get"
  fi

  cat <<'MENU'
AICEO Gateway Lite installer

1) Native install: PostgreSQL + Redis + systemd service
2) Docker install: Docker Compose + PostgreSQL + Redis + release binary

MENU
  choice="${AICEO_GATEWAY_LITE_INSTALL_MODE:-}"
  if [[ -z "${choice}" ]]; then
    prompt_input choice "Choose install mode [1/2]: "
  fi
  case "${choice}" in
    1) install_native ;;
    2) install_docker_mode ;;
    *) fail "invalid choice: ${choice}" ;;
  esac
}

main "$@"
