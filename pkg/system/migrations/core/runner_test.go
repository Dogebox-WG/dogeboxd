package core

import (
	"testing"

	dogeboxd "github.com/Dogebox-WG/dogeboxd/pkg"
)

func testMigrationContext(t *testing.T) Context {
	t.Helper()

	return Context{
		Config: dogeboxd.ServerConfig{
			DataDir: t.TempDir(),
			TmpDir:  t.TempDir(),
		},
		Enqueue: func(dogeboxd.Action) string {
			return "job-id"
		},
	}
}

func TestRunMigrationsSkipsDoNotRun(t *testing.T) {
	ctx := testMigrationContext(t)
	if err := SaveState(ctx.Config, State{
		"skip_do_not_run": {
			DoNotRun: true,
		},
	}); err != nil {
		t.Fatalf("expected save to succeed, got %v", err)
	}

	ran := false
	jobID, queued, err := RunMigrations(ctx, []Migration{
		{
			Name:        "skip_do_not_run",
			DisplayName: "Skip Do Not Run",
			RunPolicy:   RunPolicy{MaxRuns: 1},
			Run: func(Context, MigrationRecord) (string, bool, error) {
				ran = true
				return "", false, nil
			},
		},
	})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if queued || jobID != "" {
		t.Fatalf("expected no queued migration, got queued=%v jobID=%q", queued, jobID)
	}
	if ran {
		t.Fatal("expected doNotRun migration to be skipped before Run")
	}
}

func TestEvaluateRunDecisionAllowsMissingRecord(t *testing.T) {
	ctx := testMigrationContext(t)

	decision, err := EvaluateRunDecision(ctx.Config, "test_migration", RunPolicy{MaxRuns: 1})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if !decision.ShouldRun {
		t.Fatalf("expected missing record to be runnable, got %+v", decision)
	}
}

func TestEvaluateRunDecisionSkipsWhenDoNotRunSet(t *testing.T) {
	ctx := testMigrationContext(t)
	if err := SaveState(ctx.Config, State{
		"test_migration": {
			DoNotRun: true,
		},
	}); err != nil {
		t.Fatalf("expected save to succeed, got %v", err)
	}

	decision, err := EvaluateRunDecision(ctx.Config, "test_migration", RunPolicy{MaxRuns: 1})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if decision.ShouldRun || decision.SkipReason == "" {
		t.Fatalf("expected doNotRun to skip with a reason, got %+v", decision)
	}
}

func TestEvaluateRunDecisionSkipsWhenRunLimitReached(t *testing.T) {
	ctx := testMigrationContext(t)
	if err := SaveState(ctx.Config, State{
		"test_migration": {
			Runs: 1,
		},
	}); err != nil {
		t.Fatalf("expected save to succeed, got %v", err)
	}

	decision, err := EvaluateRunDecision(ctx.Config, "test_migration", RunPolicy{MaxRuns: 1})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if decision.ShouldRun || decision.SkipReason == "" {
		t.Fatalf("expected run limit to skip with a reason, got %+v", decision)
	}
}

func TestRunMigrationsSkipsWhenMaxRunsReached(t *testing.T) {
	ctx := testMigrationContext(t)
	if err := SaveState(ctx.Config, State{
		"skip_max_runs": {
			Runs: 1,
		},
	}); err != nil {
		t.Fatalf("expected save to succeed, got %v", err)
	}

	ran := false
	jobID, queued, err := RunMigrations(ctx, []Migration{
		{
			Name:        "skip_max_runs",
			DisplayName: "Skip Max Runs",
			RunPolicy:   RunPolicy{MaxRuns: 1},
			Run: func(Context, MigrationRecord) (string, bool, error) {
				ran = true
				return "", false, nil
			},
		},
	})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if queued || jobID != "" {
		t.Fatalf("expected no queued migration, got queued=%v jobID=%q", queued, jobID)
	}
	if ran {
		t.Fatal("expected max-runs migration to be skipped before Run")
	}
}

func TestRunMigrationsRecordsRunOnlyWhenQueued(t *testing.T) {
	ctx := testMigrationContext(t)

	jobID, queued, err := RunMigrations(ctx, []Migration{
		{
			Name:        "records_run",
			DisplayName: "Records Run",
			RunPolicy:   RunPolicy{MaxRuns: 1},
			Run: func(Context, MigrationRecord) (string, bool, error) {
				return "job-records-run", true, nil
			},
		},
	})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if !queued || jobID != "job-records-run" {
		t.Fatalf("expected queued job, got queued=%v jobID=%q", queued, jobID)
	}

	state, err := LoadState(ctx.Config)
	if err != nil {
		t.Fatalf("expected load to succeed, got %v", err)
	}
	if state["records_run"].Runs != 1 {
		t.Fatalf("expected run count of 1, got %+v", state["records_run"])
	}
}

func TestRunMigrationsStopsAfterFirstQueuedMigration(t *testing.T) {
	ctx := testMigrationContext(t)

	secondRan := false
	jobID, queued, err := RunMigrations(ctx, []Migration{
		{
			Name:        "first",
			DisplayName: "First",
			RunPolicy:   RunPolicy{MaxRuns: 1},
			Run: func(Context, MigrationRecord) (string, bool, error) {
				return "job-first", true, nil
			},
		},
		{
			Name:        "second",
			DisplayName: "Second",
			RunPolicy:   RunPolicy{MaxRuns: 1},
			Run: func(Context, MigrationRecord) (string, bool, error) {
				secondRan = true
				return "", false, nil
			},
		},
	})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if !queued || jobID != "job-first" {
		t.Fatalf("expected first migration to queue, got queued=%v jobID=%q", queued, jobID)
	}
	if secondRan {
		t.Fatal("expected runner to stop after first queued migration")
	}
}

func TestRunMigrationsContinuesAfterNonQueuedMigration(t *testing.T) {
	ctx := testMigrationContext(t)

	secondRan := false
	jobID, queued, err := RunMigrations(ctx, []Migration{
		{
			Name:        "first_non_queued",
			DisplayName: "First Non Queued",
			RunPolicy:   RunPolicy{MaxRuns: 1},
			Run: func(Context, MigrationRecord) (string, bool, error) {
				return "", false, nil
			},
		},
		{
			Name:        "second_queued",
			DisplayName: "Second Queued",
			RunPolicy:   RunPolicy{MaxRuns: 1},
			Run: func(Context, MigrationRecord) (string, bool, error) {
				secondRan = true
				return "job-second", true, nil
			},
		},
	})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if !queued || jobID != "job-second" {
		t.Fatalf("expected second migration to queue, got queued=%v jobID=%q", queued, jobID)
	}
	if !secondRan {
		t.Fatal("expected runner to continue after non-queued migration")
	}
}
