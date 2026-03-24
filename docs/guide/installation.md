# Cài đặt

## Yêu cầu hệ thống

| | Tối thiểu | Khuyến nghị (10-50 kênh) |
|---|---|---|
| CPU | 1 vCPU | 2 vCPU |
| RAM | 1 GB | 2 GB |
| Ổ cứng | 10 GB | 20 GB |
| OS | Ubuntu 20.04+ / Debian 11+ / AlmaLinux 8+ | Ubuntu 22.04 LTS |

Yêu cầu: **Docker** và **Docker Compose** (script cài tự động sẽ cài nếu chưa có).

Hỗ trợ macOS và Windows (qua Docker Desktop) nếu muốn chạy trên máy cá nhân.

## Cài đặt trên VPS

Có 2 cách cài đặt CQA. Khuyến nghị dùng cách 1 (tự động) cho đơn giản nhất.

## Cách 1: Cài tự động (khuyến nghị)

Chỉ cần 1 lệnh. Script sẽ tự cài Docker (nếu chưa có), tạo secrets ngẫu nhiên, pull images và khởi chạy.

```bash
curl -s https://raw.githubusercontent.com/tanviet12/chat-quality-agent/main/install.sh | sudo bash
```

Sau khi chạy xong, bạn sẽ thấy:

```
========================================
  Cài đặt thành công!
========================================
  URL: http://<IP-VPS>
  Mở trình duyệt và tạo tài khoản admin.
  Cấu hình: /opt/cqa/.env
  Xem log:  cd /opt/cqa && docker compose logs -f
```

Mở trình duyệt, truy cập `http://<IP-VPS>` — bạn sẽ thấy trang **Thiết lập ban đầu** để tạo tài khoản admin.

## Cách 2: Build từ source

Dùng cách này nếu bạn muốn tùy chỉnh code.

```bash
git clone https://github.com/tanviet12/chat-quality-agent.git
cd chat-quality-agent
cp .env.example .env
```

Mở file `.env`, điền các giá trị bắt buộc:

```bash
# Tạo secrets ngẫu nhiên
DB_PASSWORD=$(openssl rand -hex 16)
MYSQL_ROOT_PASSWORD=$(openssl rand -hex 16)
JWT_SECRET=$(openssl rand -hex 32)
ENCRYPTION_KEY=$(openssl rand -hex 16)
```

Chạy:

```bash
docker compose up -d --build
```

Truy cập:
- Nếu trên VPS: `http://<IP-VPS>`
- Nếu trên máy local: `http://localhost`

Lần đầu sẽ hiện trang Setup để tạo tài khoản admin.

## Kiểm tra trạng thái

```bash
cd /opt/cqa  # hoặc thư mục cài đặt
docker compose ps
```

Kết quả bình thường:

```
NAME        STATUS         PORTS
cqa-app     Up             0.0.0.0:8080->8080/tcp
cqa-db      Up (healthy)   127.0.0.1:3306->3306/tcp
cqa-nginx   Up             0.0.0.0:80->80/tcp
```

## Xem log

```bash
docker compose logs -f        # Xem tất cả
docker compose logs app -f    # Chỉ xem app
docker compose logs nginx -f  # Chỉ xem nginx
```

## Cập nhật phiên bản mới

```bash
cd /opt/cqa
docker compose pull
docker compose up -d
```

## Tự động cập nhật (tùy chọn)

Thêm [Watchtower](https://containrrr.dev/watchtower/) để VPS tự động pull image mới và restart khi có bản cập nhật.

Mở file `/opt/cqa/docker-compose.yml` trên VPS, sửa 3 chỗ:

**1. Thêm label vào service `app` và `nginx`** (để Watchtower biết cần update):

```yaml
services:
  app:
    image: buitanviet/chat-quality-agent:latest
    labels:
      - com.centurylinklabs.watchtower.enable=true
    ...

  nginx:
    image: buitanviet/chat-quality-agent-nginx:latest
    labels:
      - com.centurylinklabs.watchtower.enable=true
    ...
```

**2. KHÔNG thêm label vào `db`** — MySQL sẽ không bị tự động update (tránh lỗi data).

**3. Thêm service `watchtower` vào cuối phần `services:`**:

```yaml
  watchtower:
    image: containrrr/watchtower
    volumes:
      - /var/run/docker.sock:/var/run/docker.sock
    environment:
      - WATCHTOWER_CLEANUP=true
      - WATCHTOWER_POLL_INTERVAL=300
      - WATCHTOWER_LABEL_ENABLE=true
    restart: unless-stopped
```

Chạy:

```bash
cd /opt/cqa
docker compose up -d watchtower
```

Watchtower sẽ kiểm tra Docker Hub mỗi 5 phút. Khi phát hiện image mới, tự pull và restart container **app + nginx** (có label). MySQL không có label nên không bị update, dữ liệu an toàn.

::: tip Xem log Watchtower
```bash
docker compose logs watchtower -f
```
Thấy dòng `Found new ...` nghĩa là đã tự cập nhật thành công.
:::

## Gỡ cài đặt

```bash
cd /opt/cqa
docker compose down -v   # -v xóa cả database
rm -rf /opt/cqa
```

::: warning Lưu ý
`docker compose down -v` sẽ xóa toàn bộ dữ liệu (database, tin nhắn, kết quả). Nếu chỉ muốn dừng mà giữ dữ liệu, dùng `docker compose down` (không có `-v`).
:::

## Bước tiếp theo

- [Tên miền & SSL](/guide/domain-ssl) — Trỏ domain và bật HTTPS
- [Thiết lập ban đầu](/guide/initial-setup) — Tạo admin, cấu hình AI
