package engine

import (
	"context"
	"encoding/json"
	"log"
	"time"

	"github.com/go-co-op/gocron/v2"
	"github.com/nmtan2001/chat-quality-agent/config"
	"github.com/nmtan2001/chat-quality-agent/db"
	"github.com/nmtan2001/chat-quality-agent/db/models"
)

// Scheduler manages periodic tasks: channel sync, job analysis, output delivery.
type Scheduler struct {
	scheduler  gocron.Scheduler
	syncEngine *SyncEngine
	cfg        *config.Config
}

// defaultScheduler is the global scheduler instance, accessible from handlers.
var defaultScheduler *Scheduler

// SetDefaultScheduler sets the global scheduler (called once from main).
func SetDefaultScheduler(s *Scheduler) {
	defaultScheduler = s
}

// GetDefaultScheduler returns the global scheduler.
func GetDefaultScheduler() *Scheduler {
	return defaultScheduler
}

func NewScheduler(cfg *config.Config) (*Scheduler, error) {
	s, err := gocron.NewScheduler()
	if err != nil {
		return nil, err
	}

	return &Scheduler{
		scheduler:  s,
		syncEngine: NewSyncEngine(cfg),
		cfg:        cfg,
	}, nil
}

// Start begins the scheduler. Call this once at app startup.
func (s *Scheduler) Start() {
	// Check channels for sync every 5 minutes (per-channel interval in metadata)
	_, err := s.scheduler.NewJob(
		gocron.DurationJob(5*time.Minute),
		gocron.NewTask(s.syncAllChannelsTask),
		gocron.WithName("sync-all-channels"),
	)
	if err != nil {
		log.Printf("[scheduler] failed to create sync job: %v", err)
	}

	// Load and schedule cron-based analysis jobs
	s.loadCronJobs()

	// Safety net: mark any stuck "running" jobs as failed on startup
	cleanupStuckRuns()

	s.scheduler.Start()
	log.Println("[scheduler] started")
}

// cleanupStuckRuns marks any job_runs stuck in "running" status as failed.
// This happens when the app crashes or restarts while a job is processing.
func cleanupStuckRuns() {
	// On startup, any "running" job is stuck because the goroutine died with the previous process
	var stuckRuns []models.JobRun
	if err := db.DB.Where("status = ?", "running").Find(&stuckRuns).Error; err != nil {
		log.Printf("[scheduler] error querying stuck runs: %v", err)
		return
	}
	for _, run := range stuckRuns {
		now := time.Now()
		if err := db.DB.Model(&run).Updates(map[string]interface{}{
			"status":        "failed",
			"finished_at":   &now,
			"error_message": "Job bị gián đoạn do hệ thống khởi động lại. Vui lòng chạy lại.",
		}).Error; err != nil {
			log.Printf("[scheduler] error marking stuck run %s as failed: %v", run.ID, err)
		} else {
			log.Printf("[scheduler] marked stuck run %s as failed (started: %v)", run.ID, run.StartedAt)
		}
	}
	if len(stuckRuns) > 0 {
		log.Printf("[scheduler] cleaned up %d stuck job runs", len(stuckRuns))
	}
}

// Stop gracefully shuts down the scheduler.
func (s *Scheduler) Stop() {
	if err := s.scheduler.Shutdown(); err != nil {
		log.Printf("[scheduler] shutdown error: %v", err)
	}
	log.Println("[scheduler] stopped")
}

func (s *Scheduler) syncAllChannelsTask() {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	var chans []models.Channel
	db.DB.Where("is_active = true").Find(&chans)

	now := time.Now()
	synced := 0
	for _, ch := range chans {
		// Check per-channel sync interval from metadata
		interval := 15 // default 15 minutes
		if ch.Metadata != "" {
			var meta map[string]interface{}
			if json.Unmarshal([]byte(ch.Metadata), &meta) == nil {
				if si, ok := meta["sync_interval"]; ok {
					if v, ok := si.(float64); ok && v > 0 {
						interval = int(v)
					}
				}
			}
		}

		// Skip if last sync was too recent
		if ch.LastSyncAt != nil {
			elapsed := now.Sub(*ch.LastSyncAt)
			if elapsed < time.Duration(interval)*time.Minute {
				continue
			}
		}

		if err := s.syncEngine.SyncChannel(ctx, ch); err != nil {
			log.Printf("[scheduler] sync channel %s failed: %v", ch.Name, err)
			db.LogActivity(ch.TenantID, "", "system", "sync.error", "channel", ch.ID, "Sync failed: "+ch.Name, err.Error(), "")
		} else {
			synced++
			db.LogActivity(ch.TenantID, "", "system", "sync.completed", "channel", ch.ID, "Sync completed: "+ch.Name, "", "")
		}
	}
	if synced > 0 {
		log.Printf("[scheduler] synced %d/%d channels", synced, len(chans))
	}
}

// loadCronJobs loads active jobs with cron schedules and registers them.
func (s *Scheduler) loadCronJobs() {
	var jobs []models.Job
	db.DB.Where("is_active = true AND schedule_type = 'cron' AND schedule_cron != ''").Find(&jobs)

	for _, job := range jobs {
		j := job // capture
		_, err := s.scheduler.NewJob(
			gocron.CronJob(j.ScheduleCron, false),
			gocron.NewTask(func() {
				log.Printf("[scheduler] running analysis job %s (%s)", j.Name, j.ID)
				analyzer := NewAnalyzer(s.cfg)
				ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
				defer cancel()
				if _, err := analyzer.RunJob(ctx, j); err != nil {
					log.Printf("[scheduler] job %s failed: %v", j.Name, err)
				}
			}),
			gocron.WithName("job-"+j.ID),
		)
		if err != nil {
			log.Printf("[scheduler] failed to schedule job %s: %v", j.Name, err)
		}
	}

	log.Printf("[scheduler] loaded %d cron jobs", len(jobs))
}

// ReloadJobs removes all cron analysis jobs and reloads from DB.
// Call this after creating, updating, or deleting a job.
func (s *Scheduler) ReloadJobs() {
	// Remove existing cron analysis jobs
	jobs := s.scheduler.Jobs()
	for _, j := range jobs {
		if len(j.Name()) > 4 && j.Name()[:4] == "job-" {
			if err := s.scheduler.RemoveJob(j.ID()); err != nil {
				log.Printf("[scheduler] error removing job %s: %v", j.Name(), err)
			}
		}
	}
	// Reload from DB
	s.loadCronJobs()
	log.Println("[scheduler] cron jobs reloaded")
}

// TriggerAfterSyncJobs triggers all active after_sync jobs for a tenant+channel.
func (s *Scheduler) TriggerAfterSyncJobs(tenantID, channelID string) {
	var jobs []models.Job
	if err := db.DB.Where("tenant_id = ? AND is_active = true AND schedule_type = 'after_sync'",
		tenantID).Find(&jobs).Error; err != nil {
		log.Printf("[scheduler] error querying after_sync jobs: %v", err)
		return
	}

	for _, job := range jobs {
		// Check if this job uses the synced channel
		var channelIDs []string
		if err := json.Unmarshal([]byte(job.InputChannelIDs), &channelIDs); err != nil {
			continue
		}
		found := false
		for _, id := range channelIDs {
			if id == channelID {
				found = true
				break
			}
		}
		if !found {
			continue
		}

		j := job // capture
		log.Printf("[scheduler] after-sync trigger: job=%s tenant=%s channel=%s", j.Name, tenantID, channelID)
		go func() {
			defer func() {
				if r := recover(); r != nil {
					log.Printf("[security] panic in after-sync job %s: %v", j.Name, r)
				}
			}()
			analyzer := NewAnalyzer(s.cfg)
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
			defer cancel()
			if _, err := analyzer.RunJob(ctx, j); err != nil {
				log.Printf("[scheduler] after-sync job %s failed: %v", j.Name, err)
			}
		}()
	}
}

// SyncEngine returns the sync engine for manual trigger.
func (s *Scheduler) SyncEngine() *SyncEngine {
	return s.syncEngine
}
