package core

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
)

// BEIR and TREC normalize to the same Case/qrels/ranked-document vocabulary.
func (e *Engine) readRetrievalSource(cfg SourceConfig, rev DatasetRevision, p Plan) ([]Case, DatasetRevision, error) {
	total := 0
	read := func(rel string) ([]byte, error) {
		path, err := SafePath(e.Root, rel)
		if err != nil {
			return nil, err
		}
		b, err := ReadBounded(path, MaxFileBytes)
		if err != nil {
			return nil, err
		}
		total += len(b)
		if total > MaxFileBytes {
			return nil, errors.New("combined source exceeds 32 MiB")
		}
		rev.Files[rel] = Hash(b)
		return b, nil
	}
	documents := map[string]RetrievalDocument{}
	queries := map[string]string{}
	order := []string{}
	rows := func(rel string, consume func(map[string]any) error) error {
		b, err := read(rel)
		if err != nil {
			return err
		}
		scan := bufio.NewScanner(bytes.NewReader(b))
		scan.Buffer(make([]byte, 4096), 1<<20)
		line := 0
		for scan.Scan() {
			line++
			var row map[string]any
			if err = decodeJSON(scan.Bytes(), &row); err != nil {
				return fmt.Errorf("source row %d: invalid JSON", line)
			}
			if err = consume(row); err != nil {
				return fmt.Errorf("source row %d: %w", line, err)
			}
		}
		return scan.Err()
	}
	err := rows(cfg.Queries, func(row map[string]any) error {
		id, ok := row["_id"].(string)
		text, textOK := row["text"].(string)
		if !ok || id == "" || !textOK {
			return errors.New("query requires _id and text")
		}
		if _, ok = queries[id]; ok {
			return errors.New("duplicate query id")
		}
		queries[id] = text
		order = append(order, id)
		if len(order) > MaxCases {
			return errors.New("source exceeds 1000 queries")
		}
		return nil
	})
	if err != nil {
		return nil, rev, err
	}
	if cfg.Corpus != "" {
		err = rows(cfg.Corpus, func(row map[string]any) error {
			id, ok := row["_id"].(string)
			text, textOK := row["text"].(string)
			if !ok || id == "" || !textOK {
				return errors.New("document requires _id and text")
			}
			if _, ok = documents[id]; ok {
				return errors.New("duplicate document id")
			}
			documents[id] = RetrievalDocument{ID: id, Text: text}
			return nil
		})
		if err != nil {
			return nil, rev, err
		}
	}
	qrels := map[string]map[string]int{}
	b, err := read(cfg.Qrels)
	if err != nil {
		return nil, rev, err
	}
	scan := bufio.NewScanner(bytes.NewReader(b))
	line := 0
	for scan.Scan() {
		line++
		fields := strings.Fields(scan.Text())
		if len(fields) == 0 {
			continue
		}
		if cfg.Format == "beir" && line == 1 && fields[0] == "query-id" {
			continue
		}
		didx, gidx := 1, 2
		if cfg.Format == "trec" {
			didx, gidx = 2, 3
		}
		if len(fields) != gidx+1 {
			return nil, rev, fmt.Errorf("qrels row %d: wrong column count", line)
		}
		q, d := fields[0], fields[didx]
		grade, er := strconv.Atoi(fields[gidx])
		if er != nil || grade < 0 {
			return nil, rev, fmt.Errorf("qrels row %d: invalid relevance", line)
		}
		if _, ok := queries[q]; !ok {
			return nil, rev, errors.New("qrels references unknown query")
		}
		if cfg.Corpus != "" {
			if _, ok := documents[d]; !ok {
				return nil, rev, errors.New("qrels references unknown document")
			}
		}
		if qrels[q] == nil {
			qrels[q] = map[string]int{}
		}
		if _, ok := qrels[q][d]; ok {
			return nil, rev, errors.New("duplicate qrel")
		}
		qrels[q][d] = grade
	}
	if err = scan.Err(); err != nil {
		return nil, rev, err
	}
	type ranked struct {
		rank int
		doc  RetrievalDocument
	}
	rankings := map[string][]ranked{}
	if cfg.Run != "" {
		b, err = read(cfg.Run)
		if err != nil {
			return nil, rev, err
		}
		scan = bufio.NewScanner(bytes.NewReader(b))
		seen := map[string]bool{}
		for scan.Scan() {
			f := strings.Fields(scan.Text())
			if len(f) == 0 {
				continue
			}
			if len(f) != 6 {
				return nil, rev, errors.New("TREC run requires six columns")
			}
			q, d := f[0], f[2]
			rank, er := strconv.Atoi(f[3])
			score, se := strconv.ParseFloat(f[4], 64)
			if er != nil || se != nil || rank < 1 || math.IsNaN(score) || math.IsInf(score, 0) {
				return nil, rev, errors.New("invalid TREC rank/score")
			}
			if _, ok := queries[q]; !ok {
				return nil, rev, errors.New("run references unknown query")
			}
			for _, key := range []string{q + ":doc:" + d, q + ":rank:" + strconv.Itoa(rank)} {
				if seen[key] {
					return nil, rev, errors.New("duplicate run rank/document")
				}
				seen[key] = true
			}
			doc := documents[d]
			if cfg.Corpus != "" && doc.ID == "" {
				return nil, rev, errors.New("run references unknown document")
			}
			doc.ID = d
			doc.Score = &score
			rankings[q] = append(rankings[q], ranked{rank, doc})
		}
		if err = scan.Err(); err != nil {
			return nil, rev, err
		}
	}
	cases := []Case{}
	for _, q := range order {
		refs := map[string]any{}
		if qrels[q] != nil {
			refs["qrels"] = qrels[q]
		}
		docs := []RetrievalDocument{}
		sort.Slice(rankings[q], func(i, j int) bool { return rankings[q][i].rank < rankings[q][j].rank })
		for _, r := range rankings[q] {
			docs = append(docs, r.doc)
		}
		c := Case{ID: q, Inputs: map[string]any{"text": queries[q]}, References: refs, Metadata: map[string]any{"split": cfg.Split}}
		if p.Mode == "replay" {
			c.Result = &SubjectResult{Response: "", Retrieval: docs}
		}
		cases = append(cases, c)
	}
	if len(cases) == 0 {
		return nil, rev, errors.New("empty queries")
	}
	rev.Digest = Digest(struct {
		Revision DatasetRevision
		Cases    []Case
	}{rev, cases})
	return cases, rev, nil
}
