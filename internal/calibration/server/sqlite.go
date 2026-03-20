package server

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"

	_ "github.com/mattn/go-sqlite3"

	"github.com/chris-regnier/gavel/internal/calibration"
)

// Compile-time assertion: SQLiteStore must implement EventStore.
var _ EventStore = (*SQLiteStore)(nil)

// SQLiteStore is a file-backed implementation of EventStore using SQLite.
// It persists calibration events and materialised per-team rule profiles.
// All public methods are safe for concurrent use; SQLite WAL mode allows
// concurrent readers alongside a single writer.
type SQLiteStore struct {
	db *sql.DB
}

// NewSQLiteStore opens (or creates) a SQLite database at path and runs schema
// migrations. WAL journal mode and a 5-second busy timeout are enabled so that
// concurrent writers do not immediately return SQLITE_BUSY errors.
//
// The caller is responsible for calling Close when done.
func NewSQLiteStore(path string) (*SQLiteStore, error) {
	db, err := sql.Open("sqlite3", path+"?_journal_mode=WAL&_busy_timeout=5000")
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	if err := migrate(db); err != nil {
		db.Close()
		return nil, fmt.Errorf("migrate: %w", err)
	}
	return &SQLiteStore{db: db}, nil
}

// migrate creates the required tables and indexes if they do not already exist.
// It is idempotent and safe to call on every startup.
func migrate(db *sql.DB) error {
	_, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS events (
			id         INTEGER PRIMARY KEY AUTOINCREMENT,
			team_id    TEXT NOT NULL,
			event_type TEXT NOT NULL,
			payload    TEXT NOT NULL,
			created_at TEXT NOT NULL DEFAULT (datetime('now'))
		);
		CREATE INDEX IF NOT EXISTS idx_events_team ON events(team_id, created_at);

		CREATE TABLE IF NOT EXISTS team_rule_profiles (
			team_id          TEXT NOT NULL,
			rule_id          TEXT NOT NULL,
			total_findings   INTEGER DEFAULT 0,
			useful_count     INTEGER DEFAULT 0,
			noise_count      INTEGER DEFAULT 0,
			wrong_count      INTEGER DEFAULT 0,
			mean_useful_conf REAL DEFAULT 0,
			mean_noise_conf  REAL DEFAULT 0,
			dismiss_rate     REAL DEFAULT 0,
			suppress_below   REAL DEFAULT 0,
			updated_at       TEXT,
			PRIMARY KEY (team_id, rule_id)
		);

		CREATE TABLE IF NOT EXISTS global_rule_stats (
			rule_id           TEXT PRIMARY KEY,
			total_teams       INTEGER DEFAULT 0,
			global_noise_rate REAL DEFAULT 0,
			global_conf_cal   REAL DEFAULT 0,
			by_file_type      TEXT DEFAULT '{}',
			updated_at        TEXT
		);
	`)
	return err
}

// AppendEvents stores a batch of calibration events for teamID atomically.
// All events in the batch are inserted within a single transaction; if any
// insertion fails the entire batch is rolled back and an error is returned.
func (s *SQLiteStore) AppendEvents(ctx context.Context, teamID string, events []calibration.Event) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck // rollback on error path; commit handles the success path

	stmt, err := tx.PrepareContext(ctx,
		"INSERT INTO events (team_id, event_type, payload, created_at) VALUES (?, ?, ?, ?)")
	if err != nil {
		return fmt.Errorf("prepare: %w", err)
	}
	defer stmt.Close()

	for _, e := range events {
		payload, err := json.Marshal(e.Payload)
		if err != nil {
			return fmt.Errorf("marshal payload: %w", err)
		}
		_, err = stmt.ExecContext(ctx, teamID, string(e.Type), string(payload),
			e.Timestamp.UTC().Format("2006-01-02T15:04:05Z"))
		if err != nil {
			return fmt.Errorf("insert event: %w", err)
		}
	}
	return tx.Commit()
}

// UpdateProfileFromFeedback applies a single feedback verdict to the
// materialised team_rule_profiles row for fb.RuleID and teamID. It uses an
// UPSERT so the first feedback for a rule creates the row and subsequent
// feedbacks increment the appropriate counter.
//
// Derived fields (NoiseRate, ConfCalibration, SuppressBelow) are NOT stored
// in the database; they are computed in-memory by RuleProfile.Recalculate
// when the profile is read back via GetTeamProfile.
func (s *SQLiteStore) UpdateProfileFromFeedback(ctx context.Context, teamID string, fb calibration.FeedbackPayload) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO team_rule_profiles
			(team_id, rule_id, total_findings, useful_count, noise_count, wrong_count, updated_at)
		VALUES (?, ?, 1,
			CASE WHEN ? = 'useful' THEN 1 ELSE 0 END,
			CASE WHEN ? = 'noise'  THEN 1 ELSE 0 END,
			CASE WHEN ? = 'wrong'  THEN 1 ELSE 0 END,
			datetime('now'))
		ON CONFLICT(team_id, rule_id) DO UPDATE SET
			total_findings = total_findings + 1,
			useful_count   = useful_count + CASE WHEN ? = 'useful' THEN 1 ELSE 0 END,
			noise_count    = noise_count  + CASE WHEN ? = 'noise'  THEN 1 ELSE 0 END,
			wrong_count    = wrong_count  + CASE WHEN ? = 'wrong'  THEN 1 ELSE 0 END,
			updated_at     = datetime('now')
	`, teamID, fb.RuleID,
		fb.Verdict, fb.Verdict, fb.Verdict, // INSERT values
		fb.Verdict, fb.Verdict, fb.Verdict) // UPDATE expressions
	return err
}

// GetTeamProfile returns the materialised calibration profile for each rule in
// ruleIDs that has at least one feedback event recorded for teamID. Rules with
// no profile are omitted from the result slice rather than returned as zero
// values. Derived fields are recomputed via RuleProfile.Recalculate before
// the profiles are returned.
//
// Returns nil, nil when ruleIDs is empty.
func (s *SQLiteStore) GetTeamProfile(ctx context.Context, teamID string, ruleIDs []string) ([]calibration.RuleProfile, error) {
	if len(ruleIDs) == 0 {
		return nil, nil
	}

	// Build a parameterised IN clause.
	placeholders := make([]string, len(ruleIDs))
	args := make([]interface{}, 0, len(ruleIDs)+1)
	args = append(args, teamID)
	for i, id := range ruleIDs {
		placeholders[i] = "?"
		args = append(args, id)
	}
	query := fmt.Sprintf(`
		SELECT rule_id, total_findings, useful_count, noise_count, wrong_count,
		       mean_useful_conf, mean_noise_conf, dismiss_rate, suppress_below
		FROM team_rule_profiles
		WHERE team_id = ? AND rule_id IN (%s)
	`, strings.Join(placeholders, ","))

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("query profiles: %w", err)
	}
	defer rows.Close()

	var profiles []calibration.RuleProfile
	for rows.Next() {
		var p calibration.RuleProfile
		p.TeamID = teamID
		// dismissRate is stored in the DB for future use but is not yet part
		// of the public RuleProfile type; scan it into a throwaway variable.
		var dismissRate float64
		if err := rows.Scan(
			&p.RuleID, &p.TotalFindings, &p.UsefulCount,
			&p.NoiseCount, &p.WrongCount, &p.MeanUsefulConf,
			&p.MeanNoiseConf, &dismissRate, &p.SuppressBelow,
		); err != nil {
			return nil, fmt.Errorf("scan profile: %w", err)
		}
		p.Recalculate()
		profiles = append(profiles, p)
	}
	return profiles, rows.Err()
}

// GetGlobalStats returns anonymised cross-org calibration signals for each
// rule in ruleIDs. This method is a placeholder for a future task; it
// currently returns nil, nil for all inputs.
func (s *SQLiteStore) GetGlobalStats(ctx context.Context, ruleIDs []string) (map[string]calibration.CrossOrgSignal, error) {
	// Placeholder — will be implemented in a later task (cross-org aggregation).
	return nil, nil
}

// DeleteTeamData removes all events and materialised profiles associated with
// teamID. It satisfies GDPR right-to-erasure obligations by purging data from
// both the events and team_rule_profiles tables.
func (s *SQLiteStore) DeleteTeamData(ctx context.Context, teamID string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	if _, err := tx.ExecContext(ctx, "DELETE FROM events WHERE team_id = ?", teamID); err != nil {
		return fmt.Errorf("delete events: %w", err)
	}
	if _, err := tx.ExecContext(ctx, "DELETE FROM team_rule_profiles WHERE team_id = ?", teamID); err != nil {
		return fmt.Errorf("delete profiles: %w", err)
	}
	return tx.Commit()
}

// Close releases the underlying database connection. No further method calls
// may be made on s after Close returns.
func (s *SQLiteStore) Close() error {
	return s.db.Close()
}
