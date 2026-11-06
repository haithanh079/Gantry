package db

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"
)

// LandscapeFinding is one row in landscape_findings.
type LandscapeFinding struct {
	ID         string         `json:"id"`
	Rule       string         `json:"rule"`
	Category   string         `json:"category"`
	Severity   string         `json:"severity"`
	SubjectRef string         `json:"subjectRef"`
	Details    map[string]any `json:"details"`
	FirstSeen  time.Time      `json:"firstSeen"`
	LastSeen   time.Time      `json:"lastSeen"`
	Status     string         `json:"status"`
}

// LandscapeFindingFilter selects findings for the API.
type LandscapeFindingFilter struct {
	Category string
	Severity string
	Rule     string
	Status   string
}

// UpsertLandscapeFindings stores the current result of one rule and resolves
// findings that are no longer produced.
func (d *DB) UpsertLandscapeFindings(ctx context.Context, rule string, findings []LandscapeFinding) error {
	now := time.Now().UTC()
	desired := map[string]LandscapeFinding{}
	for _, finding := range findings {
		if finding.Details == nil {
			finding.Details = map[string]any{}
		}
		finding.Rule = rule
		raw, err := json.Marshal(finding.Details)
		if err != nil {
			return err
		}
		sum := sha256.Sum256(append([]byte(rule+"\n"+finding.SubjectRef+"\n"), raw...))
		finding.ID = hex.EncodeToString(sum[:])
		finding.LastSeen = now
		if finding.Status == "" {
			finding.Status = "open"
		}
		desired[finding.ID] = finding
	}

	rows, err := d.queryRows(ctx, `SELECT id, first_seen, status FROM landscape_findings WHERE rule = ?`, rule)
	if err != nil {
		return fmt.Errorf("listing findings for %s: %w", rule, err)
	}
	defer rows.Close()
	existing := map[string]time.Time{}
	for rows.Next() {
		var id, status string
		var first sql.NullTime
		if err := rows.Scan(&id, &first, &status); err != nil {
			return err
		}
		if first.Valid {
			existing[id] = first.Time
		} else {
			existing[id] = now
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}

	for id, finding := range desired {
		raw, _ := json.Marshal(finding.Details)
		if first, ok := existing[id]; ok {
			_, err = d.exec(ctx, `UPDATE landscape_findings
				SET category = ?, severity = ?, subject_ref = ?, details = ?, last_seen = ?, status = 'open'
				WHERE id = ?`, finding.Category, finding.Severity, finding.SubjectRef, string(raw), now, id)
			if err != nil {
				return err
			}
			_ = first
			continue
		}
		_, err = d.exec(ctx, `INSERT INTO landscape_findings
			(id, rule, category, severity, subject_ref, details, first_seen, last_seen, status)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, 'open')`,
			id, rule, finding.Category, finding.Severity, finding.SubjectRef, string(raw), now, now)
		if err != nil {
			return err
		}
	}
	for id := range existing {
		if _, ok := desired[id]; ok {
			continue
		}
		if _, err := d.exec(ctx, `UPDATE landscape_findings SET status = 'resolved', last_seen = ? WHERE id = ? AND status = 'open'`, now, id); err != nil {
			return err
		}
	}
	return nil
}

// ListLandscapeFindings returns findings matching the filter. Empty fields are ignored.
func (d *DB) ListLandscapeFindings(ctx context.Context, filter LandscapeFindingFilter) ([]LandscapeFinding, error) {
	query := `SELECT id, rule, category, severity, subject_ref, details, first_seen, last_seen, status
		FROM landscape_findings WHERE 1=1`
	var args []any
	if filter.Category != "" {
		query += ` AND category = ?`
		args = append(args, filter.Category)
	}
	if filter.Severity != "" {
		query += ` AND severity = ?`
		args = append(args, filter.Severity)
	}
	if filter.Rule != "" {
		query += ` AND rule = ?`
		args = append(args, filter.Rule)
	}
	if filter.Status != "" {
		query += ` AND status = ?`
		args = append(args, filter.Status)
	}
	query += ` ORDER BY severity, rule, subject_ref`
	rows, err := d.queryRows(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []LandscapeFinding
	for rows.Next() {
		var finding LandscapeFinding
		var details string
		var first, last sql.NullTime
		if err := rows.Scan(&finding.ID, &finding.Rule, &finding.Category, &finding.Severity, &finding.SubjectRef, &details, &first, &last, &finding.Status); err != nil {
			return nil, err
		}
		_ = json.Unmarshal([]byte(details), &finding.Details)
		if first.Valid {
			finding.FirstSeen = first.Time
		}
		if last.Valid {
			finding.LastSeen = last.Time
		}
		out = append(out, finding)
	}
	if out == nil {
		out = []LandscapeFinding{}
	}
	return out, rows.Err()
}
