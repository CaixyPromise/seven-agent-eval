package core

import (
	"bufio"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// Retrieval source joins and uniqueness checks live on disk; only one query's
// references and ranked evidence are materialized at a time.
func (e *Engine) visitRetrievalSource(ctx context.Context, cfg SourceConfig, rev DatasetRevision, p Plan, emit func(Case) error) (DatasetRevision, error) {
	dir, err := os.MkdirTemp(e.Store.Root, ".retrieval-")
	if err != nil {
		return rev, err
	}
	defer os.RemoveAll(dir)
	db, err := sql.Open("sqlite", filepath.Join(dir, "joins.sqlite"))
	if err != nil {
		return rev, err
	}
	defer db.Close()
	db.SetMaxOpenConns(1)
	_, err = db.Exec(`PRAGMA cache_size=-2048; PRAGMA temp_store=FILE; PRAGMA foreign_keys=ON;
 CREATE TABLE queries(seq INTEGER PRIMARY KEY,id TEXT UNIQUE,text TEXT);
 CREATE TABLE documents(id TEXT PRIMARY KEY,body TEXT);
 CREATE TABLE qrels(q TEXT REFERENCES queries(id),d TEXT,grade INTEGER,PRIMARY KEY(q,d));
 CREATE TABLE ranked(q TEXT REFERENCES queries(id),d TEXT,rank INTEGER,score REAL,PRIMARY KEY(q,d),UNIQUE(q,rank));`)
	if err != nil {
		return rev, err
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return rev, err
	}
	defer tx.Rollback()
	var total int64
	lines := func(rel string, consume func([]byte, int) error) error {
		path, err := SafePath(e.Root, rel)
		if err != nil {
			return err
		}
		f, err := os.Open(path)
		if err != nil {
			return err
		}
		defer f.Close()
		stat, err := f.Stat()
		if err != nil {
			return err
		}
		if !stat.Mode().IsRegular() {
			return errors.New("source requires regular file")
		}
		limited := &io.LimitedReader{R: f, N: int64(MaxInputFileBytes) - total + 1}
		h := sha256.New()
		scan := bufio.NewScanner(io.TeeReader(limited, h))
		scan.Buffer(make([]byte, 4096), inputRecordBytes)
		for line := 1; scan.Scan(); line++ {
			if err := ctx.Err(); err != nil {
				return err
			}
			if err := consume(scan.Bytes(), line); err != nil {
				return fmt.Errorf("source row %d: %w", line, err)
			}
		}
		if err := scan.Err(); err != nil {
			return err
		}
		if limited.N == 0 {
			return errors.New("combined source exceeds current size limit")
		}
		total = int64(MaxInputFileBytes) + 1 - limited.N
		rev.Files[rel] = hex.EncodeToString(h.Sum(nil))
		return nil
	}
	count := 0
	err = lines(cfg.Queries, func(b []byte, _ int) error {
		var row map[string]any
		if err := decodeJSON(b, &row); err != nil {
			return errors.New("invalid query JSON")
		}
		id, ok := row["_id"].(string)
		value, textOK := row["text"].(string)
		if !ok || id == "" || !textOK {
			return errors.New("query requires _id and text")
		}
		count++
		if count > MaxCases {
			return errors.New("source exceeds current query limit")
		}
		if _, err := tx.ExecContext(ctx, "INSERT INTO queries VALUES(?,?,?)", count, id, value); err != nil {
			return errors.New("duplicate query id")
		}
		return nil
	})
	if err != nil {
		return rev, err
	}
	if count == 0 {
		return rev, errors.New("empty queries")
	}
	if cfg.Corpus != "" {
		err = lines(cfg.Corpus, func(b []byte, _ int) error {
			var row map[string]any
			if err := decodeJSON(b, &row); err != nil {
				return errors.New("invalid document JSON")
			}
			id, ok := row["_id"].(string)
			body, textOK := row["text"].(string)
			if !ok || id == "" || !textOK {
				return errors.New("document requires _id and text")
			}
			if _, err := tx.ExecContext(ctx, "INSERT INTO documents VALUES(?,?)", id, body); err != nil {
				return errors.New("duplicate document id")
			}
			return nil
		})
		if err != nil {
			return rev, err
		}
	}
	documentExists := func(id string) error {
		if cfg.Corpus == "" {
			return nil
		}
		var exists int
		if err := tx.QueryRowContext(ctx, "SELECT 1 FROM documents WHERE id=?", id).Scan(&exists); err != nil {
			return errors.New("unknown document")
		}
		return nil
	}
	err = lines(cfg.Qrels, func(b []byte, line int) error {
		fields := strings.Fields(string(b))
		if len(fields) == 0 {
			return nil
		}
		if cfg.Format == "beir" && line == 1 && fields[0] == "query-id" {
			return nil
		}
		di, gi := 1, 2
		if cfg.Format == "trec" {
			di, gi = 2, 3
		}
		if len(fields) != gi+1 {
			return errors.New("qrels wrong column count")
		}
		grade, err := strconv.Atoi(fields[gi])
		if err != nil || grade < 0 {
			return errors.New("invalid relevance")
		}
		if err := documentExists(fields[di]); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, "INSERT INTO qrels VALUES(?,?,?)", fields[0], fields[di], grade); err != nil {
			return errors.New("duplicate qrel or unknown query")
		}
		return nil
	})
	if err != nil {
		return rev, err
	}
	if cfg.Run != "" {
		err = lines(cfg.Run, func(b []byte, _ int) error {
			f := strings.Fields(string(b))
			if len(f) == 0 {
				return nil
			}
			if len(f) != 6 {
				return errors.New("TREC run requires six columns")
			}
			rank, er := strconv.Atoi(f[3])
			score, se := strconv.ParseFloat(f[4], 64)
			if er != nil || se != nil || rank < 1 || math.IsNaN(score) || math.IsInf(score, 0) {
				return errors.New("invalid TREC rank/score")
			}
			if err := documentExists(f[2]); err != nil {
				return err
			}
			if _, err := tx.ExecContext(ctx, "INSERT INTO ranked VALUES(?,?,?,?)", f[0], f[2], rank, score); err != nil {
				return errors.New("duplicate rank/document or unknown query")
			}
			return nil
		})
		if err != nil {
			return rev, err
		}
	}
	for seq := 1; seq <= count; seq++ {
		var id, query string
		if err := tx.QueryRowContext(ctx, "SELECT id,text FROM queries WHERE seq=?", seq).Scan(&id, &query); err != nil {
			return rev, err
		}
		refs := map[string]any{}
		qrels := map[string]int{}
		size := len(id) + len(query)
		rows, err := tx.QueryContext(ctx, "SELECT d,grade FROM qrels WHERE q=? ORDER BY d", id)
		if err != nil {
			return rev, err
		}
		err = func() error {
			defer rows.Close()
			for rows.Next() {
				var doc string
				var grade int
				if err := rows.Scan(&doc, &grade); err != nil {
					return err
				}
				size += len(doc) + 32
				if size > inputRecordBytes {
					return errors.New("query references exceed record limit")
				}
				qrels[doc] = grade
			}
			return rows.Err()
		}()
		if err != nil {
			return rev, err
		}
		if len(qrels) > 0 {
			refs["qrels"] = qrels
		}
		docs := []RetrievalDocument{}
		rows, err = tx.QueryContext(ctx, "SELECT r.d,COALESCE(d.body,''),r.score FROM ranked r LEFT JOIN documents d ON d.id=r.d WHERE r.q=? ORDER BY r.rank", id)
		if err != nil {
			return rev, err
		}
		err = func() error {
			defer rows.Close()
			for rows.Next() {
				var doc RetrievalDocument
				var score float64
				if err := rows.Scan(&doc.ID, &doc.Text, &score); err != nil {
					return err
				}
				size += len(doc.ID) + len(doc.Text) + 64
				if size > inputRecordBytes {
					return errors.New("query retrieval exceeds record limit")
				}
				doc.Score = &score
				docs = append(docs, doc)
			}
			return rows.Err()
		}()
		if err != nil {
			return rev, err
		}
		c := Case{ID: id, Inputs: map[string]any{"text": query}, References: refs, Metadata: map[string]any{"split": cfg.Split}}
		if p.Mode == "replay" {
			c.Result = &SubjectResult{Response: "", Retrieval: docs}
		}
		if err := emit(c); err != nil {
			return rev, err
		}
	}
	rev.Digest = Digest(rev)
	return rev, nil
}
