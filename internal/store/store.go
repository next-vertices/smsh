// Package store persiste les campagnes dans SQLite (modernc.org/sqlite, sans CGO).
// La base est partagée entre le GUI et le CLI.
package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

// Statuts d'une campagne.
const (
	StatusPending   = "pending"   // créée, pas encore lancée
	StatusRunning   = "running"   // en cours d'envoi
	StatusPaused    = "paused"    // en pause (manuelle ou après une erreur bloquante)
	StatusCompleted = "completed" // terminée
	StatusCanceled  = "canceled"  // annulée
	StatusFailed    = "failed"    // échec complet (batch rejeté)
)

// Modes d'envoi d'une campagne.
const (
	ModeBatch = "batch" // batch OVH : même message pour tous, géré côté OVH
	ModeLocal = "local" // moteur local : un SMS par destinataire, avec variables
)

// Statuts d'un destinataire (mode local).
const (
	RecipientPending = "pending"
	RecipientSent    = "sent"
	RecipientFailed  = "failed"
)

// ErrNotFound : campagne inconnue.
var ErrNotFound = errors.New("campagne introuvable")

// Campaign est une campagne d'envoi.
type Campaign struct {
	ID         string     `json:"id"`
	Name       string     `json:"name"`
	Mode       string     `json:"mode"`
	AccountID  string     `json:"accountId"`
	Template   string     `json:"template"`
	Sender     string     `json:"sender"`
	StopClause bool       `json:"stopClause"`
	SendAt     *time.Time `json:"sendAt"`
	Rate       float64    `json:"rate"` // SMS/seconde (mode local)
	Status     string     `json:"status"`
	BatchID    string     `json:"batchId"`
	Total      int        `json:"total"`
	Sent       int        `json:"sent"`
	Failed     int        `json:"failed"`
	Credits    float64    `json:"credits"`
	Error      string     `json:"error"`
	CreatedAt  time.Time  `json:"createdAt"`
	UpdatedAt  time.Time  `json:"updatedAt"`
}

// Recipient est un destinataire d'une campagne locale.
type Recipient struct {
	Idx     int               `json:"idx"`
	Phone   string            `json:"phone"`
	Vars    map[string]string `json:"vars"`
	Message string            `json:"message"` // message final rendu
	Status  string            `json:"status"`
	Error   string            `json:"error"`
	OvhID   int64             `json:"ovhId"`
	Credits float64           `json:"credits"`
}

// Store est l'accès à la base.
type Store struct{ db *sql.DB }

const schema = `
CREATE TABLE IF NOT EXISTS campaigns (
	id TEXT PRIMARY KEY, name TEXT NOT NULL, mode TEXT NOT NULL, account_id TEXT NOT NULL,
	template TEXT NOT NULL, sender TEXT NOT NULL, stop_clause INTEGER NOT NULL, send_at TEXT,
	rate REAL NOT NULL, status TEXT NOT NULL, batch_id TEXT NOT NULL DEFAULT '',
	total INTEGER NOT NULL, sent INTEGER NOT NULL DEFAULT 0, failed INTEGER NOT NULL DEFAULT 0,
	credits REAL NOT NULL DEFAULT 0, error TEXT NOT NULL DEFAULT '',
	created_at TEXT NOT NULL, updated_at TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS recipients (
	campaign_id TEXT NOT NULL REFERENCES campaigns(id) ON DELETE CASCADE,
	idx INTEGER NOT NULL, phone TEXT NOT NULL, vars TEXT NOT NULL, message TEXT NOT NULL,
	status TEXT NOT NULL DEFAULT 'pending', error TEXT NOT NULL DEFAULT '',
	ovh_id INTEGER NOT NULL DEFAULT 0, credits REAL NOT NULL DEFAULT 0,
	PRIMARY KEY (campaign_id, idx)
);
CREATE INDEX IF NOT EXISTS recipients_status ON recipients(campaign_id, status, idx);
`

// Open ouvre (et migre) la base. path ":memory:" pour les tests.
func Open(path string) (*Store, error) {
	dsn := path
	if path != ":memory:" {
		dsn = "file:" + path + "?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=foreign_keys(1)"
	} else {
		dsn = "file::memory:?_pragma=foreign_keys(1)"
	}
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	// Une seule connexion : sérialise les écritures (et garde la base mémoire unique).
	db.SetMaxOpenConns(1)
	if _, err := db.Exec(schema); err != nil {
		db.Close()
		return nil, fmt.Errorf("migration SQLite : %w", err)
	}
	return &Store{db: db}, nil
}

func (s *Store) Close() error { return s.db.Close() }

const tsLayout = time.RFC3339Nano

func fmtTime(t time.Time) string { return t.UTC().Format(tsLayout) }

func parseTime(v string) time.Time {
	t, _ := time.Parse(tsLayout, v)
	return t
}

// CreateCampaign insère une campagne et ses destinataires en une transaction.
func (s *Store) CreateCampaign(ctx context.Context, c Campaign, rs []Recipient) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	now := time.Now()
	c.CreatedAt, c.UpdatedAt = now, now
	var sendAt any
	if c.SendAt != nil {
		sendAt = fmtTime(*c.SendAt)
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO campaigns
		(id,name,mode,account_id,template,sender,stop_clause,send_at,rate,status,batch_id,total,sent,failed,credits,error,created_at,updated_at)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		c.ID, c.Name, c.Mode, c.AccountID, c.Template, c.Sender, c.StopClause, sendAt, c.Rate, c.Status,
		c.BatchID, c.Total, c.Sent, c.Failed, c.Credits, c.Error, fmtTime(c.CreatedAt), fmtTime(c.UpdatedAt))
	if err != nil {
		return err
	}
	stmt, err := tx.PrepareContext(ctx, `INSERT INTO recipients (campaign_id,idx,phone,vars,message) VALUES (?,?,?,?,?)`)
	if err != nil {
		return err
	}
	defer stmt.Close()
	for _, r := range rs {
		vars, _ := json.Marshal(r.Vars)
		if _, err := stmt.ExecContext(ctx, c.ID, r.Idx, r.Phone, string(vars), r.Message); err != nil {
			return err
		}
	}
	return tx.Commit()
}

const campaignCols = `id,name,mode,account_id,template,sender,stop_clause,send_at,rate,status,batch_id,total,sent,failed,credits,error,created_at,updated_at`

type scanner interface{ Scan(...any) error }

func scanCampaign(r scanner) (Campaign, error) {
	var c Campaign
	var sendAt sql.NullString
	var created, updated string
	err := r.Scan(&c.ID, &c.Name, &c.Mode, &c.AccountID, &c.Template, &c.Sender, &c.StopClause, &sendAt, &c.Rate,
		&c.Status, &c.BatchID, &c.Total, &c.Sent, &c.Failed, &c.Credits, &c.Error, &created, &updated)
	if err != nil {
		return c, err
	}
	if sendAt.Valid {
		t := parseTime(sendAt.String)
		c.SendAt = &t
	}
	c.CreatedAt, c.UpdatedAt = parseTime(created), parseTime(updated)
	return c, nil
}

func (s *Store) GetCampaign(ctx context.Context, id string) (Campaign, error) {
	c, err := scanCampaign(s.db.QueryRowContext(ctx, `SELECT `+campaignCols+` FROM campaigns WHERE id=?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return c, ErrNotFound
	}
	return c, err
}

// ListCampaigns renvoie les campagnes, les plus récentes d'abord.
func (s *Store) ListCampaigns(ctx context.Context) ([]Campaign, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+campaignCols+` FROM campaigns ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Campaign{}
	for rows.Next() {
		c, err := scanCampaign(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// SetStatus change le statut (et le message d'erreur) d'une campagne.
func (s *Store) SetStatus(ctx context.Context, id, status, errMsg string) error {
	res, err := s.db.ExecContext(ctx, `UPDATE campaigns SET status=?, error=?, updated_at=? WHERE id=?`,
		status, errMsg, fmtTime(time.Now()), id)
	return affected(res, err)
}

// SetProgress met à jour compteurs et statut d'une campagne batch d'après OVH.
func (s *Store) SetProgress(ctx context.Context, id, status string, total, sent, failed int, credits float64, errMsg string) error {
	res, err := s.db.ExecContext(ctx, `UPDATE campaigns SET status=?, total=?, sent=?, failed=?, credits=?, error=?, updated_at=? WHERE id=?`,
		status, total, sent, failed, credits, errMsg, fmtTime(time.Now()), id)
	return affected(res, err)
}

// SetBatchID enregistre l'identifiant du batch OVH.
func (s *Store) SetBatchID(ctx context.Context, id, batchID string) error {
	res, err := s.db.ExecContext(ctx, `UPDATE campaigns SET batch_id=?, updated_at=? WHERE id=?`, batchID, fmtTime(time.Now()), id)
	return affected(res, err)
}

func affected(res sql.Result, err error) error {
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// NextPending renvoie les prochains destinataires à envoyer.
func (s *Store) NextPending(ctx context.Context, id string, limit int) ([]Recipient, error) {
	return s.queryRecipients(ctx, `SELECT idx,phone,vars,message,status,error,ovh_id,credits FROM recipients
		WHERE campaign_id=? AND status='pending' ORDER BY idx LIMIT ?`, id, limit)
}

// ListRecipients pagine les destinataires (status vide = tous) et renvoie le total filtré.
func (s *Store) ListRecipients(ctx context.Context, id, status string, offset, limit int) ([]Recipient, int, error) {
	where, args := `campaign_id=?`, []any{id}
	if status != "" {
		where += ` AND status=?`
		args = append(args, status)
	}
	var total int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM recipients WHERE `+where, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	rs, err := s.queryRecipients(ctx, `SELECT idx,phone,vars,message,status,error,ovh_id,credits FROM recipients WHERE `+where+
		` ORDER BY idx LIMIT ? OFFSET ?`, append(args, limit, offset)...)
	return rs, total, err
}

func (s *Store) queryRecipients(ctx context.Context, q string, args ...any) ([]Recipient, error) {
	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Recipient{}
	for rows.Next() {
		var r Recipient
		var vars string
		if err := rows.Scan(&r.Idx, &r.Phone, &vars, &r.Message, &r.Status, &r.Error, &r.OvhID, &r.Credits); err != nil {
			return nil, err
		}
		_ = json.Unmarshal([]byte(vars), &r.Vars)
		out = append(out, r)
	}
	return out, rows.Err()
}

// MarkRecipient enregistre le résultat d'un envoi et met à jour les compteurs de la campagne.
func (s *Store) MarkRecipient(ctx context.Context, id string, idx int, status, errMsg string, ovhID int64, credits float64) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `UPDATE recipients SET status=?, error=?, ovh_id=?, credits=? WHERE campaign_id=? AND idx=?`,
		status, errMsg, ovhID, credits, id, idx); err != nil {
		return err
	}
	col := "sent"
	if status == RecipientFailed {
		col = "failed"
	}
	if _, err := tx.ExecContext(ctx, `UPDATE campaigns SET `+col+`=`+col+`+1, credits=credits+?, updated_at=? WHERE id=?`,
		credits, fmtTime(time.Now()), id); err != nil {
		return err
	}
	return tx.Commit()
}

// RetryFailed remet les destinataires en échec à « pending » et ajuste les compteurs.
func (s *Store) RetryFailed(ctx context.Context, id string) (int, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	res, err := tx.ExecContext(ctx, `UPDATE recipients SET status='pending', error='' WHERE campaign_id=? AND status='failed'`, id)
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	if _, err := tx.ExecContext(ctx, `UPDATE campaigns SET failed=0, updated_at=? WHERE id=?`, fmtTime(time.Now()), id); err != nil {
		return 0, err
	}
	return int(n), tx.Commit()
}

// DeleteCampaign supprime une campagne et ses destinataires.
func (s *Store) DeleteCampaign(ctx context.Context, id string) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM campaigns WHERE id=?`, id)
	return affected(res, err)
}

// ResetInterrupted passe en pause les campagnes restées « running » (application fermée en cours d'envoi).
func (s *Store) ResetInterrupted(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, `UPDATE campaigns SET status='paused', error=?, updated_at=? WHERE status='running' AND mode='local'`,
		"Interrompue (application fermée) : reprenez l'envoi.", fmtTime(time.Now()))
	return err
}

// Active indique si le statut est non terminal.
func Active(status string) bool {
	return strings.Contains(" "+StatusPending+" "+StatusRunning+" "+StatusPaused+" ", " "+status+" ")
}
