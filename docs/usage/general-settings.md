# Cấu hình chung

Vào menu **Cài đặt** (icon bánh răng ở sidebar) > tab **Chung**.

![Cấu hình chung](/screenshots/cau-hinh-chung.png)

## Các mục cấu hình

| Mục | Mô tả | Mặc định |
|-----|-------|----------|
| **Tên công ty** | Hiển thị trên giao diện và trong thông báo | (tên khi tạo tenant) |
| **Múi giờ** | Ảnh hưởng đến thời gian hiển thị và lịch chạy công việc | Asia/Ho_Chi_Minh |
| **Ngôn ngữ** | Giao diện Tiếng Việt hoặc English | Tiếng Việt |
| **Tỉ giá USD → VND** | Dùng để quy đổi chi phí AI sang VND | 26,000 |
| **URL ứng dụng** | URL truy cập CQA, dùng để tạo link trong thông báo Telegram/Email | (không có) |

## Tỉ giá USD → VND

Chi phí AI (Claude, Gemini) được tính bằng USD. CQA quy đổi sang VND để bạn dễ theo dõi.

- Mặc định: 26,000 VND/USD
- Bạn có thể cập nhật theo tỉ giá thực tế
- Ảnh hưởng đến hiển thị trong **Dashboard** và **Chi phí AI**

## URL ứng dụng

Cấu hình URL để hệ thống gửi link chính xác qua [Telegram và Email](/usage/notifications).

- Nhập URL truy cập CQA của bạn, ví dụ: `https://cqa.sepay.vn`
- URL phải bắt đầu bằng `http://` hoặc `https://`, không có dấu `/` ở cuối
- Link trong thông báo sẽ dẫn thẳng đến trang kết quả trên CQA

::: tip Thứ tự ưu tiên
1. **URL ứng dụng** trong Cài đặt (ưu tiên cao nhất)
2. Biến môi trường `APP_URL` trong file `.env`
3. Fallback: `http://localhost:8080`

Nếu bạn đã cấu hình domain + SSL, hãy nhập URL ở đây để link trong thông báo trỏ đúng.
:::

## Lưu cấu hình

Bấm **Lưu cấu hình** sau khi thay đổi. Cấu hình có hiệu lực ngay lập tức, không cần khởi động lại.
