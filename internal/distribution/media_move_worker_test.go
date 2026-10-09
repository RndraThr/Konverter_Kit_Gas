package distribution

import (
	"bytes"
	"context"
	"errors"
	"io"
	"sync"
	"testing"
	"time"
)

type mediaMoveRepositoryFake struct {
	mu             sync.Mutex
	jobs           []MediaMoveJob
	claimNow       time.Time
	claimLease     time.Duration
	completeErrs   []error
	completedID    string
	completedGen   int64
	failedID       string
	failedGen      int64
	failedMessage  string
	failedNext     time.Time
	newestGen      int64
	newerQueued    bool
	processingLock time.Time
}

func (r *mediaMoveRepositoryFake) ClaimMediaMove(_ context.Context, now time.Time, lease time.Duration) (MediaMoveJob, bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.claimNow, r.claimLease = now, lease
	if !r.processingLock.IsZero() && !r.processingLock.Before(now.Add(-lease)) {
		return MediaMoveJob{}, false, nil
	}
	if len(r.jobs) == 0 {
		return MediaMoveJob{}, false, nil
	}
	job := r.jobs[0]
	r.jobs = r.jobs[1:]
	return job, true, nil
}

func (r *mediaMoveRepositoryFake) CompleteMediaMove(_ context.Context, mediaID string, generation int64) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.completedID, r.completedGen = mediaID, generation
	if generation != r.newestGen && r.newestGen != 0 {
		r.newerQueued = true
	}
	if len(r.completeErrs) == 0 {
		return nil
	}
	err := r.completeErrs[0]
	r.completeErrs = r.completeErrs[1:]
	return err
}

func (r *mediaMoveRepositoryFake) FailMediaMove(_ context.Context, mediaID string, generation int64, message string, next time.Time) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.failedID, r.failedGen, r.failedMessage, r.failedNext = mediaID, generation, message, next
	return nil
}

type movableStorageFake struct {
	moveCalls  int
	moveErr    error
	movedKey   string
	movedPath  []string
	movedName  string
	deleteCall int
	putCalls   int
}

func (s *movableStorageFake) Put(_ context.Context, key string, _ []string, source io.Reader) (string, int64, string, error) {
	s.putCalls++
	n, err := io.Copy(io.Discard, source)
	return key, n, "checksum", err
}
func (s *movableStorageFake) Open(context.Context, string) (io.ReadCloser, error) {
	return io.NopCloser(bytes.NewReader(nil)), nil
}
func (s *movableStorageFake) Delete(context.Context, string) error            { s.deleteCall++; return nil }
func (s *movableStorageFake) EnsureFolders(context.Context, [][]string) error { return nil }
func (s *movableStorageFake) Move(_ context.Context, key string, path []string, filename string) error {
	s.moveCalls++
	s.movedKey, s.movedPath, s.movedName = key, append([]string(nil), path...), filename
	return s.moveErr
}

func testMoveJob(generation int64) MediaMoveJob {
	return MediaMoveJob{MediaFileID: "media-1", StorageKey: "drive-file-1", TargetPath: []string{"PETANI", "WAJO", "20 Oktober 2026", "1"}, TargetFilename: "AHMAD - FOTO MESIN - 01.jpg", TargetGeneration: generation, Attempts: 1}
}

func TestMediaMoveWorkerMarksNewestGenerationFinal(t *testing.T) {
	repository := &mediaMoveRepositoryFake{jobs: []MediaMoveJob{testMoveJob(2)}, newestGen: 2}
	storage := &movableStorageFake{}
	worker := NewMediaMoveWorker(repository, storage, MediaMoveWorkerOptions{})
	processed, err := worker.ProcessOne(context.Background())
	if err != nil || !processed {
		t.Fatalf("processed=%v err=%v", processed, err)
	}
	if storage.moveCalls != 1 || repository.completedID != "media-1" || repository.completedGen != 2 {
		t.Fatalf("moves=%d completed=%s/%d", storage.moveCalls, repository.completedID, repository.completedGen)
	}
	if storage.movedName != "AHMAD - FOTO MESIN - 01.jpg" {
		t.Fatalf("movedName=%q, want the job's final filename", storage.movedName)
	}
}

func TestMediaMoveWorkerRetriesDriveFailureWithoutLosingFile(t *testing.T) {
	now := time.Now().UTC()
	repository := &mediaMoveRepositoryFake{jobs: []MediaMoveJob{testMoveJob(1)}, newestGen: 1}
	storage := &movableStorageFake{moveErr: errors.New("drive timeout")}
	worker := NewMediaMoveWorker(repository, storage, MediaMoveWorkerOptions{MaxBackoff: 15 * time.Minute})
	worker.now = func() time.Time { return now }
	processed, err := worker.ProcessOne(context.Background())
	if err != nil || !processed {
		t.Fatalf("processed=%v err=%v", processed, err)
	}
	if repository.failedID != "media-1" || repository.failedGen != 1 || repository.failedMessage == "" || !repository.failedNext.After(now) {
		t.Fatalf("failure=%s/%d message=%q next=%v", repository.failedID, repository.failedGen, repository.failedMessage, repository.failedNext)
	}
	if storage.deleteCall != 0 || storage.putCalls != 0 {
		t.Fatalf("file was copied/deleted on move failure: puts=%d deletes=%d", storage.putCalls, storage.deleteCall)
	}
}

func TestMediaMoveWorkerRepeatsAlreadyCompletedDriveMoveAfterCrash(t *testing.T) {
	job := testMoveJob(1)
	repository := &mediaMoveRepositoryFake{jobs: []MediaMoveJob{job, job}, newestGen: 1, completeErrs: []error{errors.New("database unavailable")}}
	storage := &movableStorageFake{}
	worker := NewMediaMoveWorker(repository, storage, MediaMoveWorkerOptions{})
	if processed, err := worker.ProcessOne(context.Background()); !processed || err == nil {
		t.Fatalf("first processed=%v err=%v", processed, err)
	}
	if processed, err := worker.ProcessOne(context.Background()); !processed || err != nil {
		t.Fatalf("second processed=%v err=%v", processed, err)
	}
	if storage.moveCalls != 2 || storage.putCalls != 0 {
		t.Fatalf("move calls=%d put calls=%d", storage.moveCalls, storage.putCalls)
	}
}

func TestMediaMoveWorkerReclaimsExpiredProcessingLease(t *testing.T) {
	now := time.Date(2026, 10, 7, 12, 0, 0, 0, time.UTC)
	repository := &mediaMoveRepositoryFake{jobs: []MediaMoveJob{testMoveJob(1)}, newestGen: 1, processingLock: now.Add(-6 * time.Minute)}
	worker := NewMediaMoveWorker(repository, &movableStorageFake{}, MediaMoveWorkerOptions{LeaseDuration: 5 * time.Minute})
	worker.now = func() time.Time { return now }
	processed, err := worker.ProcessOne(context.Background())
	if err != nil || !processed || repository.claimLease != 5*time.Minute {
		t.Fatalf("processed=%v lease=%v err=%v", processed, repository.claimLease, err)
	}
}

func TestMediaMoveWorkerLeavesNewerTargetQueued(t *testing.T) {
	repository := &mediaMoveRepositoryFake{jobs: []MediaMoveJob{testMoveJob(1)}, newestGen: 2}
	worker := NewMediaMoveWorker(repository, &movableStorageFake{}, MediaMoveWorkerOptions{})
	processed, err := worker.ProcessOne(context.Background())
	if err != nil || !processed || !repository.newerQueued {
		t.Fatalf("processed=%v newerQueued=%v err=%v", processed, repository.newerQueued, err)
	}
}

func TestMediaMoveWorkerRepositoryLeaseRetryAndGenerationSafety(t *testing.T) {
	pool := distributionIntegrationPool(t)
	fixture := seedMediaFixture(t, pool)
	ctx := context.Background()
	now := time.Now().UTC()
	must(t, func() error {
		_, err := pool.Exec(ctx, `UPDATE media_files SET storage_state='moving',storage_target_generation=1 WHERE id=$1`, fixture.mediaID)
		return err
	}())
	must(t, func() error {
		_, err := pool.Exec(ctx, `INSERT INTO distribution_media_move_jobs(media_file_id,target_path,target_generation,status,next_attempt_at) VALUES($1,ARRAY['PETANI','WAJO','20 Oktober 2026','1'],1,'queued',$2)`, fixture.mediaID, now)
		return err
	}())
	repository := NewRepository(pool)

	job, ok, err := repository.ClaimMediaMove(ctx, now, 5*time.Minute)
	if err != nil || !ok || job.Attempts != 1 || job.TargetGeneration != 1 {
		t.Fatalf("first claim job=%+v ok=%v err=%v", job, ok, err)
	}
	if _, ok, err := repository.ClaimMediaMove(ctx, now.Add(4*time.Minute), 5*time.Minute); err != nil || ok {
		t.Fatalf("unexpired lease reclaimed ok=%v err=%v", ok, err)
	}
	job, ok, err = repository.ClaimMediaMove(ctx, now.Add(6*time.Minute), 5*time.Minute)
	if err != nil || !ok || job.Attempts != 2 {
		t.Fatalf("expired lease job=%+v ok=%v err=%v", job, ok, err)
	}

	must(t, func() error {
		_, err := pool.Exec(ctx, `UPDATE media_files SET storage_target_generation=2,storage_state='moving' WHERE id=$1`, fixture.mediaID)
		return err
	}())
	must(t, func() error {
		_, err := pool.Exec(ctx, `UPDATE distribution_media_move_jobs SET target_generation=2,target_path=ARRAY['PETANI','WAJO','21 Oktober 2026','1'],status='queued',locked_at=NULL WHERE media_file_id=$1`, fixture.mediaID)
		return err
	}())
	if err := repository.CompleteMediaMove(ctx, fixture.mediaID, 1); err != nil {
		t.Fatal(err)
	}
	var state, jobStatus string
	var generation int64
	must(t, pool.QueryRow(ctx, `SELECT m.storage_state,j.status,j.target_generation FROM media_files m JOIN distribution_media_move_jobs j ON j.media_file_id=m.id WHERE m.id=$1`, fixture.mediaID).Scan(&state, &jobStatus, &generation))
	if state != "moving" || jobStatus != "queued" || generation != 2 {
		t.Fatalf("newer target state=%q job=%q generation=%d", state, jobStatus, generation)
	}

	job, ok, err = repository.ClaimMediaMove(ctx, now.Add(7*time.Minute), 5*time.Minute)
	if err != nil || !ok || job.TargetGeneration != 2 {
		t.Fatalf("newer claim job=%+v ok=%v err=%v", job, ok, err)
	}
	next := now.Add(8 * time.Minute)
	if err := repository.FailMediaMove(ctx, fixture.mediaID, 2, "drive timeout", next); err != nil {
		t.Fatal(err)
	}
	if _, ok, err := repository.ClaimMediaMove(ctx, next.Add(-time.Second), 5*time.Minute); err != nil || ok {
		t.Fatalf("retry claimed too early ok=%v err=%v", ok, err)
	}
	job, ok, err = repository.ClaimMediaMove(ctx, next, 5*time.Minute)
	if err != nil || !ok || job.Attempts < 2 {
		t.Fatalf("retry claim job=%+v ok=%v err=%v", job, ok, err)
	}
	if err := repository.CompleteMediaMove(ctx, fixture.mediaID, 2); err != nil {
		t.Fatal(err)
	}
	var jobs int
	must(t, pool.QueryRow(ctx, `SELECT storage_state FROM media_files WHERE id=$1`, fixture.mediaID).Scan(&state))
	must(t, pool.QueryRow(ctx, `SELECT count(*) FROM distribution_media_move_jobs WHERE media_file_id=$1`, fixture.mediaID).Scan(&jobs))
	if state != "final" || jobs != 0 {
		t.Fatalf("final state=%q jobs=%d", state, jobs)
	}
}
