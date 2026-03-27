package handlers

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/nmtan2001/chat-quality-agent/api/middleware"
	"github.com/nmtan2001/chat-quality-agent/db"
	"github.com/nmtan2001/chat-quality-agent/db/models"
	"github.com/nmtan2001/chat-quality-agent/pkg"
)

// GetDemoStatus returns whether tenant has data and if it's demo data.
func GetDemoStatus(c *gin.Context) {
	tenantID := middleware.GetTenantID(c)

	var channelCount int64
	db.DB.Model(&models.Channel{}).Where("tenant_id = ?", tenantID).Count(&channelCount)

	var tenant models.Tenant
	db.DB.Where("id = ?", tenantID).First(&tenant)
	isDemo := false
	if tenant.Settings != "" {
		var s map[string]interface{}
		if json.Unmarshal([]byte(tenant.Settings), &s) == nil {
			if v, ok := s["is_demo_data"]; ok {
				isDemo, _ = v.(bool)
			}
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"has_data": channelCount > 0,
		"is_demo":  isDemo,
	})
}

// ImportDemoData creates demo data for a tenant.
func ImportDemoData(c *gin.Context) {
	tenantID := middleware.GetTenantID(c)

	// Check no existing channels
	var channelCount int64
	db.DB.Model(&models.Channel{}).Where("tenant_id = ?", tenantID).Count(&channelCount)
	if channelCount > 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "tenant_has_data"})
		return
	}

	now := time.Now()
	rng := rand.New(rand.NewSource(now.UnixNano()))

	// === Channels ===
	zaloChannelID := pkg.NewUUID()
	fbChannelID := pkg.NewUUID()
	dummyCreds := []byte(`{"demo":true}`)
	channels := []models.Channel{
		{ID: zaloChannelID, TenantID: tenantID, ChannelType: "zalo_oa", Name: "SePay Coffee Zalo OA", ExternalID: "demo-zalo-oa", CredentialsEncrypted: dummyCreds, IsActive: true, Metadata: "{}", CreatedAt: now.Add(-14 * 24 * time.Hour), UpdatedAt: now},
		{ID: fbChannelID, TenantID: tenantID, ChannelType: "facebook", Name: "SePay Coffee Facebook", ExternalID: "demo-fb-page", CredentialsEncrypted: dummyCreds, IsActive: true, Metadata: "{}", CreatedAt: now.Add(-14 * 24 * time.Hour), UpdatedAt: now},
	}

	// === QC Job ===
	qcJobID := pkg.NewUUID()
	qcRunID := pkg.NewUUID()
	rulesContent := `# Quy tắc đánh giá chất lượng CSKH - SePay Coffee

## 1. Chào hỏi lịch sự
- Nhân viên phải chào hỏi khách hàng trong tin nhắn đầu tiên
- Sử dụng ngôn ngữ thân thiện, lịch sự
- Mức độ: Nghiêm trọng

## 2. Thời gian phản hồi
- Trả lời khách trong vòng 5 phút kể từ tin nhắn cuối của khách
- Không để khách chờ quá lâu mà không có phản hồi
- Mức độ: Nghiêm trọng

## 3. Giải đáp đầy đủ
- Trả lời đúng và đủ câu hỏi của khách hàng
- Cung cấp thông tin chính xác về menu, giá cả, khuyến mãi
- Nếu không biết, phải hỏi lại hoặc chuyển cho người có thẩm quyền
- Mức độ: Cần cải thiện

## 4. Kết thúc chuyên nghiệp
- Cảm ơn khách đã liên hệ
- Hỏi khách còn cần hỗ trợ gì không trước khi kết thúc
- Mức độ: Cần cải thiện`

	skipConditions := `- Cuộc chat dưới 2 tin nhắn
- Khách chỉ gửi sticker hoặc hình ảnh mà không có nội dung text
- Cuộc chat chỉ có tin nhắn tự động từ hệ thống`

	// === Classification Job ===
	classJobID := pkg.NewUUID()
	classRunID := pkg.NewUUID()
	rulesConfig := `[{"name":"Hỏi menu / Đặt bàn","description":"Khách hỏi thực đơn, giá cả, hoặc muốn đặt bàn, đặt chỗ","severity":"CAN_CAI_THIEN"},{"name":"Khiếu nại","description":"Khách phàn nàn về đồ uống, phục vụ, hoặc trải nghiệm tại quán","severity":"NGHIEM_TRONG"},{"name":"Góp ý","description":"Khách đề xuất thêm món mới, cải tiến dịch vụ, feedback tích cực hoặc tiêu cực","severity":"CAN_CAI_THIEN"},{"name":"Hỗ trợ chung","description":"Khách hỏi giờ mở cửa, wifi, địa chỉ chi nhánh, chương trình thẻ thành viên","severity":"CAN_CAI_THIEN"}]`

	inputChannelIDs, _ := json.Marshal([]string{zaloChannelID, fbChannelID})

	jobs := []models.Job{
		{
			ID: qcJobID, TenantID: tenantID, Name: "Đánh giá chất lượng CSKH",
			Description: "Tự động đánh giá chất lượng hỗ trợ khách hàng qua chat dựa trên bộ quy tắc",
			JobType: "qc_analysis", InputChannelIDs: string(inputChannelIDs),
			RulesContent: rulesContent, RulesConfig: "[]", SkipConditions: skipConditions,
			AIProvider: "claude", AIModel: "claude-sonnet-4-6",
			Outputs: "[]", OutputSchedule: "none",
			ScheduleType: "manual", IsActive: true,
			LastRunAt: &now, LastRunStatus: "success",
			CreatedAt: now.Add(-13 * 24 * time.Hour), UpdatedAt: now,
		},
		{
			ID: classJobID, TenantID: tenantID, Name: "Phân loại phản hồi khách hàng",
			Description: "Tự động phân loại cuộc chat theo nhãn: hỏi menu, khiếu nại, góp ý, hỗ trợ",
			JobType: "classification", InputChannelIDs: string(inputChannelIDs),
			RulesConfig: rulesConfig,
			AIProvider: "claude", AIModel: "claude-sonnet-4-6",
			Outputs: "[]", OutputSchedule: "none",
			ScheduleType: "manual", IsActive: true,
			LastRunAt: &now, LastRunStatus: "success",
			CreatedAt: now.Add(-13 * 24 * time.Hour), UpdatedAt: now,
		},
	}

	// === Name pool ===
	customerNames := []string{
		"Nguyễn Văn An", "Trần Thị Bình", "Lê Hoàng Cường", "Phạm Thị Dung", "Hoàng Đức Em",
		"Vũ Thị Phương", "Đặng Minh Quân", "Bùi Thị Hồng", "Ngô Văn Khải", "Đỗ Thị Lan",
		"Trương Công Minh", "Lý Thị Ngọc", "Hồ Quang Phú", "Mai Thị Quỳnh", "Phan Văn Sơn",
		"Đinh Thị Tâm", "Lương Văn Uy", "Cao Thị Vân", "Dương Bá Xuân", "Tạ Thị Yến",
		"Nguyễn Thanh Tùng", "Trần Minh Đức", "Lê Thị Hạnh", "Phạm Quốc Bảo", "Hoàng Thị Cúc",
		"Vũ Đình Dũng", "Đặng Thị Giang", "Bùi Văn Hải", "Ngô Thị Kim", "Đỗ Quang Long",
		"Trương Thị Mai", "Lý Văn Nam", "Hồ Thị Oanh", "Mai Đức Phong", "Phan Thị Rồng",
		"Đinh Văn Sang", "Lương Thị Thảo", "Cao Văn Toàn", "Dương Thị Uyên", "Tạ Văn Vinh",
		"Chu Thị Ánh", "Kiều Văn Bách", "La Thị Chi", "Mạc Văn Đạt", "Tô Thị Hà",
		"Âu Văn Hùng", "Quách Thị Liên", "Thái Văn Minh", "Trịnh Thị Nhi", "Lê Văn Phúc",
	}

	agentNames := []string{"Linh - SePay Coffee", "Hùng - SePay Coffee", "Trang - SePay Coffee", "Đức - SePay Coffee"}

	// === Conversation templates ===
	type convTemplate struct {
		category string // qc_pass, qc_fail, qc_skip, class_menu, class_complaint, class_feedback, class_support
		score    int
		messages []struct {
			sender  string // customer / agent
			content string
		}
		qcVerdict    string
		qcReview     string
		violations   []struct{ rule, evidence, severity string }
		classTags    []string
		classEvidence []string
	}

	templates := buildDemoTemplates()

	// === Generate conversations ===
	var allConversations []models.Conversation
	var allMessages []models.Message
	var allResults []models.JobResult
	var allUsageLogs []models.AIUsageLog

	convCount := 0
	for _, tmpl := range templates {
		count := tmpl.count
		for i := 0; i < count; i++ {
			convCount++
			convID := pkg.NewUUID()
			channelID := zaloChannelID
			if convCount%2 == 0 {
				channelID = fbChannelID
			}

			customerName := customerNames[rng.Intn(len(customerNames))]
			agentName := agentNames[rng.Intn(len(agentNames))]
			daysAgo := rng.Intn(14)
			hoursAgo := rng.Intn(12) + 8 // 8-20h
			baseTime := now.Add(-time.Duration(daysAgo) * 24 * time.Hour).Truncate(24 * time.Hour).Add(time.Duration(hoursAgo) * time.Hour)

			conv := models.Conversation{
				ID: convID, TenantID: tenantID, ChannelID: channelID,
				ExternalConversationID: fmt.Sprintf("demo-conv-%d", convCount),
				CustomerName:           customerName,
				MessageCount:           len(tmpl.messages),
				Metadata:               "{}",
				CreatedAt:              baseTime, UpdatedAt: now,
			}

			var lastMsgTime time.Time
			for j, msg := range tmpl.messages {
				msgTime := baseTime.Add(time.Duration(j*2) * time.Minute)
				lastMsgTime = msgTime
				senderName := customerName
				senderType := "customer"
				if msg.sender == "agent" {
					senderName = agentName
					senderType = "agent"
				}
				m := models.Message{
					ID: pkg.NewUUID(), TenantID: tenantID, ConversationID: convID,
					ExternalMessageID: fmt.Sprintf("demo-msg-%d-%d", convCount, j),
					SenderType: senderType, SenderName: senderName,
					Content: msg.content, ContentType: "text", Attachments: "[]", RawData: "{}",
					SentAt: msgTime, CreatedAt: msgTime,
				}
				allMessages = append(allMessages, m)
			}
			conv.LastMessageAt = &lastMsgTime
			allConversations = append(allConversations, conv)

			evalTime := lastMsgTime.Add(30 * time.Minute)

			// QC results
			if tmpl.category == "qc_pass" || tmpl.category == "qc_fail" || tmpl.category == "qc_skip" {
				detail, _ := json.Marshal(map[string]interface{}{"score": tmpl.score})
				allResults = append(allResults, models.JobResult{
					ID: pkg.NewUUID(), JobRunID: qcRunID, TenantID: tenantID, ConversationID: convID,
					ResultType: "conversation_evaluation", Severity: tmpl.verdict,
					Evidence: tmpl.review, Detail: string(detail), Confidence: 0.92,
					CreatedAt: evalTime,
				})
				for _, v := range tmpl.violations {
					vDetail, _ := json.Marshal(map[string]interface{}{"explanation": v.evidence})
					allResults = append(allResults, models.JobResult{
						ID: pkg.NewUUID(), JobRunID: qcRunID, TenantID: tenantID, ConversationID: convID,
						ResultType: "qc_violation", Severity: v.severity, RuleName: v.rule,
						Evidence: v.evidence, Detail: string(vDetail), Confidence: 0.88,
						CreatedAt: evalTime,
					})
				}
				allUsageLogs = append(allUsageLogs, models.AIUsageLog{
					ID: pkg.NewUUID(), TenantID: tenantID, JobID: qcJobID, JobRunID: qcRunID,
					Provider: "claude", Model: "claude-sonnet-4-6",
					InputTokens: 1800 + rng.Intn(600), OutputTokens: 600 + rng.Intn(400),
					CostUSD: 0.012 + float64(rng.Intn(8))*0.001,
					CreatedAt: evalTime,
				})
			}

			// Classification results
			if tmpl.category == "class_menu" || tmpl.category == "class_complaint" || tmpl.category == "class_feedback" || tmpl.category == "class_support" || tmpl.category == "class_skip" {
				verdict := "PASS"
				if tmpl.category == "class_skip" {
					verdict = "SKIP"
				}
				summary := ""
				if len(tmpl.classTags) > 0 {
					summary = "Cuộc chat được phân loại: " + strings.Join(tmpl.classTags, ", ")
				}
				detail, _ := json.Marshal(map[string]interface{}{"summary": summary})
				allResults = append(allResults, models.JobResult{
					ID: pkg.NewUUID(), JobRunID: classRunID, TenantID: tenantID, ConversationID: convID,
					ResultType: "conversation_evaluation", Severity: verdict,
					Evidence: summary, Detail: string(detail), Confidence: 0.90,
					CreatedAt: evalTime.Add(time.Minute),
				})
				for k, tag := range tmpl.classTags {
					evidence := ""
					if k < len(tmpl.classEvidence) {
						evidence = tmpl.classEvidence[k]
					}
					tagDetail, _ := json.Marshal(map[string]interface{}{"confidence": 0.85 + float64(rng.Intn(15))*0.01})
					allResults = append(allResults, models.JobResult{
						ID: pkg.NewUUID(), JobRunID: classRunID, TenantID: tenantID, ConversationID: convID,
						ResultType: "classification_tag", RuleName: tag, Evidence: evidence,
						Detail: string(tagDetail), Confidence: 0.85 + float64(rng.Intn(15))*0.01,
						CreatedAt: evalTime.Add(time.Minute),
					})
				}
				allUsageLogs = append(allUsageLogs, models.AIUsageLog{
					ID: pkg.NewUUID(), TenantID: tenantID, JobID: classJobID, JobRunID: classRunID,
					Provider: "claude", Model: "claude-sonnet-4-6",
					InputTokens: 1500 + rng.Intn(500), OutputTokens: 400 + rng.Intn(300),
					CostUSD: 0.008 + float64(rng.Intn(6))*0.001,
					CreatedAt: evalTime.Add(time.Minute),
				})
			}
		}
	}

	// === Job runs ===
	qcPassed := 0
	qcFailed := 0
	qcSkipped := 0
	classDone := 0
	classSkipped := 0
	for _, t := range templates {
		switch t.category {
		case "qc_pass":
			qcPassed += t.count
		case "qc_fail":
			qcFailed += t.count
		case "qc_skip":
			qcSkipped += t.count
		case "class_skip":
			classSkipped += t.count
		default:
			if strings.HasPrefix(t.category, "class_") {
				classDone += t.count
			}
		}
	}
	qcTotal := qcPassed + qcFailed + qcSkipped
	classTotal := classDone + classSkipped

	qcSummary, _ := json.Marshal(map[string]interface{}{
		"conversations_found": qcTotal, "conversations_analyzed": qcTotal,
		"conversations_passed": qcPassed, "conversations_failed": qcFailed, "conversations_skipped": qcSkipped,
	})
	classSummary, _ := json.Marshal(map[string]interface{}{
		"conversations_found": classTotal, "conversations_analyzed": classTotal,
		"conversations_passed": classDone, "conversations_skipped": classSkipped,
	})

	runFinished := now.Add(-30 * time.Minute)
	jobRuns := []models.JobRun{
		{ID: qcRunID, JobID: qcJobID, TenantID: tenantID, StartedAt: now.Add(-2 * time.Hour), FinishedAt: &runFinished, Status: "success", Summary: string(qcSummary), CreatedAt: now.Add(-2 * time.Hour)},
		{ID: classRunID, JobID: classJobID, TenantID: tenantID, StartedAt: now.Add(-1 * time.Hour), FinishedAt: &runFinished, Status: "success", Summary: string(classSummary), CreatedAt: now.Add(-1 * time.Hour)},
	}

	// === Save all in transaction ===
	tx := db.DB.Begin()

	for _, ch := range channels {
		if err := tx.Create(&ch).Error; err != nil {
			tx.Rollback()
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
	}

	// Batch insert conversations
	if len(allConversations) > 0 {
		if err := tx.CreateInBatches(allConversations, 50).Error; err != nil {
			tx.Rollback()
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
	}

	// Batch insert messages
	if len(allMessages) > 0 {
		if err := tx.CreateInBatches(allMessages, 100).Error; err != nil {
			tx.Rollback()
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
	}

	for _, j := range jobs {
		if err := tx.Create(&j).Error; err != nil {
			tx.Rollback()
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
	}
	for _, r := range jobRuns {
		if err := tx.Create(&r).Error; err != nil {
			tx.Rollback()
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
	}

	if len(allResults) > 0 {
		if err := tx.CreateInBatches(allResults, 100).Error; err != nil {
			tx.Rollback()
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
	}
	if len(allUsageLogs) > 0 {
		if err := tx.CreateInBatches(allUsageLogs, 100).Error; err != nil {
			tx.Rollback()
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
	}

	// Set demo flag
	tx.Model(&models.Tenant{}).Where("id = ?", tenantID).Update("settings", `{"is_demo_data":true}`)

	if err := tx.Commit().Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message":       "Demo data imported successfully",
		"channels":      len(channels),
		"conversations": len(allConversations),
		"messages":      len(allMessages),
		"jobs":          len(jobs),
		"results":       len(allResults),
	})
}

// ResetDemoData deletes all tenant data except users.
func ResetDemoData(c *gin.Context) {
	tenantID := middleware.GetTenantID(c)

	// Check demo flag
	var tenant models.Tenant
	if err := db.DB.Where("id = ?", tenantID).First(&tenant).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "tenant_not_found"})
		return
	}
	isDemo := false
	if tenant.Settings != "" {
		var s map[string]interface{}
		if json.Unmarshal([]byte(tenant.Settings), &s) == nil {
			isDemo, _ = s["is_demo_data"].(bool)
		}
	}
	if !isDemo {
		c.JSON(http.StatusBadRequest, gin.H{"error": "not_demo_data"})
		return
	}

	tx := db.DB.Begin()

	// Delete in dependency order
	tx.Where("tenant_id = ?", tenantID).Delete(&models.Message{})
	tx.Where("tenant_id = ?", tenantID).Delete(&models.JobResult{})
	tx.Where("tenant_id = ?", tenantID).Delete(&models.AIUsageLog{})
	tx.Where("tenant_id = ?", tenantID).Delete(&models.NotificationLog{})
	tx.Where("tenant_id = ?", tenantID).Delete(&models.ActivityLog{})
	tx.Where("tenant_id = ?", tenantID).Delete(&models.JobRun{})
	tx.Where("tenant_id = ?", tenantID).Delete(&models.Job{})
	tx.Where("tenant_id = ?", tenantID).Delete(&models.Conversation{})
	tx.Where("tenant_id = ?", tenantID).Delete(&models.AppSetting{})
	tx.Where("tenant_id = ?", tenantID).Delete(&models.Channel{})

	// Clear demo flag
	tx.Model(&models.Tenant{}).Where("id = ?", tenantID).Update("settings", "{}")

	if err := tx.Commit().Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "All demo data deleted"})
}

// --- Demo conversation templates ---

type demoTemplate struct {
	count         int
	category      string
	verdict       string
	score         int
	review        string
	violations    []struct{ rule, evidence, severity string }
	classTags     []string
	classEvidence []string
	messages      []struct {
		sender  string
		content string
	}
}

func buildDemoTemplates() []demoTemplate {
	return []demoTemplate{
		// === QC PASS: Menu inquiry, good service (x15) ===
		{count: 15, category: "qc_pass", verdict: "PASS", score: 92,
			review: "Nhân viên chào hỏi lịch sự, trả lời nhanh và đầy đủ thông tin menu cho khách.",
			messages: []struct{ sender, content string }{
				{"customer", "Chào quán, cho mình hỏi menu cà phê với ạ"},
				{"agent", "Chào anh/chị, cảm ơn đã liên hệ SePay Coffee ạ! Bên em có menu cà phê như sau:\n- Cà phê sữa đá: 35k\n- Bạc xỉu: 35k\n- Cappuccino: 45k\n- Latte: 45k\n- Americano: 40k\nAnh/chị muốn dùng món nào ạ?"},
				{"customer", "Cappuccino size lớn bao nhiêu nhỉ?"},
				{"agent", "Dạ Cappuccino size L là 55k ạ. Bên em có thêm các topping như thạch cà phê +10k, shot espresso +15k. Anh/chị có muốn thêm gì không ạ?"},
				{"customer", "Ok cho mình 1 Cappuccino L thêm shot espresso nhé"},
				{"agent", "Dạ em ghi nhận 1 Cappuccino size L + shot espresso = 70k ạ. Anh/chị cho em địa chỉ giao hoặc đến quán lấy ạ?"},
				{"customer", "Mình ghé quán lấy, tầm 15 phút nữa"},
				{"agent", "Dạ em chuẩn bị sẵn cho anh/chị nhé. Cảm ơn anh/chị đã ủng hộ SePay Coffee ạ! Hẹn gặp anh/chị ạ 😊"},
			},
		},
		// === QC PASS: Booking table (x12) ===
		{count: 12, category: "qc_pass", verdict: "PASS", score: 88,
			review: "Nhân viên xử lý đặt bàn tốt, cung cấp đầy đủ thông tin cần thiết.",
			messages: []struct{ sender, content string }{
				{"customer", "Mình muốn đặt bàn cho 6 người tối nay được không?"},
				{"agent", "Chào anh/chị, dạ được ạ! Cho em hỏi anh/chị muốn đặt lúc mấy giờ ạ? Bên em có chi nhánh Nguyễn Huệ và Lê Văn Sỹ, anh/chị muốn đến chi nhánh nào ạ?"},
				{"customer", "7h tối, chi nhánh Nguyễn Huệ nhé"},
				{"agent", "Dạ em check thì chi nhánh Nguyễn Huệ tối nay 7h còn bàn cho 6 người ạ. Em đặt cho anh/chị nhé. Cho em xin tên và số điện thoại để giữ bàn ạ."},
				{"customer", "Nguyễn Văn Minh, 0912345678"},
				{"agent", "Dạ em đã đặt bàn cho anh Minh:\n- 6 người, 19:00 tối nay\n- Chi nhánh: 123 Nguyễn Huệ, Q1\n- SĐT: 0912345678\nAnh đến sớm 5-10 phút để em sắp xếp bàn nhé. Cảm ơn anh ạ!"},
				{"customer", "Ok cảm ơn nhé"},
				{"agent", "Dạ không có gì ạ, hẹn gặp anh tối nay! Anh cần hỗ trợ gì thêm cứ nhắn em nhé ạ 😊"},
			},
		},
		// === QC PASS: Complaint handled well (x12) ===
		{count: 12, category: "qc_pass", verdict: "PASS", score: 78,
			review: "Khách phàn nàn nhưng nhân viên xử lý khéo léo, xin lỗi và đề xuất giải pháp phù hợp.",
			violations: []struct{ rule, evidence, severity string }{
				{"Giải đáp đầy đủ", "Nhân viên chưa giải thích rõ nguyên nhân gây ra vấn đề, chỉ xin lỗi và đề xuất giải pháp.", "CAN_CAI_THIEN"},
			},
			messages: []struct{ sender, content string }{
				{"customer", "Mình order cà phê sữa đá mà giao tới nguội ngắt, đá tan hết rồi"},
				{"agent", "Dạ em rất xin lỗi anh/chị về trải nghiệm không tốt này ạ. Em sẽ kiểm tra lại quy trình giao hàng ngay ạ."},
				{"customer", "Lần trước cũng bị rồi đó, giao chậm quá"},
				{"agent", "Dạ em thành thật xin lỗi anh/chị. Để bù đắp, em xin gửi anh/chị 1 voucher giảm 50% cho đơn tiếp theo ạ. Đồng thời em sẽ báo lại bộ phận giao hàng để cải thiện thời gian giao ạ."},
				{"customer", "Ok vậy gửi voucher đi"},
				{"agent", "Dạ em đã gửi mã voucher SEPAY50 qua tin nhắn cho anh/chị rồi ạ. Mã có hiệu lực 7 ngày. Một lần nữa em xin lỗi vì sự bất tiện này. Anh/chị cần hỗ trợ gì thêm không ạ?"},
				{"customer", "Được rồi, cảm ơn"},
				{"agent", "Dạ cảm ơn anh/chị đã thông cảm ạ. Chúc anh/chị ngày tốt lành!"},
			},
		},
		// === QC PASS: Customer praise (x15) ===
		{count: 15, category: "qc_pass", verdict: "PASS", score: 95,
			review: "Khách hàng khen ngợi chất lượng đồ uống và dịch vụ. Nhân viên phản hồi chuyên nghiệp.",
			messages: []struct{ sender, content string }{
				{"customer", "Hôm qua mình uống Latte ở quán ngon quá, khen thật!"},
				{"agent", "Chào anh/chị, cảm ơn anh/chị rất nhiều vì lời khen ạ! Đội ngũ barista bên em sẽ rất vui khi biết điều này 😊"},
				{"customer", "Không gian quán cũng đẹp, phục vụ nhiệt tình lắm"},
				{"agent", "Dạ cảm ơn anh/chị đã dành thời gian chia sẻ ạ! Bên em luôn cố gắng mang đến trải nghiệm tốt nhất cho khách hàng. Anh/chị có thể đánh giá 5 sao trên Google Maps giúp bên em được không ạ? 🙏"},
				{"customer", "Ok để mình rate cho"},
				{"agent", "Dạ cảm ơn anh/chị rất nhiều ạ! Hẹn gặp lại anh/chị ở SePay Coffee nhé. Chúc anh/chị cuối tuần vui vẻ ạ! ☕"},
			},
		},
		// === QC FAIL: Slow response, rude (x10) ===
		{count: 10, category: "qc_fail", verdict: "FAIL", score: 25,
			review: "Nhân viên phản hồi chậm (hơn 15 phút) và trả lời cộc lốc, không chào hỏi, không cung cấp đủ thông tin.",
			violations: []struct{ rule, evidence, severity string }{
				{"Chào hỏi lịch sự", "Nhân viên không chào hỏi khách trong tin nhắn đầu tiên, trả lời trực tiếp 'Hết rồi' mà không dùng ngôn ngữ lịch sự.", "NGHIEM_TRONG"},
				{"Thời gian phản hồi", "Khách nhắn lúc 14:02, nhân viên trả lời lúc 14:18 (16 phút), vượt quá quy định 5 phút.", "NGHIEM_TRONG"},
				{"Kết thúc chuyên nghiệp", "Nhân viên kết thúc chat mà không cảm ơn khách và không hỏi khách cần hỗ trợ gì thêm.", "CAN_CAI_THIEN"},
			},
			messages: []struct{ sender, content string }{
				{"customer", "Cho mình hỏi còn bánh tiramisu không?"},
				{"agent", "Hết rồi"},
				{"customer", "Vậy có bánh gì khác không? Mình muốn mua tráng miệng"},
				{"agent", "Có cheesecake với mousse"},
				{"customer", "Giá bao nhiêu vậy?"},
				{"agent", "Cheesecake 65k, mousse 55k"},
				{"customer", "Cho mình 1 cheesecake, giao tới 45 Lý Tự Trọng nhé"},
				{"agent", "Ok"},
			},
		},
		// === QC FAIL: No greeting, incomplete answer (x10) ===
		{count: 10, category: "qc_fail", verdict: "FAIL", score: 40,
			review: "Nhân viên không chào hỏi lịch sự, trả lời thiếu thông tin quan trọng khiến khách phải hỏi lại nhiều lần.",
			violations: []struct{ rule, evidence, severity string }{
				{"Chào hỏi lịch sự", "Nhân viên không có lời chào đầu tiên khi khách liên hệ.", "NGHIEM_TRONG"},
				{"Giải đáp đầy đủ", "Khách hỏi về khuyến mãi nhưng nhân viên chỉ trả lời 'Có' mà không cung cấp chi tiết chương trình.", "CAN_CAI_THIEN"},
			},
			messages: []struct{ sender, content string }{
				{"customer", "Quán có chương trình khuyến mãi gì không ạ?"},
				{"agent", "Có ạ"},
				{"customer", "Vậy cụ thể là gì vậy?"},
				{"agent", "Mua 1 tặng 1 ạ"},
				{"customer", "Áp dụng cho món nào? Thời gian nào?"},
				{"agent", "Cà phê, thứ 3 hàng tuần"},
				{"customer", "Cà phê loại nào cũng được hả? Size nào?"},
				{"agent", "Dạ cà phê đen và cà phê sữa, size M"},
				{"customer", "Ok cảm ơn, vậy cho mình 2 cà phê sữa đá size M thứ 3 này nhé"},
				{"agent", "Dạ ok ạ"},
			},
		},
		// === QC SKIP: Short/sticker chat (x10) ===
		{count: 10, category: "qc_skip", verdict: "SKIP", score: 0,
			review: "Cuộc chat quá ngắn, chỉ có lời chào hoặc sticker, không đủ nội dung để đánh giá.",
			messages: []struct{ sender, content string }{
				{"customer", "Hello"},
				{"agent", "Chào anh/chị, SePay Coffee xin nghe ạ!"},
			},
		},
		// === Classification: Hỏi menu / Đặt bàn (x25) ===
		{count: 25, category: "class_menu", verdict: "PASS",
			classTags: []string{"Hỏi menu / Đặt bàn"}, classEvidence: []string{"Khách hỏi thực đơn và giá các loại cà phê, trà sữa."},
			messages: []struct{ sender, content string }{
				{"customer", "Hi quán ơi, cho mình xem menu với"},
				{"agent", "Chào anh/chị, đây là menu bên em ạ:\n☕ Cà phê: 35k-55k\n🧋 Trà sữa: 40k-60k\n🍹 Nước ép: 45k-55k\n🍰 Bánh ngọt: 45k-75k\nAnh/chị muốn order gì ạ?"},
				{"customer", "Trà sữa trân châu đường đen bao nhiêu?"},
				{"agent", "Dạ Trà sữa trân châu đường đen:\n- Size M: 45k\n- Size L: 55k\nBên em có thêm topping trân châu trắng +10k, pudding +10k ạ."},
				{"customer", "Cho 2 ly size L nhé, 1 ly ít đường"},
				{"agent", "Dạ em ghi nhận:\n- 2 Trà sữa trân châu đường đen size L\n- 1 ly ít đường\nTổng: 110k ạ. Anh/chị lấy tại quán hay giao hàng ạ?"},
				{"customer", "Giao tới 78 Bạch Đằng, quận Bình Thạnh nhé"},
				{"agent", "Dạ em đã ghi nhận đơn giao tới 78 Bạch Đằng, Q. Bình Thạnh ạ. Khoảng 20-25 phút sẽ tới nơi. Cảm ơn anh/chị! 🧋"},
			},
		},
		// === Classification: Khiếu nại (x15) ===
		{count: 15, category: "class_complaint", verdict: "PASS",
			classTags: []string{"Khiếu nại"}, classEvidence: []string{"Khách phàn nàn về chất lượng đồ uống và thời gian phục vụ chậm."},
			messages: []struct{ sender, content string }{
				{"customer", "Quán ơi, mình order ly matcha latte mà uống nhạt thếch, không có vị gì hết"},
				{"agent", "Dạ em rất xin lỗi anh/chị vì trải nghiệm không tốt ạ. Anh/chị order ở chi nhánh nào và lúc mấy giờ ạ để em kiểm tra lại?"},
				{"customer", "Chi nhánh Lê Văn Sỹ, mới order cách đây 30 phút"},
				{"agent", "Dạ em đã ghi nhận và sẽ kiểm tra lại công thức tại chi nhánh Lê Văn Sỹ ạ. Em xin phép gửi anh/chị 1 ly matcha latte mới hoàn toàn miễn phí để anh/chị thử lại ạ."},
				{"customer", "Ok gửi lại cho mình nhé, lần này pha đậm hơn giùm"},
				{"agent", "Dạ em sẽ ghi chú pha đậm hơn cho anh/chị ạ. Ly mới sẽ giao trong vòng 15-20 phút. Một lần nữa em xin lỗi vì sự bất tiện này ạ!"},
				{"customer", "Ừ cảm ơn"},
				{"agent", "Dạ cảm ơn anh/chị đã thông cảm. Nếu ly mới vẫn chưa hài lòng, anh/chị cứ báo em nhé ạ. Chúc anh/chị ngày tốt lành!"},
			},
		},
		// === Classification: Góp ý (x15) ===
		{count: 15, category: "class_feedback", verdict: "PASS",
			classTags: []string{"Góp ý"}, classEvidence: []string{"Khách đề xuất thêm món mới và cải tiến không gian quán."},
			messages: []struct{ sender, content string }{
				{"customer", "Mình thấy quán nên thêm mấy món smoothie bowl đi, giờ trend lắm"},
				{"agent", "Chào anh/chị, cảm ơn anh/chị đã chia sẻ góp ý ạ! Smoothie bowl là ý tưởng rất hay, bên em sẽ ghi nhận và chuyển cho bộ phận R&D ạ 😊"},
				{"customer", "Ừ, với lại quán nên có thêm ổ cắm điện ở bàn ngoài trời nữa, mình hay ngồi làm việc mà hết pin"},
				{"agent", "Dạ em ghi nhận luôn góp ý này ạ! Bên em đang có kế hoạch nâng cấp khu vực ngoài trời, sẽ bổ sung thêm ổ cắm điện. Dự kiến hoàn thành trong tháng tới ạ."},
				{"customer", "Ok good, quán cố gắng nhé 👍"},
				{"agent", "Dạ cảm ơn anh/chị rất nhiều vì những góp ý quý giá ạ! Mọi ý kiến đều giúp bên em ngày càng tốt hơn. Hẹn gặp lại anh/chị ạ! ☕"},
			},
		},
		// === Classification: Hỗ trợ chung (x20) ===
		{count: 20, category: "class_support", verdict: "PASS",
			classTags: []string{"Hỗ trợ chung"}, classEvidence: []string{"Khách hỏi về giờ mở cửa, wifi và chương trình thẻ thành viên."},
			messages: []struct{ sender, content string }{
				{"customer", "Quán mở cửa mấy giờ vậy?"},
				{"agent", "Chào anh/chị! SePay Coffee mở cửa từ 7:00 sáng đến 22:00 tối hàng ngày, kể cả cuối tuần và ngày lễ ạ."},
				{"customer", "Quán có wifi không? Password là gì?"},
				{"agent", "Dạ có wifi miễn phí ạ!\n- Tên wifi: SePayCoffee_Guest\n- Mật khẩu: sepay2024\nTốc độ 50Mbps, anh/chị dùng thoải mái nhé ạ!"},
				{"customer", "Mình thấy quán có thẻ thành viên phải không? Đăng ký sao vậy?"},
				{"agent", "Dạ đúng rồi ạ! Bên em có chương trình thẻ thành viên:\n- Tích điểm: 1.000đ = 1 điểm\n- 100 điểm = đổi 1 ly cà phê miễn phí\n- Sinh nhật: tặng 1 ly bất kỳ\nAnh/chị chỉ cần cung cấp SĐT tại quầy để đăng ký miễn phí ạ!"},
				{"customer", "Ok hay quá, lần tới mình đăng ký nhé"},
				{"agent", "Dạ tuyệt vời ạ! Hẹn gặp anh/chị tại quán. Cần hỗ trợ gì thêm cứ nhắn em nhé ạ! 😊"},
			},
		},
		// === Classification: Multi-tag (complaint + feedback) (x8) ===
		{count: 8, category: "class_complaint", verdict: "PASS",
			classTags: []string{"Khiếu nại", "Góp ý"}, classEvidence: []string{"Khách phàn nàn về chất lượng phục vụ.", "Khách đề xuất cải thiện quy trình order."},
			messages: []struct{ sender, content string }{
				{"customer", "Mình vào quán hôm qua, phải chờ 20 phút mới có đồ uống, quá lâu"},
				{"agent", "Dạ em rất xin lỗi anh/chị vì phải chờ lâu ạ. Hôm qua quán đông khách hơn bình thường, nhưng đó không phải lý do để anh/chị phải chờ lâu như vậy ạ."},
				{"customer", "Ừ mình thấy quán nên có hệ thống order online trước rồi đến lấy thôi, đỡ chờ"},
				{"agent", "Dạ đó là góp ý rất hay ạ! Bên em đang phát triển tính năng order online trên app, dự kiến ra mắt tháng sau. Anh/chị sẽ order trước và chỉ cần đến lấy thôi ạ."},
				{"customer", "Ừ vậy tốt, mong quán cải thiện nhé"},
				{"agent", "Dạ cảm ơn anh/chị đã góp ý ạ! Để xin lỗi vì hôm qua, em gửi anh/chị mã giảm 30% cho đơn tiếp theo: SORRY30. Hẹn gặp lại anh/chị ạ!"},
			},
		},
		// === Classification: SKIP (x10) ===
		{count: 10, category: "class_skip", verdict: "SKIP",
			messages: []struct{ sender, content string }{
				{"customer", "👋"},
				{"agent", "Chào anh/chị! SePay Coffee xin nghe ạ. Anh/chị cần hỗ trợ gì ạ?"},
			},
		},
		// === QC PASS: Technical support, patient (x8) ===
		{count: 8, category: "qc_pass", verdict: "PASS", score: 85,
			review: "Nhân viên hỗ trợ kỹ thuật kiên nhẫn, hướng dẫn chi tiết từng bước cho khách.",
			messages: []struct{ sender, content string }{
				{"customer", "Mình không đăng nhập được app SePay Coffee để tích điểm"},
				{"agent", "Chào anh/chị, em sẽ hỗ trợ anh/chị ạ. Anh/chị dùng SĐT nào để đăng ký thẻ thành viên ạ?"},
				{"customer", "0909123456"},
				{"agent", "Dạ em kiểm tra thì tài khoản anh/chị vẫn hoạt động bình thường ạ. Anh/chị thử:\n1. Đóng app hoàn toàn\n2. Mở lại app\n3. Chọn 'Đăng nhập bằng SĐT'\n4. Nhập mã OTP gửi về SĐT\nAnh/chị thử và báo lại em nhé ạ."},
				{"customer", "Ok để mình thử... À được rồi, cảm ơn nhé"},
				{"agent", "Dạ tuyệt vời ạ! Anh/chị hiện có 85 điểm tích lũy, thêm 15 điểm nữa là đổi được 1 ly miễn phí rồi ạ 😊. Cần hỗ trợ gì thêm anh/chị cứ nhắn em nhé!"},
			},
		},
		// === QC PASS: Promotion inquiry (x10) ===
		{count: 10, category: "qc_pass", verdict: "PASS", score: 90,
			review: "Nhân viên cung cấp đầy đủ thông tin khuyến mãi, tư vấn nhiệt tình.",
			messages: []struct{ sender, content string }{
				{"customer", "Quán đang có khuyến mãi gì không ạ?"},
				{"agent", "Chào anh/chị! Hiện SePay Coffee đang có chương trình:\n🎉 Mua 2 tặng 1 (áp dụng T2-T4, size M)\n🎂 Sinh nhật tháng: Giảm 50% 1 ly bất kỳ\n💳 Thanh toán qua app: Giảm 10%\nAnh/chị quan tâm chương trình nào ạ?"},
				{"customer", "Mua 2 tặng 1 áp dụng cho tất cả đồ uống hả?"},
				{"agent", "Dạ áp dụng cho tất cả đồ uống size M ạ. 2 ly có thể khác loại nhé. Ví dụ mua 1 cà phê sữa + 1 trà đào thì tặng thêm 1 ly size M bất kỳ ạ!"},
				{"customer", "Hay ghê, thứ 3 tuần này mình ghé nhé"},
				{"agent", "Dạ hẹn gặp anh/chị thứ 3 ạ! Nhớ rủ thêm bạn bè để tận dụng chương trình nhé 😄. Cần hỗ trợ gì thêm anh/chị cứ nhắn em!"},
			},
		},
		// === QC PASS: Delivery issue resolved (x8) ===
		{count: 8, category: "qc_pass", verdict: "PASS", score: 82,
			review: "Nhân viên xử lý vấn đề giao hàng nhanh chóng, đề xuất giải pháp hợp lý.",
			violations: []struct{ rule, evidence, severity string }{
				{"Thời gian phản hồi", "Tin nhắn thứ 3 của nhân viên phản hồi sau 6 phút, vượt nhẹ quy định 5 phút.", "CAN_CAI_THIEN"},
			},
			messages: []struct{ sender, content string }{
				{"customer", "Mình order giao hàng mà 45 phút chưa tới, sao chậm vậy?"},
				{"agent", "Dạ em xin lỗi anh/chị ạ! Em kiểm tra đơn hàng ngay. Cho em xin mã đơn hoặc SĐT đặt hàng ạ."},
				{"customer", "0987654321"},
				{"agent", "Dạ em đã check, đơn của anh/chị đang trên đường giao, shipper bị kẹt xe ở khu vực Nguyễn Thị Minh Khai ạ. Dự kiến thêm 10 phút nữa sẽ tới."},
				{"customer", "Lâu quá, đồ uống chắc nguội hết rồi"},
				{"agent", "Dạ em hiểu anh/chị bực mình ạ. Em xin phép miễn phí đơn này và gửi anh/chị voucher 30k cho lần sau ạ. Rất xin lỗi vì sự bất tiện!"},
				{"customer", "Ok cảm ơn, lần sau mong quán cải thiện"},
				{"agent", "Dạ em ghi nhận và sẽ cải thiện ạ. Mã voucher FREE30 đã gửi cho anh/chị. Cảm ơn anh/chị đã thông cảm! 🙏"},
			},
		},
		// === Classification: Menu + Support dual tag (x7) ===
		{count: 7, category: "class_menu", verdict: "PASS",
			classTags: []string{"Hỏi menu / Đặt bàn", "Hỗ trợ chung"}, classEvidence: []string{"Khách hỏi menu cà phê.", "Khách hỏi thêm về parking và giờ happy hour."},
			messages: []struct{ sender, content string }{
				{"customer", "Hi, cho hỏi quán có chỗ đỗ xe ô tô không?"},
				{"agent", "Chào anh/chị! Dạ chi nhánh Nguyễn Huệ có bãi đỗ xe ô tô phía sau quán (miễn phí), chi nhánh Lê Văn Sỹ chỉ có chỗ đỗ xe máy ạ. Anh/chị định ghé chi nhánh nào ạ?"},
				{"customer", "Nguyễn Huệ. Cho xem menu luôn nhé"},
				{"agent", "Dạ menu bên em:\n☕ Espresso based: 35-55k\n🍵 Trà: 35-50k\n🧋 Trà sữa: 40-60k\n🥤 Sinh tố: 50-65k\n🍰 Bánh ngọt: 45-75k\n\nHappy hour 14:00-16:00 giảm 20% tất cả đồ uống ạ!"},
				{"customer", "Oh happy hour hay ghê, chiều nay mình ghé"},
				{"agent", "Dạ hẹn gặp anh/chị chiều nay ạ! Nhớ đến trong khung 14:00-16:00 để được giảm giá nhé. Bãi đỗ xe phía sau, đi vào từ hẻm bên phải quán ạ 😊"},
			},
		},
	}
}
