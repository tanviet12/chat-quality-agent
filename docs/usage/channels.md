# Kết nối kênh chat

CQA hỗ trợ 2 kênh chat: **Zalo OA** và **Facebook Messenger**. Bạn có thể kết nối nhiều kênh cùng lúc.

## Thêm kênh

Vào menu **Kênh chat** ở sidebar, bấm **Thêm kênh**.

![Kết nối kênh chat](/screenshots/ket-noi-kenh-chat.png)

---

## Zalo OA

### Yêu cầu

- Tài khoản Zalo Developers ([developers.zalo.me](https://developers.zalo.me))
- Một Zalo OA đang hoạt động, bạn phải là **quản trị viên (admin)** của OA
- Một tài khoản Zalo cá nhân (dùng để đăng nhập)
- **Deploy trên VPS**: cần tên miền + SSL (Zalo yêu cầu callback HTTPS). Xem [Tên miền & SSL](/guide/domain-ssl)
- **Test trên localhost**: Zalo cho phép callback `http://localhost`, không cần SSL

### Bước 1: Bật SSL cho CQA (bỏ qua nếu dùng localhost)

Khi deploy trên VPS, Zalo yêu cầu callback URL phải là HTTPS:
- Trỏ domain về IP VPS (ví dụ `cqa.yourdomain.com`)
- Bật SSL trong `.env` (xem [Tên miền & SSL](/guide/domain-ssl))

::: tip Dùng localhost?
Nếu chạy CQA trên máy cá nhân (`http://localhost`), bỏ qua bước này. Zalo hỗ trợ callback localhost.
:::

### Bước 2: Tạo ứng dụng trên Zalo Developers

1. Truy cập [developers.zalo.me](https://developers.zalo.me) → đăng nhập bằng tài khoản Zalo cá nhân.

![Trang Zalo Developers](/screenshots/zalooa/zalooa-1.png)

2. Click vào **avatar** góc trên bên phải → chọn **"Thêm ứng dụng mới"**.

![Thêm ứng dụng mới](/screenshots/zalooa/zalooa-2.png)

3. Điền thông tin ứng dụng:
   - **Tên hiển thị**: đặt tên ứng dụng (ví dụ: Chat Quality Agent)
   - **Danh mục**: chọn Kinh doanh
   - **Mô tả**: mô tả ngắn về ứng dụng
4. Tick "Tôi đã đọc và đồng ý..." → Click **"Tạo ID ứng dụng"**.

![Tạo ID ứng dụng](/screenshots/zalooa/zalooa-3.png)

### Bước 3: Điền thông tin và kích hoạt ứng dụng

Sau khi tạo xong, bạn sẽ được chuyển đến trang **Thông tin ứng dụng**. Ứng dụng lúc này ở trạng thái "Chưa kích hoạt".

1. Điền đầy đủ các trường bắt buộc:
   - **Điện thoại liên hệ**
   - **Email liên hệ**
   - **Icon ứng dụng** (512x512)

![Điền thông tin ứng dụng](/screenshots/zalooa/zalooa-4.png)

2. Click **"Lưu thay đổi"**.

3. Bật toggle **"Kích hoạt"** ở góc trên bên phải → trạng thái chuyển thành **"Đang hoạt động"**.

![Kích hoạt ứng dụng](/screenshots/zalooa/zalooa-5.png)

4. Tại trang này, sao chép và lưu lại:
   - **ID ứng dụng** (App ID)
   - **Khóa bí mật của ứng dụng** (Secret Key) — click icon con mắt để hiện, rồi click icon copy

![App ID và Secret Key](/screenshots/zalooa/zalooa-6.png)

### Bước 4: Xác thực domain

Zalo yêu cầu xác thực quyền sở hữu domain trước khi cho phép sử dụng callback URL.

1. Trong menu bên trái, vào **Xác thực domain**
2. Nhập domain CQA (ví dụ `cqa.yourdomain.com`)
3. Chọn phương thức xác thực **DNS TXT Record** (khuyến nghị):
   - Zalo cung cấp 1 giá trị TXT record
   - Vào nhà cung cấp DNS, thêm bản ghi TXT cho domain với giá trị Zalo cung cấp
   - Chờ DNS cập nhật (5-15 phút)
   - Quay lại Zalo bấm **Xác thực**

::: tip Dùng TXT Record
Nên dùng phương thức **DNS TXT Record** vì không cần thay đổi gì trên server CQA. Các phương thức khác (upload file HTML, thêm meta tag) yêu cầu tùy chỉnh thêm.
:::

### Bước 5: Chọn quyền API và thiết lập Callback URL

1. Trong menu bên trái, vào **Sản phẩm** → **Official Account** → **Thiết lập chung**.

![Menu Official Account](/screenshots/zalooa/zalooa-7.png)

2. Tại mục **"Chọn quyền cần yêu cầu được cấp từ OA"**, tick chọn các quyền:
   - **Quản lý thông tin OA** — lấy thông tin OA, danh sách người dùng
   - **Quản lý trường thông tin người dùng** — lấy danh sách và chi tiết trường thông tin
   - **Quản lý tin nhắn người dùng** — lấy danh sách hội thoại của OA và hội thoại với người dùng cụ thể

![Chọn quyền API](/screenshots/zalooa/zalooa-11.png)

::: danger Quan trọng
Quyền **"Quản lý tin nhắn người dùng"** là **bắt buộc** để CQA đọc được tin nhắn.
:::

3. Tại mục **"Official Account Callback Url"**, nhập URL callback: `https://cqa.yourdomain.com/api/v1/channels/zalo/callback`
4. Click **"Cập nhật"**.

![Thiết lập Callback URL](/screenshots/zalooa/zalooa-10.png)

### Bước 6: Liên kết ứng dụng với OA

1. Trong menu bên trái, vào **Official Account** → **Quản lý OA**.
2. Tại mục **"Liên kết với Official Account"**, chọn OA cần kết nối từ dropdown.
3. Click **"Liên kết"**.

![Liên kết OA](/screenshots/zalooa/zalooa-8.png)

::: warning Lưu ý
- Bạn cần đồng thời là quản trị viên của cả OA và ứng dụng để thực hiện liên kết.
- Nếu Zalo cảnh báo đã có app liên kết đến OA này, bạn nên dùng App đó (nhớ phân quyền đầy đủ). Vì liên kết mới sẽ hủy kết nối của app cũ.
:::

### Bước 7: Kết nối kênh trên CQA

1. Trong CQA, vào **Kênh chat** → click **"+ Kết nối kênh mới"**
2. Điền các thông tin:
   - **Loại kênh**: chọn Zalo OA
   - **Tên kênh**: đặt tên để phân biệt (ví dụ: SePay Coffee)
   - **App ID**: dán App ID đã lấy ở Bước 3
   - **App Secret**: dán Secret Key đã lấy ở Bước 3
   - **Chu kỳ đồng bộ**: chọn tần suất (mặc định: Mỗi 15 phút)
   - **Lưu trữ file/ảnh từ cuộc chat**: bật nếu muốn tải file ảnh về server
3. Click **"Tạo & Xác thực qua Zalo"**

![Form kết nối trên CQA](/screenshots/zalooa/zalooa-12.png)

### Bước 8: Cấp quyền trên Zalo

1. Trình duyệt mở trang xác thực của Zalo
2. Đăng nhập bằng tài khoản Zalo admin của OA (nếu chưa đăng nhập)
3. Kiểm tra 3 quyền API đã chọn:
   - Quản lý thông tin OA
   - Quản lý trường thông tin người dùng
   - Quản lý tin nhắn người dùng
4. Tick **"Tôi đã đọc và hoàn toàn đồng ý..."** → Click **"Cấp quyền"**

![Cấp quyền trên Zalo](/screenshots/zalooa/zalooa-13.png)

5. Zalo redirect về CQA. Hệ thống tự động nhận authorization code và lấy OA Access Token.
6. **Kết nối hoàn tất!** Trạng thái kênh chuyển thành "Đã kết nối".

### Lưu ý về Zalo OA

::: warning Giới hạn Zalo API
- **Cửa sổ 48 giờ**: Zalo chỉ cho phép đọc tin nhắn trong vòng 48 giờ gần nhất. Tin nhắn cũ hơn sẽ không lấy được.
- **Token hết hạn**: Access token Zalo hết hạn sau 90 ngày. CQA tự động refresh, nhưng nếu token bị revoke, bạn cần **xác thực lại**.
- **Rate limit**: Zalo giới hạn số lượng API call. Không nên đặt lịch đồng bộ quá thường xuyên (khuyến nghị 15-30 phút/lần).
:::

---

## Facebook Messenger

Hướng dẫn kết nối Facebook Messenger chi tiết (có ảnh minh họa) tại trang riêng:

**[Kết nối Facebook Messenger](/usage/facebook)**

---

## Đồng bộ tin nhắn

Sau khi kết nối kênh, bấm nút **Đồng bộ ngay** (icon refresh) để lấy tin nhắn.

### Đồng bộ lần đầu

- Hệ thống sẽ tải toàn bộ tin nhắn có thể truy cập
- Zalo OA: Tin nhắn trong 48 giờ gần nhất
- Facebook: Tin nhắn trong khoảng thời gian cho phép
- Quá trình có thể mất vài phút tùy số lượng tin nhắn

### Trạng thái đồng bộ

Mỗi kênh hiển thị trạng thái:
- **Thành công** (tick xanh): Đồng bộ lần cuối thành công
- **Đang chạy** (loading): Đang đồng bộ
- **Lỗi** (dấu X đỏ): Đồng bộ thất bại — kiểm tra log hoặc xác thực lại

### Xác thực lại

Nếu kênh báo lỗi xác thực, bấm **Xác thực lại** để kết nối lại (thường do token hết hạn).

---

## Quản lý kênh

### Xem chi tiết kênh

Bấm vào card kênh để xem:

![Chi tiết kênh chat](/screenshots/chi-tiet-kenh-chat.png)
- Thông tin kết nối
- Số cuộc hội thoại đã đồng bộ
- Lịch sử đồng bộ
- Trạng thái kết nối

### Xóa kênh

Bấm icon thùng rác trên card kênh.

::: danger Cảnh báo
Xóa kênh sẽ xóa luôn **tất cả cuộc hội thoại và tin nhắn** đã đồng bộ từ kênh đó, kèm theo kết quả đánh giá liên quan. Thao tác này không thể hoàn tác.
:::

### Xóa dữ liệu hội thoại (giữ kênh)

Nếu muốn xóa tin nhắn cũ nhưng giữ lại kết nối kênh, có thể xóa dữ liệu hội thoại trong trang chi tiết kênh. Sau đó đồng bộ lại sẽ lấy tin nhắn mới.
