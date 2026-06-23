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
BACKUP_ROOT="${AICEO_GATEWAY_LITE_BACKUP_ROOT:-/opt/aiceo/backups/gateway-lite}"
INSTALL_STATE_FILE="${INSTALL_ROOT}/install-state.env"
DOCKER_STATE_FILE="${DOCKER_ROOT}/install-state.env"

GENERATED_DB_PASSWORD=""
GENERATED_JWT_SECRET=""
GENERATED_ADMIN_SYNC_KEY=""
GENERATED_CONTROL_PLANE_TOKEN=""
GENERATED_CONFIG_FILE=""

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

read_env_value() {
  local file="$1"
  local key="$2"
  if [[ -f "${file}" ]]; then
    awk -F= -v k="${key}" '$1 == k { sub(/^[^=]*=/, ""); print; exit }' "${file}"
  fi
}

latest_version() {
  local effective
  effective="$(curl -fsSL -o /dev/null -w '%{url_effective}' "https://github.com/${REPO}/releases/latest" || true)"
  basename "${effective:-}"
}

desired_version() {
  local desired
  if [[ "${VERSION}" == "latest" ]]; then
    desired="$(latest_version)"
  else
    desired="${VERSION}"
  fi
  if [[ -z "${desired}" || "${desired}" == "." ]]; then
    desired="unknown"
  fi
  printf '%s' "${desired}"
}

normalize_version() {
  printf '%s' "$1" | sed 's/^v//'
}

version_lt() {
  local left right first
  left="$(normalize_version "$1")"
  right="$(normalize_version "$2")"
  if [[ -z "${left}" || -z "${right}" || "${left}" == "${right}" ]]; then
    return 1
  fi
  first="$(printf '%s\n%s\n' "${left}" "${right}" | sort -V | head -n 1)"
  [[ "${first}" == "${left}" ]]
}

binary_version() {
  local bin="$1"
  local out
  if [[ -x "${bin}" ]]; then
    out="$("${bin}" --version 2>&1 || true)"
    printf '%s\n' "${out}" | grep -Eo 'v[0-9]+(\.[0-9]+){1,2}([^ ]*)?' | head -n 1 ||
      printf '%s\n' "${out}" | grep -Eo '[0-9]+(\.[0-9]+){1,2}([^ ]*)?' | head -n 1 ||
      true
  fi
}

native_installed() {
  [[ -x "${INSTALL_ROOT}/gateway-lite" || -f "${CONFIG_FILE}" || -f "/etc/systemd/system/${SERVICE_NAME}.service" ]]
}

docker_installed() {
  [[ -f "${DOCKER_ROOT}/docker-compose.yml" || -f "${DOCKER_ROOT}/.env" || -f "${DOCKER_ROOT}/data/config.yaml" ]]
}

installed_version() {
  local mode="$1"
  local state_file bin version
  if [[ "${mode}" == "native" ]]; then
    state_file="${INSTALL_STATE_FILE}"
    bin="${INSTALL_ROOT}/gateway-lite"
  else
    state_file="${DOCKER_STATE_FILE}"
    bin="${DOCKER_ROOT}/bin/gateway-lite"
  fi
  version="$(read_env_value "${state_file}" "VERSION")"
  version="${version:-$(binary_version "${bin}")}"
  printf '%s' "${version:-unknown}"
}

write_install_state() {
  local mode="$1"
  local state_file installed
  installed="$(desired_version)"
  if [[ "${mode}" == "native" ]]; then
    state_file="${INSTALL_STATE_FILE}"
  else
    state_file="${DOCKER_STATE_FILE}"
  fi
  mkdir -p "$(dirname "${state_file}")"
  cat > "${state_file}" <<STATE
MODE=${mode}
REPO=${REPO}
VERSION=${installed}
UPDATED_AT=$(date -u +%Y-%m-%dT%H:%M:%SZ)
STATE
  chmod 0600 "${state_file}"
}

download_binary() {
  local target="$1"
  local arch asset url tmp
  arch="$(detect_arch)"
  asset="aiceo-gateway-lite-linux-${arch}"
  if [[ "${VERSION}" == "latest" ]]; then
    url="https://github.com/${REPO}/releases/latest/download/${asset}"
  else
    url="https://github.com/${REPO}/releases/download/${VERSION}/${asset}"
  fi
  info "downloading ${asset} from ${url}"
  mkdir -p "$(dirname "${target}")"
  tmp="${target}.tmp.$$"
  rm -f "${tmp}"
  curl -fL --retry 3 --connect-timeout 15 -o "${tmp}" "${url}"
  chmod 0755 "${tmp}"
  mv -f "${tmp}" "${target}"
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
  GENERATED_DB_PASSWORD="${db_password}"
  GENERATED_JWT_SECRET="${jwt_secret}"
  GENERATED_ADMIN_SYNC_KEY="${admin_sync_key}"
  GENERATED_CONTROL_PLANE_TOKEN="${cp_token}"
  GENERATED_CONFIG_FILE="${CONFIG_FILE}"

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
  systemctl enable "${SERVICE_NAME}"
  systemctl restart "${SERVICE_NAME}"
  write_install_state "native"
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
  db_password="$(read_env_value ".env" "POSTGRES_PASSWORD")"
  db_password="${db_password:-$(rand_secret)}"
  jwt_secret="$(rand_secret)"
  admin_sync_key="$(rand_secret)"
  cp_token="$(rand_secret)"
  GENERATED_DB_PASSWORD="${db_password}"
  GENERATED_JWT_SECRET="${jwt_secret}"
  GENERATED_ADMIN_SYNC_KEY="${admin_sync_key}"
  GENERATED_CONTROL_PLANE_TOKEN="${cp_token}"
  GENERATED_CONFIG_FILE="${DOCKER_ROOT}/data/config.yaml"

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

  docker compose up -d postgres redis
  docker compose up -d --force-recreate gateway-lite
  write_install_state "docker"
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
Config file: ${GENERATED_CONFIG_FILE}

Generated secrets:
Database password: ${GENERATED_DB_PASSWORD}
JWT secret: ${GENERATED_JWT_SECRET}
Gateway admin sync key: ${GENERATED_ADMIN_SYNC_KEY}
Control plane token: ${GENERATED_CONTROL_PLANE_TOKEN}

Please save these credentials now. They are only printed after installation.
EOF
}

backup_native() {
  local ts dest archive
  ts="$(date -u +%Y%m%dT%H%M%SZ)"
  dest="${BACKUP_ROOT}/native-${ts}"
  archive="${dest}.tar.gz"
  mkdir -p "${dest}"
  [[ -f "${CONFIG_FILE}" ]] && cp -a "${CONFIG_FILE}" "${dest}/config.yaml"
  [[ -f "/etc/systemd/system/${SERVICE_NAME}.service" ]] && cp -a "/etc/systemd/system/${SERVICE_NAME}.service" "${dest}/aiceo-gateway-lite.service"
  [[ -x "${INSTALL_ROOT}/gateway-lite" ]] && cp -a "${INSTALL_ROOT}/gateway-lite" "${dest}/gateway-lite"
  if command -v pg_dump >/dev/null 2>&1; then
    if command -v sudo >/dev/null 2>&1; then
      sudo -u postgres pg_dump -Fc aiceo_gateway_lite > "${dest}/postgres.dump" 2>/dev/null || warn "native PostgreSQL dump failed; config and binary were still backed up"
    else
      runuser -u postgres -- pg_dump -Fc aiceo_gateway_lite > "${dest}/postgres.dump" 2>/dev/null || warn "native PostgreSQL dump failed; config and binary were still backed up"
    fi
  fi
  tar -C "${dest}" -czf "${archive}" .
  rm -rf "${dest}"
  log "backup created: ${archive}"
}

backup_docker() {
  local ts dest archive db_name db_user
  ts="$(date -u +%Y%m%dT%H%M%SZ)"
  dest="${BACKUP_ROOT}/docker-${ts}"
  archive="${dest}.tar.gz"
  mkdir -p "${dest}"
  if [[ -d "${DOCKER_ROOT}" ]]; then
    [[ -f "${DOCKER_ROOT}/.env" ]] && cp -a "${DOCKER_ROOT}/.env" "${dest}/env"
    [[ -f "${DOCKER_ROOT}/docker-compose.yml" ]] && cp -a "${DOCKER_ROOT}/docker-compose.yml" "${dest}/docker-compose.yml"
    [[ -d "${DOCKER_ROOT}/data" ]] && cp -a "${DOCKER_ROOT}/data" "${dest}/data"
    [[ -f "${DOCKER_ROOT}/bin/gateway-lite" ]] && cp -a "${DOCKER_ROOT}/bin/gateway-lite" "${dest}/gateway-lite"
    db_name="$(read_env_value "${DOCKER_ROOT}/.env" "POSTGRES_DB")"
    db_user="$(read_env_value "${DOCKER_ROOT}/.env" "POSTGRES_USER")"
    db_name="${db_name:-aiceo_gateway_lite}"
    db_user="${db_user:-aiceo}"
    if command -v docker >/dev/null 2>&1; then
      (cd "${DOCKER_ROOT}" && docker compose exec -T postgres pg_dump -U "${db_user}" "${db_name}" > "${dest}/postgres.sql") 2>/dev/null || warn "docker PostgreSQL dump failed; files were still backed up"
    fi
  fi
  tar -C "${dest}" -czf "${archive}" .
  rm -rf "${dest}"
  log "backup created: ${archive}"
}

backup_install() {
  case "$1" in
    native) backup_native ;;
    docker) backup_docker ;;
    *) fail "unknown install target: $1" ;;
  esac
}

upgrade_native() {
  info "upgrading native installation"
  backup_native
  download_binary "${INSTALL_ROOT}/gateway-lite"
  chown "${SYSTEM_USER}:${SYSTEM_USER}" "${INSTALL_ROOT}/gateway-lite" 2>/dev/null || true
  systemctl daemon-reload
  systemctl restart "${SERVICE_NAME}"
  write_install_state "native"
  verify_health
  log "native installation upgraded"
}

upgrade_docker() {
  info "upgrading Docker installation"
  backup_docker
  install_docker_if_needed
  apt_install git curl openssl
  if [[ ! -d "${DOCKER_ROOT}/.git" ]]; then
    fail "Docker installation directory is missing git metadata: ${DOCKER_ROOT}"
  fi
  git -C "${DOCKER_ROOT}" pull --ff-only
  cd "${DOCKER_ROOT}"
  if [[ -n "${AICEO_GATEWAY_LITE_PORT:-}" ]]; then
    local db_password db_name db_user tz
    db_password="$(read_env_value ".env" "POSTGRES_PASSWORD")"
    db_name="$(read_env_value ".env" "POSTGRES_DB")"
    db_user="$(read_env_value ".env" "POSTGRES_USER")"
    tz="$(read_env_value ".env" "TZ")"
    cat > .env <<ENV
GATEWAY_LITE_PORT=${PORT}
POSTGRES_DB=${db_name:-aiceo_gateway_lite}
POSTGRES_USER=${db_user:-aiceo}
POSTGRES_PASSWORD=${db_password}
TZ=${tz:-Asia/Shanghai}
ENV
  fi
  download_binary "bin/gateway-lite"
  docker compose up -d postgres redis
  docker compose up -d --force-recreate gateway-lite
  write_install_state "docker"
  verify_health
  log "Docker installation upgraded"
}

upgrade_install() {
  case "$1" in
    native) upgrade_native ;;
    docker) upgrade_docker ;;
    *) fail "unknown install target: $1" ;;
  esac
}

uninstall_native() {
  local purge="$1"
  info "uninstalling native installation"
  backup_native
  systemctl disable --now "${SERVICE_NAME}" >/dev/null 2>&1 || true
  rm -f "/etc/systemd/system/${SERVICE_NAME}.service"
  systemctl daemon-reload || true
  rm -rf "${INSTALL_ROOT}"
  rm -f "${CONFIG_FILE}"
  if [[ "${purge}" == "1" ]]; then
    rm -rf /var/lib/aiceo/gateway-lite
    if command -v psql >/dev/null 2>&1; then
      if command -v sudo >/dev/null 2>&1; then
        sudo -u postgres psql -v ON_ERROR_STOP=0 -c "DROP DATABASE IF EXISTS aiceo_gateway_lite;" -c "DROP ROLE IF EXISTS aiceo_gateway_lite;" >/dev/null 2>&1 || true
      else
        runuser -u postgres -- psql -v ON_ERROR_STOP=0 -c "DROP DATABASE IF EXISTS aiceo_gateway_lite;" -c "DROP ROLE IF EXISTS aiceo_gateway_lite;" >/dev/null 2>&1 || true
      fi
    fi
  fi
  log "native installation uninstalled"
}

uninstall_docker() {
  local purge="$1"
  info "uninstalling Docker installation"
  backup_docker
  if [[ -d "${DOCKER_ROOT}" ]]; then
    if [[ "${purge}" == "1" ]]; then
      (cd "${DOCKER_ROOT}" && docker compose down -v) || true
    else
      (cd "${DOCKER_ROOT}" && docker compose down) || true
    fi
    rm -rf "${DOCKER_ROOT}"
  fi
  log "Docker installation uninstalled"
}

uninstall_install() {
  local mode="$1"
  local purge="${AICEO_GATEWAY_LITE_PURGE:-}"
  if [[ -z "${purge}" ]]; then
    local keep_data="${AICEO_GATEWAY_LITE_KEEP_DATA:-}"
    if [[ -z "${keep_data}" ]]; then
      prompt_input keep_data "Keep database/volumes after backup? [y/N]: "
    fi
    case "${keep_data}" in
      y|Y|yes|YES) purge="0" ;;
      *) purge="1" ;;
    esac
  fi
  case "${mode}" in
    native) uninstall_native "${purge}" ;;
    docker) uninstall_docker "${purge}" ;;
    *) fail "unknown install target: ${mode}" ;;
  esac
}

choose_existing_mode() {
  local target="${AICEO_GATEWAY_LITE_TARGET:-}"
  if [[ -n "${target}" ]]; then
    case "${target}" in
      native)
        native_installed || fail "target native is not installed"
        ;;
      docker)
        docker_installed || fail "target docker is not installed"
        ;;
      *)
        fail "invalid target: ${target}"
        ;;
    esac
    printf '%s' "${target}"
    return
  fi
  if [[ "${AICEO_GATEWAY_LITE_INSTALL_MODE:-}" == "1" ]]; then
    if native_installed; then
      printf 'native'
      return
    fi
    fail "another installation mode already exists; refusing to create duplicate native installation"
  fi
  if [[ "${AICEO_GATEWAY_LITE_INSTALL_MODE:-}" == "2" ]]; then
    if docker_installed; then
      printf 'docker'
      return
    fi
    fail "another installation mode already exists; refusing to create duplicate Docker installation"
  fi
  if native_installed && ! docker_installed; then
    printf 'native'
    return
  fi
  if docker_installed && ! native_installed; then
    printf 'docker'
    return
  fi
  cat >&2 <<'MENU'
Existing installations found:

1) Native systemd installation
2) Docker Compose installation

MENU
  prompt_input target "Choose target [1/2]: "
  case "${target}" in
    1|native) printf 'native' ;;
    2|docker) printf 'docker' ;;
    *) fail "invalid target: ${target}" ;;
  esac
}

handle_existing_install() {
  local mode current desired action has_update
  mode="$(choose_existing_mode)"
  current="$(installed_version "${mode}")"
  desired="$(desired_version)"
  has_update="0"
  if [[ "${desired}" != "unknown" ]]; then
    if [[ "${current}" == "unknown" ]] || version_lt "${current}" "${desired}"; then
      has_update="1"
    fi
  fi

  info "existing ${mode} installation detected. current=${current}, target=${desired}"
  action="${AICEO_GATEWAY_LITE_ACTION:-}"
  if [[ -z "${action}" ]]; then
    if [[ "${has_update}" == "1" ]]; then
      cat <<'MENU'
Maintenance actions:

1) Upgrade/overwrite existing installation
2) Backup now
3) Uninstall
4) Exit

MENU
      prompt_input action "Choose action [1/2/3/4]: "
    else
      cat <<'MENU'
No upgrade is available.

1) Backup now
2) Uninstall
3) Exit

MENU
      prompt_input action "Choose action [1/2/3]: "
      case "${action}" in
        1) action="backup" ;;
        2) action="uninstall" ;;
        3) action="exit" ;;
      esac
    fi
  fi

  case "${action}" in
    1|upgrade)
      if [[ "${has_update}" != "1" ]]; then
        fail "already up to date; refusing to reinstall over the same version"
      fi
      upgrade_install "${mode}"
      ;;
    2|backup) backup_install "${mode}" ;;
    3|uninstall) uninstall_install "${mode}" ;;
    4|exit) log "no changes made" ;;
    *) fail "invalid action: ${action}" ;;
  esac
}

main() {
  need_root
  if ! command -v apt-get >/dev/null 2>&1; then
    fail "this installer currently supports Ubuntu/Debian servers with apt-get"
  fi

  if native_installed || docker_installed; then
    handle_existing_install
    return
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
