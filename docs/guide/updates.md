# Cập nhật phiên bản

## Thông báo trên giao diện

CQA tự động kiểm tra phiên bản mới mỗi khi bạn đăng nhập (cache 1 giờ). Khi có bản cập nhật:

- **Chip phiên bản** ở header chuyển sang màu vàng (bình thường là xanh)
- **Banner thông báo** hiện bên dưới header với link đến changelog

Bấm vào chip phiên bản để xem chi tiết thay đổi trong bản mới.

## Cập nhật thủ công

```bash
cd /opt/cqa
docker compose pull
docker compose up -d
```

Lệnh trên sẽ pull image mới từ Docker Hub và restart container. Dữ liệu MySQL không bị ảnh hưởng.

## Tự động cập nhật (tùy chọn)

Thêm [Watchtower](https://containrrr.dev/watchtower/) để VPS tự động pull image mới và restart khi có bản cập nhật.

Chạy lệnh sau trên VPS để cập nhật file docker-compose.yml (đã bao gồm Watchtower + label):

```bash
cd /opt/cqa
curl -sfL https://raw.githubusercontent.com/tanviet12/chat-quality-agent/main/docker-compose.hub.yml -o docker-compose.yml
docker compose up -d
```

::: info Lệnh trên an toàn
File `.env` (chứa secrets, database password) không bị ảnh hưởng. Dữ liệu MySQL nằm trong Docker volume, không bị mất.
:::

Watchtower sẽ kiểm tra Docker Hub mỗi 5 phút. Khi phát hiện image mới, tự pull và restart container **app + nginx** (có label). MySQL không có label nên không bị update, dữ liệu an toàn.

::: tip Xem log Watchtower
```bash
docker compose logs watchtower -f
```
Thấy dòng `Found new ...` nghĩa là đã tự cập nhật thành công.
:::
