#!/bin/bash
set -e

# Colors
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
RED='\033[0;31m'
NC='\033[0m'

REPO="https://raw.githubusercontent.com/tanviet12/chat-quality-agent/main"
CQA_DIR="/opt/cqa"

echo -e "${GREEN}========================================${NC}"
echo -e "${GREEN}  Chat Quality Agent (CQA) Installer    ${NC}"
echo -e "${GREEN}========================================${NC}"
echo ""

# Check root
if [ "$EUID" -ne 0 ]; then
  echo -e "${RED}Vui long chay voi quyen root (sudo)${NC}"
  exit 1
fi

# Detect OS
detect_os() {
  if [ -f /etc/os-release ]; then
    . /etc/os-release
    OS_ID="$ID"
    OS_ID_LIKE="$ID_LIKE"
  else
    OS_ID="unknown"
    OS_ID_LIKE=""
  fi
}

# Install Docker
install_docker() {
  if command -v docker &> /dev/null; then
    echo -e "${GREEN}Docker da co san.${NC}"
    return
  fi

  detect_os
  echo -e "${YELLOW}Dang cai Docker cho ${OS_ID}...${NC}"

  case "$OS_ID" in
    ubuntu|debian)
      apt-get update -qq
      apt-get install -y -qq ca-certificates curl gnupg
      install -m 0755 -d /etc/apt/keyrings
      curl -fsSL "https://download.docker.com/linux/${OS_ID}/gpg" | gpg --dearmor -o /etc/apt/keyrings/docker.gpg
      chmod a+r /etc/apt/keyrings/docker.gpg
      echo "deb [arch=$(dpkg --print-architecture) signed-by=/etc/apt/keyrings/docker.gpg] https://download.docker.com/linux/${OS_ID} $(. /etc/os-release && echo "$VERSION_CODENAME") stable" > /etc/apt/sources.list.d/docker.list
      apt-get update -qq
      DEBIAN_FRONTEND=noninteractive apt-get install -y -qq docker-ce docker-ce-cli containerd.io docker-compose-plugin
      ;;
    centos|rhel|rocky|almalinux|ol)
      yum install -y yum-utils
      yum-config-manager --add-repo https://download.docker.com/linux/centos/docker-ce.repo
      yum install -y docker-ce docker-ce-cli containerd.io docker-compose-plugin
      ;;
    fedora)
      dnf install -y dnf-plugins-core
      dnf config-manager --add-repo https://download.docker.com/linux/fedora/docker-ce.repo
      dnf install -y docker-ce docker-ce-cli containerd.io docker-compose-plugin
      ;;
    amzn)
      # Amazon Linux 2
      amazon-linux-extras install docker -y 2>/dev/null || yum install -y docker
      # Docker Compose plugin
      mkdir -p /usr/local/lib/docker/cli-plugins
      curl -fsSL "https://github.com/docker/compose/releases/latest/download/docker-compose-linux-$(uname -m)" -o /usr/local/lib/docker/cli-plugins/docker-compose
      chmod +x /usr/local/lib/docker/cli-plugins/docker-compose
      ;;
    sles|opensuse*)
      zypper install -y docker docker-compose
      ;;
    arch|manjaro)
      pacman -Sy --noconfirm docker docker-compose
      ;;
    *)
      # Fallback: try ID_LIKE
      if echo "$OS_ID_LIKE" | grep -qE "rhel|centos|fedora"; then
        echo -e "${YELLOW}Phat hien distro tuong tu RHEL, thu cai bang yum...${NC}"
        yum install -y yum-utils 2>/dev/null || true
        yum-config-manager --add-repo https://download.docker.com/linux/centos/docker-ce.repo 2>/dev/null || true
        yum install -y docker-ce docker-ce-cli containerd.io docker-compose-plugin
      elif echo "$OS_ID_LIKE" | grep -qE "debian|ubuntu"; then
        echo -e "${YELLOW}Phat hien distro tuong tu Debian, thu cai bang apt...${NC}"
        apt-get update -qq
        DEBIAN_FRONTEND=noninteractive apt-get install -y -qq docker.io docker-compose-plugin 2>/dev/null || DEBIAN_FRONTEND=noninteractive apt-get install -y -qq docker.io
      else
        echo -e "${RED}Khong ho tro tu dong cai Docker cho: ${OS_ID}${NC}"
        echo -e "${YELLOW}Vui long cai Docker thu cong: https://docs.docker.com/engine/install/${NC}"
        exit 1
      fi
      ;;
  esac

  systemctl enable docker 2>/dev/null || true
  systemctl start docker 2>/dev/null || true
  echo -e "${GREEN}Docker da cai xong.${NC}"
}

# Install Docker Compose plugin if not present
install_compose() {
  if docker compose version &> /dev/null; then
    return
  fi

  echo -e "${YELLOW}Dang cai Docker Compose plugin...${NC}"

  # Try package manager first
  detect_os
  case "$OS_ID" in
    ubuntu|debian)
      DEBIAN_FRONTEND=noninteractive apt-get install -y -qq docker-compose-plugin 2>/dev/null && return
      ;;
    centos|rhel|rocky|almalinux|ol|fedora)
      yum install -y docker-compose-plugin 2>/dev/null && return
      ;;
  esac

  # Fallback: download binary
  COMPOSE_URL="https://github.com/docker/compose/releases/latest/download/docker-compose-linux-$(uname -m)"
  mkdir -p /usr/local/lib/docker/cli-plugins
  curl -fsSL "$COMPOSE_URL" -o /usr/local/lib/docker/cli-plugins/docker-compose
  chmod +x /usr/local/lib/docker/cli-plugins/docker-compose

  if docker compose version &> /dev/null; then
    echo -e "${GREEN}Docker Compose da cai xong.${NC}"
  else
    echo -e "${RED}Khong cai duoc Docker Compose. Vui long cai thu cong.${NC}"
    exit 1
  fi
}

# Main
install_docker
install_compose

# Create directory
echo -e "${YELLOW}Tao thu muc $CQA_DIR...${NC}"
mkdir -p "$CQA_DIR"
cd "$CQA_DIR"

# Download docker-compose
echo -e "${YELLOW}Tai docker-compose.hub.yml...${NC}"
curl -sfL "$REPO/docker-compose.hub.yml" -o docker-compose.yml

# Generate random secrets
generate_secret() {
  openssl rand -hex "$1"
}

# Create .env file (only if not exists — don't overwrite existing config)
if [ -f .env ]; then
  echo -e "${YELLOW}.env da ton tai, giu nguyen cau hinh hien tai.${NC}"
else
  JWT_SECRET=$(generate_secret 32)
  ENCRYPTION_KEY=$(generate_secret 16)
  DB_PASSWORD=$(generate_secret 16)
  MYSQL_ROOT_PASSWORD=$(generate_secret 16)

  cat > .env <<EOF
# Server
SERVER_PORT=8080
SERVER_HOST=0.0.0.0
APP_ENV=production

# Database
DB_HOST=db
DB_PORT=3306
DB_USER=cqa
DB_PASSWORD=${DB_PASSWORD}
DB_NAME=cqa
MYSQL_ROOT_PASSWORD=${MYSQL_ROOT_PASSWORD}

# Security
JWT_SECRET=${JWT_SECRET}
ENCRYPTION_KEY=${ENCRYPTION_KEY}

# Rate Limiting
RATE_LIMIT_PER_IP=500
RATE_LIMIT_PER_USER=1000

# SSL (de trong neu khong can SSL)
# LEGO_DOMAIN=cqa.yourdomain.com
# LEGO_EMAIL=admin@yourdomain.com
EOF

  echo -e "${GREEN}.env da tao voi secret ngau nhien.${NC}"
fi

# Pull and start services
echo -e "${YELLOW}Dang khoi dong CQA...${NC}"
docker compose pull
docker compose up -d

# Wait for health
echo -e "${YELLOW}Cho services san sang...${NC}"
for i in {1..30}; do
  if curl -sf http://localhost/health > /dev/null 2>&1; then
    echo -e "${GREEN}CQA da chay!${NC}"
    break
  fi
  sleep 2
done

IP=$(hostname -I 2>/dev/null | awk '{print $1}' || echo "localhost")

echo ""
echo -e "${GREEN}========================================${NC}"
echo -e "${GREEN}  Cai dat thanh cong!                   ${NC}"
echo -e "${GREEN}========================================${NC}"
echo ""
echo -e "  URL: http://${IP}"
echo -e "  Mo trinh duyet va tao tai khoan admin."
echo ""
echo -e "${YELLOW}  Cau hinh: ${CQA_DIR}/.env${NC}"
echo -e "${YELLOW}  Xem log:  cd ${CQA_DIR} && docker compose logs -f${NC}"
echo ""
