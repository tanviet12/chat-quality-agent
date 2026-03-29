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

## Chạy trên localhost (Mac / Windows)

Bạn có thể chạy CQA trên chính máy cá nhân bằng Docker Desktop để test trước khi deploy VPS.

**Yêu cầu:**
- [Docker Desktop](https://www.docker.com/products/docker-desktop/) (Mac hoặc Windows)
- Clone repo và chạy `docker compose up -d --build`
- Truy cập `http://localhost`

**Lưu ý kết nối kênh chat:**
- **Zalo OA**: Hỗ trợ callback URL là `http://localhost` — có thể test đầy đủ trên máy local
- **Facebook Fanpage**: Yêu cầu HTTPS — không dùng localhost được, cần deploy lên VPS với domain + SSL

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

- [Cập nhật phiên bản](/guide/updates) — Cập nhật thủ công hoặc tự động
- [Tên miền & SSL](/guide/domain-ssl) — Trỏ domain và bật HTTPS
- [Thiết lập ban đầu](/guide/initial-setup) — Tạo admin, cấu hình AI
