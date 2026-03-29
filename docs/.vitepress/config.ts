import { defineConfig } from 'vitepress'

export default defineConfig({
  title: 'Chat Quality Agent',
  description: 'Hệ thống phân tích chất lượng CSKH bằng AI',
  lang: 'vi-VN',
  base: '/chat-quality-agent/',

  head: [
    ['meta', { name: 'theme-color', content: '#1976D2' }],
  ],

  themeConfig: {
    nav: [
      { text: 'Hướng dẫn', link: '/guide/installation' },
      { text: 'Sử dụng', link: '/usage/channels' },
      { text: 'Tham khảo', link: '/reference/env-vars' },
    ],

    sidebar: [
      {
        text: 'Bắt đầu',
        items: [
          { text: 'Giới thiệu', link: '/guide/introduction' },
          { text: 'Cài đặt', link: '/guide/installation' },
          { text: 'Cập nhật phiên bản', link: '/guide/updates' },
          { text: 'Tên miền & SSL', link: '/guide/domain-ssl' },
          { text: 'Thiết lập ban đầu', link: '/guide/initial-setup' },
        ],
      },
      {
        text: 'Sử dụng',
        items: [
          { text: 'Cấu hình chung', link: '/usage/general-settings' },
          { text: 'Cấu hình AI', link: '/usage/ai-settings' },
          { text: 'Kết nối Zalo OA', link: '/usage/channels' },
          { text: 'Kết nối Facebook', link: '/usage/facebook' },
          { text: 'Quản lý tin nhắn', link: '/usage/messages' },
          { text: 'Tạo công việc', link: '/usage/jobs' },
          { text: 'Xem kết quả', link: '/usage/results' },
          { text: 'Thông báo', link: '/usage/notifications' },
          { text: 'Dashboard', link: '/usage/dashboard' },
          { text: 'Chi phí AI', link: '/usage/cost-logs' },
        ],
      },
      {
        text: 'Quản trị',
        items: [
          { text: 'Người dùng & phân quyền', link: '/admin/users' },
          { text: 'Quản lý đa công ty', link: '/admin/multi-tenant' },
          { text: 'Kết nối MCP', link: '/admin/mcp' },
          { text: 'Dữ liệu demo', link: '/admin/demo-data' },
        ],
      },
      {
        text: 'Tham khảo',
        items: [
          { text: 'Biến môi trường', link: '/reference/env-vars' },
          { text: 'REST API', link: '/reference/api' },
        ],
      },
      {
        text: 'Hỗ trợ',
        items: [
          { text: 'FAQ & Xử lý lỗi', link: '/faq' },
          { text: 'Changelog', link: '/changelog' },
        ],
      },
    ],

    socialLinks: [
      { icon: 'github', link: 'https://github.com/tanviet12/chat-quality-agent' },
    ],

    search: {
      provider: 'local',
    },

    footer: {
      message: 'Phát hành theo giấy phép MIT',
      copyright: 'Copyright 2026 SePay',
    },
  },
})
