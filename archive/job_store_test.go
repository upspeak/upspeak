package archive

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/upspeak/upspeak/core"
)

func TestSaveAndGetJob(t *testing.T) {
	a := setupTestArchive(t)

	job := &core.Job{
		ID:        core.NewID(),
		RepoID:    core.NewID(),
		Type:      core.JobCollect,
		Status:    core.JobStatusPending,
		CreatedBy: testOwnerID,
	}

	if err := a.SaveJob(job); err != nil {
		t.Fatalf("SaveJob failed: %v", err)
	}
	if job.ShortID == "" {
		t.Fatal("expected short ID to be generated")
	}

	got, err := a.GetJob(job.ID)
	if err != nil {
		t.Fatalf("GetJob failed: %v", err)
	}
	if got.Type != core.JobCollect {
		t.Errorf("expected type 'collect', got '%s'", got.Type)
	}
	if got.Status != core.JobStatusPending {
		t.Errorf("expected status 'pending', got '%s'", got.Status)
	}
	if got.ShortID != job.ShortID {
		t.Errorf("expected short_id '%s', got '%s'", job.ShortID, got.ShortID)
	}
}

func TestSaveJob_Update(t *testing.T) {
	a := setupTestArchive(t)

	job := &core.Job{
		ID:        core.NewID(),
		RepoID:    core.NewID(),
		Type:      core.JobCollect,
		Status:    core.JobStatusPending,
		CreatedBy: testOwnerID,
	}
	if err := a.SaveJob(job); err != nil {
		t.Fatalf("SaveJob create failed: %v", err)
	}

	// Update to running.
	now := time.Now().UTC()
	job.Status = core.JobStatusRunning
	job.StartedAt = &now
	if err := a.SaveJob(job); err != nil {
		t.Fatalf("SaveJob update failed: %v", err)
	}

	got, err := a.GetJob(job.ID)
	if err != nil {
		t.Fatalf("GetJob failed: %v", err)
	}
	if got.Status != core.JobStatusRunning {
		t.Errorf("expected status 'running', got '%s'", got.Status)
	}
	if got.StartedAt == nil {
		t.Error("expected started_at to be set")
	}

	// Complete with result.
	completed := time.Now().UTC()
	job.Status = core.JobStatusCompleted
	job.CompletedAt = &completed
	job.Result = json.RawMessage(`{"nodes_created":12}`)
	if err := a.SaveJob(job); err != nil {
		t.Fatalf("SaveJob complete failed: %v", err)
	}

	got, err = a.GetJob(job.ID)
	if err != nil {
		t.Fatalf("GetJob failed: %v", err)
	}
	if got.Status != core.JobStatusCompleted {
		t.Errorf("expected status 'completed', got '%s'", got.Status)
	}
	if got.CompletedAt == nil {
		t.Error("expected completed_at to be set")
	}
	if string(got.Result) != `{"nodes_created":12}` {
		t.Errorf("expected result '{\"nodes_created\":12}', got '%s'", got.Result)
	}
}

func TestGetJobByShortID(t *testing.T) {
	a := setupTestArchive(t)

	job := &core.Job{
		ID:        core.NewID(),
		RepoID:    core.NewID(),
		Type:      core.JobSync,
		Status:    core.JobStatusPending,
		CreatedBy: testOwnerID,
	}
	if err := a.SaveJob(job); err != nil {
		t.Fatalf("SaveJob failed: %v", err)
	}

	got, err := a.GetJobByShortID(job.ShortID)
	if err != nil {
		t.Fatalf("GetJobByShortID failed: %v", err)
	}
	if got.ID != job.ID {
		t.Errorf("expected ID %s, got %s", job.ID, got.ID)
	}
}

func TestListJobs(t *testing.T) {
	a := setupTestArchive(t)

	repoID := core.NewID()

	// Create jobs with different types and statuses.
	jobs := []struct {
		jobType core.JobType
		status  core.JobStatus
	}{
		{core.JobCollect, core.JobStatusPending},
		{core.JobCollect, core.JobStatusCompleted},
		{core.JobPublish, core.JobStatusRunning},
	}

	for _, j := range jobs {
		job := &core.Job{
			ID:        core.NewID(),
			RepoID:    repoID,
			Type:      j.jobType,
			Status:    j.status,
			CreatedBy: testOwnerID,
		}
		if err := a.SaveJob(job); err != nil {
			t.Fatalf("SaveJob failed: %v", err)
		}
		// Update status if not pending (since SaveJob creates with pending).
		if j.status != core.JobStatusPending {
			job.Status = j.status
			if err := a.SaveJob(job); err != nil {
				t.Fatalf("SaveJob status update failed: %v", err)
			}
		}
	}

	// List all.
	all, total, err := a.ListJobs(core.JobListOptions{
		ListOptions: core.DefaultListOptions(),
	})
	if err != nil {
		t.Fatalf("ListJobs all failed: %v", err)
	}
	if total != 3 {
		t.Fatalf("expected 3 total, got %d", total)
	}
	if len(all) != 3 {
		t.Fatalf("expected 3 jobs, got %d", len(all))
	}

	// Filter by status.
	pending, total, err := a.ListJobs(core.JobListOptions{
		Status:      "pending",
		ListOptions: core.DefaultListOptions(),
	})
	if err != nil {
		t.Fatalf("ListJobs pending failed: %v", err)
	}
	if total != 1 {
		t.Fatalf("expected 1 pending, got %d", total)
	}
	if len(pending) != 1 {
		t.Fatalf("expected 1 pending job, got %d", len(pending))
	}

	// Filter by type.
	collectJobs, total, err := a.ListJobs(core.JobListOptions{
		Type:        "collect",
		ListOptions: core.DefaultListOptions(),
	})
	if err != nil {
		t.Fatalf("ListJobs collect failed: %v", err)
	}
	if total != 2 {
		t.Fatalf("expected 2 collect jobs, got %d", total)
	}
	if len(collectJobs) != 2 {
		t.Fatalf("expected 2 collect jobs, got %d", len(collectJobs))
	}

	// Filter by repo.
	repoJobs, total, err := a.ListJobs(core.JobListOptions{
		RepoID:      repoID.String(),
		ListOptions: core.DefaultListOptions(),
	})
	if err != nil {
		t.Fatalf("ListJobs repo failed: %v", err)
	}
	if total != 3 {
		t.Fatalf("expected 3 repo jobs, got %d", total)
	}
	if len(repoJobs) != 3 {
		t.Fatalf("expected 3 repo jobs, got %d", len(repoJobs))
	}
}

func TestSaveJob_WithError(t *testing.T) {
	a := setupTestArchive(t)

	job := &core.Job{
		ID:        core.NewID(),
		RepoID:    core.NewID(),
		Type:      core.JobWebhook,
		Status:    core.JobStatusPending,
		CreatedBy: testOwnerID,
	}
	if err := a.SaveJob(job); err != nil {
		t.Fatalf("SaveJob failed: %v", err)
	}

	// Fail the job.
	errMsg := "connection timeout"
	now := time.Now().UTC()
	job.Status = core.JobStatusFailed
	job.Error = &errMsg
	job.CompletedAt = &now
	if err := a.SaveJob(job); err != nil {
		t.Fatalf("SaveJob fail update failed: %v", err)
	}

	got, err := a.GetJob(job.ID)
	if err != nil {
		t.Fatalf("GetJob failed: %v", err)
	}
	if got.Status != core.JobStatusFailed {
		t.Errorf("expected status 'failed', got '%s'", got.Status)
	}
	if got.Error == nil || *got.Error != "connection timeout" {
		t.Errorf("expected error 'connection timeout', got %v", got.Error)
	}
}
