package db

import (
	"fmt"
	"log"
	"os"

	"github.com/nmtan2001/chat-quality-agent/db/models"
	_ "github.com/lib/pq"
	"gorm.io/driver/postgres"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

var DB *gorm.DB

func Connect(dsn string, isProduction bool) error {
	logLevel := logger.Info
	if isProduction {
		logLevel = logger.Warn
	}

	var err error
	var dialector gorm.Dialector

	// Detect database type from DSN or environment
	dbType := os.Getenv("DB_TYPE")
	if dbType == "" {
		// Auto-detect from DSN format
		if len(dsn) > 8 && dsn[:8] == "postgres:" {
			dbType = "postgres"
		} else {
			dbType = "mysql"
		}
	}

	switch dbType {
	case "postgres":
		dialector = postgres.Open(dsn)
	case "mysql":
		dialector = mysql.Open(dsn)
	default:
		dialector = mysql.Open(dsn)
	}

	DB, err = gorm.Open(dialector, &gorm.Config{
		Logger: logger.Default.LogMode(logLevel),
	})
	if err != nil {
		return fmt.Errorf("connect to database: %w", err)
	}

	sqlDB, err := DB.DB()
	if err != nil {
		return fmt.Errorf("get sql.DB: %w", err)
	}

	sqlDB.SetMaxIdleConns(10)
	sqlDB.SetMaxOpenConns(100)

	log.Println("Database connected successfully")
	return nil
}

func AutoMigrate() error {
	err := DB.AutoMigrate(
		&models.User{},
		&models.Tenant{},
		&models.UserTenant{},
		&models.Channel{},
		&models.Conversation{},
		&models.Message{},
		&models.Job{},
		&models.JobRun{},
		&models.JobResult{},
		&models.AppSetting{},
		&models.NotificationLog{},
		&models.AIUsageLog{},
		&models.OAuthClient{},
		&models.OAuthAuthorizationCode{},
		&models.OAuthToken{},
		&models.ActivityLog{},
	)
	if err != nil {
		return fmt.Errorf("auto-migrate: %w", err)
	}

	// Add unique constraints that GORM can't express directly
	addUniqueConstraints()

	log.Println("Database migration completed")
	return nil
}

func addUniqueConstraints() {
	constraints := []struct {
		table      string
		name       string
		columns    string
	}{
		{"channels", "uq_channel_tenant_type_ext", "tenant_id, channel_type, external_id"},
		{"conversations", "uq_conv_tenant_channel_ext", "tenant_id, channel_id, external_conversation_id"},
		{"messages", "uq_msg_tenant_conv_ext", "tenant_id, conversation_id, external_message_id"},
	}

	for _, c := range constraints {
		sql := fmt.Sprintf(
			"ALTER TABLE `%s` ADD UNIQUE INDEX `%s` (%s)",
			c.table, c.name, c.columns,
		)
		// Ignore errors if constraint already exists
		DB.Exec(sql)
	}
}

func Close() {
	if DB != nil {
		sqlDB, _ := DB.DB()
		if sqlDB != nil {
			sqlDB.Close()
		}
	}
}
