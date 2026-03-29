# Thông báo

CQA gửi kết quả phân tích qua **Telegram** và/hoặc **Email** sau mỗi lần chạy công việc.

## Cấu hình thông báo

Thông báo được cấu hình trong bước 4 khi [tạo công việc](/usage/jobs). Mỗi công việc có thể có nhiều kênh thông báo riêng.

## Telegram

### Thiết lập

1. **Tạo Bot**:
   - Chat với [@BotFather](https://t.me/BotFather) trên Telegram
   - Gửi `/newbot`, đặt tên cho bot
   - BotFather trả về **Bot Token** (dạng `123456:ABC-DEF...`)

2. **Lấy Group ID**:
   - Tạo group Telegram (hoặc dùng group có sẵn)
   - Thêm bot vào group
   - Mở [Telegram Web](https://web.telegram.org) trên trình duyệt
   - Vào group cần lấy ID
   - Xem URL trên thanh địa chỉ, ví dụ: `https://web.telegram.org/k/#-1001234567890`
   - **Group ID** = số sau dấu `#` (bao gồm dấu `-`, ví dụ `-1001234567890`)

3. **Nhập vào CQA**:
   - Bot Token: paste token từ BotFather
   - Group ID: paste Group ID (bao gồm dấu trừ)
   - Bấm **Gửi thử** để kiểm tra

### Nội dung thông báo Telegram

Bot sẽ gửi tin nhắn vào group gồm:
- Tên công việc
- Thời gian chạy
- Tổng kết (số cuộc chat, đạt/không đạt)
- Danh sách vấn đề phát hiện (nếu có)
- Link xem chi tiết trên CQA

::: info Cấu hình link
Link trong thông báo dùng URL từ **Cài đặt > Chung > [URL ứng dụng](/usage/general-settings#url-ung-dung)**. Nếu chưa cấu hình, link sẽ trỏ về localhost.
:::

## Email

### Thiết lập

| Trường | Ví dụ | Ghi chú |
|--------|-------|---------|
| SMTP Host | `smtp.gmail.com` | Server gửi email |
| SMTP Port | `587` | TLS: 587, SSL: 465 |
| Username | `bot@company.com` | Tài khoản email |
| Password | `app-password` | Với Gmail dùng App Password |
| Email gửi | `bot@company.com` | Địa chỉ hiển thị |
| Email nhận | `manager@company.com, admin@company.com` | Nhiều email cách nhau bằng dấu phẩy |

::: tip Gmail App Password
Nếu dùng Gmail, bạn cần tạo App Password thay vì dùng mật khẩu Gmail:
1. Vào [myaccount.google.com/apppasswords](https://myaccount.google.com/apppasswords)
2. Chọn "Other" > đặt tên "CQA"
3. Copy mật khẩu 16 ký tự
:::

### Nội dung email

Email gồm:
- **Tiêu đề**: Tên công việc + kết quả tổng quan
- **Nội dung**: Báo cáo chi tiết với bảng kết quả
- **Link**: Đường dẫn đến trang kết quả trên CQA

## Tùy chỉnh nội dung thông báo

Bạn có thể viết template riêng thay vì dùng mặc định:

### Biến có thể dùng

| Biến | Giá trị |
|------|---------|
| Biến | Giá trị |
|------|---------|
| `job_name` | Tên công việc |
| `total` | Tổng cuộc chat đã phân tích |
| `passed` | Số cuộc chat đạt |
| `failed` | Số cuộc chat không đạt |
| `issues` | Số vấn đề phát hiện |
| `content` | Nội dung đánh giá chi tiết |
| `link` | URL xem kết quả trên hệ thống (dùng [URL ứng dụng](/usage/general-settings#url-ung-dung) đã cấu hình) |

Cú pháp dùng trong template: bọc tên biến trong dấu ngoặc nhọn kép, ví dụ: `{` `{job_name}` `}`

## Lịch sử thông báo

Vào menu **Thông báo** ở sidebar để xem lịch sử gửi thông báo:
- Thời gian gửi
- Kênh (Telegram / Email)
- Người nhận
- Trạng thái (Đã gửi / Lỗi)
- Bấm **Xem** để đọc nội dung đã gửi
