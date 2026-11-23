package store

import (
	"bidhelm/internal/model"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"golang.org/x/crypto/bcrypt"
	_ "modernc.org/sqlite"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

type Store struct {
	DB *sql.DB
	mu sync.Mutex
}

func Open(path string) (*Store, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return nil, err
	}
	db, e := sql.Open("sqlite", path)
	if e != nil {
		return nil, e
	}
	db.SetMaxOpenConns(1)
	s := &Store{DB: db}
	if e = migrate(db); e != nil {
		db.Close()
		return nil, e
	}

	var raw []byte
	e = db.QueryRow("SELECT snapshot FROM configuration WHERE id=1").Scan(&raw)
	rebuild := e == sql.ErrNoRows
	if e != nil && !rebuild {
		db.Close()
		return nil, e
	}
	if !rebuild {
		var previous model.Snapshot
		if e = json.Unmarshal(raw, &previous); e != nil {
			db.Close()
			return nil, e
		}
		if previous.Schema < 1 || previous.Schema > model.Schema {
			db.Close()
			return nil, fmt.Errorf("unsupported saved snapshot schema %d", previous.Schema)
		}
		rebuild = previous.Schema < model.Schema
	}
	if rebuild {
		tx, err := db.Begin()
		if err != nil {
			db.Close()
			return nil, err
		}
		if err = publish(tx); err != nil {
			tx.Rollback()
			db.Close()
			return nil, err
		}
		if err = tx.Commit(); err != nil {
			db.Close()
			return nil, err
		}
	}
	e = nil

	return s, e
}

// migrate applies each schema revision transactionally and refuses silent downgrades.
func migrate(db *sql.DB) error {
	if _, err := db.Exec("PRAGMA journal_mode=WAL; PRAGMA foreign_keys=ON; PRAGMA busy_timeout=5000;"); err != nil {
		return err
	}
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err = tx.Exec("CREATE TABLE IF NOT EXISTS migrations(version INTEGER PRIMARY KEY)"); err != nil {
		return err
	}
	var version int
	if err = tx.QueryRow("SELECT COALESCE(MAX(version),0) FROM migrations").Scan(&version); err != nil {
		return err
	}
	if version > 1 {
		return fmt.Errorf("database schema %d is newer than supported schema 1; restore compatible backup", version)
	}
	if version == 0 {
		_, err = tx.Exec(`CREATE TABLE admin(id INTEGER PRIMARY KEY CHECK(id=1), login TEXT NOT NULL UNIQUE, password BLOB NOT NULL);
CREATE TABLE sessions(token TEXT PRIMARY KEY, csrf TEXT NOT NULL, expires INTEGER NOT NULL);
CREATE TABLE sources(id TEXT PRIMARY KEY, name TEXT NOT NULL, adapter TEXT NOT NULL);
CREATE TABLE campaigns(id TEXT PRIMARY KEY, data BLOB NOT NULL);
CREATE TABLE configuration(id INTEGER PRIMARY KEY CHECK(id=1), revision INTEGER NOT NULL, snapshot BLOB NOT NULL);
INSERT INTO migrations(version) VALUES(1);`)
		if err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *Store) Init(login, password string) error {
	if strings.TrimSpace(login) == "" || len(password) < 12 || len(password) > 72 {
		return errors.New("login required; password must contain 12–72 bytes")
	}
	hash, e := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if e != nil {
		return e
	}
	_, e = s.DB.Exec("INSERT INTO admin(id,login,password) VALUES(1,?,?)", login, hash)
	return e
}
func (s *Store) HasAdmin() bool {
	var n int
	return s.DB.QueryRow("SELECT count(*) FROM admin").Scan(&n) == nil && n > 0
}
func (s *Store) Password(password string) error {
	if len(password) < 12 || len(password) > 72 {
		return errors.New("password must contain 12–72 bytes")
	}
	h, e := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if e != nil {
		return e
	}
	tx, e := s.DB.Begin()
	if e != nil {
		return e
	}
	defer tx.Rollback()
	res, e := tx.Exec("UPDATE admin SET password=? WHERE id=1", h)
	if e != nil {
		return e
	}
	n, _ := res.RowsAffected()
	if n != 1 {
		return errors.New("admin is not initialized")
	}
	if _, e = tx.Exec("DELETE FROM sessions"); e != nil {
		return e
	}
	return tx.Commit()
}
func (s *Store) Authenticate(login, password string) bool {
	var hash []byte
	if e := s.DB.QueryRow("SELECT password FROM admin WHERE login=?", login).Scan(&hash); e != nil {
		return false
	}
	return bcrypt.CompareHashAndPassword(hash, []byte(password)) == nil
}
func (s *Store) NewSession() (string, string, error) {
	token, csrf := model.ID(), model.ID()
	_, e := s.DB.Exec("DELETE FROM sessions WHERE expires < ?", time.Now().Unix())
	if e != nil {
		return "", "", e
	}
	_, e = s.DB.Exec("INSERT INTO sessions VALUES(?,?,?)", token, csrf, time.Now().Add(12*time.Hour).Unix())
	return token, csrf, e
}
func (s *Store) Session(token string) (string, error) {
	var csrf string
	e := s.DB.QueryRow("SELECT csrf FROM sessions WHERE token=? AND expires>?", token, time.Now().Unix()).Scan(&csrf)
	return csrf, e
}
func (s *Store) Logout(token string) { s.DB.Exec("DELETE FROM sessions WHERE token=?", token) }
func (s *Store) Sources() ([]model.Source, error) {
	rows, e := s.DB.Query("SELECT id,name,adapter FROM sources ORDER BY name,id")
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []model.Source{}
	for rows.Next() {
		var v model.Source
		if e = rows.Scan(&v.ID, &v.Name, &v.Adapter); e != nil {
			return nil, e
		}
		out = append(out, v)
	}
	return out, rows.Err()
}
func (s *Store) Campaigns() ([]model.Campaign, error) {
	rows, e := s.DB.Query("SELECT data FROM campaigns ORDER BY id")
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []model.Campaign{}
	for rows.Next() {
		var b []byte
		if e = rows.Scan(&b); e != nil {
			return nil, e
		}
		var v model.Campaign
		if e = json.Unmarshal(b, &v); e != nil {
			return nil, e
		}
		out = append(out, v)
	}
	return out, rows.Err()
}
func (s *Store) SaveSource(v model.Source) error {
	if v.Name == "" || len(v.Name) > 200 || v.Adapter != model.AdapterEXADS {
		return errors.New("name and registered adapter required")
	}
	return s.mutate(func(tx *sql.Tx) error {
		_, e := tx.Exec("INSERT INTO sources VALUES(?,?,?)", v.ID, v.Name, v.Adapter)
		return e
	})
}
func (s *Store) SaveCampaign(v model.Campaign) error {
	normalized, err := model.NormalizeIP(v.Targeting.IP)
	if err != nil {
		return err
	}
	v.Targeting.IP = normalized
	placement, err := model.NormalizePlacement(v.Targeting.Placement)
	if err != nil {
		return err
	}
	v.Targeting.Placement = placement
	if v.Name == "" || len(v.Name) > 200 || !model.ValidURL(v.URL) || v.CPM <= 0 {
		return errors.New("name, HTTPS URL and positive CPM required")
	}
	if e := model.ValidateTarget(v.Targeting); e != nil {
		return e
	}
	b, e := json.Marshal(v)
	if e != nil {
		return e
	}
	return s.mutate(func(tx *sql.Tx) error {
		_, e := tx.Exec("INSERT INTO campaigns(id,data) VALUES(?,?) ON CONFLICT(id) DO UPDATE SET data=excluded.data", v.ID, b)
		return e
	})
}
func (s *Store) mutate(f func(*sql.Tx) error) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	tx, e := s.DB.Begin()
	if e != nil {
		return e
	}
	defer tx.Rollback()
	if e = f(tx); e != nil {
		return e
	}
	if e = publish(tx); e != nil {
		return e
	}
	return tx.Commit()
}
func publish(tx *sql.Tx) error {
	snap := model.Snapshot{Schema: model.Schema, Revision: 1, Sources: []model.Source{}, Campaigns: []model.Campaign{}}
	var rev int64
	e := tx.QueryRow("SELECT revision FROM configuration WHERE id=1").Scan(&rev)
	if e != nil && e != sql.ErrNoRows {
		return e
	}
	snap.Revision = rev + 1
	rows, e := tx.Query("SELECT id,adapter FROM sources ORDER BY id")
	if e != nil {
		return e
	}
	for rows.Next() {
		var v model.Source
		if e = rows.Scan(&v.ID, &v.Adapter); e != nil {
			rows.Close()
			return e
		}
		snap.Sources = append(snap.Sources, v)
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return e
	}
	rows, e = tx.Query("SELECT data FROM campaigns ORDER BY id")
	if e != nil {
		return e
	}
	for rows.Next() {
		var b []byte
		var v model.Campaign
		if e = rows.Scan(&b); e != nil {
			rows.Close()
			return e
		}
		if e = json.Unmarshal(b, &v); e != nil {
			rows.Close()
			return e
		}
		if v.Active {
			v.Name = ""
			snap.Campaigns = append(snap.Campaigns, v)
		}
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return e
	}
	b, e := json.Marshal(snap)
	if e != nil {
		return e
	}
	_, e = tx.Exec("INSERT INTO configuration VALUES(1,?,?) ON CONFLICT(id) DO UPDATE SET revision=excluded.revision,snapshot=excluded.snapshot", snap.Revision, b)
	return e
}
func (s *Store) Snapshot() ([]byte, int64, error) {
	var b []byte
	var rev int64
	e := s.DB.QueryRow("SELECT snapshot,revision FROM configuration WHERE id=1").Scan(&b, &rev)
	return b, rev, e
}
func (s *Store) Backup(path string) error {
	if _, e := os.Stat(path); e == nil {
		return errors.New("backup destination already exists")
	}
	_, e := s.DB.Exec(fmt.Sprintf("VACUUM INTO '%s'", strings.ReplaceAll(path, "'", "''")))
	return e
}
