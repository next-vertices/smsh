package store

import (
	"context"
	"strings"
	"time"
)

// Statuts d'un SMS de l'historique (déduits des dates renvoyées par OVH).
const (
	HistDelivered = "delivered" // accusé de réception de l'opérateur reçu
	HistSent      = "sent"      // envoyé, pas (encore) d'accusé de réception
	HistScheduled = "scheduled" // différé, pas encore parti
)

// HistoryItem est un SMS envoyé, mis en cache localement.
type HistoryItem struct {
	AccountID   string     `json:"accountId"`
	ID          int64      `json:"id"`
	CreatedAt   time.Time  `json:"createdAt"`
	SentAt      *time.Time `json:"sentAt"`
	DeliveredAt *time.Time `json:"deliveredAt"`
	Receiver    string     `json:"receiver"`
	Sender      string     `json:"sender"`
	Message     string     `json:"message"`
	Credits     float64    `json:"credits"`
	Parts       int        `json:"parts"`
	Tag         string     `json:"tag"`
	BatchID     string     `json:"batchId"`
	Receipt     int64      `json:"receipt"` // deliveryReceipt brut d'OVH
	PTT         int64      `json:"ptt"`     // code d'état brut d'OVH
	Differed    int64      `json:"differed"`
	Status      string     `json:"status"`
}

// Status déduit l'état d'un SMS de ses dates.
func (h HistoryItem) computeStatus() string {
	switch {
	case h.DeliveredAt != nil:
		return HistDelivered
	case h.SentAt == nil:
		return HistScheduled
	default:
		return HistSent
	}
}

// HistoryFilter filtre la recherche dans l'historique local.
type HistoryFilter struct {
	AccountID string     `json:"accountId"`
	Query     string     `json:"query"`    // texte libre : message, destinataire, expéditeur, tag
	Receiver  string     `json:"receiver"` // numéro (E.164 ou fragment)
	Status    string     `json:"status"`   // delivered | sent | scheduled
	Sender    string     `json:"sender"`
	Tag       string     `json:"tag"`
	From      *time.Time `json:"from"`
	To        *time.Time `json:"to"`
}

const historySchema = `
CREATE TABLE IF NOT EXISTS history (
	account_id TEXT NOT NULL, id INTEGER NOT NULL,
	created_at TEXT NOT NULL, sent_at TEXT NOT NULL DEFAULT '', delivered_at TEXT NOT NULL DEFAULT '',
	receiver TEXT NOT NULL, sender TEXT NOT NULL DEFAULT '', message TEXT NOT NULL DEFAULT '',
	credits REAL NOT NULL DEFAULT 0, parts INTEGER NOT NULL DEFAULT 1, tag TEXT NOT NULL DEFAULT '',
	batch_id TEXT NOT NULL DEFAULT '', receipt INTEGER NOT NULL DEFAULT 0, ptt INTEGER NOT NULL DEFAULT 0,
	differed INTEGER NOT NULL DEFAULT 0,
	PRIMARY KEY (account_id, id)
);
CREATE INDEX IF NOT EXISTS history_created ON history(account_id, created_at DESC);
CREATE INDEX IF NOT EXISTS history_receiver ON history(account_id, receiver);
`

func optTime(t *time.Time) string {
	if t == nil || t.IsZero() {
		return ""
	}
	return fmtTime(*t)
}

func parseOpt(v string) *time.Time {
	if v == "" {
		return nil
	}
	t := parseTime(v)
	return &t
}

// UpsertHistory insère ou met à jour des SMS dans le cache.
func (s *Store) UpsertHistory(ctx context.Context, items []HistoryItem) error {
	if len(items) == 0 {
		return nil
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	stmt, err := tx.PrepareContext(ctx, `INSERT INTO history
		(account_id,id,created_at,sent_at,delivered_at,receiver,sender,message,credits,parts,tag,batch_id,receipt,ptt,differed)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)
		ON CONFLICT(account_id,id) DO UPDATE SET
		 sent_at=excluded.sent_at, delivered_at=excluded.delivered_at, credits=excluded.credits,
		 receipt=excluded.receipt, ptt=excluded.ptt`)
	if err != nil {
		return err
	}
	defer stmt.Close()
	for _, h := range items {
		if _, err := stmt.ExecContext(ctx, h.AccountID, h.ID, fmtTime(h.CreatedAt), optTime(h.SentAt), optTime(h.DeliveredAt),
			h.Receiver, h.Sender, h.Message, h.Credits, h.Parts, h.Tag, h.BatchID, h.Receipt, h.PTT, h.Differed); err != nil {
			return err
		}
	}
	return tx.Commit()
}

const histCols = `account_id,id,created_at,sent_at,delivered_at,receiver,sender,message,credits,parts,tag,batch_id,receipt,ptt,differed`

func scanHistory(r scanner) (HistoryItem, error) {
	var h HistoryItem
	var created, sent, delivered string
	err := r.Scan(&h.AccountID, &h.ID, &created, &sent, &delivered, &h.Receiver, &h.Sender, &h.Message,
		&h.Credits, &h.Parts, &h.Tag, &h.BatchID, &h.Receipt, &h.PTT, &h.Differed)
	if err != nil {
		return h, err
	}
	h.CreatedAt, h.SentAt, h.DeliveredAt = parseTime(created), parseOpt(sent), parseOpt(delivered)
	h.Status = h.computeStatus()
	return h, nil
}

// like échappe % et _ pour un LIKE ... ESCAPE '\'.
func like(v string) string {
	r := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)
	return "%" + r.Replace(v) + "%"
}

func (f HistoryFilter) where() (string, []any) {
	w, args := []string{"account_id = ?"}, []any{f.AccountID}
	if q := strings.TrimSpace(f.Query); q != "" {
		w = append(w, `(message LIKE ? ESCAPE '\' OR receiver LIKE ? ESCAPE '\' OR sender LIKE ? ESCAPE '\' OR tag LIKE ? ESCAPE '\')`)
		l := like(q)
		args = append(args, l, l, l, l)
	}
	if v := strings.TrimSpace(f.Receiver); v != "" {
		w = append(w, `receiver LIKE ? ESCAPE '\'`)
		args = append(args, like(v))
	}
	if f.Sender != "" {
		w = append(w, "sender = ?")
		args = append(args, f.Sender)
	}
	if f.Tag != "" {
		w = append(w, "tag = ?")
		args = append(args, f.Tag)
	}
	switch f.Status {
	case HistDelivered:
		w = append(w, "delivered_at != ''")
	case HistSent:
		w = append(w, "delivered_at = '' AND sent_at != ''")
	case HistScheduled:
		w = append(w, "delivered_at = '' AND sent_at = ''")
	}
	if f.From != nil {
		w = append(w, "created_at >= ?")
		args = append(args, fmtTime(*f.From))
	}
	if f.To != nil {
		w = append(w, "created_at <= ?")
		args = append(args, fmtTime(*f.To))
	}
	return strings.Join(w, " AND "), args
}

// HistoryPage est une page de résultats avec les totaux du filtre.
type HistoryPage struct {
	Items   []HistoryItem `json:"items"`
	Total   int           `json:"total"`
	Credits float64       `json:"credits"` // crédits cumulés de tous les résultats du filtre
}

// QueryHistory recherche dans le cache, du plus récent au plus ancien.
func (s *Store) QueryHistory(ctx context.Context, f HistoryFilter, offset, limit int) (HistoryPage, error) {
	where, args := f.where()
	page := HistoryPage{Items: []HistoryItem{}}
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*), COALESCE(SUM(credits),0) FROM history WHERE `+where, args...).
		Scan(&page.Total, &page.Credits); err != nil {
		return page, err
	}
	rows, err := s.db.QueryContext(ctx, `SELECT `+histCols+` FROM history WHERE `+where+
		` ORDER BY created_at DESC, id DESC LIMIT ? OFFSET ?`, append(args, limit, offset)...)
	if err != nil {
		return page, err
	}
	defer rows.Close()
	for rows.Next() {
		h, err := scanHistory(rows)
		if err != nil {
			return page, err
		}
		page.Items = append(page.Items, h)
	}
	return page, rows.Err()
}

// HistoryIDs renvoie les identifiants des SMS correspondant au filtre (plafonné).
func (s *Store) HistoryIDs(ctx context.Context, f HistoryFilter, limit int) ([]int64, error) {
	where, args := f.where()
	rows, err := s.db.QueryContext(ctx, `SELECT id FROM history WHERE `+where+` ORDER BY created_at DESC LIMIT ?`, append(args, limit)...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	ids := []int64{}
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// GetHistory renvoie un SMS du cache.
func (s *Store) GetHistory(ctx context.Context, account string, id int64) (HistoryItem, error) {
	h, err := scanHistory(s.db.QueryRowContext(ctx, `SELECT `+histCols+` FROM history WHERE account_id=? AND id=?`, account, id))
	if err != nil {
		return h, ErrNotFound
	}
	return h, nil
}

// HistoryIDsSince renvoie les identifiants en cache créés depuis `from`.
func (s *Store) HistoryIDsSince(ctx context.Context, account string, from time.Time) (map[int64]bool, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id FROM history WHERE account_id=? AND created_at >= ?`, account, fmtTime(from))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[int64]bool{}
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out[id] = true
	}
	return out, rows.Err()
}

// HistoryUndelivered renvoie les SMS récents sans accusé de réception (à rafraîchir).
func (s *Store) HistoryUndelivered(ctx context.Context, account string, since time.Time, limit int) ([]int64, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id FROM history WHERE account_id=? AND delivered_at='' AND created_at >= ?
		ORDER BY created_at DESC LIMIT ?`, account, fmtTime(since), limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// DeleteHistory retire des SMS du cache.
func (s *Store) DeleteHistory(ctx context.Context, account string, ids []int64) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, id := range ids {
		if _, err := tx.ExecContext(ctx, `DELETE FROM history WHERE account_id=? AND id=?`, account, id); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// HistoryFacets renvoie les expéditeurs et tags présents (pour alimenter les filtres).
func (s *Store) HistoryFacets(ctx context.Context, account string) (senders, tags []string, err error) {
	get := func(col string) ([]string, error) {
		rows, err := s.db.QueryContext(ctx, `SELECT DISTINCT `+col+` FROM history WHERE account_id=? AND `+col+` != '' ORDER BY `+col, account)
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		out := []string{}
		for rows.Next() {
			var v string
			if err := rows.Scan(&v); err != nil {
				return nil, err
			}
			out = append(out, v)
		}
		return out, rows.Err()
	}
	if senders, err = get("sender"); err != nil {
		return
	}
	tags, err = get("tag")
	return
}
